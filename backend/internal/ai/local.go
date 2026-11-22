package ai

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// LocalSettings reads what Local needs from the settings table each time it is asked, so a change
// in the app takes effect without a restart.
type LocalSettings func() (modelID string, idle time.Duration)

// Local runs llama-server as a child process while it is needed: it starts on the first request,
// stops after the idle time with no request, when the model changes or is deleted, and at shutdown.
// It talks to the server with the Remote client, which needs no key on loopback.
type Local struct {
	DataDir  string
	Binary   string // path to llama-server; FindLlamaServer by default
	Settings LocalSettings
	// Env is added to the child's environment. Tests use it to run the test binary as a fake server.
	Env          []string
	StartTimeout time.Duration
	StopTimeout  time.Duration

	mu       sync.Mutex
	cmd      *exec.Cmd
	exited   chan struct{}
	modelID  string
	client   *Remote
	inFlight int
	idle     *time.Timer
	// loaded mirrors modelID while a server runs, so status reads never wait on a start.
	loaded atomic.Pointer[string]
}

// NewLocal returns a Local that finds llama-server next to this executable.
func NewLocal(dataDir string, settings LocalSettings) *Local {
	return &Local{
		DataDir:      dataDir,
		Binary:       FindLlamaServer(),
		Settings:     settings,
		StartTimeout: 60 * time.Second,
		StopTimeout:  5 * time.Second,
	}
}

// FindLlamaServer returns the llama-server path beside fistbump-core, or "" if it is not there.
func FindLlamaServer() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	name := "llama-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(filepath.Dir(exe), name)
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return ""
	}
	return path
}

func (l *Local) Name() string { return EngineLocal }

// SelectedModel is the selected model id, or "".
func (l *Local) SelectedModel() string {
	if l.Settings == nil {
		return ""
	}
	id, _ := l.Settings()
	return id
}

// Ready reports that llama-server is present and the selected model is installed.
func (l *Local) Ready() bool {
	if l.Binary == "" {
		return false
	}
	id := l.SelectedModel()
	return id != "" && Installed(l.DataDir, id)
}

// Running reports whether llama-server is up.
func (l *Local) Running() bool { return l.LoadedModel() != "" }

// LoadedModel is the model the running server has loaded, or "".
func (l *Local) LoadedModel() string {
	if p := l.loaded.Load(); p != nil {
		return *p
	}
	return ""
}

// Suggest starts the server if needed and asks it for one rewrite per target.
func (l *Local) Suggest(ctx context.Context, r Request) ([]Draft, error) {
	if !l.Ready() {
		return nil, ErrUnavailable
	}
	return suggestWith(ctx, EngineLocal, l.Chat, r)
}

// Chat sends one chat completion to the local server, starting it first if needed.
func (l *Local) Chat(ctx context.Context, msgs []Message, maxTokens int, jsonMode bool) (string, error) {
	client, err := l.acquire(ctx)
	if err != nil {
		return "", err
	}
	defer l.release()
	return client.Chat(ctx, msgs, maxTokens, jsonMode)
}

// acquire makes sure the server runs the selected model and counts a request in flight, so the
// idle timer cannot stop it mid-request.
func (l *Local) acquire(ctx context.Context) (*Remote, error) {
	modelID := l.SelectedModel()
	m, ok := FindModel(modelID)
	if !ok || !Installed(l.DataDir, modelID) {
		return nil, ErrUnavailable
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cmd != nil && l.modelID != modelID {
		l.stopLocked()
	}
	if l.cmd == nil {
		if err := l.startLocked(ctx, m); err != nil {
			return nil, err
		}
	}
	l.inFlight++
	if l.idle != nil {
		l.idle.Stop()
	}
	return l.client, nil
}

func (l *Local) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.inFlight--
	if l.inFlight > 0 || l.cmd == nil {
		return
	}
	idle := 5 * time.Minute
	if l.Settings != nil {
		if _, d := l.Settings(); d > 0 {
			idle = d
		}
	}
	cmd := l.cmd
	l.idle = time.AfterFunc(idle, func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.cmd == cmd && l.inFlight == 0 {
			slog.Info("stopping llama-server after idle time")
			l.stopLocked()
		}
	})
}

// startLocked runs llama-server on a free loopback port and waits for /health to answer 200.
func (l *Local) startLocked(ctx context.Context, m Model) error {
	port, err := freePort()
	if err != nil {
		return err
	}
	cmd := exec.Command(l.Binary,
		"--model", m.Path(l.DataDir),
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--ctx-size", "4096")
	cmd.Env = append(os.Environ(), l.Env...)
	// exec copies the output into the pipe; Wait returns once that copy is done.
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting llama-server: %w", err)
	}
	go prefixLines(pr)
	exited := make(chan struct{})
	go func() { cmd.Wait(); pw.Close(); close(exited) }()

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	if err := waitHealthy(ctx, base+"/health", exited, l.StartTimeout); err != nil {
		stopProcess(cmd, exited, l.StopTimeout)
		return err
	}
	client := NewRemote()
	client.Label = EngineLocal
	if err := client.Configure(base+"/v1", m.ID, ""); err != nil {
		stopProcess(cmd, exited, l.StopTimeout)
		return err
	}
	l.cmd, l.exited, l.modelID, l.client = cmd, exited, m.ID, client
	id := m.ID
	l.loaded.Store(&id)
	slog.Info("llama-server started", "model", m.ID, "port", port)

	// If it dies on its own, forget it so the next request starts a new one.
	go func() {
		<-exited
		l.mu.Lock()
		if l.cmd == cmd {
			l.cmd, l.client = nil, nil
			l.loaded.Store(nil)
			slog.Warn("llama-server exited")
		}
		l.mu.Unlock()
	}()
	return nil
}

// Stop terminates the server if it runs. Call it at shutdown and when the selected model changes.
func (l *Local) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopLocked()
}

// StopIfLoaded stops the server when it has this model loaded, before the model is deleted.
func (l *Local) StopIfLoaded(modelID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cmd != nil && l.modelID == modelID {
		l.stopLocked()
	}
}

func (l *Local) stopLocked() {
	if l.idle != nil {
		l.idle.Stop()
		l.idle = nil
	}
	if l.cmd == nil {
		return
	}
	cmd, exited := l.cmd, l.exited
	l.cmd, l.client, l.modelID = nil, nil, ""
	l.loaded.Store(nil)
	stopProcess(cmd, exited, l.StopTimeout)
}

// stopProcess asks the process to end, waits, then kills it.
func stopProcess(cmd *exec.Cmd, exited chan struct{}, wait time.Duration) {
	if runtime.GOOS == "windows" {
		cmd.Process.Kill() // Windows has no SIGTERM for a console child
	} else {
		cmd.Process.Signal(os.Interrupt)
	}
	select {
	case <-exited:
	case <-time.After(wait):
		cmd.Process.Kill()
		<-exited
	}
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// waitHealthy polls url until it answers 200. llama-server answers 503 while the model loads.
func waitHealthy(ctx context.Context, url string, exited <-chan struct{}, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Second}
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-exited:
			return errors.New("llama-server exited while starting")
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return errors.New("llama-server did not become ready in time")
			}
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// prefixLines copies a child's output to stderr with a "llama:" prefix.
func prefixLines(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		fmt.Fprintln(os.Stderr, "llama:", sc.Text())
	}
}
