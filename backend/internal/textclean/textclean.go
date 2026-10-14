// Package textclean cleans text that comes from outside the app (job postings, resume imports,
// pasted text) before it is stored or shown.
package textclean

import (
	"strings"
	"unicode"
)

// StripInvisible removes invisible characters from s and returns the result and how many runes
// it removed. Invisible means a Unicode format character (category Cf: zero-width characters,
// bidirectional controls, the byte order mark, soft hyphens) or anything in the tag block
// U+E0000 to U+E007F, including its unassigned code points. Ordinary whitespace and line breaks
// are kept.
func StripInvisible(text string) (string, int) {
	n := 0
	out := strings.Map(func(runes rune) rune {
		if invisible(runes) {
			n++
			return -1
		}
		return runes
	}, text)
	return out, n
}

func invisible(runes rune) bool {
	return unicode.Is(unicode.Cf, r) || r >= 0xE0000 && r <= 0xE007F
}
