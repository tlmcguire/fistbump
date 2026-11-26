package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/store"
)

var eventKinds = map[string]bool{"interview": true, "call": true, "follow_up": true, "other": true}

type eventBody struct {
	JobID    *int64 `json:"job_id"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at"`
	Location string `json:"location"`
	Notes    string `json:"notes"`
}

// readEvent checks an event the page sent: a known kind, a title, and an end after its start, both RFC 3339.
func readEvent(b eventBody) (store.EventInput, error) {
	kind := strings.ToLower(strings.TrimSpace(b.Kind))
	if kind == "" {
		kind = "interview"
	}
	if !eventKinds[kind] {
		return store.EventInput{}, validationError("kind", "kind must be interview, call, follow_up or other")
	}
	title := strings.TrimSpace(b.Title)
	if title == "" {
		return store.EventInput{}, validationError("title", "Give the plan a title")
	}
	if len(title) > 200 {
		return store.EventInput{}, validationError("title", "The title is too long")
	}
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(b.StartsAt))
	if err != nil {
		return store.EventInput{}, validationError("starts_at", "Choose when it starts")
	}
	end, err := time.Parse(time.RFC3339, strings.TrimSpace(b.EndsAt))
	if err != nil || !end.After(start) {
		return store.EventInput{}, validationError("ends_at", "It has to end after it starts")
	}
	if end.Sub(start) > 7*24*time.Hour {
		return store.EventInput{}, validationError("ends_at", "A plan can last a week at most")
	}
	if len(b.Notes) > 4000 || len(b.Location) > 300 {
		return store.EventInput{}, validationError("notes", "The notes are too long")
	}
	if b.JobID != nil && *b.JobID <= 0 {
		b.JobID = nil
	}
	return store.EventInput{JobID: b.JobID, Kind: kind, Title: title, StartsAt: start.Format(time.RFC3339), EndsAt: end.Format(time.RFC3339),
		Location: b.Location, Notes: b.Notes}, nil
}

func (s *Server) eventList(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	list, err := s.store.ListEvents(r.Context(), strings.TrimSpace(q.Get("from")), strings.TrimSpace(q.Get("to")))
	if err != nil {
		return storeError(err, "Event")
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) eventCreate(w http.ResponseWriter, r *http.Request) error {
	var b eventBody
	if err := decodeJSON(w, r, &b); err != nil {
		return err
	}
	in, err := readEvent(b)
	if err != nil {
		return err
	}
	e, err := s.store.CreateEvent(r.Context(), in)
	if err != nil {
		return storeError(err, "Event")
	}
	writeJSON(w, http.StatusCreated, e)
	return nil
}

func (s *Server) eventUpdate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var b eventBody
	if err := decodeJSON(w, r, &b); err != nil {
		return err
	}
	in, err := readEvent(b)
	if err != nil {
		return err
	}
	e, err := s.store.UpdateEvent(r.Context(), id, in)
	if err != nil {
		return storeError(err, "Event")
	}
	writeJSON(w, http.StatusOK, e)
	return nil
}

func (s *Server) eventDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteEvent(r.Context(), id); err != nil {
		return storeError(err, "Event")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
