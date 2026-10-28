package resumeparse

import (
	"math"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Page.Content() in github.com/ledongthuc/pdf reports where text is but not how it is drawn. This
// walks the content stream with the library's exported interpreter and fonts and also keeps the
// render mode, the fill color and the filled rectangles that the hidden-text rules need.

// One character code drawn on a page.
type text struct {
	Font       string
	FontSize   float64 // horizontal size in points, as pdf.Text reports it
	Size       float64 // rendered height in points
	X, Y, W    float64
	S          string
	RenderMode int
	Fill       [3]float64 // RGB, 0 to 1
	FillKnown  bool       // false for patterns, spot colors and other spaces not converted to RGB
	Seq        int        // drawing order, shared with filledRect
}

// A rectangle painted with a fill, in page space.
type filledRect struct {
	Min, Max  pdf.Point
	Fill      [3]float64
	FillKnown bool
	Seq       int
}

type pageContent struct {
	Text  []text
	Fills []filledRect
}

type matrix [3][3]float64

var ident = matrix{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}

func (x matrix) mul(y matrix) (z matrix) {
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				z[i][j] += x[i][k] * y[k][j]
			}
		}
	}
	return z
}

func (m matrix) apply(x, y float64) (float64, float64) {
	return x*m[0][0] + y*m[1][0] + m[2][0], x*m[0][1] + y*m[1][1] + m[2][1]
}

// What showText needs from a font: its name, how codes decode, how wide each code is.
type fontInfo struct {
	name   string
	enc    pdf.TextEncoding
	cid    bool            // Type0 font: two-byte codes, widths from the descendant font
	widths map[int]float64 // Type0 widths by CID, in 1/1000 em
	dw     float64         // Type0 default width
	font   pdf.Font
}

// Decodes nothing: used when a font has no usable encoding.
type rawEncoding struct{}

func (rawEncoding) Decode(raw string) string { return raw }

func loadFont(p pdf.Page, name string) *fontInfo {
	f := p.Font(name)
	fi := &fontInfo{name: f.BaseFont(), font: f}
	if i := strings.Index(fi.name, "+"); i >= 0 { // subset prefix, "ABCDEF+Inter"
		fi.name = fi.name[i+1:]
	}
	if fi.enc = f.Encoder(); fi.enc == nil {
		fi.enc = rawEncoding{}
	}
	if f.V.Key("Subtype").Name() != "Type0" {
		return fi
	}
	fi.cid, fi.dw, fi.widths = true, 1000, map[int]float64{}
	desc := f.V.Key("DescendantFonts").Index(0)
	if dw := desc.Key("DW"); !dw.IsNull() {
		fi.dw = dw.Float64()
	}
	// W holds "c [w1 w2 ...]" and "cFirst cLast w" entries.
	w := desc.Key("W")
	for i := 0; i < w.Len(); {
		first := int(w.Index(i).Int64())
		if i+1 < w.Len() && w.Index(i+1).Kind() == pdf.Array {
			ws := w.Index(i + 1)
			for j := 0; j < ws.Len(); j++ {
				fi.widths[first+j] = ws.Index(j).Float64()
			}
			i += 2
			continue
		}
		if i+2 >= w.Len() {
			break
		}
		last, width := int(w.Index(i+1).Int64()), w.Index(i+2).Float64()
		for c := first; c <= last && c-first < 1<<16; c++ {
			fi.widths[c] = width
		}
		i += 3
	}
	return fi
}

func (fi *fontInfo) width(code int) float64 {
	if !fi.cid {
		return fi.font.Width(code)
	}
	if w, ok := fi.widths[code]; ok {
		return w
	}
	return fi.dw
}

type gstate struct {
	Tc, Tw, Th, Tl, Tfs, Trise float64
	Tmode                      int
	font                       *fontInfo
	Tm, Tlm, CTM               matrix
	fillSpace                  string
	fill                       [3]float64
	fillKnown                  bool
}

