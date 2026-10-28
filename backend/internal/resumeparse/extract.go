package resumeparse

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
)

// Visible text of imported document and separates the hidden text in it
type Extracted struct {
	Text   string
	Hidden Hidden
}

// Imported documents need at least 5 visible words; anything less is a scan or a page of hidden text
const minWords = 5

// Error messages if the imported document can't be read
var (
	errNoText = &Error{Reason: ReasonNoText,
		Message: "No selectable text was found in this PDF (it may be a scanned image). Paste the text instead."}
	errEncrypted = &Error{Reason: ReasonEncrypted,
		Message: "This PDF is password-protected. Remove the password (print it to a new PDF) and import again."}
	errUnreadable = &Error{Reason: ReasonUnreadable,
		Message: "The PDF could not be read. Try exporting it again from the program that made it."}
)

// Extracts the visible text of a PDF in reading order. Hidden pieces (isHidden) are dropped and reported.
// Per page, the rest is grouped into lines by baseline and sorted left to right, and a large vertical gap becomes a blank line.
// Word-processor artifacts are removed: overprinted glyphs, a decorative letter some heading fonts append to every run, private-use icon
// glyphs (Symbol-font bullets become •), and superscripts split onto their own line.
func TextFromPDF(data []byte) (out Extracted, err error) {
	defer func() {
		if r := recover(); r != nil { // the PDF library panics on some malformed files
			out, err = Extracted{}, errUnreadable
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		if errors.Is(err, pdf.ErrInvalidPassword) || strings.Contains(err.Error(), "encrypt") {
			return Extracted{}, errEncrypted
		}
		return Extracted{}, errUnreadable
	}
	log := &hiddenLog{}
	var pages [][]*line
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		content := readContent(p)
		pg := page{box: pageBox(p), fills: content.Fills}
		var visible []text
		var last *text // the last hidden piece, to space dropped words apart
		for _, t := range content.Text {
			reason, hidden := isHidden(t, pg)
			if !hidden {
				visible = append(visible, t)
				continue
			}
			if last != nil && (math.Abs(last.Y-t.Y) > 1 || t.X-(last.X+last.W) > math.Max(1, t.Size*0.15)) {
				log.gap()
			}
			log.add(reason, t.S, len([]rune(strings.TrimSpace(t.S))))
			last = &t
		}
		log.gap()
		pages = append(pages, buildLines(visible, log))
	}
	stripRunArtifacts(pages)
	var b strings.Builder
	for _, lines := range pages {
		b.WriteString(renderLines(lines))
		b.WriteString("\n\n")
	}
	text := stripInvisible(strings.TrimSpace(b.String()), log)
	if len(strings.Fields(text)) < minWords {
		return Extracted{}, errNoText
	}
	return Extracted{Text: text, Hidden: log.report()}, nil
}

type line struct {
	y, size float64
	parts   []text
}

// Maps Symbol-font bullets to "•" and drops other private-use (icon) characters.
func cleanGlyph(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == 0xf0b7 || r == 0xf0a7:
			b.WriteRune('•')
		case r >= 0xe000 && r <= 0xf8ff:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Groups by baseline: join if within 2pt or 35% of font size; else start a new line
// Lone icons are skipped
func buildLines(texts []text, log *hiddenLog) []*line {
	var lines []*line
	for _, t := range texts {
		t.S = cleanGlyph(t.S)
		if t.S == "" {
			continue
		}
		var hit *line
		for _, l := range lines {
			if math.Abs(l.y-t.Y) <= math.Max(2, t.FontSize*0.35) {
				hit = l
				break
			}
		}
		if hit == nil {
			hit = &line{y: t.Y, size: t.FontSize}
			lines = append(lines, hit)
		}
		hit.parts = append(hit.parts, t)
		hit.size = math.Max(hit.size, t.FontSize)
	}

	sort.Slice(lines, func(i, j int) bool { return lines[i].y > lines[j].y })
	for _, l := range lines {
		sort.SliceStable(l.parts, func(i, j int) bool { return l.parts[i].X < l.parts[j].X })
		l.parts = dropOverprints(l.parts, log)
	}

	// A line of small text just above a larger line is a superscript ("2nd"): merge it down.
	var out []*line
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if i+1 < len(lines) {
			next := lines[i+1]
			if l.size < next.size*0.8 && l.y-next.y < next.size*0.7 && len(l.parts) <= 4 {
				next.parts = append(next.parts, l.parts...)
				sort.SliceStable(next.parts, func(a, b int) bool { return next.parts[a].X < next.parts[b].X })
				next.parts = dropOverprints(next.parts, log)
				continue
			}
		}
		out = append(out, l)
	}
	return out
}

