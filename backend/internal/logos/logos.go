// Package logos finds a company's logo on the company's own website, so no third party learns which
// companies the user looks at. The page is read for an icon link, the image is checked and kept in
// the database, and a company with no usable icon is remembered as having none.
package logos

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	xhtml "golang.org/x/net/html"
)

const (
	timeout    = 6 * time.Second
	maxPage    = 512 << 10 // enough of a home page to reach its <head>
	maxImage   = 256 << 10
	maxTries   = 3
	userAgent  = "fistbump/0.1 (local job application assistant; +https://github.com/tlmcguire/fistbump)"
	timeFormat = time.RFC3339
)

// ErrNotFound means the company's site has no usable logo.
var ErrNotFound = errors.New("no logo found")

// Logo is an image ready to show.
type Logo struct {
	ContentType string
	Data        []byte
}

// DataURL is the logo as a data: URL, which the app can show without loading anything from the network.
func (l Logo) DataURL() string {
	return "data:" + l.ContentType + ";base64," + base64.StdEncoding.EncodeToString(l.Data)
}

// Fetcher finds and caches logos. HTTP defaults to a client that refuses private addresses.
type Fetcher struct {
	DB        *sql.DB
	HTTP      *http.Client
	Now       func() time.Time
	BoardBase string // Greenhouse's job board site; replaceable for tests
	sem       chan struct{}
}

const defaultBoardBase = "https://job-boards.greenhouse.io"

var boardToken = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// New returns a Fetcher stored in db. At most four logos are fetched at once.
func New(db *sql.DB) *Fetcher {
	return &Fetcher{DB: db, HTTP: safeClient(), Now: time.Now, sem: make(chan struct{}, 4)}
}

func (f *Fetcher) now() time.Time {
	if f.Now == nil {
		return time.Now().UTC()
	}
	return f.Now().UTC()
}

// blockedIP reports addresses a logo fetch must never reach: this machine, the local network and
// anything not routable. A company page that redirects there, or a name that resolves there, is refused.
func blockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64 { // 100.64.0.0/10, carrier-grade NAT
		return true
	}
	return false
}

// safeClient checks the address at connect time, after DNS and for every redirect, so a name that
// resolves to a private address cannot get through.
func safeClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 4 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(host); ip == nil || blockedIP(ip) {
				return fmt.Errorf("refusing to connect to %s", host)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:               nil, // a proxy would connect on our behalf and skip the address check
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 4 * time.Second,
			MaxIdleConnsPerHost: 2,
		},
		// Sites often hop http to https and apex to www. The address check covers every hop.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || (req.URL.Scheme != "https" && req.URL.Scheme != "http") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

// Host reads the site's host from a company link: https only, no user info, without a leading "www.".
func Host(site string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(site))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return "", errors.New("not an https link")
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www."), nil
}

// Logo returns a company's logo, from the cache when it is under a day old. site is the company's
// website link, if known; board is its Greenhouse board token, which finds the company's site and
// logo when site is empty or its own site has no usable icon.
func (f *Fetcher) Logo(ctx context.Context, site, board string) (Logo, error) {
	var hosts []string
	key := ""
	if site != "" {
		host, err := Host(site)
		if err != nil {
			return Logo{}, err
		}
		key, hosts = host, hostVariants(host)
	} else if boardToken.MatchString(board) {
		key = "board:" + board
	} else {
		return Logo{}, errors.New("a site or a board is needed")
	}
	if logo, known, err := f.cached(key); known {
		return logo, err
	}

	select {
	case f.sem <- struct{}{}:
		defer func() { <-f.sem }()
	case <-ctx.Done():
		return Logo{}, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, 14*time.Second)
	defer cancel()

	logo, err := f.search(ctx, hosts, board)
	if errors.Is(err, ErrNotFound) {
		f.store(key, Logo{}, false)
	} else if err == nil {
		f.store(key, logo, true)
	}
	return logo, err
}

// hostVariants lists the hosts worth trying for a site: itself, with and without "www.", and the
// parent of a careers or jobs subdomain, since the careers site often has no icon of its own.
func hostVariants(host string) []string {
	out := []string{host}
	add := func(h string) {
		for _, o := range out {
			if o == h {
				return
			}
		}
		out = append(out, h)
	}
	bare := strings.TrimPrefix(host, "www.")
	if bare != host {
		add(bare)
	} else {
		add("www." + host)
	}
	if first, rest, ok := strings.Cut(bare, "."); ok && strings.Contains(rest, ".") {
		switch first {
		case "careers", "jobs", "job", "apply", "work", "join", "boards":
			add(rest)
			add("www." + rest)
		}
	}
	return out
}

