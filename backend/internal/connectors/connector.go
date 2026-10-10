// Package connectors defines opt-in job imports behind one interface.
package connectors

import "context"

// Posting is a normalized job posting from any connector.
type Posting struct {
	ExternalID    string  `json:"external_id"`
	Board         string  `json:"board"`
	PositionTitle string  `json:"position_title"`
	CompanyName   string  `json:"company_name"`
	ListingURL    string  `json:"listing_url"`
	Location      string  `json:"location"`
	RawText       string  `json:"raw_text"`
	Score         float64 `json:"score"`
	TitleScore    float64 `json:"title_score"`
	SkillScore    float64 `json:"skill_score"`
	WorkMode      string  `json:"work_mode"`
	UpdatedAt     string  `json:"updated_at"`
}

// Query selects and filters postings. Boards is the resolved, de-duplicated board set.
type Query struct {
	Boards       []string
	Keywords     []string // title phrases; a posting matches when its title covers one of them
	Location     string   // comma-separated terms; remote postings also match unless WorkMode is On-site
	WorkMode     string   // Remote, Hybrid, On-site, or empty/Any for no filter
	ResumeSkills []string
	HomeLocation string // from the profile; ranks nearby postings higher
	Limit        int
}

type BoardError struct {
	Board   string `json:"board"`
	Message string `json:"message"`
}

type Result struct {
	Postings        []Posting    `json:"postings"`
	BoardsSearched  int          `json:"boards_searched"`
	BoardsTotal     int          `json:"boards_total"`
	PostingsScanned int          `json:"postings_scanned"`
	Matched         int          `json:"matched"`
	Errors          []BoardError `json:"errors"`
}

// Connector fetches postings for a query.
type Connector interface {
	ID() string
	Name() string
	FetchPostings(ctx context.Context, q Query) (Result, error)
}
