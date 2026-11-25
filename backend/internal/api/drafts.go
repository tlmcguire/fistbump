package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/render"
)

func (s *Server) draftList(w http.ResponseWriter, r *http.Request) error {
	jobID, err := pathID(r)
	if err != nil {
		return err
	}
	list, err := s.store.ListDrafts(r.Context(), jobID)
	if err != nil {
		return storeError(err, "Draft")
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) draftCreate(w http.ResponseWriter, r *http.Request) error {
	jobID, err := pathID(r)
	if err != nil {
		return err
	}
	var body struct {
		Kind        string `json:"kind"`
		Prompt      string `json:"prompt"`
		Content     string `json:"content"`
		BaseContent string `json:"base_content"`
		Instruction string `json:"instruction"`
		Mode        string `json:"mode"`
		Generate    bool   `json:"generate"`
	}
	if err := decodeJSONLimit(w, r, &body, maxFetchBody); err != nil {
		return err
	}
	kind := strings.ToLower(strings.TrimSpace(body.Kind))
	if kind == "" {
		kind = "qa"
	}
	mode := strings.ToLower(strings.TrimSpace(body.Mode))
	if mode == "" {
		mode = "deterministic"
	}
	if mode != "deterministic" && mode != "rules" && mode != "ai" {
		return validationError("mode", "mode must be deterministic or ai")
	}
	if kind == "questions" {
		job, err := s.store.GetJob(r.Context(), jobID)
		if err != nil {
			return storeError(err, "Job")
		}
		master, err := s.store.GetMaster(r.Context())
		if err != nil {
			return storeError(err, "Resume")
		}
		if mode == "ai" {
			qs, engine, err := s.generateQuestionsAI(r.Context(), job, master)
			if err != nil {
				if errors.Is(err, ai.ErrUnavailable) {
					return apiError{status: http.StatusServiceUnavailable, code: "ai_unavailable",
						message: "No local or remote AI model is ready. Configure one in Settings → AI suggestions, or switch to Deterministic mode."}
				}
				return apiError{status: http.StatusBadGateway, code: "ai_failed", message: err.Error()}
			}
			writeJSON(w, http.StatusOK, map[string]any{"questions": qs, "engine": engine})
			return nil
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"questions": s.generateQuestionsDeterministic(job, master),
			"engine":    ai.EngineRules,
		})
		return nil
	}
	if kind != "cover_letter" && kind != "qa" && kind != "interview_prep" {
		return validationError("kind", "kind must be cover_letter, qa, interview_prep, or questions")
	}
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		switch kind {
		case "cover_letter":
			prompt = "Cover letter"
		case "interview_prep":
			prompt = "Interview prep"
		default:
			return validationError("prompt", "Enter a question or prompt")
		}
	}
	content := strings.TrimSpace(body.Content)
	instruction := strings.TrimSpace(body.Instruction)
	baseContent := strings.TrimSpace(body.BaseContent)
	usedEngine := ""
	if instruction != "" || content == "" || body.Generate {
		job, err := s.store.GetJob(r.Context(), jobID)
		if err != nil {
			return storeError(err, "Job")
		}
		master, err := s.store.GetMaster(r.Context())
		if err != nil {
			return storeError(err, "Resume")
		}
		if mode == "ai" {
			var aiErr error
			if instruction != "" {
				if baseContent == "" {
					baseContent = content
				}
				content, usedEngine, aiErr = s.iterateDraftTextAI(r.Context(), job, master, kind, prompt, baseContent, instruction)
			} else {
				content, usedEngine, aiErr = s.generateDraftTextAI(r.Context(), job, master, kind, prompt)
			}
			if aiErr != nil {
				if errors.Is(aiErr, ai.ErrUnavailable) {
					return apiError{status: http.StatusServiceUnavailable, code: "ai_unavailable",
						message: "No local or remote AI model is ready. Configure one in Settings → AI suggestions, or switch to Instant mode."}
				}
				return apiError{status: http.StatusBadGateway, code: "ai_failed", message: aiErr.Error()}
			}
		} else if instruction != "" {
			if baseContent == "" {
				baseContent = content
			}
			content = s.iterateDraftText(job, master, kind, prompt, baseContent, instruction)
			usedEngine = ai.EngineRules
		} else {
			content = s.generateDraftText(job, master, kind, prompt)
			usedEngine = ai.EngineRules
		}
	}
	created, err := s.store.CreateDraft(r.Context(), jobID, kind, prompt, content)
	if err != nil {
		return storeError(err, "Draft")
	}
	writeJSON(w, http.StatusCreated, struct {
		models.Draft
		Engine string `json:"engine,omitempty"`
	}{Draft: created, Engine: usedEngine})
	return nil
}

