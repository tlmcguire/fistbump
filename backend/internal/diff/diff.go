// Package diff compares a base resume with its tailored version, by section and by word, for the
// side-by-side review.
package diff

import (
	"regexp"
	"strings"
)

// Word is a run of text that is in both versions (eq), only the tailored one (add) or only the
// base (del). Joining the eq and del runs rebuilds the base; the eq and add runs, the tailored text.
type Word struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

// Hunk is one heading and its text, compared across the two versions.
type Hunk struct {
	Section  string `json:"section"`   // the enclosing "## " heading in lower case; "header" before the first
	Heading  string `json:"heading"`   // the heading line itself, such as "### Data Analyst, Acme"
	TargetID *int64 `json:"target_id"` // left nil here; the caller fills it for experience headings
	Status   string `json:"status"`    // unchanged, changed, added or removed
	Before   string `json:"before"`
	After    string `json:"after"`
	Words    []Word `json:"words"`
}

// maxCells bounds the comparison table. A larger pair of blocks is shown as one deletion and one
// addition instead of using memory proportional to the product of their lengths.
const maxCells = 4_000_000

// tokens splits text into words and the whitespace between them, so nothing is lost.
var tokens = regexp.MustCompile(`\s+|\S+`)

// Words compares two texts word by word using the longest common subsequence of their tokens.
func Words(before, after string) []Word {
	a, b := tokens.FindAllString(before, -1), tokens.FindAllString(after, -1)
	if len(a)*len(b) > maxCells {
		return join([]Word{{"del", before}, {"add", after}})
	}

	// lcs[i][j] is the length of the longest common subsequence of a[i:] and b[j:].
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	var out []Word
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, Word{"eq", a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, Word{"del", a[i]})
			i++
		default:
			out = append(out, Word{"add", b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		out = append(out, Word{"del", a[i]})
	}
	for ; j < len(b); j++ {
		out = append(out, Word{"add", b[j]})
	}
	return join(out)
}

// join merges neighbouring runs that share an op and drops empty ones.
func join(words []Word) []Word {
	out := []Word{}
	for _, w := range words {
		if w.Text == "" {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Op == w.Op {
			out[n-1].Text += w.Text
		} else {
			out = append(out, w)
		}
	}
	return out
}

// block is a heading and the text under it, up to the next "## " or "### " heading.
type block struct {
	section string
	heading string
	body    string
}

// blocks splits a resume at its "## " and "### " headings. Text before the first heading is a
// block with no heading, in the "header" section.
func blocks(text string) []block {
	out := []block{{section: "header"}}
	var body []string
	flush := func() {
		out[len(out)-1].body = strings.TrimSpace(strings.Join(body, "\n"))
		body = nil
	}
	section := "header"
	for _, line := range strings.Split(text, "\n") {
		isSection := strings.HasPrefix(line, "## ")
		if isSection || strings.HasPrefix(line, "### ") {
			flush()
			if isSection {
				section = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			}
			out = append(out, block{section: section, heading: line})
			continue
		}
		body = append(body, line)
	}
	flush()
	return out
}

// Hunks pairs the blocks of the two texts by heading and compares each pair. The result follows
// the base's order, with blocks only in the tailored text placed before the next shared block.
// Blocks with no text on either side, such as a "## Experience" heading with only subheadings
// under it, are left out.
func Hunks(base, tailored string) []Hunk {
	bs, ts := blocks(base), blocks(tailored)

	// match[i] is the tailored block paired with base block i, or -1.
	match := make([]int, len(bs))
	used := make([]bool, len(ts))
	for i, b := range bs {
		match[i] = -1
		for j, t := range ts {
			if !used[j] && t.heading == b.heading {
				match[i], used[j] = j, true
				break
			}
		}
	}

	out := []Hunk{}
	add := func(base, tailored *block) {
		if h := compare(base, tailored); h.Before != "" || h.After != "" {
			out = append(out, h)
		}
	}
	emitted := 0 // tailored blocks before this index have been written
	for i := range bs {
		j := match[i]
		if j < 0 {
			add(&bs[i], nil)
			continue
		}
		for ; emitted < j; emitted++ {
			if !used[emitted] {
				add(nil, &ts[emitted])
			}
		}
		add(&bs[i], &ts[j])
		emitted = j + 1
	}
	for ; emitted < len(ts); emitted++ {
		if !used[emitted] {
			add(nil, &ts[emitted])
		}
	}
	return out
}

// compare builds the hunk for one pair. A nil block means the heading is absent from that side.
func compare(base, tailored *block) Hunk {
	h := Hunk{}
	ref := tailored
	if base != nil {
		h.Before, ref = base.body, base
	}
	if tailored != nil {
		h.After = tailored.body
	}
	h.Section, h.Heading = ref.section, ref.heading
	switch {
	case base == nil:
		h.Status = "added"
	case tailored == nil:
		h.Status = "removed"
	case h.Before == h.After:
		h.Status = "unchanged"
	default:
		h.Status = "changed"
	}
	h.Words = Words(h.Before, h.After)
	return h
}
