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
	PayMin     int     `json:"pay_min,omitempty"` // yearly USD read from the description; 0 when it lists none
	PayMax     int     `json:"pay_max,omitempty"`
	Exact      bool    `json:"exact,omitempty"`      // the title matches every keyword word and, if the keywords name a level, that level
	Duplicates int     `json:"duplicates,omitempty"` // other postings of the same title at this company, folded into this one
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
	MaxOffset    = 5000
)

// Work modes a search can ask for.
const (
	WorkModeAny    = "Any"
	WorkModeRemote = "Remote"
	WorkModeHybrid = "Hybrid"
	WorkModeOnSite = "On-site"
)

// Orders a search can ask for.
const (
	SortRelevance = "relevance"
	SortNewest    = "newest"
)

// Experience levels a search can ask for, read from the posting title.
const (
	LevelAny    = "Any"
	LevelIntern = "Intern"
	LevelEntry  = "Entry"
	LevelMid    = "Mid"
	LevelSenior = "Senior"
	LevelLead   = "Lead"
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
	Offset     int      `json:"offset"` // where in the ranked list this page starts; 0 for the first page

	// Preferences. The zero value of each means no preference.
	Sort         string   `json:"sort"`          // "relevance" (default) or "newest"
	Level        string   `json:"level"`         // Intern, Entry, Mid, Senior, Lead or Any; judged from the title
	PostedWithin int      `json:"posted_within"` // days since the posting was updated; 0 for any
	Exclude      []string `json:"exclude"`       // words that rule a posting out when its title has them
	PayMin       int      `json:"pay_min"`       // yearly USD; a posting must pay at least this much at its top
	PayMax       int      `json:"pay_max"`       // yearly USD; a posting must start at or under this
	RequirePay   bool     `json:"require_pay"`   // drop postings that list no pay when a pay range is set
}

// HasPay reports whether the query filters on pay, which needs each posting's description.
func (q Query) HasPay() bool { return q.PayMin > 0 || q.PayMax > 0 }

// Validate checks the fields that do not depend on a connector. The messages are shown to the user.
// Board tokens and categories belong to the connector, so its API layer checks them.
func (q Query) Validate() error {
	if q.Limit < 1 || q.Limit > MaxLimit {
		return errors.New("Limit must be between 1 and 50")
	}
	if q.Offset < 0 || q.Offset > MaxOffset {
		return errors.New("Offset must be between 0 and 5000")
	}
	switch q.Sort {
	case "", SortRelevance, SortNewest:
	default:
		return errors.New("Sort must be relevance or newest")
	}
	switch q.Level {
	case "", LevelAny, LevelIntern, LevelEntry, LevelMid, LevelSenior, LevelLead:
	default:
		return errors.New("Level must be Intern, Entry, Mid, Senior, Lead or Any")
	}
	if q.PostedWithin < 0 || q.PostedWithin > 365 {
		return errors.New("Posted within must be between 0 and 365 days")
	}
	if q.PayMin < 0 || q.PayMax < 0 || q.PayMin > 10_000_000 || q.PayMax > 10_000_000 {
		return errors.New("Pay must be a yearly amount between 0 and 10,000,000")
	}
	if q.PayMax > 0 && q.PayMin > q.PayMax {
		return errors.New("Minimum pay cannot be above maximum pay")
	}
	if len(q.Exclude) > 10 {
		return errors.New("Too many words to exclude")
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
	HasMore         bool          `json:"has_more"`         // more ranked postings follow this page
	NextOffset      int           `json:"next_offset"`      // the offset that asks for the next page
	ExactMatched    int           `json:"exact_matched"`    // matched postings whose title is an exact match, when keywords were given
	LevelWanted     string        `json:"level_wanted"`     // a level the keywords name ("Entry" for "junior developer"), else empty
	Partial         bool          `json:"partial"`          // matched is a lower bound: a pay filter stopped scanning early
	Companies       []string      `json:"companies"`        // companies searched when the query named one, else empty
	LocationMatched int           `json:"location_matched"` // of those, postings in the requested location itself (0 when none was asked)
	BoardsSearched  int           `json:"boards_searched"`  // boards listed without error
	BoardsTotal     int           `json:"boards_total"`     // boards the search set out to list
	PostingsScanned int           `json:"postings_scanned"` // postings seen across those boards
	Errors          []SearchError `json:"errors"`
	KeywordsUsed    []string      `json:"keywords_used"`
	WorkModeUsed    string        `json:"work_mode_used"`
}
