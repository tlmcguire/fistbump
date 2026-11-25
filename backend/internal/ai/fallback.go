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
		var proposed, reason string
		var ok bool
		switch t.Section {
		case "skills":
			var moved []string
			proposed, moved, ok = reorderSkills(t.Original, skills)
			if len(moved) > 0 {
				reason = reasonFor(r.Language, "skillsNamed", strings.Join(moved[:min(len(moved), 4)], ", "))
			} else {
				reason = reasonFor(r.Language, "skills")
			}
		case "experience":
			var cited []string
			proposed, cited, ok = reorderLines(t.Original, skills)
			if len(cited) > 0 {
				reason = reasonFor(r.Language, "linesNamed", strings.Join(cited[:min(len(cited), 4)], ", "))
			} else {
				reason = reasonFor(r.Language, "lines")
			}
		case "summary":
			proposed, ok = summaryText(t.Original, r, skills)
			list := strings.Join(skills[:min(len(skills), 5)], ", ")
			if strings.TrimSpace(t.Original) == "" {
				reason = reasonFor(r.Language, "newsummary", list)
			} else {
				reason = reasonFor(r.Language, "summary", list)
			}
		}
		if !ok || proposed == t.Original || normalizeTypography(plainText(proposed)) == normalizeTypography(plainText(t.Original)) {
			continue
		}
		out = append(out, Draft{
			Section:  t.Section,
			TargetID: t.TargetID,
			Original: t.Original,
			Proposed: proposed,
			Reason:   reason,
			Engine:   EngineRules,
		})
	}
	return out, nil
}

// matchedSkills lists the job skills the resume shows without repeats: tech skills first, required
// before preferred, then soft skills. The summary names the first five, so tools come before traits.
func matchedSkills(a analyze.Result) []string {
	var out []string
	seen := map[string]bool{}
	for _, g := range []analyze.Group{a.Required.Tech, a.Preferred.Tech, a.Required.Soft, a.Preferred.Soft} {
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
// lists them. It returns the matched skills that were prioritized and reports false when the order
// would not change.
func reorderSkills(line string, matched []string) (string, []string, bool) {
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
		return "", nil, false
	}
	return ordered, first, true
}

// reorderLines moves lines that mention a matched skill to the front, keeping their order.
// It returns the matched skills cited in the promoted lines and reports false when the order
// would not change. Text of a single line is left alone.
func reorderLines(text string, matched []string) (string, []string, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return "", nil, false
	}
	var first, rest, cited []string
	seen := map[string]bool{}
	for _, line := range lines {
		if hits := matchedInLine(line, matched); len(hits) > 0 {
			first = append(first, line)
			for _, h := range hits {
				k := strings.ToLower(h)
				if !seen[k] {
					seen[k] = true
					cited = append(cited, h)
				}
			}
		} else {
			rest = append(rest, line)
		}
	}
	ordered := strings.Join(append(first, rest...), "\n")
	if ordered == text {
		return "", nil, false
	}
	return ordered, cited, true
}

func matchedInLine(line string, matched []string) []string {
	low := strings.ToLower(line)
	var hits []string
	for _, m := range matched {
		if strings.Contains(low, strings.ToLower(m)) {
			hits = append(hits, m)
		}
	}
	return hits
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
