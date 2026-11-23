// Package cost judges a job's pay and benefits against the cost of living where the user would live.
// It is deterministic and local: bundled reference figures, no network and no model. The numbers are
// rounded estimates for one adult, so the results are guidance, not a quote.
package cost

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// Input is one posting and what is known about the user.
type Input struct {
	Title         string
	Location      string
	WorkMode      string // Remote, Hybrid, On-site or empty
	PayMin        int64  // yearly USD from the posting; 0 when it lists none
	PayMax        int64
	Text          string // the posting's description, read for benefits
	HomeLocation  string // the user's own place, where a remote job would be done from
	MinDesiredPay int64
}

// Result is the whole analysis. A section that cannot be judged is nil.
type Result struct {
	Supported bool       `json:"supported"` // false for places outside the US, where there are no figures
	Location  Location   `json:"location"`
	Pay       *Pay       `json:"pay"`
	Benefits  Benefits   `json:"benefits"`
	Lifestyle *Lifestyle `json:"lifestyle"`
	Notes     []string   `json:"notes"`
	Summary   string     `json:"summary"`
}

// Location is the place the costs are worked out for, and why that place.
type Location struct {
	Label   string  `json:"label"`
	Basis   string  `json:"basis"` // job, home or national
	Index   float64 `json:"index"` // cost of living, US average = 100
	Rent    int     `json:"rent"`  // typical one-bedroom rent per month
	Precise bool    `json:"precise"`
}

// Pay says whether the pay is competitive for the role and the place.
type Pay struct {
	Listed       bool   `json:"listed"` // the posting states its pay
	Min          int64  `json:"min"`
	Max          int64  `json:"max"`
	Mid          int64  `json:"mid"`    // the figure the rest of the analysis uses
	Source       string `json:"source"` // posting, market or desired
	Role         string `json:"role"`
	Level        string `json:"level"`
	MarketLow    int64  `json:"market_low"` // 0 when the role is not recognized
	MarketMid    int64  `json:"market_mid"`
	MarketHigh   int64  `json:"market_high"`
	DiffPercent  int    `json:"diff_percent"` // posted midpoint against the market midpoint
	Verdict      string `json:"verdict"`      // well_above, above, in_line, below, well_below or unknown
	Headline     string `json:"headline"`
	MeetsDesired *bool  `json:"meets_desired"` // against the user's minimum desired pay, when they set one
}

// Line is one monthly amount.
type Line struct {
	Label  string `json:"label"`
	Amount int    `json:"amount"`
}

// Period is the pay over one stretch of time, before and after tax.
type Period struct {
	Label string `json:"label"`
	Gross int    `json:"gross"`
	Net   int    `json:"net"`
}

// Lifestyle works out whether the pay covers living there and what is left.
type Lifestyle struct {
	Gross        int64    `json:"gross"`
	Federal      int64    `json:"federal"`
	State        int64    `json:"state"`
	Payroll      int64    `json:"payroll"`
	NetMonthly   int      `json:"net_monthly"`
	GrossMonthly int      `json:"gross_monthly"`
	TaxMonthly   int      `json:"tax_monthly"` // federal, state and local income tax plus Social Security and Medicare
	TaxRate      int      `json:"tax_rate"`    // percent of gross pay that goes to those taxes
	Periods      []Period `json:"periods"`     // the pay by the year, month, two weeks, week, day and hour
	Costs        []Line   `json:"costs"`
	CostMonthly  int      `json:"cost_monthly"`
	LeftMonthly  int      `json:"left_monthly"`
	LeftAtMin    int      `json:"left_at_min"`  // the same, at the bottom of the posted range
	SavingsRate  int      `json:"savings_rate"` // percent of take-home left over
	RentShare    int      `json:"rent_share"`   // percent of gross pay that goes to rent
	Verdict      string   `json:"verdict"`      // comfortable, okay, tight or short
	Headline     string   `json:"headline"`
}

var (
	stateCode   = regexp.MustCompile(`\b([A-Z]{2})\b`)
	remoteWord  = regexp.MustCompile(`(?i)\bremote\b`)
	foreignWord = regexp.MustCompile(`(?i)\b(canada|toronto|vancouver|montreal|ontario|british columbia|quebec|alberta|uk|united kingdom|london|england|scotland|ireland|dublin|germany|berlin|munich|france|paris|spain|madrid|barcelona|portugal|lisbon|italy|netherlands|amsterdam|belgium|poland|sweden|stockholm|norway|denmark|copenhagen|finland|switzerland|zurich|austria|czech|romania|bulgaria|hungary|ukraine|turkey|israel|tel aviv|egypt|nigeria|kenya|south africa|india|bangalore|bengaluru|hyderabad|pune|mumbai|pakistan|china|beijing|shanghai|japan|tokyo|korea|seoul|singapore|malaysia|indonesia|philippines|thailand|vietnam|taiwan|hong kong|uae|dubai|emea|apac|latam|europe|asia|africa|australia|sydney|melbourne|new zealand|mexico|brazil|sao paulo|argentina|colombia|costa rica|chile|peru)\b`)
)