// search tries each host's own site, then the company's Greenhouse board page, which names the
// company's site (when it redirects there) or carries a logo of its own.
func (f *Fetcher) search(ctx context.Context, hosts []string, board string) (Logo, error) {
	tried := map[string]bool{}
	try := func(host string) (Logo, bool) {
		if tried[host] || ctx.Err() != nil {
			return Logo{}, false
		}
		tried[host] = true
		logo, err := f.find(ctx, host)
		return logo, err == nil
	}
	for _, h := range hosts {
		if logo, ok := try(h); ok {
			return logo, nil
		}
	}
	if board == "" || !boardToken.MatchString(board) || ctx.Err() != nil {
		return Logo{}, ErrNotFound
	}
	site, logoURL := f.boardPage(ctx, board)
	if site != "" {
		for _, h := range hostVariants(site) {
			if logo, ok := try(h); ok {
				return logo, nil
			}
		}
	}
	if logoURL != "" {
		if body, _, err := f.get(ctx, logoURL, maxImage); err == nil {
			if ct := imageType(body); ct != "" {
				return Logo{ContentType: ct, Data: body}, nil
			}
		}
	}
	return Logo{}, ErrNotFound
}

var greenhouseHost = regexp.MustCompile(`(^|\.)greenhouse\.io$`)

// boardPage reads a company's Greenhouse board page. Most boards redirect to the company's own
// careers site, which gives the site's host. The rest serve a page with the company's logo.
func (f *Fetcher) boardPage(ctx context.Context, board string) (siteHost, logoURL string) {
	base := f.BoardBase
	if base == "" {
		base = defaultBoardBase
	}
	link := strings.TrimRight(base, "/") + "/" + board
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("User-Agent", userAgent)
	hc := f.HTTP
	if hc == nil {
		hc = safeClient()
	}
	stay := *hc // the redirect itself is the answer, so it is not followed
	stay.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := stay.Do(req)
	if err != nil {
		return "", ""
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode >= 300 && res.StatusCode < 400:
		to, err := res.Request.URL.Parse(res.Header.Get("Location"))
		if err != nil || to.Scheme != "https" || to.Hostname() == "" || greenhouseHost.MatchString(to.Hostname()) {
			return "", ""
		}
		return strings.ToLower(to.Host), ""
	case res.StatusCode == http.StatusOK:
		page, err := io.ReadAll(io.LimitReader(res.Body, maxPage))
		if err != nil {
			return "", ""
		}
		return "", boardLogo(page, res.Request.URL)
	}
	return "", ""
}

