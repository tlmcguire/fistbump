// Package lever reads pasted Lever job links and fetches postings from Lever's public Postings API.
package lever

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/textclean"
)

const (
	defaultHost = "api.lever.co"
	boardHost   = "jobs.lever.co"
	userAgent   = "fistbump/0.4 (local job application assistant)"
	timeout     = 10 * time.Second
	maxBody     = 4 << 20
)

var (
	ErrorNotLever    = errors.New("Not a Lever job link")
	ErrorBadLink     = errors.New("Not a Lever job posting link")
	ErrorNotFound    = errors.New("Job not found on Lever")
	ErrorUpstream    = errors.New("Lever request failed")
	ErrorRateLimited = errors.New("Lever is limiting requests")
)

var (
	boardRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	jobIDRe = regexp.MustCompile(`^[A-Za-z0-9-]{6,64}$`)
)

// ParseLink parses a Lever job URL such as jobs.lever.co/{board}/{posting_id} (with optional /apply).
func ParseLink(raw string) (board, jobID string, err error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return "", "", ErrorNotLever
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host != boardHost && host != "jobs.eu.lever.co" {
		return "", "", ErrorNotLever
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) < 2 || len(parts) > 3 || (len(parts) == 3 && parts[2] != "apply") {
		return "", "", ErrorBadLink
	}
	if !boardRe.MatchString(parts[0]) || !jobIDRe.MatchString(parts[1]) {
		return "", "", ErrorBadLink
	}
	return strings.ToLower(parts[0]), parts[1], nil
}

type cacheEntry struct {
	postings  []connectors.Posting
	err       error
	fetchedAt time.Time
	expires   time.Time
}

// Client fetches postings from Lever's public Postings API.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	MaxBody int64

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

func NewClient() *Client {
	return &Client{
		BaseURL: (&url.URL{Scheme: "https", Host: defaultHost}).String(),
		HTTP:    &http.Client{Timeout: timeout},
	}
}

type rawPosting struct {
	ID               string `json:"id"`
	Text             string `json:"text"`
	HostedURL        string `json:"hostedUrl"`
	ApplyURL         string `json:"applyUrl"`
	CreatedAt        int64  `json:"createdAt"` // epoch milliseconds
	DescriptionPlain string `json:"descriptionPlain"`
	Description      string `json:"description"`
	AdditionalPlain  string `json:"additionalPlain"`
	Additional       string `json:"additional"`
	Lists            []struct {
		Text    string `json:"text"`
		Content string `json:"content"`
	} `json:"lists"`
	Categories struct {
		Location   string `json:"location"`
		Team       string `json:"team"`
		Department string `json:"department"`
		Commitment string `json:"commitment"`
	} `json:"categories"`
	WorkplaceType string `json:"workplaceType"`
	SalaryRange   *struct {
		Min      float64 `json:"min"`
		Max      float64 `json:"max"`
		Currency string  `json:"currency"`
		Interval string  `json:"interval"`
	} `json:"salaryRange"`
}

func (c *Client) base() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return (&url.URL{Scheme: "https", Host: defaultHost}).String()
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: timeout}
}

func (c *Client) limit() int64 {
	if c.MaxBody > 0 {
		return c.MaxBody
	}
	return maxBody
}

// FetchJob retrieves a single Lever posting by board token and posting ID.
func (c *Client) FetchJob(ctx context.Context, board, jobID string) (connectors.Posting, error) {
	if !boardRe.MatchString(board) || !jobIDRe.MatchString(jobID) {
		return connectors.Posting{}, ErrorBadLink
	}
	endpoint := c.base() + "/v0/postings/" + url.PathEscape(board) + "/" + url.PathEscape(jobID) + "?mode=json"
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return connectors.Posting{}, err
	}
	var raw rawPosting
	if err := json.Unmarshal(body, &raw); err != nil {
		return connectors.Posting{}, fmt.Errorf("%w: decoding response: %v", ErrorUpstream, err)
	}
	return toPosting(board, raw), nil
}

