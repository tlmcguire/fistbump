// Package api serves the backend's HTTP API on loopback, behind a per-launch bearer token.
package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
)

// jobFetcher is the part of the Greenhouse client the API uses, so tests can pass a fake.
type jobFetcher interface {
	FetchJob(ctx context.Context, board, jobID string) (connectors.Posting, error)
}

// jobSearcher is the part of the Greenhouse client the search route uses, so tests can pass a fake.
type jobSearcher interface {
	Search(ctx context.Context, q connectors.Query, boards []string) (connectors.SearchResult, error)
}

// Config is what main.go and tests pass to New. Fields are exported so other packages can set them.
type Config struct {
	DB         *sql.DB
	Greenhouse jobFetcher
	Searcher   jobSearcher
	Token      string // per-launch bearer token from FISTBUMP_TOKEN
	Version    string
}

// Server holds the dependencies handlers need. Handlers are methods on it so they can reach them.
type Server struct {
	db      *sql.DB
	gh      jobFetcher
	search  jobSearcher
	token   string
	version string
	handler http.Handler // the mux wrapped in middleware, built once in New
}

// route pairs a mux pattern ("METHOD /path/{wildcard}") with its handler.
type route struct {
	pattern string
	h       handlerFunc
}

// routes lists the routes that are built. Each one replaces its 501 stub in New.
func (s *Server) routes() []route {
	return []route{
		{"GET /v1/health", s.health},
		{"GET /v1/connectors", s.connectorList},
		{"POST /v1/connectors/greenhouse/fetch", s.greenhouseFetch},
		{"GET /v1/connectors/greenhouse/categories", s.categoryList},
		{"GET /v1/connectors/greenhouse/link", s.greenhouseLink},
		{"GET /v1/connectors/greenhouse/boards/{board}/jobs/{job_id}", s.greenhouseJob},
	}
}

