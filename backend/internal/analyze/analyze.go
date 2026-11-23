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

// Likely is a skill the resume does not list but probably has, because it shows a related one.
type Likely struct {
	Skill   string `json:"skill"`
	Because string `json:"because"` // the related skill the resume shows
}

// Group is one skill list split into matched and missing. Neither list is nil.
type Group struct {
	Matched []Match `json:"matched"`
	// Likely skills are not on the resume but sit next to ones that are (PostgreSQL for MySQL).
	// They are not missing; the score gives them half credit.
	Likely  []Likely `json:"likely"`
	Missing []string `json:"missing"`
	// Unmentioned are the soft skills the resume does not name. A resume rarely lists them, so
	// they are no evidence against the person and never count as missing. They are reported on
	// their own, so the user still sees what the posting asked for.
	Unmentioned []string `json:"unmentioned"`
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
	Score      float64 `json:"score"` // matched required technical skills over all of them; 1 when there are none
	DurationMS int64   `json:"duration_ms"`
}

// Run compares the job's skill lists with m.
//
// The resume's skills are m.Skills() (profile and experience skills) plus the profile's clearances
// and certifications, all canonicalized with parser.Canon. A job skill is matched when it is one of
// them, when the parser finds it in an experience's description or impact, or when another skill
// covers it: a resume that shows "rest" shows "api", and one that shows "google analytics" shows
// "analytics". Its evidence is the experiences whose skill list or text has it, in resume order; a
// skill matched only through the profile's own lists has no evidence.
//
// A soft skill the resume does not name is not missing. Resumes rarely list them, so counting them
// would mark down people for what they did not write down. They are reported as unmentioned, and
// the score counts the technical skills only.
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

	// The experiences that show a skill, in resume order.
	evidence := func(skill string) []int64 {
		ids := []int64{}
		for _, e := range exps {
			if e.skills[skill] {
				ids = append(ids, e.id)
			}
		}
		return ids
	}

	// The skill the resume shows that covers this one, if there is one.
	coverOf := func(skill string) (string, bool) {
		for _, cover := range parser.Implied(skill) {
			if have[cover] {
				return cover, true
			}
		}
		return "", false
	}

	// The related skill the resume shows, if there is one: the reason to think the person has this one.
	relatedOf := func(skill string) (string, bool) {
		for _, rel := range parser.Related(skill) {
			if have[rel] {
				return rel, true
			}
		}
		return "", false
	}

	group := func(skills []string) Group {
		g := Group{Matched: []Match{}, Likely: []Likely{}, Missing: []string{}, Unmentioned: []string{}}
		for _, raw := range skills {
			skill := parser.Canon(raw)
			if have[skill] {
				g.Matched = append(g.Matched, Match{Skill: raw, ExperienceIDs: evidence(skill)})
				continue
			}
			if parser.IsSoft(skill) {
				g.Unmentioned = append(g.Unmentioned, raw)
				continue
			}
			if cover, ok := coverOf(skill); ok {
				g.Matched = append(g.Matched, Match{Skill: raw, ExperienceIDs: evidence(cover)})
				continue
			}
			if because, ok := relatedOf(skill); ok {
				g.Likely = append(g.Likely, Likely{Skill: raw, Because: because})
				continue
			}
			g.Missing = append(g.Missing, raw)
		}
		return g
	}

	r := Result{
		JobID:     job.ID,
		Required:  Split{Tech: group(job.ReqTechSkills), Soft: group(job.ReqSoftSkills)},
		Preferred: Split{Tech: group(job.PrefTechSkills), Soft: group(job.PrefSoftSkills)},
		Score:     1,
	}
	// The score counts the technical skills only: a soft skill nobody writes down is not something
	// to be marked down for.
	// A likely skill earns half credit, and is no gap.
	g := r.Required.Tech
	if total := len(g.Matched) + len(g.Likely) + len(g.Missing); total > 0 {
		r.Score = (float64(len(g.Matched)) + 0.5*float64(len(g.Likely))) / float64(total)
	}
	r.DurationMS = time.Since(start).Milliseconds()
	return r
}
