package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
	"github.com/tlmcguire/fistbump/backend/internal/db"
)

// blocker is an engine that starts a round and never finishes it, until the round is canceled.
type blocker struct{ started chan struct{} }

func newBlocker() blocker { return blocker{started: make(chan struct{}, 8)} }

func (blocker) Name() string { return ai.EngineLocal }
func (blocker) Ready() bool  { return true }
func (b blocker) Suggest(ctx context.Context, _ ai.Request) ([]ai.Draft, error) {
	b.started <- struct{}{}
	<-ctx.Done()
	return nil, ctx.Err()
}

// failing is an engine whose rounds always fail.
type failing struct{}

func (failing) Name() string { return ai.EngineLocal }
func (failing) Ready() bool  { return true }
func (failing) Suggest(context.Context, ai.Request) ([]ai.Draft, error) {
	return nil, errors.New("model crashed")
}

// newRoundServer builds a server with the given engines, and the parser and analysis fakes.
func newRoundServer(t *testing.T, engines ai.Engines) *Server {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	s := New(Config{DB: conn, Greenhouse: &fakeFetcher{}, Token: testToken, Version: "test", Engines: engines})
	s.parse, s.analyze = fakeParse, fakeAnalyze
	return s
}

// seedRound saves a profile whose skills and summary the rules engine can improve, and a job that
// asks for SQL. It returns the job id.
func seedRound(t *testing.T, s *Server) string {
	t.Helper()
	expect(t, call(t, s, "PUT", "/v1/resume/profile",
		`{"full_name":"Ada Lovelace","summary":"Analyst.","skills":["excel","sql"]}`), 200)
	job := expect(t, call(t, s, "POST", "/v1/jobs", `{"raw_text":"Data Analyst at Acme\nSQL"}`), 201)
	return fmt.Sprint(job["id"])
}

// startRound posts a revision for the job and returns its id.
func startRound(t *testing.T, s *Server, jobID, engine string) string {
	t.Helper()
	body := fmt.Sprintf(`{"job_id":%s,"engine":%q}`, jobID, engine)
	out := expect(t, call(t, s, "POST", "/v1/revisions", body), http.StatusAccepted)
	return fmt.Sprint(out["id"])
}

// waitStatus polls a revision until it has the wanted status.
func waitStatus(t *testing.T, s *Server, id, want string) map[string]any {
	t.Helper()
	var out map[string]any
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		out = expect(t, call(t, s, "GET", "/v1/revisions/"+id, ""), 200)
		if out["status"] == want {
			return out
		}
	}
	t.Fatalf("revision %s never reached %q; last seen %v", id, want, out["status"])
	return nil
}

func suggestionIDs(t *testing.T, rev map[string]any) []string {
	t.Helper()
	var ids []string
	for _, sg := range rev["suggestions"].([]any) {
		ids = append(ids, fmt.Sprint(sg.(map[string]any)["id"]))
	}
	return ids
}

