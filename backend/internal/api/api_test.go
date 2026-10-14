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
