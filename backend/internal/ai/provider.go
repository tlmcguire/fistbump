// Package ai defines the suggestion Provider and its local, remote and rule-based implementations.
package ai

import (
	"context"
	"errors"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/analyze"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/render"
)

// Target is one piece of resume text a provider may propose a rewrite for.
// Summary targets have an empty Original: the proposal is new text.
type Target struct {
	Section  string // summary, experience, skills, education
	TargetID *int64
	Label    string // human-readable, used in prompts
	Original string
}

type Request struct {
	Job      models.Job
	Analysis analyze.Result
	Profile  *models.Profile
	Targets  []Target
}

type Draft struct {
	Section  string
	TargetID *int64
	Original string
	Proposed string
}

// Provider produces suggestions for a request.
type Provider interface {
	Name() string // local, remote, rules
	Suggest(ctx context.Context, r Request) ([]Draft, error)
}

var (
	// ErrUnavailable means the engine cannot run (not configured, not installed, not reachable).
	ErrUnavailable = errors.New("engine unavailable")
)

// BuildTargets lists the text of the master resume that suggestions may rewrite.
func BuildTargets(m models.Master) []Target {
	ts := []Target{{Section: "summary", Label: "Professional summary"}}
	if m.Profile != nil && m.Profile.Summary != nil && strings.TrimSpace(*m.Profile.Summary) != "" {
		ts[0].Original = strings.TrimSpace(*m.Profile.Summary)
	}
	for _, e := range m.Experiences {
		id := e.ID
		if e.Description != nil && *e.Description != "" {
			ts = append(ts, Target{"experience", &id, e.JobTitle + " at " + e.CompanyName + " (responsibilities)", *e.Description})
		}
		if e.Impact != nil && *e.Impact != "" {
			ts = append(ts, Target{"experience", &id, e.JobTitle + " at " + e.CompanyName + " (impact)", *e.Impact})
		}
	}
	if sk := m.Skills(); len(sk) > 0 {
		ts = append(ts, Target{Section: "skills", Label: "Skills (comma-separated, reorder only)", Original: render.SkillsLine(sk)})
	}
	return ts
}
