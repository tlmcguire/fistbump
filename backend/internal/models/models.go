// Package models holds the types shared by store, api, render and the packages built later, so
// none of them imports another just to get a type. It has no database or HTTP code.
package models

import "strings"

// Profile is the user's identity and job preferences. List fields are never nil.
type Profile struct {
	ID              int64    `json:"id"`
	FullName        string   `json:"full_name"`
	Email           *string  `json:"email"`
	Phone           *string  `json:"phone"`
	HomeLocation    *string  `json:"home_location"`
	TargetPositions []string `json:"target_positions"`
	PreferredMode   *string  `json:"preferred_mode"`
	MinDesiredPay   *int64   `json:"min_desired_pay"`
	ClearanceCerts  []string `json:"clearance_certs"`
	Summary         *string  `json:"summary"`
	Skills          []string `json:"skills"`
	Links           []string `json:"links"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

// Experience is one work history entry. A nil EndDate means the role is current.
type Experience struct {
	ID          int64    `json:"id"`
	ProfileID   int64    `json:"profile_id"`
	CompanyName string   `json:"company_name"`
	JobTitle    string   `json:"job_title"`
	StartDate   *string  `json:"start_date"`
	EndDate     *string  `json:"end_date"`
	Location    *string  `json:"location"`
	Description *string  `json:"description"`
	Impact      *string  `json:"impact"`
	Skills      []string `json:"skills"`
	SortOrder   int64    `json:"sort_order"`
}

// Education is one education entry.
type Education struct {
	ID             int64    `json:"id"`
	ProfileID      int64    `json:"profile_id"`
	Institution    string   `json:"institution"`
	DegreeLevel    *string  `json:"degree_level"`
	Discipline     *string  `json:"discipline"`
	GraduationDate *string  `json:"graduation_date"`
	GPA            *float64 `json:"gpa"`
	Honors         *string  `json:"honors"`
	Details        *string  `json:"details"`
	SortOrder      int64    `json:"sort_order"`
}

// ResumeVersion is a saved resume document. Content is empty in list responses.
type ResumeVersion struct {
	ID             int64   `json:"id"`
	ProfileID      int64   `json:"profile_id"`
	VersionLabel   string  `json:"version_label"`
	TargetPosition *string `json:"target_position"`
	Content        string  `json:"content,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// Master is the master resume: the profile with its experiences and education.
type Master struct {
	Profile     *Profile
	Experiences []Experience
	Education   []Education
}

// Skills returns the de-duplicated union of the profile's skills and every experience's skills,
// in first-seen order. Duplicates are matched case-insensitively.
func (m Master) Skills() []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(list []string) {
		for _, skill := range list {
			key := strings.ToLower(strings.TrimSpace(skill))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, skill)
		}
	}
	if m.Profile != nil {
		add(m.Profile.Skills)
	}
	for _, e := range m.Experiences {
		add(e.Skills)
	}
	return out
}
