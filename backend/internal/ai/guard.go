package ai

import (
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/parser"
)

// Guards that keep model output from claiming skills the user does not have. Every model proposal
// passes through these before it is stored. A proposal that fails is dropped, never repaired, and the
// rules engine fills the item instead.

// SkillSet is the canonical set of skills the resume shows: listed skills plus skills found in its text.
type SkillSet map[string]bool

// ResumeSkills builds the set from free text (resume sections) and explicit skill lists.
func ResumeSkills(texts []string, lists ...[]string) SkillSet {
	s := SkillSet{}
	for _, t := range texts {
		tech, soft := parser.FindSkills(t)
		for _, k := range append(tech, soft...) {
			s[k] = true
		}
	}
	for _, l := range lists {
		for _, k := range l {
			s[parser.Canon(k)] = true
		}
	}
	return s
}

// UnsupportedSkills returns skills named in text that the set does not contain.
func (s SkillSet) UnsupportedSkills(text string) []string {
	tech, soft := parser.FindSkills(text)
	var out []string
	for _, k := range append(tech, soft...) {
		if !s[k] {
			out = append(out, k)
		}
	}
	return out
}

// sameItems reports whether two comma-separated lists hold the same items, ignoring order and case.
// The skills line may be reordered, never extended or trimmed.
func sameItems(a, b string) bool {
	set := func(s string) map[string]int {
		m := map[string]int{}
		for _, p := range strings.Split(s, ",") {
			if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
				m[p]++
			}
		}
		return m
	}
	x, y := set(a), set(b)
	if len(x) != len(y) {
		return false
	}
	for k, n := range x {
		if y[k] != n {
			return false
		}
	}
	return true
}

// requestSkills is what the user has, for checking proposals: every target's text, the matched skills
// from analysis, and the profile's listed skills and certifications. Missing job skills are excluded.
func requestSkills(r Request) SkillSet {
	var texts []string
	for _, t := range r.Targets {
		texts = append(texts, t.Original)
	}
	lists := [][]string{r.Analysis.Matched()}
	if r.Profile != nil {
		lists = append(lists, r.Profile.Skills, r.Profile.ClearanceCerts)
	}
	return ResumeSkills(texts, lists...)
}

// checkDraft applies the skill guards to one proposal for a target. It returns a reason when the
// proposal must be dropped.
func checkDraft(t Target, proposed string, have SkillSet) string {
	if t.Section == "skills" && !sameItems(t.Original, proposed) {
		return "skills line changed instead of reordered"
	}
	if bad := have.UnsupportedSkills(proposed); len(bad) > 0 {
		return "claims skills not on the resume: " + strings.Join(bad, ", ")
	}
	return ""
}
