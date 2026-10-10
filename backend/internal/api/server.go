package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
	"github.com/tlmcguire/fistbump/backend/internal/connectors/greenhouse"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

const Version = "0.1.0"

// Config carries everything the server needs.
type Config struct {
	Store      *store.Store
	DataDir    string
	Token      string
	LlamaBin   string
	Greenhouse *greenhouse.Connector
}

// Server holds dependencies shared by all handlers.
type Server struct {
	cfg        Config
	st         *store.Store
	engines    *ai.Engines
	downloads  *ai.Downloader
	gh         *greenhouse.Connector
	mux        *http.ServeMux
	background context.Context
	stopBG     context.CancelFunc

	mu      sync.Mutex
	running map[int64]context.CancelFunc // revision id -> cancel
	wg      sync.WaitGroup
}

func logf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s "+format+"\n", append([]any{time.Now().Format("15:04:05")}, a...)...)
}

// New builds the server and registers all routes.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg, st: cfg.Store, running: map[int64]context.CancelFunc{}}
	s.background, s.stopBG = context.WithCancel(context.Background())
	s.gh = cfg.Greenhouse
	if s.gh == nil {
		s.gh = greenhouse.New(filepath.Join(cfg.DataDir, "cache"))
	}
	s.downloads = ai.NewDownloader(cfg.DataDir)
	remote := ai.NewRemote()
	local := ai.NewLocal(cfg.DataDir, cfg.LlamaBin,
		func() string { return s.st.SettingString("ai.selected_model") },
		func() time.Duration { return time.Duration(s.st.SettingInt("ai.idle_minutes")) * time.Minute })
	s.engines = &ai.Engines{Local: local, Remote: remote}
	// The key is never stored. Base URL and model come back from settings; Electron re-sends the key.
	if base, model := s.st.SettingString("ai.remote.base_url"), s.st.SettingString("ai.remote.model"); base != "" {
		remote.Configure(base, model, "")
	}
	// A model downloaded outside the app (or before a selection was saved) is used automatically.
	if s.st.SettingString("ai.selected_model") == "" {
		for _, m := range ai.List(cfg.DataDir, "") {
			if m.Installed {
				_ = s.st.SetSetting("ai.selected_model", m.ID)
				logf("selected installed model %s", m.ID)
				break
			}
		}
	}
	// Generation runs in memory. A revision left queued or running by a previous process will never finish.
	if n, err := s.st.FailInterruptedRevisions(); err == nil && n > 0 {
		logf("marked %d interrupted revisions as failed", n)
	}
	s.mux = http.NewServeMux()
	s.routes()
	return s
}

// Handler returns the root handler with recovery, auth and logging applied.
func (s *Server) Handler() http.Handler {
	return recoverMW(logMW(s.authMW(s.mux)))
}

// Shutdown cancels background work and waits for it, then stops child processes.
func (s *Server) Shutdown() {
	s.stopBG()
	s.downloads.CancelAll()
	s.wg.Wait()
	s.engines.Local.Stop()
}

