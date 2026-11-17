package diff

import (
	"strings"
	"testing"
)

// rebuild joins the runs that belong to one side of a diff.
func rebuild(words []Word, drop string) string {
	var sb strings.Builder
	for _, w := range words {
		if w.Op != drop {
			sb.WriteString(w.Text)
		}
	}
	return sb.String()
}

func TestWordsRebuildBothTexts(t *testing.T) {
	cases := [][2]string{
		{"excel, python, sql", "sql, excel, python"},
		{"a b c", "a x c"},
		{"", "new text"},
		{"old text", ""},
		{"same  spacing\nand lines", "same  spacing\nand lines"},
		{"", ""},
	}
	for _, c := range cases {
		words := Words(c[0], c[1])
		if got := rebuild(words, "add"); got != c[0] {
			t.Errorf("Words(%q, %q): base rebuilt as %q", c[0], c[1], got)
		}
		if got := rebuild(words, "del"); got != c[1] {
			t.Errorf("Words(%q, %q): tailored rebuilt as %q", c[0], c[1], got)
		}
	}
}

func TestWordsMarksOnlyWhatChanged(t *testing.T) {
	got := Words("a b c", "a x c")
	want := []Word{{"eq", "a "}, {"del", "b"}, {"add", "x"}, {"eq", " c"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("word %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestWordsTooLargeFallsBack(t *testing.T) {
	big := strings.Repeat("word ", 3000)
	words := Words(big, big+"x")
	if len(words) != 2 || words[0].Op != "del" || words[1].Op != "add" {
		t.Errorf("want one del and one add, got %d runs", len(words))
	}
}

const base = `# Ada

## Summary

Analyst.

## Experience

### Analyst, Acme

Built dashboards.

### Intern, Globex

Filed reports.

## Skills

excel, sql
`

func hunkByHeading(hunks []Hunk, heading string) *Hunk {
	for i := range hunks {
		if hunks[i].Heading == heading {
			return &hunks[i]
		}
	}
	return nil
}

func TestHunksUnchangedAndChanged(t *testing.T) {
	tailored := strings.Replace(base, "excel, sql", "sql, excel", 1)
	hunks := Hunks(base, tailored)

	if h := hunkByHeading(hunks, "## Skills"); h == nil || h.Status != "changed" || h.Section != "skills" {
		t.Errorf("skills hunk = %+v", h)
	}
	if h := hunkByHeading(hunks, "### Analyst, Acme"); h == nil || h.Status != "unchanged" || h.Section != "experience" {
		t.Errorf("experience hunk = %+v", h)
	}
	if h := hunkByHeading(hunks, "## Experience"); h != nil {
		t.Errorf("a heading with no text of its own should be left out, got %+v", h)
	}
}

func TestHunksAddedAndRemoved(t *testing.T) {
	tailored := strings.Replace(base, "### Intern, Globex\n\nFiled reports.\n\n", "", 1)
	tailored = strings.Replace(tailored, "## Skills", "## Certifications\n\nSecurity+\n\n## Skills", 1)
	hunks := Hunks(base, tailored)

	if h := hunkByHeading(hunks, "### Intern, Globex"); h == nil || h.Status != "removed" || h.After != "" {
		t.Errorf("removed hunk = %+v", h)
	}
	if h := hunkByHeading(hunks, "## Certifications"); h == nil || h.Status != "added" || h.Before != "" {
		t.Errorf("added hunk = %+v", h)
	}
}

func TestHunksNewSummary(t *testing.T) {
	without := strings.Replace(base, "## Summary\n\nAnalyst.\n\n", "", 1)
	h := hunkByHeading(Hunks(without, base), "## Summary")
	if h == nil || h.Status != "added" || h.After != "Analyst." {
		t.Errorf("summary hunk = %+v", h)
	}
}

func TestHunksKeepBaseOrder(t *testing.T) {
	var headings []string
	for _, h := range Hunks(base, base) {
		headings = append(headings, h.Heading)
	}
	want := "|## Summary|### Analyst, Acme|### Intern, Globex|## Skills"
	if got := strings.Join(headings, "|"); got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
}
