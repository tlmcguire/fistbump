package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/render"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

const (
	maxShortText = 500   // names, titles, places
	maxLongText  = 20000 // summary, descriptions
	maxListItems = 200   // entries in a skills or links list
	maxPay       = 100000000
)

// readOnly holds the fields api.md says are ignored on write.
type readOnly struct {
	ID        json.RawMessage `json:"id"`
	ProfileID json.RawMessage `json:"profile_id"`
	CreatedAt json.RawMessage `json:"created_at"`
	UpdatedAt json.RawMessage `json:"updated_at"`
}

// storeError maps store errors to API errors. Anything else is a 500.
func storeError(err error, what string) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return notFound(what + " not found")
	case errors.Is(err, store.ErrConflict):
		return conflict("The change conflicts with existing data")
	}
	return err
}

// shortText trims a short optional text field; blank becomes nil.
func shortText(field string, v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*v)
	if len(t) > maxShortText {
		return nil, validationError(field, field+" is too long")
	}
	if t == "" {
		return nil, nil
	}
	return &t, nil
}

// longText keeps a long optional text field as written; blank becomes nil.
func longText(field string, v *string) (*string, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	if len(*v) > maxLongText {
		return nil, validationError(field, field+" is too long")
	}
	return v, nil
}

func requiredText(field string, v string) (string, error) {
	t := strings.TrimSpace(v)
	if t == "" {
		return "", validationError(field, field+" is required")
	}
	if len(t) > maxShortText {
		return "", validationError(field, field+" is too long")
	}
	return t, nil
}

func checkList(field string, list []string) error {
	if len(list) > maxListItems {
		return validationError(field, field+" has too many entries")
	}
	for _, item := range list {
		if len(item) > maxShortText {
			return validationError(field, field+" has an entry that is too long")
		}
	}
	return nil
}

// date accepts YYYY-MM-DD or YYYY-MM and returns YYYY-MM-DD; blank becomes nil.
func date(field string, v *string) (*string, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	t := strings.TrimSpace(*v)
	if len(t) == 7 {
		t += "-01"
	}
	if _, err := time.Parse("2006-01-02", t); err != nil || len(t) != 10 {
		return nil, validationError(field, field+" must be a date like 2024-06-15 or 2024-06")
	}
	return &t, nil
}

type profileBody struct {
	readOnly
	FullName        string   `json:"full_name"`
	Email           *string  `json:"email"`
	Phone           *string  `json:"phone"`
	HomeLocation    *string  `json:"home_location"`
	TargetPositions []string `json:"target_positions"`
	PreferredMode   *string  `json:"preferred_mode"`
	MinDesiredPay   *int64   `json:"min_desired_pay"`
	ClearanceCerts  []string `json:"clearance_certs"`
	Summary         *string  `json:"summary"`
	Skills          []string `json:"skills"`
	Links           []string `json:"links"`
}

func (b profileBody) validate() (models.Profile, error) {
	var p models.Profile
	var err error
	// Every profile field may be blank, the name included, so a profile can be cleared and filled in again.
	if p.FullName = strings.TrimSpace(b.FullName); len(p.FullName) > maxShortText {
		return p, validationError("full_name", "full_name is too long")
	}
	for _, f := range []struct {
		field string
		in    *string
		out   **string
	}{{"email", b.Email, &p.Email}, {"phone", b.Phone, &p.Phone}, {"home_location", b.HomeLocation, &p.HomeLocation}} {
		if *f.out, err = shortText(f.field, f.in); err != nil {
			return p, err
		}
	}
	if b.PreferredMode != nil {
		switch *b.PreferredMode {
		case "Remote", "Hybrid", "On-site", "Any":
			p.PreferredMode = b.PreferredMode
		default:
			return p, validationError("preferred_mode", "preferred_mode is not valid")
		}
	}
	if b.MinDesiredPay != nil {
		if *b.MinDesiredPay < 0 || *b.MinDesiredPay > maxPay {
			return p, validationError("min_desired_pay", "min_desired_pay must be between 0 and 100000000")
		}
		p.MinDesiredPay = b.MinDesiredPay
	}
	if p.Summary, err = longText("summary", b.Summary); err != nil {
		return p, err
	}
	for _, f := range []struct {
		field string
		in    []string
		out   *[]string
	}{{"target_positions", b.TargetPositions, &p.TargetPositions}, {"clearance_certs", b.ClearanceCerts, &p.ClearanceCerts},
		{"skills", b.Skills, &p.Skills}, {"links", b.Links, &p.Links}} {
		if err := checkList(f.field, f.in); err != nil {
			return p, err
		}
		*f.out = f.in
	}
	return p, nil
}

