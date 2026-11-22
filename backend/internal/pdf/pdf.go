// Package pdf renders resume Markdown to a one-column PDF with an embedded Unicode font.
//
// # Layout
//
// US Letter, portrait. Side margins 18 mm, top and bottom margins 16 mm. The Markdown is read line
// by line; no Markdown library is used because the render package only produces this subset:
//
//	Markdown                    Drawn as
//	# Name                      Inter Bold 21 pt, ink
//	first line after the name   contact line: Inter 9 pt, muted, parts joined by " · ", links clickable
//	## Section                  Inter Bold 9.2 pt, ink, upper case, thin rule underneath
//	### Entry                   Inter Bold 10.3 pt, ink; a following date line ("Jan 2020 to Present |
//	                            Austin") goes right-aligned on the same row in Inter 9 pt muted, or below
//	- item, * item, • item      bullet with hanging indent, Inter 9.8 pt, ink
//	anything else               paragraph, Inter 9.8 pt, ink
//
// A leading "\#", "\-" or "\*" (render.Escape's escape for user text) is drawn without the backslash.
//
// # No hidden text
//
// Every character in the file is drawn visibly: dark text (ink or muted) on a white page, at 9 pt or
// larger, inside the margins, with the normal fill render mode. Invisible Unicode (zero-width and
// other format characters, control characters) is removed from the input so it cannot carry hidden
// words. FistBump never adds text the user cannot see; recruiters' systems flag resumes that do.
package pdf

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/go-pdf/fpdf"

	"github.com/tlmcguire/fistbump/backend/internal/pdf/fonts"
)

const (
	family = "Inter"

	marginX = 18.0 // mm, left and right
	marginY = 16.0 // mm, top and bottom

	nameSize    = 21.0 // pt
	sectionSize = 9.2
	entrySize   = 10.3
	bodySize    = 9.8
	smallSize   = 9.0 // contact line and entry dates; the smallest text in the file

	nameH    = 9.0 // mm line heights
	sectionH = 5.0
	entryH   = 5.2
	lineH    = 4.7
	smallH   = 4.6

	bulletIndent = 4.8 // mm from the left margin to bullet text

	// Creator is written to the PDF's metadata.
	Creator = "FistBump"
)

// MinFontSize is the smallest text size Render uses, in points.
const MinFontSize = smallSize

var (
	ink   = [3]int{17, 24, 39}
	muted = [3]int{88, 96, 120}
	rule  = [3]int{200, 205, 218}

	emailR = regexp.MustCompile(`^[\w.+-]+@[\w-]+(\.[\w-]+)+$`)
	// Contact-line tokens may be bare domains ("joe.dev", "linkedin.com/in/joe").
	urlR = regexp.MustCompile(`(?i)^(https?://)?([a-z0-9-]+\.)+[a-z]{2,}(/\S*)?$`)
	// In body text a bare "Node.js" must not become a link, so require a scheme, "www." or a path.
	bodyURLR = regexp.MustCompile(`(?i)^(https?://\S+|www\.([a-z0-9-]+\.)+[a-z]{2,}(/\S*)?|([a-z0-9-]+\.)+[a-z]{2,}/\S+)$`)
	dateR    = regexp.MustCompile(`(?i)\b(19|20)\d{2}\b|present|current`)
	sepR     = regexp.MustCompile(`\s+[|·•]\s+`)
	spaceR   = regexp.MustCompile(`[ \t]+`)
)

// compress is turned off by tests so they can read the content streams.
var compress = true

type doc struct {
	*fpdf.Fpdf
	usable float64
	pageH  float64
}

func newDoc(title string) (*doc, error) {
	p := fpdf.New("P", "mm", "Letter", "")
	p.SetCompression(compress)
	p.AddUTF8FontFromBytes(family, "", fonts.Regular)
	p.AddUTF8FontFromBytes(family, "B", fonts.Bold)
	p.SetMargins(marginX, marginY, marginX)
	p.SetAutoPageBreak(true, marginY)
	p.SetTitle(title, true)
	p.SetCreator(Creator, true)
	p.AddPage()
	w, h := p.GetPageSize()
	return &doc{Fpdf: p, usable: w - 2*marginX, pageH: h}, p.Error()
}

func (d *doc) color(c [3]int) { d.SetTextColor(c[0], c[1], c[2]) }