// Reads one page's text and filled rectangles. Form XObjects and inline images are not entered.
func readContent(p pdf.Page) pageContent {
	strm := p.V.Key("Contents")
	if strm.IsNull() {
		return pageContent{}
	}
	var out pageContent
	fonts := map[string]*fontInfo{}
	seq := 0
	next := func() int { seq++; return seq }

	g := gstate{Th: 1, Tm: ident, Tlm: ident, CTM: ident, fillSpace: "DeviceGray", fillKnown: true}
	var stack []gstate
	var path []pdf.Rect // rectangles in the current path, in page space

	showText := func(raw string) {
		fi := g.font
		if fi == nil {
			return
		}
		step := 1
		if fi.cid {
			step = 2
		}
		for i := 0; i+step <= len(raw); i += step {
			code := raw[i : i+step]
			c := int(code[0])
			if step == 2 {
				c = c<<8 | int(code[1])
			}
			w0 := fi.width(c)
			trm := matrix{{g.Tfs * g.Th, 0, 0}, {0, g.Tfs, 0}, {0, g.Trise, 1}}.mul(g.Tm).mul(g.CTM)
			if s := fi.enc.Decode(code); s != "" {
				out.Text = append(out.Text, text{
					Font: fi.name, FontSize: trm[0][0], Size: math.Hypot(trm[1][0], trm[1][1]),
					X: trm[2][0], Y: trm[2][1], W: w0 / 1000 * trm[0][0], S: s,
					RenderMode: g.Tmode, Fill: g.fill, FillKnown: g.fillKnown, Seq: next(),
				})
			}
			tx := w0/1000*g.Tfs + g.Tc
			if step == 1 && code[0] == ' ' {
				tx += g.Tw
			}
			g.Tm = matrix{{1, 0, 0}, {0, 1, 0}, {tx * g.Th, 0, 1}}.mul(g.Tm)
		}
	}
	nextLine := func() {
		g.Tlm = matrix{{1, 0, 0}, {0, 1, 0}, {0, -g.Tl, 1}}.mul(g.Tlm)
		g.Tm = g.Tlm
	}

	pdf.Interpret(strm, func(stk *pdf.Stack, op string) {
		args := make([]pdf.Value, stk.Len())
		for i := len(args) - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}
		num := func(i int) float64 { return args[i].Float64() }
		switch op {
		case "q":
			stack = append(stack, g)
		case "Q":
			if n := len(stack) - 1; n >= 0 {
				g, stack = stack[n], stack[:n]
			}
		case "cm":
			if len(args) == 6 {
				m := matrix{{num(0), num(1), 0}, {num(2), num(3), 0}, {num(4), num(5), 1}}
				g.CTM = m.mul(g.CTM)
			}

		// Fill color. Stroke colors (G, RG, K, CS, SC, SCN) do not color text drawn in fill modes.
		case "g":
			if len(args) == 1 {
				g.fillSpace = "DeviceGray"
				g.fill, g.fillKnown = toRGB(args)
			}
		case "rg":
			if len(args) == 3 {
				g.fillSpace = "DeviceRGB"
				g.fill, g.fillKnown = toRGB(args)
			}
		case "k":
			if len(args) == 4 {
				g.fillSpace = "DeviceCMYK"
				g.fill, g.fillKnown = toRGB(args)
			}
		case "cs":
			if len(args) == 1 {
				g.fillSpace = colorFamily(p, args[0].Name())
				g.fill, g.fillKnown = [3]float64{}, convertible(g.fillSpace) // each space starts at black
			}
		case "sc", "scn":
			g.fill, g.fillKnown = [3]float64{}, false
			if convertible(g.fillSpace) {
				g.fill, g.fillKnown = toRGB(args)
			}

		// Paths: only rectangles are kept, since backgrounds are drawn with them.
		case "re":
			if len(args) == 4 {
				x, y, w, h := num(0), num(1), num(2), num(3)
				r := pdf.Rect{Min: pdf.Point{X: math.Inf(1), Y: math.Inf(1)}, Max: pdf.Point{X: math.Inf(-1), Y: math.Inf(-1)}}
				for _, c := range [][2]float64{{x, y}, {x + w, y}, {x, y + h}, {x + w, y + h}} {
					px, py := g.CTM.apply(c[0], c[1])
					r.Min.X, r.Min.Y = math.Min(r.Min.X, px), math.Min(r.Min.Y, py)
					r.Max.X, r.Max.Y = math.Max(r.Max.X, px), math.Max(r.Max.Y, py)
				}
				path = append(path, r)
			}
		case "f", "F", "f*", "B", "B*", "b", "b*":
			for _, r := range path {
				out.Fills = append(out.Fills, filledRect{Min: r.Min, Max: r.Max, Fill: g.fill, FillKnown: g.fillKnown, Seq: next()})
			}
			path = nil
		case "n", "S", "s":
			path = nil

		case "BT":
			g.Tm, g.Tlm = ident, ident
		case "Tc":
			if len(args) == 1 {
				g.Tc = num(0)
			}
		case "Tw":
			if len(args) == 1 {
				g.Tw = num(0)
			}
		case "Tz":
			if len(args) == 1 {
				g.Th = num(0) / 100
			}
		case "TL":
			if len(args) == 1 {
				g.Tl = num(0)
			}
		case "Ts":
			if len(args) == 1 {
				g.Trise = num(0)
			}
		case "Tr":
			if len(args) == 1 {
				g.Tmode = int(args[0].Int64())
			}
		case "Tf":
			if len(args) == 2 {
				name := args[0].Name()
				if fonts[name] == nil {
					fonts[name] = loadFont(p, name)
				}
				g.font, g.Tfs = fonts[name], num(1)
			}
		case "Td", "TD":
			if len(args) == 2 {
				if op == "TD" {
					g.Tl = -num(1)
				}
				g.Tlm = matrix{{1, 0, 0}, {0, 1, 0}, {num(0), num(1), 1}}.mul(g.Tlm)
				g.Tm = g.Tlm
			}
		case "Tm":
			if len(args) == 6 {
				g.Tlm = matrix{{num(0), num(1), 0}, {num(2), num(3), 0}, {num(4), num(5), 1}}
				g.Tm = g.Tlm
			}
		case "T*":
			nextLine()
		case "Tj":
			if len(args) == 1 {
				showText(args[0].RawString())
			}
		case "'":
			if len(args) == 1 {
				nextLine()
				showText(args[0].RawString())
			}
		case "\"":
			if len(args) == 3 {
				g.Tw, g.Tc = num(0), num(1)
				nextLine()
				showText(args[2].RawString())
			}
		case "TJ":
			if len(args) == 1 {
				v := args[0]
				for i := 0; i < v.Len(); i++ {
					x := v.Index(i)
					if x.Kind() == pdf.String {
						showText(x.RawString())
						continue
					}
					tx := -x.Float64() / 1000 * g.Tfs * g.Th
					g.Tm = matrix{{1, 0, 0}, {0, 1, 0}, {tx, 0, 1}}.mul(g.Tm)
				}
			}
		}
	})
	return out
}

