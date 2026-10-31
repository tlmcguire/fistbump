package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/parser"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

// maxRawText caps pasted posting text. Real postings are a few thousand words.
const maxRawText = 200000

// hiddenText reports invisible characters removed from a posting, so the app can say so.
type hiddenText struct {
	Characters int `json:"characters"`
}

func hiddenOf(n int) *hiddenText {
	if n == 0 {
		return nil
	}
	return &hiddenText{n}
}

// link trims an optional link; blank becomes nil. It must be an https URL, or http too when
// allowHTTP, so the app never opens another scheme.
func link(field string, v *string, allowHTTP bool) (*string, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	t := strings.TrimSpace(*v)
	u, err := url.Parse(t)
	ok := err == nil && u.Host != "" && len(t) <= maxLinkLen && (u.Scheme == "https" || allowHTTP && u.Scheme == "http")
	if !ok {
		if allowHTTP {
			return nil, validationError(field, field+" must be an http or https link")
		}
		return nil, validationError(field, field+" must be an https link")
	}
	return &t, nil
}

// checkJob validates and normalizes the writable fields of j in place.
func checkJob(j *models.Job) error {
	var err error
	if j.CompanyName, err = requiredText("company_name", j.CompanyName); err != nil {
		return err
	}
	if j.PositionTitle, err = requiredText("position_title", j.PositionTitle); err != nil {
		return err
	}
	if j.ListingURL, err = link("listing_url", j.ListingURL, true); err != nil {
		return err
	}
	if j.CompanyURL, err = link("company_url", j.CompanyURL, false); err != nil {
		return err
	}
	for _, f := range []struct {
		field string
		v     **string
	}{{"work_mode", &j.WorkMode}, {"employment_type", &j.EmploymentType}, {"location", &j.Location}} {
		if *f.v, err = shortText(f.field, *f.v); err != nil {
			return err
		}
	}
	if j.WorkMode != nil {
		switch *j.WorkMode {
		case "Remote", "Hybrid", "On-site":
		default:
			return validationError("work_mode", "work_mode must be Remote, Hybrid or On-site")
		}
	}
	for _, f := range []struct {
		field string
		v     *int64
	}{{"pay_min", j.PayMin}, {"pay_max", j.PayMax}} {
		if f.v != nil && (*f.v < 0 || *f.v > maxPay) {
			return validationError(f.field, f.field+" is out of range")
		}
	}
	if j.PayMin != nil && j.PayMax != nil && *j.PayMin > *j.PayMax {
		return validationError("pay_min", "pay_min cannot be above pay_max")
	}
	if j.Description, err = longText("description", j.Description); err != nil {
		return err
	}
	if j.RawText != nil && strings.TrimSpace(*j.RawText) == "" {
		j.RawText = nil
	}
	if j.RawText != nil && len(*j.RawText) > maxRawText {
		return validationError("raw_text", "raw_text is too long")
	}
	for _, f := range []struct {
		field string
		list  []string
	}{{"req_tech_skills", j.ReqTechSkills}, {"pref_tech_skills", j.PrefTechSkills}, {"req_soft_skills", j.ReqSoftSkills},
		{"pref_soft_skills", j.PrefSoftSkills}, {"requirements", j.Requirements}} {
		if err := checkList(f.field, f.list); err != nil {
			return err
		}
	}
	if j.CloseDate, err = date("close_date", j.CloseDate); err != nil {
		return err
	}
	return nil
}

// applyParse fills j's parsed fields from res. Fields the user gave (company, title, location,
// work mode) are kept; empty ones take the inferred values.
func applyParse(j *models.Job, res parser.Result) {
	if strings.TrimSpace(res.Description) != "" {
		j.Description = &res.Description
	} else {
		j.Description = nil
	}
	j.ReqTechSkills, j.PrefTechSkills = res.ReqTechSkills, res.PrefTechSkills
	j.ReqSoftSkills, j.PrefSoftSkills = res.ReqSoftSkills, res.PrefSoftSkills
	j.Requirements = res.Requirements
	in := res.Inferred
	if strings.TrimSpace(j.CompanyName) == "" {
		j.CompanyName = in.CompanyName
	}
	if strings.TrimSpace(j.PositionTitle) == "" {
		j.PositionTitle = in.PositionTitle
	}
	if (j.Location == nil || strings.TrimSpace(*j.Location) == "") && in.Location != "" {
		j.Location = &in.Location
	}
	if (j.WorkMode == nil || strings.TrimSpace(*j.WorkMode) == "") && in.WorkMode != "" {
		j.WorkMode = &in.WorkMode
	}
}

