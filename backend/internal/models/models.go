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

// Experience is one work history entry. A nil EndDate means the role is current unless DateNA is true.
type Experience struct {
	ID          int64    `json:"id"`
	ProfileID   int64    `json:"profile_id"`
	CompanyName string   `json:"company_name"`
	JobTitle    string   `json:"job_title"`
	StartDate   *string  `json:"start_date"`
	EndDate     *string  `json:"end_date"`
	DateNA      bool     `json:"date_na"`
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

// Job is a saved job posting, pasted, entered by hand, or imported from Greenhouse. Skill lists and
// Requirements are never nil.
type Job struct {
	ID             int64    `json:"id"`
	CompanyName    string   `json:"company_name"`
	PositionTitle  string   `json:"position_title"`
	ListingURL     *string  `json:"listing_url"`
	Source         string   `json:"source"`
	ExternalID     *string  `json:"external_id"`
	Board          *string  `json:"board"`
	CompanyURL     *string  `json:"company_url"`
	WorkMode       *string  `json:"work_mode"`
	EmploymentType *string  `json:"employment_type"`
	Location       *string  `json:"location"`
	PayMin         *int64   `json:"pay_min"`
	PayMax         *int64   `json:"pay_max"`
	Description    *string  `json:"description"`
	RawText        *string  `json:"raw_text"`
	ReqTechSkills  []string `json:"req_tech_skills"`
	PrefTechSkills []string `json:"pref_tech_skills"`
	ReqSoftSkills  []string `json:"req_soft_skills"`
	PrefSoftSkills []string `json:"pref_soft_skills"`
	Requirements   []string `json:"requirements"`
	CloseDate      *string  `json:"close_date"`
	CreatedAt      string   `json:"created_at"`
	SearchedAt     string   `json:"searched_at"`
	ArchivedAt     *string  `json:"archived_at"`
	TrashedAt      *string  `json:"trashed_at"`
}

// Revision is one round of suggestions for a job. The suggestions are read separately.
type Revision struct {
	ID           int64   `json:"id"`
	JobID        int64   `json:"job_id"`
	BaseResumeID *int64  `json:"base_resume_id"`
	Engine       string  `json:"engine"` // local, remote or rules; the engine that ran, once finished
	Status       string  `json:"status"` // queued, running, done, failed or canceled
	Error        *string `json:"error"`
	CreatedAt    string  `json:"created_at"`
	CompletedAt  *string `json:"completed_at"`
}

// Suggestion is one proposed change in a revision. State starts as pending.
type Suggestion struct {
	ID           int64   `json:"id"`
	RevisionID   int64   `json:"revision_id"`
	Section      string  `json:"section"`
	TargetID     *int64  `json:"target_id"`
	Engine       string  `json:"engine"`
	OriginalText string  `json:"original_text"`
	ProposedText string  `json:"proposed_text"`
	Reason       string  `json:"reason"` // a short explanation of why the engine suggested this
	State        string  `json:"state"`  // pending, accepted, rejected or edited
	EditedText   *string `json:"edited_text"`
}

// Final is the text the tailored resume uses for this suggestion: the user's edit, the proposal
// if accepted, or the original if rejected (or still pending).
func (s Suggestion) Final() string {
	switch {
	case s.State == "edited" && s.EditedText != nil:
		return *s.EditedText
	case s.State == "accepted":
		return s.ProposedText
	}
	return s.OriginalText
}

// TailoredResume is the reviewed result of a revision. Content and BaseContent are empty in lists.
type TailoredResume struct {
	ID           int64  `json:"id"`
	JobID        int64  `json:"job_id"`
	RevisionID   *int64 `json:"revision_id"`
	BaseResumeID *int64 `json:"base_resume_id"`
	Content      string `json:"content,omitempty"`
	BaseContent  string `json:"base_content,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// Application tracks one job through the pipeline. A job has at most one application. Archiving
// sets ArchivedAt and leaves Status unchanged. CompanyName and PositionTitle are read-only, joined
// from the job by every store read, and always sent (no omitempty) so the tracker can show them.
type Application struct {
	ID               int64   `json:"id"`
	JobID            int64   `json:"job_id"`
	ProfileID        int64   `json:"profile_id"`
	BaseResumeID     *int64  `json:"base_resume_id"`
	TailoredResumeID *int64  `json:"tailored_resume_id"`
	Status           string  `json:"status"` // Saved, Applied, Interviewing, Offered or Rejected
	DateApplied      *string `json:"date_applied"`
	NextStepDate     *string `json:"next_step_date"`
	ExportedPDFPath  *string `json:"exported_pdf_path"`
	Notes            *string `json:"notes"`
	ArchivedAt       *string `json:"archived_at"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	CompanyName      string  `json:"company_name"`
	PositionTitle    string  `json:"position_title"`
}
