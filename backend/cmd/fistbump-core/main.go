// Command fistbump-core is the local backend for the FistBump desktop app.
//
// Contract with Electron: flag --data-dir, env FISTBUMP_TOKEN, one "PORT=n" line on stdout,
// logs on stderr, exit when stdin closes.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/api"
	"github.com/tlmcguire/fistbump/backend/internal/db"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags)
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	dataDir := flag.String("data-dir", "", "directory for the database and models")
	llama := flag.String("llama-server", "", "path to the llama-server binary (optional)")
	flag.Parse()
	if *dataDir == "" {
		return errors.New("--data-dir is required")
	}
	token := os.Getenv("FISTBUMP_TOKEN")
	if token == "" {
		return errors.New("FISTBUMP_TOKEN is not set")
	}
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		return err
	}
	conn, err := db.Open(filepath.Join(*dataDir, api.DBFile))
	if err != nil {
		return err
	}
	defer conn.Close()

	srv := api.New(api.Config{Store: store.New(conn), DataDir: *dataDir, Token: token, LlamaBin: *llama})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	serveErr := make(chan error, 1)
	go func() { serveErr <- httpSrv.Serve(ln) }()
	// stdout carries only this line.
	fmt.Fprintf(os.Stdout, "PORT=%d\n", ln.Addr().(*net.TCPAddr).Port)

	stdinClosed := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		close(stdinClosed)
	}()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-stdinClosed:
		log.Println("stdin closed, shutting down")
	case s := <-sigs:
		log.Println("signal", s, "shutting down")
	case err := <-serveErr:
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	srv.Shutdown()
	return nil
}
