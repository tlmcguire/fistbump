package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) savedSearchList(w http.ResponseWriter, r *http.Request) error {
	list, err := s.store.ListSavedSearches(r.Context())
	if err != nil {
		return storeError(err, "Saved search")
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) savedSearchCreate(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name      string          `json:"name"`
		Query     json.RawMessage `json:"query"`
		QueryJSON string          `json:"query_json"`
		SeenKeys  []string        `json:"seen_keys"`
	}
	if err := decodeJSONLimit(w, r, &body, maxFetchBody); err != nil {
		return err
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > 120 {
		return validationError("name", "Enter a name for this saved search (1–120 characters)")
	}
	qJSON := strings.TrimSpace(body.QueryJSON)
	if qJSON == "" && len(body.Query) > 0 {
		qJSON = string(body.Query)
	}
	if qJSON == "" || !json.Valid([]byte(qJSON)) {
		return validationError("query", "Saved search query must be valid JSON")
	}
	created, err := s.store.CreateSavedSearch(r.Context(), name, qJSON, body.SeenKeys)
	if err != nil {
		return storeError(err, "Saved search")
	}
	writeJSON(w, http.StatusCreated, created)
	return nil
}

func (s *Server) savedSearchMarkSeen(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var body struct {
		SeenKeys []string `json:"seen_keys"`
	}
	if err := decodeJSONLimit(w, r, &body, maxFetchBody); err != nil {
		return err
	}
	updated, err := s.store.UpdateSavedSearchRun(r.Context(), id, body.SeenKeys)
	if err != nil {
		return storeError(err, "Saved search")
	}
	writeJSON(w, http.StatusOK, updated)
	return nil
}

func (s *Server) savedSearchDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteSavedSearch(r.Context(), id); err != nil {
		return storeError(err, "Saved search")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