// room starts a new page when less than mm of space is left, so a heading is never stranded at the
// bottom of a page without the text it introduces.
func (d *doc) room(mm float64) {
	if d.GetY()+mm > d.pageH-marginY {
		d.AddPage()
	}
}

// clean removes characters that draw nothing (zero-width spaces, joiners, soft hyphens, bidi
// controls, tag characters, control characters) so no part of the text is invisible, and folds
// tabs and runs of spaces into one space.
func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case unicode.Is(unicode.Cf, r), unicode.IsControl(r):
			return -1
		case unicode.IsSpace(r):
			return ' '
		}
		return r
	}, s)
	return strings.TrimSpace(spaceR.ReplaceAllString(s, " "))
}

// unescape removes the backslash render.Escape puts before a user line that starts with "#", "-" or "*".
func unescape(s string) string {
	if len(s) >= 2 && s[0] == '\\' && strings.ContainsRune("#-*", rune(s[1])) {
		return s[1:]
	}
	return s
}

// withScheme adds https:// to a web address that has no scheme.
func withScheme(u string) string {
	l := strings.ToLower(u)
	if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") {
		return u
	}
	return "https://" + u
}

// linkTarget returns a URL for a contact token that is an email address or a web address, else "".
func linkTarget(tok string) string {
	t := strings.TrimSpace(tok)
	switch {
	case emailR.MatchString(t):
		return "mailto:" + t
	case urlR.MatchString(t):
		return withScheme(t)
	}
	return ""
}

// bodyLinkTarget is linkTarget for a word inside body text, where only unmistakable addresses link.
func bodyLinkTarget(word string) string {
	switch {
	case emailR.MatchString(word):
		return "mailto:" + word
	case bodyURLR.MatchString(word):
		return withScheme(word)
	}
	return ""
}

// flow writes text as wrapped lines from the current position, with email and web addresses drawn as
// links (LinkString over the drawn words). Lines wrap at the left margin, so set it for a hanging
// indent before calling.
func (d *doc) flow(h float64, text string) {
	var plain strings.Builder // text since the last link, written in one run so extractors keep it whole
	flush := func() {
		if plain.Len() > 0 {
			d.Write(h, plain.String())
			plain.Reset()
		}
	}
	for i, w := range strings.Split(text, " ") {
		if i > 0 {
			plain.WriteByte(' ')
		}
		core := strings.TrimRight(w, ".,;:)]!?")
		lead := ""
		if strings.HasPrefix(core, "(") || strings.HasPrefix(core, "[") {
			lead, core = core[:1], core[1:]
		}
		target := bodyLinkTarget(core)
		if target == "" {
			plain.WriteString(w)
			continue
		}
		plain.WriteString(lead)
		flush()
		d.WriteLinkString(h, core, target)
		plain.WriteString(w[len(lead)+len(core):])
	}
	flush()
}

func (d *doc) name(text string) {
	d.SetFont(family, "B", nameSize)
	d.color(ink)
	d.MultiCell(d.usable, nameH, text, "", "L", false)
	d.Ln(0.8)
}

// contact writes the line under the name, with email addresses and web addresses as links.
func (d *doc) contact(line string) {
	d.SetFont(family, "", smallSize)
	d.color(muted)
	for i, part := range sepR.Split(line, -1) {
		if i > 0 {
			d.Write(smallH, "  ·  ")
		}
		if target := linkTarget(part); target != "" {
			d.WriteLinkString(smallH, part, target)
		} else {
			d.Write(smallH, part)
		}
	}
	d.Ln(6.5)
}

func (d *doc) section(title string) {
	d.room(22)
	d.Ln(3)
	d.SetFont(family, "B", sectionSize)
	d.color(ink)
	d.MultiCell(d.usable, sectionH, strings.ToUpper(title), "", "L", false)
	y := d.GetY() + 0.6
	d.SetDrawColor(rule[0], rule[1], rule[2])
	d.SetLineWidth(0.25)
	d.Line(marginX, y, marginX+d.usable, y)
	d.Ln(2.6)
}

