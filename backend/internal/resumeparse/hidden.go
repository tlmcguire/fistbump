package resumeparse

import (
	"math"
	"slices"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/tlmcguire/fistbump/backend/internal/textclean"
)

// Reasons text is hidden
const (
	HiddenRenderMode = "invisible text mode"
	HiddenOffPage    = "placed off the page"
	HiddenTiny       = "font smaller than 1 pt"
	HiddenSameColor  = "same color as the background"
	HiddenOverprint  = "drawn under other text"
	HiddenInvisible  = "invisible characters"
)

// Hidden reports text that was left out of an import because a reader would not see it.
type Hidden struct {
	Words      int      `json:"words"`
	Characters int      `json:"characters"`
	Reasons    []string `json:"reasons"`
	Sample     string   `json:"sample"`
}

// sampleLen is how much of the dropped text the user is shown.
const sampleLen = 200

// hiddenLog collects dropped text in drawing order and builds the report.
type hiddenLog struct {
	reasons []string
	text    strings.Builder
	chars   int
}

func (h *hiddenLog) add(reason, text string, chars int) {
	if chars == 0 {
		if text != "" { // a dropped space still separates the words around it
			h.gap()
		}
		return
	}
	if !slices.Contains(h.reasons, reason) {
		h.reasons = append(h.reasons, reason)
	}
	h.chars += chars
	h.text.WriteString(text)
}

// Separates two dropped pieces so their words don't run together.
func (h *hiddenLog) gap() {
	if s := h.text.String(); s != "" && !strings.HasSuffix(s, " ") {
		h.text.WriteByte(' ')
	}
}

// Returns the zero Hidden when nothing was hidden, so the API sends hidden_text: null.
func (h *hiddenLog) report() Hidden {
	if h.chars == 0 {
		return Hidden{}
	}
	all := strings.Join(strings.Fields(h.text.String()), " ")
	return Hidden{Words: len(strings.Fields(all)), Characters: h.chars, Reasons: h.reasons, Sample: clip(all)}
}

func clip(s string) string {
	if r := []rune(s); len(r) > sampleLen {
		return string(r[:sampleLen])
	}
	return s
}

// Empty reports whether nothing was hidden.
func (h Hidden) Empty() bool { return len(h.Reasons) == 0 }

// Merge combines two reports: counts are summed, reasons joined without repeats, and the sample
// keeps the first sampleLen characters of both.
func (h Hidden) Merge(o Hidden) Hidden {
	if o.Empty() {
		return h
	}
	if h.Empty() {
		return o
	}
	out := Hidden{Words: h.Words + o.Words, Characters: h.Characters + o.Characters,
		Reasons: slices.Clone(h.Reasons), Sample: h.Sample}
	for _, r := range o.Reasons {
		if !slices.Contains(out.Reasons, r) {
			out.Reasons = append(out.Reasons, r)
		}
	}
	if o.Sample != "" {
		out.Sample = clip(strings.TrimSpace(out.Sample + " " + o.Sample))
	}
	return out
}

// CleanText removes invisible characters (textclean.StripInvisible) and reports them. Every
// import path calls it: .txt and .md files, pasted text, and PDF text after extraction.
func CleanText(text string) (string, Hidden) {
	log := &hiddenLog{}
	out := stripInvisible(text, log)
	return out, log.report()
}

// Removes invisible characters with textclean.StripInvisible and logs them. A single
// leading byte order mark is removed without being reported. A run of Unicode tag characters can
// spell out words no viewer shows (U+E0041 is a hidden "A"), so tag text is decoded for the sample.
func stripInvisible(text string, log *hiddenLog) string {
	text = strings.TrimPrefix(text, "\uFEFF")
	out, n := textclean.StripInvisible(text)
	if n == 0 {
		return out
	}
	var spelled strings.Builder
	for _, r := range text {
		if r >= 0xE0020 && r <= 0xE007E {
			spelled.WriteRune(r - 0xE0000)
		} else if spelled.Len() > 0 && !strings.HasSuffix(spelled.String(), " ") {
			spelled.WriteByte(' ')
		}
	}
	log.gap()
	log.add(HiddenInvisible, strings.TrimSpace(spelled.String()), n)
	return out
}

