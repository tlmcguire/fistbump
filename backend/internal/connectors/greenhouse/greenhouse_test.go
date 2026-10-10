package greenhouse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
)

func TestBoardsFile(t *testing.T) {
	cats := Categories()
	if len(cats) != 32 {
		t.Errorf("categories = %d, want 32", len(cats))
	}
	ids := map[string]Category{}
	for _, c := range cats {
		if ids[c.ID].ID != "" {
			t.Errorf("duplicate category %s", c.ID)
		}
		ids[c.ID] = c
	}
	tok := regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	for _, c := range cats {
		if c.Parent != nil {
			p, ok := ids[*c.Parent]
			if !ok || p.Parent != nil {
				t.Errorf("category %s has bad parent %s", c.ID, *c.Parent)
			}
		}
		seen := map[string]bool{}
		for _, b := range c.Boards {
			if !tok.MatchString(b.Token) {
				t.Errorf("%s: bad token %q", c.ID, b.Token)
			}
			if seen[b.Token] {
				t.Errorf("%s: duplicate token %q", c.ID, b.Token)
			}
			seen[b.Token] = true
		}
	}
	if n := len(Tokens()); n != 517 {
		t.Errorf("unique boards = %d, want 517", n)
	}
}

func TestParentIncludesSubsets(t *testing.T) {
	tech := len(BoardsFor([]string{"technical"}))
	ai := len(BoardsFor([]string{"ai_ml"}))
	if ai == 0 || tech <= ai {
		t.Fatalf("technical=%d ai_ml=%d", tech, ai)
	}
}

const jobsJSON = `{"jobs":[
 {"id":1,"title":"Senior Backend Developer","absolute_url":"https://x/1","location":{"name":"Remote - US"},"company_name":"Acme Inc","updated_at":"2026-09-01"},
 {"id":2,"title":"Account Executive","absolute_url":"https://x/2","location":{"name":"NYC"},"updated_at":"2026-09-02"},
 {"id":3,"title":"Backend Engineer","absolute_url":"https://x/3","location":{"name":"Austin, TX"},"updated_at":"2026-09-03"},
 {"id":4,"title":"Frontend Engineer","absolute_url":"https://x/4","location":{"name":"Austin, TX (On-site)"},"updated_at":"2026-09-04"}
]}`

type fake struct {
	srv   *httptest.Server
	lists int
}

