// Package parser turns job posting text into skills, requirements and inferred fields. It is
// deterministic and does no I/O.
//
// This file holds the interface the api package builds on (phase 4 steps 3 and 4 fill it in).
// Parse and Canon keep these signatures; the bodies are placeholders until then.
package parser

import (
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/textclean"
)

// Inferred holds the fields guessed from the posting text. Each is empty when not found.
type Inferred struct {
	CompanyName   string `json:"company_name"`
	PositionTitle string `json:"position_title"`
	Location      string `json:"location"`
	WorkMode      string `json:"work_mode"` // Remote, Hybrid, On-site, or empty
}

// Result is what POST /v1/jobs/parse returns. Lists are never nil.
type Result struct {
	Description    string   `json:"description"`
	ReqTechSkills  []string `json:"req_tech_skills"`
	PrefTechSkills []string `json:"pref_tech_skills"`
	ReqSoftSkills  []string `json:"req_soft_skills"`
	PrefSoftSkills []string `json:"pref_soft_skills"`
	Requirements   []string `json:"requirements"`
	Inferred       Inferred `json:"inferred"`
	Hidden         int      `json:"-"` // invisible characters removed while cleaning
}

// Parse reads a posting. Placeholder: it only cleans the text and finds no skills.
func Parse(raw string) Result {
	text, hidden := textclean.StripInvisible(raw)
	return Result{
		Description:    strings.TrimSpace(text),
		ReqTechSkills:  []string{},
		PrefTechSkills: []string{},
		ReqSoftSkills:  []string{},
		PrefSoftSkills: []string{},
		Requirements:   []string{},
		Hidden:         hidden,
	}
}

// Canon returns the canonical name for a skill or alias, or the trimmed lower-case input if unknown.
// Placeholder: it knows no aliases yet.
func Canon(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
