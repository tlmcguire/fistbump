// Command fistbump-core is the backend process Electron starts. It serves the API on loopback,
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

	"github.com/tlmcguire/fistbump/backend/internal/api"
	"github.com/tlmcguire/fistbump/backend/internal/connectors/greenhouse"
	"github.com/tlmcguire/fistbump/backend/internal/db"
	"github.com/tlmcguire/fistbump/backend/internal/logos"
)

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

	handler := api.New(api.Config{
		DB:         conn,
		Greenhouse: gh,
		Searcher:   gh,
		Logos:      logoFetcher,
		Token:      token,
		Version:    version,
	})

	// Port 0 lets the OS pick a free port. Binding to 127.0.0.1 keeps the API off the network.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listening: %w", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	// Electron reads this line to find the port. Everything else goes to stderr.
	fmt.Printf("PORT=%d\n", listener.Addr().(*net.TCPAddr).Port)

	// stdin is a pipe from Electron, so it closes when Electron quits or crashes.
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

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "error", err)
		return err
	}
	return nil
}
