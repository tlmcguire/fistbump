package greenhouse

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
)

const (
	searchDeadline   = 20 * time.Second
	listConcurrency  = 24
	fetchConcurrency = 8
	perCompanyCap    = 3
	payScanCap       = 300 // descriptions read at most when a pay range is set
	payChunk         = 40
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
		Companies:    []string{},
		BoardsTotal:  len(boards),
		KeywordsUsed: q.Keywords,
		WorkModeUsed: q.WorkMode,
	}
	terms := locationTerms(q.Location)
	oldest := c.cache().now().AddDate(0, 0, -q.PostedWithin)

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
				if !LevelOK(Level(p.PositionTitle), q.Level) || !ExcludeOK(p.PositionTitle, q.Exclude) {
					continue
				}
				if q.PostedWithin > 0 && !p.UpdatedAt.IsZero() && p.UpdatedAt.Before(oldest) {
					continue
				}
				p.TitleScore = 1 // with no keywords, any title fits
				if len(q.Keywords) > 0 {
					p.TitleScore = TitleScore(p.PositionTitle, q.Keywords)
				}
				if p.TitleScore < minTitleScore {
					continue
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
	newest := q.Sort == connectors.SortNewest
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if !newest && a.Score != b.Score {
			return a.Score > b.Score
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.CompanyName != b.CompanyName {
			return a.CompanyName < b.CompanyName
		}
		if a.PositionTitle != b.PositionTitle {
			return a.PositionTitle < b.PositionTitle
		}
		return a.ExternalID < b.ExternalID
	})

	// The same role posted in several places is one result. The best-ranked copy stays and counts the rest.
	candidates = foldDuplicates(candidates)
	result.Matched = len(candidates)
	result.LocationMatched = 0
	if len(terms) > 0 {
		for _, p := range candidates {
			if locationMatches(p.Location, terms) {
				result.LocationMatched++
			}
		}
	}

	// Company variety: each company's first perCompanyCap postings come ahead of its later ones, so
	// no company fills a page. Nothing is dropped, so paging reaches every match, and the order is
	// fixed, so no page repeats another.
	var first, later []connectors.Posting
	perCompany := map[string]int{}
	for _, p := range candidates {
		if perCompany[p.Board] < perCompanyCap {
			first = append(first, p)
		} else {
			later = append(later, p)
		}
		perCompany[p.Board]++
	}
	ranked := append(first, later...)

	fill := func(posts []connectors.Posting) []*connectors.Posting {
		return c.fill(ctx, posts, &mu, failed)
	}

	var page []connectors.Posting
	if q.HasPay() {
		// A pay range is in the description, so descriptions are read in rank order, a chunk at a
		// time, until the page and one posting more are found or the scan cap is reached.
		var passing []connectors.Posting
		scanned := 0
		for scanned < len(ranked) && scanned < payScanCap && len(passing) < q.Offset+q.Limit+1 && ctx.Err() == nil {
			chunk := ranked[scanned:min(scanned+payChunk, len(ranked), payScanCap)]
			for _, p := range fill(chunk) {
				if p != nil && PayOK(p.PayMin, p.PayMax, q.PayMin, q.PayMax, q.RequirePay) {
					passing = append(passing, *p)
				}
			}
			scanned += len(chunk)
		}
		result.Matched = len(passing)
		result.Partial = scanned < len(ranked)
		start := min(q.Offset, len(passing))
		end := min(start+q.Limit, len(passing))
		page = passing[start:end]
		result.HasMore = end < len(passing) || result.Partial
		result.NextOffset = end
		result.LocationMatched = min(result.LocationMatched, len(passing)) // an upper bound, shown only as a count
	} else {
		start := min(q.Offset, len(ranked))
		end := min(start+q.Limit, len(ranked))
		for _, p := range fill(ranked[start:end]) {
			if p != nil {
				page = append(page, *p)
			}
		}
		result.HasMore = end < len(ranked)
		result.NextOffset = end
	}
	result.Postings = append(result.Postings, page...)

	for board := range failed {
		result.Errors = append(result.Errors, connectors.SearchError{Board: board, Message: failed[board]})
	}
	sort.Slice(result.Errors, func(i, j int) bool { return result.Errors[i].Board < result.Errors[j].Board })
	if result.BoardsSearched == 0 && len(boards) > 0 {
		return result, fmt.Errorf("%w: every board failed", ErrorUpstream)
	}
	return result, nil
}

// foldDuplicates keeps the first posting of each company and title and counts the others in
// Duplicates. The input is ranked, so the kept copy is the best one.
func foldDuplicates(ranked []connectors.Posting) []connectors.Posting {
	index := map[string]int{}
	out := make([]connectors.Posting, 0, len(ranked))
	for _, p := range ranked {
		key := p.Board + "\x00" + strings.Join(titleFields(p.PositionTitle), " ")
		if at, ok := index[key]; ok {
			out[at].Duplicates++
			continue
		}
		index[key] = len(out)
		out = append(out, p)
	}
	return out
}

// fill fetches the description of each posting, at most fetchConcurrency at a time, and returns the
// postings in the same order with nil where a fetch failed. A failure lists the posting's board.
func (c *Client) fill(ctx context.Context, posts []connectors.Posting, mu *sync.Mutex, failed map[string]string) []*connectors.Posting {
	full := make([]*connectors.Posting, len(posts))
	sem := make(chan struct{}, fetchConcurrency)
	var wg sync.WaitGroup
	for i, p := range posts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
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
			if low, high, ok := ParsePay(detail.RawText); ok {
				p.PayMin, p.PayMax = low, high
			}
			// A posting whose location says nothing may still say in its text that the role is remote or hybrid.
			if p.WorkMode == connectors.WorkModeOnSite {
				if mode := DescribedMode(detail.RawText); mode != "" {
					p.WorkMode = mode
				}
			}
			full[i] = &p
		}()
	}
	wg.Wait()
	return full
}

// fetchCached is FetchJob through the 24 hour description cache.
func (c *Client) fetchCached(ctx context.Context, board, id string) (connectors.Posting, error) {
	cache := c.cache()
	if p, ok := cache.description(board, id); ok {
		return p, nil
	}
	p, err := c.FetchJob(ctx, board, id)
	if err != nil {
		return connectors.Posting{}, err
	}
	cache.putDescription(board, id, p)
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
