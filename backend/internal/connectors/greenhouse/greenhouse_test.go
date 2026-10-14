package greenhouse

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture is a saved Job Board API response for one job (GET /v1/boards/{board}/jobs/{id}).
func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "api", "greenhouse_job.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fixtureWith returns the fixture with some fields replaced.
func fixtureWith(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(fixture(t), &m); err != nil {
		t.Fatal(err)
	}
	for k, v := range fields {
		m[k] = v
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// escaped encodes HTML the way Greenhouse sends content.
func escaped(s string) string { return html.EscapeString(s) }

// serve starts a fake API that answers every request with status and body, gzipped when the
// client asks for it, and records the last request.
func serve(t *testing.T, status int, body []byte) (*Client, *http.Request) {
	t.Helper()
	last := &http.Request{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*last = *r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.WriteHeader(status)
			gz := gzip.NewWriter(w)
			gz.Write(body)
			gz.Close()
			return
		}
		w.WriteHeader(status)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, HTTP: &http.Client{Timeout: 5 * time.Second}}, last
}

func TestParseLink(t *testing.T) {
	cases := []struct {
		raw          string
		board, jobID string
		err          error
	}{
		{"https://job-boards.greenhouse.io/backblaze/jobs/5277815008", "backblaze", "5277815008", nil},
		{"https://job-boards.greenhouse.io/backblaze/jobs/5277815008?gh_src=abc#app", "backblaze", "5277815008", nil},
		{"  job-boards.greenhouse.io/figma/jobs/123/  ", "figma", "123", nil},
		{"https://boards.greenhouse.io/okta/jobs/42", "okta", "42", nil},
		{"http://BOARDS.Greenhouse.io/okta/jobs/42", "okta", "42", nil},
		{"https://www.backblaze.com/company/careers?gh_jid=5277815008", "", "", ErrCareersPage},
		{"https://boards.greenhouse.io/embed/job_app?gh_jid=42", "", "", ErrCareersPage},
		{"https://www.linkedin.com/jobs/view/123", "", "", ErrNotGreenhouse},
		{"https://job-boards.greenhouse.io.evil.com/backblaze/jobs/1", "", "", ErrNotGreenhouse},
		{"https://jobs.lever.co/acme/1234", "", "", ErrNotGreenhouse},
		{"ftp://boards.greenhouse.io/okta/jobs/42", "", "", ErrNotGreenhouse},
		{"not a link", "", "", ErrNotGreenhouse},
		{"", "", "", ErrNotGreenhouse},
		{"https://job-boards.greenhouse.io/backblaze/jobs/12ab", "", "", ErrBadLink},
		{"https://job-boards.greenhouse.io/backblaze/jobs/-12", "", "", ErrBadLink},
		{"https://job-boards.greenhouse.io/backblaze/jobs/", "", "", ErrBadLink},
		{"https://job-boards.greenhouse.io/backblaze", "", "", ErrBadLink},
		{"https://job-boards.greenhouse.io/backblaze/jobs/1/apply", "", "", ErrBadLink},
		{"https://job-boards.greenhouse.io/back%2Fblaze/jobs/1", "", "", ErrBadLink},
	}
	for _, c := range cases {
		board, id, err := ParseLink(c.raw)
		if board != c.board || id != c.jobID || !errors.Is(err, c.err) {
			t.Errorf("ParseLink(%q) = %q, %q, %v; want %q, %q, %v", c.raw, board, id, err, c.board, c.jobID, c.err)
		}
	}
}

func TestFetchJobMapsFixture(t *testing.T) {
	body := fixture(t)
	var raw rawJob
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	c, req := serve(t, http.StatusOK, body)
	p, err := c.FetchJob(context.Background(), "backblaze", "5277815008")
	if err != nil {
		t.Fatal(err)
	}

	if req.URL.Path != "/v1/boards/backblaze/jobs/5277815008" {
		t.Errorf("path = %q", req.URL.Path)
	}
	if ua := req.Header.Get("User-Agent"); !strings.HasPrefix(ua, "fistbump/") {
		t.Errorf("User-Agent = %q", ua)
	}
	// Left unset by FetchJob, so Go's transport asked for gzip and decompressed the reply.
	if ae := req.Header.Get("Accept-Encoding"); ae != "gzip" {
		t.Errorf("Accept-Encoding = %q, want the transport's gzip", ae)
	}

	if p.ExternalID != "5277815008" || p.Board != "backblaze" {
		t.Errorf("ids = %q, %q", p.ExternalID, p.Board)
	}
	if p.PositionTitle != strings.TrimSpace(raw.Title) || p.Location != strings.TrimSpace(raw.Location.Name) {
		t.Errorf("title, location = %q, %q", p.PositionTitle, p.Location)
	}
	if p.CompanyName != "Backblaze" {
		t.Errorf("company = %q, want the curated name, not %q", p.CompanyName, raw.CompanyName)
	}
	if p.ListingURL != "https://job-boards.greenhouse.io/backblaze/jobs/5277815008" {
		t.Errorf("listing_url = %q", p.ListingURL)
	}
	if p.CompanyURL == nil || *p.CompanyURL != "https://www.backblaze.com/company/careers" {
		t.Errorf("company_url = %v, want the curated website (absolute_url is on Greenhouse)", p.CompanyURL)
	}

	// updated_at carries a local offset; the Posting holds the same instant in UTC.
	want, err := time.Parse(time.RFC3339, raw.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !p.UpdatedAt.Equal(want) || p.UpdatedAt.Location() != time.UTC {
		t.Errorf("updated_at = %v, want %v in UTC", p.UpdatedAt, want)
	}
	if _, off := want.Zone(); off == 0 {
		t.Log("fixture updated_at is already UTC; the offset case is covered in TestUpdatedAtUTC")
	}

	if p.RawText == "" || strings.ContainsAny(p.RawText, "<>") || strings.Contains(p.RawText, "&lt;") || strings.Contains(p.RawText, "&amp;") {
		t.Errorf("raw_text is empty or still has markup:\n%s", p.RawText)
	}
	if !strings.Contains(p.RawText, "\n- ") {
		t.Errorf("raw_text has no list items:\n%s", p.RawText)
	}
}

func TestUpdatedAtUTC(t *testing.T) {
	c, _ := serve(t, http.StatusOK, fixtureWith(t, map[string]any{"updated_at": "2026-08-19T13:57:41-04:00"}))
	p, err := c.FetchJob(context.Background(), "backblaze", "1")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.UpdatedAt.Format(time.RFC3339); got != "2026-08-19T17:57:41Z" {
		t.Errorf("updated_at = %s", got)
	}

	c, _ = serve(t, http.StatusOK, fixtureWith(t, map[string]any{"updated_at": "last Tuesday"}))
	if _, err := c.FetchJob(context.Background(), "backblaze", "1"); !errors.Is(err, ErrUpstream) {
		t.Errorf("bad updated_at: err = %v, want ErrUpstream", err)
	}
}

func TestContentText(t *testing.T) {
	in := `<div class="content-intro"><p><strong>About us</strong></p><p>We   build
tools.&nbsp;Fast.</p></div>
<h2>What you&rsquo;ll do</h2>
<ul><li>Ship <em>features</em></li><li>Review code<ul><li>Go</li><li>SQL</li></ul></li></ul>
<p>Pay: $1 &ndash; $2<br>Remote</p><p></p><p>Tom &amp; Jerry &lt;3</p>`
	want := "About us\n\nWe build tools. Fast.\n\nWhat you’ll do\n\n- Ship features\n- Review code\n  - Go\n  - SQL\n\nPay: $1 – $2\nRemote\n\nTom & Jerry <3"
	got, hidden := contentText(escaped(in))
	if got != want || hidden != 0 {
		t.Errorf("contentText =\n%q, %d\nwant\n%q, 0", got, hidden, want)
	}
}

func TestHiddenContentDropped(t *testing.T) {
	hidden := []string{
		`<script>SECRET()</script>`,
		`<style>.x{} SECRET</style>`,
		`<template><p>SECRET</p></template>`,
		`<noscript>SECRET</noscript>`,
		`<p hidden>SECRET</p>`,
		`<div aria-hidden="true"><p>SECRET</p></div>`,
		`<span aria-hidden=TRUE>SECRET</span>`,
		`<span style="display:none">SECRET</span>`,
		`<span style="color: red; DISPLAY: None !important">SECRET</span>`,
		`<span style="visibility:hidden">SECRET</span>`,
		`<span style="font-size:0">SECRET</span>`,
		`<span style="font-size: 0.5px">SECRET</span>`,
		`<span style="font-size:0em">SECRET</span>`,
		`<span style="font-size:0.5pt">SECRET</span>`,
		`<span style="opacity:0">SECRET</span>`,
		`<span style="opacity: 0.0">SECRET</span>`,
		`<span style="color:#fff">SECRET</span>`,
		`<span style="color:#FFFFFF">SECRET</span>`,
		`<span style="color:white">SECRET</span>`,
		`<span style="color:transparent">SECRET</span>`,
		`<span style="color: rgb(255, 255, 255)">SECRET</span>`,
		`<span style="color: rgba(0,0,0,0)">SECRET</span>`,
		`<span style="color: rgb(0 0 0 / 0%)">SECRET</span>`,
		`<span style="color: #00000000">SECRET</span>`,
		`<span style="color: hsl(0, 0%, 100%)">SECRET</span>`,
		`<div style="display:none"><p>SECRET <b>nested</b> SECRET</p></div>`,
	}
	for _, h := range hidden {
		text, n := contentText(escaped(`<p>Visible before.</p>` + h + `<p>Visible after.</p>`))
		if strings.Contains(text, "SECRET") || n != 1 {
			t.Errorf("%s: text %q, hidden %d; want it skipped and counted once", h, text, n)
		}
		if text != "Visible before.\n\nVisible after." {
			t.Errorf("%s: visible text changed: %q", h, text)
		}
	}

	visible := []string{
		`<span style="color:#333">SHOWN</span>`,
		`<span style="color: rgb(255, 255, 254)">SHOWN</span>`,
		`<span style="font-size:14px">SHOWN</span>`,
		`<span style="font-size:1px">SHOWN</span>`,
		`<span style="font-size:0.8em">SHOWN</span>`,
		`<span style="opacity:0.9">SHOWN</span>`,
		`<span style="display:block">SHOWN</span>`,
		`<span aria-hidden="false">SHOWN</span>`,
		`<span style="background-color:white">SHOWN</span>`,
	}
	for _, v := range visible {
		if text, n := contentText(escaped(v)); text != "SHOWN" || n != 0 {
			t.Errorf("%s: text %q, hidden %d; want it kept", v, text, n)
		}
	}
}

func TestInvisibleCharactersDropped(t *testing.T) {
	tagged := []rune{0xE0001}
	for _, r := range "apply now" {
		tagged = append(tagged, 0xE0000+r)
	}
	content := "\ufeff<p>Go\u200bphers \u202ewelcome\u202c.</p>" +
		`<p style="font-size:0">IGNORE</p>` + "<p>Team" + string(tagged) + "</p>"
	c, _ := serve(t, http.StatusOK, fixtureWith(t, map[string]any{
		"title":   "Back\u200bend Engineer",
		"content": escaped(content),
	}))
	p, err := c.FetchJob(context.Background(), "backblaze", "1")
	if err != nil {
		t.Fatal(err)
	}
	if p.PositionTitle != "Backend Engineer" {
		t.Errorf("title = %q", p.PositionTitle)
	}
	if p.RawText != "Gophers welcome.\n\nTeam" {
		t.Errorf("raw_text = %q", p.RawText)
	}
	// title 1, content 1 + 2 + 10 tag runes; the leading byte order mark is not counted.
	if p.Dropped.InvisibleChars != 14 || p.Dropped.HiddenElements != 1 {
		t.Errorf("dropped = %+v, want 14 invisible chars and 1 hidden element", p.Dropped)
	}
}

func TestCompanyNameFallback(t *testing.T) {
	c, _ := serve(t, http.StatusOK, fixtureWith(t, map[string]any{"company_name": "  Acme External Website "}))
	p, err := c.FetchJob(context.Background(), "acmeuncurated", "1")
	if err != nil {
		t.Fatal(err)
	}
	if p.CompanyName != "Acme External Website" {
		t.Errorf("company = %q, want Greenhouse's company_name for a board not in boards.json", p.CompanyName)
	}

	c, _ = serve(t, http.StatusOK, fixtureWith(t, map[string]any{"company_name": ""}))
	if p, _ = c.FetchJob(context.Background(), "acmeuncurated", "1"); p.CompanyName != "acmeuncurated" {
		t.Errorf("company = %q, want the board token when Greenhouse has no name", p.CompanyName)
	}
}

func TestCompanyAndListingURL(t *testing.T) {
	ownSite := "https://www.backblaze.com/company/careers/?gh_jid=5277815008"
	cases := []struct {
		name, board, absolute string
		want                  any // string or nil
	}{
		{"own-site absolute_url", "backblaze", ownSite, ownSite},
		{"own-site beats curated", "backblaze", "https://jobs.example.com/x?gh_jid=1", "https://jobs.example.com/x?gh_jid=1"},
		{"greenhouse absolute_url uses curated website", "backblaze", "https://job-boards.greenhouse.io/backblaze/jobs/1", "https://www.backblaze.com/company/careers"},
		{"greenhouse subdomain counts as greenhouse", "backblaze", "https://boards.eu.greenhouse.io/backblaze/jobs/1", "https://www.backblaze.com/company/careers"},
		{"look-alike host is not greenhouse", "acmeuncurated", "https://greenhouse.io.acme.com/jobs/1", "https://greenhouse.io.acme.com/jobs/1"},
		{"greenhouse and nothing curated", "acmeuncurated", "https://job-boards.greenhouse.io/acmeuncurated/jobs/1", nil},
		{"http own site is not used", "acmeuncurated", "http://acme.com/careers?gh_jid=1", nil},
		{"empty absolute_url", "acmeuncurated", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cl, _ := serve(t, http.StatusOK, fixtureWith(t, map[string]any{"absolute_url": c.absolute}))
			p, err := cl.FetchJob(context.Background(), c.board, "77")
			if err != nil {
				t.Fatal(err)
			}
			if want := "https://job-boards.greenhouse.io/" + c.board + "/jobs/77"; p.ListingURL != want {
				t.Errorf("listing_url = %q, want %q", p.ListingURL, want)
			}
			out, _ := json.Marshal(p)
			var got map[string]any
			json.Unmarshal(out, &got)
			if got["company_url"] != c.want {
				t.Errorf("company_url = %v, want %v", got["company_url"], c.want)
			}
		})
	}
}

func TestFetchJobErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   []byte
		want   error
	}{
		{"404", http.StatusNotFound, []byte(`{"status":404,"error":"Job not found"}`), ErrNotFound},
		{"500", http.StatusInternalServerError, []byte(`oops`), ErrUpstream},
		{"429", http.StatusTooManyRequests, nil, ErrUpstream},
		{"not JSON", http.StatusOK, []byte(`<html>maintenance</html>`), ErrUpstream},
		{"oversized", http.StatusOK, append([]byte(`{"title":"`), bytes.Repeat([]byte("a"), maxBody+10)...), ErrUpstream},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cl, _ := serve(t, c.status, c.body)
			_, err := cl.FetchJob(context.Background(), "backblaze", "1")
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
			if c.want == ErrUpstream && errors.Is(err, ErrNotFound) {
				t.Errorf("err = %v is also ErrNotFound", err)
			}
		})
	}

	t.Run("unreachable", func(t *testing.T) {
		cl := &Client{BaseURL: "http://127.0.0.1:1", HTTP: &http.Client{Timeout: time.Second}}
		if _, err := cl.FetchJob(context.Background(), "backblaze", "1"); !errors.Is(err, ErrUpstream) {
			t.Errorf("err = %v, want ErrUpstream", err)
		}
	})

	t.Run("bad arguments are not sent", func(t *testing.T) {
		cl, req := serve(t, http.StatusOK, fixture(t))
		for _, args := range [][2]string{{"../x", "1"}, {"backblaze", "1/../2"}, {"", "1"}, {"backblaze", ""}} {
			if _, err := cl.FetchJob(context.Background(), args[0], args[1]); !errors.Is(err, ErrBadLink) {
				t.Errorf("FetchJob(%q, %q) err = %v, want ErrBadLink", args[0], args[1], err)
			}
		}
		if req.URL != nil {
			t.Errorf("a request was sent: %v", req.URL)
		}
	})
}

func TestNewClient(t *testing.T) {
	c := NewClient()
	if c.BaseURL != DefaultBaseURL || c.HTTP == nil || c.HTTP.Timeout != 10*time.Second {
		t.Errorf("NewClient() = %+v", c)
	}
}