// entry writes a role or school heading. When the next line holds dates, they go right-aligned on
// the same row if both fit; otherwise below the heading.
func (d *doc) entry(title, meta string) {
	d.room(16)
	d.Ln(1.4)
	d.SetFont(family, "B", entrySize)
	tw := d.GetStringWidth(title)
	d.SetFont(family, "", smallSize)
	mw := d.GetStringWidth(meta)
	if meta != "" && tw+mw+6 <= d.usable {
		d.SetFont(family, "B", entrySize)
		d.color(ink)
		d.CellFormat(d.usable-mw-2, entryH, title, "", 0, "L", false, 0, "")
		d.SetFont(family, "", smallSize)
		d.color(muted)
		d.CellFormat(mw+2, entryH, meta, "", 1, "R", false, 0, "")
	} else {
		d.SetFont(family, "B", entrySize)
		d.color(ink)
		d.MultiCell(d.usable, entryH, title, "", "L", false)
		if meta != "" {
			d.SetFont(family, "", smallSize)
			d.color(muted)
			d.MultiCell(d.usable, smallH, meta, "", "L", false)
		}
	}
	d.Ln(0.8)
}

func (d *doc) bullet(text string) {
	d.room(lineH)
	d.SetFont(family, "", bodySize)
	d.color(ink)
	d.SetX(marginX + 1.2)
	d.CellFormat(3.6, lineH, "•", "", 0, "L", false, 0, "")
	d.SetLeftMargin(marginX + bulletIndent)
	d.SetX(marginX + bulletIndent)
	d.flow(lineH, text)
	d.SetLeftMargin(marginX)
	d.Ln(lineH + 0.5)
}

func (d *doc) para(text string) {
	d.SetFont(family, "", bodySize)
	d.color(ink)
	d.SetX(marginX)
	d.flow(lineH, text)
	d.Ln(lineH + 1.2)
}

// isMeta reports a short line of dates and places that follows an entry heading.
func isMeta(s string) bool {
	return s != "" && len(s) <= 90 && !strings.HasPrefix(s, "- ") && !strings.HasPrefix(s, "#") && dateR.MatchString(s) && !strings.HasSuffix(s, ".")
}

func isBullet(t string) bool {
	return strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "• ")
}

// Render converts resume Markdown to PDF bytes, titled "Resume". See RenderFile.
func Render(markdown string) ([]byte, error) { return RenderFile(markdown, "Resume.pdf") }

// RenderFile converts the Markdown produced by the render package (#, ##, ###, "- " bullets, plain
// lines) to PDF bytes, following the Layout in the package comment. The document title is filename
// without its directory or extension. Other text renders as paragraphs, so saved resumes in any shape
// still export. Errors from the PDF library, including panics, are returned, so callers can offer
// the Markdown instead.
func RenderFile(markdown, filename string) (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("pdf: render failed: %v", r)
		}
	}()
	title := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if title == "" || title == "." || title == string(filepath.Separator) {
		title = "Resume"
	}
	d, err := newDoc(title)
	if err != nil {
		return nil, fmt.Errorf("pdf: %w", err)
	}
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = clean(lines[i])
	}
	next := func(i int) (string, int) { // next non-blank line after i
		for j := i + 1; j < len(lines); j++ {
			if lines[j] != "" {
				return lines[j], j
			}
		}
		return "", -1
	}
	afterName := false
	for i := 0; i < len(lines) && d.Ok(); i++ {
		t := lines[i]
		switch {
		case t == "":
			continue
		case strings.HasPrefix(t, "# "):
			d.name(unescape(strings.TrimSpace(t[2:])))
			afterName = true
			continue
		case afterName && !strings.HasPrefix(t, "#") && !isBullet(t):
			d.contact(t)
		case strings.HasPrefix(t, "## "):
			d.section(unescape(strings.TrimSpace(t[3:])))
		case strings.HasPrefix(t, "### "):
			meta := ""
			if n, j := next(i); j > 0 && isMeta(n) {
				meta, i = n, j
			}
			d.entry(unescape(strings.TrimSpace(t[4:])), meta)
		case isBullet(t):
			if b := unescape(strings.TrimSpace(t[len(strings.Fields(t)[0]):])); b != "" {
				d.bullet(b)
			}
		default:
			d.para(unescape(t))
		}
		afterName = false
	}
	if err := d.Error(); err != nil {
		return nil, fmt.Errorf("pdf: %w", err)
	}
	var buf bytes.Buffer
	if err := d.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf: %w", err)
	}
	return buf.Bytes(), nil
}
