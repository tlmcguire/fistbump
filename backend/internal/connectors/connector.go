// Package connectors defines opt-in job imports behind one interface.
package connectors

import (
	"errors"
	"strings"
	"time"
)

// Posting is a normalized job posting from any connector.
type Posting struct {
	Source        string    `json:"source,omitempty"`
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
	// Fit is how the posting's required skills compare with the user's resume. Search results only, and
	// nil when there is no resume or the posting lists no required skills.
	Fit *Fit `json:"fit,omitempty"`
	// Ratings is a good, fair or bad for the posting's pay, benefits and cost of living. Search results only, and
	// nil when the search does not rate them.
	Ratings *Ratings `json:"ratings,omitempty"`
}

// Ratings are the cost analysis boiled down to a word for each part: good, fair or bad. A part that cannot be
// judged (no pay listed and no market figure, benefits the posting does not describe) is empty.
type Ratings struct {
	Pay      string `json:"pay"`
	Benefits string `json:"benefits"`
	Living   string `json:"living"`

	// The figures behind the words, for sorting. They are not sent to the app.
	BenefitsFound []string `json:"benefits_found,omitempty"` // the benefits the posting names
	LeftMonthly   *int     `json:"left_monthly,omitempty"`   // dollars left each month after a modest budget; negative when short
	PayAmount     int      `json:"-"`                        // yearly pay the posting lists, 0 when it lists none
	BenefitsScore int      `json:"-"`                        // 0 to 100
	LivingLeft    int      `json:"-"`                        // percent of take-home left after a modest budget; can be negative
}

// RatingRank orders the words: bad 1, fair 2, good 3, and 0 for a part that was not rated.
func RatingRank(word string) int {
	switch word {
	case "bad":
		return 1
	case "fair":
		return 2
	case "good":
		return 3
	}
	return 0
}

// Fit compares a posting's required skills with the master resume. It is the same deterministic
// analysis the Jobs view shows, never a model's opinion.
type Fit struct {
	Percent  int      `json:"percent"`  // matched required skills over all required, 0 to 100
	Matched  int      `json:"matched"`  // required skills the resume shows
	Required int      `json:"required"` // required skills the posting lists
	Missing  []string `json:"missing"`  // real gaps: required skills the resume neither shows nor suggests
	Likely   []string `json:"likely"`   // required skills not listed, but probably known from related ones
	Because  []string `json:"because"`  // for each Likely skill, the related skill the resume shows
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
	SortFit       = "fit"      // best fit with the resume
	SortPay       = "pay"      // highest listed pay
	SortBenefits  = "benefits" // best benefits listed
	SortLiving    = "living"   // most left over after the cost of living
	SortOverall   = "overall"  // fit and the three ratings together
)

// SortsByScore reports whether the order depends on each posting's description, so the descriptions must be read first.
func (q Query) SortsByScore() bool {
	switch q.Sort {
	case SortFit, SortPay, SortBenefits, SortLiving, SortOverall:
		return true
	}
	return false
}

// SortKey is a posting's value under the query's score sort, and false when it has none (an unrated posting sorts last).
func (q Query) SortKey(p Posting) (float64, bool) {
	r := p.Ratings
	rank := func(word string) float64 { return float64(RatingRank(word)) }
	switch q.Sort {
	case SortFit:
		if p.Fit == nil {
			return 0, false
		}
		return float64(p.Fit.Percent), true
	case SortPay:
		if r == nil || r.PayAmount == 0 {
			return 0, false
		}
		return float64(r.PayAmount), true
	case SortBenefits:
		if r == nil || r.Benefits == "" {
			return 0, false
		}
		return float64(r.BenefitsScore), true
	case SortLiving:
		if r == nil || r.Living == "" {
			return 0, false
		}
		return float64(r.LivingLeft), true
	case SortOverall:
		total, any := 0.0, false
		if p.Fit != nil {
			total, any = total+float64(p.Fit.Percent)/100*3, true
		}
		if r != nil {
			total += rank(r.Pay) + rank(r.Benefits) + rank(r.Living)
			any = any || r.Pay != "" || r.Benefits != "" || r.Living != ""
		}
		return total, any
	}
	return 0, false
}

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
	Source      string   `json:"source,omitempty"` // "greenhouse", "lever", "ashby", or empty for any
	Keywords    []string `json:"keywords"`
	Location    string   `json:"location"`
	RadiusMiles int      `json:"radius_miles,omitempty"`
	WorkMode    string   `json:"work_mode"`
	Categories  []string `json:"categories"`
	Boards      []string `json:"boards"`
	Company     string   `json:"company"`
	Limit       int      `json:"limit"`
	Offset      int      `json:"offset"` // where in the ranked list this page starts; 0 for the first page

	// Preferences. The zero value of each means no preference.
	Sort         string   `json:"sort"`          // "relevance" (default) or "newest"
	Level        string   `json:"level"`         // Intern, Entry, Mid, Senior, Lead or Any; judged from the title
	PostedWithin int      `json:"posted_within"` // days since the posting was updated; 0 for any
	Exclude      []string `json:"exclude"`       // words that rule a posting out when its title has them
	PayMin       int      `json:"pay_min"`       // yearly USD; a posting must pay at least this much at its top
	PayMax       int      `json:"pay_max"`       // yearly USD; a posting must start at or under this
	RequirePay   bool     `json:"require_pay"`   // drop postings that list no pay when a pay range is set
	MinFit       int      `json:"min_fit"`       // percent of required skills the resume must show; 0 for any
	MaxMissing   *int     `json:"max_missing"`   // most required skills the resume may lack; nil for any
	// Rating filters: the lowest word a posting may have, "fair" or "good"; empty for any. A posting whose part
	// could not be rated does not pass, because there is nothing to show it is good enough.
	MinPayRating string `json:"min_pay_rating"`
	MinBenefits  string `json:"min_benefits"`
	MinLiving    string `json:"min_living"`
	// CachedOnly searches only the boards earlier searches have listed, without listing any. Home's matches use it,
	// so showing Home never sends a request per board.
	CachedOnly bool `json:"cached_only"`
	// UseRatings turns the good, fair and bad ratings and their filters on. Nil means yes.
	UseRatings *bool `json:"use_ratings"`
	// UseResume tells the search to rate the postings against the user's resume and to rank them
	// by how well they fit. It is nil when the request does not say, which means yes: the fit
	// filters and the skill ranking are on unless the user turns them off.
	UseResume *bool `json:"use_resume"`

	// SkillScore rates a posting's text against the user's resume, 0 to 1. The API layer sets it when
	// a profile exists, so connectors need not know about resumes; nil ranks without skill fit.
	SkillScore func(text string) float64 `json:"-"`
	// FitOf compares a posting's text with the resume. The API layer sets it when a profile exists.
	FitOf func(text string) *Fit `json:"-"`
	// RateOf rates a posting whose description is loaded. The API layer sets it, since the rating needs the
	// user's home location and minimum pay.
	RateOf func(p Posting) *Ratings `json:"-"`
	// WithinRadius is resolved by the API from a local place index. Nil means no radius filter.
	WithinRadius func(location string) bool `json:"-"`
}

