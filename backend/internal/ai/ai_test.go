package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/analyze"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/resumeparse"
)

func analyzeMatch(s string) analyze.Match { return analyze.Match{Skill: s} }

func TestParseDrafts(t *testing.T) {
	targets := []Target{{Section: "summary"}, {Section: "experience", Original: "old"}}
	reply := "Sure!\n```json\n{\"suggestions\":[{\"index\":1,\"proposed_text\":\"new\"},{\"index\":1,\"proposed_text\":\"dup\"},{\"index\":0,\"proposed_text\":\"\"},{\"index\":9,\"proposed_text\":\"x\"}]}\n```"
	got, err := parseDrafts(reply, targets)
	if err != nil || len(got) != 1 || got[0].Proposed != "new" || got[0].Original != "old" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if _, err := parseDrafts("not json", targets); err == nil {
		t.Fatal("expected error")
	}
}

func TestRemoteChatAndKeyRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-secret" {
			http.Error(w, "bad key sk-secret", 401)
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
	}))
	defer srv.Close()
	r := NewRemote()
	r.Configure(srv.URL, "m", "sk-secret")
	if out, err := r.Test(context.Background()); err != nil || out != "pong" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	r.Configure(srv.URL, "m", "wrong")
	_, err := r.Test(context.Background())
	if err == nil || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateBaseURL(t *testing.T) {
	for url, ok := range map[string]bool{"https://api.x.com/v1": true, "http://localhost:11434/v1": true, "http://127.0.0.1:1/v1": true, "http://x.com/v1": false, "ftp://x": false, "nonsense": false} {
		if (ValidateBaseURL(url) == nil) != ok {
			t.Errorf("ValidateBaseURL(%q) ok=%v", url, !ok)
		}
	}
}

func TestRulesNeverInventSkills(t *testing.T) {
	// Rules proposals may only mention matched skills.
	// (Request construction lives in api tests; here we check the reorder path.)
	got, _ := Rules{}.Suggest(context.Background(), Request{Targets: []Target{{Section: "skills", Original: "sql, go, docker"}}})
	if len(got) != 0 {
		t.Fatalf("no matches should yield no skills reorder, got %+v", got)
	}
}

func TestDownloadResumeVerifyAndRename(t *testing.T) {
	payload := []byte(strings.Repeat("gguf-data-", 1000))
	sum := sha256.Sum256(payload)
	var sawRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rg := r.Header.Get("Range"); rg != "" {
			sawRange = rg
			var from int
			_, _ = fmtSscan(rg, &from)
			w.WriteHeader(http.StatusPartialContent)
			w.Write(payload[from:])
			return
		}
		w.Write(payload)
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := CatalogModel{ID: "t", Repo: "o/r", File: "t.gguf", Revision: "abc", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(payload))}
	_ = os.MkdirAll(ModelsDir(dir), 0o755)
	_ = os.WriteFile(ModelPath(dir, m)+".part", payload[:100], 0o644) // resume from 100 bytes
	d := NewDownloader(dir)
	d.baseURL = srv.URL
	id, err := d.Start(m)
	if err != nil {
		t.Fatal(err)
	}
	wait(t, d, id, "done")
	if sawRange != "bytes=100-" || !IsInstalled(dir, m) {
		t.Fatalf("range=%q installed=%v", sawRange, IsInstalled(dir, m))
	}

	// Bad checksum is deleted and fails.
	bad := m
	bad.File, bad.ID, bad.SHA256 = "bad.gguf", "bad", strings.Repeat("0", 64)
	id, _ = d.Start(bad)
	wait(t, d, id, "failed")
	if _, err := os.Stat(ModelPath(dir, bad) + ".part"); !os.IsNotExist(err) {
		t.Fatal("bad download kept")
	}
}

func fmtSscan(rg string, from *int) (int, error) {
	n := 0
	for _, c := range strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-") {
		n = n*10 + int(c-'0')
	}
	*from = n
	return 1, nil
}

func wait(t *testing.T, d *Downloader, id, state string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if st, _ := d.Status(id); st.State == state {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	st, _ := d.Status(id)
	t.Fatalf("download state = %+v, want %s", st, state)
}

func TestStructureResumeGroundsModelOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := `{"full_name":"Ann Lee","email":"ann@x.com","location":"Paris, France","experiences":[{"job_title":"Engineer","company_name":"Acme","location":"Austin, TX","start_date":"2020-01","end_date":"present","bullets":["Built Go services","Won a Nobel prize for physics in 1999"]}],"education":[{"institution":"State U","gpa":"3.7"}],"skills":["Go","Fortran"]}`
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": reply}}}})
		w.Write(b)
	}))
	defer srv.Close()
	rem := NewRemote()
	rem.Configure(srv.URL, "m", "")
	e := &Engines{Local: NewLocal(t.TempDir(), "", func() string { return "" }, func() time.Duration { return 0 }), Remote: rem}
	text := "Ann Lee\nann@x.com\nExperience\nEngineer, Acme, 2020 - Present\n- Built Go services\nEducation\nState U, GPA 3.7\nSkills: Go"
	d, engine, err := e.StructureResume(context.Background(), "auto", text, resumeparse.Parse(text))
	if err != nil || engine != "remote" {
		t.Fatalf("engine=%s err=%v", engine, err)
	}
	ex := d.Experiences[0]
	if d.Profile.HomeLocation != nil || ex.Location != nil || ex.EndDate != nil || *ex.StartDate != "2020-01" {
		t.Fatalf("ungrounded values kept: %+v %+v", d.Profile, ex)
	}
	if ex.Impact != nil || ex.Description == nil || strings.Contains(*ex.Description, "Nobel") {
		t.Fatalf("invented bullet kept: %v %v", ex.Description, ex.Impact)
	}
	if d.Education[0].GPA == nil || *d.Education[0].GPA != 3.7 || contains(d.Skills, "fortran") {
		t.Fatalf("edu=%+v skills=%v", d.Education[0], d.Skills)
	}
}

