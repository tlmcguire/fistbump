package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func TestSavedSearchesAPI(t *testing.T) {
	s := newTestServer(t)

	// Create
	rec := doJSON(t, s, http.MethodPost, "/v1/saved-searches", map[string]any{
		"name":      "Remote Go Roles",
		"query":     map[string]any{"keywords": []string{"golang"}, "work_mode": "Remote"},
		"seen_keys": []string{"stripe/101"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created models.SavedSearch
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.ID == 0 || created.Name != "Remote Go Roles" || len(created.SeenKeys) != 1 {
		t.Fatalf("unexpected created: %+v", created)
	}

	// List
	rec = doJSON(t, s, http.MethodGet, "/v1/saved-searches", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d", rec.Code)
	}
	var list []models.SavedSearch
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list len=%d err=%v", len(list), err)
	}

	// Mark seen
	rec = doJSON(t, s, http.MethodPost, "/v1/saved-searches/"+strconv.FormatInt(created.ID, 10)+"/mark-seen", map[string]any{
		"seen_keys": []string{"stripe/101", "stripe/102"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("mark-seen status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Delete
	rec = doJSON(t, s, http.MethodDelete, "/v1/saved-searches/"+strconv.FormatInt(created.ID, 10), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", rec.Code)
	}
}
