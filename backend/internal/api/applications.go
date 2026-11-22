package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

// applicationStages are the pipeline stages an application can be in.
var applicationStages = map[string]bool{"Saved": true, "Applied": true, "Interviewing": true, "Offered": true, "Rejected": true}

// appReadOnly lists the Application fields a PUT may send back but cannot change.
var appReadOnly = map[string]bool{"id": true, "job_id": true, "profile_id": true, "company_name": true,
	"position_title": true, "archived_at": true, "created_at": true, "updated_at": true}

const maxBulkIDs = 1000

// checkStage rejects a status that is not a pipeline stage.
func checkStage(status string) error {
	if !applicationStages[status] {
		return validationError("status", "status must be Saved, Applied, Interviewing, Offered or Rejected")
	}
	return nil
}

// checkBulkIDs rejects an empty or oversized id list.
func checkBulkIDs(ids []int64) error {
	if len(ids) == 0 || len(ids) > maxBulkIDs {
		return validationError("ids", "ids must have 1 to 1000 entries")
	}
	return nil
}

// checkFields validates and normalizes the writable fields of a in place.
func checkFields(a *models.Application) error {
	if err := checkStage(a.Status); err != nil {
		return err
	}
	var err error
	if a.DateApplied, err = date("date_applied", a.DateApplied); err != nil {
		return err
	}
	if a.NextStepDate, err = date("next_step_date", a.NextStepDate); err != nil {
		return err
	}
	if a.Notes, err = longText("notes", a.Notes); err != nil {
		return err
	}
	return nil
}

// setAppliedDate fills today's local date when an application is Applied and has no date yet.
func setAppliedDate(a *models.Application) {
	if a.Status == "Applied" && a.DateApplied == nil {
		d := time.Now().Format("2006-01-02")
		a.DateApplied = &d
	}
}

// checkLinks checks that the job and the tailored resume exist, and that the tailored resume
// was written for the same job. Unknown ids are 422, as api.md says.
func (s *Server) checkLinks(ctx context.Context, a models.Application) error {
	if _, err := s.store.GetJob(ctx, a.JobID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return validationError("job_id", "unknown job_id")
		}
		return err
	}
	if a.TailoredResumeID == nil {
		return nil
	}
	t, err := s.store.GetTailored(ctx, *a.TailoredResumeID)
	if errors.Is(err, store.ErrNotFound) {
		return validationError("tailored_resume_id", "unknown tailored_resume_id")
	}
	if err != nil {
		return err
	}
	if t.JobID != a.JobID {
		return validationError("tailored_resume_id", "tailored_resume_id is for a different job")
	}
	return nil
}

