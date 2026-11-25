// Package ashby reads pasted Ashby job links and fetches postings from Ashby's public Job Board API.
package ashby

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
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/textclean"
)

const (
	defaultHost = "api.ashbyhq.com"
	boardHost   = "jobs.ashbyhq.com"
	userAgent   = "fistbump/0.4 (local job application assistant)"
	timeout     = 10 * time.Second
	maxBody     = 4 << 20
)

var (
	ErrorNotAshby    = errors.New("Not an Ashby job link")
	ErrorBadLink     = errors.New("Not an Ashby job posting link")
	ErrorNotFound    = errors.New("Job not found on Ashby")
	ErrorUpstream    = errors.New("Ashby request failed")
	ErrorRateLimited = errors.New("Ashby is limiting requests")
)

var (
	boardRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	jobIDRe = regexp.MustCompile(`^[A-Za-z0-9-]{6,64}$`)
)

// ParseLink parses an Ashby job URL such as jobs.ashbyhq.com/{board}/{job_id} (with optional /application).
func ParseLink(raw string) (board, jobID string, err error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return "", "", ErrorNotAshby
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host != boardHost {
		return "", "", ErrorNotAshby
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) < 2 || len(parts) > 3 || (len(parts) == 3 && parts[2] != "application") {
		return "", "", ErrorBadLink
	}
	if !boardRe.MatchString(parts[0]) || !jobIDRe.MatchString(parts[1]) {
		return "", "", ErrorBadLink
	}
	return parts[0], parts[1], nil
}

// Client fetches postings from Ashby's public Job Board API.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	MaxBody int64
}

func NewClient() *Client {
	return &Client{
		BaseURL: (&url.URL{Scheme: "https", Host: defaultHost}).String(),
		HTTP:    &http.Client{Timeout: timeout},
	}
}

type boardResponse struct {
	Jobs []rawJob `json:"jobs"`
}

type rawJob struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Location         string `json:"location"`
	IsRemote         bool   `json:"isRemote"`
	WorkplaceType    string `json:"workplaceType"`
	JobURL           string `json:"jobUrl"`
	PublishedAt      string `json:"publishedAt"`
	DescriptionPlain string `json:"descriptionPlain"`
	DescriptionHTML  string `json:"descriptionHtml"`
	Compensation     *struct {
		SummaryComponents []struct {
			MinValue     float64 `json:"minValue"`
			MaxValue     float64 `json:"maxValue"`
			CurrencyCode string  `json:"currencyCode"`
			Interval     string  `json:"interval"`
		} `json:"summaryComponents"`
	} `json:"compensation"`
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

// ListBoard retrieves all open postings for an Ashby job board.
func (c *Client) ListBoard(ctx context.Context, board string) ([]connectors.Posting, error) {
	if !boardRe.MatchString(board) {
		return nil, ErrorBadLink
	}
	endpoint := c.base() + "/posting-api/job-board/" + url.PathEscape(board) + "?includeCompensation=true"
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
	var parsed boardResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: decoding response: %v", ErrorUpstream, err)
	}
	out := make([]connectors.Posting, 0, len(parsed.Jobs))
	for _, j := range parsed.Jobs {
		out = append(out, toPosting(board, j))
	}
	return out, nil
}

// FetchJob retrieves a single Ashby posting by board and job ID.
func (c *Client) FetchJob(ctx context.Context, board, jobID string) (connectors.Posting, error) {
	if !boardRe.MatchString(board) || !jobIDRe.MatchString(jobID) {
		return connectors.Posting{}, ErrorBadLink
	}
	list, err := c.ListBoard(ctx, board)
	if err != nil {
		return connectors.Posting{}, err
	}
	for _, p := range list {
		if p.ExternalID == jobID {
			return p, nil
		}
	}
	return connectors.Posting{}, ErrorNotFound
}

func toPosting(board string, raw rawJob) connectors.Posting {
	text := strings.TrimSpace(raw.DescriptionPlain)
	if text == "" {
		text = stripHTML(raw.DescriptionHTML)
	}
	rawText, invisible := textclean.StripInvisible(text)
	title, n1 := textclean.StripInvisible(strings.TrimSpace(raw.Title))
	loc, n2 := textclean.StripInvisible(strings.TrimSpace(raw.Location))
	invisible += n1 + n2

	var updated time.Time
	if raw.PublishedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, raw.PublishedAt); err == nil {
			updated = parsed.UTC()
		}
	}

	listingURL := strings.TrimSpace(raw.JobURL)
	if !strings.HasPrefix(listingURL, "https://") {
		listingURL = (&url.URL{Scheme: "https", Host: boardHost, Path: "/" + board + "/" + raw.ID}).String()
	}

	workMode := normalizeWorkplace(raw.IsRemote, raw.WorkplaceType, loc, title)
	payMin, payMax := 0, 0
	if raw.Compensation != nil {
		for _, comp := range raw.Compensation.SummaryComponents {
			if (comp.CurrencyCode == "" || strings.EqualFold(comp.CurrencyCode, "USD")) &&
				(comp.Interval == "" || strings.Contains(strings.ToLower(comp.Interval), "year")) &&
				comp.MaxValue > 0 {
				payMin = int(comp.MinValue)
				payMax = int(comp.MaxValue)
				break
			}
		}
	}

	return connectors.Posting{
		Source:        "ashby",
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

func normalizeWorkplace(isRemote bool, workplace, loc, title string) string {
	if isRemote {
		return connectors.WorkModeRemote
	}
	w := strings.ToLower(strings.TrimSpace(workplace))
	switch w {
	case "remote":
		return connectors.WorkModeRemote
	case "hybrid":
		return connectors.WorkModeHybrid
	case "onsite", "on-site":
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
