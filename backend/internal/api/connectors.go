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

// greenhouseFetch searches the curated boards and returns ranked postings with their descriptions.
// It answers 422 for an invalid query and 502 only when every board failed.
func (s *Server) greenhouseFetch(w http.ResponseWriter, r *http.Request) error {
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
	if len(q.Keywords) == 0 {
		return validationFailed("Enter keywords to search")
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
	if len(boards) == 0 {
		return validationFailed("No boards match this search")
	}

	result, err := s.search.Search(r.Context(), q, boards)
	if err != nil {
		return greenhouseError(err)
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
