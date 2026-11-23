package greenhouse

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// limited serves a board listing and one job, answering 429 while limit is set.
func limited(t *testing.T, limit *atomic.Bool, hits *atomic.Int32) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if limit.Load() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/jobs") {
			w.Write([]byte(`{"jobs":[{"id":1,"title":"Engineer","location":{"name":"Remote"},"updated_at":"2026-11-20T00:00:00Z","absolute_url":"https://job-boards.greenhouse.io/acme/jobs/1"}]}`))
			return
		}
		w.Write([]byte(`{"id":1,"title":"Engineer","location":{"name":"Remote"},"updated_at":"2026-11-20T00:00:00Z","absolute_url":"https://job-boards.greenhouse.io/acme/jobs/1","content":"Build things.","company_name":"Acme"}`))
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, HTTP: srv.Client()}
}

func TestRateLimitedListing(t *testing.T) {
	var limit atomic.Bool
	var hits atomic.Int32
	c := limited(t, &limit, &hits)
	limit.Store(true)
	_, err := c.ListBoard(context.Background(), "acme")
	if !errors.Is(err, ErrorRateLimited) || !errors.Is(err, ErrorUpstream) {
		t.Fatalf("err = %v; want a rate limit that is also upstream", err)
	}
	if got := errorMessage(context.Background(), err); got != RateLimitedMessage {
		t.Errorf("message %q", got)
	}
	// once listed, a later 429 serves the older copy instead of failing
	limit.Store(false)
	c.limit.until.Store(0) // the first 429 started a backoff; end it so the listing can be fetched
	if _, err := c.ListBoard(context.Background(), "acme"); err != nil {
		t.Fatal(err)
	}
	cache := c.cache()
	e, _ := cache.listing("acme")
	e.FetchedAt = time.Now().Add(-48 * time.Hour) // stale, so it is asked for again
	cache.putListing(e)
	limit.Store(true)
	jobs, err := c.ListBoard(context.Background(), "acme")
	if err != nil || len(jobs) != 1 {
		t.Errorf("stale copy under a rate limit: %v %d", err, len(jobs))
	}
}

