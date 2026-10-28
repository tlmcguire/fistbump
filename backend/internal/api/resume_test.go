package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
)

// call sends a request with the valid token and an optional JSON body.
func call(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// expect checks the status and returns the decoded JSON body.
func expect(t *testing.T, rec *httptest.ResponseRecorder, status int) map[string]any {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body)
	}
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

// expectField checks a 422 that names the given field.
func expectField(t *testing.T, rec *httptest.ResponseRecorder, field string) {
	t.Helper()
	out := expect(t, rec, http.StatusUnprocessableEntity)
	e := out["error"].(map[string]any)
	details, _ := e["details"].(map[string]any)
	if e["code"] != "validation_failed" || details["field"] != field {
		t.Errorf("error = %v, want validation_failed on %q", e, field)
	}
}

func newResumeServer(t *testing.T) *Server { return newTestServer(t, &fakeFetcher{}) }

func putProfile(t *testing.T, s *Server) {
	t.Helper()
	expect(t, call(t, s, "PUT", "/v1/resume/profile", `{"full_name":"Ada Lovelace","email":"ada@example.com","skills":["sql","excel"]}`), 200)
}

func TestResumeEmpty(t *testing.T) {
	s := newResumeServer(t)
	out := expect(t, call(t, s, "GET", "/v1/resume", ""), 200)
	if out["profile"] != nil || len(out["experiences"].([]any)) != 0 || len(out["skills"].([]any)) != 0 {
		t.Errorf("empty resume = %v", out)
	}
	if md := expect(t, call(t, s, "GET", "/v1/resume/preview", ""), 200)["markdown"]; md != "" {
		t.Errorf("preview = %q, want empty", md)
	}
}

func TestProfilePut(t *testing.T) {
	s := newResumeServer(t)
	rec := call(t, s, "PUT", "/v1/resume/profile", `{"id":7,"created_at":"x","full_name":" Ada ","preferred_mode":"Hybrid","target_positions":["Data Analyst"]}`)
	p := expect(t, rec, 200)
	if p["full_name"] != "Ada" || p["id"] == float64(7) || p["links"] == nil {
		t.Errorf("profile = %v (read-only id ignored, lists never null)", p)
	}
	expect(t, call(t, s, "PUT", "/v1/resume/profile", `{"full_name":"Ada L"}`), 200)
	if got := expect(t, call(t, s, "GET", "/v1/resume", ""), 200)["profile"].(map[string]any); got["full_name"] != "Ada L" {
		t.Errorf("profile not updated: %v", got)
	}
}

func TestProfileCanBeCleared(t *testing.T) {
	s := newResumeServer(t)
	expect(t, call(t, s, "PUT", "/v1/resume/profile", `{"full_name":"Ada Lovelace","email":"ada@example.com","skills":["sql"]}`), 200)
	p := expect(t, call(t, s, "PUT", "/v1/resume/profile", `{"full_name":"  ","email":null,"skills":[]}`), 200)
	if p["full_name"] != "" || p["email"] != nil || len(p["skills"].([]any)) != 0 {
		t.Errorf("cleared profile = %v", p)
	}
	if md := expect(t, call(t, s, "GET", "/v1/resume/preview", ""), 200)["markdown"]; strings.Contains(md.(string), "#") {
		t.Errorf("preview of a cleared profile = %q, want no heading", md)
	}
}

func TestProfileValidation(t *testing.T) {
	s := newResumeServer(t)
	for _, tc := range []struct{ body, field string }{
		{`{"full_name":"` + strings.Repeat("x", 1000) + `"}`, "full_name"},
		{`{"full_name":"A","preferred_mode":"Sideways"}`, "preferred_mode"},
		{`{"full_name":"A","min_desired_pay":-1}`, "min_desired_pay"},
		{`{"full_name":"A","nope":1}`, "nope"},
		{`{"full_name":5}`, "full_name"},
		{`{"full_name":"A","skills":"sql"}`, "skills"},
	} {
		expectField(t, call(t, s, "PUT", "/v1/resume/profile", tc.body), tc.field)
	}
	if rec := call(t, s, "PUT", "/v1/resume/profile", `{"full_name":`); rec.Code != 400 {
		t.Errorf("malformed JSON = %d, want 400", rec.Code)
	}
	if rec := call(t, s, "PUT", "/v1/resume/profile", `{"full_name":"A"} {}`); rec.Code != 400 {
		t.Errorf("trailing data = %d, want 400", rec.Code)
	}
}

