package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/analyze"
	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/connectors/greenhouse"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/parser"
)

// fakeParse stands in for the parser, so these tests check the routes and not the heuristics. It
// finds "sql" when the text says SQL and infers company and title from "Title at Company".
func fakeParse(raw string) parser.Result {
	res := parser.Result{Description: "desc", ReqTechSkills: []string{}, PrefTechSkills: []string{},
		ReqSoftSkills: []string{}, PrefSoftSkills: []string{}, Requirements: []string{}}
	if strings.Contains(raw, "SQL") {
		res.ReqTechSkills = []string{"sql"}
	}
	first, _, _ := strings.Cut(raw, "\n")
	if title, company, ok := strings.Cut(first, " at "); ok {
		res.Inferred = parser.Inferred{CompanyName: company, PositionTitle: title, WorkMode: "Hybrid"}
	}
	res.Hidden = strings.Count(raw, "​")
	return res
}

func newJobServer(t *testing.T, f jobFetcher) *Server {
	t.Helper()
	s := newTestServer(t, f)
	s.parse, s.analyze = fakeParse, fakeAnalyze
	return s
}

func TestJobParse(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	out := expect(t, call(t, s, "POST", "/v1/jobs/parse", `{"raw_text":"Analyst at Acme\n​SQL"}`), 200)
	if out["inferred"].(map[string]any)["company_name"] != "Acme" || out["req_tech_skills"].([]any)[0] != "sql" {
		t.Errorf("parse = %v", out)
	}
	if out["hidden_text"].(map[string]any)["characters"] != float64(1) {
		t.Errorf("hidden_text = %v", out["hidden_text"])
	}
	if out := expect(t, call(t, s, "POST", "/v1/jobs/parse", `{"raw_text":"plain"}`), 200); out["hidden_text"] != nil {
		t.Errorf("hidden_text = %v, want null", out["hidden_text"])
	}
	expectField(t, call(t, s, "POST", "/v1/jobs/parse", `{"raw_text":"  "}`), "raw_text")
	if n := len(expect(t, call(t, s, "GET", "/v1/jobs", ""), 200)); n != 0 {
		t.Error("parse must not write")
	}
}

func TestJobCreatePasted(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	out := expect(t, call(t, s, "POST", "/v1/jobs", `{"source":"pasted","raw_text":"Data Analyst at Acme Corp\nSQL",
		"company_url":"https://acme.com/careers"}`), 201)
	if out["company_name"] != "Acme Corp" || out["position_title"] != "Data Analyst" || out["work_mode"] != "Hybrid" {
		t.Errorf("inferred fields = %v", out)
	}
	if out["req_tech_skills"].([]any)[0] != "sql" || out["company_url"] != "https://acme.com/careers" || out["hidden_text"] != nil {
		t.Errorf("created = %v", out)
	}
	// Given values win over inferred ones.
	out = expect(t, call(t, s, "POST", "/v1/jobs", `{"raw_text":"Data Analyst at Acme","company_name":"Globex","work_mode":"Remote"}`), 201)
	if out["company_name"] != "Globex" || out["work_mode"] != "Remote" || out["source"] != "pasted" {
		t.Errorf("given values = %v", out)
	}

	expectField(t, call(t, s, "POST", "/v1/jobs", `{"source":"pasted","raw_text":"no title line"}`), "company_name")
	expectField(t, call(t, s, "POST", "/v1/jobs", `{"source":"pasted"}`), "raw_text")
	expectField(t, call(t, s, "POST", "/v1/jobs", `{"source":"greenhouse","raw_text":"x"}`), "source")
	expectField(t, call(t, s, "POST", "/v1/jobs", `{"raw_text":"A at B","company_url":"http://acme.com"}`), "company_url")
	expectField(t, call(t, s, "POST", "/v1/jobs", `{"raw_text":"A at B","listing_url":"javascript:alert(1)"}`), "listing_url")
	expectField(t, call(t, s, "POST", "/v1/jobs", `{"raw_text":"A at B","work_mode":"Sometimes"}`), "work_mode")
	expectField(t, call(t, s, "POST", "/v1/jobs", `{"raw_text":"A at B","pay_min":10,"pay_max":5}`), "pay_min")
	expectField(t, call(t, s, "POST", "/v1/jobs", `{"raw_text":"A at B","color":"red"}`), "color")
}

