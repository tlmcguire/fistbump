package greenhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/textclean"
)

// maxListBody caps a board listing; boards are larger than one job.
const maxListBody = 8 << 20

// Listing is one posting in a board's listing, without its description.
type Listing struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Location    string    `json:"location"`
	UpdatedAt   time.Time `json:"updated_at"`
	AbsoluteURL string    `json:"absolute_url"`
}

type rawListing struct {
	Jobs []rawJob `json:"jobs"`
}

// ListBoard lists a board's postings without descriptions, through the cache: a listing under 6
// hours old is used as is; an older one is revalidated with If-None-Match.
func (c *Client) ListBoard(ctx context.Context, board string) ([]Listing, error) {
	if !tokenRe.MatchString(board) {
		return nil, ErrorBadLink
	}
	cache := c.cache()
	cached, have := cache.listing(board)
	if have && cache.fresh(cached) {
		return cached.result(board)
	}

	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
	}
	endpoint := strings.TrimRight(base, "/") + "/v1/boards/" + url.PathEscape(board) + "/jobs"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrorUpstream, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if have && cached.ETag != "" && !cached.NotFound {
		req.Header.Set("If-None-Match", cached.ETag)
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrorUpstream, err)
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusNotModified && have && !cached.NotFound:
		cached.FetchedAt = cache.now()
		cache.putListing(cached)
		return cached.Jobs, nil
	case res.StatusCode == http.StatusNotFound:
		cache.putListing(cachedListing{Board: board, FetchedAt: cache.now(), NotFound: true})
		return nil, fmt.Errorf("%w: %s", ErrorNotFound, board)
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: HTTP %d", ErrorUpstream, res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxListBody+1))
	if err != nil {
		return nil, fmt.Errorf("%w: reading response: %v", ErrorUpstream, err)
	}
	if len(body) > maxListBody {
		return nil, fmt.Errorf("%w: response larger than %d bytes", ErrorUpstream, maxListBody)
	}
	var raw rawListing
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: decoding response: %v", ErrorUpstream, err)
	}
	jobs := make([]Listing, 0, len(raw.Jobs))
	for _, r := range raw.Jobs {
		var updated time.Time
		if r.UpdatedAt != "" {
			if t, err := time.Parse(time.RFC3339, r.UpdatedAt); err == nil {
				updated = t.UTC()
			}
		}
		title, _ := textclean.StripInvisible(r.Title)
		loc, _ := textclean.StripInvisible(r.Location.Name)
		jobs = append(jobs, Listing{ID: r.ID, Title: strings.TrimSpace(title), Location: strings.TrimSpace(loc), UpdatedAt: updated, AbsoluteURL: r.AbsoluteURL})
	}
	cache.putListing(cachedListing{Board: board, ETag: res.Header.Get("ETag"), FetchedAt: cache.now(), Jobs: jobs})
	return jobs, nil
}

func (e cachedListing) result(board string) ([]Listing, error) {
	if e.NotFound {
		return nil, fmt.Errorf("%w: %s", ErrorNotFound, board)
	}
	return e.Jobs, nil
}

// cache returns the client's cache, creating a memory-only one on first use.
func (c *Client) cache() *Cache {
	c.cacheOnce.Do(func() {
		if c.Cache == nil {
			c.Cache = NewCache("")
		}
	})
	return c.Cache
}

// listingPosting builds the part of a Posting a listing knows, using the same link rules as FetchJob.
func listingPosting(board string, l Listing) connectors.Posting {
	id := fmt.Sprint(l.ID)
	return connectors.Posting{
		ExternalID:    id,
		Board:         board,
		CompanyName:   companyName(board, ""),
		PositionTitle: l.Title,
		Location:      l.Location,
		ListingURL:    listingBase + "/" + board + "/jobs/" + id,
		CompanyURL:    companyURL(board, l.AbsoluteURL),
		UpdatedAt:     l.UpdatedAt,
	}
}

// companyName is the curated name, else the one Greenhouse sent, else the board token.
func companyName(board, sent string) string {
	if c := CompanyFor(board); c != "" {
		return c
	}
	if sent = strings.TrimSpace(sent); sent != "" {
		return sent
	}
	return board
}
