package pdf

import (
	"bytes"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"

	lpdf "github.com/ledongthuc/pdf"
)

const sample = "# Ann Lee\n\nAustin, TX · ann@x.com · github.com/annlee\n\n## Summary\n\nBackend engineer.\n\n" +
	"## Experience\n\n### Dev, Acme\n\n2020 – Present | Austin\n\n- Built “things” in Go and Node.js\n- Docs at https://acme.dev/docs.\n"

func open(t *testing.T, out []byte) *lpdf.Reader {
	t.Helper()
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatalf("not a pdf: %q", out[:min(len(out), 20)])
	}
	r, err := lpdf.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func render(t *testing.T, md, file string) []byte {
	t.Helper()
	out, err := RenderFile(md, file)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRender(t *testing.T) {
	out, err := Render(sample)
	if err != nil {
		t.Fatal(err)
	}
	r := open(t, out)
	if r.NumPage() != 1 {
		t.Fatalf("pages = %d", r.NumPage())
	}
	info := r.Trailer().Key("Info")
	if got := info.Key("Title").Text(); got != "Resume" {
		t.Errorf("title = %q", got)
	}
	if got := info.Key("Creator").Text(); got != "FistBump" {
		t.Errorf("creator = %q", got)
	}
}

func TestTitleIsFilenameWithoutExtension(t *testing.T) {
	for file, want := range map[string]string{
		"ann-lee-resume.pdf": "ann-lee-resume",
		"dir/acme-dev.pdf":   "acme-dev",
		"Łukasiewicz CV.pdf": "Łukasiewicz CV",
		"no-extension":       "no-extension",
		"":                   "Resume",
		"archive.tar.gz":     "archive.tar",
	} {
		info := open(t, render(t, sample, file)).Trailer().Key("Info")
		if got := info.Key("Title").Text(); got != want {
			t.Errorf("%q: title = %q, want %q", file, got, want)
		}
	}
}

// links returns every link URI in the file.
func links(r *lpdf.Reader) []string {
	var uris []string
	for p := 1; p <= r.NumPage(); p++ {
		annots := r.Page(p).V.Key("Annots")
		for i := 0; i < annots.Len(); i++ {
			if u := annots.Index(i).Key("A").Key("URI"); u.Kind() != lpdf.Null {
				uris = append(uris, u.RawString())
			}
		}
	}
	return uris
}

func TestLinks(t *testing.T) {
	got := strings.Join(links(open(t, render(t, sample, "a.pdf"))), "\n")
	for _, want := range []string{"mailto:ann@x.com", "https://github.com/annlee", "https://acme.dev/docs"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing link %q in:\n%s", want, got)
		}
	}
	if strings.Contains(strings.ToLower(got), "node.js") {
		t.Errorf("Node.js should not be a link:\n%s", got)
	}
	if strings.Contains(got, "docs.\n") || strings.HasSuffix(got, "docs.") {
		t.Errorf("trailing period kept in link:\n%s", got)
	}
}