type experienceBody struct {
	readOnly
	CompanyName string   `json:"company_name"`
	JobTitle    string   `json:"job_title"`
	StartDate   *string  `json:"start_date"`
	EndDate     *string  `json:"end_date"`
	Location    *string  `json:"location"`
	Description *string  `json:"description"`
	Impact      *string  `json:"impact"`
	Skills      []string `json:"skills"`
	SortOrder   *int64   `json:"sort_order"`
}

func (b experienceBody) validate() (models.Experience, error) {
	var e models.Experience
	var err error
	if e.CompanyName, err = requiredText("company_name", b.CompanyName); err != nil {
		return e, err
	}
	if e.JobTitle, err = requiredText("job_title", b.JobTitle); err != nil {
		return e, err
	}
	if e.StartDate, err = date("start_date", b.StartDate); err != nil {
		return e, err
	}
	if e.EndDate, err = date("end_date", b.EndDate); err != nil {
		return e, err
	}
	// ISO dates compare correctly as text.
	if e.StartDate != nil && e.EndDate != nil && *e.EndDate < *e.StartDate {
		return e, validationError("end_date", "end_date cannot be before start_date")
	}
	if e.Location, err = shortText("location", b.Location); err != nil {
		return e, err
	}
	if e.Description, err = longText("description", b.Description); err != nil {
		return e, err
	}
	if e.Impact, err = longText("impact", b.Impact); err != nil {
		return e, err
	}
	if err := checkList("skills", b.Skills); err != nil {
		return e, err
	}
	e.Skills = b.Skills
	return e, nil
}

type educationBody struct {
	readOnly
	Institution    string   `json:"institution"`
	DegreeLevel    *string  `json:"degree_level"`
	Discipline     *string  `json:"discipline"`
	GraduationDate *string  `json:"graduation_date"`
	GPA            *float64 `json:"gpa"`
	Honors         *string  `json:"honors"`
	Details        *string  `json:"details"`
	SortOrder      *int64   `json:"sort_order"`
}

func (b educationBody) validate() (models.Education, error) {
	var e models.Education
	var err error
	if e.Institution, err = requiredText("institution", b.Institution); err != nil {
		return e, err
	}
	if e.DegreeLevel, err = shortText("degree_level", b.DegreeLevel); err != nil {
		return e, err
	}
	if e.Discipline, err = shortText("discipline", b.Discipline); err != nil {
		return e, err
	}
	if e.GraduationDate, err = date("graduation_date", b.GraduationDate); err != nil {
		return e, err
	}
	if b.GPA != nil {
		if *b.GPA < 0 || *b.GPA > 5 {
			return e, validationError("gpa", "gpa must be between 0 and 5")
		}
		e.GPA = b.GPA
	}
	if e.Honors, err = shortText("honors", b.Honors); err != nil {
		return e, err
	}
	if e.Details, err = longText("details", b.Details); err != nil {
		return e, err
	}
	return e, nil
}

