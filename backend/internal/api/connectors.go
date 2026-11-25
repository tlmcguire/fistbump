package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/connectors/ashby"
	"github.com/tlmcguire/fistbump/backend/internal/connectors/greenhouse"
	"github.com/tlmcguire/fistbump/backend/internal/connectors/lever"
	"github.com/tlmcguire/fistbump/backend/internal/cost"
	"github.com/tlmcguire/fistbump/backend/internal/logos"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/parser"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

// Board tokens are lowercase letters, digits, - and _. Job ids are digits. The length caps sit well
// above real values (the longest curated board is 23 characters, ids are about 10 digits) and only
// reject junk before it goes into an outbound URL.
var (
	boardPattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
	jobIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)
)

// connectorList is the fixed body of GET /v1/connectors.
var connectorList = []struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}{{"greenhouse", "Greenhouse"}, {"lever", "Lever"}, {"ashby", "Ashby"}}

const (
	// maxLinkLen caps the pasted link before it is parsed.
	maxLinkLen = 2048
	// maxFetchBody caps the search request body (64 KiB) before it is decoded.
	maxFetchBody = 64 << 10
	// The caps below keep one request from fanning out into thousands of outbound requests.
	maxBoards     = 100
	maxKeywords   = 10
	maxKeywordLen = 100
)

// connectorList handles GET /v1/connectors by writing the fixed list.
func (s *Server) connectorList(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, http.StatusOK, connectorList)
	return nil
}

// validateIDs checks a board and job id. Both routes use it, including for the values ParseLink
// reads from a pasted link, so they accept exactly the same boards and jobs.
func validateIDs(board, jobID string) error {
	if !boardPattern.MatchString(board) {
		return validationFailed("Board token is not valid")
	}
	if !jobIDPattern.MatchString(jobID) {
		return validationFailed("Job id is not valid")
	}
	return nil
}

