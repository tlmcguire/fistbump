package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestApplicationsCSVICSAndStats(t *testing.T) {
	s := newTestServer(t)

	// Create a profile, a job, and an application with a next_step_date.
	rec := doJSON(t, s, http.MethodPut, "/v1/resume/profile", map[string]any{
		"full_name": "Ada Lovelace",
		"skills":    []string{"go"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("put profile: %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, s, http.MethodPost, "/v1/jobs", map[string]any{
		"company_name":   "Acme",
		"position_title": "Backend Engineer",
		"source":         "pasted",
		"raw_text":       "Required: Go, PostgreSQL, Docker",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create job: %d %s", rec.Code, rec.Body.String())
	}
	var job struct{ ID int64 }
	_ = json.Unmarshal(rec.Body.Bytes(), &job)

	rec = doJSON(t, s, http.MethodPost, "/v1/applications", map[string]any{
		"job_id":         job.ID,
		"status":         "Interviewing",
		"next_step_date": "2026-12-10",
		"notes":          "System design panel",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create app: %d %s", rec.Code, rec.Body.String())
	}
	var app struct{ ID int64 }
	_ = json.Unmarshal(rec.Body.Bytes(), &app)

	// CSV export
	rec = doJSON(t, s, http.MethodGet, "/v1/applications/export?format=csv", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("csv status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Acme") || !strings.Contains(rec.Body.String(), "Interviewing") {
		t.Fatalf("unexpected csv: %s", rec.Body.String())
	}

	// ICS export
	rec = doJSON(t, s, http.MethodGet, "/v1/applications/"+strconv.FormatInt(app.ID, 10)+"/ics", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ics status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "BEGIN:VCALENDAR") || !strings.Contains(rec.Body.String(), "DTSTART;VALUE=DATE:20261210") {
		t.Fatalf("unexpected ics: %s", rec.Body.String())
	}

	// Funnel & skill-gap stats
	rec = doJSON(t, s, http.MethodGet, "/v1/applications/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats status=%d", rec.Code)
	}
	var stats struct {
		Total   int            `json:"total"`
		ByStage map[string]int `json:"by_stage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.Total != 1 || stats.ByStage["Interviewing"] != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}