func TestLinkTarget(t *testing.T) {
	for in, want := range map[string]string{
		"ann@x.com":           "mailto:ann@x.com",
		"linkedin.com/in/ann": "https://linkedin.com/in/ann",
		"https://ann.dev":     "https://ann.dev",
		"HTTP://ann.dev":      "HTTP://ann.dev",
		"Austin, TX":          "",
		"(512) 555-0100":      "",
	} {
		if got := linkTarget(in); got != want {
			t.Errorf("linkTarget(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"Node.js":          "",
		"ASP.NET":          "",
		"www.ann.dev":      "https://www.ann.dev",
		"github.com/ann/x": "https://github.com/ann/x",
		"https://x.io":     "https://x.io",
		"ann@x.com":        "mailto:ann@x.com",
	} {
		if got := bodyLinkTarget(in); got != want {
			t.Errorf("bodyLinkTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

// hostile is a resume with the tricks used to hide keywords: zero-width characters, soft hyphens,
// bidi controls, Unicode tag characters, tabs, and text long enough to run off a page.
var hostile = "# José Łukasiewicz\n\n" +
	"jose@example.com | linkedin.com/in/jose | 512-555-0100\n\n" +
	"## Summary\n\n" +
	"Data\u200b engineer\u00ad who ships.\u202e Kubernetes\u2060Terraform \U000E0041\U000E0042 tagged\tend.\n\n" +
	"## Experience\n\n" +
	"### Senior Data Engineer with an extraordinarily long title that cannot fit on one line, Globex Corporation International\n\n" +
	"Jan 2019 – Present | Springfield\n\n" +
	strings.Repeat("- Built pipelines moving billions of rows a day with Python, Spark and Airflow.\n", 40) +
	"- " + strings.Repeat("Supercalifragilistic", 12) + "\n" +
	"- https://example.com/" + strings.Repeat("a", 150) + "\n" +
	"\\# of incidents cut in half\n\n" +
	"## Skills\n\nGo, SQL, Python\n"

// visible returns the characters a reader should see for md: Markdown markers, separators and
// whitespace removed, invisible characters dropped, upper-cased so section titles compare.
func visible(md string) string {
	var b strings.Builder
	for _, l := range strings.Split(md, "\n") {
		l = strings.TrimSpace(l)
		for _, p := range []string{"### ", "## ", "# ", "- ", "\\"} {
			if strings.HasPrefix(l, p) {
				l = l[len(p):]
				break
			}
		}
		for _, r := range l {
			if unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) || unicode.IsControl(r) || strings.ContainsRune("|·•", r) {
				continue
			}
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}

var (
	rgR   = regexp.MustCompile(`([\d.]+) ([\d.]+) ([\d.]+) rg\b`)
	gR    = regexp.MustCompile(`(?:^|\s)([\d.]+) g\b`)
	trR   = regexp.MustCompile(`(?:^|\s)(\d+) Tr\b`)
	tfR   = regexp.MustCompile(`/\S+ ([\d.]+) Tf\b`)
	fillR = regexp.MustCompile(`(?:^|\s)(f|f\*|B|B\*|b|b\*)\s`)
	tzR   = regexp.MustCompile(`(?:^|\s)([\d.]+) Tz\b`)
)

func pageStream(t *testing.T, p lpdf.Page) string {
	t.Helper()
	rc := p.V.Key("Contents").Reader()
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func num(t *testing.T, s string) float64 {
	t.Helper()
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestNoHiddenText checks that every character in the file is drawn visibly: dark text, normal render
// mode, no fills that could cover text or paint a background, at least MinFontSize, inside the margins,
// and that the text drawn is exactly the resume's text.
func TestNoHiddenText(t *testing.T) {
	compress = false
	defer func() { compress = true }()
	for name, md := range map[string]string{"sample": sample, "hostile": hostile} {
		t.Run(name, func(t *testing.T) {
			r := open(t, render(t, md, "x.pdf"))
			const mm = 72 / 25.4
			const tol = 1.0 // pt
			var drawn strings.Builder
			for p := 1; p <= r.NumPage(); p++ {
				page := r.Page(p)
				box := page.V.Key("MediaBox")
				if box.Kind() == lpdf.Null {
					box = r.Trailer().Key("Root").Key("Pages").Key("MediaBox")
				}
				pw, ph := box.Index(2).Float64(), box.Index(3).Float64()
				if pw < 600 || ph < 780 {
					t.Fatalf("page %d size %vx%v", p, pw, ph)
				}
				s := pageStream(t, page)
				for _, m := range rgR.FindAllStringSubmatch(s, -1) {
					for _, c := range m[1:] {
						if num(t, c) > 0.5 {
							t.Errorf("page %d: light fill/text colour %q", p, m[0])
						}
					}
				}
				for _, m := range gR.FindAllStringSubmatch(s, -1) {
					if num(t, m[1]) > 0.5 {
						t.Errorf("page %d: light grey fill/text %q", p, m[0])
					}
				}
				for _, m := range trR.FindAllStringSubmatch(s, -1) {
					if m[1] != "0" {
						t.Errorf("page %d: text render mode %s", p, m[1])
					}
				}
				for _, m := range tfR.FindAllStringSubmatch(s, -1) {
					if num(t, m[1]) < MinFontSize {
						t.Errorf("page %d: font size %s below %v", p, m[1], MinFontSize)
					}
				}
				for _, m := range tzR.FindAllStringSubmatch(s, -1) {
					if num(t, m[1]) != 100 {
						t.Errorf("page %d: horizontal scaling %s", p, m[1])
					}
				}
				if m := fillR.FindString(s); m != "" {
					t.Errorf("page %d: fill operator %q could paint over text", p, m)
				}
				for _, tx := range page.Content().Text {
					if strings.TrimSpace(tx.S) == "" {
						continue
					}
					if tx.FontSize < MinFontSize {
						t.Errorf("page %d: %q at %vpt", p, tx.S, tx.FontSize)
					}
					if tx.X < marginX*mm-tol || tx.X+tx.W > pw-marginX*mm+tol {
						t.Errorf("page %d: %q x=%.1f..%.1f outside side margins", p, tx.S, tx.X, tx.X+tx.W)
					}
					if tx.Y < marginY*mm-tol || tx.Y > ph-marginY*mm+tol {
						t.Errorf("page %d: %q y=%.1f outside top/bottom margins", p, tx.S, tx.Y)
					}
					drawn.WriteString(tx.S)
				}
			}
			got := visible(drawn.String())
			want := visible(md)
			if got != want {
				i := 0
				for i < len(got) && i < len(want) && got[i] == want[i] {
					i++
				}
				t.Errorf("drawn text differs from resume at byte %d:\n got …%q\nwant …%q", i, got[i:min(len(got), i+60)], want[i:min(len(want), i+60)])
			}
		})
	}
}

func TestMultiPage(t *testing.T) {
	if n := open(t, render(t, hostile, "x.pdf")).NumPage(); n < 2 {
		t.Fatalf("pages = %d, want the long resume to break across pages", n)
	}
}

func TestOddInputDoesNotFail(t *testing.T) {
	for _, md := range []string{"", "#", "# ", "- ", "### ", "##", "\n\n\n", "# A\n# B\n- \n* x\n• y", "\x00\x01\u200b"} {
		if _, err := Render(md); err != nil {
			t.Errorf("Render(%q): %v", md, err)
		}
	}
}

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"a\u200bb":     "ab",
		"a\u00adb":     "ab",
		"\ta \t  b ":   "a b",
		"x\U000E0041y": "xy",
		"r\u202etl":    "rtl",
		"Łódź – “ok”":  "Łódź – “ok”",
	} {
		if got := clean(in); got != want {
			t.Errorf("clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEscapedLinesDrawWithoutBackslash(t *testing.T) {
	compress = false
	defer func() { compress = true }()
	md := "# \\-Ann\n\n## Experience\n\n### Dev, Acme\n\n\\# of incidents cut in half\n\\- not a bullet\n\\* not a bullet either\n"
	r := open(t, render(t, md, "x.pdf"))
	var drawn strings.Builder
	for _, tx := range r.Page(1).Content().Text {
		drawn.WriteString(tx.S)
	}
	got := drawn.String()
	for _, want := range []string{"-Ann", "# of incidents", "- not a bullet", "* not a bullet either"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, `\`) {
		t.Errorf("backslash drawn: %q", got)
	}
}
