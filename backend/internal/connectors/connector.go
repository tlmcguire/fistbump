// Package connectors defines opt-in job imports behind one interface.
package connectors

import (
	"errors"
	"time"
)

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
	// Search results only. Phase 1's routes leave these zero, and omitempty keeps them out of the response.
	WorkMode   string  `json:"work_mode,omitempty"`
	Score      float64 `json:"score,omitempty"`
	TitleScore float64 `json:"title_score,omitempty"`
	SkillScore float64 `json:"skill_score,omitempty"`
	Dropped    Dropped `json:"-"` // what cleaning removed; phase 4 reports it when the job is saved
}

// Dropped counts what was removed from a posting before it was kept.
type Dropped struct {
	HiddenElements int // elements a reader would not see, skipped with everything inside them
	InvisibleChars int // invisible characters, such as zero-width and tag characters
}

// Limits on Query.Limit. A request with no limit gets DefaultLimit.
const (
	DefaultLimit = 20
	MaxLimit     = 50
)

// Work modes a search can ask for.
const (
	WorkModeAny    = "Any"
	WorkModeRemote = "Remote"
	WorkModeHybrid = "Hybrid"
	WorkModeOnSite = "On-site"
)

// Query is a job search request. The API layer fills the defaults before calling Validate.
type Query struct {
	Keywords   []string `json:"keywords"`
	Location   string   `json:"location"`
	WorkMode   string   `json:"work_mode"`
	Categories []string `json:"categories"`
	Boards     []string `json:"boards"`
	Company    string   `json:"company"`
	Limit      int      `json:"limit"`
}

// Validate checks the fields that do not depend on a connector. The messages are shown to the user.
// Board tokens and categories belong to the connector, so its API layer checks them.
func (q Query) Validate() error {
	if q.Limit < 1 || q.Limit > MaxLimit {
		return errors.New("Limit must be between 1 and 50")
	}
	switch q.WorkMode {
	case WorkModeAny, WorkModeRemote, WorkModeHybrid, WorkModeOnSite:
	default:
		return errors.New("Work mode must be Remote, Hybrid, On-site or Any")
	}
	return nil
}

// SearchError names a board that could not be searched, and why.
type SearchError struct {
	Board   string `json:"board"`
	Message string `json:"message"`
}

// SearchResult is a ranked list of postings with the counts behind it.
type SearchResult struct {
	Postings        []Posting     `json:"postings"`
	Matched         int           `json:"matched"`          // postings that passed the filters
	LocationMatched int           `json:"location_matched"` // of those, postings in the requested location itself (0 when none was asked)
	BoardsSearched  int           `json:"boards_searched"`  // boards listed without error
	BoardsTotal     int           `json:"boards_total"`     // boards the search set out to list
	PostingsScanned int           `json:"postings_scanned"` // postings seen across those boards
	Errors          []SearchError `json:"errors"`
	KeywordsUsed    []string      `json:"keywords_used"`
	WorkModeUsed    string        `json:"work_mode_used"`
}
