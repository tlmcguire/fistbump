// Package pdf renders resume Markdown to a one-column PDF with an embedded Unicode font.
//
// # Styles
//
// Five built-in styles are supported: "classic" (default), "modern", "minimal", "compact", and
// "executive". Every style keeps text dark on white, at MinFontSize (9 pt) or larger, with no
// filled background rectangles and no hidden characters.
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

	// Default margins (used by tests to validate the classic layout).
	marginX = 18.0 // mm, left and right
	marginY = 16.0 // mm, top and bottom

	smallSize = 9.0 // smallest text in the file

	// Creator is written to the PDF's metadata.
	Creator = "FistBump"
)

// MinFontSize is the smallest text size Render uses, in points.
const MinFontSize = smallSize

// Style names accepted by RenderStyled.
const (
	StyleClassic   = "classic"
	StyleModern    = "modern"
	StyleMinimal   = "minimal"
	StyleCompact   = "compact"
	StyleExecutive = "executive"
)

// StyleInfo describes a selectable PDF style.
type StyleInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Styles returns the available PDF style options in display order.
func Styles() []StyleInfo {
	return []StyleInfo{
		{ID: StyleClassic, Name: "Classic", Description: "Clean, balanced layout with clear section rules."},
		{ID: StyleModern, Name: "Modern", Description: "Deep navy accents with crisp visual hierarchy."},
		{ID: StyleMinimal, Name: "Minimal", Description: "Centered header with airy spacing and hairline rules."},
		{ID: StyleCompact, Name: "Compact", Description: "Tighter margins and spacing to fit more on one page."},
		{ID: StyleExecutive, Name: "Executive", Description: "Strong header divider with deep slate-green accents."},
	}
}

// ValidStyle reports whether name is a recognized style ID (empty means classic).
func ValidStyle(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", StyleClassic, StyleModern, StyleMinimal, StyleCompact, StyleExecutive:
		return true
	}
	return false
}

// theme defines all visual parameters for a PDF style. All ink/accent RGB channels stay <= 125
// so text always remains dark and clearly readable on white paper.
type theme struct {
	marginX      float64
	marginY      float64
	nameSize     float64
	sectionSize  float64
	entrySize    float64
	bodySize     float64
	smallSize    float64
	nameH        float64
	sectionH     float64
	entryH       float64
	lineH        float64
	smallH       float64
	bulletIndent float64
	contactGap   float64
	sectionGap   float64
	entryGap     float64
	bulletGap    float64
	paraGap      float64
	ruleWidth    float64
	centered     bool
	headerRule   bool
	doubleRule   bool
	ink          [3]int
	accent       [3]int
	muted        [3]int
	rule         [3]int
}

