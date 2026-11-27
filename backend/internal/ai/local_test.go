package ai

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"testing"
	"time"
)

// TestMain doubles as a fake llama-server: when FISTBUMP_FAKE_LLAMA is set, the test binary serves
// /health (503 while "loading", then 200) and /v1/chat/completions, and exits on interrupt.
func TestMain(m *testing.M) {
	if os.Getenv("FISTBUMP_FAKE_LLAMA") == "1" {
		fakeLlamaServer()
		return
	}
	os.Exit(m.Run())
}

func fakeLlamaServer() {
	fs := flag.NewFlagSet("llama-server", flag.ExitOnError)
	model := fs.String("model", "", "")
	host := fs.String("host", "", "")
	port := fs.Int("port", 0, "")
	fs.Int("ctx-size", 0, "")
	fs.Bool("no-webui", false, "")
	fs.Parse(os.Args[1:])
	ready := time.Now().Add(300 * time.Millisecond)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if time.Now().Before(ready) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		key := os.Getenv("LLAMA_API_KEY")
		if key == "" || r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": "pong from " + *model}}},
		})
	})
	srv := &http.Server{Addr: fmt.Sprintf("%s:%d", *host, *port), Handler: mux}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	go func() { <-stop; srv.Close() }()
	srv.ListenAndServe()
}

// installFake makes a sparse file with the catalog size, so Installed reports true instantly.
func installFake(t *testing.T, dir string, m Model) {
	t.Helper()
	os.MkdirAll(ModelsDir(dir), 0o700)
	f, err := os.Create(m.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(m.SizeBytes); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

type fakeSettings struct {
	mu    sync.Mutex
	model string
	idle  time.Duration
}

func (s *fakeSettings) get() (string, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.model, s.idle
}

func (s *fakeSettings) set(model string) {
	s.mu.Lock()
	s.model = model
	s.mu.Unlock()
}

func newFakeLocal(t *testing.T, idle time.Duration) (*Local, *fakeSettings) {
	t.Helper()
	dir := t.TempDir()
	for _, m := range Catalog() {
		installFake(t, dir, m)
	}
	settings := &fakeSettings{model: Catalog()[0].ID, idle: idle}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	l := NewLocal(dir, settings.get)
	l.Binary = exe
	l.Env = []string{"FISTBUMP_FAKE_LLAMA=1"}
	l.StartTimeout = 10 * time.Second
	t.Cleanup(l.Stop)
	return l, settings
}

func TestLocalStartsWaitsForHealthAndAnswers(t *testing.T) {
	l, _ := newFakeLocal(t, time.Minute)
	if !l.Ready() {
		t.Fatal("binary present and model installed: should be ready")
	}
	if l.Running() {
		t.Fatal("must not start before the first request")
	}
	got, err := l.Chat(context.Background(), []Message{{Role: "user", Content: "ping"}}, 8, false)
	if err != nil {
		t.Fatal(err)
	}
	want := "pong from " + Catalog()[0].Path(l.DataDir)
	if got != want {
		t.Errorf("reply = %q, want %q", got, want)
	}
	if !l.Running() || l.LoadedModel() != Catalog()[0].ID {
		t.Errorf("running %v, loaded %q", l.Running(), l.LoadedModel())
	}
}

func TestLocalStopsWhenIdle(t *testing.T) {
	l, _ := newFakeLocal(t, 200*time.Millisecond)
	if _, err := l.Chat(context.Background(), nil, 8, false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for l.Running() {
		if time.Now().After(deadline) {
			t.Fatal("still running after the idle time")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestLocalRestartsOnModelChange(t *testing.T) {
	l, settings := newFakeLocal(t, time.Minute)
	if _, err := l.Chat(context.Background(), nil, 8, false); err != nil {
		t.Fatal(err)
	}
	second := Catalog()[1]
	settings.set(second.ID)
	got, err := l.Chat(context.Background(), nil, 8, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "pong from "+second.Path(l.DataDir) || l.LoadedModel() != second.ID {
		t.Errorf("reply %q, loaded %q", got, l.LoadedModel())
	}
	l.StopIfLoaded("other")
	if !l.Running() {
		t.Error("StopIfLoaded stopped a different model")
	}
	l.StopIfLoaded(second.ID)
	if l.Running() {
		t.Error("StopIfLoaded should stop the loaded model")
	}
}

func TestLocalNotReady(t *testing.T) {
	l, settings := newFakeLocal(t, time.Minute)
	settings.set("")
	if l.Ready() {
		t.Error("no selected model: not ready")
	}
	settings.set(Catalog()[0].ID)
	l.Binary = ""
	if l.Ready() {
		t.Error("no binary: not ready")
	}
}
