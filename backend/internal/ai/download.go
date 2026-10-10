package ai

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

type DownloadStatus struct {
	State   string  `json:"state"` // running, verifying, done, failed, cancelled
	Bytes   int64   `json:"bytes"`
	Total   int64   `json:"total"`
	Error   *string `json:"error"`
	ModelID string  `json:"model_id"`
}

type download struct {
	mu     sync.Mutex
	st     DownloadStatus
	cancel context.CancelFunc
}

// Downloader fetches catalog models from Hugging Face with resume, SHA-256 verification and an atomic rename.
type Downloader struct {
	dataDir string
	mu      sync.Mutex
	items   map[string]*download
	token   string
	baseURL string // override in tests
}

func NewDownloader(dataDir string) *Downloader {
	return &Downloader{dataDir: dataDir, items: map[string]*download{}, baseURL: "https://huggingface.co"}
}

var ErrBusy = errors.New("model is already downloading")

func (d *Downloader) SetToken(t string) { d.mu.Lock(); d.token = t; d.mu.Unlock() }

func (d *Downloader) ActiveFor(modelID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, it := range d.items {
		it.mu.Lock()
		active := it.st.ModelID == modelID && (it.st.State == "running" || it.st.State == "verifying")
		it.mu.Unlock()
		if active {
			return true
		}
	}
	return false
}

// Start begins a download and returns its id.
func (d *Downloader) Start(m CatalogModel) (string, error) {
	if d.ActiveFor(m.ID) {
		return "", ErrBusy
	}
	if err := os.MkdirAll(ModelsDir(d.dataDir), 0o755); err != nil {
		return "", err
	}
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)
	ctx, cancel := context.WithCancel(context.Background())
	it := &download{st: DownloadStatus{State: "running", Total: m.SizeBytes, ModelID: m.ID}, cancel: cancel}
	d.mu.Lock()
	d.items[id] = it
	token := d.token
	d.mu.Unlock()
	go d.run(ctx, it, m, token)
	return id, nil
}

func (d *Downloader) Status(id string) (DownloadStatus, bool) {
	d.mu.Lock()
	it, ok := d.items[id]
	d.mu.Unlock()
	if !ok {
		return DownloadStatus{}, false
	}
	it.mu.Lock()
	defer it.mu.Unlock()
	return it.st, true
}

// Cancel stops the transfer and keeps the .part file so a later download resumes.
func (d *Downloader) Cancel(id string) bool {
	d.mu.Lock()
	it, ok := d.items[id]
	d.mu.Unlock()
	if ok {
		it.cancel()
	}
	return ok
}

// CancelAll stops every transfer, used at shutdown.
func (d *Downloader) CancelAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, it := range d.items {
		it.cancel()
	}
}

func (it *download) set(f func(*DownloadStatus)) { it.mu.Lock(); f(&it.st); it.mu.Unlock() }

func (it *download) fail(err error) {
	msg := err.Error()
	it.set(func(s *DownloadStatus) { s.State = "failed"; s.Error = &msg })
}

func (d *Downloader) run(ctx context.Context, it *download, m CatalogModel, token string) {
	final := ModelPath(d.dataDir, m)
	part := final + ".part"
	var have int64
	if st, err := os.Stat(part); err == nil {
		have = st.Size()
	}
	if have > m.SizeBytes {
		_ = os.Remove(part)
		have = 0
	}
	if have < m.SizeBytes {
		url := fmt.Sprintf("%s/%s/resolve/%s/%s", d.baseURL, m.Repo, m.Revision, m.File)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if have > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token) // Go drops this on cross-host redirects
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			d.finishErr(ctx, it, err)
			return
		}
		defer resp.Body.Close()
		flags := os.O_CREATE | os.O_WRONLY
		switch resp.StatusCode {
		case http.StatusPartialContent:
			flags |= os.O_APPEND
		case http.StatusOK:
			flags |= os.O_TRUNC
			have = 0
		default:
			it.fail(fmt.Errorf("download failed: HTTP %d", resp.StatusCode))
			return
		}
		f, err := os.OpenFile(part, flags, 0o644)
		if err != nil {
			it.fail(err)
			return
		}
		it.set(func(s *DownloadStatus) { s.Bytes = have })
		buf := make([]byte, 256<<10)
		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := f.Write(buf[:n]); werr != nil {
					f.Close()
					it.fail(werr)
					return
				}
				it.set(func(s *DownloadStatus) { s.Bytes += int64(n) })
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				f.Close()
				d.finishErr(ctx, it, rerr)
				return
			}
		}
		f.Close()
	}
	it.set(func(s *DownloadStatus) { s.State = "verifying" })
	sum, err := fileSHA256(part)
	if err != nil {
		it.fail(err)
		return
	}
	if sum != m.SHA256 {
		_ = os.Remove(part)
		it.fail(errors.New("checksum mismatch, file deleted"))
		return
	}
	if err := os.Rename(part, final); err != nil {
		it.fail(err)
		return
	}
	it.set(func(s *DownloadStatus) { s.State = "done"; s.Bytes = s.Total })
}

func (d *Downloader) finishErr(ctx context.Context, it *download, err error) {
	if ctx.Err() != nil {
		it.set(func(s *DownloadStatus) { s.State = "cancelled" })
		return
	}
	it.fail(err)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// PartialBytes sums the size of .part files in the models directory.
func PartialBytes(dataDir string) int64 {
	var n int64
	matches, _ := filepath.Glob(filepath.Join(ModelsDir(dataDir), "*.part"))
	for _, p := range matches {
		if st, err := os.Stat(p); err == nil {
			n += st.Size()
		}
	}
	return n
}
