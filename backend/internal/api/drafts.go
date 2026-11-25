package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/models"
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
		Generate    bool   `json:"generate"`
	}
	if err := decodeJSONLimit(w, r, &body, maxFetchBody); err != nil {
		return err
	}
	kind := strings.ToLower(strings.TrimSpace(body.Kind))
	if kind == "" {
		kind = "qa"
	}
	if kind != "cover_letter" && kind != "qa" {
		return validationError("kind", "kind must be cover_letter or qa")
	}
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		if kind == "cover_letter" {
			prompt = "Cover letter"
		} else {
			return validationError("prompt", "Enter a question or prompt")
		}
	}
	content := strings.TrimSpace(body.Content)
	instruction := strings.TrimSpace(body.Instruction)
	baseContent := strings.TrimSpace(body.BaseContent)
	if instruction != "" || content == "" || body.Generate {
		job, err := s.store.GetJob(r.Context(), jobID)
		if err != nil {
			return storeError(err, "Job")
		}
		master, err := s.store.GetMaster(r.Context())
		if err != nil {
			return storeError(err, "Resume")
		}
		if instruction != "" {
			if baseContent == "" {
				baseContent = content
			}
			content = s.iterateDraftText(job, master, kind, prompt, baseContent, instruction)
		} else {
			content = s.generateDraftText(job, master, kind, prompt)
		}
	}
	created, err := s.store.CreateDraft(r.Context(), jobID, kind, prompt, content)
	if err != nil {
		return storeError(err, "Draft")
	}
	writeJSON(w, http.StatusCreated, created)
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

	// Q&A prompt response
	var b strings.Builder
	low := strings.ToLower(prompt)
	switch {
	case strings.Contains(low, "project") || strings.Contains(low, "proud"):
		if expLine != "" {
			b.WriteString(expLine)
		} else {
			fmt.Fprintf(&b, "In my work toward the %s role at %s, I have focused on delivering reliable, measurable results.", role, company)
		}
		if len(matched) > 0 {
			fmt.Fprintf(&b, " Key technologies I used that carry over directly include %s.", strings.Join(matched[:min(5, len(matched))], ", "))
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
