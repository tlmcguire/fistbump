package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/diff"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/render"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

// tailoredCreated is the create response: the tailored resume plus any changes that could not be placed.
type tailoredCreated struct {
	models.TailoredResume
	Warnings []string `json:"warnings"`
}

// tailoredCreate handles POST /v1/tailored-resumes. It applies the user's decisions to the base
// resume, so every suggestion must be decided first.
func (s *Server) tailoredCreate(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		RevisionID int64 `json:"revision_id"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	if req.RevisionID < 1 {
		return validationError("revision_id", "revision_id is required")
	}

	ctx := r.Context()
	rev, err := s.store.GetRevision(ctx, req.RevisionID)
	if err != nil {
		return storeError(err, "Revision")
	}
	if rev.Status != "done" {
		return conflict("The revision is not finished")
	}
	suggestions, err := s.store.ListSuggestions(ctx, rev.ID)
	if err != nil {
		return err
	}
	pending := 0
	for _, sg := range suggestions {
		if sg.State == "pending" {
			pending++
		}
	}
	if pending > 0 {
		e := conflict(fmt.Sprintf("%d suggestions still need a decision", pending))
		e.Details = map[string]any{"pending": pending}
		return e
	}

	base, err := s.baseContent(ctx, rev)
	if err != nil {
		return err
	}
	// A suggestion whose final text is its original changes nothing, so it is not applied.
	var changes []render.Change
	for _, sg := range suggestions {
		if final := sg.Final(); final != sg.OriginalText {
			changes = append(changes, render.Change{Original: sg.OriginalText, Final: final})
		}
	}
	content, notFound := render.Apply(base, changes)
	warnings := []string{}
	for _, c := range notFound {
		warnings = append(warnings, warning(c))
	}

	created, err := s.store.CreateTailored(ctx, models.TailoredResume{
		JobID:        rev.JobID,
		RevisionID:   &rev.ID,
		BaseResumeID: rev.BaseResumeID,
		Content:      content,
		BaseContent:  base,
	})
	if err != nil {
		return storeError(err, "Tailored resume")
	}
	writeJSON(w, http.StatusCreated, tailoredCreated{TailoredResume: created, Warnings: warnings})
	return nil
}

// baseContent is the text the revision started from: the saved resume it named, or the master
// resume as it is now.
func (s *Server) baseContent(ctx context.Context, rev models.Revision) (string, error) {
	if rev.BaseResumeID != nil {
		saved, err := s.store.GetResume(ctx, *rev.BaseResumeID)
		if err == nil {
			return saved.Content, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return "", err
		}
	}
	master, err := s.store.GetMaster(ctx)
	if err != nil {
		return "", err
	}
	return render.Master(master), nil
}

// warning describes a change that could not be placed in the base resume.
func warning(c render.Change) string {
	if c.Original == "" {
		return "The summary was not added because the base resume already has one"
	}
	text := []rune(c.Original)
	if len(text) > 60 {
		text = append(text[:60], '…')
	}
	return fmt.Sprintf("A change was not applied because %q is not in the base resume", string(text))
}

// tailoredList handles GET /v1/tailored-resumes?job_id=, newest first and without the text.
func (s *Server) tailoredList(w http.ResponseWriter, r *http.Request) error {
	jobID, err := jobIDQuery(r)
	if err != nil {
		return err
	}
	list, err := s.store.ListTailored(r.Context(), jobID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) tailoredGet(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	t, err := s.store.GetTailored(r.Context(), id)
	if err != nil {
		return storeError(err, "Tailored resume")
	}
	writeJSON(w, http.StatusOK, t)
	return nil
}

// tailoredDiff handles GET /v1/tailored-resumes/{id}/diff, comparing the base with the result.
func (s *Server) tailoredDiff(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	t, err := s.store.GetTailored(r.Context(), id)
	if err != nil {
		return storeError(err, "Tailored resume")
	}
	master, err := s.store.GetMaster(r.Context())
	if err != nil {
		return err
	}

	// Name the experience behind each experience heading, using the heading render.Master writes.
	experiences := map[string]int64{}
	for _, e := range master.Experiences {
		experiences["### "+render.Escape(e.JobTitle+", "+e.CompanyName)] = e.ID
	}
	hunks := diff.Hunks(t.BaseContent, t.Content)
	for i := range hunks {
		if expID, ok := experiences[hunks[i].Heading]; ok {
			hunks[i].TargetID = &expID
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"hunks": hunks})
	return nil
}

// tailoredUpdate handles PUT /v1/tailored-resumes/{id}: the user's own edit of the resume text.
func (s *Server) tailoredUpdate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	if strings.TrimSpace(body.Content) == "" {
		return validationError("content", "content cannot be empty")
	}
	if len(body.Content) > 1<<20 {
		return validationError("content", "content is too long")
	}
	t, err := s.store.UpdateTailoredContent(r.Context(), id, body.Content)
	if err != nil {
		return storeError(err, "Tailored resume")
	}
	writeJSON(w, http.StatusOK, t)
	return nil
}

func (s *Server) tailoredDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteTailored(r.Context(), id); err != nil {
		return storeError(err, "Tailored resume")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
