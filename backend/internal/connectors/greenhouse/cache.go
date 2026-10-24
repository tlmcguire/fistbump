package greenhouse

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	listingTTL     = 6 * time.Hour
	descriptionTTL = 24 * time.Hour
)

// cachedListing is one board's listing as kept in memory and on disk.
type cachedListing struct {
	Board     string    `json:"board"`
	ETag      string    `json:"etag"`
	FetchedAt time.Time `json:"fetched_at"`
	NotFound  bool      `json:"not_found,omitempty"` // the board returned 404; remembered for the cache window
	Jobs      []Listing `json:"jobs"`
}

type cachedDescription struct {
	posting any
	at      time.Time
}

// Cache holds board listings (memory and disk) and job descriptions (memory). Now is replaceable
// so tests can expire entries.
type Cache struct {
	Dir string // empty means memory only
	Now func() time.Time

	mu       sync.Mutex
	listings map[string]cachedListing
	details  map[string]cachedDescription
}

// NewCache returns a cache that writes listings under dir. An empty dir keeps them in memory only.
func NewCache(dir string) *Cache {
	return &Cache{Dir: dir, Now: time.Now}
}

func (c *Cache) now() time.Time {
	if c.Now == nil {
		return time.Now()
	}
	return c.Now()
}

func (c *Cache) path(board string) string {
	return filepath.Join(c.Dir, board+".json")
}

// listing returns the cached entry for a board, from memory or else from disk.
func (c *Cache) listing(board string) (cachedListing, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.listings[board]; ok {
		return e, true
	}
	if c.Dir == "" {
		return cachedListing{}, false
	}
	data, err := os.ReadFile(c.path(board))
	if err != nil {
		return cachedListing{}, false
	}
	var e cachedListing
	if json.Unmarshal(data, &e) != nil || e.Board != board {
		return cachedListing{}, false
	}
	if c.listings == nil {
		c.listings = map[string]cachedListing{}
	}
	c.listings[board] = e
	return e, true
}

func (c *Cache) fresh(e cachedListing) bool {
	return c.now().Sub(e.FetchedAt) < listingTTL
}

// putListing stores an entry. The disk write goes to a temp file in the same folder and is then
// renamed, so a crash never leaves half a file. A disk failure leaves the memory copy in place.
func (c *Cache) putListing(e cachedListing) {
	c.mu.Lock()
	if c.listings == nil {
		c.listings = map[string]cachedListing{}
	}
	c.listings[e.Board] = e
	c.mu.Unlock()
	if c.Dir == "" {
		return
	}
	data, err := json.Marshal(e)
	if err != nil || os.MkdirAll(c.Dir, 0o700) != nil {
		return
	}
	tmp, err := os.CreateTemp(c.Dir, e.Board+".*.tmp")
	if err != nil {
		return
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), c.path(e.Board)) != nil {
		os.Remove(tmp.Name())
	}
}

func (c *Cache) description(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.details[key]
	if !ok || c.now().Sub(d.at) >= descriptionTTL {
		delete(c.details, key)
		return nil, false
	}
	return d.posting, true
}

func (c *Cache) putDescription(key string, posting any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.details == nil {
		c.details = map[string]cachedDescription{}
	}
	c.details[key] = cachedDescription{posting, c.now()}
}

// Clear removes every cached listing and description.
func (c *Cache) Clear() {
	c.mu.Lock()
	c.listings, c.details = nil, nil
	c.mu.Unlock()
	if c.Dir != "" {
		os.RemoveAll(c.Dir)
		os.MkdirAll(c.Dir, 0o700)
	}
}
