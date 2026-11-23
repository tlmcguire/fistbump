package cost

import (
	"math"
	"strings"
	"testing"
)

// The reference tables are hand-written, so check them for the slips a hand makes.
func TestDataSanity(t *testing.T) {
	codes := map[string]bool{}
	for _, s := range states {
		if codes[s.State] {
			t.Errorf("duplicate state %s", s.State)
		}
		codes[s.State] = true
		if s.Index < 70 || s.Index > 200 || s.Rent < 600 || s.Rent > 3000 || s.StateTax < 0 || s.StateTax > 0.12 {
			t.Errorf("state %s out of range: %+v", s.Name, s)
		}
	}
	if len(states) != 51 {
		t.Errorf("%d states; want 50 and DC", len(states))
	}
	seen := map[string]string{}
	for _, m := range metros {
		if !codes[m.State] {
			t.Errorf("%s has unknown state %s", m.Name, m.State)
		}
		if m.Index < 70 || m.Index > 200 || m.Rent < 800 || m.Rent > 4000 || m.CityTax > 0.05 || m.StateTax > 0.12 {
			t.Errorf("metro %s out of range: %+v", m.Name, m)
		}
		// a metro costs more rent than its state's typical rent only if its index is higher, roughly
		if m.Rent*12 > 1800*12 && m.Index < 100 {
			t.Errorf("%s: high rent %d with low index %v", m.Name, m.Rent, m.Index)
		}
		for _, a := range append([]string{m.Name}, m.Aliases...) {
			if a != strings.ToLower(a) && a != m.Name {
				t.Errorf("alias %q should be lower case", a)
			}
			key := strings.ToLower(a)
			if other, dup := seen[key]; dup && other != m.Name {
				t.Errorf("alias %q belongs to both %s and %s", a, other, m.Name)
			}
			seen[key] = m.Name
		}
	}
	for _, r := range roles {
		if r.Median < 40000 || r.Median > 250000 || len(r.Words) == 0 {
			t.Errorf("role %s: %+v", r.Name, r)
		}
	}
}

func TestEveryPlaceResolvesToItself(t *testing.T) {
	for _, m := range metros {
		if p, ok, precise := resolve(m.Name); !ok || p.Name != m.Name || !precise {
			t.Errorf("resolve(%q) = %q %v %v", m.Name, p.Name, ok, precise)
		}
		if m.State != "DC" && m.Name != "the San Francisco Bay Area, CA" {
			if p, ok, _ := resolve(strings.TrimSuffix(m.Name, ", "+m.State) + ", " + m.State); !ok || p.Name != m.Name {
				t.Errorf("city, code form of %q gave %q", m.Name, p.Name)
			}
		}
	}
	for _, s := range states {
		if p, ok, precise := resolve(s.Name); !ok || p.State != s.State || (precise && !strings.Contains(p.Name, ",")) {
			t.Errorf("resolve(%q) = %q %v", s.Name, p.Name, ok)
		}
	}
}

func TestResolveTrickyLocations(t *testing.T) {
	for loc, want := range map[string]string{
		"Portland, OR":                      "Portland, OR",
		"Portland, Oregon":                  "Portland, OR",
		"Portland, ME":                      "Maine",
		"Washington, DC":                    "Washington, DC",
		"Washington, D.C.":                  "Washington, DC",
		"Washington":                        "Washington",
		"Seattle, Washington, USA":          "Seattle, WA",
		"Columbus, OH":                      "Columbus, OH",
		"Columbus, GA":                      "Georgia",
		"Richmond, VA":                      "Richmond, VA",
		"Birmingham, AL":                    "Birmingham, AL",
		"Greenville, SC":                    "Greenville, SC",
		"Cambridge, MA":                     "Boston, MA",
		"Jersey City, NJ":                   "Jersey City, NJ",
		"SF Bay Area":                       "the San Francisco Bay Area, CA",
		"Mountain View, CA":                 "the San Francisco Bay Area, CA",
		"San Francisco, California":         "San Francisco, CA",
		"New York, NY or Remote":            "New York City, NY",
		"Remote | Austin, TX":               "Austin, TX",
		"Hybrid - Denver, CO":               "Denver, CO",
		"Santa Fe, New Mexico":              "New Mexico",
		"Kansas City, MO / Kansas City, KS": "Kansas City, MO",
		"Fort Worth, Texas":                 "Dallas, TX",
	} {
		p, ok, _ := resolve(loc)
		if !ok || p.Name != want {
			t.Errorf("resolve(%q) = %q (ok %v); want %q", loc, p.Name, ok, want)
		}
	}
	for _, loc := range []string{"", "Remote", "Remote - US", "United States", "Anywhere", "Hybrid"} {
		if _, ok, _ := resolve(loc); ok {
			t.Errorf("resolve(%q) should name no place", loc)
		}
	}
}