func themeFor(style string) theme {
	base := theme{
		marginX:      marginX,
		marginY:      marginY,
		nameSize:     21.0,
		sectionSize:  9.4,
		entrySize:    10.4,
		bodySize:     9.8,
		smallSize:    smallSize,
		nameH:        9.0,
		sectionH:     5.0,
		entryH:       5.2,
		lineH:        4.7,
		smallH:       4.6,
		bulletIndent: 4.8,
		contactGap:   6.0,
		sectionGap:   3.2,
		entryGap:     1.6,
		bulletGap:    0.6,
		paraGap:      1.3,
		ruleWidth:    0.25,
		ink:          [3]int{17, 24, 39},
		accent:       [3]int{17, 24, 39},
		muted:        [3]int{82, 90, 110},
		rule:         [3]int{110, 118, 132},
	}
	switch strings.ToLower(strings.TrimSpace(style)) {
	case StyleModern:
		base.marginX = 17.0
		base.marginY = 15.0
		base.nameSize = 22.0
		base.sectionSize = 9.5
		base.entrySize = 10.5
		base.ruleWidth = 0.45
		base.headerRule = true
		base.accent = [3]int{24, 52, 102} // deep navy
		base.rule = [3]int{45, 76, 122}
		base.muted = [3]int{75, 85, 105}
	case StyleMinimal:
		base.marginX = 20.0
		base.marginY = 18.0
		base.nameSize = 20.0
		base.sectionSize = 9.2
		base.entrySize = 10.2
		base.bodySize = 9.6
		base.lineH = 4.9
		base.contactGap = 7.0
		base.sectionGap = 4.0
		base.entryGap = 2.0
		base.ruleWidth = 0.18
		base.centered = true
		base.ink = [3]int{24, 24, 27}
		base.accent = [3]int{24, 24, 27}
		base.muted = [3]int{92, 92, 100}
		base.rule = [3]int{115, 115, 122}
	case StyleCompact:
		base.marginX = 12.5
		base.marginY = 12.0
		base.nameSize = 18.0
		base.sectionSize = 9.0
		base.entrySize = 9.8
		base.bodySize = 9.2
		base.smallSize = 9.0
		base.nameH = 7.6
		base.sectionH = 4.4
		base.entryH = 4.6
		base.lineH = 4.2
		base.smallH = 4.2
		base.bulletIndent = 4.2
		base.contactGap = 4.2
		base.sectionGap = 2.2
		base.entryGap = 1.0
		base.bulletGap = 0.3
		base.paraGap = 0.8
		base.ruleWidth = 0.22
	case StyleExecutive:
		base.marginX = 17.5
		base.marginY = 15.5
		base.nameSize = 22.5
		base.sectionSize = 9.6
		base.entrySize = 10.6
		base.bodySize = 9.8
		base.headerRule = true
		base.doubleRule = true
		base.ruleWidth = 0.4
		base.accent = [3]int{20, 62, 56} // deep slate-green
		base.rule = [3]int{36, 80, 74}
		base.muted = [3]int{78, 88, 96}
	}
	return base
}

var (
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
	th     theme
	usable float64
	pageH  float64
}

func newDoc(title, style string) (*doc, error) {
	th := themeFor(style)
	p := fpdf.New("P", "mm", "Letter", "")
	p.SetCompression(compress)
	p.AddUTF8FontFromBytes(family, "", fonts.Regular)
	p.AddUTF8FontFromBytes(family, "B", fonts.Bold)
	p.SetMargins(th.marginX, th.marginY, th.marginX)
	p.SetAutoPageBreak(true, th.marginY)
	p.SetTitle(title, true)
	p.SetCreator(Creator, true)
	p.AddPage()
	w, h := p.GetPageSize()
	return &doc{Fpdf: p, th: th, usable: w - 2*th.marginX, pageH: h}, p.Error()
}

func (d *doc) color(c [3]int) { d.SetTextColor(c[0], c[1], c[2]) }

