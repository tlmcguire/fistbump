package models

// Event is a plan on the calendar: an interview, a call or a follow-up, with a start and an end.
// Times are RFC 3339 with the offset of the computer that made them, so they read back in local time.
type Event struct {
	ID        int64  `json:"id"`
	JobID     *int64 `json:"job_id"`
	Kind      string `json:"kind"` // interview | call | follow_up | other
	Title     string `json:"title"`
	StartsAt  string `json:"starts_at"`
	EndsAt    string `json:"ends_at"`
	Location  string `json:"location"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	// Filled in on read from the job, when there is one.
	CompanyName   string `json:"company_name,omitempty"`
	PositionTitle string `json:"position_title,omitempty"`
}
