package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/analyze"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/parser"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

func (s *Server) parseJob(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		RawText string `json:"raw_text"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.RawText) == "" {
		return invalid("raw_text", "raw_text is required")
	}
	writeJSON(w, 200, parser.Parse(in.RawText))
	return nil
}

// applyParse fills description, skill lists and requirements from raw text.
func applyParse(j *models.Job, res parser.Result) {
	d := res.Description
	j.Description = &d
	j.ReqTech, j.PrefTech, j.ReqSoft, j.PrefSoft, j.Requirements = res.ReqTech, res.PrefTech, res.ReqSoft, res.PrefSoft, res.Requirements
}

func validateJob(j *models.Job) error {
	for _, p := range []**string{&j.ListingURL, &j.WorkMode, &j.EmploymentType, &j.Location} {
		blankToNil(p)
	}
	if j.WorkMode != nil {
		switch *j.WorkMode {
		case "Remote", "Hybrid", "On-site":
		default:
			return invalid("work_mode", "work_mode must be Remote, Hybrid or On-site")
		}
	}
	if err := validDate("close_date", j.CloseDate); err != nil {
		return err
	}
	if j.PayMin != nil && j.PayMax != nil && *j.PayMax < *j.PayMin {
		return invalid("pay_max", "pay_max must not be below pay_min")
	}
	if j.ListingURL != nil && !strings.HasPrefix(*j.ListingURL, "http://") && !strings.HasPrefix(*j.ListingURL, "https://") {
		return invalid("listing_url", "listing_url must be an http or https URL")
	}
	return nil
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) error {
	var in models.Job
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Source != "pasted" && in.Source != "manual" {
		return invalid("source", "source must be pasted or manual")
	}
	if in.Source == "pasted" && (in.RawText == nil || strings.TrimSpace(*in.RawText) == "") {
		return invalid("raw_text", "raw_text is required for pasted jobs")
	}
	in.ExternalID = nil
	if in.RawText != nil && strings.TrimSpace(*in.RawText) != "" {
		res := parser.Parse(*in.RawText)
		if in.Description == nil {
			applyParse(&in, res)
		}
		in.CompanyName = firstNonEmpty(strings.TrimSpace(in.CompanyName), res.Inferred.CompanyName)
		in.PositionTitle = firstNonEmpty(strings.TrimSpace(in.PositionTitle), res.Inferred.PositionTitle)
		if in.Location == nil && res.Inferred.Location != "" {
			l := res.Inferred.Location
			in.Location = &l
		}
		if in.WorkMode == nil && res.Inferred.WorkMode != "" {
			m := res.Inferred.WorkMode
			in.WorkMode = &m
		}
	}
	in.CompanyName, in.PositionTitle = strings.TrimSpace(in.CompanyName), strings.TrimSpace(in.PositionTitle)
	if in.CompanyName == "" {
		return invalid("company_name", "company_name is required and could not be inferred")
	}
	if in.PositionTitle == "" {
		return invalid("position_title", "position_title is required and could not be inferred")
	}
	if err := validateJob(&in); err != nil {
		return err
	}
	out, err := s.st.CreateJob(in)
	if err != nil {
		return err
	}
	writeJSON(w, 201, out)
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	archived := q.Get("archived")
	if archived != "" && archived != "include" && archived != "only" {
		return invalid("archived", "archived must be include or only")
	}
	out, err := s.st.ListJobs(q.Get("q"), q.Get("source"), archived, queryInt(r, "limit"), queryInt(r, "offset"))
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	j, err := s.st.GetJob(id)
	if err != nil {
		return fromStore(err, "job")
	}
	writeJSON(w, 200, j)
	return nil
}

var jobWritable = map[string]bool{
	"company_name": true, "position_title": true, "listing_url": true, "work_mode": true, "employment_type": true, "location": true,
	"pay_min": true, "pay_max": true, "description": true, "raw_text": true, "req_tech_skills": true, "pref_tech_skills": true,
	"req_soft_skills": true, "pref_soft_skills": true, "requirements": true, "close_date": true,
	"id": true, "created_at": true, "searched_at": true, "source": true, "external_id": true, "archived_at": true,
}

func (s *Server) updateJob(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	cur, err := s.st.GetJob(id)
	if err != nil {
		return fromStore(err, "job")
	}
	data, err := readBody(r)
	if err != nil {
		return err
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(data, &keys) != nil {
		return badRequest("malformed JSON")
	}
	for k := range keys {
		if !jobWritable[k] {
			return invalid(k, "unknown field %q", k)
		}
	}
	oldRaw := ""
	if cur.RawText != nil {
		oldRaw = *cur.RawText
	}
	// Decoding onto the current row overwrites only the supplied fields.
	patch := *cur
	if err := decodeBytes(data, &patch); err != nil {
		return err
	}
	patch.ID, patch.Source, patch.ExternalID, patch.CreatedAt, patch.SearchedAt, patch.ArchivedAt = cur.ID, cur.Source, cur.ExternalID, cur.CreatedAt, cur.SearchedAt, cur.ArchivedAt
	newRaw := ""
	if patch.RawText != nil {
		newRaw = *patch.RawText
	}
	suppliedLists := false
	for _, k := range []string{"req_tech_skills", "pref_tech_skills", "req_soft_skills", "pref_soft_skills", "requirements"} {
		if _, ok := keys[k]; ok {
			suppliedLists = true
		}
	}
	if newRaw != oldRaw && strings.TrimSpace(newRaw) != "" && !suppliedLists {
		applyParse(&patch, parser.Parse(newRaw))
	}
	patch.CompanyName, patch.PositionTitle = strings.TrimSpace(patch.CompanyName), strings.TrimSpace(patch.PositionTitle)
	if patch.CompanyName == "" || patch.PositionTitle == "" {
		return invalid("company_name", "company_name and position_title must not be empty")
	}
	if err := validateJob(&patch); err != nil {
		return err
	}
	out, err := s.st.UpdateJob(id, patch)
	if err != nil {
		return fromStore(err, "job")
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.st.DeleteJob(id); err != nil {
		if err == store.ErrConflict {
			return conflict("job has applications, remove them first")
		}
		return fromStore(err, "job")
	}
	return noContent(w)
}

func (s *Server) analyzeJob(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	j, err := s.st.GetJob(id)
	if err != nil {
		return fromStore(err, "job")
	}
	m, err := s.st.GetMaster()
	if err != nil {
		return err
	}
	start := time.Now()
	res := analyze.Run(*j, m)
	res.DurationMS = time.Since(start).Milliseconds()
	writeJSON(w, 200, res)
	return nil
}

// archiveJob archives or restores a job. With "archive_application": true, an application for the job
// is archived too, so one action clears it from both lists.
func (s *Server) archiveJob(archived bool) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := pathID(r, "id")
		if err != nil {
			return err
		}
		var in struct {
			ArchiveApplication bool `json:"archive_application"`
		}
		if err := decodeOptional(r, &in); err != nil {
			return err
		}
		j, err := s.st.SetJobArchived(id, archived)
		if err != nil {
			return fromStore(err, "job")
		}
		if archived && in.ArchiveApplication {
			apps, err := s.st.ListApplications("", "")
			if err != nil {
				return err
			}
			for _, a := range apps {
				if a.JobID == id && a.Status != "Archived" {
					a.Status = "Archived"
					if _, err := s.st.UpdateApplication(a.ID, a); err != nil {
						return err
					}
				}
			}
		}
		writeJSON(w, 200, j)
		return nil
	}
}
