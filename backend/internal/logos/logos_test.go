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
	logo, err := f.Logo(context.Background(), s.srv.URL, "")
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
	if logo, err := f.Logo(context.Background(), s.srv.URL, ""); err != nil || logo.ContentType != "image/png" {
		t.Errorf("favicon.ico fallback: %+v, %v", logo, err)
	}
}

func TestRejectsNonImages(t *testing.T) {
	s, f := newSite(t, map[string]string{"/": "<html></html>", "/favicon.png": ""})
	if _, err := f.Logo(context.Background(), s.srv.URL, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("no icon: err %v", err)
	}

	// An icon link that serves HTML is not a logo, whatever the server says it is.
	s, f = newSite(t, map[string]string{"/": `<link rel="icon" href="/x.ico">`, "/x.ico": "<html>not an image</html>"})
	if _, err := f.Logo(context.Background(), s.srv.URL, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("html as icon: err %v", err)
	}
}

func TestCachesFoundAndMissing(t *testing.T) {
	s, f := newSite(t, map[string]string{"/": `<link rel="icon" href="/logo.png">`, "/logo.png": ""})
	f.Logo(context.Background(), s.srv.URL, "")
	hits := s.hits.Load()
	if _, err := f.Logo(context.Background(), s.srv.URL, ""); err != nil || s.hits.Load() != hits {
		t.Errorf("second call hit the site again: err %v, hits %d -> %d", err, hits, s.hits.Load())
	}

	s2, f2 := newSite(t, map[string]string{"/": "<html></html>"})
	f2.Logo(context.Background(), s2.srv.URL, "")
	hits = s2.hits.Load()
	if _, err := f2.Logo(context.Background(), s2.srv.URL, ""); !errors.Is(err, ErrNotFound) || s2.hits.Load() != hits {
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
	if _, err := f.Logo(context.Background(), s.srv.URL, ""); !errors.Is(err, ErrNotFound) {
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
	if _, err := f.Logo(context.Background(), srv.URL, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("local server: err %v", err)
	}
}

func TestSchemaLogoRanksAboveIcons(t *testing.T) {
	page := []byte(`<link rel="icon" href="/fav.ico"><script type="application/ld+json">{"@type":"Organization","logo":"https:\/\/cdn.acme.com\/logo.png"}</script>`)
	links := iconLinks(page, mustURL("https://acme.com"))
	if len(links) != 2 || links[0] != "https://cdn.acme.com/logo.png" {
		t.Errorf("links = %v", links)
	}
	page = []byte(`<script>{"logo":{"@type":"ImageObject","url":"/img/logo.svg"}}</script>`)
	if links := iconLinks(page, mustURL("https://acme.com")); len(links) != 1 || links[0] != "https://acme.com/img/logo.svg" {
		t.Errorf("object logo = %v", links)
	}
}

func TestHostVariants(t *testing.T) {
	cases := map[string][]string{
		"acme.com":         {"acme.com", "www.acme.com"},
		"www.acme.com":     {"www.acme.com", "acme.com"},
		"careers.acme.com": {"careers.acme.com", "www.careers.acme.com", "acme.com", "www.acme.com"},
		"shop.acme.com":    {"shop.acme.com", "www.shop.acme.com"},
	}
	for host, want := range cases {
		if got := hostVariants(host); strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("hostVariants(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestFollowsHTTPHopsAndWwwRedirects(t *testing.T) {
	// The home page answers with a redirect chain before the page; the icon is found at the end.
	var final *httptest.Server
	hop := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+r.URL.Path, http.StatusMovedPermanently)
	}))
	defer hop.Close()
	final = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Write([]byte(`<link rel="icon" href="/i.png">`))
			return
		}
		w.Write(tinyPNG)
	}))
	defer final.Close()
	f := &Fetcher{HTTP: hop.Client(), sem: make(chan struct{}, 1)}
	// both test servers use the same certificate authority, so one client trusts both
	f.HTTP = &http.Client{Transport: hop.Client().Transport}
	if logo, err := f.Logo(context.Background(), hop.URL, ""); err != nil || logo.ContentType != "image/png" {
		t.Errorf("after redirect: %+v, %v", logo, err)
	}
}

func TestBoardPageRedirectGivesTheSite(t *testing.T) {
	site, f := newSite(t, map[string]string{"/": `<link rel="icon" href="/logo.png">`, "/logo.png": ""})
	boards := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/acme" {
			http.Redirect(w, r, site.srv.URL+"/careers", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer boards.Close()
	f.BoardBase = boards.URL
	logo, err := f.Logo(context.Background(), "", "acme")
	if err != nil || logo.ContentType != "image/png" {
		t.Fatalf("board redirect: %+v, %v", logo, err)
	}
	// A second call is served from the cache under the board.
	hits := site.hits.Load()
	f.Logo(context.Background(), "", "acme")
	if site.hits.Load() != hits {
		t.Error("the board result was not cached")
	}
	if _, err := f.Logo(context.Background(), "", "nosuchboard"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown board: %v", err)
	}
	if _, err := f.Logo(context.Background(), "", "Bad Token"); err == nil {
		t.Error("a bad board token was accepted")
	}
}

func TestBoardPageLogoWhenThereIsNoRedirect(t *testing.T) {
	var cdn *httptest.Server
	cdn = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backblaze" {
			w.Write([]byte(`<html><head><meta property="og:image" content="/ignored.png"></head><body><img src="` + cdn.URL + `/logo.png" alt="Backblaze Logo"></body></html>`))
			return
		}
		w.Write(tinyPNG)
	}))
	defer cdn.Close()
	f := &Fetcher{HTTP: cdn.Client(), BoardBase: cdn.URL, sem: make(chan struct{}, 1)}
	if logo, err := f.Logo(context.Background(), "", "backblaze"); err != nil || logo.ContentType != "image/png" {
		t.Errorf("board page logo: %+v, %v", logo, err)
	}
}

func TestSiteFailureFallsBackToTheBoard(t *testing.T) {
	// The company's own site has no icon, but its board page has a logo.
	site, f := newSite(t, map[string]string{"/": "<html></html>"})
	board := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/acme" {
			w.Write([]byte(`<img src="/l.png" alt="Acme Logo">`))
			return
		}
		w.Write(tinyPNG)
	}))
	defer board.Close()
	f.BoardBase = board.URL
	if logo, err := f.Logo(context.Background(), site.srv.URL, "acme"); err != nil || logo.ContentType != "image/png" {
		t.Errorf("fallback: %+v, %v", logo, err)
	}
}