func (s *Server) authMW(next http.Handler) http.Handler {
	want := []byte("Bearer " + s.cfg.Token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if s.cfg.Token == "" || subtle.ConstantTimeCompare(got, want) != 1 {
			writeError(w, errf(401, "unauthorized", "missing or invalid token"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

// logMW logs method, path and status. It never logs headers or bodies.
func logMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		next.ServeHTTP(sw, r)
		path := r.URL.Path
		if strings.HasPrefix(path, "/v1/ai/remote") || strings.HasPrefix(path, "/v1/models/token") {
			r.URL.RawQuery = ""
		}
		logf("%s %s %d %s", r.Method, path, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

func recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logf("panic: %v", rec)
				writeError(w, errf(500, "internal", "internal error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	m := s.mux
	h := func(pattern string, fn handler) { m.HandleFunc(pattern, wrap(fn)) }

	h("GET /v1/health", s.health)

	h("GET /v1/resume", s.getResume)
	h("PUT /v1/resume/profile", s.putProfile)
	h("GET /v1/resume/experiences", s.listExperiences)
	h("POST /v1/resume/experiences", s.createExperience)
	h("PUT /v1/resume/experiences/{id}", s.updateExperience)
	h("DELETE /v1/resume/experiences/{id}", s.deleteExperience)
	h("GET /v1/resume/education", s.listEducation)
	h("POST /v1/resume/education", s.createEducation)
	h("PUT /v1/resume/education/{id}", s.updateEducation)
	h("DELETE /v1/resume/education/{id}", s.deleteEducation)
	h("POST /v1/resume/import", s.importResume)
	h("POST /v1/resume/import/apply", s.applyImport)
	h("POST /v1/resume/summary", s.resumeSummary)
	h("GET /v1/resume/preview", s.resumePreview)
	h("POST /v1/resume/export", s.exportMaster)

	h("GET /v1/resumes", s.listResumes)
	h("POST /v1/resumes", s.createResume)
	h("GET /v1/resumes/{id}", s.getResumeVersion)
	h("PUT /v1/resumes/{id}", s.updateResume)
	h("DELETE /v1/resumes/{id}", s.deleteResume)

	h("POST /v1/jobs/parse", s.parseJob)
	h("POST /v1/jobs", s.createJob)
	h("GET /v1/jobs", s.listJobs)
	h("GET /v1/jobs/{id}", s.getJob)
	h("PUT /v1/jobs/{id}", s.updateJob)
	h("DELETE /v1/jobs/{id}", s.deleteJob)
	h("POST /v1/jobs/{id}/analyze", s.analyzeJob)
	h("POST /v1/jobs/{id}/archive", s.archiveJob(true))
	h("POST /v1/jobs/{id}/unarchive", s.archiveJob(false))

	h("POST /v1/revisions", s.createRevision)
	h("GET /v1/revisions", s.listRevisions)
	h("GET /v1/revisions/{id}", s.getRevision)
	h("POST /v1/revisions/{id}/cancel", s.cancelRevision)
	h("DELETE /v1/revisions/{id}", s.deleteRevision)
	h("POST /v1/revisions/{id}/suggestions/{sid}/accept", s.suggestionState("accepted"))
	h("POST /v1/revisions/{id}/suggestions/{sid}/reject", s.suggestionState("rejected"))
	h("POST /v1/revisions/{id}/suggestions/{sid}/edit", s.editSuggestion)

	h("POST /v1/tailored-resumes", s.createTailored)
	h("GET /v1/tailored-resumes", s.listTailored)
	h("GET /v1/tailored-resumes/{id}", s.getTailored)
	h("GET /v1/tailored-resumes/{id}/diff", s.diffTailored)
	h("POST /v1/tailored-resumes/{id}/export", s.exportTailored)
	h("DELETE /v1/tailored-resumes/{id}", s.deleteTailored)

	h("GET /v1/applications", s.listApplications)
	h("POST /v1/applications", s.createApplication)
	h("GET /v1/applications/{id}", s.getApplication)
	h("PUT /v1/applications/{id}", s.updateApplication)
	h("DELETE /v1/applications/{id}", s.deleteApplication)
	h("POST /v1/applications/bulk-status", s.bulkApplicationStatus)

	h("GET /v1/connectors", s.listConnectors)
	h("PUT /v1/connectors/{id}", s.putConnector)
	h("POST /v1/connectors/{id}/fetch", s.fetchConnector)
	h("POST /v1/connectors/{id}/import", s.importConnector)
	h("GET /v1/connectors/greenhouse/categories", s.listCategories)
	h("PUT /v1/connectors/greenhouse/categories/{category}", s.putCategory)
	h("POST /v1/connectors/greenhouse/boards", s.addBoard)
	h("GET /v1/connectors/greenhouse/postings/{external_id}", s.getPosting)
	h("DELETE /v1/connectors/greenhouse/boards/{board}", s.removeBoard)

	h("GET /v1/ai/status", s.aiStatus)
	h("PUT /v1/ai/mode", s.putAIMode)
	h("PUT /v1/ai/remote", s.putAIRemote)
	h("DELETE /v1/ai/remote", s.deleteAIRemote)
	h("POST /v1/ai/test", s.aiTest)

	h("GET /v1/models", s.listModels)
	h("PUT /v1/models/token", s.putModelToken)
	h("DELETE /v1/models/token", s.deleteModelToken)
	h("GET /v1/models/downloads/{download_id}", s.getDownload)
	h("DELETE /v1/models/downloads/{download_id}", s.cancelDownload)
	h("GET /v1/models/{id}", s.getModel)
	h("POST /v1/models/{id}/download", s.startDownload)
	h("DELETE /v1/models/{id}", s.deleteModel)

	h("GET /v1/settings", s.getSettings)
	h("PUT /v1/settings", s.putSettings)

	h("GET /v1/storage", s.getStorage)
	h("POST /v1/storage/cleanup", s.cleanupStorage)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) error {
	var v int
	if err := s.st.DB.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "version": Version, "schema_version": v})
	return nil
}