func TestFullRoundWithRules(t *testing.T) {
	s := newRoundServer(t, ai.Engines{Rules: ai.Rules{}})
	jobID := seedRound(t, s)

	created := expect(t, call(t, s, "POST", "/v1/revisions", `{"job_id":`+jobID+`}`), http.StatusAccepted)
	id := fmt.Sprint(created["id"])
	if created["status"] != "queued" || created["engine"] != "rules" {
		t.Errorf("created = %v", created)
	}

	rev := waitStatus(t, s, id, "done")
	if rev["engine"] != "rules" || rev["completed_at"] == nil {
		t.Errorf("done revision = %v", rev)
	}
	ids := suggestionIDs(t, rev)
	if len(ids) != 2 {
		t.Fatalf("want a skills and a summary suggestion, got %v", rev["suggestions"])
	}

	// Pending suggestions block building.
	rec := call(t, s, "POST", "/v1/tailored-resumes", `{"revision_id":`+id+`}`)
	if body := decodeError(t, rec); rec.Code != 409 || body.Error.Details["pending"] != float64(2) {
		t.Errorf("build with pending = %d %+v", rec.Code, body.Error)
	}

	// The summary suggestion comes first (targets are listed summary, experience, skills).
	base := "/v1/revisions/" + id + "/suggestions/"
	expect(t, call(t, s, "POST", base+ids[0]+"/edit", `{"edited_text":"My own summary."}`), 200)
	if sg := expect(t, call(t, s, "POST", base+ids[1]+"/accept", ""), 200); sg["state"] != "accepted" {
		t.Errorf("accept = %v", sg)
	}

	tailored := expect(t, call(t, s, "POST", "/v1/tailored-resumes", `{"revision_id":`+id+`}`), 201)
	content := tailored["content"].(string)
	if !strings.Contains(content, "My own summary.") || !strings.Contains(content, "sql, excel") {
		t.Errorf("content must hold the edit and the accepted proposal:\n%s", content)
	}
	if strings.Contains(content, "Analyst.\n") || !strings.Contains(tailored["base_content"].(string), "excel, sql") {
		t.Errorf("the original summary is replaced, and the base keeps the old text:\n%s", content)
	}
	if w := tailored["warnings"].([]any); len(w) != 0 {
		t.Errorf("warnings = %v, want none", w)
	}

	tid := fmt.Sprint(tailored["id"])
	hunks := expect(t, call(t, s, "GET", "/v1/tailored-resumes/"+tid+"/diff", ""), 200)["hunks"].([]any)
	changed := map[string]string{}
	for _, h := range hunks {
		hm := h.(map[string]any)
		changed[hm["section"].(string)] = hm["status"].(string)
	}
	if changed["skills"] != "changed" || changed["summary"] != "changed" {
		t.Errorf("hunk statuses = %v", changed)
	}

	list := expect2(t, call(t, s, "GET", "/v1/tailored-resumes?job_id="+jobID, ""))
	if len(list) != 1 || list[0]["content"] != nil {
		t.Errorf("a list leaves out the text: %v", list)
	}
	if call(t, s, "DELETE", "/v1/tailored-resumes/"+tid, "").Code != 204 {
		t.Error("delete should return 204")
	}
	wantError(t, call(t, s, "GET", "/v1/tailored-resumes/"+tid, ""), 404, "not_found")
}

func TestRejectedSuggestionIsNotApplied(t *testing.T) {
	s := newRoundServer(t, ai.Engines{Rules: ai.Rules{}})
	id := startRound(t, s, seedRound(t, s), "auto")
	rev := waitStatus(t, s, id, "done")
	for _, sid := range suggestionIDs(t, rev) {
		expect(t, call(t, s, "POST", "/v1/revisions/"+id+"/suggestions/"+sid+"/reject", ""), 200)
	}
	tailored := expect(t, call(t, s, "POST", "/v1/tailored-resumes", `{"revision_id":`+id+`}`), 201)
	if tailored["content"] != tailored["base_content"] {
		t.Errorf("rejecting everything must leave the resume as it was:\n%v", tailored["content"])
	}
}

func TestTailoredWarnsWhenBaseLacksTheText(t *testing.T) {
	s := newRoundServer(t, ai.Engines{Rules: ai.Rules{}})
	jobID := seedRound(t, s)
	// A saved resume the user has edited by hand: it no longer holds the master's summary.
	saved := expect(t, call(t, s, "POST", "/v1/resumes",
		`{"version_label":"Hand edited","content":"# Ada\n\n## Skills\n\nexcel, sql\n"}`), 201)
	body := fmt.Sprintf(`{"job_id":%s,"base_resume_id":%v}`, jobID, saved["id"])
	id := fmt.Sprint(expect(t, call(t, s, "POST", "/v1/revisions", body), http.StatusAccepted)["id"])
	for _, sid := range suggestionIDs(t, waitStatus(t, s, id, "done")) {
		expect(t, call(t, s, "POST", "/v1/revisions/"+id+"/suggestions/"+sid+"/accept", ""), 200)
	}

	tailored := expect(t, call(t, s, "POST", "/v1/tailored-resumes", `{"revision_id":`+id+`}`), 201)
	warnings := tailored["warnings"].([]any)
	if len(warnings) != 1 || !strings.Contains(warnings[0].(string), "Analyst.") {
		t.Errorf("warnings = %v, want the missing summary named", warnings)
	}
	if !strings.Contains(tailored["content"].(string), "sql, excel") {
		t.Errorf("changes that do apply are kept:\n%v", tailored["content"])
	}
}

