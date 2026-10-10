// Package greenhouse implements the Greenhouse job board connector.
//
// Greenhouse has no search API. Search lists every selected board (title, location, id: cheap), keeps the
// listings in memory and on disk for a few hours, filters and scores titles locally, then fetches full
// posting text only for the best title matches so they can be ranked against the resume.
package greenhouse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/parser"
)

const (
	defaultBase     = "https://boards-api.greenhouse.io"
	listConcurrency = 24
	detailFetches   = 40
	maxResponse     = 25 << 20
	listingTTL      = 6 * time.Hour
	detailTTL       = 24 * time.Hour
	defaultLimit    = 100
	minTitleScore   = 0.67
)

var tokenRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// ValidToken reports whether s can be a board token (the path segment in boards.greenhouse.io/<token>).
func ValidToken(s string) bool { return tokenRe.MatchString(s) }

var ErrNoBoard = errors.New("board not found")

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

// listing is the cached, content-free view of one posting.
type listing struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Location  string `json:"location"`
	Company   string `json:"company"`
	UpdatedAt string `json:"updated_at"`
}

type boardEntry struct {
	At      time.Time `json:"fetched_at"`
	Jobs    []listing `json:"jobs"`
	Missing bool      `json:"missing"`
}

type detail struct {
	at   time.Time
	text string
}

// Connector fetches and searches Greenhouse boards.
type Connector struct {
	Base     string
	Client   *http.Client
	CacheDir string // optional; listings persist here across restarts

	mu      sync.Mutex
	boards  map[string]boardEntry
	details map[int64]detail
	seen    map[string]connectors.Posting // external id -> last search result, for import
	names   map[string]string
}

func New(cacheDir string) *Connector {
	c := &Connector{Base: defaultBase, Client: &http.Client{Timeout: 20 * time.Second}, CacheDir: cacheDir}
	c.ClearCache()
	return c
}

func (c *Connector) ID() string   { return "greenhouse" }
func (c *Connector) Name() string { return "Greenhouse" }

// ClearCache drops in-memory listings and details. Disk files are removed by the caller.
func (c *Connector) ClearCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.boards, c.details, c.seen, c.names = map[string]boardEntry{}, map[int64]detail{}, map[string]connectors.Posting{}, map[string]string{}
}

func (c *Connector) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNoBoard
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("greenhouse returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return err
	}
	if len(data) > maxResponse {
		return errors.New("response too large")
	}
	return json.Unmarshal(data, out)
}

// ValidateBoard checks a token with one request and returns the company name and open posting count.
func (c *Connector) ValidateBoard(ctx context.Context, token string) (company string, open int, err error) {
	if !ValidToken(token) {
		return "", 0, ErrNoBoard
	}
	var meta struct {
		Name string `json:"name"`
	}
	if err := c.get(ctx, "/v1/boards/"+url.PathEscape(token), &meta); err != nil {
		return "", 0, err
	}
	jobs, err := c.listBoard(ctx, token, true)
	if err != nil {
		return "", 0, err
	}
	c.mu.Lock()
	c.names[token] = meta.Name
	c.mu.Unlock()
	return meta.Name, len(jobs), nil
}

func (c *Connector) diskPath(token string) string {
	if c.CacheDir == "" {
		return ""
	}
	return filepath.Join(c.CacheDir, "greenhouse", strings.ToLower(token)+".json")
}

