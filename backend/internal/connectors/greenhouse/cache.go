package greenhouse

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
)

const (
	listingTTL     = 6 * time.Hour
	descriptionTTL = 24 * time.Hour
	timeFormat     = time.RFC3339
)

// cachedListing is one board's listing as kept in memory and in the database.
type cachedListing struct {
	Board     string
	ETag      string
	FetchedAt time.Time
	NotFound  bool // the board returned 404; remembered for the cache window
	Jobs      []Listing
}

// cachedDetail is a fetched posting with the counts of what cleaning removed, which Posting does not encode.
type cachedDetail struct {
	Posting connectors.Posting `json:"posting"`
	Dropped connectors.Dropped `json:"dropped"`
}

type timedDetail struct {
	detail connectors.Posting
	at     time.Time
}

// Cache holds board listings and job descriptions. Both live in the database, with a copy in memory
// so a search does not read a large listing from disk twice. Now is replaceable so tests can expire
// entries. A nil DB keeps everything in memory only. A database error is logged and the cache carries
// on from memory, since a cache that cannot be written must not fail a search.
type Cache struct {
	DB  *sql.DB
	Now func() time.Time

	mu       sync.Mutex
	listings map[string]cachedListing
	details  map[string]timedDetail
}

// NewCache returns a cache stored in db. A nil db keeps it in memory only.
func NewCache(db *sql.DB) *Cache {
	return &Cache{DB: db, Now: time.Now}
}

func (c *Cache) now() time.Time {
	if c.Now == nil {
		return time.Now()
	}
	return c.Now().UTC()
}

// listing returns the cached entry for a board, from memory or else from the database.
func (c *Cache) listing(board string) (cachedListing, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.listings[board]; ok {
		return e, true
	}
	if c.DB == nil {
		return cachedListing{}, false
	}
	var (
		e       = cachedListing{Board: board}
		jobs    string
		fetched string
		missing int
	)
	err := c.DB.QueryRow(`SELECT etag, not_found, jobs, fetched_at FROM greenhouse_listing_cache WHERE board = ?`, board).
		Scan(&e.ETag, &missing, &jobs, &fetched)
	if err != nil {
		if err != sql.ErrNoRows {
			slog.Warn("greenhouse cache read", "board", board, "error", err)
		}
		return cachedListing{}, false
	}
	t, err := time.Parse(timeFormat, fetched)
	if err != nil || json.Unmarshal([]byte(jobs), &e.Jobs) != nil {
		return cachedListing{}, false
	}
	e.FetchedAt, e.NotFound = t, missing == 1
	if c.listings == nil {
		c.listings = map[string]cachedListing{}
	}
	c.listings[board] = e
	return e, true
}

func (c *Cache) fresh(e cachedListing) bool {
	return c.now().Sub(e.FetchedAt) < listingTTL
}

func (c *Cache) putListing(e cachedListing) {
	c.mu.Lock()
	if c.listings == nil {
		c.listings = map[string]cachedListing{}
	}
	c.listings[e.Board] = e
	c.mu.Unlock()
	if c.DB == nil {
		return
	}
	jobs, err := json.Marshal(e.Jobs)
	if err != nil {
		return
	}
	if len(e.Jobs) == 0 {
		jobs = []byte("[]") // a nil slice encodes as null, which the column rejects
	}
	missing := 0
	if e.NotFound {
		missing = 1
	}
	_, err = c.DB.Exec(`INSERT INTO greenhouse_listing_cache (board, etag, not_found, jobs, fetched_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(board) DO UPDATE SET etag = excluded.etag, not_found = excluded.not_found, jobs = excluded.jobs, fetched_at = excluded.fetched_at`,
		e.Board, e.ETag, missing, string(jobs), e.FetchedAt.UTC().Format(timeFormat))
	if err != nil {
		slog.Warn("greenhouse cache write", "board", e.Board, "error", err)
	}
}

// description returns a cached posting that is under 24 hours old.
func (c *Cache) description(board, id string) (connectors.Posting, bool) {
	key := board + "/" + id
	c.mu.Lock()
	defer c.mu.Unlock()
	if d, ok := c.details[key]; ok {
		if c.now().Sub(d.at) < descriptionTTL {
			return d.detail, true
		}
		delete(c.details, key)
	}
	if c.DB == nil {
		return connectors.Posting{}, false
	}
	var body, fetched string
	err := c.DB.QueryRow(`SELECT posting, fetched_at FROM greenhouse_description_cache WHERE board = ? AND job_id = ?`, board, id).Scan(&body, &fetched)
	if err != nil {
		if err != sql.ErrNoRows {
			slog.Warn("greenhouse cache read", "board", board, "error", err)
		}
		return connectors.Posting{}, false
	}
	at, err := time.Parse(timeFormat, fetched)
	var d cachedDetail
	if err != nil || c.now().Sub(at) >= descriptionTTL || json.Unmarshal([]byte(body), &d) != nil {
		return connectors.Posting{}, false
	}
	d.Posting.Dropped = d.Dropped
	if c.details == nil {
		c.details = map[string]timedDetail{}
	}
	c.details[key] = timedDetail{d.Posting, at}
	return d.Posting, true
}

func (c *Cache) putDescription(board, id string, p connectors.Posting) {
	now := c.now()
	c.mu.Lock()
	if c.details == nil {
		c.details = map[string]timedDetail{}
	}
	c.details[board+"/"+id] = timedDetail{p, now}
	c.mu.Unlock()
	if c.DB == nil {
		return
	}
	body, err := json.Marshal(cachedDetail{Posting: p, Dropped: p.Dropped})
	if err != nil {
		return
	}
	_, err = c.DB.Exec(`INSERT INTO greenhouse_description_cache (board, job_id, posting, fetched_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(board, job_id) DO UPDATE SET posting = excluded.posting, fetched_at = excluded.fetched_at`,
		board, id, string(body), now.Format(timeFormat))
	if err != nil {
		slog.Warn("greenhouse cache write", "board", board, "error", err)
	}
}

// PurgeOlderThan deletes cached listings and descriptions fetched more than age ago, from the
// database and from memory, and returns how many database rows it removed.
func (c *Cache) PurgeOlderThan(age time.Duration) (int64, error) {
	cutoff := c.now().Add(-age)
	c.mu.Lock()
	for board, e := range c.listings {
		if e.FetchedAt.Before(cutoff) {
			delete(c.listings, board)
		}
	}
	for key, d := range c.details {
		if d.at.Before(cutoff) {
			delete(c.details, key)
		}
	}
	c.mu.Unlock()
	if c.DB == nil {
		return 0, nil
	}
	var total int64
	for _, table := range []string{"greenhouse_listing_cache", "greenhouse_description_cache"} {
		res, err := c.DB.Exec(`DELETE FROM `+table+` WHERE fetched_at < ?`, cutoff.Format(timeFormat))
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// Clear removes every cached listing and description. Storage cleanup calls it.
func (c *Cache) Clear() error {
	c.mu.Lock()
	c.listings, c.details = nil, nil
	c.mu.Unlock()
	if c.DB == nil {
		return nil
	}
	for _, table := range []string{"greenhouse_listing_cache", "greenhouse_description_cache"} {
		if _, err := c.DB.Exec(`DELETE FROM ` + table); err != nil {
			return err
		}
	}
	return nil
}