// chooseBoards returns the de-duplicated board tokens to search: q.Boards if set, else the curated
// boards (limited to q.Categories if given), then narrowed by q.Company. It assumes q is validated.
func chooseBoards(q connectors.Query) []string {
	var tokens []string
	switch {
	case len(q.Boards) > 0:
		tokens = q.Boards
	case len(q.Categories) > 0:
		for _, b := range greenhouse.BoardsFor(q.Categories) {
			tokens = append(tokens, b.Token)
		}
	default:
		tokens = greenhouse.Tokens()
	}

	// An explicit board list is used as given, so the company filter only narrows the curated set.
	want := strings.ToLower(strings.TrimSpace(q.Company))
	filter := want != "" && len(q.Boards) == 0

	seen := make(map[string]bool, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if seen[token] {
			continue
		}
		if filter && !strings.Contains(strings.ToLower(greenhouse.CompanyFor(token)), want) {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}

// categoryList handles GET /v1/connectors/greenhouse/categories: the curated industries, so the app can offer them.
func (s *Server) categoryList(w http.ResponseWriter, r *http.Request) error {
	type category struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		Parent     *string `json:"parent"`
		BoardCount int     `json:"board_count"`
	}
	out := []category{}
	for _, c := range greenhouse.Categories() {
		out = append(out, category{c.ID, c.Name, c.Parent, greenhouse.BoardCount(c.ID)})
	}
	writeJSON(w, http.StatusOK, struct {
		Categories []category `json:"categories"`
		Companies  []string   `json:"companies"`
		BoardTotal int        `json:"board_total"`
		VerifiedAt string     `json:"verified_at"`
		Notice     string     `json:"notice"`
	}{out, greenhouse.CompanyNames(), len(greenhouse.Tokens()), greenhouse.VerifiedAt(), "These lists are not exhaustive. Only the companies listed are searched."})
	return nil
}

// greenhouseFetch searches the curated boards and returns ranked postings with their descriptions.
// It answers 422 for an invalid query and 502 only when every board failed.
func (s *Server) greenhouseFetch(w http.ResponseWriter, r *http.Request) error {
	if s.search == nil {
		return notImplemented() // no searcher configured yet
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFetchBody)

	var body struct {
		Query connectors.Query `json:"query"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		return bodyError(err)
	}
	// Require exactly one JSON value: after it, only whitespace may remain.
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return badRequest("Request must contain a single JSON value")
		}
		return bodyError(err)
	}

	q := body.Query
	q.Source = strings.ToLower(strings.TrimSpace(q.Source))
	keywords := q.Keywords[:0]
	for _, keyword := range q.Keywords {
		if keyword = strings.TrimSpace(keyword); keyword != "" {
			keywords = append(keywords, keyword)
		}
	}
	q.Keywords = keywords
	// With nothing to search by, fall back to the user's target positions, then their latest job title.
	if len(q.Keywords) == 0 && strings.TrimSpace(q.Location) == "" && len(q.Categories) == 0 && len(q.Boards) == 0 && strings.TrimSpace(q.Company) == "" && q.Source == "" {
		q.Keywords = s.defaultKeywords(r.Context())
	}
	exclude := make([]string, 0, len(q.Exclude))
	for _, word := range q.Exclude {
		if word = strings.TrimSpace(word); word != "" {
			exclude = append(exclude, word)
		}
	}
	q.Exclude = exclude
	if q.Limit == 0 {
		q.Limit = 20
	}
	if q.WorkMode == "" {
		q.WorkMode = "Any"
	}

	if err := q.Validate(); err != nil {
		return validationFailed(err.Error())
	}
	// Keywords are optional when the search is narrowed some other way: a place, an industry, ATS source or boards.
	if len(q.Keywords) == 0 && strings.TrimSpace(q.Location) == "" && len(q.Categories) == 0 && len(q.Boards) == 0 && strings.TrimSpace(q.Company) == "" && q.Source == "" {
		return validationFailed("Enter keywords, a location or an industry to search")
	}
	if len(q.Keywords) > maxKeywords {
		return validationFailed("Too many keywords")
	}
	for _, word := range q.Exclude {
		if len(word) > maxKeywordLen {
			return validationFailed("A word to exclude is too long")
		}
	}
	for _, keyword := range q.Keywords {
		if len(keyword) > maxKeywordLen {
			return validationFailed("A keyword is too long")
		}
	}
	for _, category := range q.Categories {
		if _, ok := greenhouse.FindCategory(category); !ok {
			return validationFailed("Category is not valid")
		}
	}
	// Board tokens are checked here, not in Query.Validate, because boardPattern lives in this package.
	if len(q.Boards) > maxBoards {
		return validationFailed("Too many boards")
	}
	for _, board := range q.Boards {
		if !boardPattern.MatchString(board) {
			return validationFailed("Board token is not valid")
		}
	}

	boards := s.chooseBoardsWithCustom(r.Context(), q)
	named := strings.TrimSpace(q.Company) != "" && len(q.Boards) == 0 // the search is for one company
	guessed := false
	if len(boards) == 0 && named {
		// The curated list has no such company. Greenhouse board tokens are usually the company name,
		// so try that; one listing request tells us whether the board exists.
		boards, guessed = guessBoards(q.Company), true
	}
	if len(boards) == 0 {
		if named {
			return writeNoCompany(w, q)
		}
		return validationFailed("No boards match this search")
	}

	// The user can turn the resume off: the postings are then neither rated against it nor
	// ranked by it, and the fit filters do not apply.
	if !q.MatchesResume() {
		q.MinFit = 0
		q.MaxMissing = nil
	} else {
		q.SkillScore = s.skillScorer(r.Context())
		q.FitOf = s.fitOf(r.Context())
	}
	// A sort by fit or by a rating needs that part switched on.
	switch q.Sort {
	case connectors.SortFit:
		if !q.MatchesResume() {
			return validationFailed("Turn on matching with your resume to sort by fit")
		}
	case connectors.SortPay, connectors.SortBenefits, connectors.SortLiving, connectors.SortOverall:
		if !q.MatchesRatings() {
			return validationFailed("Turn on the pay, benefits and cost of living ratings to sort this way")
		}
	}
	// The ratings can be turned off too; their filters then do not apply.
	if !q.MatchesRatings() {
		q.MinPayRating, q.MinBenefits, q.MinLiving = "", "", ""
	} else {
		q.RateOf = s.rateOf(r.Context())
	}
	if q.HasFit() && q.FitOf == nil {
		return conflict("Add your resume first, so jobs can be filtered by how well you fit")
	}
	if q.Source == "lever" || q.Source == "ashby" {
		result := s.searchCustomATS(r.Context(), q, q.Source)
		writeJSON(w, http.StatusOK, result)
		return nil
	}
	result, err := s.search.Search(r.Context(), q, boards)
	if guessed && (err != nil || result.BoardsSearched == 0) {
		return writeNoCompany(w, q) // not a Greenhouse board we can find
	}
	if err != nil {
		return greenhouseError(err)
	}
	if named {
		result.Companies = companyNames(boards)
	}
	// Nil slices would encode as null; the app expects arrays.
	if result.Postings == nil {
		result.Postings = []connectors.Posting{}
	}
	if result.Errors == nil {
		result.Errors = []connectors.SearchError{}
	}
	// When searching all ATS sources on the first page, also merge matching postings from watchlisted Lever/Ashby boards.
	if q.Source == "" && q.Offset == 0 && len(q.Categories) == 0 && len(q.Boards) == 0 {
		for _, extraSrc := range []string{"lever", "ashby"} {
			extra := s.searchCustomATS(r.Context(), q, extraSrc)
			if extra.BoardsTotal > 0 {
				result.BoardsTotal += extra.BoardsTotal
				result.BoardsSearched += extra.BoardsSearched
				result.PostingsScanned += extra.PostingsScanned
				result.Matched += extra.Matched
				result.Errors = append(result.Errors, extra.Errors...)
				for _, p := range extra.Postings {
					if len(result.Postings) < q.Limit {
						result.Postings = append(result.Postings, p)
					}
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// searchCustomATS searches watchlisted (or company-named) Lever or Ashby boards.
func (s *Server) searchCustomATS(ctx context.Context, q connectors.Query, source string) connectors.SearchResult {
	result := connectors.SearchResult{
		Postings:     []connectors.Posting{},
		Errors:       []connectors.SearchError{},
		Companies:    []string{},
		KeywordsUsed: q.Keywords,
		WorkModeUsed: q.WorkMode,
	}
	var boards []string
	seen := map[string]bool{}
	wantCompany := strings.ToLower(strings.TrimSpace(q.Company))
	if vals, err := s.store.GetSettings(ctx); err == nil {
		if custom, ok := vals["connectors.custom_boards"].([]string); ok {
			for _, entry := range custom {
				src, token, ok := strings.Cut(strings.ToLower(strings.TrimSpace(entry)), ":")
				if !ok || strings.TrimSpace(src) != source {
					continue
				}
				token = strings.TrimSpace(token)
				if token == "" || seen[token] {
					continue
				}
				if wantCompany != "" && !strings.Contains(token, wantCompany) {
					continue
				}
				seen[token] = true
				boards = append(boards, token)
			}
		}
	}
	if len(boards) == 0 && wantCompany != "" && q.Source == source {
		for _, g := range guessBoards(q.Company) {
			if !seen[g] {
				seen[g] = true
				boards = append(boards, g)
			}
		}
	}
	result.BoardsTotal = len(boards)
	if len(boards) == 0 {
		return result
	}
	oldest := time.Now().UTC().AddDate(0, 0, -q.PostedWithin)
	locNeedle := strings.ToLower(strings.TrimSpace(q.Location))
	var matched []connectors.Posting
	type boardLister interface {
		ListBoard(ctx context.Context, board string) ([]connectors.Posting, error)
	}
	for _, board := range boards {
		var lister boardLister
		if source == "lever" {
			if l, ok := s.lever.(boardLister); ok && l != nil {
				lister = l
			} else {
				lister = lever.NewClient()
			}
		} else {
			if l, ok := s.ashby.(boardLister); ok && l != nil {
				lister = l
			} else {
				lister = ashby.NewClient()
			}
		}
		list, err := lister.ListBoard(ctx, board)
		if err != nil {
			result.Errors = append(result.Errors, connectors.SearchError{Board: source + ":" + board, Message: err.Error()})
			continue
		}
		result.BoardsSearched++
		result.PostingsScanned += len(list)
		if wantCompany != "" {
			result.Companies = append(result.Companies, board)
		}
		for _, p := range list {
			if !customPostingMatches(p, q, locNeedle, oldest) {
				continue
			}
			if q.FitOf != nil && p.RawText != "" {
				p.Fit = q.FitOf(p.RawText)
			}
			if !q.FitOK(p.Fit) {
				continue
			}
			if q.RateOf != nil && q.MatchesRatings() && p.RawText != "" {
				p.Ratings = q.RateOf(p)
			}
			if !q.RatingsOK(p.Ratings) {
				continue
			}
			matched = append(matched, p)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].UpdatedAt.After(matched[j].UpdatedAt)
	})
	result.Matched = len(matched)
	start := min(q.Offset, len(matched))
	end := min(start+q.Limit, len(matched))
	result.Postings = append(result.Postings, matched[start:end]...)
	result.HasMore = end < len(matched)
	result.NextOffset = end
	return result
}

func customPostingMatches(p connectors.Posting, q connectors.Query, locNeedle string, oldest time.Time) bool {
	titleLower := strings.ToLower(p.PositionTitle)
	if q.WorkMode != "" && q.WorkMode != connectors.WorkModeAny && !strings.EqualFold(p.WorkMode, q.WorkMode) {
		return false
	}
	if locNeedle != "" && !strings.Contains(strings.ToLower(p.Location), locNeedle) && !strings.EqualFold(p.WorkMode, connectors.WorkModeRemote) {
		return false
	}
	if q.PostedWithin > 0 && !p.UpdatedAt.IsZero() && p.UpdatedAt.Before(oldest) {
		return false
	}
	for _, ex := range q.Exclude {
		if ex != "" && strings.Contains(titleLower, strings.ToLower(ex)) {
			return false
		}
	}
	if len(q.Keywords) > 0 {
		matchedKW := false
		for _, kw := range q.Keywords {
			if strings.Contains(titleLower, strings.ToLower(kw)) || strings.Contains(strings.ToLower(p.RawText), strings.ToLower(kw)) {
				matchedKW = true
				break
			}
		}
		if !matchedKW {
			return false
		}
	}
	if q.RequirePay && p.PayMax == 0 {
		return false
	}
	if q.PayMin > 0 && p.PayMax > 0 && p.PayMax < q.PayMin {
		return false
	}
	if q.PayMax > 0 && p.PayMin > 0 && p.PayMin > q.PayMax {
		return false
	}
	return true
}

// companyLogo handles GET /v1/connectors/greenhouse/logo?url=&board=: the company's logo as a data URL,
// from its own site and, failing that, its Greenhouse board page. A company with no usable logo answers 404, which the app shows as a letter instead.
func (s *Server) companyLogo(w http.ResponseWriter, r *http.Request) error {
	if s.logos == nil {
		return notImplemented()
	}
	site, board := r.URL.Query().Get("url"), r.URL.Query().Get("board")
	if (site == "" && board == "") || len(site) > maxLinkLen {
		return validationFailed("Enter a company website link or a board")
	}
	if site != "" {
		if _, err := logos.Host(site); err != nil {
			return validationFailed("The company link must be an https link")
		}
	}
	if board != "" && !boardPattern.MatchString(board) {
		return validationFailed("Board token is not valid")
	}
	logo, err := s.logos.Logo(r.Context(), site, board)
	if err != nil {
		if r.Context().Err() != nil {
			return err
		}
		return notFound("No logo found for this company.")
	}
	writeJSON(w, http.StatusOK, struct {
		ContentType string `json:"content_type"`
		DataURL     string `json:"data_url"`
	}{logo.ContentType, logo.DataURL()})
	return nil
}

// guessBoards turns a company name into likely Greenhouse board tokens: "Acme Corp" gives
// "acmecorp" and "acme-corp".
func guessBoards(company string) []string {
	var words []string
	for _, w := range strings.FieldsFunc(strings.ToLower(company), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		words = append(words, w)
	}
	var out []string
	for _, token := range []string{strings.Join(words, ""), strings.Join(words, "-")} {
		if boardPattern.MatchString(token) && (len(out) == 0 || out[0] != token) {
			out = append(out, token)
		}
	}
	return out
}

// skillScorer rates posting text against the master resume, for ranking search results by skill
// fit. It returns nil without a profile, so the search keeps its title and location ranking.
func (s *Server) skillScorer(ctx context.Context) func(text string) float64 {
	m, err := s.store.GetMaster(ctx)
	if err != nil || m.Profile == nil {
		return nil
	}
	return func(text string) float64 {
		var j models.Job
		applyParse(&j, s.parse(text))
		return s.analyze(j, m).Score
	}
}

// rateOf rates a posting's pay, benefits and cost of living, using the user's home location and minimum pay.
func (s *Server) rateOf(ctx context.Context) func(p connectors.Posting) *connectors.Ratings {
	home, desired := "", int64(0)
	if m, err := s.store.GetMaster(ctx); err == nil && m.Profile != nil {
		if m.Profile.HomeLocation != nil {
			home = *m.Profile.HomeLocation
		}
		if m.Profile.MinDesiredPay != nil {
			desired = *m.Profile.MinDesiredPay
		}
	}
	return func(p connectors.Posting) *connectors.Ratings {
		r := cost.Rate(cost.Analyze(cost.Input{Title: p.PositionTitle, Location: p.Location, WorkMode: p.WorkMode,
			PayMin: int64(p.PayMin), PayMax: int64(p.PayMax), Text: p.RawText, HomeLocation: home, MinDesiredPay: desired}))
		return &connectors.Ratings{Pay: r.Pay, Benefits: r.Benefits, Living: r.Living,
			PayAmount: int(r.PayAmount), BenefitsScore: r.BenefitsScore, LivingLeft: r.LivingLeft, BenefitsFound: r.Found, LeftMonthly: r.LeftMonthly}
	}
}

// fitOf compares posting text with the master resume's skills: the share of required skills shown, and
// the ones missing. It returns nil without a profile, and a nil result for a posting that lists no required skills.
func (s *Server) fitOf(ctx context.Context) func(text string) *connectors.Fit {
	m, err := s.store.GetMaster(ctx)
	if err != nil || m.Profile == nil {
		return nil
	}
	return func(text string) *connectors.Fit {
		var j models.Job
		applyParse(&j, s.parse(text))
		res := s.analyze(j, m)
		// The technical skills count. A soft skill nobody writes on a resume is not evidence
		// against the person, so a posting that lists only soft skills is not rated at all.
		missing := append([]string{}, res.Required.Tech.Missing...)
		matched := len(res.Required.Tech.Matched)
		likely, because := []string{}, []string{}
		for _, l := range res.Required.Tech.Likely {
			likely, because = append(likely, l.Skill), append(because, l.Because)
		}
		required := matched + len(likely) + len(missing)
		if required == 0 {
			return nil
		}
		return &connectors.Fit{Percent: int(res.Score*100 + 0.5), Matched: matched, Required: required, Missing: missing, Likely: likely, Because: because}
	}
}

// companyNames is the display name of each board, at most ten.
func companyNames(boards []string) []string {
	names := []string{}
	for _, token := range boards {
		if len(names) == 10 {
			break
		}
		if name := greenhouse.CompanyFor(token); name != "" {
			names = append(names, name)
		} else {
			names = append(names, token)
		}
	}
	return names
}

// writeNoCompany answers a search for a company whose listings cannot be found: a normal empty
// result with no companies, which the app explains, instead of an error.
func writeNoCompany(w http.ResponseWriter, q connectors.Query) error {
	writeJSON(w, http.StatusOK, connectors.SearchResult{
		Postings:     []connectors.Posting{},
		Errors:       []connectors.SearchError{},
		Companies:    []string{},
		KeywordsUsed: q.Keywords,
		WorkModeUsed: q.WorkMode,
	})
	return nil
}

// bodyError maps a request-body decode failure to an API error. The messages are fixed because
// the decoder's own text can name Go types.
func bodyError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		e := validationFailed("Request is too large")
		e.cause = err
		return e
	}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return badRequest("Request contains invalid JSON")
	}
	return validationFailed("Request has an invalid field")
}

// greenhouseJob returns a listing the app already knows, by board and job id. It serves reloads
// and, later, re-pulling a saved job; a 404 means the posting has closed.
func (s *Server) greenhouseJob(w http.ResponseWriter, r *http.Request) error {
	board, jobID := r.PathValue("board"), r.PathValue("job_id")
	if err := validateIDs(board, jobID); err != nil {
		return err
	}
	return s.writePosting(w, r, board, jobID)
}

// chooseBoardsWithCustom merges user-watchlisted Greenhouse boards from connectors.custom_boards.
func (s *Server) chooseBoardsWithCustom(ctx context.Context, q connectors.Query) []string {
	boards := chooseBoards(q)
	if len(q.Boards) > 0 || len(q.Categories) > 0 {
		return boards
	}
	vals, err := s.store.GetSettings(ctx)
	if err != nil {
		return boards
	}
	custom, _ := vals["connectors.custom_boards"].([]string)
	if len(custom) == 0 {
		return boards
	}
	want := strings.ToLower(strings.TrimSpace(q.Company))
	seen := make(map[string]bool, len(boards)+len(custom))
	for _, b := range boards {
		seen[b] = true
	}
	for _, entry := range custom {
		src, token := "greenhouse", strings.TrimSpace(strings.ToLower(entry))
		if p, t, ok := strings.Cut(token, ":"); ok {
			src, token = strings.TrimSpace(p), strings.TrimSpace(t)
		}
		if src != "greenhouse" || !boardPattern.MatchString(token) || seen[token] {
			continue
		}
		if want != "" && !strings.Contains(token, want) {
			continue
		}
		seen[token] = true
		boards = append(boards, token)
	}
	return boards
}

// greenhouseLink opens a pasted job link from Greenhouse, Lever, or Ashby.
func (s *Server) greenhouseLink(w http.ResponseWriter, r *http.Request) error {
	raw := r.URL.Query().Get("url")
	if raw == "" {
		return validationFailed("Enter a Greenhouse, Lever, or Ashby job link")
	}
	if len(raw) > maxLinkLen {
		return validationFailed("The job link is too long")
	}

	if board, jobID, err := lever.ParseLink(raw); err == nil {
		client := s.lever
		if client == nil {
			client = lever.NewClient()
		}
		p, err := client.FetchJob(r.Context(), board, jobID)
		if err != nil {
			return leverError(err)
		}
		writeJSON(w, http.StatusOK, p)
		return nil
	}
	if board, jobID, err := ashby.ParseLink(raw); err == nil {
		client := s.ashby
		if client == nil {
			client = ashby.NewClient()
		}
		p, err := client.FetchJob(r.Context(), board, jobID)
		if err != nil {
			return ashbyError(err)
		}
		writeJSON(w, http.StatusOK, p)
		return nil
	}

	// ParseLink's error text is written for the user (for example, the explanation for links
	// that carry only gh_jid), so it is passed through as the 422 message.
	board, jobID, err := greenhouse.ParseLink(raw)
	if err != nil {
		return validationFailed(err.Error())
	}
	if err := validateIDs(board, jobID); err != nil {
		return err
	}
	return s.writePosting(w, r, board, jobID)
}

func leverError(err error) error {
	switch {
	case errors.Is(err, lever.ErrorNotFound):
		return notFound("This Lever job is closed or does not exist.")
	case errors.Is(err, lever.ErrorRateLimited):
		return upstreamError("Lever is limiting requests right now. Wait a minute, then try again.")
	case errors.Is(err, lever.ErrorUpstream):
		return upstreamError("Lever did not respond. Try again.")
	default:
		return err
	}
}

func ashbyError(err error) error {
	switch {
	case errors.Is(err, ashby.ErrorNotFound):
		return notFound("This Ashby job is closed or does not exist.")
	case errors.Is(err, ashby.ErrorRateLimited):
		return upstreamError("Ashby is limiting requests right now. Wait a minute, then try again.")
	case errors.Is(err, ashby.ErrorUpstream):
		return upstreamError("Ashby did not respond. Try again.")
	default:
		return err
	}
}

// writePosting fetches one posting and writes it as JSON. Both routes share it so they cannot
// drift apart. The request's context cancels the Greenhouse call if the app stops waiting.
func (s *Server) writePosting(w http.ResponseWriter, r *http.Request, board, jobID string) error {
	p, err := s.gh.FetchJob(r.Context(), board, jobID)
	if err != nil {
		return greenhouseError(err)
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

// greenhouseError maps the Greenhouse client's errors to API errors. The messages are shown to
// the user. Any other error is returned unchanged and becomes a 500.
func greenhouseError(err error) error {
	switch {
	case errors.Is(err, greenhouse.ErrorNotFound):
		return notFound("This job is closed or does not exist.")
	case errors.Is(err, greenhouse.ErrorRateLimited):
		return upstreamError("Greenhouse is limiting how many requests it answers right now. Wait a minute, then try again.")
	case errors.Is(err, greenhouse.ErrorUpstream):
		return upstreamError("Greenhouse did not respond. Try again.")
	default:
		return err
	}
}

// maxImport caps the postings one import saves.
const maxImport = 50

// greenhouseImport handles POST /v1/connectors/greenhouse/import: it saves postings as jobs, or
// refreshes the ones already saved. Every posting is fetched before anything is written, so a closed
// posting or a Greenhouse failure saves nothing.
func (s *Server) greenhouseImport(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Postings []struct {
			Source     string `json:"source"`
			Board      string `json:"board"`
			ExternalID string `json:"external_id"`
		} `json:"postings"`
	}
	if err := decodeJSONLimit(w, r, &body, maxFetchBody); err != nil {
		return err
	}
	if len(body.Postings) == 0 || len(body.Postings) > maxImport {
		return validationError("postings", "postings must have 1 to 50 entries")
	}
	type key struct{ source, board, id string }
	var keys []key
	seen := map[key]bool{}
	for _, p := range body.Postings {
		src := strings.ToLower(strings.TrimSpace(p.Source))
		if src == "" {
			src = "greenhouse"
		}
		if src == "greenhouse" {
			if err := validateIDs(p.Board, p.ExternalID); err != nil {
				return err
			}
		} else if src != "lever" && src != "ashby" {
			return validationFailed("Unsupported connector source")
		} else if strings.TrimSpace(p.Board) == "" || strings.TrimSpace(p.ExternalID) == "" {
			return validationFailed("Board and job id are required")
		}
		k := key{src, p.Board, p.ExternalID}
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}

	// The first failure cancels the other fetches. Greenhouse reports a cancelled fetch as an upstream
	// error, so the failure kept is the first one seen, not the first in request order.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	postings := make([]connectors.Posting, len(keys))
	var (
		mu       sync.Mutex
		failed   error
		failedID string
		wg       sync.WaitGroup
		sem      = make(chan struct{}, 8)
	)
	for i, k := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			var p connectors.Posting
			var err error
			switch k.source {
			case "lever":
				client := s.lever
				if client == nil {
					client = lever.NewClient()
				}
				p, err = client.FetchJob(ctx, k.board, k.id)
			case "ashby":
				client := s.ashby
				if client == nil {
					client = ashby.NewClient()
				}
				p, err = client.FetchJob(ctx, k.board, k.id)
			default:
				p, err = s.gh.FetchJob(ctx, k.board, k.id)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil && failed == nil {
				failed, failedID = err, k.id
				cancel()
			}
			postings[i] = p
		}()
	}
	wg.Wait()
	if err := r.Context().Err(); err != nil {
		return err // the app stopped waiting
	}
	if failed != nil {
		apiErr := greenhouseError(failed)
		var e *apiError
		if errors.As(apiErr, &e) && e.Status == http.StatusNotFound {
			e.Details = map[string]any{"external_id": failedID}
		}
		return apiErr
	}

	out := struct {
		Imported  []models.Job `json:"imported"`
		Refreshed []models.Job `json:"refreshed"`
	}{[]models.Job{}, []models.Job{}}
	err := s.store.WithTx(r.Context(), func(tx *store.Store) error {
		for _, p := range postings {
			j, created, err := tx.UpsertGreenhouseJob(r.Context(), s.jobFromPosting(p))
			if err != nil {
				return err
			}
			if created {
				out.Imported = append(out.Imported, j)
			} else {
				out.Refreshed = append(out.Refreshed, j)
			}
		}
		return nil
	})
	if err != nil {
		return storeError(err, "Job")
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// jobFromPosting builds the job saved for a Greenhouse posting: the posting's own fields, with the
// parser's skills and requirements, and its inferred values only where the posting has none.
func (s *Server) jobFromPosting(p connectors.Posting) models.Job {
	src := p.Source
	if src == "" {
		src = "greenhouse"
	}
	j := models.Job{
		CompanyName:   p.CompanyName,
		PositionTitle: p.PositionTitle,
		Source:        src,
		ExternalID:    &p.ExternalID,
		Board:         &p.Board,
		CompanyURL:    p.CompanyURL,
		RawText:       &p.RawText,
	}
	if p.ListingURL != "" {
		j.ListingURL = &p.ListingURL
	}
	if p.Location != "" {
		j.Location = &p.Location
	}
	mode := greenhouse.WorkMode(p.Location, p.PositionTitle)
	if described := parser.DescribedMode(p.RawText); mode == connectors.WorkModeOnSite && described != "" {
		mode = described
	}
	j.WorkMode = &mode
	applyParse(&j, s.parse(p.RawText))
	if j.CompanyName == "" {
		j.CompanyName = greenhouse.CompanyFor(p.Board)
	}
	if j.CompanyName == "" {
		j.CompanyName = p.Board
	}
	if j.PositionTitle == "" {
		j.PositionTitle = "Untitled job"
	}
	return j
}
