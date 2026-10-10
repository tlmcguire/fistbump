package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/db"
	"github.com/tlmcguire/fistbump/backend/internal/pdf"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

type env struct {
	t   *testing.T
	srv *httptest.Server
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, DBFile))
	if err != nil {
		t.Fatal(err)
	}
	s := New(Config{Store: store.New(conn), DataDir: dir, Token: "tok"})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { ts.Close(); s.Shutdown(); conn.Close() })
	return &env{t, ts}
}

func (e *env) do(method, path string, body any) (int, []byte) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	req.Header.Set("Authorization", "Bearer tok")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

// call asserts the status and decodes the JSON body into a generic map.
func (e *env) call(method, path string, body any, want int) map[string]any {
	e.t.Helper()
	code, data := e.do(method, path, body)
	if code != want {
		e.t.Fatalf("%s %s = %d, want %d: %s", method, path, code, want, data)
	}
	out := map[string]any{}
	_ = json.Unmarshal(data, &out)
	return out
}

func num(v any) int64 { return int64(v.(float64)) }

func TestAuthRequired(t *testing.T) {
	e := newEnv(t)
	resp, _ := http.Get(e.srv.URL + "/v1/health")
	if resp.StatusCode != 401 {
		t.Fatalf("no token: %d", resp.StatusCode)
	}
	h := e.call("GET", "/v1/health", nil, 200)
	if h["status"] != "ok" || num(h["schema_version"]) != 6 {
		t.Fatalf("health = %v", h)
	}
}

func TestValidationErrors(t *testing.T) {
	e := newEnv(t)
	got := e.call("PUT", "/v1/resume/profile", map[string]any{"full_name": ""}, 422)
	if got["error"].(map[string]any)["code"] != "validation_failed" {
		t.Fatalf("error = %v", got)
	}
	e.call("PUT", "/v1/resume/profile", map[string]any{"full_name": "A", "preferred_mode": "Sideways"}, 422)
	e.call("PUT", "/v1/resume/profile", map[string]any{"full_name": "A", "bogus": 1}, 422)
	e.call("POST", "/v1/resume/experiences", map[string]any{"company_name": "X", "job_title": "Y"}, 409) // no profile
	// Tracking a job before any resume exists says what to do, not "a profile ... conflicts".
	job := e.call("POST", "/v1/jobs", map[string]any{"source": "pasted", "raw_text": "Backend Engineer\nCompany: Acme"}, 201)
	noProfile := e.call("POST", "/v1/applications", map[string]any{"job_id": num(job["id"])}, 409)
	if msg := noProfile["error"].(map[string]any)["message"]; msg != "save your resume first" {
		t.Fatalf("message = %v", msg)
	}
	code, _ := e.do("PUT", "/v1/resume/profile", nil)
	if code != 400 {
		t.Fatalf("empty body = %d", code)
	}
	e.call("PUT", "/v1/settings", map[string]any{"ai.remote.api_key": "x"}, 422)
	e.call("PUT", "/v1/settings", map[string]any{"nope": 1}, 422)
}

