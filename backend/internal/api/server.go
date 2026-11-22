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

	"github.com/tlmcguire/fistbump/backend/internal/ai"
	"github.com/tlmcguire/fistbump/backend/internal/analyze"
	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/logos"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/parser"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

// jobFetcher is the part of the Greenhouse client the API uses, so tests can pass a fake.
type jobFetcher interface {
	FetchJob(ctx context.Context, board, jobID string) (connectors.Posting, error)
}

// jobSearcher is the part of the Greenhouse client the search route uses, so tests can pass a fake.
type jobSearcher interface {
	Search(ctx context.Context, q connectors.Query, boards []string) (connectors.SearchResult, error)
}

// logoFinder is the part of the logos package the logo route uses, so tests can pass a fake.
type logoFinder interface {
	Logo(ctx context.Context, site, board string) (logos.Logo, error)
}

// Config is what main.go and tests pass to New. Fields are exported so other packages can set them.
type Config struct {
	DB         *sql.DB
	Greenhouse jobFetcher
	Searcher   jobSearcher
	Logos      logoFinder
	DataDir    string         // where the database and other app data live, for the storage report
	Caches     []CacheClearer // emptied by storage cleanup
	Token      string         // per-launch bearer token from FISTBUMP_TOKEN
	Version    string

	// Engines write resume suggestions. Rules is filled in when left nil, so a zero Config works.
	// Remote and Local are filled from the fields below when left nil.
	Engines ai.Engines
	// Remote is the OpenAI-compatible client the AI routes configure. Nil means a new, empty one.
	Remote *ai.Remote
	// Local runs llama-server. Nil means no local engine (tests, or a build without the binary).
	Local *ai.Local
	// Downloads fetches models into DataDir. Nil means a new manager for DataDir.
	Downloads *ai.Downloads
	// PDF turns resume Markdown into a PDF titled after fileName (the download name, extension
	// included). Without it, PDF export fails and offers Markdown.
	PDF func(markdown, fileName string) ([]byte, error)
	// BaseContext is the parent of every background round. Cancel it to stop them at shutdown.
	// Nil means context.Background().
	BaseContext context.Context
}

