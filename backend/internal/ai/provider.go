package ai

import (
	"context"
	"errors"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/analyze"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/render"
)

// Engine names
const (
	EngineLocal  = "local"
	EngineRemote = "remote"
	EngineRules  = "rules"
)

// ErrUnavailable means an engine cannot run right now. API code = 503 ai_unavailable
var ErrUnavailable = errors.New("ai engine unavailable")

// Target is the parts of the master resume that a suggestion may alter.
type Target struct {
	Section  string // e.g. "summary", "experience", "skills"
	TargetID *int64 // experiences.id for expereience targets; nil otherwise
	Label    string // e,g, "Data Analyst at ACME (responsibilities)"
	Original string // original content before edit
}

// Request is everything an engine may use for its analysis
type Request struct {
	Job      models.Job
	Analysis analyze.Result
	Profile  *models.Profile
	Targets  []Target
	// Resume is the whole master resume as text (render.Master). The guard checks model
	// proposals against it, so they cannot claim what the resume does not show.
	Resume string
	// Language is the two-letter code the explanations are written in ("en" when empty).
	Language string
}

// Draft is a proposed change before it is stored as a suggestion.
type Draft struct {
	Section  string
	TargetID *int64
	Original string
	Proposed string
	Reason   string // a short explanation of the change, in the request's language
	Engine   string
}

// Provider is implemented in all engines
type Provider interface {
	Name() string // EngineLocal, EngineRemote, or EngineRules
	Ready() bool  // can run now
	Suggest(ctx context.Context, r Request) ([]Draft, error)
}

// BuildTargets lists what suggestions may replace.
// Each Original matches the text render.Master prints for that field.
func BuildTargets(m models.Master) []Target {
	var out []Target

	// Summary: always one target. render.Master trims and escapes it.
	summary := ""
	if m.Profile != nil && m.Profile.Summary != nil {
		summary = render.Escape(strings.TrimSpace(*m.Profile.Summary))
	}
	out = append(out, Target{Section: "summary", Label: "Summary", Original: summary})

	// Experience: description and impact, when non-blank. render.Master escapes them but does not trim.
	for i := range m.Experiences {
		e := m.Experiences[i]
		id := e.ID
		role := e.JobTitle + " at " + e.CompanyName
		if e.Description != nil && strings.TrimSpace(*e.Description) != "" {
			out = append(out, Target{
				Section:  "experience",
				TargetID: &id,
				Label:    role + " (responsibilities)",
				Original: render.Escape(*e.Description),
			})
		}
		if e.Impact != nil && strings.TrimSpace(*e.Impact) != "" {
			out = append(out, Target{
				Section:  "experience",
				TargetID: &id,
				Label:    role + " (impact)",
				Original: render.Escape(*e.Impact),
			})
		}
	}

	// Skills: the union render.Master prints, joined the same way. Not escaped.
	if skills := m.Skills(); len(skills) > 0 {
		out = append(out, Target{
			Section:  "skills",
			Label:    "Skills",
			Original: strings.Join(skills, ", "),
		})
	}
	return out
}
