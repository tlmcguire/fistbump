package greenhouse

import "strings"

// usStateCodes maps each state name to its postal code, so "South Carolina" finds "Charleston, SC" and the reverse.
var usStateCodes = map[string]string{
	"alabama": "al", "alaska": "ak", "arizona": "az", "arkansas": "ar", "california": "ca", "colorado": "co",
	"connecticut": "ct", "delaware": "de", "district of columbia": "dc", "florida": "fl", "georgia": "ga",
	"hawaii": "hi", "idaho": "id", "illinois": "il", "indiana": "in", "iowa": "ia", "kansas": "ks",
	"kentucky": "ky", "louisiana": "la", "maine": "me", "maryland": "md", "massachusetts": "ma",
	"michigan": "mi", "minnesota": "mn", "mississippi": "ms", "missouri": "mo", "montana": "mt",
	"nebraska": "ne", "nevada": "nv", "new hampshire": "nh", "new jersey": "nj", "new mexico": "nm",
	"new york": "ny", "north carolina": "nc", "north dakota": "nd", "ohio": "oh", "oklahoma": "ok",
	"oregon": "or", "pennsylvania": "pa", "rhode island": "ri", "south carolina": "sc", "south dakota": "sd",
	"tennessee": "tn", "texas": "tx", "utah": "ut", "vermont": "vt", "virginia": "va", "washington": "wa",
	"west virginia": "wv", "wisconsin": "wi", "wyoming": "wy",
}

// placeGroups are names people use for the same place. A term in a group also matches the others.
// Two-letter members are state-style codes and are matched as capitals.
var placeGroups = [][]string{
	{"new york", "new york city", "nyc", "new york, ny"},
	{"san francisco", "sf", "san fran", "bay area"},
	{"washington, dc", "dc", "district of columbia"},
	{"philadelphia", "philly"},
	{"atlanta", "atl"},
	{"los angeles", "la county"},
	{"boston", "greater boston"},
}

// bayArea is what "Bay Area" stands for in a posting location.
var bayArea = []string{"san francisco", "oakland", "san jose", "palo alto", "mountain view", "sunnyvale", "menlo park", "redwood city", "berkeley", "santa clara", "south san francisco"}

// expandTerms adds the other spellings of each term: a state's code or name, and the group names.
// The result is lower case without duplicates. A two-letter term is a state code.
func expandTerms(terms []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(t string) {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, t := range terms {
		add(t)
		if code, ok := usStateCodes[t]; ok {
			add(code)
		}
		for name, code := range usStateCodes {
			if code == t {
				add(name)
			}
		}
		for _, group := range placeGroups {
			for _, member := range group {
				if member == t {
					for _, other := range group {
						add(other)
					}
				}
			}
		}
		if t == "bay area" || t == "san francisco bay area" {
			for _, city := range bayArea {
				add(city)
			}
		}
	}
	return out
}

// codeLike reports whether a term is a two-letter code, which must match as capitals so that
// "or" in "Oakland or Remote" is not Oregon.
func codeLike(term string) bool {
	return len(term) == 2 && strings.ToLower(term) == term
}