func (s *Server) draftUpdate(w http.ResponseWriter, r *http.Request) error {
	jobID, err := pathID(r)
	if err != nil {
		return err
	}
	did, err := strconv.ParseInt(r.PathValue("did"), 10, 64)
	if err != nil || did < 1 {
		return validationFailed("Draft id is not valid")
	}
	var body struct {
		Prompt  string `json:"prompt"`
		Content string `json:"content"`
	}
	if err := decodeJSONLimit(w, r, &body, maxFetchBody); err != nil {
		return err
	}
	if strings.TrimSpace(body.Content) == "" {
		return validationError("content", "Draft content cannot be empty")
	}
	existing, err := s.store.GetDraft(r.Context(), jobID, did)
	if err != nil {
		return storeError(err, "Draft")
	}
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		prompt = existing.Prompt
	}
	updated, err := s.store.UpdateDraft(r.Context(), jobID, did, prompt, body.Content)
	if err != nil {
		return storeError(err, "Draft")
	}
	writeJSON(w, http.StatusOK, updated)
	return nil
}

func (s *Server) draftDelete(w http.ResponseWriter, r *http.Request) error {
	jobID, err := pathID(r)
	if err != nil {
		return err
	}
	did, err := strconv.ParseInt(r.PathValue("did"), 10, 64)
	if err != nil || did < 1 {
		return validationFailed("Draft id is not valid")
	}
	if err := s.store.DeleteDraft(r.Context(), jobID, did); err != nil {
		return storeError(err, "Draft")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// generateDraftText writes a resume-grounded cover letter or Q&A answer using only verified resume facts.
func (s *Server) generateDraftText(job models.Job, master models.Master, kind, prompt string) string {
	name := "Candidate"
	if master.Profile != nil && strings.TrimSpace(master.Profile.FullName) != "" {
		name = strings.TrimSpace(master.Profile.FullName)
	}
	company := strings.TrimSpace(job.CompanyName)
	if company == "" {
		company = "your team"
	}
	role := strings.TrimSpace(job.PositionTitle)
	if role == "" {
		role = "this role"
	}
	analysis := s.analyze(job, master)
	var matched []string
	for _, m := range analysis.Required.Tech.Matched {
		matched = append(matched, m.Skill)
	}
	for _, m := range analysis.Preferred.Tech.Matched {
		if len(matched) < 6 {
			matched = append(matched, m.Skill)
		}
	}
	var missing []string
	missing = append(missing, analysis.Required.Tech.Missing...)

	var expLine string
	if len(master.Experiences) > 0 {
		e := master.Experiences[0]
		expLine = fmt.Sprintf("Most recently as %s at %s", e.JobTitle, e.CompanyName)
		if e.Impact != nil && strings.TrimSpace(*e.Impact) != "" {
			expLine += ", " + strings.TrimSuffix(strings.TrimSpace(*e.Impact), ".") + "."
		} else if e.Description != nil && strings.TrimSpace(*e.Description) != "" {
			expLine += ", " + strings.TrimSuffix(strings.TrimSpace(*e.Description), ".") + "."
		} else {
			expLine += "."
		}
	}

	if kind == "cover_letter" {
		var b strings.Builder
		fmt.Fprintf(&b, "Dear %s Hiring Team,\n\n", company)
		fmt.Fprintf(&b, "I am writing to apply for the %s position at %s.", role, company)
		if len(matched) > 0 {
			fmt.Fprintf(&b, " My background aligns directly with your core requirements, including hands-on experience with %s.", strings.Join(matched[:min(5, len(matched))], ", "))
		}
		b.WriteString("\n\n")
		if expLine != "" {
			b.WriteString(expLine + "\n\n")
		}
		if len(missing) > 0 {
			fmt.Fprintf(&b, "While I do not yet have production experience with %s, I am actively learning and eager to ramp up quickly alongside the team.\n\n", strings.Join(missing[:min(3, len(missing))], ", "))
		}
		fmt.Fprintf(&b, "Thank you for your time and consideration. I would welcome the chance to discuss how my experience can contribute to %s.\n\nBest regards,\n%s", company, name)
		return b.String()
	}

	if kind == "interview_prep" {
		var b strings.Builder
		fmt.Fprintf(&b, "# Interview Prep: %s at %s\n\n", role, company)
		b.WriteString("## 60-Second Pitch\n")
		if master.Profile != nil && strings.TrimSpace(master.Profile.Summary) != "" {
			b.WriteString(strings.TrimSpace(master.Profile.Summary) + "\n")
		} else if expLine != "" {
			b.WriteString(expLine + "\n")
		} else {
			fmt.Fprintf(&b, "I'm %s, applying for the %s role at %s.\n", name, role, company)
		}
		if len(matched) > 0 {
			fmt.Fprintf(&b, "\n## Core Strengths to Highlight\n- Direct overlap with required skills: %s.\n", strings.Join(matched[:min(6, len(matched))], ", "))
		}
		for i, e := range master.Experiences {
			if i >= 2 {
				break
			}
			line := fmt.Sprintf("- %s at %s", e.JobTitle, e.CompanyName)
			if e.Impact != nil && strings.TrimSpace(*e.Impact) != "" {
				line += " — " + strings.TrimSpace(*e.Impact)
			}
			b.WriteString(line + "\n")
		}
		if len(missing) > 0 {
			fmt.Fprintf(&b, "\n## Gaps to Address Honestly\n- Skills listed in the posting that aren't on your resume yet: %s.\n- Framing: Be upfront about not having production experience yet, connect adjacent fundamentals you do know, and share how you're ramping up.\n", strings.Join(missing[:min(4, len(missing))], ", "))
		}
		b.WriteString("\n## Practice Questions (Use STAR: Situation, Task, Action, Result)\n")
		fmt.Fprintf(&b, "1. Why %s and why this %s role right now?\n", company, role)
		if len(matched) > 0 {
			fmt.Fprintf(&b, "2. Walk through a tough problem you solved using %s and how you measured the outcome.\n", strings.Join(matched[:min(2, len(matched))], " and "))
		} else {
			b.WriteString("2. Walk through a complex project you owned end-to-end and how you measured success.\n")
		}
		if len(missing) > 0 {
			fmt.Fprintf(&b, "3. How do you plan to ramp up on %s in your first 30–60 days?\n", missing[0])
		} else {
			b.WriteString("3. Tell me about a time you had to make a trade-off under tight constraints.\n")
		}
		b.WriteString("4. Describe a time you collaborated across teams or resolved a technical disagreement.\n")
		b.WriteString("\n## Questions to Ask the Interviewer\n")
		fmt.Fprintf(&b, "- What would success look like in the first 90 days for this %s role?\n", role)
		fmt.Fprintf(&b, "- What are the biggest technical or product challenges the team at %s is tackling this quarter?\n", company)
		b.WriteString("- How does the team collaborate on code reviews, design decisions, and feedback?\n")
		return strings.TrimSpace(b.String())
	}

	// Q&A prompt response
	var b strings.Builder
	low := strings.ToLower(prompt)
	switch {
	case strings.Contains(low, "project") || strings.Contains(low, "proud") || strings.Contains(low, "production"):
		if expLine != "" {
			b.WriteString(expLine)
		} else {
			fmt.Fprintf(&b, "In my work toward the %s role at %s, I have focused on delivering reliable, measurable results.", role, company)
		}
		if len(matched) > 0 {
			fmt.Fprintf(&b, " Key technologies I used that carry over directly include %s.", strings.Join(matched[:min(5, len(matched))], ", "))
		}
	case strings.Contains(low, "ramp") || strings.Contains(low, "learn") || strings.Contains(low, "gap") || strings.Contains(low, "missing"):
		if len(missing) > 0 {
			fmt.Fprintf(&b, "While I have not used %s in production yet, I am actively learning it and ramping up through hands-on practice.", strings.Join(missing[:min(2, len(missing))], " and "))
		} else {
			fmt.Fprintf(&b, "When ramping up on a new tool or stack for the %s role at %s, I start by connecting it to adjacent fundamentals and shipping a small scoped milestone early.", role, company)
		}
		if len(matched) > 0 {
			fmt.Fprintf(&b, " My existing experience with %s gives me a strong foundation to transfer quickly.", strings.Join(matched[:min(3, len(matched))], ", "))
		}
	case strings.Contains(low, "collaborat") || strings.Contains(low, "team") || strings.Contains(low, "remote") || strings.Contains(low, "hybrid"):
		fmt.Fprintf(&b, "I focus on clear written updates, early alignment on requirements, and constructive feedback so everyone on the %s team stays unblocked.", company)
		if expLine != "" {
			b.WriteString(" " + expLine)
		}
	default:
		fmt.Fprintf(&b, "I am excited about the %s role at %s because it builds directly on my experience", role, company)
		if len(matched) > 0 {
			fmt.Fprintf(&b, " with %s", strings.Join(matched[:min(5, len(matched))], ", "))
		}
		b.WriteString(".")
		if expLine != "" {
			b.WriteString(" " + expLine)
		}
		if len(missing) > 0 {
			fmt.Fprintf(&b, " I am also currently learning %s so I can hit the ground running.", strings.Join(missing[:min(3, len(missing))], ", "))
		}
	}
	return b.String()
}

// iterateDraftText refines an existing cover letter or Q&A draft according to the user's instruction
// while staying grounded in verified master resume facts.
func (s *Server) iterateDraftText(job models.Job, master models.Master, kind, prompt, baseContent, instruction string) string {
	base := strings.TrimSpace(baseContent)
	if base == "" {
		base = s.generateDraftText(job, master, kind, prompt)
	}
	low := strings.ToLower(instruction)
	company := strings.TrimSpace(job.CompanyName)
	if company == "" {
		company = "your team"
	}
	role := strings.TrimSpace(job.PositionTitle)
	if role == "" {
		role = "this role"
	}
	analysis := s.analyze(job, master)
	var matched []string
	for _, m := range analysis.Required.Tech.Matched {
		matched = append(matched, m.Skill)
	}

	switch {
	case strings.Contains(low, "short") || strings.Contains(low, "concise") || strings.Contains(low, "tight"):
		paras := splitParagraphs(base)
		if len(paras) >= 4 && kind == "cover_letter" {
			return strings.Join([]string{paras[0], paras[1], paras[len(paras)-1]}, "\n\n")
		}
		sentences := splitSentences(base)
		if len(sentences) > 2 {
			return strings.Join(sentences[:2], " ")
		}
		return base

	case strings.Contains(low, "detail") || strings.Contains(low, "longer") || strings.Contains(low, "expand"):
		var extra []string
		if len(master.Experiences) > 1 {
			e := master.Experiences[1]
			line := fmt.Sprintf("Previously as %s at %s", e.JobTitle, e.CompanyName)
			if e.Impact != nil && strings.TrimSpace(*e.Impact) != "" {
				line += ", " + strings.TrimSuffix(strings.TrimSpace(*e.Impact), ".") + "."
			} else if e.Description != nil && strings.TrimSpace(*e.Description) != "" {
				line += ", " + strings.TrimSuffix(strings.TrimSpace(*e.Description), ".") + "."
			} else {
				line += ", I built strong foundations that carry directly into this role."
			}
			extra = append(extra, line)
		}
		if len(matched) > 0 {
			extra = append(extra, fmt.Sprintf("Across these roles, I applied %s in day-to-day delivery.", strings.Join(matched[:min(5, len(matched))], ", ")))
		}
		if len(extra) == 0 {
			return base
		}
		return insertBeforeSignOff(base, strings.Join(extra, " "))

	case strings.Contains(low, "formal") || strings.Contains(low, "polish") || strings.Contains(low, "executive"):
		out := base
		out = strings.ReplaceAll(out, "Hi ", "Dear ")
		out = strings.ReplaceAll(out, "I'm ", "I am ")
		out = strings.ReplaceAll(out, "I'd ", "I would ")
		out = strings.ReplaceAll(out, "excited about", "keenly interested in")
		out = strings.ReplaceAll(out, "hit the ground running", "contribute effectively from day one")
		out = strings.ReplaceAll(out, "Thanks,", "Sincerely,")
		out = strings.ReplaceAll(out, "Best,", "Sincerely,")
		out = strings.ReplaceAll(out, "Best regards,", "Sincerely,")
		if kind == "cover_letter" && !strings.HasPrefix(out, "Dear ") {
			out = fmt.Sprintf("Dear %s Hiring Team,\n\n%s", company, out)
		}
		return out

	case strings.Contains(low, "conversational") || strings.Contains(low, "warm") || strings.Contains(low, "friendly"):
		out := base
		out = strings.ReplaceAll(out, "Dear "+company+" Hiring Team,", "Hi "+company+" team,")
		out = strings.ReplaceAll(out, "I am writing to apply for", "I'd love to apply for")
		out = strings.ReplaceAll(out, "I am excited about", "I'm genuinely excited about")
		out = strings.ReplaceAll(out, "Sincerely,", "Warmly,")
		out = strings.ReplaceAll(out, "Best regards,", "Thanks so much,")
		return out

	case strings.Contains(low, "impact") || strings.Contains(low, "metric") || strings.Contains(low, "result"):
		var impacts []string
		for _, e := range master.Experiences {
			if e.Impact != nil && strings.TrimSpace(*e.Impact) != "" {
				impacts = append(impacts, fmt.Sprintf("At %s (%s): %s", e.CompanyName, e.JobTitle, strings.TrimSpace(*e.Impact)))
			}
			if len(impacts) >= 2 {
				break
			}
		}
		if len(impacts) == 0 {
			return base
		}
		return insertBeforeSignOff(base, "Key measurable outcomes from my work include: "+strings.Join(impacts, " "))

	case strings.Contains(low, "opening") || strings.Contains(low, "hook") || strings.Contains(low, "stronger"):
		hook := fmt.Sprintf("With hands-on experience delivering results for roles like %s, I am eager to bring immediate value to %s.", role, company)
		if len(matched) > 0 {
			hook = fmt.Sprintf("With hands-on experience in %s, I am eager to bring immediate value as a %s at %s.",
				strings.Join(matched[:min(4, len(matched))], ", "), role, company)
		}
		paras := splitParagraphs(base)
		if len(paras) >= 2 && strings.HasPrefix(paras[0], "Dear ") {
			paras[1] = hook + " " + paras[1]
			return strings.Join(paras, "\n\n")
		}
		return hook + "\n\n" + base

	default:
		focus := strings.TrimSuffix(strings.TrimSpace(instruction), ".")
		note := fmt.Sprintf("In particular, regarding %s: my background for the %s role at %s", focus, role, company)
		if len(matched) > 0 {
			note += fmt.Sprintf(" combines practical work in %s with a track record of shipping reliable outcomes.", strings.Join(matched[:min(4, len(matched))], ", "))
		} else {
			note += " focuses on clear execution and measurable team impact."
		}
		return insertBeforeSignOff(base, note)
	}
}

func splitParagraphs(s string) []string {
	raw := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n\n")
	var out []string
	for _, p := range raw {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func splitSentences(s string) []string {
	flat := strings.Join(strings.Fields(s), " ")
	var out []string
	start := 0
	for i := 0; i < len(flat); i++ {
		if (flat[i] == '.' || flat[i] == '!' || flat[i] == '?') && (i+1 == len(flat) || flat[i+1] == ' ') {
			if seg := strings.TrimSpace(flat[start : i+1]); seg != "" {
				out = append(out, seg)
			}
			start = i + 1
		}
	}
	if rest := strings.TrimSpace(flat[start:]); rest != "" {
		out = append(out, rest)
	}
	return out
}

func insertBeforeSignOff(base, addition string) string {
	paras := splitParagraphs(base)
	if len(paras) >= 2 {
		last := strings.ToLower(paras[len(paras)-1])
		if strings.HasPrefix(last, "best") || strings.HasPrefix(last, "sincerely") || strings.HasPrefix(last, "warmly") || strings.HasPrefix(last, "thanks") {
			head := append([]string{}, paras[:len(paras)-1]...)
			head = append(head, addition, paras[len(paras)-1])
			return strings.Join(head, "\n\n")
		}
	}
	return base + "\n\n" + addition
}

func (s *Server) draftContextBlock(job models.Job, master models.Master) string {
	analysis := s.analyze(job, master)
	var matched []string
	for _, m := range analysis.Required.Tech.Matched {
		matched = append(matched, m.Skill)
	}
	var missing []string
	missing = append(missing, analysis.Required.Tech.Missing...)
	resumeText := strings.TrimSpace(render.Master(master))
	if len(resumeText) > 4000 {
		resumeText = resumeText[:4000]
	}
	jobText := ""
	if job.RawText != nil {
		jobText = strings.TrimSpace(*job.RawText)
	}
	if len(jobText) > 2500 {
		jobText = jobText[:2500]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Company: %s\nRole: %s\n", strings.TrimSpace(job.CompanyName), strings.TrimSpace(job.PositionTitle))
	if job.WorkMode != nil && strings.TrimSpace(*job.WorkMode) != "" {
		fmt.Fprintf(&b, "Work mode: %s\n", strings.TrimSpace(*job.WorkMode))
	}
	if len(matched) > 0 {
		fmt.Fprintf(&b, "Matched skills: %s\n", strings.Join(matched[:min(8, len(matched))], ", "))
	}
	if len(missing) > 0 {
		fmt.Fprintf(&b, "Missing required skills (candidate does NOT have production experience yet): %s\n", strings.Join(missing[:min(6, len(missing))], ", "))
	}
	if jobText != "" {
		fmt.Fprintf(&b, "\nJob description:\n%s\n", jobText)
	}
	if resumeText != "" {
		fmt.Fprintf(&b, "\nCandidate master resume (ONLY cite facts present here):\n%s\n", resumeText)
	}
	return b.String()
}

func (s *Server) generateQuestionsDeterministic(job models.Job, master models.Master) []string {
	company := strings.TrimSpace(job.CompanyName)
	if company == "" {
		company = "this company"
	}
	role := strings.TrimSpace(job.PositionTitle)
	if role == "" {
		role = "this role"
	}
	analysis := s.analyze(job, master)
	var matched []string
	for _, m := range analysis.Required.Tech.Matched {
		matched = append(matched, m.Skill)
	}
	missing := analysis.Required.Tech.Missing
	q2 := "Describe a project you're most proud of and how you measured its impact."
	if len(matched) >= 2 {
		q2 = fmt.Sprintf("Tell us about a time you used %s and %s in production.", matched[0], matched[1])
	} else if len(matched) == 1 {
		q2 = fmt.Sprintf("Walk through a concrete project where you used %s and how you measured the impact.", matched[0])
	}
	q3 := "Tell us about a tough technical trade-off you had to make under constraints."
	if len(missing) > 0 {
		q3 = fmt.Sprintf("This role mentions %s — how would you ramp up quickly?", strings.Join(missing[:min(2, len(missing))], " and "))
	}
	q4 := "Describe a time you collaborated across teams or resolved a disagreement."
	if job.WorkMode != nil && strings.TrimSpace(*job.WorkMode) != "" && !strings.EqualFold(*job.WorkMode, "Any") {
		q4 = fmt.Sprintf("How do you collaborate effectively and stay aligned in a %s team?", strings.ToLower(strings.TrimSpace(*job.WorkMode)))
	}
	return []string{
		fmt.Sprintf("Why are you interested in the %s role at %s?", role, company),
		q2,
		q3,
		q4,
		fmt.Sprintf("What excites you most about what %s is building right now?", company),
	}
}

func (s *Server) generateQuestionsAI(ctx context.Context, job models.Job, master models.Master) ([]string, string, error) {
	mode, _ := s.aiMode(ctx)
	msgs := []ai.Message{
		{
			Role: "system",
			Content: "You generate realistic job application and interview questions tailored to a specific role and candidate resume. " +
				"Return ONLY 5 concise, distinct questions, one per line, with no preamble or numbering.",
		},
		{
			Role:    "user",
			Content: "Generate 5 likely application or interview questions for this role:\n\n" + s.draftContextBlock(job, master),
		},
	}
	reply, engine, err := s.rounds.engines.Chat(ctx, mode, msgs, 400, false)
	if err != nil {
		return nil, "", err
	}
	var out []string
	for _, raw := range strings.Split(strings.ReplaceAll(reply, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		line = strings.TrimLeft(line, "-*•0123456789.) ")
		line = strings.TrimSpace(line)
		if len(line) < 10 {
			continue
		}
		out = append(out, line)
		if len(out) >= 6 {
			break
		}
	}
	if len(out) == 0 {
		return nil, "", errors.New("the AI model did not return readable questions — try again or switch to Instant mode")
	}
	return out, engine, nil
}

func (s *Server) generateDraftTextAI(ctx context.Context, job models.Job, master models.Master, kind, prompt string) (string, string, error) {
	mode, _ := s.aiMode(ctx)
	var task string
	switch kind {
	case "cover_letter":
		task = "Write a concise, genuine, well-structured cover letter (3–4 short paragraphs) tailored to this role."
	case "interview_prep":
		task = "Write a practical Markdown interview prep sheet with these sections: # Interview Prep, ## 60-Second Pitch, ## Core Strengths to Highlight, ## Gaps to Address Honestly, ## Practice Questions (STAR), and ## Questions to Ask the Interviewer."
	default:
		task = fmt.Sprintf("Write a clear, natural first-person answer to this application question: %q", prompt)
	}
	msgs := []ai.Message{
		{
			Role: "system",
			Content: "You help a job candidate draft honest, resume-grounded application materials. " +
				"NEVER invent employers, degrees, metrics, or skills that are not in the candidate's master resume. " +
				"If mentioning a missing required skill, frame it honestly as something they are learning or eager to ramp up on. " +
				"Output only the draft text itself with no extra commentary.",
		},
		{
			Role:    "user",
			Content: task + "\n\n" + s.draftContextBlock(job, master),
		},
	}
	reply, engine, err := s.rounds.engines.Chat(ctx, mode, msgs, 1200, false)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(reply) == "" {
		return "", "", errors.New("the AI model returned an empty reply — try again or switch to Instant mode")
	}
	return reply, engine, nil
}

func (s *Server) iterateDraftTextAI(ctx context.Context, job models.Job, master models.Master, kind, prompt, baseContent, instruction string) (string, string, error) {
	mode, _ := s.aiMode(ctx)
	msgs := []ai.Message{
		{
			Role: "system",
			Content: "You revise a job candidate's draft according to their instruction while staying 100% grounded in their master resume. " +
				"NEVER invent facts, metrics, or skills not present in the resume. Output only the revised draft text with no commentary.",
		},
		{
			Role: "user",
			Content: fmt.Sprintf("Draft type: %s\nPrompt: %s\nInstruction: %s\n\nCurrent draft:\n%s\n\n%s",
				kind, prompt, instruction, baseContent, s.draftContextBlock(job, master)),
		},
	}
	reply, engine, err := s.rounds.engines.Chat(ctx, mode, msgs, 1200, false)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(reply) == "" {
		return "", "", errors.New("the AI model returned an empty reply — try again or switch to Instant mode")
	}
	return reply, engine, nil
}
