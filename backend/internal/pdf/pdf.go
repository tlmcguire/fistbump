// Package pdf renders resume Markdown to a one-column PDF with an embedded Unicode font.
package pdf

import (
	"bytes"
	"embed"
	"regexp"
	"strings"

	"github.com/go-pdf/fpdf"
)

// Inter (SIL Open Font License, see fonts/OFL.txt). Only the glyphs a resume uses are embedded.
//
//go:embed fonts/*.ttf
var fontFS embed.FS

const (
	margin   = 18.0 // mm
	family   = "Inter"
	bodySize = 9.8
	lineH    = 4.7
)

var (
	ink    = [3]int{17, 24, 39}
	muted  = [3]int{88, 96, 120}
	rule   = [3]int{200, 205, 218}
	emailR = regexp.MustCompile(`^[\w.+-]+@[\w-]+\.[\w.-]+$`)
	urlR   = regexp.MustCompile(`(?i)^(https?://)?([a-z0-9-]+\.)+[a-z]{2,}(/\S*)?$`)
	dateR  = regexp.MustCompile(`(?i)\b(19|20)\d{2}\b|present|current`)
	sepR   = regexp.MustCompile(`\s+[|·•]\s+`)
)

type doc struct {
	*fpdf.Fpdf
	usable float64
	pageH  float64
}

func newDoc() (*doc, error) {
	p := fpdf.New("P", "mm", "Letter", "")
	for _, f := range [][2]string{{"", "Inter-Regular.ttf"}, {"B", "Inter-Bold.ttf"}, {"I", "Inter-Italic.ttf"}} {
		data, err := fontFS.ReadFile("fonts/" + f[1])
		if err != nil {
			return nil, err
		}
		p.AddUTF8FontFromBytes(family, f[0], data)
	}
	semi, err := fontFS.ReadFile("fonts/Inter-SemiBold.ttf")
	if err != nil {
		return nil, err
	}
	p.AddUTF8FontFromBytes(family+"Semi", "", semi)
	p.SetMargins(margin, 16, margin)
	p.SetAutoPageBreak(true, 16)
	p.SetCreator("fistbump", true)
	p.AddPage()
	w, h := p.GetPageSize()
	return &doc{Fpdf: p, usable: w - 2*margin, pageH: h}, nil
}

func (d *doc) color(c [3]int) { d.SetTextColor(c[0], c[1], c[2]) }

// room starts a new page when less than mm of space is left, so a heading is never stranded at the
// bottom of a page without the text it introduces.
func (d *doc) room(mm float64) {
	if d.GetY()+mm > d.pageH-16 {
		d.AddPage()
	}
}

// unescape removes the backslash the render package puts before a user line that starts with "#".
func unescape(s string) string {
	if strings.HasPrefix(s, `\#`) {
		return s[1:]
	}
	return s
}

// linkTarget returns a URL for a contact token that is an email address or a web address, else "".
func linkTarget(tok string) string {
	t := strings.TrimSpace(tok)
	switch {
	case emailR.MatchString(t):
		return "mailto:" + t
	case urlR.MatchString(t) && strings.Contains(t, "."):
		if strings.HasPrefix(strings.ToLower(t), "http") {
			return t
		}
		return "https://" + t
	}
	return ""
}

// contact writes the line under the name, with email addresses and web addresses as links.
func (d *doc) contact(line string) {
	d.SetFont(family, "", 9)
	d.color(muted)
	parts := sepR.Split(line, -1)
	for i, part := range parts {
		if i > 0 {
			d.Write(4.6, "  ·  ")
		}
		if target := linkTarget(part); target != "" {
			d.WriteLinkString(4.6, part, target)
		} else {
			d.Write(4.6, part)
		}
	}
	d.Ln(6.5)
}