func TestRateLimitBacksOff(t *testing.T) {
	var limit atomic.Bool
	var hits atomic.Int32
	c := limited(t, &limit, &hits)
	limit.Store(true)
	if _, err := c.fetchCached(context.Background(), "acme", "1"); !errors.Is(err, ErrorRateLimited) {
		t.Fatalf("err = %v", err)
	}
	// while backing off, nothing more is sent, even after the limit lifts
	limit.Store(false)
	for range 5 {
		if _, err := c.fetchCached(context.Background(), "acme", "1"); !errors.Is(err, ErrorRateLimited) {
			t.Errorf("during the backoff: %v", err)
		}
		if _, err := c.ListBoard(context.Background(), "other"); !errors.Is(err, ErrorRateLimited) {
			t.Errorf("listing during the backoff: %v", err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("%d requests; want only the first", hits.Load())
	}
	// once it ends, requests go out again
	c.limit.until.Store(0)
	if _, err := c.fetchCached(context.Background(), "acme", "1"); err != nil {
		t.Errorf("after the backoff: %v", err)
	}
}

func TestBackoffHonorsRetryAfter(t *testing.T) {
	var b backoff
	now := time.Now()
	b.start(now, &http.Response{Header: http.Header{"Retry-After": []string{"5"}}})
	if !b.active(now.Add(4*time.Second)) || b.active(now.Add(6*time.Second)) {
		t.Error("Retry-After of 5 seconds not honored")
	}
	b.start(now, &http.Response{Header: http.Header{"Retry-After": []string{"99999"}}})
	if b.active(now.Add(maxBackoff + time.Second)) {
		t.Error("a huge Retry-After should be capped")
	}
	var d backoff
	d.start(now, nil)
	if !d.active(now.Add(defaultBackoff-time.Second)) || d.active(now.Add(defaultBackoff+time.Second)) {
		t.Error("default backoff")
	}
}

func TestSearchReportsRateLimit(t *testing.T) {
	var limit atomic.Bool
	var hits atomic.Int32
	c := limited(t, &limit, &hits)
	// list fine, then descriptions are limited
	if _, err := c.ListBoard(context.Background(), "acme"); err != nil {
		t.Fatal(err)
	}
	limit.Store(true)
	r, err := c.Search(context.Background(), query("engineer"), []string{"acme"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.RateLimited || len(r.Postings) != 0 || r.Matched != 1 {
		t.Errorf("rate limited %v, postings %d, matched %d", r.RateLimited, len(r.Postings), r.Matched)
	}
}

func TestSearchSaysWhenEveryBoardIsRateLimited(t *testing.T) {
	var limit atomic.Bool
	var hits atomic.Int32
	c := limited(t, &limit, &hits)
	limit.Store(true)
	_, err := c.Search(context.Background(), query("engineer"), []string{"acme"})
	if !errors.Is(err, ErrorRateLimited) {
		t.Errorf("err = %v; want a rate limit", err)
	}
}

func TestConcurrentCallersShareOneRequest(t *testing.T) {
	var limit atomic.Bool
	var hits atomic.Int32
	c := limited(t, &limit, &hits)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.ListBoard(context.Background(), "acme"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if hits.Load() > 2 {
		t.Errorf("%d requests for 8 callers at once; want them shared", hits.Load())
	}
}

// A full budget goes out at once; an empty one waits for the refill; an idle spell earns it back.
func TestBudget(t *testing.T) {
	b := budget{size: 10, refill: 50}
	start := time.Now()
	for range 10 {
		if err := b.spend(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Errorf("a full budget should not wait (%s)", time.Since(start))
	}
	start = time.Now()
	for range 10 {
		b.spend(context.Background())
	}
	// 10 more at 50 a second take about 200ms
	if got := time.Since(start); got < 150*time.Millisecond {
		t.Errorf("an empty budget should slow to the refill rate, took %s", got)
	}
	time.Sleep(220 * time.Millisecond) // idle: refills
	start = time.Now()
	for range 10 {
		b.spend(context.Background())
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Errorf("after an idle spell the budget should be back (%s)", time.Since(start))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for range 20 {
		b.spend(context.Background())
	}
	if err := b.spend(ctx); err == nil {
		t.Error("a canceled context should end the wait")
	}
}

// With the budget low, a board listed before is served from the cache instead of being asked again.
func TestLowBudgetUsesCache(t *testing.T) {
	var limit atomic.Bool
	var hits atomic.Int32
	c := limited(t, &limit, &hits)
	if _, err := c.ListBoard(context.Background(), "acme"); err != nil {
		t.Fatal(err)
	}
	cache := c.cache()
	e, _ := cache.listing("acme")
	e.FetchedAt = time.Now().Add(-48 * time.Hour)
	cache.putListing(e)
	c.budget.mu.Lock()
	c.budget.tokens = lowBudget - 1
	c.budget.mu.Unlock()
	before := hits.Load()
	jobs, err := c.ListBoard(context.Background(), "acme")
	if err != nil || len(jobs) != 1 || hits.Load() != before {
		t.Errorf("low budget: err %v, jobs %d, requests %d", err, len(jobs), hits.Load()-before)
	}
}

// A waiter on a shared request whose leader was canceled asks again rather than taking the canceled answer.
func TestSharedRequestSurvivesLeaderCancel(t *testing.T) {
	var f flights
	leader, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	go f.do(leader, "k", func(ctx context.Context) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	<-started
	got := make(chan any)
	go func() {
		v, _ := f.do(context.Background(), "k", func(context.Context) (any, error) { return "fresh", nil })
		got <- v
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case v := <-got:
		if v != "fresh" {
			t.Errorf("waiter got %v; want its own answer", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiter never answered")
	}
}

// Home's matches search only what earlier searches listed, and send no listing request.
func TestCachedOnlySearchListsNothing(t *testing.T) {
	var limit atomic.Bool
	var hits atomic.Int32
	c := limited(t, &limit, &hits)
	q := query("engineer")
	q.CachedOnly = true
	r, err := c.Search(context.Background(), q, []string{"acme"})
	if err != nil || hits.Load() != 0 || r.BoardsSearched != 0 {
		t.Fatalf("with nothing cached: err %v, requests %d, boards %d", err, hits.Load(), r.BoardsSearched)
	}
	if _, err := c.ListBoard(context.Background(), "acme"); err != nil {
		t.Fatal(err)
	}
	before := hits.Load()
	r, err = c.Search(context.Background(), q, []string{"acme"})
	if err != nil || r.BoardsSearched != 1 || len(r.Postings) != 1 {
		t.Fatalf("with a cached board: err %v, boards %d, postings %d", err, r.BoardsSearched, len(r.Postings))
	}
	if got := hits.Load() - before; got != 1 {
		t.Errorf("%d requests; want only the one description", got)
	}
}

// However many callers there are, no more than maxInFlight requests are open at once.
func TestInFlightIsBounded(t *testing.T) {
	var open, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := open.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		open.Add(-1)
		w.Write([]byte(`{"jobs":[]}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	var wg sync.WaitGroup
	for i := range 60 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.ListBoard(context.Background(), fmt.Sprintf("board%d", i))
		}()
	}
	wg.Wait()
	if peak.Load() > maxInFlight {
		t.Errorf("%d requests open at once; the cap is %d", peak.Load(), maxInFlight)
	}
}