// Server holds the dependencies handlers need. Handlers are methods on it so they can reach them.
type Server struct {
	db      *sql.DB
	store   *store.Store
	gh      jobFetcher
	search  jobSearcher
	logos   logoFinder
	dataDir string
	caches  []CacheClearer
	token   string
	version string
	// parse and analyze are parser.Parse and analyze.Run; tests swap them for fixed results.
	parse     func(raw string) parser.Result
	analyze   func(job models.Job, m models.Master) analyze.Result
	rounds    *roundRunner // background suggestion rounds, see revisions.go
	remote    *ai.Remote
	local     *ai.Local
	downloads *ai.Downloads
	// renderPDF makes the PDF for the export routes; tests swap it for a fake.
	renderPDF func(markdown, fileName string) ([]byte, error)
	handler   http.Handler // the mux wrapped in middleware, built once in New
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
		{"GET /v1/connectors/greenhouse/logo", s.companyLogo},
		{"GET /v1/storage", s.storage},
		{"POST /v1/storage/cleanup", s.storageCleanup},
		{"GET /v1/connectors/greenhouse/link", s.greenhouseLink},
		{"GET /v1/resume", s.resumeGet},
		{"PUT /v1/resume/profile", s.profilePut},
		{"GET /v1/resume/experiences", s.experienceList},
		{"POST /v1/resume/experiences", s.experienceCreate},
		{"PUT /v1/resume/experiences/{id}", s.experienceUpdate},
		{"DELETE /v1/resume/experiences/{id}", s.experienceDelete},
		{"GET /v1/resume/education", s.educationList},
		{"POST /v1/resume/education", s.educationCreate},
		{"PUT /v1/resume/education/{id}", s.educationUpdate},
		{"DELETE /v1/resume/education/{id}", s.educationDelete},
		{"POST /v1/resume/import", s.resumeImport},
		{"POST /v1/resume/import/apply", s.resumeImportApply},
		{"GET /v1/resume/preview", s.resumePreview},
		{"POST /v1/resume/export", s.resumeExport},
		{"GET /v1/resumes", s.resumeList},
		{"POST /v1/resumes", s.resumeCreate},
		{"GET /v1/resumes/{id}", s.resumeGetOne},
		{"PUT /v1/resumes/{id}", s.resumeUpdate},
		{"DELETE /v1/resumes/{id}", s.resumeDelete},
		{"GET /v1/settings", s.settingsGet},
		{"PUT /v1/settings", s.settingsPut},
		{"GET /v1/connectors/greenhouse/boards/{board}/jobs/{job_id}", s.greenhouseJob},
		{"POST /v1/connectors/greenhouse/import", s.greenhouseImport},
		{"POST /v1/jobs/parse", s.jobParse},
		{"POST /v1/jobs", s.jobCreate},
		{"GET /v1/jobs", s.jobList},
		{"GET /v1/jobs/{id}", s.jobGet},
		{"PUT /v1/jobs/{id}", s.jobUpdate},
		{"DELETE /v1/jobs/{id}", s.jobDelete},
		{"POST /v1/jobs/{id}/analyze", s.jobAnalyze},
		{"POST /v1/jobs/{id}/archive", s.jobArchive},
		{"POST /v1/jobs/{id}/unarchive", s.jobUnarchive},
		{"POST /v1/jobs/{id}/trash", s.jobTrash},
		{"POST /v1/jobs/{id}/restore", s.jobRestore},
		{"GET /v1/applications", s.appList},
		{"POST /v1/applications", s.appCreate},
		{"GET /v1/applications/{id}", s.appGet},
		{"PUT /v1/applications/{id}", s.appUpdate},
		{"DELETE /v1/applications/{id}", s.appDelete},
		{"POST /v1/applications/bulk-status", s.appBulkStatus},
		{"POST /v1/applications/{id}/archive", s.appArchive},
		{"POST /v1/applications/{id}/unarchive", s.appUnarchive},
		{"POST /v1/applications/bulk-archive", s.appBulkArchive},
		{"POST /v1/revisions", s.revisionCreate},
		{"GET /v1/revisions", s.revisionList},
		{"GET /v1/revisions/{id}", s.revisionGet},
		{"POST /v1/revisions/{id}/cancel", s.revisionCancel},
		{"DELETE /v1/revisions/{id}", s.revisionDelete},
		{"POST /v1/revisions/{id}/suggestions/{sid}/accept", s.suggestionAccept},
		{"POST /v1/revisions/{id}/suggestions/{sid}/reject", s.suggestionReject},
		{"POST /v1/revisions/{id}/suggestions/{sid}/edit", s.suggestionEdit},
		{"POST /v1/tailored-resumes", s.tailoredCreate},
		{"GET /v1/tailored-resumes", s.tailoredList},
		{"GET /v1/tailored-resumes/{id}", s.tailoredGet},
		{"GET /v1/tailored-resumes/{id}/diff", s.tailoredDiff},
		{"POST /v1/tailored-resumes/{id}/export", s.tailoredExport},
		{"DELETE /v1/tailored-resumes/{id}", s.tailoredDelete},
		{"GET /v1/ai/status", s.aiStatusGet},
		{"PUT /v1/ai/mode", s.aiModePut},
		{"PUT /v1/ai/remote", s.aiRemotePut},
		{"DELETE /v1/ai/remote", s.aiRemoteDelete},
		{"POST /v1/ai/test", s.aiTest},
		{"GET /v1/models", s.modelList},
		{"GET /v1/models/{id}", s.modelGet},
		{"POST /v1/models/{id}/download", s.modelDownload},
		{"GET /v1/models/downloads/{download_id}", s.downloadStatus},
		{"DELETE /v1/models/downloads/{download_id}", s.downloadCancel},
		{"DELETE /v1/models/{id}", s.modelDelete},
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
		store:   store.New(cfg.DB),
		gh:      cfg.Greenhouse,
		search:  cfg.Searcher,
		logos:   cfg.Logos,
		dataDir: cfg.DataDir,
		caches:  cfg.Caches,
		token:   cfg.Token,
		version: cfg.Version,
		parse:   parser.Parse,
		analyze: analyze.Run,

		renderPDF: cfg.PDF,
	}
	s.remote = cfg.Remote
	if s.remote == nil {
		s.remote = ai.NewRemote()
	}
	s.local = cfg.Local
	s.downloads = cfg.Downloads
	if s.downloads == nil {
		s.downloads = ai.NewDownloads(cfg.DataDir)
	}
	engines := cfg.Engines
	if engines.Rules == nil {
		engines.Rules = ai.Rules{}
	}
	if engines.Remote == nil {
		engines.Remote = s.remote
	}
	if engines.Local == nil && s.local != nil {
		engines.Local = s.local
	}
	base := cfg.BaseContext
	if base == nil {
		base = context.Background()
	}
	s.rounds = newRoundRunner(base, engines, s.failRound)
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