// jobParse handles POST /v1/jobs/parse: a preview of what saving the text would find. No writes.
func (s *Server) jobParse(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		RawText string `json:"raw_text"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	if strings.TrimSpace(body.RawText) == "" {
		return validationError("raw_text", "raw_text is required")
	}
	if len(body.RawText) > maxRawText {
		return validationError("raw_text", "raw_text is too long")
	}
	res := s.parse(body.RawText)
	writeJSON(w, http.StatusOK, struct {
		parser.Result
		HiddenText *hiddenText `json:"hidden_text"`
	}{res, hiddenOf(res.Hidden)})
	return nil
}

// jobCreate handles POST /v1/jobs. A pasted job is parsed and takes a missing company or title
// from the parser; a manual job is stored as given.
func (s *Server) jobCreate(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		CompanyName    string  `json:"company_name"`
		PositionTitle  string  `json:"position_title"`
		Source         string  `json:"source"`
		RawText        *string `json:"raw_text"`
		ListingURL     *string `json:"listing_url"`
		CompanyURL     *string `json:"company_url"`
		Location       *string `json:"location"`
		WorkMode       *string `json:"work_mode"`
		EmploymentType *string `json:"employment_type"`
		PayMin         *int64  `json:"pay_min"`
		PayMax         *int64  `json:"pay_max"`
		CloseDate      *string `json:"close_date"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	if body.Source == "" {
		body.Source = "pasted"
	}
	if body.Source != "pasted" && body.Source != "manual" {
		return validationError("source", "source must be pasted or manual")
	}
	j := models.Job{
		CompanyName: body.CompanyName, PositionTitle: body.PositionTitle, Source: body.Source, RawText: body.RawText,
		ListingURL: body.ListingURL, CompanyURL: body.CompanyURL, Location: body.Location, WorkMode: body.WorkMode,
		EmploymentType: body.EmploymentType, PayMin: body.PayMin, PayMax: body.PayMax, CloseDate: body.CloseDate,
	}
	hidden := 0
	if j.Source == "pasted" {
		if j.RawText == nil || strings.TrimSpace(*j.RawText) == "" {
			return validationError("raw_text", "raw_text is required for a pasted job")
		}
		if len(*j.RawText) > maxRawText {
			return validationError("raw_text", "raw_text is too long")
		}
		res := s.parse(*j.RawText)
		applyParse(&j, res)
		hidden = res.Hidden
	}
	if err := checkJob(&j); err != nil {
		return err
	}
	out, err := s.store.CreateJob(r.Context(), j)
	if err != nil {
		return storeError(err, "Job")
	}
	writeJSON(w, http.StatusCreated, struct {
		models.Job
		HiddenText *hiddenText `json:"hidden_text"`
	}{out, hiddenOf(hidden)})
	return nil
}

