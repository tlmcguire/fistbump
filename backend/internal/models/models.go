// Package models holds types shared across packages.
package models

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
	Skills          []string `json:"skills"` // general skills not tied to one role
	Links           []string `json:"links"`  // web addresses from the resume header, as written
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

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
	SortOrder   int      `json:"sort_order"`
}

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
	SortOrder      int      `json:"sort_order"`
}

// Master is the structured master resume.
type Master struct {
	Profile     *Profile
	Experiences []Experience
	Education   []Education
}

// Skills returns the de-duplicated union of experience skills and the profile's general skills.
func (m Master) Skills() []string {
	seen := map[string]bool{}
	out := []string{}
	if m.Profile != nil {
		for _, s := range m.Profile.Skills {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	for _, e := range m.Experiences {
		for _, s := range e.Skills {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

type ResumeVersion struct {
	ID             int64   `json:"id"`
	ProfileID      int64   `json:"profile_id"`
	VersionLabel   string  `json:"version_label"`
	TargetPosition *string `json:"target_position"`
	Content        *string `json:"content,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

type Job struct {
	ID             int64    `json:"id"`
	CompanyName    string   `json:"company_name"`
	PositionTitle  string   `json:"position_title"`
	ListingURL     *string  `json:"listing_url"`
	Source         string   `json:"source"`
	ExternalID     *string  `json:"external_id"`
	WorkMode       *string  `json:"work_mode"`
	EmploymentType *string  `json:"employment_type"`
	Location       *string  `json:"location"`
	PayMin         *int64   `json:"pay_min"`
	PayMax         *int64   `json:"pay_max"`
	Description    *string  `json:"description,omitempty"`
	RawText        *string  `json:"raw_text,omitempty"`
	ReqTech        []string `json:"req_tech_skills"`
	PrefTech       []string `json:"pref_tech_skills"`
	ReqSoft        []string `json:"req_soft_skills"`
	PrefSoft       []string `json:"pref_soft_skills"`
	Requirements   []string `json:"requirements"`
	CloseDate      *string  `json:"close_date"`
	CreatedAt      string   `json:"created_at"`
	SearchedAt     string   `json:"searched_at"`
	ArchivedAt     *string  `json:"archived_at"`
}

type Revision struct {
	ID           int64        `json:"id"`
	JobID        int64        `json:"job_id"`
	BaseResumeID *int64       `json:"base_resume_id"`
	Engine       string       `json:"engine"`
	Status       string       `json:"status"`
	Error        *string      `json:"error"`
	CreatedAt    string       `json:"created_at"`
	CompletedAt  *string      `json:"completed_at"`
	Suggestions  []Suggestion `json:"suggestions,omitempty"` // set (possibly empty) on single-revision reads
}

type Suggestion struct {
	ID           int64   `json:"id"`
	RevisionID   int64   `json:"revision_id"`
	Section      string  `json:"section"`
	TargetID     *int64  `json:"target_id"`
	OriginalText string  `json:"original_text"`
	ProposedText string  `json:"proposed_text"`
	State        string  `json:"state"`
	EditedText   *string `json:"edited_text"`
}

type TailoredResume struct {
	ID           int64   `json:"id"`
	JobID        int64   `json:"job_id"`
	RevisionID   *int64  `json:"revision_id"`
	BaseResumeID *int64  `json:"base_resume_id"`
	Content      *string `json:"content,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

type Application struct {
	ID               int64   `json:"id"`
	JobID            int64   `json:"job_id"`
	ProfileID        int64   `json:"profile_id"`
	BaseResumeID     *int64  `json:"base_resume_id"`
	TailoredResumeID *int64  `json:"tailored_resume_id"`
	Status           string  `json:"status"`
	DateApplied      *string `json:"date_applied"`
	NextStepDate     *string `json:"next_step_date"`
	ExportedPDFPath  *string `json:"exported_pdf_path"`
	Notes            *string `json:"notes"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	CompanyName      string  `json:"company_name,omitempty"`
	PositionTitle    string  `json:"position_title,omitempty"`
}