// ListBoard retrieves all open postings for a Lever board (cached in memory for 6 hours).
func (c *Client) ListBoard(ctx context.Context, board string) ([]connectors.Posting, error) {
	if !boardRe.MatchString(board) {
		return nil, ErrorBadLink
	}
	if postings, err, found := c.CachedBoard(board); found {
		return postings, err
	}
	key := strings.ToLower(board)

	endpoint := c.base() + "/v0/postings/" + url.PathEscape(board) + "?mode=json"
	body, err := c.get(ctx, endpoint)
	if err != nil {
		if errors.Is(err, ErrorNotFound) {
			c.storeCache(key, nil, ErrorNotFound, 30*time.Minute)
		}
		return nil, err
	}
	var raws []rawPosting
	if err := json.Unmarshal(body, &raws); err != nil {
		return nil, fmt.Errorf("%w: decoding response: %v", ErrorUpstream, err)
	}
	out := make([]connectors.Posting, 0, len(raws))
	for _, raw := range raws {
		out = append(out, toPosting(board, raw))
	}
	c.storeCache(key, out, nil, 6*time.Hour)
	return out, nil
}

// CachedBoard returns a fresh in-memory entry without making a network request.
func (c *Client) CachedBoard(board string) ([]connectors.Posting, error, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.cache[strings.ToLower(board)]
	if !ok || !time.Now().Before(entry.expires) {
		return nil, nil, false
	}
	return entry.postings, entry.err, true
}

func (c *Client) storeCache(key string, postings []connectors.Posting, err error, ttl time.Duration) {
	c.mu.Lock()
	if c.cache == nil {
		c.cache = make(map[string]cacheEntry)
	}
	now := time.Now()
	c.cache[key] = cacheEntry{postings: postings, err: err, fetchedAt: now, expires: now.Add(ttl)}
	c.mu.Unlock()
}

// BoardsStaleBefore returns successfully searched boards whose cached listings are older than cutoff.
func (c *Client) BoardsStaleBefore(cutoff time.Time) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var boards []string
	for board, entry := range c.cache {
		if entry.err == nil && entry.fetchedAt.Before(cutoff) {
			boards = append(boards, board)
		}
	}
	sort.Strings(boards)
	return boards
}

// Clear removes the in-memory listing cache.
func (c *Client) Clear() error {
	c.mu.Lock()
	c.cache = nil
	c.mu.Unlock()
	return nil
}

func (c *Client) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrorUpstream, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrorUpstream, err)
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusNotFound:
		return nil, ErrorNotFound
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("%w: %w", ErrorUpstream, ErrorRateLimited)
	case http.StatusOK:
	default:
		return nil, fmt.Errorf("%w: HTTP %d", ErrorUpstream, res.StatusCode)
	}
	lim := c.limit()
	body, err := io.ReadAll(io.LimitReader(res.Body, lim+1))
	if err != nil {
		return nil, fmt.Errorf("%w: reading response: %v", ErrorUpstream, err)
	}
	if int64(len(body)) > lim {
		return nil, fmt.Errorf("%w: response too large", ErrorUpstream)
	}
	return body, nil
}

