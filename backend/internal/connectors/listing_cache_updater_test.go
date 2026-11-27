package connectors

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestBoardsForRefreshIsBoundedAndDeterministic(t *testing.T) {
	boards := make([]string, listingCacheMaxBoards+25)
	for i := range boards {
		boards[i] = fmt.Sprintf("board-%03d", i)
	}
	now := time.Date(2026, time.November, 26, 12, 0, 0, 0, time.UTC)

	first := boardsForRefresh(boards, "greenhouse", now)
	second := boardsForRefresh(boards, "greenhouse", now)
	if len(first) != listingCacheMaxBoards {
		t.Fatalf("selected %d boards, want %d", len(first), listingCacheMaxBoards)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("same day selected different boards at index %d", i)
		}
	}
}

func TestRefreshListingCachesOnlyStaleBoardsWithinCap(t *testing.T) {
	boards := make([]string, listingCacheMaxBoards+10)
	for i := range boards {
		boards[i] = fmt.Sprintf("board-%03d", i)
	}
	var refreshed atomic.Int32
	var inFlight, maxInFlight atomic.Int32
	started := make(chan struct{}, len(boards))
	release := make(chan struct{})
	source := ListingCacheSource{
		Name: "fake",
		StaleBoards: func(_ context.Context, cutoff time.Time) ([]string, error) {
			if cutoff.IsZero() {
				t.Fatal("stale cutoff was not set")
			}
			return boards, nil
		},
		Refresh: func(_ context.Context, board string) error {
			if board == "" {
				t.Error("empty board was refreshed")
			}
			current := inFlight.Add(1)
			for {
				previous := maxInFlight.Load()
				if current <= previous || maxInFlight.CompareAndSwap(previous, current) {
					break
				}
			}
			started <- struct{}{}
			<-release
			inFlight.Add(-1)
			refreshed.Add(1)
			return nil
		},
	}
	now := time.Date(2026, time.November, 26, 12, 0, 0, 0, time.UTC)
	done := make(chan struct{})
	go func() {
		refreshListingCaches(context.Background(), []ListingCacheSource{source}, now)
		close(done)
	}()
	for i := 0; i < listingCacheConcurrency; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			<-done
			t.Fatal("refresh workers did not start within the timeout")
		}
	}
	select {
	case <-started:
		close(release)
		<-done
		t.Fatal("refresh concurrency exceeded its configured limit")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-done
	if got := refreshed.Load(); got != listingCacheMaxBoards {
		t.Fatalf("refreshed %d boards, want %d", got, listingCacheMaxBoards)
	}
	if got := maxInFlight.Load(); got != listingCacheConcurrency {
		t.Fatalf("maximum refresh concurrency = %d, want %d", got, listingCacheConcurrency)
	}
}
