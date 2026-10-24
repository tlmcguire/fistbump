package greenhouse

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/db"
)

// boardServer serves one board listing with an ETag and counts requests. It answers 304 when
// If-None-Match matches.
type boardServer struct {
	hits    atomic.Int32
	lastINM atomic.Value
	etag    string
	body    string
}

func (b *boardServer) start(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.hits.Add(1)
		b.lastINM.Store(r.Header.Get("If-None-Match"))
		if b.etag != "" {
			if r.Header.Get("If-None-Match") == b.etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", b.etag)
		}
		w.Write([]byte(b.body))
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, HTTP: srv.Client()}
}

const listBody = `{"jobs":[{"id":1,"title":"Data Analyst","location":{"name":"Remote - US"},"updated_at":"2026-10-06T20:15:10-04:00","absolute_url":"https://job-boards.greenhouse.io/acme/jobs/1"}]}`

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newCachedClient(t *testing.T, b *boardServer) (*Client, *clock, *sql.DB) {
	t.Helper()
	c := b.start(t)
	clk := &clock{time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	c.Cache = &Cache{DB: conn, Now: clk.now}
	return c, clk, conn
}

func rows(t *testing.T, conn *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestFreshListingUsedWithoutRequest(t *testing.T) {
	b := &boardServer{etag: `"v1"`, body: listBody}
	c, clk, _ := newCachedClient(t, b)
	for i := 0; i < 2; i++ {
		jobs, err := c.ListBoard(context.Background(), "acme")
		if err != nil || len(jobs) != 1 || jobs[0].Title != "Data Analyst" {
			t.Fatalf("jobs = %v, err = %v", jobs, err)
		}
		clk.t = clk.t.Add(5 * time.Hour)
	}
	if b.hits.Load() != 1 {
		t.Errorf("requests = %d, want 1", b.hits.Load())
	}
}

func TestExpiredListingRevalidates(t *testing.T) {
	b := &boardServer{etag: `"v1"`, body: listBody}
	c, clk, _ := newCachedClient(t, b)
	c.ListBoard(context.Background(), "acme")
	clk.t = clk.t.Add(7 * time.Hour)
	jobs, err := c.ListBoard(context.Background(), "acme")
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs = %v, err = %v", jobs, err)
	}
	if b.hits.Load() != 2 || b.lastINM.Load() != `"v1"` {
		t.Errorf("hits = %d, If-None-Match = %v", b.hits.Load(), b.lastINM.Load())
	}
	// The 304 renewed the entry, so the next call sends nothing.
	c.ListBoard(context.Background(), "acme")
	if b.hits.Load() != 2 {
		t.Errorf("requests after 304 = %d, want 2", b.hits.Load())
	}
}

func TestListingReadFromDatabaseAfterRestart(t *testing.T) {
	b := &boardServer{etag: `"v1"`, body: listBody}
	c, clk, conn := newCachedClient(t, b)
	c.ListBoard(context.Background(), "acme")
	c.Cache = &Cache{DB: conn, Now: clk.now} // a new process: empty memory, same database
	if _, err := c.ListBoard(context.Background(), "acme"); err != nil {
		t.Fatal(err)
	}
	if b.hits.Load() != 1 {
		t.Errorf("requests = %d, want 1", b.hits.Load())
	}
}

func TestNoETagFallsBackToExpiry(t *testing.T) {
	b := &boardServer{body: listBody}
	c, clk, _ := newCachedClient(t, b)
	c.ListBoard(context.Background(), "acme")
	clk.t = clk.t.Add(7 * time.Hour)
	c.ListBoard(context.Background(), "acme")
	if b.hits.Load() != 2 || b.lastINM.Load() != "" {
		t.Errorf("hits = %d, If-None-Match = %q", b.hits.Load(), b.lastINM.Load())
	}
}

func TestListingStoredInDatabase(t *testing.T) {
	b := &boardServer{etag: `"v1"`, body: listBody}
	c, _, conn := newCachedClient(t, b)
	c.ListBoard(context.Background(), "acme")
	var etag, jobs string
	if err := conn.QueryRow("SELECT etag, jobs FROM greenhouse_listing_cache WHERE board = 'acme'").Scan(&etag, &jobs); err != nil {
		t.Fatal(err)
	}
	if etag != `"v1"` || jobs == "[]" {
		t.Errorf("row = etag %q, jobs %q", etag, jobs)
	}
	// A refresh replaces the row instead of adding one.
	c.Cache.Now = (&clock{c.Cache.now().Add(7 * time.Hour)}).now
	c.ListBoard(context.Background(), "acme")
	if n := rows(t, conn, "greenhouse_listing_cache"); n != 1 {
		t.Errorf("rows = %d, want 1", n)
	}
}

func TestNotFoundBoardRemembered(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Cache: NewCache(openDB(t))}
	for i := 0; i < 2; i++ {
		if _, err := c.ListBoard(context.Background(), "gone"); err == nil {
			t.Fatal("want error")
		}
	}
	if hits.Load() != 1 {
		t.Errorf("requests = %d, want 1", hits.Load())
	}
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestClear(t *testing.T) {
	b := &boardServer{body: listBody}
	c, _, conn := newCachedClient(t, b)
	c.ListBoard(context.Background(), "acme")
	c.Cache.putDescription("acme", "1", samplePosting())
	if err := c.Cache.Clear(); err != nil {
		t.Fatal(err)
	}
	if rows(t, conn, "greenhouse_listing_cache") != 0 || rows(t, conn, "greenhouse_description_cache") != 0 {
		t.Error("tables not empty after Clear")
	}
	c.ListBoard(context.Background(), "acme")
	if b.hits.Load() != 2 {
		t.Errorf("requests = %d, want 2", b.hits.Load())
	}
}

func samplePosting() connectors.Posting {
	return connectors.Posting{
		ExternalID: "1", Board: "acme", CompanyName: "Acme", PositionTitle: "Data Analyst",
		RawText: "About the job", Dropped: connectors.Dropped{HiddenElements: 2, InvisibleChars: 3},
	}
}

func TestDescriptionPersistsWithDroppedCounts(t *testing.T) {
	b := &boardServer{body: listBody}
	c, clk, conn := newCachedClient(t, b)
	c.Cache.putDescription("acme", "1", samplePosting())

	c.Cache = &Cache{DB: conn, Now: clk.now} // a new process
	got, ok := c.Cache.description("acme", "1")
	if !ok || got.RawText != "About the job" || got.Dropped.HiddenElements != 2 || got.Dropped.InvisibleChars != 3 {
		t.Errorf("description = %+v, ok %v", got, ok)
	}
	clk.t = clk.t.Add(25 * time.Hour)
	if _, ok := c.Cache.description("acme", "1"); ok {
		t.Error("a description older than 24 hours must not be used")
	}
}

func TestPurgeOlderThanADay(t *testing.T) {
	b := &boardServer{body: listBody}
	c, clk, conn := newCachedClient(t, b)
	c.ListBoard(context.Background(), "old")
	c.Cache.putDescription("old", "1", samplePosting())
	clk.t = clk.t.Add(23 * time.Hour)
	c.ListBoard(context.Background(), "new")
	c.Cache.putDescription("new", "1", samplePosting())
	clk.t = clk.t.Add(2 * time.Hour) // old is 25 hours old, new is 2

	n, err := c.Cache.PurgeOlderThan(24 * time.Hour)
	if err != nil || n != 2 {
		t.Fatalf("purged %d rows, err %v; want 2", n, err)
	}
	if rows(t, conn, "greenhouse_listing_cache") != 1 || rows(t, conn, "greenhouse_description_cache") != 1 {
		t.Error("only the recent rows should remain")
	}
	var board string
	conn.QueryRow("SELECT board FROM greenhouse_listing_cache").Scan(&board)
	if board != "new" {
		t.Errorf("kept board %q, want new", board)
	}
	if _, ok := c.Cache.listing("old"); ok {
		t.Error("memory copy of the old listing survived")
	}
}