// The family of a fill color space: a device space by name, or the first entry of a named
// space in the page resources ("ICCBased", "Separation", "Pattern").
func colorFamily(p pdf.Page, name string) string {
	switch name {
	case "DeviceGray", "DeviceRGB", "DeviceCMYK", "Pattern":
		return name
	}
	cs := p.Resources().Key("ColorSpace").Key(name)
	switch cs.Kind() {
	case pdf.Array:
		return cs.Index(0).Name()
	case pdf.Name:
		return cs.Name()
	}
	return ""
}

// Spaces whose components convert to RGB by count: 1 gray, 3 RGB, 4 CMYK.
func convertible(family string) bool {
	switch family {
	case "DeviceGray", "DeviceRGB", "DeviceCMYK", "ICCBased", "CalGray", "CalRGB":
		return true
	}
	return false
}

func toRGB(args []pdf.Value) (rgb [3]float64, known bool) {
	var c []float64
	for _, a := range args {
		if a.Kind() != pdf.Integer && a.Kind() != pdf.Real {
			return rgb, false // a pattern name
		}
		c = append(c, math.Min(1, math.Max(0, a.Float64())))
	}
	switch len(c) {
	case 1:
		return [3]float64{c[0], c[0], c[0]}, true
	case 3:
		return [3]float64{c[0], c[1], c[2]}, true
	case 4:
		k := 1 - c[3]
		return [3]float64{(1 - c[0]) * k, (1 - c[1]) * k, (1 - c[2]) * k}, true
	}
	return rgb, false
}
