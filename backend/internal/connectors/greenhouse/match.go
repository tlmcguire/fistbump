package greenhouse

import (
	"regexp"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
)

const minTitleScore = 0.5

var (
	nonWord = regexp.MustCompile(`[^a-z0-9+#]+`)

	// seniority words say how senior a role is, not what it is. "intern" is kept: it changes the job.
	seniority = map[string]bool{
		"senior": true, "sr": true, "junior": true, "jr": true, "lead": true,
		"principal": true, "staff": true,
	}
	// Multi-word spellings are folded first, then single words.
	phraseFolds = [][2]string{{"front end", "frontend"}, {"back end", "backend"}, {"full stack", "fullstack"}}
	// Roman numerals fold to digits, so "SOC Analyst I" matches "soc analyst 1".
	wordFolds = map[string]string{"developer": "engineer", "dev": "engineer", "swe": "engineer", "i": "1", "ii": "2", "iii": "3", "iv": "4"}

	remoteRe = regexp.MustCompile(`\bremote\b`)
	hybridRe = regexp.MustCompile(`\bhybrid\b`)
)

// titleWords lower-cases text, folds synonyms and drops seniority words.
func titleWords(s string) []string {
	s = " " + strings.TrimSpace(nonWord.ReplaceAllString(strings.ToLower(s), " ")) + " "
	for _, f := range phraseFolds {
		s = strings.ReplaceAll(s, " "+f[0]+" ", " "+f[1]+" ")
	}
	var out []string
	for _, w := range strings.Fields(s) {
		if seniority[w] {
			continue
		}
		if v, ok := wordFolds[w]; ok {
			w = v
		}
		out = append(out, w)
	}
	return out
}

// TitleScore is the best score over the keywords, 0 to 1. A keyword scores 1 when its words appear
// in the title as a phrase, else the fraction of its words found anywhere in the title.
func TitleScore(title string, keywords []string) float64 {
	tw := titleWords(title)
	set := make(map[string]bool, len(tw))
	for _, w := range tw {
		set[w] = true
	}
	joined := " " + strings.Join(tw, " ") + " "
	best := 0.0
	for _, k := range keywords {
		kw := titleWords(k)
		if len(kw) == 0 {
			continue
		}
		if strings.Contains(joined, " "+strings.Join(kw, " ")+" ") {
			return 1
		}
		hit := 0
		for _, w := range kw {
			if set[w] {
				hit++
			}
		}
		best = max(best, float64(hit)/float64(len(kw)))
	}
	return best
}

// WorkMode reads the mode from the location and title. Remote and Hybrid must be stated.
func WorkMode(location, title string) string {
	text := strings.ToLower(location + " " + title)
	switch {
	case remoteRe.MatchString(text):
		return connectors.WorkModeRemote
	case hybridRe.MatchString(text):
		return connectors.WorkModeHybrid
	}
	return connectors.WorkModeOnSite
}

// ModeOK applies the requested work mode. Remote keeps only Remote. On-site and Hybrid keep their
// own mode plus postings that say neither Remote nor Hybrid (which are treated as On-site).
func ModeOK(mode, want string) bool {
	switch want {
	case "", connectors.WorkModeAny:
		return true
	case connectors.WorkModeRemote:
		return mode == connectors.WorkModeRemote
	}
	return mode == want || (mode == connectors.WorkModeOnSite && want == connectors.WorkModeHybrid)
}