// Keep one glyph from each group drawn at the same position. Word processors leave glyphs under real ones; the real glyph is the one whose width
// reaches the next glyph. Dropped glyphs are logged as hidden text.
func dropOverprints(parts []text, log *hiddenLog) []text {
	var out []text
	for i := 0; i < len(parts); {
		j := i + 1
		// Without glyph widths (standard fonts) positions carry no overlap information.
		for parts[i].W > 0 && j < len(parts) && parts[j].W > 0 && parts[j].X-parts[i].X < math.Max(0.6, parts[i].W*0.3) && len([]rune(parts[j].S)) == 1 {
			j++
		}
		if j-i == 1 {
			out = append(out, parts[i])
			i = j
			continue
		}
		best := j - 1 // with nothing after the group, keep the last drawn
		if j < len(parts) {
			nextX := parts[j].X
			bestD := math.Inf(1)
			for k := i; k < j; k++ {
				if d := math.Abs(parts[k].X + parts[k].W - nextX); d < bestD {
					best, bestD = k, d
				}
			}
		}
		for k := i; k < j; k++ {
			// The same letter drawn twice in one place is bold emulation, not hidden text.
			if k != best && parts[k].S != parts[best].S {
				log.gap()
				log.add(HiddenOverprint, parts[k].S, len([]rune(strings.TrimSpace(parts[k].S))))
			}
		}
		out = append(out, parts[best])
		i = j
	}
	return out
}

// stripRunArtifacts removes a character that a font appends to nearly every run of text. Some
// templates draw headings with a decorative glyph that extracts as a letter ("Experienceu").
func stripRunArtifacts(pages [][]*line) {
	type stat struct {
		runs int
		ends map[string]int
	}
	stats := map[string]*stat{}
	// A run is consecutive glyphs in one font at one size; headings often share a font with body text
	// at a different size.
	key := func(t text) string { return fmt.Sprintf("%s@%.0f", t.Font, t.FontSize) }
	for _, lines := range pages {
		for _, l := range lines {
			start := 0
			for i := 1; i <= len(l.parts); i++ {
				if i < len(l.parts) && key(l.parts[i]) == key(l.parts[start]) {
					continue
				}
				if run := l.parts[start:i]; len(run) >= 3 {
					st := stats[key(run[0])]
					if st == nil {
						st = &stat{ends: map[string]int{}}
						stats[key(run[0])] = st
					}
					st.runs++
					st.ends[run[len(run)-1].S]++
				}
				start = i
			}
		}
	}
	strip := map[string]string{}
	byFont := map[string]string{} // font name -> artifact char, for one-off sizes such as the name line
	for font, st := range stats {
		for ch, c := range st.ends {
			r := []rune(ch)
			if st.runs >= 3 && len(r) == 1 && unicode.IsLetter(r[0]) && float64(c)/float64(st.runs) >= 0.8 {
				strip[font] = ch
				byFont[font[:strings.LastIndex(font, "@")]] = ch
			}
		}
	}
	if len(strip) == 0 {
		return
	}
	for _, lines := range pages {
		for _, l := range lines {
			kept := l.parts[:0]
			wholeLine := len(l.parts) > 0
			for _, t := range l.parts {
				wholeLine = wholeLine && t.Font == l.parts[0].Font
			}
			for i, t := range l.parts {
				ch, ok := strip[key(t)]
				if !ok && wholeLine { // a line drawn entirely in an affected font, at another size
					ch, ok = byFont[t.Font]
				}
				endOfRun := i+1 == len(l.parts) || key(l.parts[i+1]) != key(t)
				if ok && endOfRun && t.S == ch {
					continue
				}
				kept = append(kept, t)
			}
			l.parts = kept
		}
	}
}

// Gap over 15% of font size adds a space
// Gap over 3x font size is a column break
// Vertical jump of 1.9x of line size becomes a blank line
func renderLines(lines []*line) string {
	var b strings.Builder
	prevY := math.NaN()
	for _, l := range lines {
		var sb strings.Builder
		end := math.Inf(-1)
		for _, t := range l.parts {
			gap := t.X - end
			if sb.Len() > 0 && gap > math.Max(1, t.FontSize*0.15) && !strings.HasSuffix(sb.String(), " ") && !strings.HasPrefix(t.S, " ") {
				if gap > t.FontSize*3 {
					sb.WriteString("  |  ") // a wide gap separates columns, such as a title and a right-aligned date
				} else {
					sb.WriteString(" ")
				}
			}
			sb.WriteString(t.S)
			end = t.X + t.W
		}
		if !math.IsNaN(prevY) && prevY-l.y > l.size*1.9 {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimSpace(strings.Join(strings.Fields(sb.String()), " ")))
		b.WriteString("\n")
		prevY = l.y
	}
	return b.String()
}

// TextFromPlain prepares text from a .txt or .md file or pasted text: a leading byte order mark is
// removed, line endings become "\n", and invisible characters are dropped and reported.
func TextFromPlain(text string) Extracted {
	log := &hiddenLog{}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = stripInvisible(text, log)
	return Extracted{Text: strings.TrimSpace(text), Hidden: log.report()}
}
