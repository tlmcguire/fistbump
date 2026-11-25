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
		Kind     string `json:"kind"`
		Prompt   string `json:"prompt"`
		Content  string `json:"content"`
		Generate bool   `json:"generate"`
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
	if content == "" || body.Generate {
		job, err := s.store.GetJob(r.Context(), jobID)
		if err != nil {
			return storeError(err, "Job")
		}
		master, err := s.store.GetMaster(r.Context())
		if err != nil {
			return storeError(err, "Resume")
		}
		content = s.generateDraftText(job, master, kind, prompt)
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
