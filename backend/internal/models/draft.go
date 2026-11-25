package models

// ApplicationDraft is a cover letter or application Q&A answer tied to a saved job.
type ApplicationDraft struct {
	ID        int64  `json:"id"`
	JobID     int64  `json:"job_id"`
	Kind      string `json:"kind"` // "cover_letter" or "qa"
	Prompt    string `json:"prompt"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