func TestFullWorkflow(t *testing.T) {
	e := newEnv(t)
	e.call("PUT", "/v1/resume/profile", map[string]any{
		"full_name": "Ann Lee", "email": "ann@example.com", "target_positions": []string{"Backend Engineer"}, "preferred_mode": "Remote",
	}, 200)
	exp := e.call("POST", "/v1/resume/experiences", map[string]any{
		"company_name": "Initech", "job_title": "Software Engineer", "start_date": "2020-01-01",
		"description": "Built payment services in Go and PostgreSQL.", "impact": "Mentored 3 engineers and cut latency 40%.",
		"skills": []string{"Go", "SQL", "go", "Docker"},
	}, 201)
	if sk := exp["skills"].([]any); len(sk) != 3 || sk[0] != "go" {
		t.Fatalf("skills not normalized: %v", sk)
	}
	e.call("POST", "/v1/resume/education", map[string]any{"institution": "State U", "degree_level": "BS", "gpa": 3.8}, 201)
	e.call("POST", "/v1/resume/education", map[string]any{"institution": "State U", "gpa": 9}, 422)
	e.call("POST", "/v1/resume/experiences", map[string]any{"company_name": "X", "job_title": "Y", "start_date": "2020-05-01", "end_date": "2019-01-01"}, 422)

	res := e.call("GET", "/v1/resume", nil, 200)
	if len(res["skills"].([]any)) != 3 {
		t.Fatalf("resume = %v", res)
	}

	parsed := e.call("POST", "/v1/jobs/parse", map[string]any{"raw_text": "Backend Engineer\nCompany: Acme\nRequirements\n- Go and Kubernetes\n- Mentoring\nNice to have\n- Terraform"}, 200)
	if parsed["inferred"].(map[string]any)["company_name"] != "Acme" {
		t.Fatalf("parse = %v", parsed)
	}
	job := e.call("POST", "/v1/jobs", map[string]any{"source": "pasted",
		"raw_text": "Backend Engineer\nCompany: Acme\nRequirements\n- Go, SQL and Kubernetes\n- Mentoring\nNice to have\n- Terraform"}, 201)
	jobID := num(job["id"])
	if job["company_name"] != "Acme" || job["position_title"] != "Backend Engineer" {
		t.Fatalf("job = %v", job)
	}
	e.call("POST", "/v1/jobs", map[string]any{"source": "pasted"}, 422)

	an := e.call("POST", "/v1/jobs/"+itoa(jobID)+"/analyze", nil, 200)
	if an["score"].(float64) < 0.7 {
		t.Fatalf("analysis = %v", an)
	}
	missing := an["required"].(map[string]any)["tech"].(map[string]any)["missing"].([]any)
	if len(missing) != 1 || missing[0] != "kubernetes" {
		t.Fatalf("missing = %v", missing)
	}

	// Revision with the rules engine.
	rev := e.call("POST", "/v1/revisions", map[string]any{"job_id": jobID, "engine": "rules"}, 202)
	revID := num(rev["id"])
	e.call("POST", "/v1/revisions", map[string]any{"job_id": jobID, "engine": "remote"}, 503)
	var full map[string]any
	for i := 0; i < 100; i++ {
		full = e.call("GET", "/v1/revisions/"+itoa(revID), nil, 200)
		if full["status"] == "done" || full["status"] == "failed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if full["status"] != "done" || full["engine"] != "rules" {
		t.Fatalf("revision = %v", full)
	}
	sugs := full["suggestions"].([]any)
	if len(sugs) == 0 {
		t.Fatal("no suggestions")
	}

	// Pending suggestions block the tailored resume.
	blocked := e.call("POST", "/v1/tailored-resumes", map[string]any{"revision_id": revID}, 409)
	if num(blocked["error"].(map[string]any)["details"].(map[string]any)["pending"]) != int64(len(sugs)) {
		t.Fatalf("blocked = %v", blocked)
	}
	for i, g := range sugs {
		id := itoa(num(g.(map[string]any)["id"]))
		base := "/v1/revisions/" + itoa(revID) + "/suggestions/" + id
		switch {
		case i == 0:
			e.call("POST", base+"/edit", map[string]any{"edited_text": "Edited summary text."}, 200)
		case i%2 == 1:
			e.call("POST", base+"/accept", nil, 200)
		default:
			e.call("POST", base+"/reject", nil, 200)
		}
	}
	tr := e.call("POST", "/v1/tailored-resumes", map[string]any{"revision_id": revID}, 201)
	trID := itoa(num(tr["id"]))
	if !strings.Contains(tr["content"].(string), "Edited summary text.") {
		t.Fatalf("tailored content:\n%s", tr["content"])
	}
	d := e.call("GET", "/v1/tailored-resumes/"+trID+"/diff", nil, 200)
	hunks := d["hunks"].([]any)
	changed := 0
	for _, h := range hunks {
		if s := h.(map[string]any)["status"]; s == "changed" || s == "added" {
			changed++
		}
	}
	if changed == 0 {
		t.Fatalf("diff has no changes: %v", d)
	}

	code, body := e.do("POST", "/v1/tailored-resumes/"+trID+"/export", map[string]any{"format": "pdf"})
	if code != 200 || !bytes.HasPrefix(body, []byte("%PDF")) {
		t.Fatalf("pdf export = %d %.20q", code, body)
	}
	code, body = e.do("POST", "/v1/tailored-resumes/"+trID+"/export", map[string]any{"format": "md"})
	if code != 200 || !strings.Contains(string(body), "Ann Lee") {
		t.Fatalf("md export = %d %s", code, body)
	}

	// Application tracking.
	app := e.call("POST", "/v1/applications", map[string]any{"job_id": jobID, "tailored_resume_id": num(tr["id"])}, 201)
	if app["status"] != "Saved" || app["company_name"] != "Acme" {
		t.Fatalf("app = %v", app)
	}
	appID := itoa(num(app["id"]))
	up := e.call("PUT", "/v1/applications/"+appID, map[string]any{"status": "Applied"}, 200)
	if up["date_applied"] == nil {
		t.Fatalf("date_applied not set: %v", up)
	}
	e.call("PUT", "/v1/applications/"+appID, map[string]any{"job_id": 5}, 422)
	if dup := e.call("POST", "/v1/applications", map[string]any{"job_id": jobID}, 409); dup["error"].(map[string]any)["details"].(map[string]any)["application_id"] == nil {
		t.Fatalf("duplicate = %v", dup)
	}
	e.call("POST", "/v1/applications", map[string]any{"job_id": 999}, 422)
	e.call("DELETE", "/v1/jobs/"+itoa(jobID), nil, 409) // RESTRICT

	// Archiving hides the job from the default list and can archive its application in the same step.
	arch := e.call("POST", "/v1/jobs/"+itoa(jobID)+"/archive", map[string]any{"archive_application": true}, 200)
	if arch["archived_at"] == nil {
		t.Fatalf("archive = %v", arch)
	}
	if code, body := e.do("GET", "/v1/jobs", nil); code != 200 || strings.Contains(string(body), `"id":`+itoa(jobID)+`,`) {
		t.Fatalf("archived job still listed: %s", body)
	}
	if code, body := e.do("GET", "/v1/jobs?archived=only", nil); code != 200 || !strings.Contains(string(body), "Acme") {
		t.Fatalf("archived list = %s", body)
	}
	if got := e.call("GET", "/v1/applications/"+appID, nil, 200); got["status"] != "Archived" {
		t.Fatalf("application not archived with job: %v", got)
	}
	e.call("POST", "/v1/jobs/"+itoa(jobID)+"/unarchive", nil, 200)
	e.call("GET", "/v1/jobs?archived=bogus", nil, 422)
	e.call("POST", "/v1/applications/bulk-status", map[string]any{"ids": []int64{num(app["id"])}, "status": "Applied"}, 200)
	e.call("POST", "/v1/applications/bulk-status", map[string]any{"ids": []int64{num(app["id"])}, "status": "Nope"}, 422)
	e.call("DELETE", "/v1/tailored-resumes/"+trID, nil, 204)
	if got := e.call("GET", "/v1/applications/"+appID, nil, 200); got["tailored_resume_id"] != nil {
		t.Fatalf("tailored_resume_id should be null after delete: %v", got)
	}
}