func TestExperienceFlow(t *testing.T) {
	s := newResumeServer(t)
	rec := call(t, s, "POST", "/v1/resume/experiences", `{"company_name":"Acme","job_title":"Analyst"}`)
	if e := expect(t, rec, 409)["error"].(map[string]any); e["code"] != "conflict" || e["message"] != "create a profile first" {
		t.Errorf("error = %v", e)
	}
	putProfile(t, s)
	e := expect(t, call(t, s, "POST", "/v1/resume/experiences", `{"company_name":"Acme","job_title":"Analyst","start_date":"2022-06","skills":["python"]}`), 201)
	if e["start_date"] != "2022-06-01" || e["sort_order"] != float64(1) || e["end_date"] != nil {
		t.Errorf("experience = %v (month stored as first of month)", e)
	}
	id := int(e["id"].(float64))
	path := "/v1/resume/experiences/" + itoa(id)
	up := expect(t, call(t, s, "PUT", path, `{"company_name":"Acme","job_title":"Senior","start_date":"2022-06-01","end_date":"2024-01"}`), 200)
	if up["job_title"] != "Senior" || up["sort_order"] != float64(1) {
		t.Errorf("update = %v (sort_order kept when not sent)", up)
	}
	moved := expect(t, call(t, s, "PUT", path, `{"company_name":"Acme","job_title":"Senior","skills":["python"],"sort_order":5}`), 200)
	if moved["sort_order"] != float64(5) {
		t.Errorf("sort_order = %v, want 5", moved["sort_order"])
	}
	if list := call(t, s, "GET", "/v1/resume/experiences", ""); !strings.HasPrefix(list.Body.String(), "[") {
		t.Errorf("list should be a bare array: %s", list.Body)
	}
	if got := expect(t, call(t, s, "GET", "/v1/resume", ""), 200)["skills"].([]any); len(got) != 3 {
		t.Errorf("skills = %v, want union of profile and experience", got)
	}
	expectField(t, call(t, s, "POST", "/v1/resume/experiences", `{"company_name":"A","job_title":"B","start_date":"2024-06-01","end_date":"2024-01-01"}`), "end_date")
	expectField(t, call(t, s, "POST", "/v1/resume/experiences", `{"company_name":"A","job_title":"B","start_date":"June"}`), "start_date")
	expectField(t, call(t, s, "POST", "/v1/resume/experiences", `{"job_title":"B"}`), "company_name")
	if rec := call(t, s, "PUT", "/v1/resume/experiences/999", `{"company_name":"A","job_title":"B"}`); rec.Code != 404 {
		t.Errorf("missing update = %d, want 404", rec.Code)
	}
	if rec := call(t, s, "DELETE", path, ""); rec.Code != 204 {
		t.Errorf("delete = %d, want 204", rec.Code)
	}
	if rec := call(t, s, "DELETE", path, ""); rec.Code != 404 {
		t.Errorf("second delete = %d, want 404", rec.Code)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestEducationFlow(t *testing.T) {
	s := newResumeServer(t)
	if rec := call(t, s, "POST", "/v1/resume/education", `{"institution":"CofC"}`); rec.Code != 409 {
		t.Errorf("no profile = %d, want 409", rec.Code)
	}
	putProfile(t, s)
	expectField(t, call(t, s, "POST", "/v1/resume/education", `{"institution":"CofC","gpa":7}`), "gpa")
	expectField(t, call(t, s, "POST", "/v1/resume/education", `{}`), "institution")
	e := expect(t, call(t, s, "POST", "/v1/resume/education", `{"institution":"CofC","gpa":3.8,"graduation_date":"2022-05"}`), 201)
	if e["graduation_date"] != "2022-05-01" {
		t.Errorf("education = %v", e)
	}
	path := "/v1/resume/education/" + itoa(int(e["id"].(float64)))
	up := expect(t, call(t, s, "PUT", path, `{"institution":"CofC","degree_level":"BS"}`), 200)
	if up["degree_level"] != "BS" || up["gpa"] != nil {
		t.Errorf("update = %v", up)
	}
	if rec := call(t, s, "GET", "/v1/resume/education", ""); !strings.Contains(rec.Body.String(), "CofC") {
		t.Errorf("list = %s", rec.Body)
	}
	if rec := call(t, s, "PUT", "/v1/resume/education/999", `{"institution":"X"}`); rec.Code != 404 {
		t.Errorf("missing = %d", rec.Code)
	}
	if rec := call(t, s, "DELETE", path, ""); rec.Code != 204 {
		t.Errorf("delete = %d", rec.Code)
	}
}

func TestSavedResumes(t *testing.T) {
	s := newResumeServer(t)
	if rec := call(t, s, "POST", "/v1/resumes", `{"version_label":"v1","content":"x"}`); rec.Code != 409 {
		t.Errorf("no profile = %d, want 409", rec.Code)
	}
	putProfile(t, s)
	expectField(t, call(t, s, "POST", "/v1/resumes", `{"version_label":"v1"}`), "content")
	expectField(t, call(t, s, "POST", "/v1/resumes", `{"content":"x"}`), "version_label")
	expectField(t, call(t, s, "POST", "/v1/resumes", `{"version_label":"v1","content":"x","from_master":true}`), "content")

	preview := expect(t, call(t, s, "GET", "/v1/resume/preview", ""), 200)["markdown"].(string)
	v := expect(t, call(t, s, "POST", "/v1/resumes", `{"version_label":"Master","from_master":true,"target_position":"Analyst"}`), 201)
	if v["content"] != preview || !strings.HasPrefix(preview, "# Ada Lovelace") {
		t.Errorf("saved content %q does not match preview %q", v["content"], preview)
	}
	id := itoa(int(v["id"].(float64)))
	expect(t, call(t, s, "POST", "/v1/resumes", `{"version_label":"Pasted","content":"hello"}`), 201)

	list := call(t, s, "GET", "/v1/resumes", "")
	var rows []map[string]any
	json.Unmarshal(list.Body.Bytes(), &rows)
	if len(rows) != 2 || rows[0]["version_label"] != "Pasted" {
		t.Fatalf("list = %s", list.Body)
	}
	if _, has := rows[0]["content"]; has {
		t.Error("list must not include content")
	}
	if got := expect(t, call(t, s, "GET", "/v1/resumes/"+id, ""), 200); got["content"] != preview {
		t.Errorf("get = %v", got)
	}
	up := expect(t, call(t, s, "PUT", "/v1/resumes/"+id, `{"version_label":"Renamed","target_position":null}`), 200)
	if up["version_label"] != "Renamed" || up["target_position"] != nil || up["content"] != preview {
		t.Errorf("update = %v", up)
	}
	expectField(t, call(t, s, "PUT", "/v1/resumes/"+id, `{"version_label":" "}`), "version_label")
	if rec := call(t, s, "GET", "/v1/resumes/999", ""); rec.Code != 404 {
		t.Errorf("missing = %d", rec.Code)
	}
	if rec := call(t, s, "GET", "/v1/resumes/abc", ""); rec.Code != 404 {
		t.Errorf("non-numeric id = %d, want 404", rec.Code)
	}
	if rec := call(t, s, "DELETE", "/v1/resumes/"+id, ""); rec.Code != 204 {
		t.Errorf("delete = %d", rec.Code)
	}
}

func TestSettingsRoutes(t *testing.T) {
	s := newResumeServer(t)
	got := expect(t, call(t, s, "GET", "/v1/settings", ""), 200)
	if got["ai.mode"] != "auto" || got["ai.idle_minutes"] != float64(5) || got["connectors.greenhouse.categories"] == nil {
		t.Errorf("defaults = %v", got)
	}
	put := expect(t, call(t, s, "PUT", "/v1/settings", `{"jobs.trash_days":14}`), 200)
	if put["jobs.trash_days"] != float64(14) || put["jobs.retention_days"] != float64(30) {
		t.Errorf("put = %v, want the full map", put)
	}
	expectField(t, call(t, s, "PUT", "/v1/settings", `{"jobs.trash_days":3,"jobs.retention_days":0}`), "jobs.retention_days")
	if got := expect(t, call(t, s, "GET", "/v1/settings", ""), 200); got["jobs.trash_days"] != float64(14) {
		t.Errorf("a rejected batch changed trash_days to %v", got["jobs.trash_days"])
	}
	expectField(t, call(t, s, "PUT", "/v1/settings", `{"nope":1}`), "nope")
	expectField(t, call(t, s, "PUT", "/v1/settings", `{"ai.api_key":"sk-1"}`), "ai.api_key")
	expectField(t, call(t, s, "PUT", "/v1/settings", `{"ai.idle_minutes":"x"}`), "ai.idle_minutes")
}

func TestSearchKeywordDefault(t *testing.T) {
	f := &fakeSearcher{}
	s := newSearchServer(t, f)
	call(t, s, "POST", "/v1/connectors/greenhouse/fetch", `{"query":{"boards":["acme"]}}`)
	if len(f.gotQuery.Keywords) != 0 {
		t.Errorf("a narrowed search should not get default keywords: %v", f.gotQuery.Keywords)
	}
	if rec := call(t, s, "POST", "/v1/connectors/greenhouse/fetch", `{"query":{}}`); rec.Code != 422 {
		t.Errorf("nothing to search by, no profile = %d, want 422", rec.Code)
	}

	call(t, s, "PUT", "/v1/resume/profile", `{"full_name":"Ada"}`)
	call(t, s, "POST", "/v1/resume/experiences", `{"company_name":"Old","job_title":"Intern","start_date":"2019-01-01","end_date":"2020-01-01"}`)
	call(t, s, "POST", "/v1/resume/experiences", `{"company_name":"Acme","job_title":"Data Analyst","start_date":"2022-06-01"}`)
	f.result = connectors.SearchResult{}
	call(t, s, "POST", "/v1/connectors/greenhouse/fetch", `{"query":{}}`)
	if len(f.gotQuery.Keywords) != 1 || f.gotQuery.Keywords[0] != "Data Analyst" {
		t.Errorf("keywords = %v, want the latest title", f.gotQuery.Keywords)
	}
	call(t, s, "PUT", "/v1/resume/profile", `{"full_name":"Ada","target_positions":["Backend Engineer"]}`)
	call(t, s, "POST", "/v1/connectors/greenhouse/fetch", `{"query":{}}`)
	if len(f.gotQuery.Keywords) != 1 || f.gotQuery.Keywords[0] != "Backend Engineer" {
		t.Errorf("keywords = %v, want target positions", f.gotQuery.Keywords)
	}
	call(t, s, "POST", "/v1/connectors/greenhouse/fetch", `{"query":{"keywords":["go"]}}`)
	if f.gotQuery.Keywords[0] != "go" {
		t.Errorf("request keywords must win: %v", f.gotQuery.Keywords)
	}
}