func TestJobManualFlow(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	out := expect(t, call(t, s, "POST", "/v1/jobs", `{"source":"manual","company_name":"Acme","position_title":"Analyst",
		"raw_text":"SQL","company_url":"https://acme.com"}`), 201)
	if len(out["req_tech_skills"].([]any)) != 0 {
		t.Errorf("a manual job is not parsed: %v", out)
	}
	id := fmt.Sprint(out["id"])
	expectField(t, call(t, s, "POST", "/v1/jobs", `{"source":"manual","position_title":"Analyst"}`), "company_name")

	list := expect2(t, call(t, s, "GET", "/v1/jobs?q=acm", ""))
	if len(list) != 1 || list[0]["raw_text"] != nil {
		t.Errorf("list = %v", list)
	}
	expectField(t, call(t, s, "GET", "/v1/jobs?archived=yes", ""), "archived")
	expectField(t, call(t, s, "GET", "/v1/jobs?limit=500", ""), "limit")

	// New raw_text re-runs the parser; null clears an optional field; read-only fields are ignored.
	out = expect(t, call(t, s, "PUT", "/v1/jobs/"+id, `{"raw_text":"Needs SQL","company_url":null,"source":"greenhouse"}`), 200)
	if out["req_tech_skills"].([]any)[0] != "sql" || out["company_url"] != nil || out["source"] != "manual" {
		t.Errorf("update = %v", out)
	}
	// Skill lists given with raw_text are kept as given.
	out = expect(t, call(t, s, "PUT", "/v1/jobs/"+id, `{"raw_text":"Needs SQL again","req_tech_skills":["go"]}`), 200)
	if out["req_tech_skills"].([]any)[0] != "go" {
		t.Errorf("skills given = %v", out["req_tech_skills"])
	}
	out = expect(t, call(t, s, "PUT", "/v1/jobs/"+id, `{"position_title":"Senior Analyst"}`), 200)
	if out["position_title"] != "Senior Analyst" || out["req_tech_skills"].([]any)[0] != "go" {
		t.Errorf("partial update = %v", out)
	}
	expectField(t, call(t, s, "PUT", "/v1/jobs/"+id, `{"position_title":"  "}`), "position_title")
	expectField(t, call(t, s, "PUT", "/v1/jobs/"+id, `{"pay_min":"lots"}`), "pay_min")
	expectField(t, call(t, s, "PUT", "/v1/jobs/"+id, `{"color":"red"}`), "color")
	wantError(t, call(t, s, "PUT", "/v1/jobs/99", `{}`), 404, "not_found")

	expect(t, call(t, s, "GET", "/v1/jobs/"+id, ""), 200)
	if rec := call(t, s, "DELETE", "/v1/jobs/"+id, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	wantError(t, call(t, s, "GET", "/v1/jobs/"+id, ""), 404, "not_found")
}

// expect2 checks a 200 list response and returns its items.
func expect2(t *testing.T, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	if rec.Code != 200 {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// fakeAnalyze matches a job skill when the master lists it, without the real package's rules.
func fakeAnalyze(j models.Job, m models.Master) analyze.Result {
	have := map[string]bool{}
	for _, skill := range m.Skills() {
		have[skill] = true
	}
	r := analyze.Result{JobID: j.ID, Score: 1}
	r.Required.Tech = analyze.Group{Matched: []analyze.Match{}, Missing: []string{}}
	for _, skill := range j.ReqTechSkills {
		if have[skill] {
			r.Required.Tech.Matched = append(r.Required.Tech.Matched, analyze.Match{Skill: skill, ExperienceIDs: []int64{}})
		} else {
			r.Required.Tech.Missing = append(r.Required.Tech.Missing, skill)
		}
	}
	if n := len(j.ReqTechSkills); n > 0 {
		r.Score = float64(len(r.Required.Tech.Matched)) / float64(n)
	}
	return r
}

func TestJobDeleteWithApplication(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	putProfile(t, s)
	out := expect(t, call(t, s, "POST", "/v1/jobs", `{"raw_text":"Analyst at Acme"}`), 201)
	res, err := s.db.Exec(`INSERT INTO applications (job_id, profile_id) VALUES (?, 1)`, out["id"])
	if err != nil {
		t.Fatal(err)
	}
	appID, _ := res.LastInsertId()
	rec := call(t, s, "DELETE", fmt.Sprintf("/v1/jobs/%v", out["id"]), "")
	body := decodeError(t, rec)
	if rec.Code != 409 || body.Error.Code != "conflict" || body.Error.Details["application_id"] != float64(appID) {
		t.Errorf("delete = %d %+v, want 409 naming application %d", rec.Code, body.Error, appID)
	}
}

func TestJobAnalyze(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	out := expect(t, call(t, s, "POST", "/v1/jobs", `{"raw_text":"Analyst at Acme\nSQL"}`), 201)
	path := fmt.Sprintf("/v1/jobs/%v/analyze", out["id"])
	wantError(t, call(t, s, "POST", path, ""), 409, "conflict")
	wantError(t, call(t, s, "POST", "/v1/jobs/99/analyze", ""), 404, "not_found")

	putProfile(t, s) // skills: sql, excel
	res := expect(t, call(t, s, "POST", path, ""), 200)
	matched := res["required"].(map[string]any)["tech"].(map[string]any)["matched"].([]any)
	if res["job_id"] != out["id"] || res["score"] != float64(1) || len(matched) != 1 {
		t.Errorf("analysis = %v", res)
	}
	if _, ok := res["duration_ms"]; !ok {
		t.Error("analysis has no duration_ms")
	}
}

// postingFetcher serves canned postings by job id. An id in closed is a 404; in broken, a 502.
type postingFetcher struct {
	text   string
	closed map[string]bool
	broken map[string]bool
}

func (f *postingFetcher) FetchJob(_ context.Context, board, jobID string) (connectors.Posting, error) {
	switch {
	case f.closed[jobID]:
		return connectors.Posting{}, greenhouse.ErrorNotFound
	case f.broken[jobID]:
		return connectors.Posting{}, fmt.Errorf("%w: boom", greenhouse.ErrorUpstream)
	}
	site := "https://acme.com/jobs/" + jobID
	return connectors.Posting{ExternalID: jobID, Board: board, CompanyName: "Acme", PositionTitle: "Analyst " + jobID,
		Location: "Remote - US", ListingURL: "https://job-boards.greenhouse.io/" + board + "/jobs/" + jobID,
		CompanyURL: &site, RawText: f.text}, nil
}

func TestGreenhouseImport(t *testing.T) {
	f := &postingFetcher{text: "Knows SQL", closed: map[string]bool{"404": true}, broken: map[string]bool{"500": true}}
	s := newJobServer(t, f)
	const one = `{"postings":[{"board":"acme","external_id":"1"},{"board":"acme","external_id":"2"},{"board":"acme","external_id":"1"}]}`
	out := expect(t, call(t, s, "POST", "/v1/connectors/greenhouse/import", one), 200)
	imported := out["imported"].([]any)
	if len(imported) != 2 || len(out["refreshed"].([]any)) != 0 {
		t.Fatalf("first import = %v", out)
	}
	j := imported[0].(map[string]any)
	if j["source"] != "greenhouse" || j["board"] != "acme" || j["company_url"] != "https://acme.com/jobs/1" ||
		j["work_mode"] != "Remote" || j["req_tech_skills"].([]any)[0] != "sql" {
		t.Errorf("imported job = %v", j)
	}

	f.text = "Knows SQL and more"
	out = expect(t, call(t, s, "POST", "/v1/connectors/greenhouse/import", `{"postings":[{"board":"acme","external_id":"1"}]}`), 200)
	refreshed := out["refreshed"].([]any)
	if len(out["imported"].([]any)) != 0 || len(refreshed) != 1 || refreshed[0].(map[string]any)["id"] != j["id"] {
		t.Fatalf("second import = %v; want the same row refreshed", out)
	}
	if refreshed[0].(map[string]any)["raw_text"] != "Knows SQL and more" {
		t.Errorf("refresh kept old text: %v", refreshed[0])
	}

	// A closed posting fails the whole import and writes nothing.
	rec := call(t, s, "POST", "/v1/connectors/greenhouse/import", `{"postings":[{"board":"acme","external_id":"3"},{"board":"acme","external_id":"404"}]}`)
	body := decodeError(t, rec)
	if rec.Code != 404 || body.Error.Code != "not_found" || body.Error.Details["external_id"] != "404" {
		t.Errorf("closed posting = %d %+v, want 404 naming external_id 404", rec.Code, body.Error)
	}
	if list := expect2(t, call(t, s, "GET", "/v1/jobs", "")); len(list) != 2 {
		t.Errorf("jobs after a failed import = %d, want 2", len(list))
	}
	wantError(t, call(t, s, "POST", "/v1/connectors/greenhouse/import", `{"postings":[{"board":"acme","external_id":"500"}]}`), 502, "upstream_error")

	expectField(t, call(t, s, "POST", "/v1/connectors/greenhouse/import", `{"postings":[]}`), "postings")
	wantError(t, call(t, s, "POST", "/v1/connectors/greenhouse/import", `{"postings":[{"board":"Bad Board","external_id":"1"}]}`), 422, "validation_failed")
	expectField(t, call(t, s, "POST", "/v1/connectors/greenhouse/import", `{"external_ids":["1"]}`), "external_ids")
}

func TestSearchSkillScorerNeedsProfile(t *testing.T) {
	f := &fakeSearcher{}
	s := newSearchServer(t, f)
	s.parse, s.analyze = fakeParse, fakeAnalyze
	call(t, s, "POST", "/v1/connectors/greenhouse/fetch", `{"query":{"keywords":["analyst"]}}`)
	if f.gotQuery.SkillScore != nil {
		t.Error("without a profile, search must not score skills")
	}
	putProfile(t, s) // skills: sql, excel
	call(t, s, "POST", "/v1/connectors/greenhouse/fetch", `{"query":{"keywords":["analyst"]}}`)
	if f.gotQuery.SkillScore == nil {
		t.Fatal("with a profile, search should get a skill scorer")
	}
	if got := f.gotQuery.SkillScore("Needs SQL"); got != 1 {
		t.Errorf("score for a posting the resume fits = %v, want 1", got)
	}
}
