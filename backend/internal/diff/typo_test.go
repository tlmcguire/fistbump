package diff

import "testing"

func TestWordsIgnoresCurlyApostropheEquivalents(t *testing.T) {
	words := Words("Earned a Bachelor\u2019s degree in CS.", "Earned a Bachelor's degree in CS.")
	for _, w := range words {
		if w.Op != "eq" {
			t.Fatalf("expected only eq ops for typographic apostrophe match, got %+v", words)
		}
	}
}
