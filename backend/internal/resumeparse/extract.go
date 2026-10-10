// Package resumeparse turns an existing resume (PDF or text) into a draft master resume for review.
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

// ErrNoText means the PDF has no text layer, usually a scanned image.
var ErrNoText = errors.New("this PDF has no selectable text (it may be a scanned image); paste the text instead")

// TextFromPDF extracts text line by line. Pieces on the same baseline are joined left to right, and a
// larger vertical gap becomes a blank line so paragraphs and entries stay apart. Common artifacts of
// word processors are removed: overprinted glyphs, a decorative character a font appends to every run,
// private-use icon glyphs, and superscripts split onto their own line.
func TextFromPDF(data []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil { // the PDF library panics on some malformed files
			text, err = "", fmt.Errorf("could not read this PDF: %v", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("could not read this PDF: %w", err)
	}
	var pages [][]*line
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		pages = append(pages, buildLines(p.Content().Text))
	}
	stripRunArtifacts(pages)
	var b strings.Builder
	for _, lines := range pages {
		b.WriteString(renderLines(lines))
		b.WriteString("\n\n")
	}
	out := strings.TrimSpace(b.String())
	if len(strings.Fields(out)) < 5 {
		return "", ErrNoText
	}
	return out, nil
}

type line struct {
	y, size float64
	parts   []pdf.Text
}

// cleanGlyph maps the Symbol-font bullet to "•" and drops other private-use (icon) characters.
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

func buildLines(texts []pdf.Text) []*line {
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
		l.parts = dropOverprints(l.parts)
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
				next.parts = dropOverprints(next.parts)
				continue
			}
		}
		out = append(out, l)
	}
	return out
}

// dropOverprints keeps one glyph from each group drawn at the same position. Word processors leave
// invisible characters under real ones; the real glyph is the one whose width reaches the next glyph.
func dropOverprints(parts []pdf.Text) []pdf.Text {
	var out []pdf.Text
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
		out = append(out, parts[best])
		i = j
	}
	return out
}

// stripRunArtifacts removes a character that a font appends to nearly every run of text. Some
// templates draw headings with a decorative glyph that extracts as a letter ("Experienceu").
func stripRunArtifacts(pages [][]*line) {
	type stat struct{ runs, ends map[string]int }
	stats := map[string]*stat{}
	// A run is consecutive glyphs in one font at one size; headings often share a font with body text
	// at a different size.
	key := func(t pdf.Text) string { return fmt.Sprintf("%s@%.0f", t.Font, t.FontSize) }
	forRuns := func(fn func(font string, run []pdf.Text)) {
		for _, lines := range pages {
			for _, l := range lines {
				start := 0
				for i := 1; i <= len(l.parts); i++ {
					if i == len(l.parts) || key(l.parts[i]) != key(l.parts[start]) {
						fn(key(l.parts[start]), l.parts[start:i])
						start = i
					}
				}
			}
		}
	}
	forRuns(func(font string, run []pdf.Text) {
		if len(run) < 3 {
			return
		}
		st := stats[font]
		if st == nil {
			st = &stat{runs: map[string]int{}, ends: map[string]int{}}
			stats[font] = st
		}
		st.runs[""]++
		st.ends[run[len(run)-1].S]++
	})
	strip := map[string]string{}
	byFont := map[string]string{} // font name -> artifact char, for one-off sizes such as the name line
	for font, st := range stats {
		n := st.runs[""]
		for ch, c := range st.ends {
			r := []rune(ch)
			if n >= 3 && len(r) == 1 && unicode.IsLetter(r[0]) && float64(c)/float64(n) >= 0.8 {
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
