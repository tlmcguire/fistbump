package cost

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/catalogfeed"
)

// RefreshNow fetches feedURL (or the catalog's configured FeedURL when empty), validates the
// catalog, updates the active in-memory catalog, and caches it to <dataDir>/cost-sources.json.
func RefreshNow(ctx context.Context, dataDir, feedURL string, client *http.Client) bool {
	if feedURL == "" {
		feedURL = loadBundledCatalog().FeedURL
	}
	if feedURL == "" {
		return false
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := catalogfeed.Get(reqCtx, client, feedURL, 256<<10)
	if err != nil {
		slog.Debug("cost sources refresh skipped", "error", err)
		return false
	}
	if !ApplyCatalogJSON(dataDir, body, time.Now()) {
		return false
	}
	cat, _ := CurrentCatalog()
	slog.Info("refreshed cost reference sources", "updated_at", cat.UpdatedAt)
	return true
}

// StartUpdater loads any cached cost-sources file from dataDir and checks in the background every
// 24 hours whether 30 days have elapsed since the last refresh. Offline launches gracefully keep
// the cached or bundled data.
func StartUpdater(ctx context.Context, dataDir, feedURL string) {
	LoadCachedCatalog(dataDir)
	go func() {
		check := func() {
			catalogMu.RLock()
			cur := activeCat
			catalogMu.RUnlock()
			if NeedsRefresh(cur, time.Now().UTC()) {
				RefreshNow(ctx, dataDir, feedURL, nil)
			}
		}
		check()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				check()
			}
		}
	}()
}
