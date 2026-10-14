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
	UpdatedAt     time.Time `json:"updated_at"` // UTC
	RawText       string    `json:"raw_text"`
}
