package ashby

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
	u1 := "https://" + boardHost + "/linear/9f8e7d6c-1234-5678"
	board, id, err := ParseLink(u1)
	if err != nil {
		t.Fatalf("ParseLink failed: %v", err)
	}
	if board != "linear" || id != "9f8e7d6c-1234-5678" {
		t.Fatalf("got board=%q id=%q", board, id)
	}

	u2 := boardHost + "/ramp/abc12345/application"
	board, id, err = ParseLink(u2)
	if err != nil {
		t.Fatalf("ParseLink with /application failed: %v", err)
	}
	if board != "ramp" || id != "abc12345" {
		t.Fatalf("got board=%q id=%q", board, id)
	}

	if _, _, err := ParseLink("https://example.com/jobs/123"); err != ErrorNotAshby {
		t.Fatalf("expected ErrorNotAshby, got %v", err)
	}
}

func TestFetchAndListBoard(t *testing.T) {
	payload := `{"jobs":[{
		"id": "ashby-999999",
		"title": "Staff Product Engineer",
		"location": "San Francisco, CA",
		"isRemote": false,
		"workplaceType": "Hybrid",
		"jobUrl": "",
		"publishedAt": "2026-10-01T12:00:00Z",
		"descriptionPlain": "Own end-to-end TypeScript and Go systems.",
		"compensation": {"summaryComponents": [{"minValue": 190000, "maxValue": 230000, "currencyCode": "USD", "interval": "1Year"}]}
	}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/linear") {
			w.Write([]byte(payload))
			return
		}

		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	p, err := c.FetchJob(context.Background(), "linear", "ashby-999999")
	if err != nil {
		t.Fatalf("FetchJob failed: %v", err)
	}
	if p.Source != "ashby" || p.Board != "linear" || p.PositionTitle != "Staff Product Engineer" {
		t.Fatalf("unexpected posting: %+v", p)
	}
	if p.WorkMode != connectors.WorkModeHybrid || p.PayMin != 190000 || p.PayMax != 230000 {
		t.Fatalf("unexpected work mode/pay: %+v", p)
	}
	if !strings.Contains(p.RawText, "TypeScript and Go") {
		t.Fatalf("unexpected RawText: %q", p.RawText)
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