// writeApplicationByID reads an application back and writes it as the response.
func (s *Server) writeApplicationByID(w http.ResponseWriter, ctx context.Context, id int64) error {
	out, err := s.store.GetApplication(ctx, id)
	if err != nil {
		return storeError(err, "Application")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// appList handles GET /v1/applications.
func (s *Server) appList(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	f := store.ApplicationFilter{Status: q.Get("status"), Q: q.Get("q"), Archived: q.Get("archived")}
	if f.Status != "" {
		if err := checkStage(f.Status); err != nil {
			return err
		}
	}
	if len(f.Q) > maxShortText {
		return validationError("q", "q is too long")
	}
	if f.Archived != "" && f.Archived != "include" && f.Archived != "only" {
		return validationError("archived", "archived must be include or only")
	}
	list, err := s.store.ListApplications(r.Context(), f)
	if err != nil {
		return storeError(err, "Application")
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

// appCreate handles POST /v1/applications. One application per job: a second one is 409 and
// names the existing application.
func (s *Server) appCreate(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		JobID            int64   `json:"job_id"`
		BaseResumeID     *int64  `json:"base_resume_id"`
		TailoredResumeID *int64  `json:"tailored_resume_id"`
		Status           *string `json:"status"`
		DateApplied      *string `json:"date_applied"`
		NextStepDate     *string `json:"next_step_date"`
		Notes            *string `json:"notes"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	a := models.Application{
		JobID: body.JobID, BaseResumeID: body.BaseResumeID, TailoredResumeID: body.TailoredResumeID,
		Status: "Saved", DateApplied: body.DateApplied, NextStepDate: body.NextStepDate, Notes: body.Notes,
	}
	if body.Status != nil {
		a.Status = *body.Status
	}
	if err := checkFields(&a); err != nil {
		return err
	}
	setAppliedDate(&a)
	if err := s.checkLinks(r.Context(), a); err != nil {
		return err
	}

	out, err := s.store.CreateApplication(r.Context(), a)
	if errors.Is(err, store.ErrConflict) {
		// The job may already be tracked. If it is not, the conflict has another cause (such as
		// no profile) and falls through to the generic conflict below.
		if id, idErr := s.store.JobApplicationID(r.Context(), a.JobID); idErr == nil {
			e := conflict("job already has an application")
			e.Details = map[string]any{"application_id": id}
			return e
		}
	}
	if err != nil {
		return storeError(err, "Application")
	}
	writeJSON(w, http.StatusCreated, out)
	return nil
}

// appGet handles GET /v1/applications/{id}.
func (s *Server) appGet(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	return s.writeApplicationByID(w, r.Context(), id)
}

// appUpdate handles PUT /v1/applications/{id}: any subset of the writable fields. A null clears
// an optional field. Setting Applied with no date fills today's.
func (s *Server) appUpdate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var patch map[string]json.RawMessage
	if err := decodeJSON(w, r, &patch); err != nil {
		return err
	}
	var out models.Application
	err = s.store.WithTx(r.Context(), func(tx *store.Store) error {
		a, err := tx.GetApplication(r.Context(), id)
		if err != nil {
			return err
		}
		fields := map[string]any{
			"status": &a.Status, "date_applied": &a.DateApplied, "next_step_date": &a.NextStepDate,
			"notes": &a.Notes, "base_resume_id": &a.BaseResumeID, "tailored_resume_id": &a.TailoredResumeID,
			"exported_pdf_path": &a.ExportedPDFPath,
		}
		for name, raw := range patch {
			if appReadOnly[name] {
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
		if err := checkFields(&a); err != nil {
			return err
		}
		setAppliedDate(&a)
		if err := s.checkLinks(r.Context(), a); err != nil {
			return err
		}
		out, err = tx.UpdateApplication(r.Context(), id, a)
		return err
	})
	if err != nil {
		return storeError(err, "Application")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// appDelete handles DELETE /v1/applications/{id}.
func (s *Server) appDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteApplication(r.Context(), id); err != nil {
		return storeError(err, "Application")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// appBulkStatus handles POST /v1/applications/bulk-status: all or nothing.
func (s *Server) appBulkStatus(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		IDs    []int64 `json:"ids"`
		Status string  `json:"status"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	if err := checkBulkIDs(body.IDs); err != nil {
		return err
	}
	if err := checkStage(body.Status); err != nil {
		return err
	}
	out, err := s.store.BulkStatus(r.Context(), body.IDs, body.Status)
	if err != nil {
		return storeError(err, "Application")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// appArchive handles POST /v1/applications/{id}/archive. The status does not change.
func (s *Server) appArchive(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.store.ArchiveApplication(r.Context(), id); err != nil {
		return storeError(err, "Application")
	}
	return s.writeApplicationByID(w, r.Context(), id)
}

// appUnarchive handles POST /v1/applications/{id}/unarchive.
func (s *Server) appUnarchive(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.store.UnarchiveApplication(r.Context(), id); err != nil {
		return storeError(err, "Application")
	}
	return s.writeApplicationByID(w, r.Context(), id)
}

// appBulkArchive handles POST /v1/applications/bulk-archive: archived true hides, false restores.
func (s *Server) appBulkArchive(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		IDs      []int64 `json:"ids"`
		Archived bool    `json:"archived"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	if err := checkBulkIDs(body.IDs); err != nil {
		return err
	}
	out, err := s.store.BulkArchive(r.Context(), body.IDs, body.Archived)
	if err != nil {
		return storeError(err, "Application")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