func words(s string) string {
	return " " + strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.')
	}), " ") + " "
}

// containsAlias reports whether text has the alias as whole words.
func containsAlias(text, alias string) bool {
	return strings.Contains(words(text), words(alias))
}

// resolve finds the place a location names: a city first, then a state. The first part of a list
// ("Seattle, WA; Austin, TX" or "New York or Remote") that names a place wins.
func resolve(loc string) (place, bool, bool) {
	for _, part := range regexp.MustCompile(`[;|/]| or | and `).Split(loc, -1) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var best place
		bestLen := 0
		for _, m := range metros {
			names := m.Aliases
			if m.State != "DC" {
				names = append([]string{strings.TrimSuffix(strings.ToLower(m.Name), ", "+strings.ToLower(m.State))}, names...)
			}
			for _, a := range names {
				if len(a) > bestLen && containsAlias(part, a) && stateAgrees(part, m.State) {
					best, bestLen = m, len(a)
				}
			}
		}
		if bestLen > 0 {
			return withTax(best), true, true
		}
		if st, ok := findState(part); ok {
			return st, true, false
		}
	}
	return place{}, false, false
}

// stateAgrees rejects a city match when the text names a different state ("Portland, ME" is not Portland, OR).
func stateAgrees(part, state string) bool {
	// a state written out beats a lookalike ("Washington, DC" is not the state)
	if code := stateCode.FindString(part); code != "" {
		for _, s := range states {
			if s.State == code {
				return code == state
			}
		}
	}
	for _, s := range states {
		if s.Name != "Washington" && containsAlias(part, s.Name) && s.State != state {
			return s.Name == "New York" && state == "NY"
		}
	}
	return true
}

func findState(part string) (place, bool) {
	if code := stateCode.FindString(part); code != "" {
		for _, s := range states {
			if s.State == code {
				return s, true
			}
		}
	}
	// the longest name wins, so "West Virginia" is not Virginia
	var best place
	for _, s := range states {
		if len(s.Name) > len(best.Name) && containsAlias(part, s.Name) {
			best = s
		}
	}
	return best, best.Name != ""
}

// namesUS reports whether a location says it is in the United States: a state code or name, or the country.
func namesUS(loc string) bool {
	if _, ok := findState(loc); ok {
		return true
	}
	return usWord.MatchString(loc)
}

var usWord = regexp.MustCompile(`(?i)\b(united states|usa|u\.s\.a?\.?)\b|\bUS\b|\bUS-`)

// withTax fills a metro's state income tax from its state when the metro does not set one.
func withTax(m place) place {
	if m.StateTax == 0 {
		for _, s := range states {
			if s.State == m.State {
				m.StateTax = s.StateTax
			}
		}
	}
	return m
}

// livingPlace chooses where the user would live: the job's place for a hybrid or on-site job, the
// user's own place for a remote one.
func livingPlace(in Input) (place, Location, bool) {
	// A location in another country has no figures, even when a city there shares a name with one here ("Cambridge, UK").
	if in.Location != "" && foreignWord.MatchString(in.Location) && !namesUS(in.Location) {
		return national, Location{}, false
	}
	remote := strings.EqualFold(in.WorkMode, "Remote") || (in.WorkMode == "" && remoteWord.MatchString(in.Location))
	if !remote {
		if p, ok, precise := resolve(in.Location); ok {
			return p, Location{Label: p.Name, Basis: "job", Index: p.Index, Rent: p.Rent, Precise: precise}, true
		}
	}
	if p, ok, precise := resolve(in.HomeLocation); ok {
		return p, Location{Label: p.Name, Basis: "home", Index: p.Index, Rent: p.Rent, Precise: precise}, true
	}
	n := national
	return n, Location{Label: n.Name, Basis: "national", Index: n.Index, Rent: n.Rent}, true
}