func TestRevisionCreateErrors(t *testing.T) {
	s := newRoundServer(t, ai.Engines{Rules: ai.Rules{}})
	wantError(t, call(t, s, "POST", "/v1/revisions", `{"job_id":1}`), 404, "not_found")

	job := expect(t, call(t, s, "POST", "/v1/jobs", `{"raw_text":"Data Analyst at Acme"}`), 201)
	jobID := fmt.Sprint(job["id"])
	wantError(t, call(t, s, "POST", "/v1/revisions", `{"job_id":`+jobID+`}`), 409, "conflict") // no profile

	seedRound(t, s)
	expectField(t, call(t, s, "POST", "/v1/revisions", `{}`), "job_id")
	expectField(t, call(t, s, "POST", "/v1/revisions", `{"job_id":`+jobID+`,"engine":"fast"}`), "engine")
	expectField(t, call(t, s, "POST", "/v1/revisions", `{"job_id":`+jobID+`,"base_resume_id":99}`), "base_resume_id")
	wantError(t, call(t, s, "POST", "/v1/revisions", `{"job_id":`+jobID+`,"engine":"local"}`), 503, "ai_unavailable")
	wantError(t, call(t, s, "POST", "/v1/revisions", `{"job_id":`+jobID+`,"engine":"remote"}`), 503, "ai_unavailable")
}

func TestRevisionRoutesRejectBadIDs(t *testing.T) {
	s := newRoundServer(t, ai.Engines{})
	wantError(t, call(t, s, "GET", "/v1/revisions/99", ""), 404, "not_found")
	wantError(t, call(t, s, "GET", "/v1/revisions/abc", ""), 404, "not_found")
	wantError(t, call(t, s, "POST", "/v1/revisions/99/cancel", ""), 404, "not_found")
	wantError(t, call(t, s, "DELETE", "/v1/revisions/99", ""), 404, "not_found")
	wantError(t, call(t, s, "POST", "/v1/tailored-resumes", `{"revision_id":99}`), 404, "not_found")
	expectField(t, call(t, s, "GET", "/v1/revisions", ""), "job_id")
	expectField(t, call(t, s, "GET", "/v1/tailored-resumes", ""), "job_id")
}

func TestSuggestionDecisionRules(t *testing.T) {
	s := newRoundServer(t, ai.Engines{Rules: ai.Rules{}})
	id := startRound(t, s, seedRound(t, s), "rules")
	rev := waitStatus(t, s, id, "done")
	ids := suggestionIDs(t, rev)
	base := "/v1/revisions/" + id + "/suggestions/"

	expectField(t, call(t, s, "POST", base+ids[0]+"/edit", `{"edited_text":"  "}`), "edited_text")
	expectField(t, call(t, s, "POST", base+ids[0]+"/edit", `{}`), "edited_text")
	wantError(t, call(t, s, "POST", base+"999/accept", ""), 404, "not_found")
	wantError(t, call(t, s, "POST", base+"abc/accept", ""), 404, "not_found")

	// Deleting the revision removes its suggestions and leaves any tailored resume.
	if call(t, s, "DELETE", "/v1/revisions/"+id, "").Code != 204 {
		t.Error("delete should return 204")
	}
	wantError(t, call(t, s, "POST", base+ids[0]+"/accept", ""), 404, "not_found")
}

func TestDecisionsWaitForTheRound(t *testing.T) {
	b := newBlocker()
	s := newRoundServer(t, ai.Engines{Local: b, Rules: ai.Rules{}})
	id := startRound(t, s, seedRound(t, s), "local")
	<-b.started

	wantError(t, call(t, s, "POST", "/v1/revisions/"+id+"/suggestions/1/accept", ""), 409, "conflict")
	wantError(t, call(t, s, "POST", "/v1/tailored-resumes", `{"revision_id":`+id+`}`), 409, "conflict")
	expect(t, call(t, s, "POST", "/v1/revisions/"+id+"/cancel", ""), 200) // lets the blocked round end
}

func TestCancelRunningRound(t *testing.T) {
	b := newBlocker()
	s := newRoundServer(t, ai.Engines{Local: b, Rules: ai.Rules{}})
	id := startRound(t, s, seedRound(t, s), "local")
	<-b.started
	waitStatus(t, s, id, "running")

	out := expect(t, call(t, s, "POST", "/v1/revisions/"+id+"/cancel", ""), 200)
	if out["status"] != "canceled" {
		t.Errorf("cancel = %v", out)
	}
	// The stopped round must not overwrite the status with a failure.
	time.Sleep(50 * time.Millisecond)
	if rev := expect(t, call(t, s, "GET", "/v1/revisions/"+id, ""), 200); rev["status"] != "canceled" || rev["error"] != nil {
		t.Errorf("revision = %v, want canceled with no error", rev)
	}
	wantError(t, call(t, s, "POST", "/v1/revisions/"+id+"/cancel", ""), 409, "conflict")
}

