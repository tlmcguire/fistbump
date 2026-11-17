package ai

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/analyze"
)

// Rules is the engine that always works. It only reorders text the resume already has and adds
// one summary sentence built from skills the job and the resume share. It never invents content.
type Rules struct{}

func (Rules) Name() string { return EngineRules }
func (Rules) Ready() bool  { return true }

// Suggest proposes a change for each target it can improve. It does no I/O.
func (Rules) Suggest(ctx context.Context, r Request) ([]Draft, error) {
	skills := matchedSkills(r.Analysis)
	var out []Draft
	for _, t := range r.Targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var proposed string
		var ok bool
		switch t.Section {
		case "skills":
			proposed, ok = reorderSkills(t.Original, skills)
		case "experience":
			proposed, ok = reorderLines(t.Original, skills)
		case "summary":
			proposed, ok = summaryText(t.Original, r, skills)
		}
		if !ok || proposed == t.Original {
			continue
		}
		out = append(out, Draft{
			Section:  t.Section,
			TargetID: t.TargetID,
			Original: t.Original,
			Proposed: proposed,
			Engine:   EngineRules,
		})
	}
	return out, nil
}

// matchedSkills lists the job skills the resume shows, required before preferred, without repeats.
func matchedSkills(a analyze.Result) []string {
	var out []string
	seen := map[string]bool{}
	for _, g := range []analyze.Group{a.Required.Tech, a.Required.Soft, a.Preferred.Tech, a.Preferred.Soft} {
		for _, m := range g.Matched {
			key := strings.ToLower(m.Skill)
			if !seen[key] {
				seen[key] = true
				out = append(out, m.Skill)
			}
		}
	}
	return out
}

// reorderSkills moves matched skills to the front of a comma-separated line, in the order the job
// lists them. It reports false when the order would not change.
func reorderSkills(line string, matched []string) (string, bool) {
	items := splitList(line, ",")
	rank := map[string]int{}
	for i, m := range matched {
		rank[strings.ToLower(m)] = i
	}
	var first, rest []string
	for _, item := range items {
		if _, ok := rank[strings.ToLower(item)]; ok {
			first = append(first, item)
		} else {
			rest = append(rest, item)
		}
	}
	sort.SliceStable(first, func(i, j int) bool {
		return rank[strings.ToLower(first[i])] < rank[strings.ToLower(first[j])]
	})
	ordered := strings.Join(append(first, rest...), ", ")
	if ordered == strings.Join(items, ", ") {
		return "", false
	}
	return ordered, true
}

// reorderLines moves lines that mention a matched skill to the front, keeping their order.
// Text of a single line is left alone.
func reorderLines(text string, matched []string) (string, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return "", false
	}
	var first, rest []string
	for _, line := range lines {
		if mentionsSkill(line, matched) {
			first = append(first, line)
		} else {
			rest = append(rest, line)
		}
	}
	return strings.Join(append(first, rest...), "\n"), true
}

// summaryText adds up to five matched skills to the summary, or writes a short one when there is
// none. It reports false when no skill matched.
func summaryText(original string, r Request, matched []string) (string, bool) {
	if len(matched) == 0 {
		return "", false
	}
	top := matched[:min(len(matched), 5)]
	list := strings.Join(top, ", ")
	if strings.TrimSpace(original) != "" {
		return strings.TrimSpace(original) + " Relevant strengths for this role: " + list + ".", true
	}
	position := r.Job.PositionTitle
	if r.Profile != nil && len(r.Profile.TargetPositions) > 0 {
		position = r.Profile.TargetPositions[0]
	}
	return fmt.Sprintf("%s with hands-on experience in %s.", position, list), true
}

func mentionsSkill(line string, matched []string) bool {
	low := strings.ToLower(line)
	for _, m := range matched {
		if strings.Contains(low, strings.ToLower(m)) {
			return true
		}
	}
	return false
}

// splitList splits s on sep, trims each item and drops empty ones.
func splitList(s, sep string) []string {
	var out []string
	for _, part := range strings.Split(s, sep) {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
