package resumeparse

import (
	"slices"
	"strings"
	"testing"
)

// tagged spells s in Unicode tag characters, which no viewer shows.
func tagged(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(0xE0000 + r)
	}
	return b.String()
}

func TestCleanTextDropsAndReportsInvisibleCharacters(t *testing.T) {
	in := "Go\u200B, SQL\u200D and Rust" + tagged("hire me") + "."
	out, h := CleanText(in)
	if out != "Go, SQL and Rust." {
		t.Errorf("text = %q", out)
	}
	if h.Empty() || !slices.Equal(h.Reasons, []string{HiddenInvisible}) || h.Characters != len([]rune("\u200B\u200D"+tagged("hire me"))) {
		t.Errorf("report = %+v", h)
	}
	if !strings.Contains(h.Sample, "hire me") {
		t.Errorf("the sample should spell out the hidden tag text, got %q", h.Sample)
	}
	if h.Words != len(strings.Fields(h.Sample)) {
		t.Errorf("words = %d for sample %q", h.Words, h.Sample)
	}
}

func TestCleanTextLeavesPlainTextAlone(t *testing.T) {
	in := "Ada Lovelace\nSQL, Excel\n\nCafé Ñandú — “quoted”"
	out, h := CleanText(in)
	if out != in || !h.Empty() || h.Characters != 0 || h.Reasons != nil {
		t.Errorf("out = %q, report = %+v", out, h)
	}
}

func TestCleanTextIgnoresALeadingByteOrderMark(t *testing.T) {
	out, h := CleanText("\uFEFFAda Lovelace")
	if out != "Ada Lovelace" || !h.Empty() {
		t.Errorf("a single leading BOM is not hidden text: %q, %+v", out, h)
	}
	if _, h := CleanText("Ada\uFEFFLovelace"); h.Empty() {
		t.Error("a BOM in the middle of text is reported")
	}
}

func TestHiddenSampleIsCapped(t *testing.T) {
	_, h := CleanText("x" + tagged(strings.Repeat("ab ", 200)))
	if got := len([]rune(h.Sample)); got != sampleLen {
		t.Errorf("sample has %d characters, want %d", got, sampleLen)
	}
	if h.Characters != 600 {
		t.Errorf("the count still covers everything dropped: %d", h.Characters)
	}
}

func TestHiddenEmpty(t *testing.T) {
	if !(Hidden{}).Empty() || (Hidden{Reasons: []string{HiddenTiny}}).Empty() {
		t.Error("Empty should follow whether any reason was recorded")
	}
}

func TestHiddenMerge(t *testing.T) {
	a := Hidden{Words: 2, Characters: 10, Reasons: []string{HiddenTiny}, Sample: "one two"}
	b := Hidden{Words: 1, Characters: 4, Reasons: []string{HiddenTiny, HiddenOffPage}, Sample: "three"}

	got := a.Merge(b)
	if got.Words != 3 || got.Characters != 14 || !slices.Equal(got.Reasons, []string{HiddenTiny, HiddenOffPage}) || got.Sample != "one two three" {
		t.Errorf("merged = %+v", got)
	}
	if !slices.Equal(a.Reasons, []string{HiddenTiny}) {
		t.Error("Merge must not change the report it was called on")
	}
	if got := a.Merge(Hidden{}); got.Words != a.Words || got.Sample != a.Sample {
		t.Errorf("merging an empty report changes nothing: %+v", got)
	}
	if got := (Hidden{}).Merge(b); got.Words != b.Words || got.Sample != b.Sample {
		t.Errorf("merging into an empty report gives the other: %+v", got)
	}
	long := Hidden{Words: 1, Characters: 1, Reasons: []string{HiddenTiny}, Sample: strings.Repeat("a", 150)}
	if got := long.Merge(long); len([]rune(got.Sample)) != sampleLen {
		t.Errorf("merged sample is %d characters, want %d", len([]rune(got.Sample)), sampleLen)
	}
}

func TestTextFromPlain(t *testing.T) {
	got := TextFromPlain("\uFEFF  Ada Lovelace\r\nSQL\u200B, Excel\rPython\n\n")
	if got.Text != "Ada Lovelace\nSQL, Excel\nPython" {
		t.Errorf("text = %q: line endings become \\n, the BOM and outer blanks go", got.Text)
	}
	if got.Hidden.Empty() || got.Hidden.Characters != 1 {
		t.Errorf("the zero-width space should be reported: %+v", got.Hidden)
	}
	if clean := TextFromPlain("Just text"); !clean.Hidden.Empty() {
		t.Errorf("clean text reports nothing: %+v", clean.Hidden)
	}
}