// Analyze runs the whole analysis for one posting.
func Analyze(in Input) Result {
	res := Result{Supported: true, Notes: []string{}}
	p, loc, ok := livingPlace(in)
	if !ok {
		res.Supported = false
		res.Benefits = benefits(in.Text)
		res.Notes = append(res.Notes, "The cost of living figures cover the United States only, so pay and lifestyle are not judged for this location.")
		res.Summary = "This posting is outside the United States, where there are no cost of living figures."
		return res
	}
	res.Location = loc
	switch loc.Basis {
	case "home":
		res.Notes = append(res.Notes, "This job is remote, so costs use where you live: "+loc.Label+".")
	case "national":
		if strings.EqualFold(in.WorkMode, "Remote") || remoteWord.MatchString(in.Location) {
			res.Notes = append(res.Notes, "This job is remote and your home location is not set, so costs use the US average. Add your location in your profile for a closer estimate.")
		} else {
			res.Notes = append(res.Notes, "The location is not specific enough to find its costs, so the US average is used.")
		}
	default:
		if !loc.Precise {
			res.Notes = append(res.Notes, "Costs use the average for the state, not the city.")
		}
	}

	res.Pay = judgePay(in, loc)
	res.Benefits = benefits(in.Text)
	if res.Pay != nil && res.Pay.Mid > 0 {
		res.Lifestyle = lifestyle(res.Pay, p, loc, res.Benefits, in.PayMin > 0)
	}
	res.Summary = summarize(res)
	return res
}

func detectLevel(title string) string {
	t := words(title)
	has := func(ws ...string) bool {
		for _, w := range ws {
			if strings.Contains(t, " "+w+" ") {
				return true
			}
		}
		return false
	}
	switch {
	case has("intern", "internship", "co-op", "apprentice"):
		return "Intern"
	case has("staff", "principal", "lead", "distinguished", "head"):
		return "Lead"
	case has("senior", "sr", "sr."):
		return "Senior"
	case has("junior", "jr", "jr.", "entry", "associate", "graduate", "grad", "i", "1", "tier 1", "new"):
		return "Entry"
	}
	return "Mid"
}

func findRole(title string) (role, bool) {
	t := strings.ToLower(title)
	for _, r := range roles {
		for _, w := range r.Words {
			if len(w) <= 3 {
				if strings.Contains(words(t), " "+w+" ") {
					return r, true
				}
			} else if strings.Contains(t, w) {
				return r, true
			}
		}
	}
	return role{}, false
}

// payFactor adjusts a national salary to a place. Pay follows living costs only partly.
func payFactor(index float64) float64 {
	return math.Min(math.Max(1+0.55*(index/100-1), 0.82), 1.5)
}

func round500(v float64) int64 { return int64(math.Round(v/500) * 500) }

func judgePay(in Input, loc Location) *Pay {
	pay := &Pay{Verdict: "unknown", Level: detectLevel(in.Title)}
	if in.PayMax == 0 {
		in.PayMax = in.PayMin
	}
	if in.PayMin == 0 {
		in.PayMin = in.PayMax
	}
	if in.PayMin > 0 {
		pay.Listed, pay.Min, pay.Max, pay.Mid, pay.Source = true, in.PayMin, in.PayMax, (in.PayMin+in.PayMax)/2, "posting"
	}
	if r, ok := findRole(in.Title); ok {
		pay.Role = r.Name
		f := payFactor(loc.Index)
		if r.Ladders {
			f *= levelFactor[pay.Level]
		}
		mid := float64(r.Median) * f
		pay.MarketLow, pay.MarketMid, pay.MarketHigh = round500(mid*0.85), round500(mid), round500(mid*1.15)
	}
	if !pay.Listed {
		switch {
		case pay.MarketMid > 0:
			pay.Mid, pay.Source = pay.MarketMid, "market"
		case in.MinDesiredPay > 0:
			pay.Mid, pay.Source = in.MinDesiredPay, "desired"
		default:
			return nil
		}
	}
	if in.MinDesiredPay > 0 && pay.Listed {
		meets := pay.Max >= in.MinDesiredPay
		pay.MeetsDesired = &meets
	}

	where := loc.Label
	switch {
	case pay.Listed && pay.MarketMid > 0:
		diff := float64(pay.Mid)/float64(pay.MarketMid) - 1
		pay.DiffPercent = int(math.Round(diff * 100))
		switch {
		case diff >= 0.15:
			pay.Verdict = "well_above"
		case diff >= 0.05:
			pay.Verdict = "above"
		case diff > -0.07:
			pay.Verdict = "in_line"
		case diff > -0.17:
			pay.Verdict = "below"
		default:
			pay.Verdict = "well_below"
		}
		label := map[string]string{"well_above": "Well above market", "above": "Above market", "in_line": "In line with the market", "below": "Below market", "well_below": "Well below market"}[pay.Verdict]
		pay.Headline = fmt.Sprintf("%s: the midpoint is %s, about %d%% %s the typical %s for a %s role in %s (%s to %s).",
			label, money(pay.Mid), abs(pay.DiffPercent), aboveBelow(pay.DiffPercent), money(pay.MarketMid), strings.ToLower(pay.Level), where, money(pay.MarketLow), money(pay.MarketHigh))
	case pay.Listed:
		pay.Headline = "This role is not one the app has a market figure for, so its pay cannot be compared with the market."
	case pay.MarketMid > 0:
		pay.Headline = fmt.Sprintf("The posting lists no pay. A typical %s %s role in %s earns about %s (%s to %s); the lifestyle estimate uses that.",
			strings.ToLower(pay.Level), strings.ToLower(pay.Role), where, money(pay.MarketMid), money(pay.MarketLow), money(pay.MarketHigh))
	default:
		pay.Headline = "The posting lists no pay and the role has no market figure, so the estimate uses your minimum desired pay."
	}
	return pay
}

