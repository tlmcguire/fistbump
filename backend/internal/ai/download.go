package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// Download states, as GET /v1/models/downloads/{id} reports them.
const (
	DownloadRunning   = "running"
	DownloadVerifying = "verifying"
	DownloadDone      = "done"
	DownloadFailed    = "failed"
	DownloadCancelled = "cancelled"
)

var (
	// ErrBusy means another download is running; only one runs at a time.
	ErrBusy = errors.New("a download is already running")
	// ErrInstalled means the model is already downloaded.
	ErrInstalled = errors.New("the model is already installed")
)

// DownloadStatus is one download's progress.
type DownloadStatus struct {
	State   string  `json:"state"`
	ModelID string  `json:"-"`
	Bytes   int64   `json:"bytes"`
	Total   int64   `json:"total"`
	Error   *string `json:"error"`
}

type download struct {
	status DownloadStatus
	cancel context.CancelFunc
	done   chan struct{}
}

// Downloads fetches catalog models into the data dir. A transfer writes <file>.part, resumes it
// with a Range request, hashes the whole file and renames it into place only when the SHA-256
// matches the catalog. State lives in memory; a .part file is what survives a restart.
type Downloads struct {
	DataDir string
	Client  *http.Client
	// URLFor is where a model is fetched from. Tests point it at a local server.
	URLFor func(Model) string
	// IdleTimeout ends a transfer that receives no data for this long. There is no timeout on
	// the whole transfer, since a model is a gigabyte or more.
	IdleTimeout time.Duration

	mu     sync.Mutex
	next   int
	byID   map[string]*download
	active *download
}

// NewDownloads returns a manager that writes under dataDir/models.
func NewDownloads(dataDir string) *Downloads {
	return &Downloads{
		DataDir:     dataDir,
		Client:      &http.Client{},
		URLFor:      Model.URL,
		IdleTimeout: 30 * time.Second,
		byID:        map[string]*download{},
	}
}

// Start begins downloading m and returns the download id.
func (d *Downloads) Start(m Model) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.active != nil {
		return "", ErrBusy
	}
	if Installed(d.DataDir, m.ID) {
		return "", ErrInstalled
	}
	d.next++
	id := "d" + strconv.Itoa(d.next)
	ctx, cancel := context.WithCancel(context.Background())
	dl := &download{
		status: DownloadStatus{State: DownloadRunning, ModelID: m.ID, Total: m.SizeBytes},
		cancel: cancel,
		done:   make(chan struct{}),
	}
	d.byID[id] = dl
	d.active = dl
	go d.run(ctx, dl, m)
	return id, nil
}

// Status returns a download's progress.
func (d *Downloads) Status(id string) (DownloadStatus, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	dl, ok := d.byID[id]
	if !ok {
		return DownloadStatus{}, false
	}
	return dl.status, true
}

// Cancel stops a download and keeps its .part file, so a later download resumes it. It reports
// false for an unknown id.
func (d *Downloads) Cancel(id string) bool {
	d.mu.Lock()
	dl, ok := d.byID[id]
	d.mu.Unlock()
	if !ok {
		return false
	}
	dl.cancel()
	<-dl.done
	return true
}

// Downloading reports whether a download of this model is running.
func (d *Downloads) Downloading(modelID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.active != nil && d.active.status.ModelID == modelID
}

// Wait blocks until the download ends. Tests use it.
func (d *Downloads) Wait(id string) {
	d.mu.Lock()
	dl, ok := d.byID[id]
	d.mu.Unlock()
	if ok {
		<-dl.done
	}
}

func (d *Downloads) update(dl *download, f func(*DownloadStatus)) {
	d.mu.Lock()
	f(&dl.status)
	d.mu.Unlock()
}

func (d *Downloads) run(ctx context.Context, dl *download, m Model) {
	err := d.transfer(ctx, dl, m)
	d.mu.Lock()
	switch {
	case err == nil:
		dl.status.State = DownloadDone
	case ctx.Err() != nil: // Cancel was called
		dl.status.State = DownloadCancelled
	default:
		msg := err.Error()
		dl.status.State = DownloadFailed
		dl.status.Error = &msg
		slog.Warn("model download failed", "model", m.ID, "error", msg)
	}
	d.active = nil
	d.mu.Unlock()
	dl.cancel()
	close(dl.done)
}

func (d *Downloads) transfer(ctx context.Context, dl *download, m Model) error {
	if err := os.MkdirAll(ModelsDir(d.DataDir), 0o700); err != nil {
		return err
	}
	final := m.Path(d.DataDir)
	partPath := final + ".part"
	part, err := os.OpenFile(partPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer part.Close()

	// Hash what is already there, so the sum covers the whole file once the rest arrives.
	sum := sha256.New()
	have, err := io.Copy(sum, part)
	if err != nil {
		return err
	}
	if have > m.SizeBytes {
		have, sum = 0, sha256.New()
		if err := part.Truncate(0); err != nil {
			return err
		}
	}
	d.update(dl, func(s *DownloadStatus) { s.Bytes = have })

	if have < m.SizeBytes {
		if have, err = d.fetch(ctx, dl, m, part, sum, have); err != nil {
			return err
		}
	}
	if err := part.Close(); err != nil {
		return err
	}

	d.update(dl, func(s *DownloadStatus) { s.State = DownloadVerifying })
	got := hex.EncodeToString(sum.Sum(nil))
	if have != m.SizeBytes || got != m.SHA256 {
		os.Remove(partPath)
		return fmt.Errorf("the downloaded file did not match its checksum")
	}
	// Rename is atomic on one volume, so the model is never seen half written.
	return os.Rename(partPath, final)
}

// fetch requests the rest of the file and appends it to part. It returns the new size.
func (d *Downloads) fetch(ctx context.Context, dl *download, m Model, part *os.File, sum hash.Hash, have int64) (int64, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.URLFor(m), nil)
	if err != nil {
		return have, err
	}
	if have > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(have, 10)+"-")
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return have, unwrapCause(ctx, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusPartialContent && have > 0:
		// Appending to what we have.
	case resp.StatusCode == http.StatusOK:
		// The server ignored Range: start over.
		if _, err := part.Seek(0, io.SeekStart); err != nil {
			return have, err
		}
		if err := part.Truncate(0); err != nil {
			return have, err
		}
		have = 0
		sum.Reset()
	default:
		return have, fmt.Errorf("download server answered %s", resp.Status)
	}
	if _, err := part.Seek(have, io.SeekStart); err != nil {
		return have, err
	}

	// A timer that is pushed back on every read ends a stalled transfer.
	idle := time.AfterFunc(d.IdleTimeout, func() { cancel(errStalled) })
	defer idle.Stop()
	buf := make([]byte, 256<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			idle.Reset(d.IdleTimeout)
			if have+int64(n) > m.SizeBytes {
				return have, fmt.Errorf("the download is larger than expected")
			}
			if _, err := part.Write(buf[:n]); err != nil {
				return have, err
			}
			sum.Write(buf[:n])
			have += int64(n)
			d.update(dl, func(s *DownloadStatus) { s.Bytes = have })
		}
		if rerr == io.EOF {
			return have, nil
		}
		if rerr != nil {
			return have, unwrapCause(ctx, rerr)
		}
	}
}

var errStalled = errors.New("the download stalled: no data was received for a while")

// unwrapCause reports a stall as itself rather than as a cancel.
func unwrapCause(ctx context.Context, err error) error {
	if cause := context.Cause(ctx); errors.Is(cause, errStalled) {
		return errStalled
	}
	return err
}
