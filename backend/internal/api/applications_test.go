package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

// trackableJob saves a pasted job and returns its id.
func trackableJob(t *testing.T, s *Server) int64 {
	t.Helper()
	out := expect(t, call(t, s, "POST", "/v1/jobs", `{"raw_text":"Analyst at Acme\nSQL"}`), 201)
	return int64(out["id"].(float64))
}

// trackJob creates an application for a job and returns its id.
func trackJob(t *testing.T, s *Server, jobID int64) int64 {
	t.Helper()
	out := expect(t, call(t, s, "POST", "/v1/applications", fmt.Sprintf(`{"job_id":%d}`, jobID)), 201)
	return int64(out["id"].(float64))
}

// listApplications returns the applications a GET /v1/applications query gives.
func listApplications(t *testing.T, s *Server, query string) []map[string]any {
	t.Helper()
	rec := call(t, s, "GET", "/v1/applications"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	return list
}

func TestApplicationCreate(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	job := trackableJob(t, s)
	out := expect(t, call(t, s, "POST", "/v1/applications", fmt.Sprintf(`{"job_id":%d}`, job)), 201)
	if out["status"] != "Saved" || out["date_applied"] != nil {
		t.Errorf("created = %v, want status Saved and no date", out)
	}
	expect(t, call(t, s, "GET", fmt.Sprintf("/v1/applications/%d", int64(out["id"].(float64))), ""), 200)
}

func TestApplicationOnePerJob(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	job := trackableJob(t, s)
	first := trackJob(t, s, job)
	out := expect(t, call(t, s, "POST", "/v1/applications", fmt.Sprintf(`{"job_id":%d}`, job)), 409)
	details := out["error"].(map[string]any)["details"].(map[string]any)
	if details["application_id"] != float64(first) {
		t.Errorf("details = %v, want application_id %d", details, first)
	}
}

func TestApplicationValidation(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	job := trackableJob(t, s)
	expectField(t, call(t, s, "POST", "/v1/applications", `{"job_id":999999}`), "job_id")
	expectField(t, call(t, s, "POST", "/v1/applications", fmt.Sprintf(`{"job_id":%d,"status":"Hired"}`, job)), "status")
	expectField(t, call(t, s, "POST", "/v1/applications", fmt.Sprintf(`{"job_id":%d,"date_applied":"soon"}`, job)), "date_applied")
	expectField(t, call(t, s, "PUT", fmt.Sprintf("/v1/applications/%d", trackJob(t, s, job)), `{"status":"Hired"}`), "status")
}

func TestApplicationTailoredMustBelongToJob(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	jobA := trackableJob(t, s)
	jobB := trackableJob(t, s)
	tailored, err := s.store.CreateTailored(context.Background(), models.TailoredResume{JobID: jobB, Content: "resume"})
	if err != nil {
		t.Fatal(err)
	}
	expectField(t, call(t, s, "POST", "/v1/applications",
		fmt.Sprintf(`{"job_id":%d,"tailored_resume_id":%d}`, jobA, tailored.ID)), "tailored_resume_id")
}

func TestApplicationAppliedSetsDate(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	id := trackJob(t, s, trackableJob(t, s))
	out := expect(t, call(t, s, "PUT", fmt.Sprintf("/v1/applications/%d", id), `{"status":"Applied"}`), 200)
	if out["date_applied"] != time.Now().Format("2006-01-02") {
		t.Errorf("date_applied = %v, want today", out["date_applied"])
	}
}

func TestApplicationArchiveKeepsStage(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	id := trackJob(t, s, trackableJob(t, s))
	expect(t, call(t, s, "PUT", fmt.Sprintf("/v1/applications/%d", id), `{"status":"Interviewing"}`), 200)
	out := expect(t, call(t, s, "POST", fmt.Sprintf("/v1/applications/%d/archive", id), ""), 200)
	if out["status"] != "Interviewing" || out["archived_at"] == nil {
		t.Errorf("archived = %v, want Interviewing with archived_at set", out)
	}
	if n := len(listApplications(t, s, "")); n != 0 {
		t.Errorf("default list has %d, want 0 archived hidden", n)
	}
	if n := len(listApplications(t, s, "?archived=only")); n != 1 {
		t.Errorf("archived=only has %d, want 1", n)
	}
	expect(t, call(t, s, "POST", fmt.Sprintf("/v1/applications/%d/unarchive", id), ""), 200)
	if n := len(listApplications(t, s, "")); n != 1 {
		t.Errorf("after unarchive list has %d, want 1", n)
	}
}

func TestApplicationBulk(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	id1 := trackJob(t, s, trackableJob(t, s))
	id2 := trackJob(t, s, trackableJob(t, s))
	expectField(t, call(t, s, "POST", "/v1/applications/bulk-status", `{"ids":[],"status":"Rejected"}`), "ids")

	// An unknown id fails the whole request, so the known one must not change either.
	expect(t, call(t, s, "POST", "/v1/applications/bulk-status",
		fmt.Sprintf(`{"ids":[%d,999999],"status":"Offered"}`, id1)), 404)
	if out := expect(t, call(t, s, "GET", fmt.Sprintf("/v1/applications/%d", id1), ""), 200); out["status"] != "Saved" {
		t.Errorf("status = %v after failed bulk, want Saved", out["status"])
	}

	rec := call(t, s, "POST", "/v1/applications/bulk-status",
		fmt.Sprintf(`{"ids":[%d,%d],"status":"Rejected"}`, id1, id2))
	if rec.Code != http.StatusOK {
		t.Fatalf("bulk status = %d; body %s", rec.Code, rec.Body)
	}
	var updated []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil || len(updated) != 2 {
		t.Fatalf("bulk returned %v (err %v), want 2 applications", updated, err)
	}
}

func TestApplicationDelete(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	id := trackJob(t, s, trackableJob(t, s))
	expect(t, call(t, s, "DELETE", fmt.Sprintf("/v1/applications/%d", id), ""), 204)
	expect(t, call(t, s, "GET", fmt.Sprintf("/v1/applications/%d", id), ""), 404)
}
