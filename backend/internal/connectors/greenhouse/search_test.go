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
				fmt.Fprintf(w, `{"id":%d,"title":%q,"location":{"name":%q},"updated_at":%q,"content":"&lt;p&gt;About %s&lt;/p&gt;","absolute_url":"https://job-boards.greenhouse.io/%s/jobs/%d"}`, j.id, j.title, j.location, j.updated, j.title, board, j.id)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, HTTP: srv.Client(), Cache: NewCache("")}
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
	if r.Postings[0].RawText != "About Data Analyst" {
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
		{1, "Data Analyst", "Remote - US", "2026-10-01T00:00:00Z"},
		{2, "Data Analyst", "Charleston, SC", "2026-10-01T00:00:00Z"},
		{3, "Data Analyst", "Denver, CO", "2026-10-01T00:00:00Z"},
		{4, "Data Analyst", "Remote - Australia", "2026-10-01T00:00:00Z"},
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
		many = append(many, fakeJob{i, "Data Analyst", "Remote", fmt.Sprintf("2026-10-0%dT00:00:00Z", i)})
	}
	s := &searchServer{boards: map[string][]fakeJob{
		"big":   many,
		"small": {{10, "Data Analyst", "Remote", "2026-09-01T00:00:00Z"}},
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
			{1, "Data Analyst", "Remote", "2026-10-02T00:00:00Z"},
			{2, "Data Analyst", "Remote", "2026-10-01T00:00:00Z"},
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