func toPosting(board string, raw rawPosting) connectors.Posting {
	var parts []string
	if t := strings.TrimSpace(raw.DescriptionPlain); t != "" {
		parts = append(parts, t)
	} else if t := stripHTML(raw.Description); t != "" {
		parts = append(parts, t)
	}
	for _, list := range raw.Lists {
		heading := strings.TrimSpace(list.Text)
		content := stripHTML(list.Content)
		if heading != "" && content != "" {
			parts = append(parts, heading+"\n"+content)
		} else if content != "" {
			parts = append(parts, content)
		}
	}
	if t := strings.TrimSpace(raw.AdditionalPlain); t != "" {
		parts = append(parts, t)
	} else if t := stripHTML(raw.Additional); t != "" {
		parts = append(parts, t)
	}

	rawText, invisible := textclean.StripInvisible(strings.Join(parts, "\n\n"))
	title, n1 := textclean.StripInvisible(strings.TrimSpace(raw.Text))
	loc, n2 := textclean.StripInvisible(strings.TrimSpace(raw.Categories.Location))
	invisible += n1 + n2

	var updated time.Time
	if raw.CreatedAt > 0 {
		updated = time.UnixMilli(raw.CreatedAt).UTC()
	}

	listingURL := strings.TrimSpace(raw.HostedURL)
	if !strings.HasPrefix(listingURL, "https://") {
		listingURL = (&url.URL{Scheme: "https", Host: boardHost, Path: "/" + board + "/" + raw.ID}).String()
	}

	workMode := normalizeWorkplace(raw.WorkplaceType, loc, title)
	payMin, payMax := 0, 0
	if raw.SalaryRange != nil && (raw.SalaryRange.Currency == "" || strings.EqualFold(raw.SalaryRange.Currency, "USD")) {
		interval := strings.ToLower(raw.SalaryRange.Interval)
		if interval == "" || strings.Contains(interval, "year") || interval == "per-year-salary" {
			payMin = int(raw.SalaryRange.Min)
			payMax = int(raw.SalaryRange.Max)
		}
	}

	return connectors.Posting{
		Source:        "lever",
		ExternalID:    raw.ID,
		Board:         board,
		CompanyName:   prettyCompany(board),
		PositionTitle: title,
		Location:      loc,
		ListingURL:    listingURL,
		UpdatedAt:     updated,
		RawText:       strings.TrimSpace(rawText),
		WorkMode:      workMode,
		PayMin:        payMin,
		PayMax:        payMax,
		Dropped:       connectors.Dropped{InvisibleChars: invisible},
	}
}

func normalizeWorkplace(workplace, loc, title string) string {
	w := strings.ToLower(strings.TrimSpace(workplace))
	switch w {
	case "remote":
		return connectors.WorkModeRemote
	case "hybrid":
		return connectors.WorkModeHybrid
	case "on-site", "onsite":
		return connectors.WorkModeOnSite
	}
	combined := strings.ToLower(loc + " " + title)
	switch {
	case strings.Contains(combined, "remote"):
		return connectors.WorkModeRemote
	case strings.Contains(combined, "hybrid"):
		return connectors.WorkModeHybrid
	default:
		return connectors.WorkModeOnSite
	}
}

func prettyCompany(board string) string {
	if name := CompanyFor(board); name != "" {
		return name
	}
	words := strings.FieldsFunc(board, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	if len(words) == 0 {
		return board
	}
	return strings.Join(words, " ")
}

func stripHTML(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	src = html.UnescapeString(src)
	nodes, err := xhtml.ParseFragment(strings.NewReader(src), &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return ""
	}
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		switch n.Type {
		case xhtml.TextNode:
			b.WriteString(n.Data)
		case xhtml.ElementNode:
			switch n.DataAtom {
			case atom.Script, atom.Style, atom.Svg, atom.Noscript:
				return
			case atom.Br:
				b.WriteByte('\n')
				return
			case atom.Li:
				if b.Len() > 0 && b.String()[b.Len()-1] != '\n' {
					b.WriteByte('\n')
				}
				b.WriteString("- ")
			case atom.P, atom.Div, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
				if b.Len() > 0 && b.String()[b.Len()-1] != '\n' {
					b.WriteByte('\n')
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			switch n.DataAtom {
			case atom.P, atom.Li, atom.Div, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
				if b.Len() > 0 && b.String()[b.Len()-1] != '\n' {
					b.WriteByte('\n')
				}
			}
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	return strings.TrimSpace(b.String())
}
