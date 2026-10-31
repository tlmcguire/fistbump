// Package analyze compares a job's skills with the master resume. It is deterministic and never
// calls a model.
//
// This file holds the interface the api package builds on (phase 4 step 5 fills it in). Run keeps
// this signature; the body is a placeholder until then.
package analyze

import (
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
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

// Run compares job with m. Placeholder: every skill is reported missing.
func Run(job models.Job, m models.Master) Result {
	start := time.Now()
	group := func(skills []string) Group {
		return Group{Matched: []Match{}, Missing: append([]string{}, skills...)}
	}
	r := Result{
		JobID:     job.ID,
		Required:  Split{Tech: group(job.ReqTechSkills), Soft: group(job.ReqSoftSkills)},
		Preferred: Split{Tech: group(job.PrefTechSkills), Soft: group(job.PrefSoftSkills)},
		Score:     1,
	}
	if len(job.ReqTechSkills)+len(job.ReqSoftSkills) > 0 {
		r.Score = 0
	}
	r.DurationMS = time.Since(start).Milliseconds()
	return r
}