// What the hidden-text rules need to know about one PDF page.
type page struct {
	box   pdf.Rect
	fills []filledRect
}

// How close (per channel, 0 to 1) text may be to its background and still count as the same color.
const colorTolerance = 0.1

// Smallest font size, in points, a reader can see.
const minVisibleSize = 1.0

// Lets text touch the page edge, where rounding can place it a hair outside.
const pageEdgeSlack = 0.5

// Applies the hidden-text rules in order: render mode, off the page, microscopic, same
// color as the background. Overprinted glyphs are found while lines are built (dropOverprints).
func isHidden(t text, p page) (reason string, hidden bool) {
	if t.RenderMode == 3 || t.RenderMode == 7 {
		return HiddenRenderMode, true
	}
	if offPage(t, p.box) {
		return HiddenOffPage, true
	}
	if t.Size < minVisibleSize {
		return HiddenTiny, true
	}
	if t.FillKnown {
		bg, known := background(t, p.fills)
		if known && colorDistance(t.Fill, bg) <= colorTolerance {
			return HiddenSameColor, true
		}
	}
	return "", false
}

func offPage(t text, box pdf.Rect) bool {
	if box.Max.X <= box.Min.X || box.Max.Y <= box.Min.Y {
		return false // no usable box: nothing to compare against
	}
	return t.X < box.Min.X-pageEdgeSlack || t.X > box.Max.X+pageEdgeSlack ||
		t.Y < box.Min.Y-pageEdgeSlack || t.Y > box.Max.Y+pageEdgeSlack
}

// Returns the color under the text: the last filled rectangle drawn before it that contains its position, default to white; known is false when
// that rectangle's color could not be read, so no comparison is made.
func background(t text, fills []filledRect) (color [3]float64, known bool) {
	x, y := t.X+t.W/2, t.Y+t.Size*0.3
	for i := len(fills) - 1; i >= 0; i-- {
		f := fills[i]
		if f.Seq > t.Seq {
			continue
		}
		if x >= f.Min.X && x <= f.Max.X && y >= f.Min.Y && y <= f.Max.Y {
			return f.Fill, f.FillKnown
		}
	}
	return [3]float64{1, 1, 1}, true
}

// Largest per-channel difference between two RGB colors.
func colorDistance(a, b [3]float64) float64 {
	return math.Max(math.Abs(a[0]-b[0]), math.Max(math.Abs(a[1]-b[1]), math.Abs(a[2]-b[2])))
}

// Reads the crop box as a rectangle
func pageBox(p pdf.Page) pdf.Rect {
	box := inherited(p, "CropBox")
	if box.Kind() != pdf.Array || box.Len() != 4 {
		box = inherited(p, "MediaBox")
	}
	if box.Kind() != pdf.Array || box.Len() != 4 {
		return pdf.Rect{}
	}
	x1, y1, x2, y2 := box.Index(0).Float64(), box.Index(1).Float64(), box.Index(2).Float64(), box.Index(3).Float64()
	return pdf.Rect{
		Min: pdf.Point{X: math.Min(x1, x2), Y: math.Min(y1, y2)},
		Max: pdf.Point{X: math.Max(x1, x2), Y: math.Max(y1, y2)},
	}
}

// Reads a page attribute that may be set on the page or on a parent in the page tree.
func inherited(p pdf.Page, key string) pdf.Value {
	for v, depth := p.V, 0; !v.IsNull() && depth < 32; v, depth = v.Key("Parent"), depth+1 {
		if r := v.Key(key); !r.IsNull() {
			return r
		}
	}
	return pdf.Value{}
}
