package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/connectors/greenhouse"
)

// Board tokens are lowercase letters, digits, - and _. Job ids are digits. The length caps sit well
// above real values (the longest curated board is 23 characters, ids are about 10 digits) and only
// reject junk before it goes into an outbound URL.
var (
	boardPattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
	jobIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)
)

// connectorList is the fixed body of GET /v1/connectors. Greenhouse is the only connector so far.
var connectorList = []struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}{{"greenhouse", "Greenhouse"}}

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
		VerifiedAt string     `json:"verified_at"`
		Notice     string     `json:"notice"`
	}{out, greenhouse.CompanyNames(), greenhouse.VerifiedAt(), "These lists are not exhaustive. Only the companies listed are searched."})
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
	keywords := q.Keywords[:0]
	for _, keyword := range q.Keywords {
		if keyword = strings.TrimSpace(keyword); keyword != "" {
			keywords = append(keywords, keyword)
		}
	}
	q.Keywords = keywords
	if q.Limit == 0 {
		q.Limit = 20
	}
	if q.WorkMode == "" {
		q.WorkMode = "Any"
	}

	if err := q.Validate(); err != nil {
		return validationFailed(err.Error())
	}
	// Keywords are optional when the search is narrowed some other way: a place, an industry or boards.
	if len(q.Keywords) == 0 && strings.TrimSpace(q.Location) == "" && len(q.Categories) == 0 && len(q.Boards) == 0 && strings.TrimSpace(q.Company) == "" {
		return validationFailed("Enter keywords, a location or an industry to search")
	}
	if len(q.Keywords) > maxKeywords {
		return validationFailed("Too many keywords")
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

	boards := chooseBoards(q)
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
	writeJSON(w, http.StatusOK, result)
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
		return validationFailed("Request is too large")
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

// greenhouseLink opens a pasted job link: it reads the board and job id from the link, then
// fetches the posting. This is how a listing first enters the app.
func (s *Server) greenhouseLink(w http.ResponseWriter, r *http.Request) error {
	raw := r.URL.Query().Get("url")
	if raw == "" {
		return validationFailed("Enter a Greenhouse job link")
	}
	if len(raw) > maxLinkLen {
		return validationFailed("The job link is too long")
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
	case errors.Is(err, greenhouse.ErrorUpstream):
		return upstreamError("Greenhouse did not respond. Try again.")
	default:
		return err
	}
}
