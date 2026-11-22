package ai

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fileServer serves content, honoring Range unless ignoreRange is set. block, when set, holds the
// response open after the first half until it is closed.
type fileServer struct {
	content     []byte
	ignoreRange bool
	block       chan struct{}
	gotRange    []string
}

func (f *fileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rng := r.Header.Get("Range")
	f.gotRange = append(f.gotRange, rng)
	start := 0
	if rng != "" && !f.ignoreRange {
		n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rng, "bytes="), "-"))
		start = n
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", n, len(f.content)-1, len(f.content)))
		w.WriteHeader(http.StatusPartialContent)
	}
	body := f.content[start:]
	if f.block != nil {
		half := len(body) / 2
		w.Write(body[:half])
		w.(http.Flusher).Flush()
		select {
		case <-f.block:
		case <-r.Context().Done():
		}
		return
	}
	w.Write(body)
}

func testModel(content []byte) Model {
	sum := sha256.Sum256(content)
	return Model{ID: "test-model", File: "test.gguf", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(content))}
}

func newTestDownloads(t *testing.T, srv http.Handler) (*Downloads, string) {
	t.Helper()
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	d := NewDownloads(t.TempDir())
	d.URLFor = func(Model) string { return ts.URL + "/model" }
	return d, d.DataDir
}

func content() []byte { return bytes.Repeat([]byte("fistbump model bytes "), 5000) }

func runDownload(t *testing.T, d *Downloads, m Model) DownloadStatus {
	t.Helper()
	id, err := d.Start(m)
	if err != nil {
		t.Fatal(err)
	}
	d.Wait(id)
	st, _ := d.Status(id)
	return st
}

func TestFullDownload(t *testing.T) {
	c := content()
	d, dir := newTestDownloads(t, &fileServer{content: c})
	m := testModel(c)
	st := runDownload(t, d, m)
	if st.State != DownloadDone || st.Bytes != int64(len(c)) || st.Error != nil {
		t.Fatalf("status = %+v", st)
	}
	got, err := os.ReadFile(m.Path(dir))
	if err != nil || !bytes.Equal(got, c) {
		t.Fatalf("file not in place: %v", err)
	}
	if _, err := os.Stat(m.Path(dir) + ".part"); !errors.Is(err, os.ErrNotExist) {
		t.Error("the .part file should be gone")
	}
}

func TestResumeFromPart(t *testing.T) {
	c := content()
	srv := &fileServer{content: c}
	d, dir := newTestDownloads(t, srv)
	m := testModel(c)
	os.MkdirAll(ModelsDir(dir), 0o700)
	os.WriteFile(m.Path(dir)+".part", c[:1000], 0o600)

	st := runDownload(t, d, m)
	if st.State != DownloadDone {
		t.Fatalf("status = %+v", st)
	}
	if srv.gotRange[0] != "bytes=1000-" {
		t.Errorf("Range = %q, want bytes=1000-", srv.gotRange[0])
	}
	got, _ := os.ReadFile(m.Path(dir))
	if !bytes.Equal(got, c) {
		t.Error("resumed file differs from the original")
	}
}

func TestServerIgnoringRangeStartsOver(t *testing.T) {
	c := content()
	d, dir := newTestDownloads(t, &fileServer{content: c, ignoreRange: true})
	m := testModel(c)
	os.MkdirAll(ModelsDir(dir), 0o700)
	os.WriteFile(m.Path(dir)+".part", c[:1000], 0o600)

	if st := runDownload(t, d, m); st.State != DownloadDone {
		t.Fatalf("status = %+v", st)
	}
	got, _ := os.ReadFile(m.Path(dir))
	if !bytes.Equal(got, c) {
		t.Error("a 200 should replace the .part, not append to it")
	}
}

func TestHashMismatchDeletesAndFails(t *testing.T) {
	c := content()
	d, dir := newTestDownloads(t, &fileServer{content: c})
	m := testModel(c)
	m.SHA256 = strings.Repeat("0", 64)
	st := runDownload(t, d, m)
	if st.State != DownloadFailed || st.Error == nil {
		t.Fatalf("status = %+v", st)
	}
	for _, p := range []string{m.Path(dir), m.Path(dir) + ".part"} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s should not exist", p)
		}
	}
}

func TestCancelKeepsPart(t *testing.T) {
	c := content()
	srv := &fileServer{content: c, block: make(chan struct{})}
	defer close(srv.block)
	d, dir := newTestDownloads(t, srv)
	m := testModel(c)
	id, err := d.Start(m)
	if err != nil {
		t.Fatal(err)
	}
	// Wait for the first half to land.
	for deadline := time.Now().Add(5 * time.Second); ; {
		if st, _ := d.Status(id); st.Bytes > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no bytes arrived")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := d.Start(m); !errors.Is(err, ErrBusy) {
		t.Errorf("second Start = %v, want ErrBusy", err)
	}
	if !d.Cancel(id) {
		t.Fatal("Cancel reported an unknown id")
	}
	st, _ := d.Status(id)
	if st.State != DownloadCancelled {
		t.Errorf("state = %q, want cancelled", st.State)
	}
	info, err := os.Stat(m.Path(dir) + ".part")
	if err != nil || info.Size() == 0 {
		t.Errorf("the .part file should be kept: %v", err)
	}
	if d.Downloading(m.ID) {
		t.Error("nothing should be downloading after cancel")
	}
}

func TestStalledDownloadFails(t *testing.T) {
	c := content()
	srv := &fileServer{content: c, block: make(chan struct{})}
	defer close(srv.block)
	d, _ := newTestDownloads(t, srv)
	d.IdleTimeout = 100 * time.Millisecond
	st := runDownload(t, d, testModel(c))
	if st.State != DownloadFailed || st.Error == nil || !strings.Contains(*st.Error, "stalled") {
		t.Fatalf("status = %+v", st)
	}
}

func TestCatalogEntriesAreComplete(t *testing.T) {
	if len(Catalog()) < 2 {
		t.Fatal("want the default model and one alternative")
	}
	for _, m := range Catalog() {
		if m.ID == "" || m.File == "" || len(m.Revision) != 40 || len(m.SHA256) != 64 || m.SizeBytes <= 0 || m.MinRAMGB <= 0 || m.License == "" {
			t.Errorf("incomplete catalog entry: %+v", m)
		}
		if !strings.HasPrefix(m.URL(), "https://huggingface.co/"+m.Repo+"/resolve/"+m.Revision+"/") {
			t.Errorf("URL = %s", m.URL())
		}
	}
	if Catalog()[0].ID != "qwen2.5-1.5b-instruct-q4" {
		t.Errorf("default model = %s", Catalog()[0].ID)
	}
}

func TestInstalledChecksSize(t *testing.T) {
	dir := t.TempDir()
	m := Catalog()[0]
	os.MkdirAll(ModelsDir(dir), 0o700)
	os.WriteFile(m.Path(dir), []byte("short"), 0o600)
	if Installed(dir, m.ID) {
		t.Error("a file of the wrong size is not installed")
	}
	if Installed(dir, "no-such-model") {
		t.Error("an unknown id is not installed")
	}
}

func TestTotalRAM(t *testing.T) {
	if TotalRAM() == 0 {
		t.Skip("total RAM is not readable here")
	}
	if TotalRAM() < 512<<20 {
		t.Errorf("TotalRAM = %d, implausibly small", TotalRAM())
	}
}