func TestSecondRoundWaitsItsTurn(t *testing.T) {
	b := newBlocker()
	s := newRoundServer(t, ai.Engines{Local: b, Rules: ai.Rules{}})
	jobID := seedRound(t, s)
	first := startRound(t, s, jobID, "local")
	<-b.started
	second := startRound(t, s, jobID, "rules")

	time.Sleep(50 * time.Millisecond)
	if rev := expect(t, call(t, s, "GET", "/v1/revisions/"+second, ""), 200); rev["status"] != "queued" {
		t.Errorf("second round = %v, want queued while the first runs", rev["status"])
	}
	expect(t, call(t, s, "POST", "/v1/revisions/"+first+"/cancel", ""), 200)
	waitStatus(t, s, second, "done")
}

func TestCancelQueuedRoundNeverRuns(t *testing.T) {
	b := newBlocker()
	s := newRoundServer(t, ai.Engines{Local: b, Rules: ai.Rules{}})
	jobID := seedRound(t, s)
	first := startRound(t, s, jobID, "local")
	<-b.started
	second := startRound(t, s, jobID, "rules")

	expect(t, call(t, s, "POST", "/v1/revisions/"+second+"/cancel", ""), 200)
	expect(t, call(t, s, "POST", "/v1/revisions/"+first+"/cancel", ""), 200)
	time.Sleep(50 * time.Millisecond)
	rev := expect(t, call(t, s, "GET", "/v1/revisions/"+second, ""), 200)
	if rev["status"] != "canceled" || len(rev["suggestions"].([]any)) != 0 {
		t.Errorf("a canceled queued round stays canceled and empty: %v", rev)
	}
}

func TestFailedRoundRecordsTheError(t *testing.T) {
	s := newRoundServer(t, ai.Engines{Local: failing{}, Rules: ai.Rules{}})
	id := startRound(t, s, seedRound(t, s), "local")
	rev := waitStatus(t, s, id, "failed")
	if msg, _ := rev["error"].(string); !strings.Contains(msg, "model crashed") {
		t.Errorf("error = %v", rev["error"])
	}
	if sg := rev["suggestions"].([]any); sg == nil || len(sg) != 0 {
		t.Errorf("suggestions must be [] and not null: %v", rev["suggestions"])
	}
}

func TestAutoFallsBackToRules(t *testing.T) {
	s := newRoundServer(t, ai.Engines{Local: failing{}, Rules: ai.Rules{}})
	id := startRound(t, s, seedRound(t, s), "auto")
	rev := waitStatus(t, s, id, "done")
	if rev["engine"] != "rules" || len(suggestionIDs(t, rev)) == 0 {
		t.Errorf("revision = %v, want rules suggestions", rev)
	}
}

func TestRoundWithNothingToSuggest(t *testing.T) {
	s := newRoundServer(t, ai.Engines{Rules: ai.Rules{}})
	expect(t, call(t, s, "PUT", "/v1/resume/profile", `{"full_name":"Ada"}`), 200)
	job := expect(t, call(t, s, "POST", "/v1/jobs", `{"raw_text":"Data Analyst at Acme"}`), 201)
	id := startRound(t, s, fmt.Sprint(job["id"]), "auto")
	rev := waitStatus(t, s, id, "done")
	if sg := rev["suggestions"].([]any); sg == nil || len(sg) != 0 {
		t.Errorf("a round with nothing to suggest finishes done with []: %v", rev["suggestions"])
	}
	// It can still be built; the result equals the base.
	tailored := expect(t, call(t, s, "POST", "/v1/tailored-resumes", `{"revision_id":`+id+`}`), 201)
	if tailored["content"] != tailored["base_content"] {
		t.Error("content should equal the base")
	}
}

func TestRevisionList(t *testing.T) {
	s := newRoundServer(t, ai.Engines{Rules: ai.Rules{}})
	jobID := seedRound(t, s)
	first := startRound(t, s, jobID, "rules")
	waitStatus(t, s, first, "done")
	second := startRound(t, s, jobID, "rules")
	waitStatus(t, s, second, "done")

	list := expect2(t, call(t, s, "GET", "/v1/revisions?job_id="+jobID, ""))
	if len(list) != 2 || fmt.Sprint(list[0]["id"]) != second || list[0]["suggestions"] != nil {
		t.Errorf("list = %v, want newest first and no suggestions", list)
	}
}