// boardLogo finds the logo image on a Greenhouse board page: an image whose alt text ends in
// "Logo", else the page's og:image.
func boardLogo(page []byte, base *url.URL) string {
	doc, err := xhtml.Parse(strings.NewReader(string(page)))
	if err != nil {
		return ""
	}
	var alt, og string
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			attr := func(k string) string {
				for _, a := range n.Attr {
					if strings.EqualFold(a.Key, k) {
						return a.Val
					}
				}
				return ""
			}
			switch n.Data {
			case "img":
				if alt == "" && strings.HasSuffix(strings.ToLower(strings.TrimSpace(attr("alt"))), "logo") {
					alt = attr("src")
				}
			case "meta":
				if og == "" && attr("property") == "og:image" {
					og = attr("content")
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	for _, raw := range []string{alt, og} {
		if ref, err := url.Parse(raw); err == nil && raw != "" {
			if abs := base.ResolveReference(ref); abs.Scheme == "https" {
				return abs.String()
			}
		}
	}
	return ""
}

func (f *Fetcher) cached(host string) (Logo, bool, error) {
	if f.DB == nil {
		return Logo{}, false, nil
	}
	var (
		found   int
		logo    Logo
		fetched string
	)
	err := f.DB.QueryRow(`SELECT found, content_type, data, fetched_at FROM logo_cache WHERE host = ?`, host).
		Scan(&found, &logo.ContentType, &logo.Data, &fetched)
	if err != nil {
		return Logo{}, false, nil
	}
	if at, err := time.Parse(timeFormat, fetched); err != nil || f.now().Sub(at) >= 24*time.Hour {
		return Logo{}, false, nil
	}
	if found == 0 {
		return Logo{}, true, ErrNotFound
	}
	return logo, true, nil
}

func (f *Fetcher) store(host string, logo Logo, found bool) {
	if f.DB == nil {
		return
	}
	n := 0
	if found {
		n = 1
	}
	f.DB.Exec(`INSERT INTO logo_cache (host, found, content_type, data, fetched_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(host) DO UPDATE SET found = excluded.found, content_type = excluded.content_type, data = excluded.data, fetched_at = excluded.fetched_at`,
		host, n, logo.ContentType, logo.Data, f.now().Format(timeFormat))
}

// PurgeOlderThan deletes cached logos fetched more than age ago and returns how many it removed.
func (f *Fetcher) PurgeOlderThan(age time.Duration) (int64, error) {
	if f.DB == nil {
		return 0, nil
	}
	res, err := f.DB.Exec(`DELETE FROM logo_cache WHERE fetched_at < ?`, f.now().Add(-age).Format(timeFormat))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Clear removes every cached logo.
func (f *Fetcher) Clear() error {
	if f.DB == nil {
		return nil
	}
	_, err := f.DB.Exec(`DELETE FROM logo_cache`)
	return err
}

// find reads the home page for icon links and tries the best few, then /favicon.ico.
func (f *Fetcher) find(ctx context.Context, host string) (Logo, error) {
	origin := &url.URL{Scheme: "https", Host: host, Path: "/"}
	var candidates []string
	if page, final, err := f.get(ctx, origin.String(), maxPage); err == nil {
		candidates = iconLinks(page, final)
	}
	candidates = append(candidates, origin.ResolveReference(&url.URL{Path: "/favicon.ico"}).String())

	tried := 0
	for _, c := range candidates {
		if tried == maxTries {
			break
		}
		tried++
		body, _, err := f.get(ctx, c, maxImage)
		if err != nil {
			continue
		}
		if ct := imageType(body); ct != "" {
			return Logo{ContentType: ct, Data: body}, nil
		}
	}
	return Logo{}, ErrNotFound
}

// get reads at most limit bytes of a response, failing on any status but 200 and on bodies over the limit.
// It returns the final URL after redirects, which relative links resolve against.
func (f *Fetcher) get(ctx context.Context, link string, limit int64) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	hc := f.HTTP
	if hc == nil {
		hc = safeClient()
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(body)) > limit && limit == maxImage {
		return nil, nil, errors.New("image too large")
	}
	if int64(len(body)) > limit {
		body = body[:limit] // a long home page is only read as far as its head
	}
	return body, res.Request.URL, nil
}

// schemaLogo matches "logo": "https://..." and "logo": {"url": "https://..."} in a page's JSON.
var schemaLogo = regexp.MustCompile(`"logo"\s*:\s*(?:\{[^}]*?"url"\s*:\s*)?"((?:https?:)?\\?/[^"]+)"`)

// iconLinks lists icon URLs in a page, best first: apple touch icons, then icons by declared size.
func iconLinks(page []byte, base *url.URL) []string {
	doc, err := xhtml.Parse(strings.NewReader(string(page)))
	if err != nil {
		return nil
	}
	type icon struct {
		href  string
		score int
	}
	var found []icon
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "link" {
			var rel, href, sizes string
			for _, a := range n.Attr {
				switch strings.ToLower(a.Key) {
				case "rel":
					rel = strings.ToLower(a.Val)
				case "href":
					href = a.Val
				case "sizes":
					sizes = strings.ToLower(a.Val)
				}
			}
			score := 0
			switch {
			case strings.Contains(rel, "apple-touch-icon"):
				score = 1000
			case strings.Contains(rel, "icon"):
				score = 10
			}
			if score > 0 && href != "" {
				if w, _, ok := strings.Cut(sizes, "x"); ok {
					if px, err := strconv.Atoi(w); err == nil {
						score += px
					}
				}
				if ref, err := url.Parse(href); err == nil {
					if abs := base.ResolveReference(ref); abs.Scheme == "https" {
						found = append(found, icon{abs.String(), score})
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	// A logo named in the page's structured data is usually the real logo, so it ranks above plain icons.
	for _, m := range schemaLogo.FindAllStringSubmatch(string(page), -1) {
		raw := strings.ReplaceAll(m[1], `\/`, "/")
		if ref, err := url.Parse(raw); err == nil {
			if abs := base.ResolveReference(ref); abs.Scheme == "https" {
				found = append(found, icon{abs.String(), 500})
			}
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].score > found[j].score })
	out := make([]string, 0, len(found))
	for _, i := range found {
		out = append(out, i.href)
	}
	return out
}

// imageType names the format of an icon from its first bytes, or "" if it is not a usable image.
// It does not trust the server's Content-Type. SVG is allowed because an <img> never runs its scripts.
func imageType(body []byte) string {
	if len(body) < 8 {
		return ""
	}
	switch ct := http.DetectContentType(body); {
	case ct == "image/png", ct == "image/jpeg", ct == "image/gif", ct == "image/webp":
		return ct
	case ct == "image/x-icon", ct == "image/vnd.microsoft.icon":
		return "image/x-icon"
	}
	head := strings.ToLower(string(body[:min(len(body), 512)]))
	if strings.Contains(head, "<svg") {
		return "image/svg+xml"
	}
	return ""
}