// listBoard returns a board's postings from memory, disk, or the API, in that order.
func (c *Connector) listBoard(ctx context.Context, token string, force bool) ([]listing, error) {
	if !force {
		c.mu.Lock()
		e, ok := c.boards[token]
		c.mu.Unlock()
		if ok && time.Since(e.At) < listingTTL {
			if e.Missing {
				return nil, ErrNoBoard
			}
			return e.Jobs, nil
		}
		if p := c.diskPath(token); p != "" {
			if data, err := os.ReadFile(p); err == nil {
				var de boardEntry
				if json.Unmarshal(data, &de) == nil && time.Since(de.At) < listingTTL {
					c.mu.Lock()
					c.boards[token] = de
					c.mu.Unlock()
					return de.Jobs, nil
				}
			}
		}
	}
	var out struct {
		Jobs []rawJob `json:"jobs"`
	}
	err := c.get(ctx, "/v1/boards/"+url.PathEscape(token)+"/jobs", &out)
	if errors.Is(err, ErrNoBoard) {
		c.mu.Lock()
		c.boards[token] = boardEntry{At: time.Now(), Missing: true}
		c.mu.Unlock()
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	e := boardEntry{At: time.Now(), Jobs: make([]listing, 0, len(out.Jobs))}
	for _, j := range out.Jobs {
		e.Jobs = append(e.Jobs, listing{ID: j.ID, Title: strings.TrimSpace(j.Title), URL: j.AbsoluteURL, Location: j.Location.Name, Company: j.CompanyName, UpdatedAt: j.UpdatedAt})
	}
	c.mu.Lock()
	c.boards[token] = e
	c.mu.Unlock()
	if p := c.diskPath(token); p != "" {
		if data, err := json.Marshal(e); err == nil && os.MkdirAll(filepath.Dir(p), 0o755) == nil {
			_ = os.WriteFile(p, data, 0o644)
		}
	}
	return e.Jobs, nil
}

// postingText returns the cleaned description of one posting, cached for a day.
func (c *Connector) postingText(ctx context.Context, token string, id int64) (string, error) {
	c.mu.Lock()
	d, ok := c.details[id]
	c.mu.Unlock()
	if ok && time.Since(d.at) < detailTTL {
		return d.text, nil
	}
	var j rawJob
	if err := c.get(ctx, fmt.Sprintf("/v1/boards/%s/jobs/%d", url.PathEscape(token), id), &j); err != nil {
		return "", err
	}
	text := parser.Clean(j.Content)
	c.mu.Lock()
	c.details[id] = detail{at: time.Now(), text: text}
	c.mu.Unlock()
	return text, nil
}

func (c *Connector) company(token string, l listing) string {
	if l.Company != "" {
		return l.Company
	}
	if n := CompanyFor(token); n != "" {
		return n
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if n := c.names[token]; n != "" {
		return n
	}
	return token
}

// FetchPostings searches the query's boards. A failing board is reported in Errors and does not fail the search.
func (c *Connector) FetchPostings(ctx context.Context, q connectors.Query) (connectors.Result, error) {
	res := connectors.Result{Postings: []connectors.Posting{}, Errors: []connectors.BoardError{}, BoardsTotal: len(q.Boards)}
	type fetched struct {
		token string
		jobs  []listing
		err   error
	}
	results := make(chan fetched, len(q.Boards))
	sem := make(chan struct{}, listConcurrency)
	var wg sync.WaitGroup
	for _, b := range q.Boards {
		wg.Add(1)
		go func(token string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			jobs, err := c.listBoard(ctx, token, false)
			results <- fetched{token, jobs, err}
		}(b)
	}
	wg.Wait()
	close(results)

	terms := compileKeywords(q.Keywords)
	home := newHome(q.HomeLocation)
	locTerms := splitTerms(q.Location)
	var cands []connectors.Posting
	for f := range results {
		if f.err != nil {
			msg := f.err.Error()
			if errors.Is(f.err, ErrNoBoard) {
				msg = "board not found (it may have moved to another system)"
			}
			res.Errors = append(res.Errors, connectors.BoardError{Board: f.token, Message: msg})
			continue
		}
		res.BoardsSearched++
		res.PostingsScanned += len(f.jobs)
		for _, l := range f.jobs {
			ts := titleScore(l.Title, terms)
			if len(terms) > 0 && ts < minTitleScore {
				continue
			}
			mode := parser.InferWorkMode(l.Location + " " + l.Title)
			if !modeOK(mode, q.WorkMode) || !locationOK(l.Location, mode, locTerms, q.WorkMode) {
				continue
			}
			if q.WorkMode != "" && q.WorkMode != "Any" && mode == q.WorkMode {
				ts += 0.05
			}
			ts += homeBoost(l.Location, home)
			cands = append(cands, connectors.Posting{
				ExternalID: fmt.Sprint(l.ID), Board: f.token, PositionTitle: l.Title, CompanyName: c.company(f.token, l),
				ListingURL: l.URL, Location: l.Location, WorkMode: mode, UpdatedAt: l.UpdatedAt, TitleScore: round2(ts),
			})
		}
	}
	res.Matched = len(cands)
	sort.Slice(res.Errors, func(i, k int) bool { return res.Errors[i].Board < res.Errors[k].Board })
	sort.SliceStable(cands, func(i, k int) bool {
		if cands[i].TitleScore != cands[k].TitleScore {
			return cands[i].TitleScore > cands[k].TitleScore
		}
		return cands[i].UpdatedAt > cands[k].UpdatedAt
	})

	// Fetch text for the best title matches and rank them by resume skill overlap.
	have := map[string]bool{}
	for _, s := range q.ResumeSkills {
		have[parser.Canon(s)] = true
	}
	n := min(len(cands), detailFetches)
	// One slow posting must not stall the search: each detail has its own timeout and the phase a deadline.
	phase, cancelPhase := context.WithTimeout(ctx, 8*time.Second)
	defer cancelPhase()
	var dwg sync.WaitGroup
	dsem := make(chan struct{}, 12)
	for i := 0; i < n; i++ {
		dwg.Add(1)
		go func(p *connectors.Posting) {
			defer dwg.Done()
			dsem <- struct{}{}
			defer func() { <-dsem }()
			var id int64
			fmt.Sscan(p.ExternalID, &id)
			dctx, cancel := context.WithTimeout(phase, 5*time.Second)
			defer cancel()
			if text, err := c.postingText(dctx, p.Board, id); err == nil {
				p.RawText = text
				p.SkillScore = skillScore(text, have)
			}
		}(&cands[i])
	}
	dwg.Wait()
	for i := range cands {
		p := &cands[i]
		p.Score = p.TitleScore
		if p.RawText != "" && len(have) > 0 {
			p.Score = round2(0.55*p.TitleScore + 0.45*p.SkillScore)
		} else if len(have) > 0 {
			p.Score = round2(0.55 * p.TitleScore) // unranked by skills: keep below enriched matches of equal title
		}
	}
	for i := range cands {
		cands[i].Score = max(0, min(cands[i].Score, 1))
	}
	sort.SliceStable(cands, func(i, k int) bool { return cands[i].Score > cands[k].Score })
	cands = diversify(cands, 3)
	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if len(cands) > limit {
		cands = cands[:limit]
	}
	c.mu.Lock()
	if len(c.seen) > 20000 {
		c.seen = map[string]connectors.Posting{}
	}
	for _, p := range cands {
		c.seen[p.ExternalID] = p
	}
	c.mu.Unlock()
	res.Postings = cands
	return res, nil
}

// diversify keeps score order but moves a company's postings after its first perCompany to the end,
// so one large employer with many similar openings does not fill the first page.
func diversify(ps []connectors.Posting, perCompany int) []connectors.Posting {
	seen := map[string]int{}
	out := make([]connectors.Posting, 0, len(ps))
	var later []connectors.Posting
	for _, p := range ps {
		k := strings.ToLower(p.CompanyName)
		if seen[k] < perCompany {
			seen[k]++
			out = append(out, p)
		} else {
			later = append(later, p)
		}
	}
	return append(out, later...)
}

// Lookup returns a posting from a recent search, with its full text fetched if needed.
func (c *Connector) Lookup(ctx context.Context, externalID string) (connectors.Posting, error) {
	c.mu.Lock()
	p, ok := c.seen[externalID]
	c.mu.Unlock()
	if !ok {
		return p, fmt.Errorf("posting %s is not in recent search results; search again", externalID)
	}
	if p.RawText == "" {
		var id int64
		fmt.Sscan(externalID, &id)
		text, err := c.postingText(ctx, p.Board, id)
		if err != nil {
			return p, err
		}
		p.RawText = text
	}
	return p, nil
}

// ---- title matching ----

var (
	nonWord  = regexp.MustCompile(`[^a-z0-9+#]+`)
	phrases  = [][2]string{{"front end", "frontend"}, {"back end", "backend"}, {"full stack", "fullstack"}, {"machine learning", "ml"}, {"site reliability", "sre"}, {"dev ops", "devops"}, {"user experience", "ux"}, {"quality assurance", "qa"}}
	synonyms = map[string]string{"developer": "engineer", "programmer": "engineer", "swe": "engineer", "engineering": "engineer", "sw": "software", "mgr": "manager", "dev": "engineer"}
	// Words that do not decide whether a title is the right kind of role.
	ignore = map[string]bool{"senior": true, "sr": true, "junior": true, "jr": true, "i": true, "ii": true, "iii": true, "iv": true, "staff": true,
		"principal": true, "lead": true, "the": true, "of": true, "and": true, "a": true, "an": true, "level": true, "entry": true, "mid": true, "remote": true}
)

func normTokens(s string) []string {
	s = " " + nonWord.ReplaceAllString(strings.ToLower(s), " ") + " "
	for _, p := range phrases {
		s = strings.ReplaceAll(s, " "+p[0]+" ", " "+p[1]+" ")
	}
	var out []string
	for _, t := range strings.Fields(s) {
		if v, ok := synonyms[t]; ok {
			t = v
		}
		out = append(out, t)
	}
	return out
}

type keyword struct {
	tokens []string
	phrase string
}

func compileKeywords(kws []string) []keyword {
	var out []keyword
	for _, k := range kws {
		var toks []string
		for _, t := range normTokens(k) {
			if !ignore[t] {
				toks = append(toks, t)
			}
		}
		if len(toks) > 0 {
			out = append(out, keyword{toks, strings.Join(toks, " ")})
		}
	}
	return out
}

// titleScore is the best share of a keyword's words found in the title, with a bonus for the exact phrase.
func titleScore(title string, kws []keyword) float64 {
	if len(kws) == 0 {
		return 0.5
	}
	tt := normTokens(title)
	set := map[string]bool{}
	for _, t := range tt {
		set[t] = true
	}
	joined := " " + strings.Join(tt, " ") + " "
	best := 0.0
	for _, k := range kws {
		hit := 0
		for _, t := range k.tokens {
			if set[t] {
				hit++
			}
		}
		s := float64(hit) / float64(len(k.tokens))
		if s == 1 && strings.Contains(joined, " "+k.phrase+" ") {
			s += 0.1
		}
		best = max(best, s)
	}
	return best
}

func splitTerms(s string) []string {
	var out []string
	for _, p := range strings.Split(strings.ToLower(s), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// modeOK filters by work mode. Remote must be stated, because a listing that says nothing is almost
// always on-site. Hybrid and on-site listings often do not say, so unknown passes for those.
func modeOK(mode, want string) bool {
	switch want {
	case "", "Any":
		return true
	case "Remote":
		return mode == "Remote"
	}
	return mode == "" || mode == want
}

var (
	usStates = regexp.MustCompile(`\b(AL|AK|AZ|AR|CA|CO|CT|DE|DC|FL|GA|HI|ID|IL|IN|IA|KS|KY|LA|ME|MD|MA|MI|MN|MS|MO|MT|NE|NV|NH|NJ|NM|NY|NC|ND|OH|OK|OR|PA|RI|SC|SD|TN|TX|UT|VT|VA|WA|WV|WI|WY)\b`)
	usWords  = regexp.MustCompile(`(?i)\b(united states|usa|u\.s\.|us|alabama|alaska|arizona|arkansas|california|colorado|connecticut|delaware|florida|georgia|hawaii|idaho|illinois|indiana|iowa|kansas|kentucky|louisiana|maine|maryland|massachusetts|michigan|minnesota|mississippi|missouri|montana|nebraska|nevada|new hampshire|new jersey|new mexico|new york|north carolina|north dakota|ohio|oklahoma|oregon|pennsylvania|rhode island|south carolina|south dakota|tennessee|texas|utah|vermont|virginia|washington|west virginia|wisconsin|wyoming|nyc|san francisco|seattle|boston|chicago|austin|denver|atlanta|los angeles)\b`)
)

func isUS(loc string) bool { return usStates.MatchString(loc) || usWords.MatchString(loc) }

// home describes where the user lives, for ranking.
type home struct {
	terms []string // city and region, lower case
	us    bool
}

func newHome(loc string) home {
	h := home{terms: splitTerms(loc), us: loc != "" && isUS(loc)}
	return h
}

// homeBoost nudges postings near the user up and postings in other countries down. Plain "Remote" is neutral.
func homeBoost(loc string, h home) float64 {
	if len(h.terms) == 0 || strings.TrimSpace(loc) == "" {
		return 0
	}
	l := strings.ToLower(loc)
	for _, t := range h.terms {
		if len(t) >= 2 && strings.Contains(l, t) && !(len(t) == 2 && !usStates.MatchString(loc)) {
			return 0.08
		}
	}
	if h.us {
		if isUS(loc) {
			return 0.04
		}
		if strings.TrimSpace(strings.Trim(l, "()- ")) == "remote" {
			return 0
		}
		return -0.25
	}
	return 0
}

func locationOK(loc, mode string, terms []string, wantMode string) bool {
	if len(terms) == 0 {
		return true
	}
	if mode == "Remote" && wantMode != "On-site" {
		return true
	}
	l := strings.ToLower(loc)
	for _, t := range terms {
		if strings.Contains(l, t) {
			return true
		}
	}
	return false
}

// skillScore is the share of the posting's detected skills the resume has, softened for postings that mention few.
func skillScore(text string, have map[string]bool) float64 {
	tech, soft := parser.FindSkills(text)
	all := append(tech, soft...)
	matched := 0
	for _, s := range all {
		if have[s] {
			matched++
		}
	}
	return round2(float64(matched) / float64(max(len(all), 5)))
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }
