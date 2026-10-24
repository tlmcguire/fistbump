package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/connectors/greenhouse"
	"github.com/tlmcguire/fistbump/backend/internal/db"
)

// fakeSearcher stands in for Greenhouse search. It records the query and boards it was given.
type fakeSearcher struct {
	result    connectors.SearchResult
	err       error
	called    bool
	gotQuery  connectors.Query
	gotBoards []string
}

func (f *fakeSearcher) Search(_ context.Context, q connectors.Query, boards []string) (connectors.SearchResult, error) {
	f.called, f.gotQuery, f.gotBoards = true, q, boards
	return f.result, f.err
}

// newSearchServer builds a server whose search route uses the given searcher.
func newSearchServer(t *testing.T, f jobSearcher) *Server {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return New(Config{DB: conn, Greenhouse: &fakeFetcher{}, Searcher: f, Token: testToken, Version: "test"})
}

// postFetch sends body to the search route with the valid token.
func postFetch(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/connectors/greenhouse/fetch", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

const testToken = "test-token"

// fakeFetcher stands in for the Greenhouse client. It records what it was asked for and returns
// a canned result, so these tests never touch the network.
type fakeFetcher struct {
	err              error
	gotBoard, gotJob string
}

func (f *fakeFetcher) FetchJob(_ context.Context, board, jobID string) (connectors.Posting, error) {
	f.gotBoard, f.gotJob = board, jobID
	return connectors.Posting{}, f.err
}

// newTestServer builds a server on a fresh migrated database.
func newTestServer(t *testing.T, f jobFetcher) *Server {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return New(Config{DB: conn, Greenhouse: f, Token: testToken, Version: "test"})
}

// do sends a request through the whole handler chain. An empty auth sends no Authorization header.
func do(t *testing.T, s *Server, method, target, auth string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// doAuth sends a request with the valid token.
func doAuth(t *testing.T, s *Server, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, s, method, target, "Bearer "+testToken)
}

// decodeError reads the error shape from api.md out of a response.
func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("response is not the error shape: %v (body %q)", err, rec.Body.String())
	}
	return body
}

// wantError checks the status and the error code of a response.
func wantError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Errorf("status = %d, want %d (body %q)", rec.Code, status, rec.Body.String())
		return
	}
	if got := decodeError(t, rec).Error.Code; got != code {
		t.Errorf("error code = %q, want %q", got, code)
	}
}

func TestRequiresToken(t *testing.T) {
	s := newTestServer(t, &fakeFetcher{})
	for _, tc := range []struct{ name, path, auth string }{
		{"missing header", "/v1/health", ""},
		{"wrong scheme", "/v1/health", "Basic " + testToken},
		{"wrong token", "/v1/health", "Bearer nope"},
		{"empty token", "/v1/health", "Bearer "},
		{"unbuilt route", "/v1/resume", ""},
		{"unknown path", "/nope", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantError(t, do(t, s, "GET", tc.path, tc.auth), http.StatusUnauthorized, "unauthorized")
		})
	}
}

// An unset token must never match, even though two empty strings compare equal.
func TestEmptyConfiguredTokenRejectsEverything(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	s := New(Config{DB: conn, Greenhouse: &fakeFetcher{}, Token: ""})

	wantError(t, do(t, s, "GET", "/v1/health", ""), http.StatusUnauthorized, "unauthorized")
	wantError(t, do(t, s, "GET", "/v1/health", "Bearer "), http.StatusUnauthorized, "unauthorized")
}

func TestHealth(t *testing.T) {
	s := newTestServer(t, &fakeFetcher{})
	rec := doAuth(t, s, "GET", "/v1/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var body struct {
		Status        string `json:"status"`
		Version       string `json:"version"`
		SchemaVersion int    `json:"schema_version"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Version != "test" {
		t.Errorf("body = %+v, want status ok and version test", body)
	}
	if body.SchemaVersion < 1 {
		t.Errorf("schema_version = %d, want at least 1 (migrations applied)", body.SchemaVersion)
	}
}

func TestGreenhouseLink(t *testing.T) {
	link := func(raw string) string {
		return "/v1/connectors/greenhouse/link?url=" + url.QueryEscape(raw)
	}
	for _, tc := range []struct {
		name, target string
		wantStatus   int
		wantBoard    string // what the fetcher should have been asked for; empty means not called
		wantJob      string
	}{
		{"new host", link("https://job-boards.greenhouse.io/backblaze/jobs/5287532008"), 200, "backblaze", "5287532008"},
		{"old host", link("https://boards.greenhouse.io/backblaze/jobs/5287532008"), 200, "backblaze", "5287532008"},
		{"missing url", "/v1/connectors/greenhouse/link", 422, "", ""},
		{"not a Greenhouse link", link("https://example.com/jobs/1"), 422, "", ""},
		{"company site with only gh_jid", link("https://stripe.com/jobs/search?gh_jid=8172510"), 422, "", ""},
		{"non-numeric job id", link("https://job-boards.greenhouse.io/backblaze/jobs/abc"), 422, "", ""},
		{"link too long", link("https://job-boards.greenhouse.io/backblaze/jobs/1?" + strings.Repeat("x", maxLinkLen)), 422, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeFetcher{}
			rec := doAuth(t, newTestServer(t, f), "GET", tc.target)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if f.gotBoard != tc.wantBoard || f.gotJob != tc.wantJob {
				t.Errorf("fetched %q/%q, want %q/%q", f.gotBoard, f.gotJob, tc.wantBoard, tc.wantJob)
			}
			if tc.wantStatus == 422 {
				body := decodeError(t, rec)
				if body.Error.Code != "validation_failed" {
					t.Errorf("error code = %q, want validation_failed", body.Error.Code)
				}
				// The message is shown to the user as written, so it must not be empty.
				if body.Error.Message == "" {
					t.Error("422 has no message for the user")
				}
			}
		})
	}
}

func TestGreenhouseJob(t *testing.T) {
	for _, tc := range []struct {
		name, board, jobID string
		wantStatus         int
		wantFetched        bool
	}{
		{"valid", "backblaze", "5287532008", 200, true},
		{"board with digits and dashes", "acme-2_x", "1", 200, true},
		{"uppercase board", "Backblaze", "1", 422, false},
		{"board with a dot", "back.blaze", "1", 422, false},
		{"board too long", strings.Repeat("a", 65), "1", 422, false},
		{"non-numeric job id", "backblaze", "12x", 422, false},
		{"job id too long", "backblaze", strings.Repeat("1", 21), 422, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeFetcher{}
			target := fmt.Sprintf("/v1/connectors/greenhouse/boards/%s/jobs/%s", tc.board, tc.jobID)
			rec := doAuth(t, newTestServer(t, f), "GET", target)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if fetched := f.gotBoard != ""; fetched != tc.wantFetched {
				t.Errorf("fetcher called = %v, want %v", fetched, tc.wantFetched)
			}
			if tc.wantFetched && (f.gotBoard != tc.board || f.gotJob != tc.jobID) {
				t.Errorf("fetched %q/%q, want %q/%q", f.gotBoard, f.gotJob, tc.board, tc.jobID)
			}
		})
	}
}

func TestGreenhouseErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"closed job", greenhouse.ErrorNotFound, 404, "not_found"},
		{"closed job, wrapped", fmt.Errorf("fetch backblaze: %w", greenhouse.ErrorNotFound), 404, "not_found"},
		{"upstream failure", greenhouse.ErrorUpstream, 502, "upstream_error"},
		{"upstream failure, wrapped", fmt.Errorf("status 500: %w", greenhouse.ErrorUpstream), 502, "upstream_error"},
		{"unexpected error", errors.New("secret detail"), 500, "internal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t, &fakeFetcher{err: tc.err})
			rec := doAuth(t, s, "GET", "/v1/connectors/greenhouse/boards/acme/jobs/1")
			wantError(t, rec, tc.wantStatus, tc.wantCode)
		})
	}
}

// An unexpected error is logged, never sent: its text could hold a path, a query or an upstream URL.
func TestInternalErrorHidesDetails(t *testing.T) {
	s := newTestServer(t, &fakeFetcher{err: errors.New("secret detail")})
	rec := doAuth(t, s, "GET", "/v1/connectors/greenhouse/boards/acme/jobs/1")
	if strings.Contains(rec.Body.String(), "secret detail") {
		t.Errorf("500 body leaks the error: %q", rec.Body.String())
	}
}

// Every route in api.md must be registered, and the ones not built answer 501 with the shared
// error shape, so the frontend can wire against the whole contract. New would panic on a
// duplicate or conflicting pattern, so this also checks the list registers cleanly.
func TestUnbuiltRoutesReturn501(t *testing.T) {
	s := newTestServer(t, &fakeFetcher{})
	built := map[string]bool{}
	for _, rt := range s.routes() {
		built[rt.pattern] = true
	}
	wildcard := regexp.MustCompile(`\{[^}]+\}`)

	for _, pattern := range unbuiltRoutePatterns() {
		if built[pattern] {
			continue
		}
		t.Run(pattern, func(t *testing.T) {
			method, path, _ := strings.Cut(pattern, " ")
			wantError(t, doAuth(t, s, method, wildcard.ReplaceAllString(path, "1")), http.StatusNotImplemented, "not_implemented")
		})
	}
}

func TestUnknownRoutesReturn404(t *testing.T) {
	s := newTestServer(t, &fakeFetcher{})
	for _, tc := range []struct{ name, method, path string }{
		{"unknown path", "GET", "/nope"},
		{"unknown version", "GET", "/v2/health"},
		{"trailing slash", "GET", "/v1/health/"},
		{"wrong method on a known path", "DELETE", "/v1/health"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantError(t, doAuth(t, s, tc.method, tc.path), http.StatusNotFound, "not_found")
		})
	}
}

func TestRecoverPanics(t *testing.T) {
	h := recoverPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	wantError(t, rec, http.StatusInternalServerError, "internal")
	if strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("500 body leaks the panic value: %q", rec.Body.String())
	}
}

// handle chooses the response from the error a handler returns, including wrapped errors.
func TestHandleMapsErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"apiError", notFound("x"), 404, "not_found"},
		{"wrapped apiError", fmt.Errorf("db: %w", notFound("x")), 404, "not_found"},
		{"plain error", errors.New("boom"), 500, "internal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := handle(func(http.ResponseWriter, *http.Request) error { return tc.err })
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			wantError(t, rec, tc.wantStatus, tc.wantCode)
		})
	}
}

func TestConnectorList(t *testing.T) {
	rec := doAuth(t, newTestServer(t, &fakeFetcher{}), "GET", "/v1/connectors")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	var got []struct{ ID, Name string }
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "greenhouse" || got[0].Name != "Greenhouse" {
		t.Errorf("connectors = %+v, want just greenhouse", got)
	}
}

func TestGreenhouseFetchSuccess(t *testing.T) {
	f := &fakeSearcher{}
	rec := postFetch(t, newSearchServer(t, f), `{"query":{"keywords":["  data analyst ",""],"boards":["acme","acme","globex"]}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if f.gotQuery.Limit != 20 || f.gotQuery.WorkMode != "Any" {
		t.Errorf("defaults = limit %d, work mode %q; want 20 and Any", f.gotQuery.Limit, f.gotQuery.WorkMode)
	}
	if got := f.gotQuery.Keywords; len(got) != 1 || got[0] != "data analyst" {
		t.Errorf("keywords = %q, want [data analyst]", got)
	}
	if got := f.gotBoards; len(got) != 2 || got[0] != "acme" || got[1] != "globex" {
		t.Errorf("boards = %q, want [acme globex]", got)
	}
	// Nil slices from the searcher must encode as arrays, not null.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"postings", "errors"} {
		if string(raw[key]) != "[]" {
			t.Errorf("%s = %s, want []", key, raw[key])
		}
	}
}

func TestGreenhouseFetchRejectsBadRequests(t *testing.T) {
	kw := `"keywords":["go"]`
	for _, tc := range []struct {
		name, body string
		status     int
		code       string
	}{
		{"malformed JSON", `{"query":`, 400, "bad_request"},
		{"empty body", ``, 400, "bad_request"},
		{"two JSON values", `{"query":{` + kw + `}} {}`, 400, "bad_request"},
		{"unknown top-level field", `{"query":{` + kw + `},"x":1}`, 422, "validation_failed"},
		{"unknown query field", `{"query":{` + kw + `,"salary":1}}`, 422, "validation_failed"},
		{"bad level", `{"query":{` + kw + `,"level":"Wizard"}}`, 422, "validation_failed"},
		{"pay range reversed", `{"query":{` + kw + `,"pay_min":100,"pay_max":50}}`, 422, "validation_failed"},
		{"posted within too long", `{"query":{` + kw + `,"posted_within":400}}`, 422, "validation_failed"},
		{"wrong type", `{"query":{"keywords":"go"}}`, 422, "validation_failed"},
		{"too large", `{"query":{"location":"` + strings.Repeat("x", maxFetchBody) + `"}}`, 422, "validation_failed"},
		{"nothing to search by", `{"query":{}}`, 422, "validation_failed"},
		{"blank keywords and location", `{"query":{"keywords":["  "],"location":" "}}`, 422, "validation_failed"},
		{"too many keywords", `{"query":{"keywords":["a","b","c","d","e","f","g","h","i","j","k"]}}`, 422, "validation_failed"},
		{"keyword too long", `{"query":{"keywords":["` + strings.Repeat("x", maxKeywordLen+1) + `"]}}`, 422, "validation_failed"},
		{"limit too high", `{"query":{` + kw + `,"limit":51}}`, 422, "validation_failed"},
		{"offset negative", `{"query":{` + kw + `,"offset":-1}}`, 422, "validation_failed"},
		{"offset too high", `{"query":{` + kw + `,"offset":5001}}`, 422, "validation_failed"},
		{"limit negative", `{"query":{` + kw + `,"limit":-1}}`, 422, "validation_failed"},
		{"bad work mode", `{"query":{` + kw + `,"work_mode":"Anywhere"}}`, 422, "validation_failed"},
		{"unknown category", `{"query":{` + kw + `,"categories":["nope"]}}`, 422, "validation_failed"},
		{"bad board token", `{"query":{` + kw + `,"boards":["Acme.com"]}}`, 422, "validation_failed"},
		{"too many boards", `{"query":{` + kw + `,"boards":[` + strings.TrimSuffix(strings.Repeat(`"a",`, maxBoards+1), ",") + `]}}`, 422, "validation_failed"},
		{"no boards match", `{"query":{` + kw + `,"company":"zzzz no such company"}}`, 422, "validation_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeSearcher{}
			wantError(t, postFetch(t, newSearchServer(t, f), tc.body), tc.status, tc.code)
			if f.called {
				t.Error("searcher was called for an invalid request")
			}
		})
	}
}

