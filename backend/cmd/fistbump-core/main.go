// Command fistbump-core is the backend process the Tauri host starts. It serves the API on loopback,
// prints the chosen port on stdout, and exits when stdin closes.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
	"github.com/tlmcguire/fistbump/backend/internal/api"
	"github.com/tlmcguire/fistbump/backend/internal/connectors/greenhouse"
	"github.com/tlmcguire/fistbump/backend/internal/cost"
	"github.com/tlmcguire/fistbump/backend/internal/db"
	"github.com/tlmcguire/fistbump/backend/internal/logos"
	"github.com/tlmcguire/fistbump/backend/internal/pdf"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

// settingDays reads a number of days from the settings, falling back to def when it is missing
// or below one.
func settingDays(value any, def int) int {
	n, _ := value.(int)
	if n < 1 {
		return def
	}
	return n
}

// version is reported by GET /v1/health. Release builds can set it with -ldflags "-X main.version=...".
var version = "dev"

const (
	dbFile          = "fistbump.db"
	shutdownTimeout = 5 * time.Second
	cacheMaxAge     = 24 * time.Hour
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fistbump-core:", err)
		os.Exit(1)
	}
}

func run() error {
	dataDir := flag.String("data-dir", "", "directory for the database and other app data")
	flag.Parse()

	token := os.Getenv("FISTBUMP_TOKEN")
	if token == "" {
		return errors.New("FISTBUMP_TOKEN is not set")
	}
	if *dataDir == "" {
		return errors.New("--data-dir is required")
	}
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	conn, err := db.Open(filepath.Join(*dataDir, dbFile))
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer conn.Close()

	st := store.New(conn)

	// A round left queued or running by the last launch can never finish, so mark it failed.
	if n, err := st.FailInterrupted(context.Background()); err != nil {
		slog.Warn("marking interrupted rounds", "error", err)
	} else if n > 0 {
		slog.Info("marked interrupted rounds as failed", "rounds", n)
	}

	// Jobs are cleaned up at startup: an old job nobody tracks is thrown away, and a job in
	// the Trash is deleted once it has been there for jobs.trash_days. The check runs at startup
	// only: the app is not always on, and a job can wait for the next launch.
	values, err := st.GetSettings(context.Background())
	if err != nil {
		slog.Warn("reading the settings for the job cleanup", "error", err)
	} else {
		retentionDays := settingDays(values["jobs.retention_days"], 30)
		trashDays := settingDays(values["jobs.trash_days"], 7)
		if n, err := st.OldJobs(context.Background(), retentionDays, false); err != nil {
			slog.Warn("removing old jobs", "error", err)
		} else if n > 0 {
			slog.Info("removed old jobs", "jobs", n, "days", retentionDays)
		}
		if n, err := st.TrashedJobs(context.Background(), trashDays, false); err != nil {
			slog.Warn("purging the trash", "error", err)
		} else if n > 0 {
			slog.Info("purged the trash", "jobs", n, "days", trashDays)
		}
	}

	// Suggestion rounds and background updaters run under this context; it is canceled before shutdown.
	roundsCtx, stopRounds := context.WithCancel(context.Background())
	defer stopRounds()

	// Refresh cost-of-living, pay, tax, and benefits source citations in-app every 30 days.
	cost.StartUpdater(roundsCtx, *dataDir, "")

	// Search results are cached in the database. Files from the earlier on-disk cache are not used.
	os.RemoveAll(filepath.Join(*dataDir, "cache", "greenhouse"))
	gh := greenhouse.NewClient()
	gh.Cache = greenhouse.NewCache(conn)
	// Cached results older than a day are of no use: listings revalidate after 6 hours and descriptions expire after 24.
	if n, err := gh.Cache.PurgeOlderThan(cacheMaxAge); err != nil {
		slog.Warn("purging the search cache", "error", err)
	} else if n > 0 {
		slog.Info("purged old search cache", "rows", n)
	}

	logoFetcher := logos.New(conn)
	if _, err := logoFetcher.PurgeOlderThan(cacheMaxAge); err != nil {
		slog.Warn("purging the logo cache", "error", err)
	}

	// A long-running app would otherwise keep a day-old cache until the next launch.
	go func() {
		for range time.Tick(time.Hour) {
			if _, err := gh.Cache.PurgeOlderThan(cacheMaxAge); err != nil {
				slog.Warn("purging the search cache", "error", err)
			}
			if _, err := logoFetcher.PurgeOlderThan(cacheMaxAge); err != nil {
				slog.Warn("purging the logo cache", "error", err)
			}
		}
	}()

	// The AI engines. The remote key is not stored here; the Tauri host sends it with PUT /v1/ai/remote
	// after every start. A keyless provider on this computer (Ollama) works from the settings alone.
	settings := st
	remote := ai.NewRemote()
	if values, err := settings.GetSettings(context.Background()); err == nil {
		baseURL, _ := values["ai.remote.base_url"].(string)
		model, _ := values["ai.remote.model"].(string)
		if baseURL != "" {
			if err := remote.Configure(baseURL, model, ""); err != nil {
				slog.Warn("ignoring the saved remote provider", "error", err)
			}
		}
	}
	local := ai.NewLocal(*dataDir, func() (string, time.Duration) {
		values, err := settings.GetSettings(context.Background())
		if err != nil {
			return "", 0
		}
		id, _ := values["ai.selected_model"].(string)
		minutes, _ := values["ai.idle_minutes"].(int)
		return id, time.Duration(minutes) * time.Minute
	})
	if local.Binary == "" {
		slog.Info("llama-server is not next to fistbump-core; the local engine is off")
	}
	defer local.Stop()

	handler := api.New(api.Config{
		DB:          conn,
		Greenhouse:  gh,
		Searcher:    gh,
		Logos:       logoFetcher,
		DataDir:     *dataDir,
		Caches:      []api.CacheClearer{gh.Cache, logoFetcher},
		Token:       token,
		Version:     version,
		PDF:         pdf.RenderFile,
		PDFStyled:   pdf.RenderStyled,
		Engines:     ai.Engines{Local: local, Remote: remote, Rules: ai.Rules{}},
		Remote:      remote,
		Local:       local,
		BaseContext: roundsCtx,
	})

	// Port 0 lets the OS pick a free port. Binding to 127.0.0.1 keeps the API off the network.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listening: %w", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	// The Tauri host reads this line to find the port. Everything else goes to stderr.
	fmt.Printf("PORT=%d\n", listener.Addr().(*net.TCPAddr).Port)

	// stdin is a pipe from the Tauri host, so it closes when the app quits or crashes.
	stdinClosed := make(chan struct{})
	go func() {
		io.Copy(io.Discard, os.Stdin)
		close(stdinClosed)
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serving: %w", err)
	case <-stdinClosed:
	}

	stopRounds()
	local.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "error", err)
		return err
	}
	return nil
}