func newFake(t *testing.T) *fake {
	f := &fake{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/boards/acme/jobs":
			f.lists++
			w.Write([]byte(jobsJSON))
		case strings.HasPrefix(r.URL.Path, "/v1/boards/acme/jobs/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/boards/acme/jobs/")
			body := "Requirements: Go and SQL"
			if id == "3" {
				body = "Requirements: Java and Spring"
			}
			w.Write([]byte(`{"id":` + id + `,"content":"&lt;p&gt;` + body + `&lt;/p&gt;"}`))
		case r.URL.Path == "/v1/boards/acme":
			w.Write([]byte(`{"name":"Acme Inc"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func TestSearchMatchesTitlesRanksBySkillsAndReportsErrors(t *testing.T) {
	f := newFake(t)
	c := New(t.TempDir())
	c.Base = f.srv.URL
	res, err := c.FetchPostings(context.Background(), connectors.Query{
		Boards: []string{"acme", "gone"}, Keywords: []string{"Backend Engineer"}, ResumeSkills: []string{"golang", "sql"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.BoardsSearched != 1 || len(res.Errors) != 1 || res.Errors[0].Board != "gone" || res.PostingsScanned != 4 {
		t.Fatalf("searched=%d scanned=%d errors=%+v", res.BoardsSearched, res.PostingsScanned, res.Errors)
	}
	// "Senior Backend Developer" matches "Backend Engineer" through synonyms and ranks first on skills.
	if len(res.Postings) != 2 || res.Postings[0].ExternalID != "1" || res.Postings[1].ExternalID != "3" {
		t.Fatalf("postings = %+v", res.Postings)
	}
	if p := res.Postings[0]; p.CompanyName != "Acme Inc" || !strings.Contains(p.RawText, "Go and SQL") || p.SkillScore == 0 {
		t.Fatalf("posting = %+v", p)
	}
}

func TestSearchFiltersAndCache(t *testing.T) {
	f := newFake(t)
	dir := t.TempDir()
	c := New(dir)
	c.Base = f.srv.URL
	q := connectors.Query{Boards: []string{"acme"}, Keywords: []string{"engineer"}, Location: "Austin", WorkMode: "Remote"}
	res, _ := c.FetchPostings(context.Background(), q)
	ids := []string{}
	for _, p := range res.Postings {
		ids = append(ids, p.ExternalID)
	}
	// Remote must be stated: only the remote posting passes.
	if strings.Join(ids, ",") != "1" {
		t.Fatalf("ids = %v", ids)
	}
	q.WorkMode = ""
	res, _ = c.FetchPostings(context.Background(), q)
	if len(res.Postings) != 3 { // Austin x2 plus the remote posting
		t.Fatalf("location filter = %+v", res.Postings)
	}
	q.WorkMode = "Remote"
	c.FetchPostings(context.Background(), q)
	if f.lists != 1 {
		t.Fatalf("listing fetched %d times, want 1 (memory cache)", f.lists)
	}
	c2 := New(dir)
	c2.Base = f.srv.URL
	c2.FetchPostings(context.Background(), q)
	if f.lists != 1 {
		t.Fatalf("listing fetched %d times, want 1 (disk cache)", f.lists)
	}
	p, err := c2.Lookup(context.Background(), "1")
	if err != nil || !strings.Contains(p.RawText, "Go and SQL") {
		t.Fatalf("lookup = %+v %v", p, err)
	}
	if _, err := c2.Lookup(context.Background(), "999"); err == nil {
		t.Fatal("unknown id should fail")
	}
}

func TestTitleScore(t *testing.T) {
	kw := compileKeywords([]string{"Sr. Front-End Developer"})
	for title, want := range map[string]bool{"Frontend Engineer": true, "Senior Front End Engineer, Payments": true, "Backend Engineer": false, "Product Designer": false} {
		if got := titleScore(title, kw) >= minTitleScore; got != want {
			t.Errorf("%q: match=%v want %v (score %v)", title, got, want, titleScore(title, kw))
		}
	}
}

func TestValidateBoard(t *testing.T) {
	f := newFake(t)
	c := New("")
	c.Base = f.srv.URL
	name, open, err := c.ValidateBoard(context.Background(), "acme")
	if err != nil || name != "Acme Inc" || open != 4 {
		t.Fatalf("got %q %d %v", name, open, err)
	}
	if _, _, err := c.ValidateBoard(context.Background(), "nope"); err != ErrNoBoard {
		t.Fatalf("err = %v", err)
	}
}

func TestHomeBoost(t *testing.T) {
	h := newHome("Austin, TX")
	if homeBoost("Austin, TX", h) <= homeBoost("Denver, CO", h) || homeBoost("Denver, CO", h) <= 0 || homeBoost("Prague, Czech Republic", h) >= 0 || homeBoost("Remote", h) != 0 {
		t.Fatal("unexpected ordering")
	}
}

func TestExactPhraseRanksHigher(t *testing.T) {
	kw := compileKeywords([]string{"product manager"})
	if titleScore("Senior Product Manager", kw) <= titleScore("Product Marketing Manager", kw) {
		t.Fatal("exact phrase should outrank scattered words")
	}
}

func TestDiversify(t *testing.T) {
	var ps []connectors.Posting
	for _, c := range []string{"A", "A", "A", "A", "B", "A", "C"} {
		ps = append(ps, connectors.Posting{CompanyName: c})
	}
	got := ""
	for _, p := range diversify(ps, 2) {
		got += p.CompanyName
	}
	if got != "AABCAAA" {
		t.Fatalf("got %s", got)
	}
}
