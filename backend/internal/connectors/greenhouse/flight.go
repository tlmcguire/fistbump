package greenhouse

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// flights shares one request among callers that want the same thing at the same time. Home's job matches and a
// search in Find often want the same boards together; with this they ask Greenhouse once, not twice.
type flights struct {
	mu    sync.Mutex
	calls map[string]*flight
}

type flight struct {
	done     chan struct{}
	val      any
	err      error
	canceled bool // the caller that ran it gave up, so its answer is not the others' answer
}

// do runs fn for key unless a call for key is already running, in which case it waits for that call's answer. The
// call runs on its caller's context, so canceling a search stops its requests; a waiter whose own context is still
// live when that happens asks again instead of taking the canceled answer.
func (f *flights) do(ctx context.Context, key string, fn func(context.Context) (any, error)) (any, error) {
	for {
		f.mu.Lock()
		if f.calls == nil {
			f.calls = map[string]*flight{}
		}
		if c, ok := f.calls[key]; ok {
			f.mu.Unlock()
			select {
			case <-c.done:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			if c.canceled && ctx.Err() == nil {
				continue
			}
			return c.val, c.err
		}
		c := &flight{done: make(chan struct{})}
		f.calls[key] = c
		f.mu.Unlock()

		c.val, c.err = fn(ctx)
		c.canceled = ctx.Err() != nil
		f.mu.Lock()
		delete(f.calls, key)
		f.mu.Unlock()
		close(c.done)
		return c.val, c.err
	}
}

// backoff is how long the client stops asking Greenhouse after a 429. Greenhouse's Retry-After wins when it sends one,
// within limits. During a backoff, listings come from the cache when there is a copy and everything else fails at once
// with a rate-limit error, so the app stops adding to the limit instead of pushing through it.
const (
	defaultBackoff = 30 * time.Second
	maxBackoff     = 2 * time.Minute
)

type backoff struct{ until atomic.Int64 } // unix nanoseconds; 0 when not backing off

// active reports whether the client is backing off now.
func (b *backoff) active(now time.Time) bool { return now.UnixNano() < b.until.Load() }

// start begins or extends a backoff from a 429 answer.
func (b *backoff) start(now time.Time, res *http.Response) {
	wait := defaultBackoff
	if res != nil {
		if secs, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && secs > 0 {
			wait = min(time.Duration(secs)*time.Second, maxBackoff)
		}
	}
	until := now.Add(wait).UnixNano()
	for {
		old := b.until.Load()
		if old >= until || b.until.CompareAndSwap(old, until) {
			return
		}
	}
}

// The request budget. Every request to the Greenhouse API spends one. The budget holds budgetSize and refills at
// budgetRefill a second, so a search after a quiet spell, even one that lists every board, goes out at full speed,
// while back-to-back searches drain it and slow to the refill rate. Greenhouse publishes no limit for its job board
// API; a full budget is about one cold search, and the backoff above covers anything the budget misses.
const (
	budgetSize   = 1000
	budgetRefill = 8 // a second: about 480 a minute when searching steadily
	// lowBudget is the point below which a board listed before is served from the cache, at any age, instead of
	// being asked again, so a busy user keeps getting answers without spending what is left.
	lowBudget = 200
)

type budget struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
	size   float64 // 0 means budgetSize; tests set smaller ones
	refill float64 // 0 means budgetRefill
}

func (b *budget) params() (size, refill float64) {
	size, refill = b.size, b.refill
	if size <= 0 {
		size = budgetSize
	}
	if refill <= 0 {
		refill = budgetRefill
	}
	return size, refill
}

// fill adds what has refilled since last time. The caller holds mu.
func (b *budget) fill(now time.Time) {
	size, refill := b.params()
	if b.last.IsZero() {
		b.tokens, b.last = size, now // a fresh start has the whole budget
		return
	}
	b.tokens = min(size, b.tokens+now.Sub(b.last).Seconds()*refill)
	b.last = now
}

// low reports whether the budget is low enough that a cached listing should be used instead of a request.
func (b *budget) low() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fill(time.Now())
	return b.tokens < lowBudget
}

// spend takes one request from the budget, waiting for it to refill if it is empty, or until the context ends.
func (b *budget) spend(ctx context.Context) error {
	for {
		b.mu.Lock()
		now := time.Now()
		b.fill(now)
		if b.tokens >= 1 {
			b.tokens--
			b.mu.Unlock()
			return nil
		}
		_, refill := b.params()
		wait := time.Duration((1 - b.tokens) / refill * float64(time.Second))
		b.mu.Unlock()
		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		}
	}
}

// maxInFlight caps the requests to Greenhouse that are open at once, across every search, page and Home's matches
// together. Each search also has its own smaller limits; this one holds when several run at the same time.
const maxInFlight = 16

type slots struct {
	once sync.Once
	ch   chan struct{}
}

// acquire waits for a free slot, or until the context ends. The caller releases it when the response is read.
func (s *slots) acquire(ctx context.Context) (release func(), err error) {
	s.once.Do(func() { s.ch = make(chan struct{}, maxInFlight) })
	select {
	case s.ch <- struct{}{}:
		return func() { <-s.ch }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
