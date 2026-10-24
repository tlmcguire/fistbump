package greenhouse

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
)

type fakeJob struct {
	id       int
	title    string
	location string
	updated  string
}

// searchServer serves listings and details for boards. A board mapped to nil is a 404; a board in
// broken answers 500; a job id in badDetail answers 500 for its detail.
type searchServer struct {
	boards    map[string][]fakeJob
	broken    map[string]bool
	badDetail map[string]bool
	pay       map[string]string // board/id -> text added to the description
	listHits  atomic.Int32
	slow      chan struct{}
}

func (s *searchServer) client(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/") // v1 boards {board} jobs [id]
		board := parts[2]
		if s.slow != nil {
			select {
			case <-s.slow:
			case <-r.Context().Done():
				return
			}
		}
		if s.broken[board] {
			http.Error(w, "boom", 500)
			return
		}
		jobs, ok := s.boards[board]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if len(parts) == 4 {
			s.listHits.Add(1)
			var items []string
			for _, j := range jobs {
				items = append(items, fmt.Sprintf(`{"id":%d,"title":%q,"location":{"name":%q},"updated_at":%q,"absolute_url":"https://job-boards.greenhouse.io/%s/jobs/%d"}`, j.id, j.title, j.location, j.updated, board, j.id))
			}
			fmt.Fprintf(w, `{"jobs":[%s]}`, strings.Join(items, ","))
			return
		}
		id := parts[4]
		if s.badDetail[board+"/"+id] {
			http.Error(w, "boom", 500)
			return
		}
		for _, j := range jobs {
			if fmt.Sprint(j.id) == id {
				fmt.Fprintf(w, `{"id":%d,"title":%q,"location":{"name":%q},"updated_at":%q,"content":"&lt;p&gt;About %s. %s&lt;/p&gt;","absolute_url":"https://job-boards.greenhouse.io/%s/jobs/%d"}`, j.id, j.title, j.location, j.updated, j.title, s.pay[board+"/"+id], board, j.id)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, HTTP: srv.Client(), Cache: NewCache(nil)}
}

func query(keywords ...string) connectors.Query {
	return connectors.Query{Keywords: keywords, WorkMode: "Any", Limit: 20}
}

func titles(r connectors.SearchResult) []string {
	var out []string
	for _, p := range r.Postings {
		out = append(out, p.Board+":"+p.PositionTitle)
	}
	return out
}

func TestSearchRanksAndFills(t *testing.T) {
	s := &searchServer{boards: map[string][]fakeJob{
		"acme": {
			{1, "Data Analyst", "Remote - US", "2026-10-01T00:00:00Z"},
			{2, "Senior Data Analyst", "Austin, TX", "2026-10-02T00:00:00Z"},
			{3, "Data Scientist", "Austin, TX", "2026-10-03T00:00:00Z"},
			{4, "Chef", "Austin, TX", "2026-10-03T00:00:00Z"},
		},
		"zeta": {{5, "Data Analyst", "Remote - US", "2026-10-05T00:00:00Z"}},
	}}
	c := s.client(t)
	r, err := c.Search(context.Background(), query("data analyst"), []string{"acme", "zeta"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"zeta:Data Analyst", "acme:Data Analyst", "acme:Senior Data Analyst", "acme:Data Scientist"}
	if got := titles(r); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("order = %v, want %v", got, want)
	}
	if r.Matched != 4 || r.PostingsScanned != 5 || r.BoardsSearched != 2 || r.BoardsTotal != 2 {
		t.Errorf("counts = %+v", r)
	}
	for _, p := range r.Postings {
		if p.RawText == "" || p.Score == 0 || p.WorkMode == "" {
			t.Errorf("posting not filled: %+v", p)
		}
	}
	if r.Postings[0].RawText != "About Data Analyst." {
		t.Errorf("raw_text = %q", r.Postings[0].RawText)
	}
	// Stable: the same input gives the same order.
	r2, _ := c.Search(context.Background(), query("data analyst"), []string{"zeta", "acme"})
	if strings.Join(titles(r2), "|") != strings.Join(want, "|") {
		t.Errorf("order changed with board order: %v", titles(r2))
	}
}

func TestSearchFilters(t *testing.T) {
	s := &searchServer{boards: map[string][]fakeJob{"acme": {
		{1, "Data Analyst 1", "Remote - US", "2026-10-01T00:00:00Z"},
		{2, "Data Analyst 2", "Charleston, SC", "2026-10-01T00:00:00Z"},
		{3, "Data Analyst 3", "Denver, CO", "2026-10-01T00:00:00Z"},
		{4, "Data Analyst 4", "Remote - Australia", "2026-10-01T00:00:00Z"},
	}}}
	c := s.client(t)
	q := query("data analyst")
	q.Location, q.WorkMode = "Charleston, SC", "On-site"
	r, _ := c.Search(context.Background(), q, []string{"acme"})
	if len(r.Postings) != 1 || r.Postings[0].ExternalID != "2" {
		t.Errorf("on-site Charleston = %v", titles(r))
	}
	q.WorkMode = "Remote"
	r, _ = c.Search(context.Background(), q, []string{"acme"})
	if len(r.Postings) != 1 || r.Postings[0].ExternalID != "1" {
		t.Errorf("remote = %v", r.Postings)
	}
	q.WorkMode = "Any"
	r, _ = c.Search(context.Background(), q, []string{"acme"})
	if len(r.Postings) != 2 || r.Matched != 2 || r.LocationMatched != 1 || r.Postings[0].ExternalID != "2" {
		t.Errorf("any = %v, matched %d, location_matched %d", titles(r), r.Matched, r.LocationMatched)
	}
}

func TestSearchCompanyVarietyAndPaging(t *testing.T) {
	var many []fakeJob
	for i := 1; i <= 6; i++ {
		many = append(many, fakeJob{i, fmt.Sprintf("Data Analyst %d", i), "Remote", fmt.Sprintf("2026-10-0%dT00:00:00Z", i)})
	}
	s := &searchServer{boards: map[string][]fakeJob{
		"big":   many,
		"small": {{10, "Data Analyst 10", "Remote", "2026-09-01T00:00:00Z"}},
	}}
	c := s.client(t)
	boards := []string{"big", "small"}

	// A page of 4 holds big's first 3 and small's one, ahead of big's later postings.
	q := query("data analyst")
	q.Limit = 4
	r, _ := c.Search(context.Background(), q, boards)
	if len(r.Postings) != 4 || r.Matched != 7 || !r.HasMore || r.NextOffset != 4 {
		t.Fatalf("page 1: %v, matched %d, more %v, next %d", titles(r), r.Matched, r.HasMore, r.NextOffset)
	}
	seen := map[string]bool{}
	big := 0
	for _, p := range r.Postings {
		seen[p.Board+p.ExternalID] = true
		if p.Board == "big" {
			big++
		}
	}
	if big != 3 {
		t.Errorf("page 1 has %d from big, want 3", big)
	}

	// The next pages continue from NextOffset and never repeat a posting.
	for offset, pages := r.NextOffset, 0; ; pages++ {
		if pages > 5 {
			t.Fatal("paging did not finish")
		}
		q.Offset = offset
		r, _ = c.Search(context.Background(), q, boards)
		for _, p := range r.Postings {
			if seen[p.Board+p.ExternalID] {
				t.Errorf("%s%s repeated", p.Board, p.ExternalID)
			}
			seen[p.Board+p.ExternalID] = true
		}
		if !r.HasMore {
			break
		}
		offset = r.NextOffset
	}
	if len(seen) != 7 {
		t.Errorf("paging reached %d postings, want 7", len(seen))
	}

	// An offset past the end is an empty last page.
	q.Offset = 500
	r, _ = c.Search(context.Background(), q, boards)
	if len(r.Postings) != 0 || r.HasMore {
		t.Errorf("past the end: %v, more %v", titles(r), r.HasMore)
	}
}

func TestSearchPartialFailure(t *testing.T) {
	s := &searchServer{
		boards: map[string][]fakeJob{
			"acme":   {{1, "Data Analyst", "Remote", "2026-10-01T00:00:00Z"}},
			"broken": {},
		},
		broken: map[string]bool{"broken": true},
	}
	c := s.client(t)
	r, err := c.Search(context.Background(), query("data analyst"), []string{"acme", "broken", "oldco"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Postings) != 1 || r.BoardsSearched != 1 || r.BoardsTotal != 3 {
		t.Errorf("result = %+v", r)
	}
	msgs := map[string]string{}
	for _, e := range r.Errors {
		msgs[e.Board] = e.Message
	}
	if msgs["oldco"] != "board not found" || msgs["broken"] != "Greenhouse did not respond" || len(msgs) != 2 {
		t.Errorf("errors = %v", msgs)
	}
}

func TestSearchEveryBoardFails(t *testing.T) {
	s := &searchServer{boards: map[string][]fakeJob{}}
	c := s.client(t)
	r, err := c.Search(context.Background(), query("data analyst"), []string{"a", "b"})
	if !errors.Is(err, ErrorUpstream) {
		t.Fatalf("err = %v", err)
	}
	if len(r.Errors) != 2 {
		t.Errorf("errors = %v", r.Errors)
	}
}

func TestSearchDetailFailureDropsPosting(t *testing.T) {
	s := &searchServer{
		boards: map[string][]fakeJob{"acme": {
			{1, "Data Analyst 1", "Remote", "2026-10-02T00:00:00Z"},
			{2, "Data Analyst 2", "Remote", "2026-10-01T00:00:00Z"},
		}},
		badDetail: map[string]bool{"acme/1": true},
	}
	c := s.client(t)
	r, err := c.Search(context.Background(), query("data analyst"), []string{"acme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Postings) != 1 || r.Postings[0].ExternalID != "2" || len(r.Errors) != 1 || r.Errors[0].Board != "acme" {
		t.Errorf("result = %+v", r)
	}
}

func TestSearchSecondRunMakesNoListingRequests(t *testing.T) {
	s := &searchServer{boards: map[string][]fakeJob{"acme": {{1, "Data Analyst", "Remote", "2026-10-01T00:00:00Z"}}}}
	c := s.client(t)
	c.Search(context.Background(), query("data analyst"), []string{"acme"})
	c.Search(context.Background(), query("data analyst"), []string{"acme"})
	if s.listHits.Load() != 1 {
		t.Errorf("listing requests = %d, want 1", s.listHits.Load())
	}
}

func TestSearchContextCancel(t *testing.T) {
	s := &searchServer{boards: map[string][]fakeJob{"acme": {{1, "Data Analyst", "Remote", "2026-10-01T00:00:00Z"}}}, slow: make(chan struct{})}
	c := s.client(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.Search(ctx, query("data analyst"), []string{"acme"})
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("search did not stop after cancel")
	}
}

func TestSearchWithoutKeywords(t *testing.T) {
	s := &searchServer{boards: map[string][]fakeJob{"acme": {
		{1, "Chef", "Charleston, SC", "2026-10-01T00:00:00Z"},
		{2, "Data Analyst", "Denver, CO", "2026-10-02T00:00:00Z"},
		{3, "Accountant", "Charleston, SC", "2026-10-03T00:00:00Z"},
	}}}
	c := s.client(t)
	q := connectors.Query{WorkMode: "Any", Limit: 20, Location: "Charleston, SC"}
	r, err := c.Search(context.Background(), q, []string{"acme"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(titles(r), "|"); got != "acme:Accountant|acme:Chef" {
		t.Errorf("location only = %v", got)
	}
	q.Location = ""
	r, _ = c.Search(context.Background(), q, []string{"acme"})
	if len(r.Postings) != 3 || r.Postings[0].PositionTitle != "Accountant" {
		t.Errorf("industry only = %v", titles(r))
	}
}

func TestSearchPreferences(t *testing.T) {
	now := time.Now().UTC()
	day := func(n int) string { return now.AddDate(0, 0, -n).Format(time.RFC3339) }
	s := &searchServer{boards: map[string][]fakeJob{"acme": {
		{1, "Data Analyst 1", "Remote", day(1)},
		{2, "Senior Data Analyst 2", "Remote", day(2)},
		{3, "Data Analyst Intern 3", "Remote", day(3)},
		{4, "Sales Data Analyst 4", "Remote", day(4)},
		{5, "Data Analyst 5", "Remote", day(60)},
	}}}
	c := s.client(t)
	run := func(mod func(*connectors.Query)) string {
		q := query("data analyst")
		mod(&q)
		r, err := c.Search(context.Background(), q, []string{"acme"})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, p := range r.Postings {
			ids = append(ids, p.ExternalID)
		}
		return strings.Join(ids, ",")
	}
	if got := run(func(q *connectors.Query) { q.Level = "Senior" }); got != "2" {
		t.Errorf("senior = %q", got)
	}
	if got := run(func(q *connectors.Query) { q.Level = "Intern" }); got != "3" {
		t.Errorf("intern = %q", got)
	}
	if got := run(func(q *connectors.Query) { q.Exclude = []string{"sales", "intern"} }); got != "1,2,5" {
		t.Errorf("exclude = %q", got)
	}
	if got := run(func(q *connectors.Query) { q.PostedWithin = 7 }); got != "1,2,3,4" {
		t.Errorf("posted within 7 days = %q", got)
	}
}

func TestSearchPayRange(t *testing.T) {
	now := time.Now().UTC()
	var jobs []fakeJob
	pay := map[string]string{}
	for i := 1; i <= 10; i++ {
		jobs = append(jobs, fakeJob{i, fmt.Sprintf("Data Analyst %d", i), "Remote", now.AddDate(0, 0, -i).Format(time.RFC3339)})
	}
	pay["acme/1"] = "$80,000 - $90,000"
	pay["acme/2"] = "$130,000 - $150,000"
	pay["acme/4"] = "$120,000 - $160,000"
	pay["acme/6"] = "$200,000 - $220,000"
	s := &searchServer{boards: map[string][]fakeJob{"acme": jobs}, pay: pay}
	c := s.client(t)

	q := query("data analyst")
	q.PayMin, q.PayMax = 125000, 180000
	r, err := c.Search(context.Background(), q, []string{"acme"})
	if err != nil {
		t.Fatal(err)
	}
	// 2 and 4 fit; 3, 5 and 7 to 10 list no pay and are kept; 1 and 6 are out of range.
	if r.Matched != 8 || len(r.Postings) != 8 || r.Partial {
		t.Errorf("matched %d, postings %d, partial %v", r.Matched, len(r.Postings), r.Partial)
	}
	for _, p := range r.Postings {
		if p.ExternalID == "1" || p.ExternalID == "6" {
			t.Errorf("posting %s is outside the range", p.ExternalID)
		}
	}
	for _, p := range r.Postings {
		if p.ExternalID == "2" && (p.PayMin != 130000 || p.PayMax != 150000) {
			t.Errorf("pay on posting 2 = %d to %d", p.PayMin, p.PayMax)
		}
	}

	q.RequirePay = true
	r, _ = c.Search(context.Background(), q, []string{"acme"})
	if r.Matched != 2 {
		t.Errorf("require pay: matched %d, want 2", r.Matched)
	}

	// Paging walks the filtered list.
	q.RequirePay = false
	q.Limit = 3
	r, _ = c.Search(context.Background(), q, []string{"acme"})
	if len(r.Postings) != 3 || !r.HasMore || r.NextOffset != 3 {
		t.Errorf("page 1: %d postings, more %v, next %d", len(r.Postings), r.HasMore, r.NextOffset)
	}
	q.Offset = 3
	r, _ = c.Search(context.Background(), q, []string{"acme"})
	if len(r.Postings) != 3 || !r.HasMore {
		t.Errorf("page 2: %d postings, more %v", len(r.Postings), r.HasMore)
	}
}

func TestSearchFoldsDuplicatesAndSorts(t *testing.T) {
	s := &searchServer{boards: map[string][]fakeJob{"acme": {
		{1, "Data Analyst", "Austin, TX", "2026-10-01T00:00:00Z"},
		{2, "Data Analyst", "Denver, CO", "2026-10-02T00:00:00Z"},
		{3, "Data Analyst", "Remote", "2026-10-03T00:00:00Z"},
		{4, "Data Analyst, Finance", "Austin, TX", "2026-10-09T00:00:00Z"},
	}}}
	c := s.client(t)
	r, _ := c.Search(context.Background(), query("data analyst"), []string{"acme"})
	if r.Matched != 2 || len(r.Postings) != 2 {
		t.Fatalf("matched %d, postings %v", r.Matched, titles(r))
	}
	for _, p := range r.Postings {
		if p.PositionTitle == "Data Analyst" && p.Duplicates != 2 {
			t.Errorf("duplicates = %d, want 2", p.Duplicates)
		}
	}
	// Relevance puts the exact title first; newest puts the most recently updated first.
	if r.Postings[0].PositionTitle != "Data Analyst" {
		t.Errorf("relevance first = %q", r.Postings[0].PositionTitle)
	}
	q := query("data analyst")
	q.Sort = "newest"
	r, _ = c.Search(context.Background(), q, []string{"acme"})
	if r.Postings[0].PositionTitle != "Data Analyst, Finance" {
		t.Errorf("newest first = %q", r.Postings[0].PositionTitle)
	}
}

func TestSearchReportsExactMatches(t *testing.T) {
	s := &searchServer{boards: map[string][]fakeJob{
		"acme": {
			{1, "Senior Software Engineer", "Remote", "2026-10-03T00:00:00Z"},
			{2, "Software Engineer", "Remote", "2026-10-02T00:00:00Z"},
			{3, "Junior Software Engineer", "Remote", "2026-10-01T00:00:00Z"},
		},
		"zeta": {{4, "Staff Software Engineer", "Remote", "2026-10-05T00:00:00Z"}},
	}}
	c := s.client(t)
	boards := []string{"acme", "zeta"}

	// A level in the keywords puts that level first and counts it as the exact match.
	r, _ := c.Search(context.Background(), query("junior software engineer"), boards)
	if r.LevelWanted != "Entry" || r.ExactMatched != 1 || r.Matched != 4 || r.Postings[0].PositionTitle != "Junior Software Engineer" || !r.Postings[0].Exact {
		t.Errorf("junior: level %q, exact %d, matched %d, first %q", r.LevelWanted, r.ExactMatched, r.Matched, r.Postings[0].PositionTitle)
	}

	// With no posting of that level there is no exact match, and the others still show.
	delete(s.boards, "zeta")
	s.boards["acme"] = s.boards["acme"][:2]
	c.Cache.Clear() // the first search cached the old listing
	r, _ = c.Search(context.Background(), query("junior software engineer"), []string{"acme"})
	if r.ExactMatched != 0 || r.LevelWanted != "Entry" || len(r.Postings) != 2 {
		t.Errorf("no junior: exact %d, level %q, postings %d", r.ExactMatched, r.LevelWanted, len(r.Postings))
	}

	// Keywords with no level treat every posting of that title as exact, whatever its seniority.
	r, _ = c.Search(context.Background(), query("software engineer"), []string{"acme"})
	if r.LevelWanted != "" || r.ExactMatched != 2 {
		t.Errorf("plain: level %q, exact %d", r.LevelWanted, r.ExactMatched)
	}

	// A close but not exact title is counted as a close match only.
	r, _ = c.Search(context.Background(), query("software developer team"), []string{"acme"})
	if r.ExactMatched != 0 {
		t.Errorf("partial title counted as exact: %d", r.ExactMatched)
	}

	// Without keywords nothing is exact.
	q := connectors.Query{WorkMode: "Any", Limit: 20, Location: "Remote"}
	r, _ = c.Search(context.Background(), q, []string{"acme"})
	if r.ExactMatched != 0 {
		t.Errorf("no keywords: exact %d", r.ExactMatched)
	}
}