func TestForeignLocationsAreUnsupported(t *testing.T) {
	for _, loc := range []string{"London, UK", "Cambridge, UK", "Toronto, Canada", "Berlin, Germany", "Remote - Europe", "Dublin, Ireland",
		"Remote - Canada", "Sydney, Australia", "Bengaluru, India", "Mexico City, Mexico", "Remote, EMEA"} {
		if Analyze(Input{Title: "Software Engineer", Location: loc, PayMin: 100000, PayMax: 120000}).Supported {
			t.Errorf("%q should not be supported", loc)
		}
	}
	for _, loc := range []string{"Santa Fe, New Mexico", "Remote - US", "Remote", "London, UK; New York, NY", "Boston, MA", "Remote - United States"} {
		if !Analyze(Input{Title: "Software Engineer", Location: loc, PayMin: 100000, PayMax: 120000}).Supported {
			t.Errorf("%q should be supported", loc)
		}
	}
}

func TestTaxes(t *testing.T) {
	// 2026 single filer, no credits: 10% to 12,400; 12% to 50,400; 22% to 105,700
	if got := federalTax(100000); math.Abs(got-13170) > 1 {
		t.Errorf("federal tax on 100k = %.0f; want 13170", got)
	}
	if got := federalTax(10000); got != 0 {
		t.Errorf("below the standard deduction = %.0f", got)
	}
	if got := payroll(100000); math.Abs(got-7650) > 1 {
		t.Errorf("payroll on 100k = %.0f; want 7650", got)
	}
	// Social Security stops at the cap; Medicare does not
	if hi, lo := payroll(300000)-payroll(200000), 100000*0.0145+100000*0.009; math.Abs(hi-lo) > 1 {
		t.Errorf("payroll above the cap grew by %.0f; want %.0f", hi, lo)
	}
	prev := 0.0
	for g := 20000.0; g <= 500000; g += 5000 {
		net, _, _, _ := netMonthly(g, place{StateTax: 0.05})
		if net <= prev {
			t.Fatalf("take-home fell or stalled at %.0f", g)
		}
		prev = net
	}
	tx, _, _ := resolve("Austin, TX")
	ca, _, _ := resolve("San Jose, CA")
	nyc, _, _ := resolve("New York, NY")
	a, _, _, _ := netMonthly(120000, tx)
	b, _, _, _ := netMonthly(120000, ca)
	c, _, _, _ := netMonthly(120000, nyc)
	if !(a > b && b > c) {
		t.Errorf("take-home should fall from TX (%.0f) to CA (%.0f) to NYC (%.0f)", a, b, c)
	}
}

func TestExpectedTaxesAreInTheCostOfLiving(t *testing.T) {
	r := Analyze(Input{Title: "Software Engineer", Location: "Austin, TX", PayMin: 120000, PayMax: 120000})
	l := r.Lifestyle
	gross := float64(l.Gross) / 12
	if float64(l.NetMonthly) >= gross {
		t.Fatalf("take-home %d is not below gross %.0f", l.NetMonthly, gross)
	}
	if got := l.GrossMonthly - l.TaxMonthly; abs(got-l.NetMonthly) > 1 {
		t.Errorf("gross %d minus tax %d = %d; take-home %d", l.GrossMonthly, l.TaxMonthly, got, l.NetMonthly)
	}
	if l.LeftMonthly != l.NetMonthly-l.CostMonthly {
		t.Errorf("left over %d should be take-home %d minus costs %d", l.LeftMonthly, l.NetMonthly, l.CostMonthly)
	}
	sum := 0
	for _, c := range l.Costs {
		sum += c.Amount
	}
	if sum != l.CostMonthly {
		t.Errorf("costs add to %d, not %d", sum, l.CostMonthly)
	}
	// the same pay left over less in a state with income tax
	ny := Analyze(Input{Title: "Software Engineer", Location: "Austin, TX", PayMin: 120000, PayMax: 120000}).Lifestyle
	ca := Analyze(Input{Title: "Software Engineer", Location: "Sacramento, CA", PayMin: 120000, PayMax: 120000}).Lifestyle
	if ca.TaxMonthly <= ny.TaxMonthly {
		t.Errorf("California tax %d should exceed Texas %d", ca.TaxMonthly, ny.TaxMonthly)
	}
	// a higher raise means more tax in absolute terms but a lower rate than the top bracket
	if l.TaxRate < 15 || l.TaxRate > 30 {
		t.Errorf("effective rate %d%% on 120k", l.TaxRate)
	}
}

func TestPeriodsAddUp(t *testing.T) {
	l := Analyze(Input{Title: "Data Analyst", Location: "Chicago, IL", PayMin: 90000, PayMax: 110000}).Lifestyle
	if l.Gross != 100000 {
		t.Fatalf("midpoint %d", l.Gross)
	}
	want := map[string]int{"Per year": 100000, "Per month": 8333, "Every two weeks": 3846, "Per week": 1923, "Per day": 385, "Per hour": 48}
	for _, p := range l.Periods {
		if want[p.Label] != p.Gross {
			t.Errorf("%s gross %d; want %d", p.Label, p.Gross, want[p.Label])
		}
		if p.Net >= p.Gross || p.Net <= 0 {
			t.Errorf("%s net %d of gross %d", p.Label, p.Net, p.Gross)
		}
	}
	if abs(l.Periods[1].Net-l.NetMonthly) > 1 {
		t.Errorf("monthly net %d vs %d", l.Periods[1].Net, l.NetMonthly)
	}
}

func abs0(n int) int { return abs(n) }
