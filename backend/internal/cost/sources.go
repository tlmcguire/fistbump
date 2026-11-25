package cost

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

//go:embed sources.json
var bundledSourcesJSON []byte

// RefreshDays is the cadence (in days) for refreshing cost-of-living, pay, tax, and benefits sources.
const RefreshDays = 30

// Source cites an authoritative reference behind cost-of-living, pay, tax, or benefits figures.
type Source struct {
	Category    string `json:"category"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
	UpdatedAt   string `json:"updated_at"`
}

// ReferenceCatalog holds the active citation metadata and 30-day refresh schedule.
type ReferenceCatalog struct {
	UpdatedAt   string   `json:"updated_at"`
	RefreshDays int      `json:"refresh_days"`
	FeedURL     string   `json:"feed_url,omitempty"`
	FetchedAt   string   `json:"fetched_at,omitempty"`
	Sources     []Source `json:"sources"`
}

var (
	catalogMu sync.RWMutex
	activeCat = loadBundledCatalog()
)

func loadBundledCatalog() ReferenceCatalog {
	var cat ReferenceCatalog
	if err := json.Unmarshal(bundledSourcesJSON, &cat); err != nil {
		return ReferenceCatalog{UpdatedAt: "2026-11-25", RefreshDays: RefreshDays}
	}
	if cat.RefreshDays <= 0 {
		cat.RefreshDays = RefreshDays
	}
	return cat
}

// ResetCatalogForTest restores activeCat to the bundled catalog.
func ResetCatalogForTest() {
	catalogMu.Lock()
	activeCat = loadBundledCatalog()
	catalogMu.Unlock()
}

// LoadCachedCatalog loads an updated sources catalog from dataDir if present and newer than the bundled one.
func LoadCachedCatalog(dataDir string) {
	if dataDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "cost-sources.json"))
	if err != nil {
		return
	}
	var cat ReferenceCatalog
	if err := json.Unmarshal(data, &cat); err != nil || len(cat.Sources) == 0 || cat.UpdatedAt == "" {
		return
	}
	if cat.RefreshDays <= 0 {
		cat.RefreshDays = RefreshDays
	}
	catalogMu.Lock()
	if cat.UpdatedAt >= activeCat.UpdatedAt {
		activeCat = cat
	}
	catalogMu.Unlock()
}

// NeedsRefresh reports whether cat is due for a 30-day refresh relative to now.
func NeedsRefresh(cat ReferenceCatalog, now time.Time) bool {
	days := cat.RefreshDays
	if days <= 0 {
		days = RefreshDays
	}
	interval := time.Duration(days) * 24 * time.Hour
	if cat.FetchedAt != "" {
		if t, err := time.Parse(time.RFC3339, cat.FetchedAt); err == nil {
			return now.Sub(t) >= interval
		}
	}
	if cat.UpdatedAt != "" {
		if t, err := time.Parse("2006-01-02", cat.UpdatedAt); err == nil {
			return now.Sub(t) >= interval
		}
	}
	return true
}

// ApplyCatalogJSON validates a JSON catalog payload, activates it in memory, and caches it in dataDir.
func ApplyCatalogJSON(dataDir string, body []byte, now time.Time) bool {
	var fresh ReferenceCatalog
	if err := json.Unmarshal(body, &fresh); err != nil || len(fresh.Sources) == 0 || fresh.UpdatedAt == "" {
		return false
	}
	if fresh.RefreshDays <= 0 {
		fresh.RefreshDays = RefreshDays
	}
	fresh.FetchedAt = now.UTC().Format(time.RFC3339)
	catalogMu.Lock()
	activeCat = fresh
	catalogMu.Unlock()
	if dataDir != "" {
		if encoded, err := json.MarshalIndent(fresh, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(dataDir, "cost-sources.json"), encoded, 0o600)
		}
	}
	return true
}

// CurrentCatalog returns a copy of the active reference sources and next scheduled 30-day update date.
func CurrentCatalog() (ReferenceCatalog, string) {
	catalogMu.RLock()
	cat := activeCat
	catalogMu.RUnlock()

	sources := make([]Source, len(cat.Sources))
	copy(sources, cat.Sources)
	cat.Sources = sources

	nextUpdate := ""
	if t, err := time.Parse("2006-01-02", cat.UpdatedAt); err == nil {
		days := cat.RefreshDays
		if days <= 0 {
			days = RefreshDays
		}
		nextUpdate = t.AddDate(0, 0, days).Format("2006-01-02")
	}
	return cat, nextUpdate
}
