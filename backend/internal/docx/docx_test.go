package docx

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRenderStyledProducesValidDOCX(t *testing.T) {
	md := "# Ada Lovelace\n\nLondon · ada@example.com\n\n## Experience\n\n### Engineer, Analytical Engine\n\nJan 2022 to Present\n\n- Built numerical algorithms reducing latency by 40%.\n"
	for _, style := range []string{"classic", "modern", "minimal", "compact", "executive"} {
		data, err := RenderStyled(md, style)
		if err != nil {
			t.Fatalf("RenderStyled(%q) failed: %v", style, err)
		}
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("invalid zip for %q: %v", style, err)
		}
		var docXML string
		for _, f := range zr.File {
			if f.Name == "word/document.xml" {
				rc, err := f.Open()
				if err != nil {
					t.Fatalf("open document.xml: %v", err)
				}
				b, _ := io.ReadAll(rc)
				rc.Close()
				docXML = string(b)
			}
		}
		if !strings.Contains(docXML, "Ada Lovelace") || !strings.Contains(docXML, "reducing latency by 40%") {
			t.Fatalf("expected resume text in document.xml for %q, got: %s", style, docXML)
		}
	}
}
