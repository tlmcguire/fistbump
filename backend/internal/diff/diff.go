// Package diff builds the line and word hunks shown in the diff view.
package diff

import "strings"

type Word struct {
	Op   string `json:"op"` // eq, add, del
	Text string `json:"text"`
}

type Hunk struct {
	Section  string `json:"section"`
	TargetID *int64 `json:"target_id"`
	Status   string `json:"status"` // unchanged, changed, added, removed
	Before   string `json:"before"`
	After    string `json:"after"`
	Words    []Word `json:"words"`
}

// Hint ties changed text back to the suggestion that produced it.
type Hint struct {
	Section  string
	TargetID *int64
	Original string
	Final    string
}

type op struct {
	kind string // eq, add, del
	text string
}

// lcsDiff compares two token slices and returns a minimal edit script.
func lcsDiff(a, b []string) []op {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var out []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, op{"eq", a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			out = append(out, op{"del", a[i]})
			i++
		default:
			out = append(out, op{"add", b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, op{"del", a[i]})
	}
	for ; j < m; j++ {
		out = append(out, op{"add", b[j]})
	}
	return out
}

// tokens splits on spaces and keeps each newline as its own token, so multi-line text keeps its lines.
func tokens(s string) []string {
	return strings.FieldsFunc(strings.ReplaceAll(s, "\n", " \n "), func(r rune) bool { return r == ' ' || r == '\t' })
}

// Words returns a word-level diff of two strings, merging adjacent ops of the same kind.
func Words(before, after string) []Word {
	var out []Word
	for _, o := range lcsDiff(tokens(before), tokens(after)) {
		if n := len(out); n > 0 && out[n-1].Op == o.kind {
			if o.text == "\n" || strings.HasSuffix(out[n-1].Text, "\n") {
				out[n-1].Text += o.text
			} else {
				out[n-1].Text += " " + o.text
			}
		} else {
			out = append(out, Word{Op: o.kind, Text: o.text})
		}
	}
	return out
}

// Hunks diffs base against tailored line by line. Consecutive changed lines form one hunk; unchanged
// runs are reported once as "unchanged" so the view can collapse them. Section comes from the nearest
// preceding Markdown "## " heading, and hints refine it with the suggestion's section and target id.
func Hunks(base, tailored string, hints []Hint) []Hunk {
	ops := lcsDiff(strings.Split(base, "\n"), strings.Split(tailored, "\n"))
	var hunks []Hunk
	section := "header"
	var eq, del, add []string

	flushEq := func() {
		if len(eq) > 0 {
			t := strings.Join(eq, "\n")
			hunks = append(hunks, Hunk{Section: section, Status: "unchanged", Before: t, After: t, Words: []Word{}})
			eq = nil
		}
	}
	flushChange := func() {
		if len(del) == 0 && len(add) == 0 {
			return
		}
		before, after := strings.Join(del, "\n"), strings.Join(add, "\n")
		h := Hunk{Section: section, Before: before, After: after}
		switch {
		case before == "":
			h.Status = "added"
		case after == "":
			h.Status = "removed"
		default:
			h.Status = "changed"
		}
		h.Words = Words(before, after)
		for _, hint := range hints {
			if hint.Original != "" && strings.Contains(before, hint.Original) || hint.Original == "" && strings.Contains(after, hint.Final) {
				h.Section, h.TargetID = hint.Section, hint.TargetID
				break
			}
		}
		hunks = append(hunks, h)
		del, add = nil, nil
	}
	for _, o := range ops {
		switch o.kind {
		case "eq":
			flushChange()
			if strings.HasPrefix(o.text, "## ") {
				section = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(o.text, "## ")))
			}
			eq = append(eq, o.text)
		case "del":
			flushEq()
			del = append(del, o.text)
		case "add":
			flushEq()
			add = append(add, o.text)
		}
		if o.kind != "eq" && strings.HasPrefix(o.text, "## ") {
			section = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(o.text, "## ")))
		}
	}
	flushChange()
	flushEq()
	if hunks == nil {
		hunks = []Hunk{}
	}
	return hunks
}
