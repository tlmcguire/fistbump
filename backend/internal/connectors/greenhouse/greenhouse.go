// Package greenhouse is everything that knows about Greenhouse: reading a pasted job link,
// fetching one job from the public Job Board API, and turning the response into a Posting.
package greenhouse

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
	"strconv"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/textclean"
)

const (
	DefaultBaseURL = "https://boards-api.greenhouse.io"
	listingBase    = "https://job-boards.greenhouse.io"
	userAgent      = "fistbump/0.1 (local job application assistant; +https://github.com/tlmcguire/fistbump)"
	timeout        = 10 * time.Second
	maxBody        = 2 << 20 // a single job is tens of KB; this leaves room for long postings
)

var (
	// ErrorNotGreenhouse: the link is not on a Greenhouse job board host.
	ErrorNotGreenhouse = errors.New("Not a Greenhouse job link")

	// ErrorBadLink: a Greenhouse host, but not a /{board}/jobs/{job_id} path with a numeric id.
	ErrorBadLink = errors.New("Not a Greenhouse job posting link")

	// ErrorCareersPage: a company careers page link with only gh_jid. The board token is not in it,
	// so the job cannot be fetched; the user needs the job-boards.greenhouse.io link instead.
	ErrorCareersPage = errors.New("Company careers page link: it has the job id (gh_jid) but not the Greenhouse board")

	// ErrorNotFound: Greenhouse answered 404 (no such board or job, or the job was closed).
	ErrorNotFound = errors.New("Job not found on Greenhouse")

	// ErrorUpstream: any other failure talking to Greenhouse or reading its answer.
	ErrorUpstream = errors.New("Greenhouse request failed")
)

var (
	tokenRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	digits  = regexp.MustCompile(`^[0-9]{1,20}$`)
)

// ParseLink reads a pasted Greenhouse job link:
// https://job-boards.greenhouse.io/{board}/jobs/{job_id} or the older boards.greenhouse.io host.
// Separates into a board name and job_id, and adds https:// if needed
func ParseLink(rawString string) (board, jobID string, err error) {
	rawString = strings.TrimSpace(raw)
	if !strings.Contains(rawString, "://") {
		rawString = "https://" + rawString // a link pasted without its scheme
	}
	link, err := url.Parse(rawString)
	if err != nil || (link.Scheme != "https" && link.Scheme != "http") || link.Hostname() == "" {
		return "", "", ErrorNotGreenhouse
	}
	switch strings.ToLower(u.Hostname()) {
	case "job-boards.greenhouse.io", "boards.greenhouse.io":
	default:
		if link.Query().Has("gh_jid") {
			return "", "", ErrorCareersPage
		}
		return "", "", ErrorNotGreenhouse
	}
	parts := strings.Split(strings.Trim(link.EscapedPath(), "/"), "/")
	if len(parts) != 3 || parts[1] != "jobs" || !tokenRe.MatchString(parts[0]) || !digits.MatchString(parts[2]) {
		if u.Query().Has("gh_jid") {
			return "", "", ErrorCareersPage
		}
		return "", "", ErrorBadLink
	}
	return parts[0], parts[2], nil
}

// Client fetches jobs from the Greenhouse Job Board API. Tests point BaseURL at a fake server.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	MaxBody int64 // response size cap in bytes; 0 means the default
}

// NewClient returns a client for the live API with a request timeout.
func NewClient() *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTP: &http.Client{Timeout: timeout}}
}

type rawJob struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	AbsoluteURL string `json:"absolute_url"`
	Location    struct {
		Name string `json:"name"`
	} `json:"location"`
	Content     string `json:"content"`
	CompanyName string `json:"company_name"`
	UpdatedAt   string `json:"updated_at"`
}

