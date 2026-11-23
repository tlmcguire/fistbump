package cost

import (
	"regexp"
	"strings"
)

// Benefits are read from the posting's own words and nothing else. A benefit counts only when the posting offers
// it: under a benefits heading, or in a sentence that reads like an offer. A bare mention does not count, because
// "diversity, equity and inclusion", "our vision", "the health insurance industry" and "reasonable accommodations
// for a disability" are in postings that offer nothing of the kind. Where the text cannot be read as a list of
// benefits the result says so (rating "unclear" or "unlisted") and rates nothing.

type benefit struct {
	Label  string
	Weight int
	Core   bool
	// NeedsCue marks wording that is common outside an offer, so it counts only with a benefits heading or an offering
	// word in the same sentence. Paid time off and the like are unambiguous and count wherever they appear.
	NeedsCue bool
	Re       *regexp.Regexp
}

func b(label string, weight int, core, needsCue bool, pattern string) benefit {
	return benefit{label, weight, core, needsCue, regexp.MustCompile(`(?i)` + pattern)}
}

var benefitList = []benefit{
	// Health: the word alone is too common ("the healthcare industry"), so it needs insurance, coverage, a plan or benefits.
	b("Health insurance", 22, true, true, `\b(medical|health(?:care)?\s+(?:insurance|coverage|plans?|benefits)|health,\s+dental|health\s+and\s+(?:dental|wellness\s+benefits))\b`),
	b("Dental", 8, true, true, `\bdental\b`),
	// Vision is a company's aim far more often than an insurance line, so it needs a benefits word beside it.
	b("Vision", 6, true, true, `\bvision\s+(?:insurance|coverage|plans?|care|benefits)\b|\b(?:dental|medical)(?:,|,?\s+and|\s*&)\s+vision\b`),
	b("Retirement plan", 14, true, true, `\b(401\s?\(?k\)?|403\s?\(?b\)?|retirement\s+(?:plan|savings|benefits|contributions?)|pension|(?:roth\s+)?IRA\s+(?:plan|match|contribution))`),
	b("Paid time off", 12, true, false, `\b(paid\s+time\s+off|pto|paid\s+vacation|vacation\s+days|paid\s+leave|flexible\s+time\s+off|unlimited\s+(?:pto|vacation|time\s+off))\b`),
	b("Parental leave", 8, false, false, `\b(parental\s+leave|maternity\s+leave|paternity\s+leave|paid\s+family\s+leave|family\s+leave)\b`),
	b("Paid holidays", 4, false, false, `\bpaid\s+holidays?\b|\bcompany\s+holidays?\b`),
	// Equity must be pay, not a value: "diversity, equity and inclusion" says nothing about stock.
	b("Equity or stock", 6, false, true, `\b(stock\s+options?|stock\s+grants?|rsus?|espp|employee\s+stock|stock\s+purchase|equity\s+(?:compensation|grants?|awards?|package|options?|refresh|stake)|(?:competitive|meaningful|generous)\s+equity|equity\s+in\s+the\s+company)\b`),
	b("Bonus", 4, false, true, `\b(annual\s+bonus|performance\s+bonus|signing\s+bonus|sign-on\s+bonus|bonus\s+(?:plan|program|eligible)|profit\s+sharing)\b`),
	b("Learning budget", 4, false, true, `\b(tuition|professional\s+development|learning\s+(?:budget|stipend|allowance)|education\s+(?:assistance|reimbursement|benefit)|training\s+budget|certification\s+(?:reimbursement|support|assistance))\b`),
	b("Wellness", 3, false, true, `\b(wellness|mental\s+health\s+(?:support|benefits|coverage|days)|gym|fitness|employee\s+assistance)\b`),
	// Disability must be insurance: "reasonable accommodations for a disability" is in every equal-opportunity line.
	b("Life and disability insurance", 5, false, true, `\b(life\s+insurance|(?:short|long)-term\s+disability|disability\s+(?:insurance|coverage)|std\s+and\s+ltd|AD&D)\b`),
	b("Stipends and commuter", 4, false, true, `\b(stipend|commuter|home\s+office\s+(?:budget|allowance|stipend)|fsa|hsa|flexible\s+spending|health\s+savings)\b`),
}