// room starts a new page when less than mm of space is left, so a heading is never stranded at the
// bottom of a page without the text it introduces.
func (d *doc) room(mm float64) {
	if d.GetY()+mm > d.pageH-d.th.marginY {
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
	d.SetFont(family, "B", d.th.nameSize)
	d.color(d.th.accent)
	align := "L"
	if d.th.centered {
		align = "C"
	}
	d.MultiCell(d.usable, d.th.nameH, text, "", align, false)
	d.Ln(0.8)
}

// contact writes the line under the name, with email addresses and web addresses as links.
func (d *doc) contact(line string) {
	d.SetFont(family, "", d.th.smallSize)
	d.color(d.th.muted)
	parts := sepR.Split(line, -1)
	if d.th.centered {
		joined := strings.Join(parts, "  ·  ")
		w := d.GetStringWidth(joined)
		if w < d.usable {
			d.SetX(d.th.marginX + (d.usable-w)/2)
		}
	}
	for i, part := range parts {
		if i > 0 {
			d.Write(d.th.smallH, "  ·  ")
		}
		if target := linkTarget(part); target != "" {
			d.WriteLinkString(d.th.smallH, part, target)
		} else {
			d.Write(d.th.smallH, part)
		}
	}
	if d.th.headerRule {
		d.Ln(d.th.smallH + 1.2)
		y := d.GetY()
		d.SetDrawColor(d.th.rule[0], d.th.rule[1], d.th.rule[2])
		d.SetLineWidth(d.th.ruleWidth)
		d.Line(d.th.marginX, y, d.th.marginX+d.usable, y)
		if d.th.doubleRule {
			d.SetLineWidth(0.18)
			d.Line(d.th.marginX, y+0.9, d.th.marginX+d.usable, y+0.9)
		}
		d.Ln(2.4)
	} else {
		d.Ln(d.th.contactGap)
	}
}

func (d *doc) section(title string) {
	d.room(22)
	d.Ln(d.th.sectionGap)
	d.SetFont(family, "B", d.th.sectionSize)
	d.color(d.th.accent)
	align := "L"
	if d.th.centered {
		align = "C"
	}
	d.MultiCell(d.usable, d.th.sectionH, strings.ToUpper(title), "", align, false)
	y := d.GetY() + 0.6
	d.SetDrawColor(d.th.rule[0], d.th.rule[1], d.th.rule[2])
	d.SetLineWidth(d.th.ruleWidth)
	d.Line(d.th.marginX, y, d.th.marginX+d.usable, y)
	d.Ln(2.4)
}

// entry writes a role or school heading. When the next line holds dates, they go right-aligned on
// the same row if both fit; otherwise below the heading.
func (d *doc) entry(title, meta string) {
	d.room(16)
	d.Ln(d.th.entryGap)
	d.SetFont(family, "B", d.th.entrySize)
	tw := d.GetStringWidth(title)
	d.SetFont(family, "", d.th.smallSize)
	mw := d.GetStringWidth(meta)
	if meta != "" && tw+mw+6 <= d.usable {
		d.SetFont(family, "B", d.th.entrySize)
		d.color(d.th.ink)
		d.CellFormat(d.usable-mw-2, d.th.entryH, title, "", 0, "L", false, 0, "")
		d.SetFont(family, "", d.th.smallSize)
		d.color(d.th.muted)
		d.CellFormat(mw+2, d.th.entryH, meta, "", 1, "R", false, 0, "")
	} else {
		d.SetFont(family, "B", d.th.entrySize)
		d.color(d.th.ink)
		d.MultiCell(d.usable, d.th.entryH, title, "", "L", false)
		if meta != "" {
			d.SetFont(family, "", d.th.smallSize)
			d.color(d.th.muted)
			d.MultiCell(d.usable, d.th.smallH, meta, "", "L", false)
		}
	}
	d.Ln(0.7)
}

func (d *doc) bullet(text string) {
	d.room(d.th.lineH)
	d.SetFont(family, "", d.th.bodySize)
	d.color(d.th.ink)
	d.SetX(d.th.marginX + 1.2)
	d.CellFormat(3.6, d.th.lineH, "•", "", 0, "L", false, 0, "")
	d.SetLeftMargin(d.th.marginX + d.th.bulletIndent)
	d.SetX(d.th.marginX + d.th.bulletIndent)
	d.flow(d.th.lineH, text)
	d.SetLeftMargin(d.th.marginX)
	d.Ln(d.th.lineH + d.th.bulletGap)
}

func (d *doc) para(text string) {
	d.SetFont(family, "", d.th.bodySize)
	d.color(d.th.ink)
	d.SetX(d.th.marginX)
	d.flow(d.th.lineH, text)
	d.Ln(d.th.lineH + d.th.paraGap)
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

// RenderFile converts resume Markdown to PDF bytes using the classic style.
func RenderFile(markdown, filename string) ([]byte, error) {
	return RenderStyled(markdown, filename, StyleClassic)
}

// RenderStyled converts the Markdown produced by the render package (#, ##, ###, "- " bullets, plain
// lines) to PDF bytes using the chosen style ("classic", "modern", "minimal", "compact", "executive").
func RenderStyled(markdown, filename, style string) (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("pdf: render failed: %v", r)
		}
	}()
	title := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if title == "" || title == "." || title == string(filepath.Separator) {
		title = "Resume"
	}
	d, err := newDoc(title, style)
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
