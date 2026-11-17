// Package analyze compares a job's skills with the master resume. It is deterministic and never
// calls a model, so it stays well under the 1.5 s that NFR-3 allows.
package analyze

import (
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/parser"
)

// Match is a job skill the resume shows, with the experiences that show it.
type Match struct {
	Skill         string  `json:"skill"`
	ExperienceIDs []int64 `json:"experience_ids"`
}

// Group is one skill list split into matched and missing. Neither list is nil.
type Group struct {
	Matched []Match  `json:"matched"`
	Missing []string `json:"missing"`
}

// Split is the tech and soft halves of the required or preferred skills.
type Split struct {
	Tech Group `json:"tech"`
	Soft Group `json:"soft"`
}

// Result is the analysis shape in docs/api.md.
type Result struct {
	JobID      int64   `json:"job_id"`
	Required   Split   `json:"required"`
	Preferred  Split   `json:"preferred"`
	Score      float64 `json:"score"` // matched required skills over all required; 1 when none are required
	DurationMS int64   `json:"duration_ms"`
}

// Run compares the job's skill lists with m.
//
// The resume's skills are m.Skills() (profile and experience skills) plus the profile's clearances
// and certifications, all canonicalized with parser.Canon. A job skill is matched when it is one of
// them, or when the parser finds it in an experience's description or impact. Its evidence is the
// experiences whose skill list or text has it, in resume order; a skill matched only through the
// profile's own lists has no evidence.
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

	// Each experience's skills: its own list plus what the parser finds in its text. The text is
	// scanned once per experience, not once per job skill, because search ranking runs this for
	// every result.
	type exp struct {
		id     int64
		skills map[string]bool
	}
	exps := make([]exp, 0, len(m.Experiences))
	for _, e := range m.Experiences {
		sk := map[string]bool{}
		for _, s := range e.Skills {
			sk[parser.Canon(s)] = true
		}
		for _, text := range []*string{e.Description, e.Impact} {
			if text == nil {
				continue
			}
			tech, soft := parser.FindSkills(*text)
			for _, s := range append(tech, soft...) {
				sk[s] = true
				have[s] = true
			}
		}
		exps = append(exps, exp{e.ID, sk})
	}

	group := func(skills []string) Group {
		g := Group{Matched: []Match{}, Missing: []string{}}
		for _, raw := range skills {
			skill := parser.Canon(raw)
			if !have[skill] {
				g.Missing = append(g.Missing, raw)
				continue
			}
			ids := []int64{}
			for _, e := range exps {
				if e.skills[skill] {
					ids = append(ids, e.id)
				}
			}
			g.Matched = append(g.Matched, Match{Skill: raw, ExperienceIDs: ids})
		}
		return g
	}

	r := Result{
		JobID:     job.ID,
		Required:  Split{Tech: group(job.ReqTechSkills), Soft: group(job.ReqSoftSkills)},
		Preferred: Split{Tech: group(job.PrefTechSkills), Soft: group(job.PrefSoftSkills)},
		Score:     1,
	}
	if total := len(job.ReqTechSkills) + len(job.ReqSoftSkills); total > 0 {
		matched := len(r.Required.Tech.Matched) + len(r.Required.Soft.Matched)
		r.Score = float64(matched) / float64(total)
	}
	r.DurationMS = time.Since(start).Milliseconds()
	return r
}