// locationTerms splits the request's location into lower-case terms (city, state, country) and
// adds their other spellings, so "South Carolina" also looks for "SC".
func locationTerms(s string) []string {
	var out []string
	for _, p := range strings.Split(strings.ToLower(s), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	// "Charleston, South Carolina" is also worth trying as one name, for groups like "new york, ny".
	if len(out) > 1 {
		out = append(out, strings.Join(out, ", "))
	}
	return expandTerms(out)
}

// locationMatches reports whether any term appears in the posting location. Two-letter terms are
// state codes and must match a whole word in capitals, so "in" does not match "Berlin" and "or" in
// "Oakland or Remote" is not Oregon.
func locationMatches(location string, terms []string) bool {
	l := strings.ToLower(location)
	for _, t := range terms {
		if codeLike(t) {
			if capitalWord(location, strings.ToUpper(t)) {
				return true
			}
		} else if strings.Contains(l, t) {
			return true
		}
	}
	return false
}

// capitalWord reports whether word appears in s as a whole word, in this case.
func capitalWord(s, word string) bool {
	for i := 0; ; {
		at := strings.Index(s[i:], word)
		if at < 0 {
			return false
		}
		start, end := i+at, i+at+len(word)
		before := start == 0 || !isLetter(s[start-1])
		after := end == len(s) || !isLetter(s[end])
		if before && after {
			return true
		}
		i = start + 1
	}
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// LocationOK applies the location filter. A remote posting passes unless the request is On-site,
// or unless it names a country the user is not in (so "Remote - Australia" is dropped for a US
// location). An empty location passes everything.
func LocationOK(location, mode string, terms []string, wantMode string) bool {
	if len(terms) == 0 {
		return true
	}
	if locationMatches(location, terms) {
		return true
	}
	return mode == connectors.WorkModeRemote && wantMode != connectors.WorkModeOnSite && !abroad(location, terms)
}

var (
	usStates = regexp.MustCompile(`^(al|ak|az|ar|ca|co|ct|de|dc|fl|ga|hi|id|il|in|ia|ks|ky|la|me|md|ma|mi|mn|ms|mo|mt|ne|nv|nh|nj|nm|ny|nc|nd|oh|ok|or|pa|ri|sc|sd|tn|tx|ut|vt|va|wa|wv|wi|wy)$`)
	usNames  = regexp.MustCompile(`\b(united states|usa|u\.s\.|america|alabama|alaska|arizona|arkansas|california|colorado|connecticut|delaware|florida|georgia|hawaii|idaho|illinois|indiana|iowa|kansas|kentucky|louisiana|maine|maryland|massachusetts|michigan|minnesota|mississippi|missouri|montana|nebraska|nevada|new hampshire|new jersey|new mexico|new york|north carolina|north dakota|ohio|oklahoma|oregon|pennsylvania|rhode island|south carolina|south dakota|tennessee|texas|utah|vermont|virginia|washington|west virginia|wisconsin|wyoming)\b`)
	usWord   = regexp.MustCompile(`\b(us|usa|united states|u\.s\.|north america|americas)\b|\bus-`)
	// Countries and regions that mark a remote posting as not for a US-based user.
	foreignRe = regexp.MustCompile(`\b(australia|new zealand|canada|mexico|brazil|argentina|colombia|costa rica|chile|peru|uk|united kingdom|ireland|england|scotland|germany|france|spain|portugal|italy|netherlands|belgium|poland|sweden|norway|denmark|finland|switzerland|austria|czech|romania|bulgaria|hungary|ukraine|turkey|israel|egypt|nigeria|kenya|south africa|india|pakistan|bangladesh|sri lanka|china|japan|korea|singapore|malaysia|indonesia|philippines|thailand|vietnam|taiwan|hong kong|uae|dubai|emea|apac|latam|europe|eu|asia|africa|oceania)\b`)
)

// inUS reports whether the requested location terms put the user in the United States.
func inUS(terms []string) bool {
	for _, t := range terms {
		if usStates.MatchString(t) || usNames.MatchString(t) || usWord.MatchString(t) {
			return true
		}
	}
	return false
}

// abroad reports whether a posting location names a foreign country and nothing that says US, for a
// user in the US. Other users are not filtered: the posting passes.
func abroad(location string, terms []string) bool {
	if !inUS(terms) {
		return false
	}
	l := strings.ToLower(location)
	return foreignRe.MatchString(l) && !usWord.MatchString(l) && !usNames.MatchString(l)
}

// LocationScore is 1 for a location match, 0.5 for remote and 0 otherwise.
func LocationScore(location, mode string, terms []string) float64 {
	if len(terms) > 0 && locationMatches(location, terms) {
		return 1
	}
	if mode == connectors.WorkModeRemote {
		return 0.5
	}
	return 0
}

// Score combines title fit and location. Phase 4 adds skill fit and re-weights.
func Score(titleScore, locationScore float64) float64 {
	return round2(0.7*titleScore + 0.3*locationScore)
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

var (
	internRe = regexp.MustCompile(`\b(intern|internship|co-op|apprentice)\b`)
	entryRe  = regexp.MustCompile(`\b(junior|jr|entry|entry-level|graduate|new grad)\b|\bassociate\b`)
	seniorRe = regexp.MustCompile(`\b(senior|sr|iii|iv)\b`)
	// A manager is a mid-level role unless the title says more: an engineering manager leads a team.
	leadRe = regexp.MustCompile(`\b(lead|principal|staff|director|head|vp|vice president|chief|associate director|associate vice president|associate general counsel|engineering manager|senior manager|group manager|general manager|managing)\b`)

	describedRemote = regexp.MustCompile(`(?i)\b(fully remote|100% remote|remote[- ]first|this is a remote|remote position|remote role|remote opportunity|work from anywhere|work remotely from)\b`)
	describedHybrid = regexp.MustCompile(`(?i)\b(hybrid (role|position|work|schedule|model|environment)|this is a hybrid|\d days? (a|per) week (in|at) the office|in[- ]office \d days?)\b`)
)

// Level reads the experience level from a title. A title that says none is Mid.
func Level(title string) string {
	t := strings.ToLower(nonWordKeepDash.ReplaceAllString(title, " "))
	switch {
	case internRe.MatchString(t):
		return connectors.LevelIntern
	case leadRe.MatchString(t):
		return connectors.LevelLead
	case seniorRe.MatchString(t):
		return connectors.LevelSenior
	case entryRe.MatchString(t):
		return connectors.LevelEntry
	}
	return connectors.LevelMid
}

var nonWordKeepDash = regexp.MustCompile(`[^A-Za-z0-9 -]+`)

// DescribedMode reads a stricter work mode from a description: "fully remote", "hybrid role" and the
// like. It returns "" when the text does not say, since benefits lists mention "remote" in passing.
func DescribedMode(text string) string {
	switch {
	case describedRemote.MatchString(text):
		return connectors.WorkModeRemote
	case describedHybrid.MatchString(text):
		return connectors.WorkModeHybrid
	}
	return ""
}

// LevelOK applies the requested level.
func LevelOK(level, want string) bool {
	return want == "" || want == connectors.LevelAny || level == want
}

// ExcludeOK is false when the title contains any of the words, as whole words or phrases.
func ExcludeOK(title string, words []string) bool {
	t := " " + strings.Join(titleFields(title), " ") + " "
	for _, w := range words {
		f := titleFields(w)
		if len(f) > 0 && strings.Contains(t, " "+strings.Join(f, " ")+" ") {
			return false
		}
	}
	return true
}

// titleFields lower-cases and splits text into words without folding synonyms or dropping any.
func titleFields(s string) []string {
	return strings.Fields(nonWord.ReplaceAllString(strings.ToLower(s), " "))
}