func TestStorageCleanupDryRun(t *testing.T) {
	e := newEnv(t)
	e.call("POST", "/v1/jobs", map[string]any{"source": "manual", "company_name": "A", "position_title": "B"}, 201)
	st := e.call("GET", "/v1/storage", nil, 200)
	if num(st["job_count"]) != 1 {
		t.Fatalf("storage = %v", st)
	}
	out := e.call("POST", "/v1/storage/cleanup", map[string]any{"targets": []string{"old_jobs"}, "older_than_days": 1, "dry_run": true}, 200)
	_ = out
	if st := e.call("GET", "/v1/storage", nil, 200); num(st["job_count"]) != 1 {
		t.Fatal("dry run deleted data")
	}
}

func TestConnectorsDisabled(t *testing.T) {
	e := newEnv(t)
	e.call("PUT", "/v1/connectors/greenhouse", map[string]any{"enabled": false}, 200)
	e.call("POST", "/v1/connectors/greenhouse/fetch", map[string]any{"query": map[string]any{"boards": []string{"x"}}}, 409)
	cats := e.call("GET", "/v1/connectors/greenhouse/categories", nil, 200)
	if len(cats["categories"].([]any)) != 32 || cats["notice"] == "" {
		t.Fatalf("categories = %v", cats)
	}
	e.call("PUT", "/v1/connectors/greenhouse/categories/nope", map[string]any{"enabled": true}, 404)
	e.call("PUT", "/v1/connectors/greenhouse", map[string]any{"enabled": true}, 200)
	e.call("GET", "/v1/connectors/greenhouse/postings/123", nil, 404)
	c := e.call("PUT", "/v1/connectors/greenhouse/categories/ai_ml", map[string]any{"enabled": true}, 200)
	if c["enabled"] != true {
		t.Fatalf("category = %v", c)
	}
}

