package api

import (
	"net/http"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/render"
)

func (s *Server) getResume(w http.ResponseWriter, r *http.Request) error {
	m, err := s.st.GetMaster()
	if err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"profile": m.Profile, "experiences": m.Experiences, "education": m.Education, "skills": m.Skills()})
	return nil
}

func (s *Server) putProfile(w http.ResponseWriter, r *http.Request) error {
	var in models.Profile
	if err := decode(r, &in); err != nil {
		return err
	}
	in.FullName = strings.TrimSpace(in.FullName)
	if in.FullName == "" {
		return invalid("full_name", "full_name is required")
	}
	if in.PreferredMode != nil {
		switch *in.PreferredMode {
		case "Remote", "Hybrid", "On-site", "Any":
		default:
			return invalid("preferred_mode", "preferred_mode is not valid")
		}
	}
	if in.MinDesiredPay != nil && *in.MinDesiredPay < 0 {
		return invalid("min_desired_pay", "min_desired_pay must not be negative")
	}
	for _, p := range []**string{&in.Email, &in.Phone, &in.HomeLocation, &in.Summary} {
		blankToNil(p)
	}
	in.TargetPositions = cleanList(in.TargetPositions, false)
	in.ClearanceCerts = cleanList(in.ClearanceCerts, false)
	in.Skills = cleanList(in.Skills, true)
	in.Links = cleanList(in.Links, false)
	out, err := s.st.UpsertProfile(in)
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) listExperiences(w http.ResponseWriter, r *http.Request) error {
	out, err := s.st.ListExperiences()
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func validateExperience(e *models.Experience) error {
	e.CompanyName, e.JobTitle = strings.TrimSpace(e.CompanyName), strings.TrimSpace(e.JobTitle)
	if e.CompanyName == "" {
		return invalid("company_name", "company_name is required")
	}
	if e.JobTitle == "" {
		return invalid("job_title", "job_title is required")
	}
	for f, p := range map[string]**string{"start_date": &e.StartDate, "end_date": &e.EndDate, "location": &e.Location, "description": &e.Description, "impact": &e.Impact} {
		blankToNil(p)
		if (f == "start_date" || f == "end_date") && *p != nil && !partialDateRe.MatchString(**p) {
			return invalid(f, "%s must be YYYY, YYYY-MM or YYYY-MM-DD", f)
		}
	}
	if e.StartDate != nil && e.EndDate != nil && padDate(*e.EndDate) < padDate(*e.StartDate) {
		return invalid("end_date", "end_date must not precede start_date")
	}
	e.Skills = cleanList(e.Skills, true)
	return nil
}

// padDate extends YYYY and YYYY-MM so dates compare as strings.
func padDate(d string) string {
	switch len(d) {
	case 4:
		return d + "-01-01"
	case 7:
		return d + "-01"
	}
	return d
}

func (s *Server) createExperience(w http.ResponseWriter, r *http.Request) error {
	var in models.Experience
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := validateExperience(&in); err != nil {
		return err
	}
	out, err := s.st.CreateExperience(in)
	if err != nil {
		return needsProfile(err)
	}
	writeJSON(w, 201, out)
	return nil
}

func (s *Server) updateExperience(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in models.Experience
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := validateExperience(&in); err != nil {
		return err
	}
	out, err := s.st.UpdateExperience(id, in)
	if err != nil {
		return fromStore(err, "experience")
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) deleteExperience(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.st.DeleteExperience(id); err != nil {
		return fromStore(err, "experience")
	}
	return noContent(w)
}

func (s *Server) listEducation(w http.ResponseWriter, r *http.Request) error {
	out, err := s.st.ListEducation()
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func validateEducation(e *models.Education) error {
	e.Institution = strings.TrimSpace(e.Institution)
	if e.Institution == "" {
		return invalid("institution", "institution is required")
	}
	for _, p := range []**string{&e.DegreeLevel, &e.Discipline, &e.GraduationDate, &e.Honors, &e.Details} {
		blankToNil(p)
	}
	if e.GraduationDate != nil && !partialDateRe.MatchString(*e.GraduationDate) {
		return invalid("graduation_date", "graduation_date must be YYYY, YYYY-MM or YYYY-MM-DD")
	}
	if e.GPA != nil && (*e.GPA < 0 || *e.GPA > 5) {
		return invalid("gpa", "gpa must be between 0 and 5")
	}
	return nil
}

func (s *Server) createEducation(w http.ResponseWriter, r *http.Request) error {
	var in models.Education
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := validateEducation(&in); err != nil {
		return err
	}
	out, err := s.st.CreateEducation(in)
	if err != nil {
		return needsProfile(err)
	}
	writeJSON(w, 201, out)
	return nil
}

func (s *Server) updateEducation(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in models.Education
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := validateEducation(&in); err != nil {
		return err
	}
	out, err := s.st.UpdateEducation(id, in)
	if err != nil {
		return fromStore(err, "education")
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) deleteEducation(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.st.DeleteEducation(id); err != nil {
		return fromStore(err, "education")
	}
	return noContent(w)
}

// ---- saved resumes ----

func (s *Server) listResumes(w http.ResponseWriter, r *http.Request) error {
	out, err := s.st.ListResumes()
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) createResume(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		VersionLabel   string  `json:"version_label"`
		TargetPosition *string `json:"target_position"`
		Content        *string `json:"content"`
		FromMaster     bool    `json:"from_master"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	in.VersionLabel = strings.TrimSpace(in.VersionLabel)
	if in.VersionLabel == "" {
		return invalid("version_label", "version_label is required")
	}
	blankToNil(&in.TargetPosition)
	var content string
	switch {
	case in.FromMaster:
		m, err := s.st.GetMaster()
		if err != nil {
			return err
		}
		if m.Profile == nil {
			return conflict("no master resume yet")
		}
		content = render.Master(m)
	case in.Content != nil && strings.TrimSpace(*in.Content) != "":
		content = *in.Content
	default:
		return invalid("content", "content or from_master is required")
	}
	out, err := s.st.CreateResume(in.VersionLabel, in.TargetPosition, content)
	if err != nil {
		return needsProfile(err)
	}
	writeJSON(w, 201, out)
	return nil
}

func (s *Server) getResumeVersion(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	out, err := s.st.GetResume(id)
	if err != nil {
		return fromStore(err, "resume")
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) updateResume(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		VersionLabel   *string `json:"version_label"`
		TargetPosition *string `json:"target_position"`
		Content        *string `json:"content"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.VersionLabel != nil && strings.TrimSpace(*in.VersionLabel) == "" {
		return invalid("version_label", "version_label must not be empty")
	}
	if in.Content != nil && strings.TrimSpace(*in.Content) == "" {
		return invalid("content", "content must not be empty")
	}
	out, err := s.st.UpdateResume(id, in.VersionLabel, in.TargetPosition, in.Content)
	if err != nil {
		return fromStore(err, "resume")
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) deleteResume(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.st.DeleteResume(id); err != nil {
		return fromStore(err, "resume")
	}
	return noContent(w)
}