func aboveBelow(d int) string {
	if d < 0 {
		return "below"
	}
	return "above"
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func money(v int64) string {
	s := fmt.Sprint(v)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return "$" + s
}

// 2026 US federal figures for a single filer. Rounded and simplified: no credits, no pre-tax deductions.
var brackets = []struct {
	Up   float64
	Rate float64
}{{12400, .10}, {50400, .12}, {105700, .22}, {201775, .24}, {256225, .32}, {640600, .35}, {math.MaxFloat64, .37}}

const (
	standardDeduction = 16100.0
	socialSecurityCap = 184500.0
)

func federalTax(gross float64) float64 {
	taxable := math.Max(gross-standardDeduction, 0)
	tax, prev := 0.0, 0.0
	for _, b := range brackets {
		if taxable > prev {
			tax += (math.Min(taxable, b.Up) - prev) * b.Rate
		}
		prev = b.Up
	}
	return tax
}

func payroll(gross float64) float64 {
	tax := math.Min(gross, socialSecurityCap)*0.062 + gross*0.0145
	if gross > 200000 {
		tax += (gross - 200000) * 0.009
	}
	return tax
}

func netMonthly(gross float64, p place) (net, fed, state, fica float64) {
	fed, fica = federalTax(gross), payroll(gross)
	state = math.Max(gross-standardDeduction, 0)*p.StateTax + gross*p.CityTax
	return (gross - fed - fica - state) / 12, fed, state, fica
}

// monthlyCosts is a modest single-adult budget at the place's cost of living. Health spending depends on
// whether the posting lists health insurance.
func monthlyCosts(loc Location, insured bool) []Line {
	idx := loc.Index / 100
	health := 450
	if insured {
		health = 180
	}
	return []Line{
		{"Rent (one bedroom)", loc.Rent},
		{"Utilities and internet", int(math.Round(210 * (0.5 + 0.5*idx)))},
		{"Groceries", int(math.Round(420 * (0.4 + 0.6*idx)))},
		{"Transportation", int(math.Round(450 * (0.6 + 0.4*idx)))},
		{"Health care", health},
		{"Dining, fun and personal", int(math.Round(450 * (0.5 + 0.5*idx)))},
		{"Phone, clothes and other", 250},
	}
}

func lifestyle(pay *Pay, p place, loc Location, b Benefits, listed bool) *Lifestyle {
	insured := false
	for _, f := range b.Found {
		if f == "Health insurance" {
			insured = true
		}
	}
	costs := monthlyCosts(loc, insured)
	total := 0
	for _, c := range costs {
		total += c.Amount
	}
	net, fed, state, fica := netMonthly(float64(pay.Mid), p)
	l := &Lifestyle{Gross: pay.Mid, Federal: int64(fed), State: int64(state), Payroll: int64(fica),
		NetMonthly: int(net), Costs: costs, CostMonthly: total, LeftMonthly: int(net) - total}
	l.GrossMonthly = int(math.Round(float64(pay.Mid) / 12))
	l.TaxMonthly = int(math.Round((fed + state + fica) / 12))
	l.TaxRate = int(math.Round((fed + state + fica) / float64(pay.Mid) * 100))
	netYear := float64(pay.Mid) - fed - state - fica
	for _, p := range []struct {
		label string
		per   float64
	}{{"Per year", 1}, {"Per month", 12}, {"Every two weeks", 26}, {"Per week", 52}, {"Per day", 260}, {"Per hour", 2080}} {
		l.Periods = append(l.Periods, Period{p.label, int(math.Round(float64(pay.Mid) / p.per)), int(math.Round(netYear / p.per))})
	}
	if net > 0 {
		l.SavingsRate = int(math.Round(float64(l.LeftMonthly) / net * 100))
	}
	l.RentShare = int(math.Round(float64(loc.Rent*12) / float64(pay.Mid) * 100))
	l.LeftAtMin = l.LeftMonthly
	if listed && pay.Min > 0 && pay.Min != pay.Mid {
		n, _, _, _ := netMonthly(float64(pay.Min), p)
		l.LeftAtMin = int(n) - total
	}
	switch {
	case l.LeftMonthly < 0:
		l.Verdict = "short"
		l.Headline = fmt.Sprintf("Not enough: this pay falls about %s a month short of a modest life in %s.", money(int64(-l.LeftMonthly)), loc.Label)
	case l.SavingsRate < 8:
		l.Verdict = "tight"
		l.Headline = fmt.Sprintf("Tight: after a modest budget, about %s a month is left. Little room to save or absorb a surprise.", money(int64(l.LeftMonthly)))
	case l.SavingsRate < 22:
		l.Verdict = "okay"
		l.Headline = fmt.Sprintf("Workable: about %s a month is left (%d%% of take-home) for savings and extras.", money(int64(l.LeftMonthly)), l.SavingsRate)
	default:
		l.Verdict = "comfortable"
		l.Headline = fmt.Sprintf("Comfortable: about %s a month is left (%d%% of take-home) after a modest budget, enough to save steadily and enjoy life.", money(int64(l.LeftMonthly)), l.SavingsRate)
	}
	if pay.Source != "posting" {
		l.Headline += " This uses an estimated salary, since the posting lists none."
	}
	return l
}

func list(items []string, most int) string {
	if len(items) > most {
		items = items[:most]
	}
	switch len(items) {
	case 1:
		return strings.ToLower(items[0])
	case 2:
		return strings.ToLower(items[0]) + " and " + strings.ToLower(items[1])
	}
	lower := make([]string, len(items))
	for i, s := range items {
		lower[i] = strings.ToLower(s)
	}
	return strings.Join(lower[:len(lower)-1], ", ") + " and " + lower[len(lower)-1]
}

// Rating is one word, good, fair or bad, for each part of a result; a part that could not be judged is empty.
type Rating struct {
	Pay, Benefits, Living string
	PayAmount             int64 // the posting's listed pay, 0 when none
	BenefitsScore         int
	LivingLeft            int // percent of take-home left over
	Found                 []string
	LeftMonthly           *int
}

// Rate boils a result down to good, fair or bad for pay, benefits and cost of living.
func Rate(r Result) Rating {
	var out Rating
	if !r.Supported {
		return out
	}
	out.BenefitsScore = r.Benefits.Score
	out.Found = r.Benefits.Found
	if r.Pay != nil {
		if r.Pay.Listed {
			out.PayAmount = r.Pay.Mid
		}
		out.Pay = map[string]string{"well_above": "good", "above": "good", "in_line": "fair", "below": "bad", "well_below": "bad"}[r.Pay.Verdict]
	}
	out.Benefits = map[string]string{"strong": "good", "decent": "fair", "basic": "bad"}[r.Benefits.Rating]
	if r.Lifestyle != nil {
		out.LivingLeft = r.Lifestyle.SavingsRate
		left := r.Lifestyle.LeftMonthly
		out.LeftMonthly = &left
		out.Living = map[string]string{"comfortable": "good", "okay": "fair", "tight": "bad", "short": "bad"}[r.Lifestyle.Verdict]
	}
	return out
}

func summarize(r Result) string {
	parts := []string{}
	if r.Pay != nil {
		switch r.Pay.Verdict {
		case "well_above", "above":
			parts = append(parts, "pays above market")
		case "in_line":
			parts = append(parts, "pays about market rate")
		case "below", "well_below":
			parts = append(parts, "pays below market")
		}
		if r.Pay.MeetsDesired != nil && !*r.Pay.MeetsDesired {
			parts = append(parts, "falls under your minimum desired pay")
		}
	}
	switch r.Benefits.Rating {
	case "strong", "decent":
		parts = append(parts, r.Benefits.Rating+" benefits")
	}
	if r.Lifestyle != nil {
		parts = append(parts, map[string]string{"comfortable": "leaves comfortable room to live", "okay": "covers living costs with some room to save",
			"tight": "leaves little room after living costs", "short": "does not cover living costs"}[r.Lifestyle.Verdict])
	}
	if len(parts) == 0 {
		return "Not enough was listed to judge this job."
	}
	return "This job " + strings.Join(parts, ", ") + "."
}

func hasCore(found []string, label string) bool {
	for _, f := range found {
		if f == label {
			return true
		}
	}
	return false
}
