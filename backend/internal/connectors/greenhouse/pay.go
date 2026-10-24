package greenhouse

import (
	"regexp"
	"strconv"
	"strings"
)

const hoursPerYear = 2080

// Pieces of a pay figure: an optional currency mark, the number, an optional k, an optional currency code.
const (
	figure = `(CA\$|C\$|A\$|AU\$|NZ\$|US\$|\$)\s?(\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?)\s?([kK])?(?:\s?(USD|CAD|AUD|NZD))?`
	second = `(\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?)\s?([kK])?(?:\s?(USD|CAD|AUD|NZD))?`
)

// A range as written in a posting: "$120,000 - $150,000", "$60k to $75k", "$120,000 USD - $150,000 USD".
var payRange = regexp.MustCompile(figure + `\s*(?:-|–|—|to)\s*(?:\$\s?)?` + second)

// One figure with a yearly or hourly unit: "$150,000 per year", "$55/hour".
var paySingle = regexp.MustCompile(figure + `\s*(?:per year|a year|annually|per annum|/\s?yr|/\s?year|per hour|an hour|/\s?hr|/\s?hour|hourly)`)

var (
	hourlyWord = regexp.MustCompile(`(?i)(per hour|an hour|/\s?hr|/\s?hour|hourly)`)
	// A figure is foreign when a currency code follows it, or the words just before it name another country.
	foreignBefore = regexp.MustCompile(`(?i)\b(cad|aud|nzd|canada|canadian|australia|australian)\b|\b(ca|c|a|au|nz)\$`)
	foreignAfter  = regexp.MustCompile(`(?i)^[\s.,;)]{0,3}(cad|aud|nzd)\b`)
)

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

// ParsePay reads the yearly USD pay range from a description. A posting that lists several ranges
// (one per region, say) gives the lowest start and the highest end. Hourly pay is converted to a
// year. Amounts in other currencies, and numbers that are not salaries, are skipped. It returns
// false when no sensible range is written.
func ParsePay(text string) (low, high int, ok bool) {
	take := func(lo, hi float64, unitText string) {
		if hourlyWord.MatchString(unitText) {
			lo, hi = lo*hoursPerYear, hi*hoursPerYear
		}
		if lo > hi || lo < 15_000 || hi > 2_000_000 {
			return // not a salary: a bonus, a revenue figure, or an hourly rate we cannot place
		}
		if !ok || int(lo) < low {
			low = int(lo)
		}
		if int(hi) > high {
			high = int(hi)
		}
		ok = true
	}
	// Foreign marks are looked for in the figure and a few words around it.
	foreign := func(start, end int) bool {
		return foreignBefore.MatchString(text[max(start-24, 0):end]) || foreignAfter.MatchString(text[end:min(end+8, len(text))])
	}
	var spans [][2]int
	for _, m := range payRange.FindAllStringSubmatchIndex(text, -1) {
		get := func(i int) string {
			if m[2*i] < 0 {
				return ""
			}
			return text[m[2*i]:m[2*i+1]]
		}
		spans = append(spans, [2]int{m[0], m[1]})
		if foreign(m[0], m[1]) || get(1) != "$" && get(1) != "US$" {
			continue
		}
		lo, hi := amount(get(2), get(3)), amount(get(5), get(6))
		// "$100 - 150k": the first number shares the second's k.
		if get(3) == "" && get(6) != "" && lo < 1000 {
			lo *= 1000
		}
		take(lo, hi, text[m[1]:min(m[1]+40, len(text))])
	}
	for _, m := range paySingle.FindAllStringSubmatchIndex(text, -1) {
		inRange := false
		for _, sp := range spans {
			inRange = inRange || m[0] >= sp[0] && m[0] < sp[1]
		}
		if inRange || foreign(m[0], m[1]) || text[m[2]:m[3]] != "$" && text[m[2]:m[3]] != "US$" {
			continue
		}
		var k string
		if m[6] >= 0 {
			k = text[m[6]:m[7]]
		}
		v := amount(text[m[4]:m[5]], k)
		take(v, v, text[m[5]:m[1]])
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