func TestGreenhouseFetchErrors(t *testing.T) {
	body := `{"query":{"keywords":["go"],"boards":["acme"]}}`
	wantError(t, postFetch(t, newSearchServer(t, &fakeSearcher{err: greenhouse.ErrorUpstream}), body), 502, "upstream_error")
	wantError(t, postFetch(t, newSearchServer(t, &fakeSearcher{err: errors.New("secret detail")}), body), 500, "internal")
	// No searcher configured: the route answers 501 instead of panicking.
	wantError(t, postFetch(t, newTestServer(t, &fakeFetcher{}), body), 501, "not_implemented")
}

// The single-posting route was dropped: results carry raw_text, and phase 1's route covers reloads.
func TestGreenhousePostingsRouteIsGone(t *testing.T) {
	wantError(t, doAuth(t, newTestServer(t, &fakeFetcher{}), "GET", "/v1/connectors/greenhouse/postings/1"), 404, "not_found")
}

func TestChooseBoards(t *testing.T) {
	all := greenhouse.Tokens()
	if len(all) == 0 {
		t.Fatal("no curated boards")
	}
	first := greenhouse.Board{}
	for _, c := range greenhouse.Categories() {
		if len(c.Boards) > 0 {
			first = c.Boards[0]
			break
		}
	}

	for _, tc := range []struct {
		name string
		q    connectors.Query
		want func(got []string) string // returns a failure message, or ""
	}{
		{"explicit boards are used as given and de-duplicated", connectors.Query{Boards: []string{"b", "a", "b"}, Company: "ignored", Categories: []string{"nope"}},
			func(got []string) string {
				if strings.Join(got, ",") != "b,a" {
					return "want b,a"
				}
				return ""
			}},
		{"empty boards list means not set", connectors.Query{Boards: []string{}},
			func(got []string) string {
				if len(got) != len(all) {
					return "want every curated board"
				}
				return ""
			}},
		{"no filters gives every curated board once", connectors.Query{},
			func(got []string) string {
				if len(got) != len(all) {
					return "want every curated board"
				}
				return ""
			}},
		{"company filter is case-insensitive and trimmed", connectors.Query{Company: "  " + strings.ToUpper(first.Company) + " "},
			func(got []string) string {
				for _, tok := range got {
					if !strings.Contains(strings.ToLower(greenhouse.CompanyFor(tok)), strings.ToLower(first.Company)) {
						return "returned a board that does not match the company: " + tok
					}
				}
				for _, tok := range got {
					if tok == first.Token {
						return ""
					}
				}
				return "missing " + first.Token
			}},
		{"company matching nothing gives no boards", connectors.Query{Company: "zzzz no such company"},
			func(got []string) string {
				if len(got) != 0 {
					return "want none"
				}
				return ""
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := chooseBoards(tc.q)
			if msg := tc.want(got); msg != "" {
				t.Errorf("%s (got %d boards)", msg, len(got))
			}
			seen := map[string]bool{}
			for _, tok := range got {
				if seen[tok] {
					t.Errorf("duplicate board %q", tok)
				}
				seen[tok] = true
			}
		})
	}

	t.Run("categories narrow the curated set", func(t *testing.T) {
		cats := greenhouse.Categories()
		got := chooseBoards(connectors.Query{Categories: []string{cats[0].ID}})
		if len(got) == 0 || len(got) > len(all) {
			t.Errorf("got %d boards for %q, want between 1 and %d", len(got), cats[0].ID, len(all))
		}
	})
}

func TestGreenhouseFetchWithoutKeywords(t *testing.T) {
	for name, body := range map[string]string{
		"location": `{"query":{"location":"Charleston, SC"}}`,
		"industry": `{"query":{"categories":["technical"]}}`,
	} {
		f := &fakeSearcher{}
		rec := postFetch(t, newSearchServer(t, f), body)
		if rec.Code != http.StatusOK || !f.called || len(f.gotQuery.Keywords) != 0 {
			t.Errorf("%s: status %d, called %v, keywords %v (body %q)", name, rec.Code, f.called, f.gotQuery.Keywords, rec.Body.String())
		}
	}
}

func TestCategoryList(t *testing.T) {
	s := newSearchServer(t, &fakeSearcher{})
	req := httptest.NewRequest("GET", "/v1/connectors/greenhouse/categories", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	var got struct {
		Categories []struct {
			ID         string  `json:"id"`
			Name       string  `json:"name"`
			Parent     *string `json:"parent"`
			BoardCount int     `json:"board_count"`
		} `json:"categories"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got.Categories) == 0 {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if c := got.Categories[0]; c.ID == "" || c.Name == "" || c.BoardCount == 0 {
		t.Errorf("first category = %+v", c)
	}
}
