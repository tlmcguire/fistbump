package ai

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Local manages a llama-server child process for an on-device GGUF model and talks to it over the
// OpenAI-compatible API. The process starts on first use and stops after the idle period.
type Local struct {
	dataDir  string
	binPath  string // optional override
	selected func() string
	idle     func() time.Duration

	startMu sync.Mutex // serializes starts; held while llama-server boots
	mu      sync.Mutex // guards the fields below; never held across slow work
	cmd     *exec.Cmd
	model   string
	client  *Remote
	timer   *time.Timer
	started time.Time
}

func NewLocal(dataDir, binPath string, selected func() string, idle func() time.Duration) *Local {
	return &Local{dataDir: dataDir, binPath: binPath, selected: selected, idle: idle, client: newClient("local")}
}

func (l *Local) Name() string { return "local" }

// FindBinary looks for llama-server: explicit path, env, data dir, then PATH.
func (l *Local) FindBinary() (string, bool) {
	exe := "llama-server"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	for _, c := range []string{l.binPath, os.Getenv("FISTBUMP_LLAMA_SERVER"), filepath.Join(l.dataDir, "bin", exe)} {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, true
		}
	}
	if p, err := exec.LookPath(exe); err == nil {
		return p, true
	}
	return "", false
}

// Ready reports whether the selected model is installed and the server binary exists.
func (l *Local) Ready() bool {
	m, ok := FindModel(l.selected())
	if !ok || !IsInstalled(l.dataDir, m) {
		return false
	}
	_, found := l.FindBinary()
	return found
}

func (l *Local) Running() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cmd != nil
}

func (l *Local) ensure(ctx context.Context) error {
	l.startMu.Lock()
	defer l.startMu.Unlock()
	m, ok := FindModel(l.selected())
	if !ok || !IsInstalled(l.dataDir, m) {
		return fmt.Errorf("%w: no local model installed", ErrUnavailable)
	}
	bin, found := l.FindBinary()
	if !found {
		return fmt.Errorf("%w: llama-server not found", ErrUnavailable)
	}
	l.mu.Lock()
	if l.cmd != nil && l.model == m.ID {
		l.resetTimerLocked()
		l.mu.Unlock()
		return nil
	}
	l.stopLocked()
	l.mu.Unlock()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	cmd := exec.Command(bin, "-m", ModelPath(l.dataDir, m), "--host", "127.0.0.1", "--port", fmt.Sprint(port), "-c", "8192")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitHealthy(ctx, url, exited); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	l.mu.Lock()
	l.cmd, l.model, l.started = cmd, m.ID, time.Now()
	l.client.Configure(url+"/v1", m.ID, "")
	l.resetTimerLocked()
	l.mu.Unlock()
	go func() {
		<-exited
		l.mu.Lock()
		if l.cmd == cmd {
			l.cmd = nil
		}
		l.mu.Unlock()
	}()
	return nil
}

func waitHealthy(ctx context.Context, url string, exited <-chan struct{}) error {
	deadline := time.Now().Add(120 * time.Second)
	for {
		select {
		case <-exited:
			return fmt.Errorf("%w: llama-server exited during startup", ErrUnavailable)
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		if resp, err := http.Get(url + "/health"); err == nil {
			ok := resp.StatusCode == http.StatusOK
			resp.Body.Close()
			if ok {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: llama-server did not become healthy", ErrUnavailable)
		}
	}
}

func (l *Local) resetTimerLocked() {
	if l.timer != nil {
		l.timer.Stop()
	}
	d := l.idle()
	if d <= 0 {
		d = 5 * time.Minute
	}
	l.timer = time.AfterFunc(d, l.Stop)
}

func (l *Local) stopLocked() {
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
	if l.cmd != nil {
		_ = l.cmd.Process.Kill()
		l.cmd = nil
	}
}

// Stop kills the child process if one is running.
func (l *Local) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopLocked()
}

// StopIfModel stops the server when it has the given model loaded.
func (l *Local) StopIfModel(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cmd != nil && l.model == id {
		l.stopLocked()
	}
}

func (l *Local) Suggest(ctx context.Context, r Request) ([]Draft, error) {
	if err := l.ensure(ctx); err != nil {
		return nil, err
	}
	reply, err := l.client.ChatJSON(ctx, []chatMessage{{"system", systemPrompt}, {"user", buildPrompt(r)}}, 2048)
	if err != nil {
		return nil, err
	}
	return parseDraftsGrounded(reply, r.Targets, groundingSource(r), requestSkills(r))
}

func (l *Local) Test(ctx context.Context) (string, error) {
	if err := l.ensure(ctx); err != nil {
		return "", err
	}
	return l.client.Test(ctx)
}
