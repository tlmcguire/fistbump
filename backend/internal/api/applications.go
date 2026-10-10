package api

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

var appStatuses = []string{"Saved", "Applied", "Interviewing", "Offered", "Rejected", "Archived"}

func (s *Server) listApplications(w http.ResponseWriter, r *http.Request) error {
	status := r.URL.Query().Get("status")
	if status != "" && !slices.Contains(appStatuses, status) {
		return invalid("status", "status is not valid")
	}
	out, err := s.st.ListApplications(status, r.URL.Query().Get("q"))
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) checkAppRefs(a *models.Application) error {
	if a.Status != "" && !slices.Contains(appStatuses, a.Status) {
		return invalid("status", "status is not valid")
	}
	blankToNil(&a.Notes)
	if err := validDate("date_applied", a.DateApplied); err != nil {
		return err
	}
	if err := validDate("next_step_date", a.NextStepDate); err != nil {
		return err
	}
	if a.BaseResumeID != nil {
		if _, err := s.st.GetResume(*a.BaseResumeID); err != nil {
			return invalid("base_resume_id", "unknown base_resume_id")
		}
	}
	if a.TailoredResumeID != nil {
		if _, err := s.st.GetTailored(*a.TailoredResumeID); err != nil {
			return invalid("tailored_resume_id", "unknown tailored_resume_id")
		}
	}
	return nil
}

func (s *Server) createApplication(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		JobID            int64   `json:"job_id"`
		BaseResumeID     *int64  `json:"base_resume_id"`
		TailoredResumeID *int64  `json:"tailored_resume_id"`
		Status           string  `json:"status"`
		DateApplied      *string `json:"date_applied"`
		NextStepDate     *string `json:"next_step_date"`
		Notes            *string `json:"notes"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if _, err := s.st.GetJob(in.JobID); err != nil {
		return invalid("job_id", "unknown job_id")
	}
	// One row per job keeps the tracker a clean sheet. Callers update the existing row instead.
	if existing, err := s.st.ListApplications("", ""); err == nil {
		for _, e := range existing {
			if e.JobID == in.JobID {
				return conflict("this job is already in the tracker").with("application_id", e.ID)
			}
		}
	}
	a := models.Application{JobID: in.JobID, BaseResumeID: in.BaseResumeID, TailoredResumeID: in.TailoredResumeID,
		Status: in.Status, DateApplied: in.DateApplied, NextStepDate: in.NextStepDate, Notes: in.Notes}
	if err := s.checkAppRefs(&a); err != nil {
		return err
	}
	out, err := s.st.CreateApplication(a)
	if err != nil {
		return needsProfile(err)
	}
	writeJSON(w, 201, out)
	return nil
}

func (s *Server) getApplication(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	a, err := s.st.GetApplication(id)
	if err != nil {
		return fromStore(err, "application")
	}
	writeJSON(w, 200, a)
	return nil
}

var appWritable = []string{"status", "date_applied", "next_step_date", "notes", "base_resume_id", "tailored_resume_id", "exported_pdf_path"}

func (s *Server) updateApplication(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	cur, err := s.st.GetApplication(id)
	if err != nil {
		return fromStore(err, "application")
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
		if !slices.Contains(appWritable, k) {
			return invalid(k, "field %q cannot be updated", k)
		}
	}
	patch := *cur
	if err := decodeBytes(data, &patch); err != nil {
		return err
	}
	if _, ok := keys["status"]; ok && patch.Status == "" {
		return invalid("status", "status is not valid")
	}
	if err := s.checkAppRefs(&patch); err != nil {
		return err
	}
	out, err := s.st.UpdateApplication(id, patch)
	if err != nil {
		return fromStore(err, "application")
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) deleteApplication(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.st.DeleteApplication(id); err != nil {
		return fromStore(err, "application")
	}
	return noContent(w)
}

// bulkApplicationStatus sets one status on many applications, for archiving from the tracker.
func (s *Server) bulkApplicationStatus(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		IDs    []int64 `json:"ids"`
		Status string  `json:"status"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if !slices.Contains(appStatuses, in.Status) {
		return invalid("status", "status is not valid")
	}
	if len(in.IDs) == 0 || len(in.IDs) > 1000 {
		return invalid("ids", "choose between 1 and 1000 applications")
	}
	out := []models.Application{}
	for _, id := range in.IDs {
		a, err := s.st.GetApplication(id)
		if err != nil {
			return invalid("ids", "unknown application %d", id)
		}
		a.Status = in.Status
		u, err := s.st.UpdateApplication(id, *a)
		if err != nil {
			return err
		}
		out = append(out, *u)
	}
	writeJSON(w, 200, out)
	return nil
}
