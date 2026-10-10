// Package analyze compares a job posting against the master resume. It is deterministic and never calls a model.
package analyze

import (
	"sort"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/parser"
)

type Match struct {
	Skill         string  `json:"skill"`
	ExperienceIDs []int64 `json:"experience_ids"`
}

type Group struct {
	Matched []Match  `json:"matched"`
	Missing []string `json:"missing"`
}

type Pair struct {
	Tech Group `json:"tech"`
	Soft Group `json:"soft"`
}

type Result struct {
	JobID      int64   `json:"job_id"`
	Required   Pair    `json:"required"`
	Preferred  Pair    `json:"preferred"`
	Score      float64 `json:"score"`
	DurationMS int64   `json:"duration_ms"`
}

// Run compares the job's skill lists with the master resume.
// Master skills are experiences.skills plus profile.clearance_certs. Evidence also searches
// experience title, description and impact text.
func Run(job models.Job, m models.Master) Result {
	start := time.Now()
	have := map[string]bool{}
	for _, s := range m.Skills() {
		have[parser.Canon(s)] = true
	}
	if m.Profile != nil {
		for _, c := range m.Profile.ClearanceCerts {
			have[parser.Canon(c)] = true
		}
	}
	// Per experience: lower-cased searchable text and its own skill set.
	type exp struct {
		id     int64
		text   string
		skills map[string]bool
	}
	exps := make([]exp, 0, len(m.Experiences))
	for _, e := range m.Experiences {
		sk := map[string]bool{}
		for _, s := range e.Skills {
			sk[parser.Canon(s)] = true
		}
		var parts []string
		parts = append(parts, e.JobTitle)
		if e.Description != nil {
			parts = append(parts, *e.Description)
		}
		if e.Impact != nil {
			parts = append(parts, *e.Impact)
		}
		exps = append(exps, exp{e.ID, strings.ToLower(strings.Join(parts, "\n")), sk})
	}

	group := func(skills []string) Group {
		g := Group{Matched: []Match{}, Missing: []string{}}
		for _, raw := range skills {
			skill := parser.Canon(raw)
			ids := []int64{}
			for _, e := range exps {
				if e.skills[skill] || textHas(e.text, skill) {
					ids = append(ids, e.id)
				}
			}
			if len(ids) > 0 || have[skill] {
				g.Matched = append(g.Matched, Match{Skill: skill, ExperienceIDs: ids})
			} else {
				g.Missing = append(g.Missing, skill)
			}
		}
		sort.Slice(g.Matched, func(i, j int) bool { return g.Matched[i].Skill < g.Matched[j].Skill })
		sort.Strings(g.Missing)
		return g
	}

	r := Result{JobID: job.ID}
	r.Required = Pair{Tech: group(job.ReqTech), Soft: group(job.ReqSoft)}
	r.Preferred = Pair{Tech: group(job.PrefTech), Soft: group(job.PrefSoft)}
	total := len(job.ReqTech) + len(job.ReqSoft)
	if total > 0 {
		matched := len(r.Required.Tech.Matched) + len(r.Required.Soft.Matched)
		r.Score = float64(int(float64(matched)/float64(total)*100+0.5)) / 100
	}
	r.DurationMS = time.Since(start).Milliseconds()
	return r
}

func textHas(text, skill string) bool { return parser.ContainsSkill(text, skill) }

// Matched returns the canonical names of every matched skill (required first), without duplicates.
func (r Result) Matched() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, g := range []Group{r.Required.Tech, r.Required.Soft, r.Preferred.Tech, r.Preferred.Soft} {
		for _, m := range g.Matched {
			if !seen[m.Skill] {
				seen[m.Skill] = true
				out = append(out, m.Skill)
			}
		}
	}
	return out
}

// Missing returns the canonical names of every missing required and preferred skill.
func (r Result) Missing() []string {
	out := []string{}
	for _, g := range []Group{r.Required.Tech, r.Required.Soft, r.Preferred.Tech, r.Preferred.Soft} {
		out = append(out, g.Missing...)
	}
	return out
}
