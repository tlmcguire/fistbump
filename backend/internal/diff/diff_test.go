package diff

import (
	"strings"
	"testing"
)

func TestWords(t *testing.T) {
	w := Words("built the api", "built a fast api")
	var adds, dels int
	for _, x := range w {
		switch x.Op {
		case "add":
			adds++
		case "del":
			dels++
		}
	}
	if adds == 0 || dels == 0 {
		t.Fatalf("words = %+v", w)
	}
}

func TestHunks(t *testing.T) {
	base := "# Ann\n\n## Experience\n\n### Dev, Acme\n\nWrote code.\n\n## Skills\n\ngo, sql\n"
	tail := "# Ann\n\n## Summary\n\nEngineer.\n\n## Experience\n\n### Dev, Acme\n\nWrote better code.\n\n## Skills\n\nsql, go\n"
	id := int64(7)
	hunks := Hunks(base, tail, []Hint{{Section: "experience", TargetID: &id, Original: "Wrote code.", Final: "Wrote better code."}})
	var statuses []string
	var hinted bool
	for _, h := range hunks {
		statuses = append(statuses, h.Status)
		if h.TargetID != nil && *h.TargetID == 7 && h.Status == "changed" {
			hinted = true
		}
	}
	if !hinted {
		t.Fatalf("experience hunk not linked to its suggestion: %+v", hunks)
	}
	if len(statuses) < 4 {
		t.Fatalf("statuses = %v", statuses)
	}
	if got := Hunks("a\nb", "a\nb", nil); len(got) != 1 || got[0].Status != "unchanged" {
		t.Fatalf("identical input: %+v", got)
	}
}

func TestWordsKeepLines(t *testing.T) {
	w := Words("- a\n- b", "- a\n- c")
	joined := ""
	for _, x := range w {
		joined += x.Op + ":" + x.Text + "|"
	}
	if !strings.Contains(joined, "\n") {
		t.Fatalf("newline lost: %q", joined)
	}
}
