package connectors

import (
	"context"
	"hash/fnv"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	listingCacheRefreshInterval = 24 * time.Hour
	listingCacheMaxAge          = 24 * time.Hour
	listingCacheMaxBoards       = 100
	listingCacheConcurrency     = 4
	listingCacheRequestTimeout  = 15 * time.Second
)

// ListingCacheSource exposes stale, previously searched boards and refreshes one board from its official ATS API.
type ListingCacheSource struct {
	Name        string
	StaleBoards func(context.Context, time.Time) ([]string, error)
	Refresh     func(context.Context, string) error
}

// StartListingCacheUpdater refreshes only boards already present in the local search caches.
func StartListingCacheUpdater(ctx context.Context, sources ...ListingCacheSource) {
	if len(sources) == 0 {
		return
	}
	go func() {
		refreshListingCaches(ctx, sources, time.Now().UTC())
		ticker := time.NewTicker(listingCacheRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				refreshListingCaches(ctx, sources, now.UTC())
			}
		}
	}()
}

func refreshListingCaches(ctx context.Context, sources []ListingCacheSource, now time.Time) {
	type boardJob struct {
		source ListingCacheSource
		board  string
	}
	var jobs []boardJob
	cutoff := now.Add(-listingCacheMaxAge)
	for _, source := range sources {
		if source.Name == "" || source.StaleBoards == nil || source.Refresh == nil {
			slog.Warn("skipping invalid listing cache source", "source", source.Name)
			continue
		}
		boards, err := source.StaleBoards(ctx, cutoff)
		if err != nil {
			slog.Warn("listing cache refresh: reading stale boards", "source", source.Name, "error", err)
			continue
		}
		for _, board := range boardsForRefresh(boards, source.Name, now) {
			jobs = append(jobs, boardJob{source: source, board: board})
		}
	}
	if len(jobs) == 0 {
		return
	}

	sem := make(chan struct{}, listingCacheConcurrency)
	var wg sync.WaitGroup
	var refreshed atomic.Int32
	for _, job := range jobs {
		wg.Add(1)
		go func(job boardJob) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			boardCtx, cancel := context.WithTimeout(ctx, listingCacheRequestTimeout)
			defer cancel()
			if err := job.source.Refresh(boardCtx, job.board); err != nil {
				slog.Warn("listing cache refresh failed", "source", job.source.Name, "board", job.board, "error", err)
				return
			}
			refreshed.Add(1)
		}(job)
	}
	wg.Wait()
	slog.Info("completed local ATS listing cache refresh", "refreshed", refreshed.Load(), "selected", len(jobs))
}

func boardsForRefresh(boards []string, source string, now time.Time) []string {
	ordered := append([]string(nil), boards...)
	if len(ordered) <= listingCacheMaxBoards {
		sort.Strings(ordered)
		return ordered
	}
	day := now.Format("2006-01-02")
	rank := func(board string) uint64 {
		h := fnv.New64a()
		_, _ = h.Write([]byte(day + "\x00" + source + "\x00" + board))
		return h.Sum64()
	}
	sort.Slice(ordered, func(i, j int) bool {
		left, right := rank(ordered[i]), rank(ordered[j])
		if left == right {
			return ordered[i] < ordered[j]
		}
		return left < right
	})
	return ordered[:listingCacheMaxBoards]
}
