package greenhouse

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
)

const (
	searchDeadline   = 20 * time.Second
	listConcurrency  = 24
	fetchConcurrency = 8
	perCompanyCap    = 3
)

// Search lists the given boards, filters and ranks their postings, and fetches full data for the
// top q.Limit. A board that fails is reported in Errors; Search fails only when every board does.
// The query must already be validated, with Limit and WorkMode set.
func (c *Client) Search(ctx context.Context, q connectors.Query, boards []string) (connectors.SearchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, searchDeadline)
	defer cancel()

	result := connectors.SearchResult{
		Postings:     []connectors.Posting{},
		Errors:       []connectors.SearchError{},
		BoardsTotal:  len(boards),
		KeywordsUsed: q.Keywords,
		WorkModeUsed: q.WorkMode,
	}
	terms := locationTerms(q.Location)

	var (
		mu         sync.Mutex
		wg         sync.WaitGroup
		candidates []connectors.Posting
		failed     = map[string]string{}
		sem        = make(chan struct{}, listConcurrency)
	)
	for _, board := range boards {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				mu.Lock()
				failed[board] = "timed out"
				mu.Unlock()
				return
			}
			jobs, err := c.ListBoard(ctx, board)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed[board] = errorMessage(ctx, err)
				return
			}
			result.BoardsSearched++
			result.PostingsScanned += len(jobs)
			for _, l := range jobs {
				p := listingPosting(board, l)
				p.WorkMode = WorkMode(p.Location, p.PositionTitle)
				if !ModeOK(p.WorkMode, q.WorkMode) || !LocationOK(p.Location, p.WorkMode, terms, q.WorkMode) {
					continue
				}
				p.TitleScore = TitleScore(p.PositionTitle, q.Keywords)
				if p.TitleScore < minTitleScore {
					continue
				}
				if len(terms) > 0 && locationMatches(p.Location, terms) {
					result.LocationMatched++
				}
				p.Score = Score(p.TitleScore, LocationScore(p.Location, p.WorkMode, terms))
				candidates = append(candidates, p)
			}
		}()
	}
	wg.Wait()

	if err := ctx.Err(); err != nil && errors.Is(err, context.Canceled) {
		return result, err // the caller stopped waiting
	}
	result.Matched = len(candidates)

	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		if a.CompanyName != b.CompanyName {
			return a.CompanyName < b.CompanyName
		}
		if a.PositionTitle != b.PositionTitle {
			return a.PositionTitle < b.PositionTitle
		}
		return a.ExternalID < b.ExternalID
	})

	// Company variety: no company takes more than perCompanyCap places.
	var top []connectors.Posting
	perCompany := map[string]int{}
	for _, p := range candidates {
		if len(top) == q.Limit {
			break
		}
		if perCompany[p.Board] >= perCompanyCap {
			continue
		}
		perCompany[p.Board]++
		top = append(top, p)
	}

	// Full data for the top postings. A failed detail drops the posting and lists its board.
	full := make([]*connectors.Posting, len(top))
	fsem := make(chan struct{}, fetchConcurrency)
	for i, p := range top {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case fsem <- struct{}{}:
				defer func() { <-fsem }()
			case <-ctx.Done():
				mu.Lock()
				failed[p.Board] = "timed out"
				mu.Unlock()
				return
			}
			detail, err := c.fetchCached(ctx, p.Board, p.ExternalID)
			if err != nil {
				mu.Lock()
				failed[p.Board] = errorMessage(ctx, err)
				mu.Unlock()
				return
			}
			// Keep the listing's ranking fields and link rules; take the description from the detail.
			p.RawText = detail.RawText
			p.Dropped = detail.Dropped
			if detail.CompanyName != "" {
				p.CompanyName = detail.CompanyName
			}
			full[i] = &p
		}()
	}
	wg.Wait()
	for _, p := range full {
		if p != nil {
			result.Postings = append(result.Postings, *p)
		}
	}

	for board := range failed {
		result.Errors = append(result.Errors, connectors.SearchError{Board: board, Message: failed[board]})
	}
	sort.Slice(result.Errors, func(i, j int) bool { return result.Errors[i].Board < result.Errors[j].Board })
	if result.BoardsSearched == 0 && len(boards) > 0 {
		return result, fmt.Errorf("%w: every board failed", ErrorUpstream)
	}
	return result, nil
}

// fetchCached is FetchJob through the 24 hour description cache.
func (c *Client) fetchCached(ctx context.Context, board, id string) (connectors.Posting, error) {
	key := board + "/" + id
	cache := c.cache()
	if v, ok := cache.description(key); ok {
		return v.(connectors.Posting), nil
	}
	p, err := c.FetchJob(ctx, board, id)
	if err != nil {
		return connectors.Posting{}, err
	}
	cache.putDescription(key, p)
	return p, nil
}

// errorMessage is the short reason shown for a failed board. It never includes URLs or response text.
func errorMessage(ctx context.Context, err error) string {
	switch {
	case errors.Is(err, ErrorNotFound):
		return "board not found"
	case ctx.Err() != nil:
		return "timed out"
	case errors.Is(err, ErrorUpstream):
		return "Greenhouse did not respond"
	}
	return "request failed"
}