// MatchesRatings reports whether the search rates pay, benefits and cost of living. A request that does not say means yes.
func (q Query) MatchesRatings() bool { return q.UseRatings == nil || *q.UseRatings }

// HasRatingFilter reports whether the query filters on a rating, which needs each posting's description.
func (q Query) HasRatingFilter() bool {
	return q.MatchesRatings() && (q.MinPayRating != "" || q.MinBenefits != "" || q.MinLiving != "")
}

// RatingsOK reports whether a posting's ratings pass the rating filters.
func (q Query) RatingsOK(r *Ratings) bool {
	if !q.HasRatingFilter() {
		return true
	}
	if r == nil {
		return false
	}
	return RatingRank(r.Pay) >= RatingRank(q.MinPayRating) && RatingRank(r.Benefits) >= RatingRank(q.MinBenefits) &&
		RatingRank(r.Living) >= RatingRank(q.MinLiving)
}

// MatchesResume reports whether the search rates the postings against the user's resume. A
// request that does not say means yes.
func (q Query) MatchesResume() bool { return q.UseResume == nil || *q.UseResume }

// HasFit reports whether the query filters on fit, which needs each posting's description.
func (q Query) HasFit() bool { return q.MinFit > 0 || q.MaxMissing != nil }

// FitOK reports whether a posting passes the fit filters. A posting with no rating (it lists no
// required skills) is kept, because there is nothing to hold against it.
func (q Query) FitOK(f *Fit) bool {
	if f == nil {
		return true
	}
	if q.MinFit > 0 && f.Percent < q.MinFit {
		return false
	}
	return q.MaxMissing == nil || len(f.Missing) <= *q.MaxMissing
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
	case "", SortRelevance, SortNewest, SortFit, SortPay, SortBenefits, SortLiving, SortOverall:
	default:
		return errors.New("Sort is not valid")
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
	if q.MinFit < 0 || q.MinFit > 100 {
		return errors.New("Minimum fit must be between 0 and 100 percent")
	}
	if q.MaxMissing != nil && (*q.MaxMissing < 0 || *q.MaxMissing > 50) {
		return errors.New("Missing skills must be between 0 and 50")
	}
	for _, min := range []string{q.MinPayRating, q.MinBenefits, q.MinLiving} {
		if min != "" && min != "fair" && min != "good" {
			return errors.New("A rating filter must be fair or good")
		}
	}
	if len(q.Exclude) > 10 {
		return errors.New("Too many words to exclude")
	}
	switch q.WorkMode {
	case WorkModeAny, WorkModeRemote, WorkModeHybrid, WorkModeOnSite:
	default:
		return errors.New("Work mode must be Remote, Hybrid, On-site or Any")
	}
	switch strings.ToLower(strings.TrimSpace(q.Source)) {
	case "", "greenhouse", "lever", "ashby":
	default:
		return errors.New("ATS source must be greenhouse, lever, ashby, or empty")
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
	BoardsCached    int           `json:"boards_cached"`    // of those, boards read from this computer's saved listings instead of Greenhouse
	PostingsScanned int           `json:"postings_scanned"` // postings seen across those boards
	Errors          []SearchError `json:"errors"`
	RateLimited     bool          `json:"rate_limited"` // Greenhouse answered "too many requests", so some boards or descriptions were skipped
	KeywordsUsed    []string      `json:"keywords_used"`
	WorkModeUsed    string        `json:"work_mode_used"`
}