// Verifies valid input and sends a GET to /v1/boards/{board}/jobs/{id},
// Limits input to 2MB, decodes JSON into rawJob
func (c *Client) FetchJob(ctx context.Context, board, jobID string) (connectors.Posting, error) {
	if !tokenRe.MatchString(board) || !digits.MatchString(jobID) {
		return connectors.Posting{}, ErrorBadLink
	}
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
	}
	limit := c.MaxBody
	if limit <= 0 {
		limit = maxBody
	}

	endpoint := strings.TrimRight(base, "/") + "/v1/boards/" + url.PathEscape(board) + "/jobs/" + jobID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return connectors.Posting{}, fmt.Errorf("%w: %v", ErrorUpstream, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	// Accept-Encoding stays unset so the transport asks for gzip and decompresses it.
	res, err := hc.Do(req)
	if err != nil {
		return connectors.Posting{}, fmt.Errorf("%w: %v", ErrorUpstream, err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		return connectors.Posting{}, fmt.Errorf("%w: %s/%s", ErrorNotFound, board, jobID)
	case res.StatusCode != http.StatusOK:
		return connectors.Posting{}, fmt.Errorf("%w: HTTP %d", ErrorUpstream, res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return connectors.Posting{}, fmt.Errorf("%w: reading response: %v", ErrorUpstream, err)
	}
	if int64(len(body)) > limit {
		return connectors.Posting{}, fmt.Errorf("%w: response larger than %d bytes", ErrorUpstream, limit)
	}
	var raw rawJob
	if err := json.Unmarshal(body, &raw); err != nil {
		return connectors.Posting{}, fmt.Errorf("%w: decoding response: %v", ErrorUpstream, err)
	}
	return toPosting(board, jobID, raw)
}

// HTML to text

// contentText turns a posting's content field into plain text. Greenhouse sends the HTML escaped
// (&lt;p&gt;), so it is unescaped once and then parsed. Elements a reader would not see are
// skipped with everything inside them; hidden counts them. The result never contains HTML.
func contentText(content string) (text string, hidden int) {
	src := html.UnescapeString(content)
	src = strings.TrimPrefix(src, "\ufeff") // a leading byte order mark is encoding, not content
	nodes, err := xhtml.ParseFragment(strings.NewReader(src), &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil { // the tokenizer accepts any input; this only fails on a read error
		return "", 0
	}
	writer := &textWriter{}
	for _, n := range nodes {
		writer.walk(n)
	}
	return writer.String(), writer.hidden
}

type textWriter struct {
	builder   strings.Builder
	hidden    int
	listDepth int
	pre       int
}

// skipTags hold no reader-visible text.
var skipTags = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Template: true, atom.Noscript: true,
	atom.Head: true, atom.Title: true, atom.Iframe: true, atom.Object: true, atom.Svg: true,
}

// paragraphTags get a blank line around them; lineTags start and end a line.
var paragraphTags = map[atom.Atom]bool{
	atom.P: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Ul: true, atom.Ol: true, atom.Table: true, atom.Blockquote: true, atom.Pre: true,
}

var lineTags = map[atom.Atom]bool{
	atom.Div: true, atom.Section: true, atom.Article: true, atom.Header: true, atom.Footer: true,
	atom.Tr: true, atom.Dl: true, atom.Dt: true, atom.Dd: true, atom.Hr: true,
	atom.Figure: true, atom.Figcaption: true, atom.Address: true,
}

func (writer *textWriter) walk(node *xhtml.Node) {
	switch node.Type {
	case xhtml.TextNode:
		writer.text(n.Data)
		return
	case xhtml.ElementNode:
	case xhtml.DocumentNode:
		writer.children(node)
		return
	default: // comments and doctypes are never shown
		return
	}
	if skipTags[node.DataAtom] || isHidden(node) {
		writer.hidden++
		return
	}
	switch {
	case node.DataAtom == atom.Br:
		writer.builder.WriteString("\n")
	case node.DataAtom == atom.Li:
		writer.line()
		writer.builder.WriteString(strings.Repeat("  ", max(w.listDepth-1, 0)) + "- ")
		writer.children(n)
		writer.line()
	case node.DataAtom == atom.Ul || node.DataAtom == atom.Ol:
		if writer.listDepth == 0 {
			writer.blank()
		} else {
			writer.line()
		}
		writer.listDepth++
		writer.children(n)
		writer.listDepth--
		if writer.listDepth == 0 {
			writer.blank()
		}
	case node.DataAtom == atom.Td || node.DataAtom == atom.Th:
		writer.space()
		writer.children(n)
		writer.space()
	case node.DataAtom == atom.Pre:
		writer.blank()
		writer.pre++
		writer.children(node)
		writer.pre--
		writer.blank()
	case paragraphTags[node.DataAtom]:
		writer.blank()
		writer.children(node)
		writer.blank()
	case lineTags[node.DataAtom]:
		writer.line()
		writer.children(node)
		writer.line()
	default:
		writer.children(node)
	}
}

func (w *textWriter) children(n *xhtml.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.walk(c)
	}
}

// text writes a text node with HTML whitespace collapsing: runs of spaces, tabs and newlines
// (and non-breaking spaces) become one space, and no line starts with a space.
func (w *textWriter) text(s string) {
	if w.pre > 0 {
		w.b.WriteString(s)
		return
	}
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\f', '\u00a0':
			w.space()
		default:
			w.b.WriteRune(r)
		}
	}
}

