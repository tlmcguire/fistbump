package pdf

import (
	"bytes"
	"testing"
)

func TestAllPDFStylesRenderValidAndPassSafetyChecks(t *testing.T) {
	compress = false
	defer func() { compress = true }()

	styles := Styles()
	if len(styles) < 5 {
		t.Fatalf("expected at least 5 PDF styles, got %d", len(styles))
	}

	classicOut, err := RenderStyled(sample, "resume.pdf", StyleClassic)
	if err != nil {
		t.Fatalf("classic render failed: %v", err)
	}

	for _, st := range styles {
		t.Run(st.ID, func(t *testing.T) {
			if !ValidStyle(st.ID) {
				t.Errorf("ValidStyle(%q) = false, want true", st.ID)
			}
			out, err := RenderStyled(sample, "resume.pdf", st.ID)
			if err != nil {
				t.Fatalf("RenderStyled(%q) failed: %v", st.ID, err)
			}
			r := open(t, out)
			if r.NumPage() < 1 {
				t.Fatalf("style %q produced 0 pages", st.ID)
			}
			if st.ID != StyleClassic && bytes.Equal(out, classicOut) {
				t.Errorf("style %q produced identical bytes to classic", st.ID)
			}
			for p := 1; p <= r.NumPage(); p++ {
				s := pageStream(t, r.Page(p))
				for _, m := range rgR.FindAllStringSubmatch(s, -1) {
					for _, c := range m[1:] {
						if num(t, c) > 0.5 {
							t.Errorf("style %q page %d: light color %q", st.ID, p, m[0])
						}
					}
				}
				for _, m := range tfR.FindAllStringSubmatch(s, -1) {
					if num(t, m[1]) < MinFontSize {
						t.Errorf("style %q page %d: font size %s below MinFontSize", st.ID, p, m[1])
					}
				}
				if m := fillR.FindString(s); m != "" {
					t.Errorf("style %q page %d: unexpected fill operator %q", st.ID, p, m)
				}
			}
		})
	}

	if ValidStyle("nonexistent-style") {
		t.Errorf("ValidStyle(nonexistent-style) = true, want false")
	}
}