func (d *doc) section(title string) {
	d.room(22)
	d.Ln(3)
	d.SetFont(family, "B", 9.2)
	d.color(ink)
	d.CellFormat(d.usable, 5, strings.ToUpper(title), "", 1, "L", false, 0, "")
	y := d.GetY() + 0.6
	d.SetDrawColor(rule[0], rule[1], rule[2])
	d.SetLineWidth(0.25)
	d.Line(margin, y, margin+d.usable, y)
	d.Ln(2.6)
}

// entry writes a role or school heading. When the next line holds dates, they go right-aligned on
// the same row if both fit; otherwise below the heading.
func (d *doc) entry(title, meta string) {
	d.room(16)
	d.Ln(1.4)
	d.SetFont(family+"Semi", "", 10.3)
	d.color(ink)
	tw := d.GetStringWidth(title)
	d.SetFont(family, "", 9)
	mw := d.GetStringWidth(meta)
	if meta != "" && tw+mw+6 <= d.usable {
		d.SetFont(family+"Semi", "", 10.3)
		d.color(ink)
		d.CellFormat(d.usable-mw-2, 5.2, title, "", 0, "L", false, 0, "")
		d.SetFont(family, "", 9)
		d.color(muted)
		d.CellFormat(mw+2, 5.2, meta, "", 1, "R", false, 0, "")
	} else {
		d.SetFont(family+"Semi", "", 10.3)
		d.color(ink)
		d.MultiCell(d.usable, 5.2, title, "", "L", false)
		if meta != "" {
			d.SetFont(family, "", 9)
			d.color(muted)
			d.MultiCell(d.usable, 4.6, meta, "", "L", false)
		}
	}
	d.Ln(0.8)
}

func (d *doc) bullet(text string) {
	d.SetFont(family, "", bodySize)
	d.color(ink)
	d.SetX(margin + 1.2)
	d.CellFormat(3.6, lineH, "•", "", 0, "L", false, 0, "")
	d.SetX(margin + 4.8)
	d.MultiCell(d.usable-4.8, lineH, text, "", "L", false)
	d.Ln(0.5)
}

func (d *doc) para(text string) {
	d.SetFont(family, "", bodySize)
	d.color(ink)
	d.MultiCell(d.usable, lineH, text, "", "L", false)
	d.Ln(1.2)
}

// isMeta reports a short line of dates and places that follows an entry heading.
func isMeta(s string) bool {
	return s != "" && len(s) <= 90 && !strings.HasPrefix(s, "- ") && !strings.HasPrefix(s, "#") && dateR.MatchString(s) && !strings.HasSuffix(s, ".")
}

// Render converts the Markdown produced by the render package (#, ##, ###, "- " bullets, plain lines)
// to PDF bytes. Other text renders as paragraphs, so saved resumes in any shape still export.
func Render(markdown string) ([]byte, error) {
	d, err := newDoc()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	next := func(i int) (string, int) { // next non-blank line after i
		for j := i + 1; j < len(lines); j++ {
			if t := strings.TrimSpace(lines[j]); t != "" {
				return t, j
			}
		}
		return "", -1
	}
	afterName := false
	for i := 0; i < len(lines); i++ {
		l := strings.TrimRight(lines[i], " \t")
		t := strings.TrimSpace(l)
		switch {
		case t == "":
			continue
		case strings.HasPrefix(t, "# "):
			d.SetFont(family, "B", 21)
			d.color(ink)
			d.MultiCell(d.usable, 9, strings.TrimPrefix(t, "# "), "", "L", false)
			d.Ln(0.8)
			afterName = true
			continue
		case afterName && !strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "- "):
			d.contact(t)
		case strings.HasPrefix(t, "## "):
			d.section(strings.TrimPrefix(t, "## "))
		case strings.HasPrefix(t, "### "):
			meta := ""
			if n, j := next(i); j > 0 && isMeta(n) {
				meta, i = n, j
			}
			d.entry(strings.TrimPrefix(t, "### "), meta)
		case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "• "):
			d.bullet(unescape(strings.TrimSpace(t[2:])))
		default:
			d.para(unescape(t))
		}
		afterName = false
	}
	var buf bytes.Buffer
	if err := d.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