// resumeGet returns the master resume.
func (s *Server) resumeGet(w http.ResponseWriter, r *http.Request) error {
	m, err := s.store.GetMaster(r.Context())
	if err != nil {
		return storeError(err, "Resume")
	}
	writeJSON(w, http.StatusOK, struct {
		Profile     *models.Profile     `json:"profile"`
		Experiences []models.Experience `json:"experiences"`
		Education   []models.Education  `json:"education"`
		Skills      []string            `json:"skills"`
	}{m.Profile, m.Experiences, m.Education, m.Skills()})
	return nil
}

func (s *Server) profilePut(w http.ResponseWriter, r *http.Request) error {
	var body profileBody
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	p, err := body.validate()
	if err != nil {
		return err
	}
	out, err := s.store.UpsertProfile(r.Context(), p)
	if err != nil {
		return storeError(err, "Profile")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) experienceList(w http.ResponseWriter, r *http.Request) error {
	list, err := s.store.ListExperiences(r.Context())
	if err != nil {
		return storeError(err, "Experience")
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) experienceCreate(w http.ResponseWriter, r *http.Request) error {
	var body experienceBody
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	e, err := body.validate()
	if err != nil {
		return err
	}
	if body.SortOrder != nil {
		e.SortOrder = *body.SortOrder
	}
	out, err := s.store.CreateExperience(r.Context(), e)
	if errors.Is(err, store.ErrConflict) {
		return conflict("create a profile first")
	}
	if err != nil {
		return storeError(err, "Experience")
	}
	writeJSON(w, http.StatusCreated, out)
	return nil
}

func (s *Server) experienceUpdate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var body experienceBody
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	e, err := body.validate()
	if err != nil {
		return err
	}
	cur, err := s.store.GetExperience(r.Context(), id)
	if err != nil {
		return storeError(err, "Experience")
	}
	e.SortOrder = cur.SortOrder // a full replace keeps the position unless sort_order is sent
	if body.SortOrder != nil {
		e.SortOrder = *body.SortOrder
	}
	out, err := s.store.UpdateExperience(r.Context(), id, e)
	if err != nil {
		return storeError(err, "Experience")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) experienceDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteExperience(r.Context(), id); err != nil {
		return storeError(err, "Experience")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) educationList(w http.ResponseWriter, r *http.Request) error {
	list, err := s.store.ListEducation(r.Context())
	if err != nil {
		return storeError(err, "Education")
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) educationCreate(w http.ResponseWriter, r *http.Request) error {
	var body educationBody
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	e, err := body.validate()
	if err != nil {
		return err
	}
	if body.SortOrder != nil {
		e.SortOrder = *body.SortOrder
	}
	out, err := s.store.CreateEducation(r.Context(), e)
	if errors.Is(err, store.ErrConflict) {
		return conflict("create a profile first")
	}
	if err != nil {
		return storeError(err, "Education")
	}
	writeJSON(w, http.StatusCreated, out)
	return nil
}

func (s *Server) educationUpdate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var body educationBody
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	e, err := body.validate()
	if err != nil {
		return err
	}
	cur, err := s.store.GetEducation(r.Context(), id)
	if err != nil {
		return storeError(err, "Education")
	}
	e.SortOrder = cur.SortOrder
	if body.SortOrder != nil {
		e.SortOrder = *body.SortOrder
	}
	out, err := s.store.UpdateEducation(r.Context(), id, e)
	if err != nil {
		return storeError(err, "Education")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) educationDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteEducation(r.Context(), id); err != nil {
		return storeError(err, "Education")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// resumePreview returns the master resume as Markdown.
func (s *Server) resumePreview(w http.ResponseWriter, r *http.Request) error {
	m, err := s.store.GetMaster(r.Context())
	if err != nil {
		return storeError(err, "Resume")
	}
	writeJSON(w, http.StatusOK, struct {
		Markdown string `json:"markdown"`
	}{render.Master(m)})
	return nil
}

func (s *Server) resumeList(w http.ResponseWriter, r *http.Request) error {
	list, err := s.store.ListResumes(r.Context())
	if err != nil {
		return storeError(err, "Resume")
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) resumeCreate(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		VersionLabel   string  `json:"version_label"`
		TargetPosition *string `json:"target_position"`
		Content        *string `json:"content"`
		FromMaster     bool    `json:"from_master"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	label, err := requiredText("version_label", body.VersionLabel)
	if err != nil {
		return err
	}
	target, err := shortText("target_position", body.TargetPosition)
	if err != nil {
		return err
	}
	hasContent := body.Content != nil && strings.TrimSpace(*body.Content) != ""
	switch {
	case hasContent && body.FromMaster:
		return validationError("content", "give content or from_master, not both")
	case !hasContent && !body.FromMaster:
		return validationError("content", "content or from_master is required")
	}
	rv := models.ResumeVersion{VersionLabel: label, TargetPosition: target}
	if hasContent {
		if len(*body.Content) > 1<<20 {
			return validationError("content", "content is too long")
		}
		rv.Content = *body.Content
	}
	var out models.ResumeVersion
	err = s.store.WithTx(r.Context(), func(tx *store.Store) error {
		if body.FromMaster {
			m, err := tx.GetMaster(r.Context())
			if err != nil {
				return err
			}
			if m.Profile == nil {
				return store.ErrConflict
			}
			rv.Content = render.Master(m)
		}
		var err error
		out, err = tx.CreateResume(r.Context(), rv)
		return err
	})
	if errors.Is(err, store.ErrConflict) {
		return conflict("create a profile first")
	}
	if err != nil {
		return storeError(err, "Resume")
	}
	writeJSON(w, http.StatusCreated, out)
	return nil
}

func (s *Server) resumeGetOne(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	out, err := s.store.GetResume(r.Context(), id)
	if err != nil {
		return storeError(err, "Resume")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) resumeUpdate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var body struct {
		readOnly
		VersionLabel   *string         `json:"version_label"`
		TargetPosition json.RawMessage `json:"target_position"`
		Content        *string         `json:"content"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	var patch store.ResumePatch
	if body.VersionLabel != nil {
		label, err := requiredText("version_label", *body.VersionLabel)
		if err != nil {
			return err
		}
		patch.VersionLabel = &label
	}
	if body.TargetPosition != nil {
		var v *string
		if err := json.Unmarshal(body.TargetPosition, &v); err != nil {
			return validationError("target_position", "target_position has the wrong type")
		}
		if patch.TargetPosition, err = shortText("target_position", v); err != nil {
			return err
		}
		patch.SetTarget = true
	}
	if body.Content != nil {
		if strings.TrimSpace(*body.Content) == "" {
			return validationError("content", "content cannot be empty")
		}
		if len(*body.Content) > 1<<20 {
			return validationError("content", "content is too long")
		}
		patch.Content = body.Content
	}
	out, err := s.store.UpdateResume(r.Context(), id, patch)
	if err != nil {
		return storeError(err, "Resume")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) resumeDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteResume(r.Context(), id); err != nil {
		return storeError(err, "Resume")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// defaultKeywords is what a job search uses when the request has no keywords: the profile's
// target positions, else the title of the latest experience (a current role, then the newest
// start date). It returns nil when the master resume has neither.
func (s *Server) defaultKeywords(ctx context.Context) []string {
	m, err := s.store.GetMaster(ctx)
	if err != nil {
		return nil
	}
	if m.Profile != nil && len(m.Profile.TargetPositions) > 0 {
		return append([]string(nil), m.Profile.TargetPositions...)
	}
	var latest *models.Experience
	rank := func(e *models.Experience) string {
		start := ""
		if e.StartDate != nil {
			start = *e.StartDate
		}
		if e.EndDate == nil {
			return "9" + start // a current role beats any ended one
		}
		return "0" + *e.EndDate
	}
	for i := range m.Experiences {
		if latest == nil || rank(&m.Experiences[i]) > rank(latest) {
			latest = &m.Experiences[i]
		}
	}
	if latest == nil {
		return nil
	}
	return []string{latest.JobTitle}
}
