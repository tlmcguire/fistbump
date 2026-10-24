package logos

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/db"
)

// a 1x1 PNG
var tinyPNG, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")

type site struct {
	hits atomic.Int32
	srv  *httptest.Server
}

// newSite serves routes by path. A route is a body; a path ending in .png gets the PNG.
func newSite(t *testing.T, routes map[string]string) (*site, *Fetcher) {
	t.Helper()
	s := &site{}
	s.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".png") {
			w.Write(tinyPNG)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(s.srv.Close)
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return s, &Fetcher{DB: conn, HTTP: s.srv.Client(), sem: make(chan struct{}, 4)}
}

func TestPrefersTouchIconAndReturnsDataURL(t *testing.T) {
	s, f := newSite(t, map[string]string{
		"/":          `<html><head><link rel="icon" href="/small.png" sizes="16x16"><link rel="apple-touch-icon" href="/touch.png"></head></html>`,
		"/small.png": "", "/touch.png": "",
	})
	logo, err := f.Logo(context.Background(), s.srv.URL)
	if err != nil || logo.ContentType != "image/png" {
		t.Fatalf("logo = %+v, err %v", logo, err)
	}
	if !strings.HasPrefix(logo.DataURL(), "data:image/png;base64,") {
		t.Errorf("data url = %.30s", logo.DataURL())
	}
	if links := iconLinks([]byte(`<link rel="icon" href="/a.png" sizes="32x32"><link rel="icon" href="/b.png" sizes="192x192"><link rel="apple-touch-icon" href="/c.png">`), mustURL(s.srv.URL)); len(links) != 3 || !strings.HasSuffix(links[0], "/c.png") || !strings.HasSuffix(links[1], "/b.png") {
		t.Errorf("order = %v", links)
	}
}

func mustURL(s string) *url.URL { u, _ := url.Parse(s); return u }

func TestFallsBackToFaviconIco(t *testing.T) {
	s, f := newSite(t, map[string]string{"/": "<html></html>", "/favicon.ico": string(tinyPNG)})
	if logo, err := f.Logo(context.Background(), s.srv.URL); err != nil || logo.ContentType != "image/png" {
		t.Errorf("favicon.ico fallback: %+v, %v", logo, err)
	}
}

func TestRejectsNonImages(t *testing.T) {
	s, f := newSite(t, map[string]string{"/": "<html></html>", "/favicon.png": ""})
	if _, err := f.Logo(context.Background(), s.srv.URL); !errors.Is(err, ErrNotFound) {
		t.Errorf("no icon: err %v", err)
	}

	// An icon link that serves HTML is not a logo, whatever the server says it is.
	s, f = newSite(t, map[string]string{"/": `<link rel="icon" href="/x.ico">`, "/x.ico": "<html>not an image</html>"})
	if _, err := f.Logo(context.Background(), s.srv.URL); !errors.Is(err, ErrNotFound) {
		t.Errorf("html as icon: err %v", err)
	}
}

func TestCachesFoundAndMissing(t *testing.T) {
	s, f := newSite(t, map[string]string{"/": `<link rel="icon" href="/logo.png">`, "/logo.png": ""})
	f.Logo(context.Background(), s.srv.URL)
	hits := s.hits.Load()
	if _, err := f.Logo(context.Background(), s.srv.URL); err != nil || s.hits.Load() != hits {
		t.Errorf("second call hit the site again: err %v, hits %d -> %d", err, hits, s.hits.Load())
	}

	s2, f2 := newSite(t, map[string]string{"/": "<html></html>"})
	f2.Logo(context.Background(), s2.srv.URL)
	hits = s2.hits.Load()
	if _, err := f2.Logo(context.Background(), s2.srv.URL); !errors.Is(err, ErrNotFound) || s2.hits.Load() != hits {
		t.Errorf("a missing logo was fetched again: err %v", err)
	}

	// Rows older than a day are purged, and an old row is not used.
	clk := time.Now().Add(25 * time.Hour)
	f.Now = func() time.Time { return clk }
	if n, err := f.PurgeOlderThan(24 * time.Hour); err != nil || n != 1 {
		t.Errorf("purged %d, err %v", n, err)
	}
}

func TestSizeLimit(t *testing.T) {
	big := strings.Repeat("a", maxImage+10)
	s, f := newSite(t, map[string]string{"/": `<link rel="icon" href="/big.ico">`, "/big.ico": "\x89PNG\r\n\x1a\n" + big})
	if _, err := f.Logo(context.Background(), s.srv.URL); !errors.Is(err, ErrNotFound) {
		t.Errorf("oversize image: err %v", err)
	}
}

func TestHostValidation(t *testing.T) {
	for _, bad := range []string{"http://acme.com", "https://user:pw@acme.com", "acme.com", "", "ftp://acme.com", "https://"} {
		if _, err := Host(bad); err == nil {
			t.Errorf("Host(%q) accepted", bad)
		}
	}
	if h, err := Host("https://www.Acme.com/careers?x=1"); err != nil || h != "acme.com" {
		t.Errorf("Host = %q, %v", h, err)
	}
}

func TestPrivateAddressesAreRefused(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.0.5", "172.16.0.1", "169.254.169.254", "100.64.0.1", "::1", "fe80::1", "0.0.0.0", "fc00::1"} {
		if !blockedIP(net.ParseIP(ip)) {
			t.Errorf("%s not blocked", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "93.184.216.34", "2606:4700:4700::1111"} {
		if blockedIP(net.ParseIP(ip)) {
			t.Errorf("%s blocked", ip)
		}
	}

	// The real client refuses to connect to a local server, so a company page cannot point it at this machine.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(tinyPNG) }))
	defer srv.Close()
	f := &Fetcher{HTTP: safeClient(), sem: make(chan struct{}, 1)}
	if _, err := f.Logo(context.Background(), srv.URL); !errors.Is(err, ErrNotFound) {
		t.Errorf("local server: err %v", err)
	}
}