// jobList handles GET /v1/jobs.
func (s *Server) jobList(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	f := store.JobFilter{Q: q.Get("q"), Source: q.Get("source"), Archived: q.Get("archived"), Trashed: q.Get("trashed")}
	if len(f.Q) > maxShortText {
		return validationError("q", "q is too long")
	}
	switch f.Source {
	case "", "pasted", "manual", "greenhouse":
	default:
		return validationError("source", "source must be pasted, manual or greenhouse")
	}
	if f.Archived != "" && f.Archived != "include" && f.Archived != "only" {
		return validationError("archived", "archived must be include or only")
	}
	if f.Trashed != "" && f.Trashed != "only" {
		return validationError("trashed", "trashed must be only")
	}
	for _, p := range []struct {
		field    string
		dst      *int
		min, max int
	}{{"limit", &f.Limit, 1, 200}, {"offset", &f.Offset, 0, 1 << 30}} {
		v := q.Get(p.field)
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < p.min || n > p.max {
			return validationError(p.field, p.field+" is out of range")
		}
		*p.dst = n
	}
	list, err := s.store.ListJobs(r.Context(), f)
	if err != nil {
		return storeError(err, "Job")
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

// jobGet handles GET /v1/jobs/{id}.
func (s *Server) jobGet(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	out, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		return storeError(err, "Job")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// jobReadOnly lists the Job fields a PUT may send back but cannot change.
var jobReadOnly = map[string]bool{"id": true, "source": true, "external_id": true, "board": true,
	"created_at": true, "searched_at": true, "archived_at": true, "trashed_at": true, "hidden_text": true}

// jobSkillFields are the parsed fields; a PUT that sets any of them is not re-parsed.
var jobSkillFields = []string{"req_tech_skills", "pref_tech_skills", "req_soft_skills", "pref_soft_skills"}

// jobUpdate handles PUT /v1/jobs/{id}: any subset of the writable fields. A null clears an
// optional field. New raw_text re-runs the parser unless the skill lists are also given.
func (s *Server) jobUpdate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var patch map[string]json.RawMessage
	if err := decodeJSON(w, r, &patch); err != nil {
		return err
	}
	var out models.Job
	err = s.store.WithTx(r.Context(), func(tx *store.Store) error {
		j, err := tx.GetJob(r.Context(), id)
		if err != nil {
			return err
		}
		// A copy, because json.Unmarshal writes through the existing pointer.
		var oldRaw *string
		if j.RawText != nil {
			v := *j.RawText
			oldRaw = &v
		}
		fields := map[string]any{
			"company_name": &j.CompanyName, "position_title": &j.PositionTitle, "listing_url": &j.ListingURL,
			"company_url": &j.CompanyURL, "work_mode": &j.WorkMode, "employment_type": &j.EmploymentType,
			"location": &j.Location, "pay_min": &j.PayMin, "pay_max": &j.PayMax, "description": &j.Description,
			"raw_text": &j.RawText, "req_tech_skills": &j.ReqTechSkills, "pref_tech_skills": &j.PrefTechSkills,
			"req_soft_skills": &j.ReqSoftSkills, "pref_soft_skills": &j.PrefSoftSkills,
			"requirements": &j.Requirements, "close_date": &j.CloseDate,
		}
		for name, raw := range patch {
			if jobReadOnly[name] {
				continue
			}
			dst, ok := fields[name]
			if !ok {
				return validationError(name, "unknown field "+name)
			}
			if err := json.Unmarshal(raw, dst); err != nil {
				return validationError(name, name+" has the wrong type")
			}
		}
		_, rawGiven := patch["raw_text"]
		skillsGiven := false
		for _, name := range jobSkillFields {
			_, given := patch[name]
			skillsGiven = skillsGiven || given
		}
		if rawGiven && !skillsGiven && j.RawText != nil && (oldRaw == nil || *oldRaw != *j.RawText) {
			if len(*j.RawText) > maxRawText {
				return validationError("raw_text", "raw_text is too long")
			}
			applyParse(&j, s.parse(*j.RawText))
		}
		if err := checkJob(&j); err != nil {
			return err
		}
		out, err = tx.UpdateJob(r.Context(), id, j)
		return err
	})
	if err != nil {
		return storeError(err, "Job")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// jobDelete handles DELETE /v1/jobs/{id}. A job with an application answers 409 naming it.
func (s *Server) jobDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	err = s.store.DeleteJob(r.Context(), id)
	if errors.Is(err, store.ErrConflict) {
		e := conflict("job has an application")
		if appID, err := s.store.JobApplicationID(r.Context(), id); err == nil {
			e.Details = map[string]any{"application_id": appID}
		}
		return e
	}
	if err != nil {
		return storeError(err, "Job")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// jobAnalyze handles POST /v1/jobs/{id}/analyze: the job's skills against the master resume.
func (s *Server) jobAnalyze(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	j, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		return storeError(err, "Job")
	}
	m, err := s.store.GetMaster(r.Context())
	if err != nil {
		return storeError(err, "Resume")
	}
	if m.Profile == nil {
		return conflict("create a profile first")
	}
	res := s.analyze(j, m)
	res.JobID = j.ID
	writeJSON(w, http.StatusOK, res)
	return nil
}
