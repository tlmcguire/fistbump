package ai

import (
	"sort"
	"strings"
	"unicode"

	"github.com/tlmcguire/fistbump/backend/internal/parser"
)

// minGrounding is the share of a proposal's content words that must appear in the resume.
const minGrounding = 0.75

// Check returns why a model proposal must be dropped, or "" when it may reach the review. A
// dropped proposal is never repaired; the rules engine fills the item instead.
func Check(t Target, proposed string, r Request) string {
	if strings.TrimSpace(proposed) == strings.TrimSpace(t.Original) {
		return "no change"
	}
	switch t.Section {
	case "skills":
		if !sameItems(t.Original, proposed) {
			return "skills line changed its items"
		}
		return "" // a reorder of the same items adds nothing
	case "summary":
		if sentences(plainText(proposed)) > 3 {
			return "summary longer than 3 sentences"
		}
	case "experience":
		if lineCount(proposed) > lineCount(t.Original) {
			return "more lines than the original"
		}
	}
	if skill := newSkill(proposed, t, r); skill != "" {
		return "adds a skill the resume does not show"
	}
	if grounding(proposed, t, r) < minGrounding {
		return "too many words not found in the resume"
	}
	return ""
}

// sameItems compares two comma-separated lists, ignoring order, case and spacing.
func sameItems(a, b string) bool {
	norm := func(s string) []string {
		items := splitList(s, ",")
		for i := range items {
			items[i] = strings.ToLower(items[i])
		}
		sort.Strings(items)
		return items
	}
	x, y := norm(a), norm(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func lineCount(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// sentences counts sentence ends. A period inside a word or number ("e.g", "3.5", "Node.js") is
// not one: it must be followed by a space or the end of the text.
func sentences(s string) int {
	n := 0
	rs := []rune(strings.TrimSpace(s))
	for i, c := range rs {
		if (c == '.' || c == '!' || c == '?') && (i == len(rs)-1 || unicode.IsSpace(rs[i+1])) {
			n++
		}
	}
	if n == 0 && len(rs) > 0 {
		n = 1
	}
	return n
}

// allowedSkills is every skill the resume shows: what the parser finds in its text, its listed
// skills and certifications, and the job skills the analysis matched.
func allowedSkills(t Target, r Request) map[string]bool {
	have := map[string]bool{}
	add := func(s string) { have[parser.Canon(s)] = true }
	tech, soft := parser.FindSkills(r.Resume + "\n" + plainText(t.Original))
	for _, s := range append(tech, soft...) {
		add(s)
	}
	if r.Profile != nil {
		for _, s := range r.Profile.Skills {
			add(s)
		}
		for _, s := range r.Profile.ClearanceCerts {
			add(s)
		}
	}
	for _, s := range matchedSkills(r.Analysis) {
		add(s)
	}
	return have
}

// newSkill returns the first skill in the proposal that the resume does not show.
func newSkill(proposed string, t Target, r Request) string {
	have := allowedSkills(t, r)
	tech, soft := parser.FindSkills(plainText(proposed))
	for _, s := range append(tech, soft...) {
		if !have[parser.Canon(s)] {
			return s
		}
	}
	return ""
}

// grounding is the share of the proposal's content words that appear in the resume text, the
// original, or the matched skills. Words are compared lower case with a light suffix strip, so
// "managed" matches "manage" and "teams" matches "team".
func grounding(proposed string, t Target, r Request) float64 {
	vocab := map[string]bool{}
	for _, w := range contentWords(r.Resume + "\n" + plainText(t.Original) + "\n" + strings.Join(matchedSkills(r.Analysis), " ")) {
		vocab[w] = true
	}
	words := contentWords(plainText(proposed))
	if len(words) == 0 {
		return 1
	}
	found := 0
	for _, w := range words {
		if vocab[w] {
			found++
		}
	}
	return float64(found) / float64(len(words))
}

func contentWords(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '#'
	})
	var out []string
	for _, f := range fields {
		if len(f) < 2 || stopWords[f] {
			continue
		}
		out = append(out, stem(f))
	}
	return out
}

func stem(w string) string {
	for _, suffix := range []string{"ing", "ed", "es", "s", "ly"} {
		if len(w) > len(suffix)+3 && strings.HasSuffix(w, suffix) {
			return strings.TrimSuffix(w, suffix)
		}
	}
	return w
}

var stopWords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`a an the and or but nor so yet for of in on at to by with from into onto
		over under about across after before during through within without between among per via
		is are was were be been being am do does did done have has had having will would shall should
		can could may might must this that these those it its it's i me my we our us you your he she
		they them their his her as than then also too very more most less least such each every all
		any both either neither some other another own same not no only just which who whom whose
		what when where while how why if because until up down out off again further once here there
		including include includes using use used well new`) {
		m[w] = true
	}
	return m
}()
