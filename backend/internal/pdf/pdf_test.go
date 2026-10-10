package pdf

import (
	"bytes"
	"testing"
)

func TestRender(t *testing.T) {
	out, err := Render("# Ann Lee\n\nann@x.com | Austin\n\n## Experience\n\n### Dev, Acme\n\n2020 – Present\n\n- Built “things”\n")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) || len(out) < 500 {
		t.Fatalf("not a pdf: %d bytes", len(out))
	}
}
