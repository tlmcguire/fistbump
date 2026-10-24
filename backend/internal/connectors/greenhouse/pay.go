package greenhouse

import (
	"regexp"
	"strconv"
	"strings"
)

const hoursPerYear = 2080

// A pay range as written in a posting: "$120,000 - $150,000", "$60k to $75k", "$45 - $55 per hour".
var payRange = regexp.MustCompile(`\$\s?(\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?)\s?([kK])?\s*(?:-|–|—|to)\s*\$?\s?(\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?)\s?([kK])?`)

var hourlyWord = regexp.MustCompile(`(?i)(per hour|an hour|/\s?hr|/\s?hour|hourly)`)

func amount(num, k string) float64 {
	v, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", ""), 64)
	if err != nil {
		return 0
	}
	if k != "" {
		v *= 1000
	}
	return v
}

// ParsePay reads the yearly pay range from a description. A posting that lists several ranges
// (one per region, say) gives the lowest start and the highest end. Hourly pay is converted to a
// year. It returns false when no sensible range is written.
func ParsePay(text string) (low, high int, ok bool) {
	for _, m := range payRange.FindAllStringSubmatchIndex(text, -1) {
		get := func(i int) string {
			if m[2*i] < 0 {
				return ""
			}
			return text[m[2*i]:m[2*i+1]]
		}
		lo, hi := amount(get(1), get(2)), amount(get(3), get(4))
		// "$100 - 150k": the first number shares the second's k.
		if get(2) == "" && get(4) != "" && lo < 1000 {
			lo *= 1000
		}
		after := text[m[1]:min(m[1]+40, len(text))]
		if hourlyWord.MatchString(after) {
			lo, hi = lo*hoursPerYear, hi*hoursPerYear
		}
		if lo > hi || lo < 15_000 || hi > 2_000_000 {
			continue // not a salary: a bonus, a revenue figure, a year or an hourly rate we cannot place
		}
		if !ok || int(lo) < low {
			low = int(lo)
		}
		if int(hi) > high {
			high = int(hi)
		}
		ok = true
	}
	return low, high, ok
}

// PayOK reports whether a posting's pay fits the wanted yearly range. The posting's range must
// overlap it. A posting with no listed pay passes unless requirePay is set.
func PayOK(payMin, payMax, wantMin, wantMax int, requirePay bool) bool {
	if payMax == 0 {
		return !requirePay
	}
	if wantMin > 0 && payMax < wantMin {
		return false
	}
	if wantMax > 0 && payMin > wantMax {
		return false
	}
	return true
}