// unbuiltRoutePatterns lists every other route in docs/api.md, so the ones not built yet answer 501.
// Keep it in sync with docs/api.md. Routes in routes() are skipped, so listing one here is harmless.
func unbuiltRoutePatterns() []string {
	return []string{
		// Resume
		"GET /v1/resume",
		"PUT /v1/resume/profile",
		"GET /v1/resume/experiences",
		"POST /v1/resume/experiences",
		"PUT /v1/resume/experiences/{id}",
		"DELETE /v1/resume/experiences/{id}",
		"GET /v1/resume/education",
		"POST /v1/resume/education",
		"PUT /v1/resume/education/{id}",
		"DELETE /v1/resume/education/{id}",
		// Resume import and generation
		"POST /v1/resume/import",
		"POST /v1/resume/import/apply",
		"POST /v1/resume/summary",
		"GET /v1/resume/preview",
		"POST /v1/resume/export",
		// Saved resumes
		"GET /v1/resumes",
		"POST /v1/resumes",
		"GET /v1/resumes/{id}",
		"PUT /v1/resumes/{id}",
		"DELETE /v1/resumes/{id}",
		// Jobs
		"POST /v1/jobs/parse",
		"POST /v1/jobs",
		"GET /v1/jobs",
		"GET /v1/jobs/{id}",
		"PUT /v1/jobs/{id}",
		"DELETE /v1/jobs/{id}",
		"POST /v1/jobs/{id}/analyze",
		"POST /v1/jobs/{id}/archive",
		"POST /v1/jobs/{id}/unarchive",
		"POST /v1/jobs/{id}/trash",
		"POST /v1/jobs/{id}/restore",
		// Revisions
		"POST /v1/revisions",
		"GET /v1/revisions",
		"GET /v1/revisions/{id}",
		"POST /v1/revisions/{id}/cancel",
		"DELETE /v1/revisions/{id}",
		"POST /v1/revisions/{id}/suggestions/{sid}/accept",
		"POST /v1/revisions/{id}/suggestions/{sid}/reject",
		"POST /v1/revisions/{id}/suggestions/{sid}/edit",
		// Tailored resumes
		"POST /v1/tailored-resumes",
		"GET /v1/tailored-resumes",
		"GET /v1/tailored-resumes/{id}",
		"GET /v1/tailored-resumes/{id}/diff",
		"POST /v1/tailored-resumes/{id}/export",
		"DELETE /v1/tailored-resumes/{id}",
		// Applications
		"GET /v1/applications",
		"POST /v1/applications",
		"GET /v1/applications/{id}",
		"PUT /v1/applications/{id}",
		"DELETE /v1/applications/{id}",
		"POST /v1/applications/bulk-status",
		"POST /v1/applications/{id}/archive",
		"POST /v1/applications/{id}/unarchive",
		"POST /v1/applications/bulk-archive",
		// Connectors
		"GET /v1/connectors",
		"POST /v1/connectors/{id}/fetch",
		"POST /v1/connectors/{id}/import",
		"GET /v1/connectors/greenhouse/categories",
		"PUT /v1/connectors/greenhouse/categories/{category}",
		"POST /v1/connectors/greenhouse/boards",
		"DELETE /v1/connectors/greenhouse/boards/{board}",
		// AI
		"GET /v1/ai/status",
		"PUT /v1/ai/mode",
		"PUT /v1/ai/remote",
		"DELETE /v1/ai/remote",
		"POST /v1/ai/test",
		// Models
		"GET /v1/models",
		"GET /v1/models/{id}",
		"POST /v1/models/{id}/download",
		"GET /v1/models/downloads/{download_id}",
		"DELETE /v1/models/downloads/{download_id}",
		"DELETE /v1/models/{id}",
		"PUT /v1/models/token",
		"DELETE /v1/models/token",
		// Settings
		"GET /v1/settings",
		"PUT /v1/settings",
		// Storage
		"GET /v1/storage",
		"POST /v1/storage/cleanup",
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// New builds the router. Unbuilt routes return 501.
func New(cfg Config) *Server {
	s := &Server{
		db:      cfg.DB,
		gh:      cfg.Greenhouse,
		search:  cfg.Searcher,
		token:   cfg.Token,
		version: cfg.Version,
	}
	mux := http.NewServeMux()

	// A set of built patterns, because registering the same pattern twice panics at startup.
	built := make(map[string]bool)
	for _, rt := range s.routes() {
		mux.Handle(rt.pattern, handle(rt.h))
		built[rt.pattern] = true
	}

	stub := handle(func(w http.ResponseWriter, r *http.Request) error {
		return notImplemented()
	})
	for _, pattern := range unbuiltRoutePatterns() {
		if !built[pattern] {
			mux.Handle(pattern, stub)
		}
	}

	// "/" matches every path no other pattern claims, so unknown paths get a JSON 404.
	mux.Handle("/", handle(func(w http.ResponseWriter, r *http.Request) error {
		return notFound("Route not found")
	}))

	// Logging is outermost so it sees every final status, including 401s and recovered panics.
	// The token check wraps the whole mux, so unknown paths ask for a token too.
	s.handler = logRequests(recoverPanics(s.requireToken(mux)))
	return s
}

// health reports that the backend is up and which schema version the database is on.
// Electron calls it after reading the port line.
func (s *Server) health(w http.ResponseWriter, r *http.Request) error {
	var schemaVersion int
	// PRAGMA user_version is where db.Open records the last migration it applied.
	// https://www.sqlite.org/pragma.html#pragma_user_version
	if err := s.db.QueryRowContext(r.Context(), "PRAGMA user_version").Scan(&schemaVersion); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	writeJSON(w, http.StatusOK, struct {
		Status        string `json:"status"`
		Version       string `json:"version"`
		SchemaVersion int    `json:"schema_version"`
	}{"ok", s.version, schemaVersion})
	return nil
}

// requireToken rejects requests without "Authorization: Bearer <token>". The comparison is
// constant-time so response timing cannot be used to guess the token.
func (s *Server) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		// Two empty strings compare equal, so an unset token must never match.
		if !ok || s.token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			writeError(w, unauthorized())
			return
		}
		next.ServeHTTP(w, r)
	})
}

// recoverPanics turns a panic in a handler into a 500, so one bad request cannot stop the backend.
func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			// net/http uses this panic value to abort a response on purpose; let it through.
			if v == http.ErrAbortHandler {
				panic(v)
			}
			slog.Error("panic", "path", r.URL.Path, "value", v, "stack", string(debug.Stack()))
			writeError(w, internal())
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder remembers the status code for logging. It starts at 200 because a handler
// that never calls WriteHeader sends 200.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) Unwrap() http.ResponseWriter { return rec.ResponseWriter }

// logRequests logs method, path, status and duration to stderr. It never logs headers, bodies or
// the query string, which can hold a pasted job link or the token.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start))
	})
}