func TestStripEcho(t *testing.T) {
	sum := Target{Section: "summary", Label: "Professional summary"}
	exp := Target{Section: "experience", Label: "Dev at Acme (responsibilities)", Original: "Built it."}
	for in, want := range map[string]string{
		"Professional summary: Engineer.":  "Engineer.",
		"Professional summary - Engineer.": "Engineer.",
		"Engineer focused on payments.":    "Engineer focused on payments.",
	} {
		if got := stripEcho(in, sum); got != want {
			t.Errorf("stripEcho(%q) = %q", in, got)
		}
	}
	if got := stripEcho("Dev at Acme (responsibilities)\nBuilt it well.", exp); got != "Built it well." {
		t.Errorf("first-line echo: %q", got)
	}
}

func TestGroundedEnough(t *testing.T) {
	src := "built payment services in go and postgresql. mentored 3 engineers. kubernetes"
	if !groundedEnough("Built Go payment services; mentored engineers.", src) {
		t.Error("grounded text rejected")
	}
	if groundedEnough("Collaborated with cross-functional stakeholders to drive secure innovation.", src) {
		t.Error("invented text accepted")
	}
}

func TestRulesReorderBullets(t *testing.T) {
	id := int64(1)
	r := Request{Targets: []Target{{Section: "experience", TargetID: &id, Label: "x (responsibilities)", Original: "- Ran meetings\n- Built Go services"}}}
	r.Analysis.Required.Tech.Matched = append(r.Analysis.Required.Tech.Matched, analyzeMatch("go"))
	got, _ := Rules{}.Suggest(context.Background(), r)
	if len(got) != 1 || got[0].Proposed != "- Built Go services\n- Ran meetings" {
		t.Fatalf("got %+v", got)
	}
}

func TestSkillGuardRejectsInventedSkills(t *testing.T) {
	id := int64(1)
	targets := []Target{
		{Section: "experience", TargetID: &id, Label: "Dev at Acme (responsibilities)", Original: "Built payment services in Go and PostgreSQL."},
		{Section: "skills", Label: "Skills", Original: "go, postgresql, docker"},
		{Section: "summary", Label: "Professional summary"},
	}
	req := Request{Targets: targets}
	req.Job.PositionTitle = "Backend Engineer"
	req.Job.ReqTech = []string{"go", "kubernetes", "terraform"} // the job wants skills the user lacks
	req.Analysis.Required.Tech.Matched = append(req.Analysis.Required.Tech.Matched, analyzeMatch("go"))
	reply := `{"suggestions":[
	 {"index":0,"proposed_text":"Built payment services in Go and PostgreSQL, deployed on Kubernetes."},
	 {"index":1,"proposed_text":"go, kubernetes, postgresql, docker"},
	 {"index":2,"proposed_text":"Backend engineer building payment services in Go and Terraform."}]}`
	got, err := parseDraftsGrounded(reply, targets, groundingSource(req), requestSkills(req))
	if err != nil || len(got) != 0 {
		t.Fatalf("invented skills got through: %+v %v", got, err)
	}
	ok := `{"suggestions":[
	 {"index":0,"proposed_text":"Built Go and PostgreSQL payment services."},
	 {"index":1,"proposed_text":"go, docker, postgresql"},
	 {"index":2,"proposed_text":"Engineer building payment services in Go and PostgreSQL."}]}`
	got, _ = parseDraftsGrounded(ok, targets, groundingSource(req), requestSkills(req))
	if len(got) != 3 {
		t.Fatalf("grounded proposals dropped: %+v", got)
	}
}

func TestSameItems(t *testing.T) {
	if !sameItems("go, SQL, docker", "docker,go , sql") || sameItems("go, sql", "go, sql, rust") || sameItems("go, sql", "go") {
		t.Fatal("sameItems wrong")
	}
}

func TestRulesNeverAddSkills(t *testing.T) {
	// Every rules draft must pass the same guard as model output.
	id := int64(1)
	r := Request{Targets: []Target{
		{Section: "summary", Label: "Professional summary"},
		{Section: "experience", TargetID: &id, Label: "x (responsibilities)", Original: "- Ran meetings\n- Built Go services"},
		{Section: "skills", Label: "Skills", Original: "sql, go"},
	}}
	r.Job.PositionTitle, r.Job.ReqTech = "Engineer", []string{"go", "kubernetes"}
	r.Analysis.Required.Tech.Matched = append(r.Analysis.Required.Tech.Matched, analyzeMatch("go"))
	r.Analysis.Required.Tech.Missing = []string{"kubernetes"}
	drafts, _ := Rules{}.Suggest(context.Background(), r)
	have := requestSkills(r)
	for _, d := range drafts {
		for _, tg := range r.Targets {
			if tg.Section == d.Section && tg.Original == d.Original {
				if reason := checkDraft(tg, d.Proposed, have); reason != "" {
					t.Errorf("rules draft %q failed guard: %s", d.Proposed, reason)
				}
			}
		}
	}
}

func TestRulesSummaryIgnoresActivitiesForYears(t *testing.T) {
	s := func(v string) *string { return &v }
	m := models.Master{Experiences: []models.Experience{
		{JobTitle: "Software Engineering Intern", StartDate: s("2024-03"), Skills: []string{"go"}},
		{JobTitle: "Cybersecurity Club", StartDate: s("2015-08")},
	}}
	got := RulesSummary(m)
	if strings.Contains(got, fmt.Sprint(time.Now().Year()-2015)) {
		t.Fatalf("club inflated years: %q", got)
	}
}
