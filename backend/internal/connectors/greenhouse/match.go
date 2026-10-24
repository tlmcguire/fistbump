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
		"principal": true, "staff": true, "i": true, "ii": true, "iii": true,
	}
	// Multi-word spellings are folded first, then single words.
	phraseFolds = [][2]string{{"front end", "frontend"}, {"back end", "backend"}, {"full stack", "fullstack"}}
	wordFolds   = map[string]string{"developer": "engineer", "dev": "engineer", "swe": "engineer"}

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

// locationTerms splits the request's location into lower-case city, state or country terms.
func locationTerms(s string) []string {
	var out []string
	for _, p := range strings.Split(strings.ToLower(s), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// locationMatches reports whether any term appears in the posting location. Terms of two letters
// (state codes) must match a whole word, so "in" does not match "Berlin".
func locationMatches(location string, terms []string) bool {
	l := strings.ToLower(location)
	for _, t := range terms {
		if len(t) <= 2 {
			re := regexp.MustCompile(`\b` + regexp.QuoteMeta(t) + `\b`)
			if re.MatchString(l) {
				return true
			}
		} else if strings.Contains(l, t) {
			return true
		}
	}
	return false
}

// LocationOK applies the location filter. Remote postings pass unless the request is On-site;
// an empty location passes everything.
func LocationOK(location, mode string, terms []string, wantMode string) bool {
	if len(terms) == 0 {
		return true
	}
	if mode == connectors.WorkModeRemote && wantMode != connectors.WorkModeOnSite {
		return true
	}
	return locationMatches(location, terms)
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
