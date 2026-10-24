package greenhouse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
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

func newCachedClient(t *testing.T, b *boardServer) (*Client, *clock, string) {
	t.Helper()
	c := b.start(t)
	clk := &clock{time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
	dir := t.TempDir()
	c.Cache = &Cache{Dir: dir, Now: clk.now}
	return c, clk, dir
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

func TestListingReadFromDiskAfterRestart(t *testing.T) {
	b := &boardServer{etag: `"v1"`, body: listBody}
	c, clk, dir := newCachedClient(t, b)
	c.ListBoard(context.Background(), "acme")
	c.Cache = &Cache{Dir: dir, Now: clk.now} // a new process: empty memory, same folder
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

func TestAtomicWriteLeavesNoTempFiles(t *testing.T) {
	b := &boardServer{etag: `"v1"`, body: listBody}
	c, _, dir := newCachedClient(t, b)
	c.ListBoard(context.Background(), "acme")
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "acme.json" {
		t.Fatalf("cache folder = %v", entries)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "acme.json"))
	var e cachedListing
	if err := json.Unmarshal(data, &e); err != nil || e.ETag != `"v1"` || len(e.Jobs) != 1 {
		t.Errorf("entry = %+v, err = %v", e, err)
	}
}

func TestNotFoundBoardRemembered(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), Cache: NewCache(t.TempDir())}
	for i := 0; i < 2; i++ {
		if _, err := c.ListBoard(context.Background(), "gone"); err == nil {
			t.Fatal("want error")
		}
	}
	if hits.Load() != 1 {
		t.Errorf("requests = %d, want 1", hits.Load())
	}
}

func TestClear(t *testing.T) {
	b := &boardServer{body: listBody}
	c, _, dir := newCachedClient(t, b)
	c.ListBoard(context.Background(), "acme")
	c.Cache.Clear()
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("cache folder = %v", entries)
	}
	c.ListBoard(context.Background(), "acme")
	if b.hits.Load() != 2 {
		t.Errorf("requests = %d, want 2", b.hits.Load())
	}
}