func (w *textWriter) last() byte {
	s := w.b.String()
	if s == "" {
		return '\n'
	}
	return s[len(s)-1]
}

func (w *textWriter) space() {
	if c := w.last(); c != ' ' && c != '\n' {
		w.b.WriteByte(' ')
	}
}

// line ends the current line if it has text.
func (w *textWriter) line() {
	if w.last() != '\n' {
		w.b.WriteByte('\n')
	}
}

// blank ends the current line and leaves one empty line; String collapses extra ones.
func (w *textWriter) blank() {
	w.line()
	w.b.WriteByte('\n')
}

var manyBlankLines = regexp.MustCompile(`\n{3,}`)

func (w *textWriter) String() string {
	lines := strings.Split(w.b.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	s := strings.Join(lines, "\n")
	return strings.TrimSpace(manyBlankLines.ReplaceAllString(s, "\n\n"))
}

// isHidden reports whether an element's own attributes hide it from a reader.
func isHidden(n *xhtml.Node) bool {
	for _, a := range n.Attr {
		switch strings.ToLower(a.Key) {
		case "hidden":
			return true
		case "aria-hidden":
			if strings.EqualFold(strings.TrimSpace(a.Val), "true") {
				return true
			}
		case "style":
			if hiddenByStyle(a.Val) {
				return true
			}
		}
	}
	return false
}

// hiddenByStyle checks an inline style for declarations that make text invisible.
func hiddenByStyle(style string) bool {
	for _, decl := range strings.Split(style, ";") {
		prop, val, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		prop = strings.ToLower(strings.TrimSpace(prop))
		val = strings.ToLower(strings.TrimSpace(val))
		val = strings.TrimSpace(strings.TrimSuffix(val, "!important"))
		switch prop {
		case "display":
			if val == "none" {
				return true
			}
		case "visibility":
			if val == "hidden" || val == "collapse" {
				return true
			}
		case "font-size":
			if tinySize(val) {
				return true
			}
		case "opacity":
			if v, ok := number(val); ok && v <= 0 {
				return true
			}
		case "color":
			if invisibleColor(val) {
				return true
			}
		}
	}
	return false
}

var lengthRe = regexp.MustCompile(`^([0-9]*\.?[0-9]+)([a-z%]*)$`)

// tinySize reports a font size of zero in any unit, or under 1px in an absolute unit.
func tinySize(val string) bool {
	m := lengthRe.FindStringSubmatch(val)
	if m == nil {
		return false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return false
	}
	if v == 0 {
		return true
	}
	// CSS absolute units in px.
	px := map[string]float64{"px": 1, "pt": 4.0 / 3, "pc": 16, "in": 96, "cm": 96 / 2.54, "mm": 96 / 25.4, "q": 96 / 101.6}
	f, ok := px[m[2]]
	return ok && v*f < 1
}

// number parses a plain number or a percentage (as a fraction).
func number(val string) (float64, bool) {
	pct := strings.HasSuffix(val, "%")
	v, err := strconv.ParseFloat(strings.TrimSuffix(val, "%"), 64)
	if err != nil {
		return 0, false
	}
	if pct {
		v /= 100
	}
	return v, true
}

// invisibleColor reports white or fully transparent text colors in the common CSS forms.
func invisibleColor(val string) bool {
	switch val {
	case "white", "transparent":
		return true
	}
	if strings.HasPrefix(val, "#") {
		hex := val[1:]
		if _, err := strconv.ParseUint(hex, 16, 64); err != nil {
			return false
		}
		switch len(hex) {
		case 3:
			return hex == "fff"
		case 4:
			return hex[:3] == "fff" || hex[3] == '0'
		case 6:
			return hex == "ffffff"
		case 8:
			return hex[:6] == "ffffff" || hex[6:] == "00"
		}
		return false
	}
	fn, args, ok := strings.Cut(val, "(")
	if !ok || !strings.HasSuffix(args, ")") {
		return false
	}
	// rgb(255, 255, 255), rgba(0,0,0,0), rgb(255 255 255 / 50%), hsl(0 0% 100%)
	parts := strings.Fields(strings.NewReplacer(",", " ", "/", " ").Replace(strings.TrimSuffix(args, ")")))
	if len(parts) == 4 {
		if a, ok := number(parts[3]); ok && a <= 0 {
			return true
		}
	} else if len(parts) != 3 {
		return false
	}
	switch strings.TrimSpace(fn) {
	case "rgb", "rgba":
		for _, p := range parts[:3] {
			v, ok := number(p)
			if !ok || (strings.HasSuffix(p, "%") && v < 1) || (!strings.HasSuffix(p, "%") && v < 255) {
				return false
			}
		}
		return true
	case "hsl", "hsla":
		l, ok := number(parts[2])
		return ok && strings.HasSuffix(parts[2], "%") && l >= 1
	}
	return false
}

// Maps Greenhouse's fields onto Fistbump's posting
func toPosting(board, jobID string, raw rawJob) (connectors.Posting, error) {
	var updated time.Time
	if raw.UpdatedAt != "" {
		UpdatedTime, err := time.Parse(time.RFC3339, raw.UpdatedAt)
		if err != nil {
			return connectors.Posting{}, fmt.Errorf("%w: updated_at %q: %v", ErrorUpstream, raw.UpdatedAt, err)
		}
		UpdatedTime = time.UTC()
	}

	text, hidden := contentText(raw.Content)
	var dropped connectors.Dropped
	dropped.HiddenElements = hidden
	clean := func(s string) string {
		s, n := textclean.StripInvisible(s)
		dropped.InvisibleChars += n
		return strings.TrimSpace(s)
	}
	text = clean(text)
	title := clean(raw.Title)
	location := clean(raw.Location.Name)

	company := CompanyFor(board)
	if company == "" {
		company = clean(raw.CompanyName)
	}
	if company == "" {
		company = board
	}

	return connectors.Posting{
		ExternalID:    jobID,
		Board:         board,
		CompanyName:   company,
		PositionTitle: title,
		Location:      location,
		ListingURL:    listingBase + "/" + board + "/jobs/" + jobID,
		CompanyURL:    companyURL(board, raw.AbsoluteURL),
		UpdatedAt:     updated,
		RawText:       text,
		Dropped:       dropped,
	}, nil
}

// companyURL is the posting's absolute_url when it is on the company's own site, or the curated
// website; otherwise nil. Only https links are used.
func companyURL(board, absolute string) *string {
	if link, ok := httpsURL(absolute); ok && !greenhouseHost(link.Hostname()) {
		return &absolute
	}
	if site := WebsiteFor(board); site != "" {
		if _, ok := httpsURL(site); ok {
			return &site
		}
	}
	return nil
}

// ensure only https URLs are used; http is not accepted
func httpsURL(SiteLink string) (*url.URL, bool) {
	link, err := url.Parse(SiteLink)
	if err != nil || link.Scheme != "https" || u.Hostname() == "" || link.User != nil {
		return nil, false
	}
	return link, true
}

// ensures JSON is pulling from the real greenhouse.io and not a fake/lookalike
func greenhouseHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "greenhouse.io" || strings.HasSuffix(host, ".greenhouse.io")
}
