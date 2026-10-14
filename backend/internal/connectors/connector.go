// Package connectors defines opt-in job imports behind one interface.
package connectors

import "time"

// Posting is a normalized job posting from any connector.
type Posting struct {
	ExternalID    string    `json:"external_id"`
	Board         string    `json:"board"`
	CompanyName   string    `json:"company_name"`
	PositionTitle string    `json:"position_title"`
	Location      string    `json:"location"`
	ListingURL    string    `json:"listing_url"`
	CompanyURL    *string   `json:"company_url"` // null when unknown
	UpdatedAt     time.Time `json:"updated_at"`  // UTC
	RawText       string    `json:"raw_text"`
	Dropped       Dropped   `json:"-"` // what cleaning removed; phase 4 reports it when the job is saved
}

// Dropped counts what was removed from a posting before it was kept.
type Dropped struct {
	HiddenElements int // elements a reader would not see, skipped with everything inside them
	InvisibleChars int // invisible characters, such as zero-width and tag characters
}