var (
	// A line that is only a benefits heading ("Benefits", "What we offer:", "Perks & Benefits").
	headings = []string{"benefits", "perks", "what we offer", "total rewards", "compensation and benefits", "compensation & benefits",
		"benefits and perks", "benefits & perks", "perks and benefits", "perks & benefits", "our benefits", "what you'll get",
		"what you will get", "what's in it for you", "the perks", "employee benefits", "benefits summary", "rewards"}
	// Words that make a sentence an offer: "we offer", "benefits include", "you will receive". Only first-person and benefits wording
	// counts: "offers" or "provides" alone would also match a company that sells insurance.
	offerCue = regexp.MustCompile(`(?i)\b(we offer|we provide|we cover|we pay|benefits?|perks?|package|you(?:'ll| will) (?:get|receive|enjoy)|company-paid|employer-paid|eligible|for you and your family|for employees|available to employees)\b`)
	// Wording that says the posting talks about benefits even if it never lists any.
	mentionsBenefits = regexp.MustCompile(`(?i)\b(benefits package|benefits (?:and|&) perks|perks (?:and|&) benefits|(?:comprehensive|competitive|great|excellent|full|generous|robust|attractive) (?:benefits|perks)|our benefits|total rewards|compensation package|rewards package|perks|what we offer)\b`)
	sentenceSplit    = regexp.MustCompile(`[.\n!?•·]+`)
	matchWord        = regexp.MustCompile(`(?i)\b(?:401\s?\(?k\)?|403\s?\(?b\)?)[^.\n]{0,40}\bmatch|\bmatch(?:ing)?\b[^.\n]{0,40}\b401`)
)

// benefitsScope returns the part of the text to read for benefits: from the first benefits heading to the end when
// there is one, else the whole text. headed says which.
func benefitsScope(text string) (scope string, headed bool) {
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		clean := strings.ToLower(strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "#*_-•·:|>\t ")))
		for _, h := range headings {
			// "Benefits" alone on a line, or "Benefits: medical, dental..." with the list starting on the same line
			if clean != "" && (clean == h || strings.HasPrefix(clean, h+":")) {
				return text[offset:], true
			}
		}
		// "Our benefits include:" or "We offer:" opens a list too
		if clean != "" && len(clean) <= 80 && strings.HasSuffix(strings.TrimSpace(line), ":") && offerCue.MatchString(clean) {
			return text[offset:], true
		}
		offset += len(line)
	}
	return text, false
}

// Benefits is read from the posting's wording only.
type Benefits struct {
	Score    int      `json:"score"`  // 0 to 100
	Rating   string   `json:"rating"` // strong, decent, basic, unclear or unlisted
	Found    []string `json:"found"`
	Missing  []string `json:"missing"` // the core ones the posting does not mention
	Headline string   `json:"headline"`
}

func benefits(text string) Benefits {
	out := Benefits{Found: []string{}, Missing: []string{}}
	if strings.TrimSpace(text) == "" {
		out.Rating = "unlisted"
		out.Headline = "There is no posting text to read, so the benefits are unknown."
		return out
	}
	scope, headed := benefitsScope(text)
	sentences := sentenceSplit.Split(scope, -1)
	score := 0
	for _, f := range benefitList {
		hit := false
		for _, s := range sentences {
			if f.Re.MatchString(s) && (headed || !f.NeedsCue || offerCue.MatchString(s)) {
				hit = true
				break
			}
		}
		if !hit {
			if f.Core {
				out.Missing = append(out.Missing, f.Label)
			}
			continue
		}
		label := f.Label
		score += f.Weight
		if f.Label == "Retirement plan" && matchWord.MatchString(scope) {
			label, score = "Retirement plan with a match", score+6
		}
		out.Found = append(out.Found, label)
	}
	out.Score = min(score, 100)

	switch {
	case len(out.Found) == 0 && (headed || mentionsBenefits.MatchString(text)):
		// It talks about benefits but the wording is not one the app can read, so nothing is rated.
		out.Rating = "unclear"
		out.Missing = []string{}
		out.Headline = "The posting mentions benefits but does not say what they are, or says it in a way the app cannot read, so they are not rated. Check the original posting or ask the recruiter."
	case len(out.Found) == 0:
		out.Rating = "unlisted"
		out.Missing = []string{}
		out.Headline = "The posting does not describe benefits, so they are not rated. That does not mean there are none: ask the recruiter about health insurance, retirement matching and time off."
	case out.Score >= 65 && hasCore(out.Found, "Health insurance"):
		// Strong needs health coverage: a long list of perks does not make up for missing insurance.
		out.Rating = "strong"
		out.Headline = "Strong benefits: the posting lists " + list(out.Found, 4) + "."
	case out.Score >= 38:
		out.Rating = "decent"
		out.Headline = "Decent benefits: the posting lists " + list(out.Found, 4) + "."
	default:
		out.Rating = "basic"
		out.Headline = "Few benefits are listed: " + list(out.Found, 4) + ". The posting may not list them all."
	}
	if len(out.Found) > 0 && len(out.Missing) > 0 {
		out.Headline += " Not mentioned in the posting: " + strings.ToLower(list(out.Missing, 5)) + "."
	}
	return out
}