func TestAIStatusAndSettings(t *testing.T) {
	e := newEnv(t)
	st := e.call("GET", "/v1/ai/status", nil, 200)
	if st["mode"] != "auto" || st["active_engine"] != "rules" {
		t.Fatalf("status = %v", st)
	}
	e.call("PUT", "/v1/ai/remote", map[string]any{"base_url": "http://example.com/v1", "model": "m"}, 422) // http, non-loopback
	st = e.call("PUT", "/v1/ai/remote", map[string]any{"base_url": "http://127.0.0.1:11434/v1", "model": "llama3", "api_key": "secret"}, 200)
	r := st["remote"].(map[string]any)
	if r["has_key"] != true || strings.Contains(toJSON(st), "secret") {
		t.Fatalf("key leaked or missing: %v", st)
	}
	if st["active_engine"] != "remote" {
		t.Fatalf("active = %v", st["active_engine"])
	}
	e.call("PUT", "/v1/ai/mode", map[string]any{"mode": "bogus"}, 422)
	e.call("PUT", "/v1/settings", map[string]any{"ai.selected_model": "nope"}, 422)
	e.call("GET", "/v1/models", nil, 200)
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }
func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestImportPDFApplyAndGenerate(t *testing.T) {
	e := newEnv(t)
	md := "# Jordan Rivera\n\njordan@example.com | Austin, TX\n\n## Experience\n\n### Software Engineer, Initech\n\n2021-01 – Present | Austin, TX\n\n- Built Go services on Kubernetes\n- Cut latency 40%\n\n## Education\n\n### BS in Computer Science, State University\n\n2019\n\n## Skills\n\ngo, sql, kubernetes\n"
	data, err := pdf.Render(md)
	if err != nil {
		t.Fatal(err)
	}
	got := e.call("POST", "/v1/resume/import", map[string]any{"filename": "cv.pdf", "data_base64": base64.StdEncoding.EncodeToString(data)}, 200)
	draft := got["draft"].(map[string]any)
	if draft["profile"].(map[string]any)["full_name"] != "Jordan Rivera" || len(draft["experiences"].([]any)) != 1 || got["engine"] != "rules" {
		t.Fatalf("draft = %v\ntext = %v", draft, got["text"])
	}
	e.call("POST", "/v1/resume/import", map[string]any{"filename": "cv.docx", "data_base64": "AAAA"}, 422)
	e.call("POST", "/v1/resume/import", map[string]any{"filename": "cv.pdf", "data_base64": base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 junk"))}, 422)

	res := e.call("POST", "/v1/resume/import/apply", map[string]any{"draft": draft, "replace": true, "save_copy": map[string]any{"version_label": "Original", "content": got["text"]}}, 200)
	if len(res["experiences"].([]any)) != 1 || len(res["education"].([]any)) != 1 {
		t.Fatalf("applied = %v", res)
	}
	// Applying again with replace does not duplicate entries.
	res = e.call("POST", "/v1/resume/import/apply", map[string]any{"draft": draft, "replace": true}, 200)
	if len(res["experiences"].([]any)) != 1 {
		t.Fatalf("replace duplicated entries: %v", res["experiences"])
	}
	bad := map[string]any{"profile": map[string]any{"full_name": "X"}, "experiences": []any{map[string]any{"company_name": "", "job_title": "Y"}}}
	if r := e.call("POST", "/v1/resume/import/apply", map[string]any{"draft": bad}, 422); r["error"].(map[string]any)["details"].(map[string]any)["experience_index"] == nil {
		t.Fatalf("missing index: %v", r)
	}

	sum := e.call("POST", "/v1/resume/summary", nil, 200)
	if !strings.Contains(sum["summary"].(string), "Software Engineer") || sum["engine"] != "rules" {
		t.Fatalf("summary = %v", sum)
	}
	if pv := e.call("GET", "/v1/resume/preview", nil, 200); !strings.Contains(pv["markdown"].(string), "# Jordan Rivera") {
		t.Fatalf("preview = %v", pv)
	}
	code, body := e.do("POST", "/v1/resume/export", map[string]any{"format": "pdf"})
	if code != 200 || !bytes.HasPrefix(body, []byte("%PDF")) {
		t.Fatalf("export = %d", code)
	}
	if rs := e.call("GET", "/v1/resume", nil, 200); rs["profile"].(map[string]any)["target_positions"].([]any)[0] != "Software Engineer" {
		t.Fatalf("target positions not set from import: %v", rs["profile"])
	}
}

func TestSettingsAreAllOrNothing(t *testing.T) {
	e := newEnv(t)
	e.call("PUT", "/v1/settings", map[string]any{"ai.idle_minutes": 9, "jobs.retention_days": 0}, 422)
	got := e.call("GET", "/v1/settings", nil, 200)
	if num(got["ai.idle_minutes"]) != 5 {
		t.Fatalf("valid key was written despite the invalid one: %v", got["ai.idle_minutes"])
	}
	e.call("POST", "/v1/storage/cleanup", map[string]any{"targets": []string{"old_jobs"}, "older_than_days": 0}, 422)
}

func TestImportApplyValidatesBeforeWriting(t *testing.T) {
	e := newEnv(t)
	e.call("PUT", "/v1/resume/profile", map[string]any{"full_name": "Ann"}, 200)
	e.call("POST", "/v1/resume/experiences", map[string]any{"company_name": "Keep", "job_title": "Engineer"}, 201)
	e.call("POST", "/v1/resume/import/apply", map[string]any{"replace": true, "draft": map[string]any{
		"profile":     map[string]any{"full_name": "Ann"},
		"experiences": []any{map[string]any{"company_name": "New", "job_title": "Dev"}},
		"education":   []any{map[string]any{"institution": ""}},
	}}, 422)
	exps := e.call("GET", "/v1/resume", nil, 200)["experiences"].([]any)
	if len(exps) != 1 || exps[0].(map[string]any)["company_name"] != "Keep" {
		t.Fatalf("resume changed after a rejected apply: %v", exps)
	}
}

func TestRemoteWithoutKeyIsSkipped(t *testing.T) {
	e := newEnv(t)
	st := e.call("PUT", "/v1/ai/remote", map[string]any{"base_url": "https://api.example.com/v1", "model": "m"}, 200)
	r := st["remote"].(map[string]any)
	if r["needs_key"] != true || st["active_engine"] != "rules" {
		t.Fatalf("hosted remote without a key should not be used: %v", st)
	}
	st = e.call("PUT", "/v1/ai/remote", map[string]any{"base_url": "http://localhost:11434/v1", "model": "m"}, 200)
	if st["remote"].(map[string]any)["needs_key"] != false || st["active_engine"] != "remote" {
		t.Fatalf("local server needs no key: %v", st)
	}
}

func TestRejectedStatus(t *testing.T) {
	e := newEnv(t)
	e.call("PUT", "/v1/resume/profile", map[string]any{"full_name": "Ann"}, 200)
	job := e.call("POST", "/v1/jobs", map[string]any{"source": "manual", "company_name": "Acme", "position_title": "Dev"}, 201)
	app := e.call("POST", "/v1/applications", map[string]any{"job_id": num(job["id"]), "status": "Applied"}, 201)
	got := e.call("PUT", "/v1/applications/"+itoa(num(app["id"])), map[string]any{"status": "Rejected", "notes": "No offer after the final round"}, 200)
	if got["status"] != "Rejected" || got["date_applied"] == nil {
		t.Fatalf("rejected = %v", got)
	}
	if list := e.call("GET", "/v1/applications?status=Rejected", nil, 200); list == nil {
		t.Fatal("filter by Rejected failed")
	}
}
