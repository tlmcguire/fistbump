package ai

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/parser"
)

// Rules is the always-available template engine. It reorders and highlights what the resume already
// states and never adds skills the user does not have.
type Rules struct{}

func (Rules) Name() string { return "rules" }

func (Rules) Suggest(_ context.Context, r Request) ([]Draft, error) {
	matched := r.Analysis.Matched()
	var out []Draft
	for _, t := range r.Targets {
		switch t.Section {
		case "summary":
			if len(matched) == 0 {
				continue
			}
			top := matched
			if len(top) > 5 {
				top = top[:5]
			}
			role := r.Job.PositionTitle
			if r.Profile != nil && len(r.Profile.TargetPositions) > 0 {
				role = r.Profile.TargetPositions[0]
			}
			prop := fmt.Sprintf("%s with hands-on experience in %s.", role, joinList(top))
			if t.Original != "" {
				prop = strings.TrimSpace(t.Original) + " Relevant strengths for this role: " + joinList(top) + "."
			}
			out = append(out, Draft{Section: "summary", Original: t.Original, Proposed: prop})
		case "experience":
			// Put the lines that show this job's skills first. Nothing is added or reworded.
			lines := strings.Split(strings.TrimSpace(t.Original), "\n")
			if len(lines) < 2 {
				continue
			}
			hits := func(l string) int {
				n := 0
				for _, s := range matched {
					if parser.ContainsSkill(strings.ToLower(l), s) {
						n++
					}
				}
				return n
			}
			sorted := slices.Clone(lines)
			slices.SortStableFunc(sorted, func(a, b string) int { return hits(b) - hits(a) })
			if prop := strings.Join(sorted, "\n"); prop != strings.Join(lines, "\n") {
				out = append(out, Draft{Section: t.Section, TargetID: t.TargetID, Original: t.Original, Proposed: prop})
			}
		case "skills":
			parts := strings.Split(t.Original, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			sorted := make([]string, 0, len(parts))
			for _, m := range matched {
				for _, p := range parts {
					if parser.Canon(p) == m && !slices.Contains(sorted, p) {
						sorted = append(sorted, p)
					}
				}
			}
			used := map[string]bool{}
			for _, s := range sorted {
				used[strings.ToLower(s)] = true
			}
			for _, p := range parts {
				if !used[strings.ToLower(p)] {
					sorted = append(sorted, p)
				}
			}
			if prop := strings.Join(sorted, ", "); prop != t.Original {
				out = append(out, Draft{Section: "skills", Original: t.Original, Proposed: prop})
			}
		}
	}
	return out, nil
}

func joinList(s []string) string {
	switch len(s) {
	case 0:
		return ""
	case 1:
		return s[0]
	case 2:
		return s[0] + " and " + s[1]
	}
	return strings.Join(s[:len(s)-1], ", ") + ", and " + s[len(s)-1]
}
