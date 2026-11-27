package lever

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
)

func TestParseLink(t *testing.T) {
	u1 := "https://" + boardHost + "/acme-corp/12345678-aaaa-bbbb-cccc-1234567890ab"
	board, id, err := ParseLink(u1)
	if err != nil {
		t.Fatalf("ParseLink failed: %v", err)
	}
	if board != "acme-corp" || id != "12345678-aaaa-bbbb-cccc-1234567890ab" {
		t.Fatalf("got board=%q id=%q", board, id)
	}

	u2 := boardHost + "/Netflix/abcdef12-3456/apply"
	board, id, err = ParseLink(u2)
	if err != nil {
		t.Fatalf("ParseLink with /apply failed: %v", err)
	}
	if board != "netflix" || id != "abcdef12-3456" {
		t.Fatalf("got board=%q id=%q", board, id)
	}

	if _, _, err := ParseLink("https://example.com/jobs/123"); err != ErrorNotLever {
		t.Fatalf("expected ErrorNotLever, got %v", err)
	}
}

func TestFetchAndListBoard(t *testing.T) {
	payload := `{
		"id": "job-123456",
		"text": "Senior Backend Engineer",
		"hostedUrl": "",
		"createdAt": 1700000000000,
		"descriptionPlain": "Build scalable APIs in Go.",
		"lists": [{"text": "Requirements", "content": "<li>5+ years Go</li><li>SQL</li>"}],
		"categories": {"location": "Austin, TX"},
		"workplaceType": "remote",
		"salaryRange": {"min": 150000, "max": 185000, "currency": "USD", "interval": "per-year-salary"}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/job-123456") {
			w.Write([]byte(payload))
			return
		}

		if strings.HasSuffix(r.URL.Path, "/acme") {
			w.Write([]byte("[" + payload + "]"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	p, err := c.FetchJob(context.Background(), "acme", "job-123456")
	if err != nil {
		t.Fatalf("FetchJob failed: %v", err)
	}
	if p.Source != "lever" || p.Board != "acme" || p.PositionTitle != "Senior Backend Engineer" {
		t.Fatalf("unexpected posting: %+v", p)
	}
	if p.WorkMode != connectors.WorkModeRemote || p.PayMin != 150000 || p.PayMax != 185000 {
		t.Fatalf("unexpected work mode/pay: %+v", p)
	}
	if !strings.Contains(p.RawText, "5+ years Go") {
		t.Fatalf("expected requirements in RawText, got %q", p.RawText)
	}

	list, err := c.ListBoard(context.Background(), "acme")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListBoard failed: len=%d err=%v", len(list), err)
	}
}

func TestBoardsStaleBeforeAndClear(t *testing.T) {
	now := time.Now()
	c := &Client{cache: map[string]cacheEntry{
		"stale":  {fetchedAt: now.Add(-48 * time.Hour)},
		"recent": {fetchedAt: now.Add(-time.Hour)},
		"failed": {err: ErrorUpstream, fetchedAt: now.Add(-48 * time.Hour)},
	}}
	boards := c.BoardsStaleBefore(now.Add(-24 * time.Hour))
	if len(boards) != 1 || boards[0] != "stale" {
		t.Fatalf("stale boards = %v, want [stale]", boards)
	}
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if got := c.BoardsStaleBefore(now); len(got) != 0 {
		t.Fatalf("boards after Clear = %v, want none", got)
	}
}
