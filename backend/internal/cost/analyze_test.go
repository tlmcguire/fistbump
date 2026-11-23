package cost

import "testing"

func TestResolve(t *testing.T) {
	for loc, want := range map[string]string{
		"San Francisco, CA":                 "San Francisco, CA",
		"Seattle, WA | Austin, TX":          "Seattle, WA",
		"New York, New York, United States": "New York City, NY",
		"Austin, Texas":                     "Austin, TX",
		"Portland, ME":                      "Maine",
		"Boise, Idaho":                      "Boise, ID",
		"Montana":                           "Montana",
	} {
		p, ok, _ := resolve(loc)
		if !ok || p.Name != want {
			t.Errorf("resolve(%q) = %q, %v; want %q", loc, p.Name, ok, want)
		}
	}
	if _, ok, _ := resolve("Remote - US"); ok {
		t.Error("remote should name no place")
	}
}

func TestAnalyzeCompetitiveAndLivable(t *testing.T) {
	r := Analyze(Input{Title: "Senior Software Engineer", Location: "Austin, TX", WorkMode: "On-site", PayMin: 170000, PayMax: 200000,
		Text: "We offer medical, dental and vision, a 401(k) match, unlimited PTO, parental leave and equity."})
	if !r.Supported || r.Pay == nil || r.Lifestyle == nil {
		t.Fatalf("incomplete: %+v", r)
	}
	if r.Pay.Verdict != "above" && r.Pay.Verdict != "well_above" {
		t.Errorf("verdict = %s, diff %d", r.Pay.Verdict, r.Pay.DiffPercent)
	}
	if r.Lifestyle.Verdict != "comfortable" {
		t.Errorf("lifestyle = %s (%d left)", r.Lifestyle.Verdict, r.Lifestyle.LeftMonthly)
	}
	if r.Benefits.Rating != "strong" {
		t.Errorf("benefits = %s (%d)", r.Benefits.Rating, r.Benefits.Score)
	}
}

func TestLowPayExpensiveCity(t *testing.T) {
	r := Analyze(Input{Title: "Customer Support Specialist", Location: "San Francisco, CA", PayMin: 50000, PayMax: 56000})
	if r.Lifestyle.Verdict != "short" {
		t.Errorf("lifestyle = %s", r.Lifestyle.Verdict)
	}
	if r.Pay.Verdict != "below" && r.Pay.Verdict != "well_below" {
		t.Errorf("verdict = %s", r.Pay.Verdict)
	}
}

func TestRemoteUsesHome(t *testing.T) {
	r := Analyze(Input{Title: "Data Analyst", Location: "Remote - US", WorkMode: "Remote", HomeLocation: "Boise, ID", PayMin: 80000, PayMax: 90000})
	if r.Location.Basis != "home" || r.Location.Label != "Boise, ID" {
		t.Errorf("location = %+v", r.Location)
	}
	r = Analyze(Input{Title: "Data Analyst", Location: "Remote - US", WorkMode: "Remote"})
	if r.Location.Basis != "national" {
		t.Errorf("location = %+v", r.Location)
	}
}

func TestUnlistedPayUsesMarket(t *testing.T) {
	r := Analyze(Input{Title: "Product Manager", Location: "Chicago, IL"})
	if r.Pay == nil || r.Pay.Source != "market" || r.Lifestyle == nil {
		t.Fatalf("%+v", r.Pay)
	}
	if r.Benefits.Rating != "unlisted" {
		t.Errorf("benefits = %s", r.Benefits.Rating)
	}
}

func TestForeign(t *testing.T) {
	if Analyze(Input{Title: "Engineer", Location: "Toronto, Canada", PayMin: 100000}).Supported {
		t.Error("Canada should be unsupported")
	}
}

func TestTax(t *testing.T) {
	if f := federalTax(100000); f < 13000 || f > 15500 {
		t.Errorf("federal tax on 100k = %.0f", f)
	}
}

func TestPayBreakdown(t *testing.T) {
	r := Analyze(Input{Title: "Software Engineer", Location: "Austin, TX", PayMin: 120000, PayMax: 120000})
	l := r.Lifestyle
	if len(l.Periods) != 6 || l.Periods[0].Gross != 120000 || l.Periods[1].Gross != 10000 || l.Periods[3].Gross != 2308 {
		t.Fatalf("periods: %+v", l.Periods)
	}
	if l.Periods[0].Net >= l.Periods[0].Gross || l.TaxRate < 15 || l.TaxRate > 35 {
		t.Errorf("tax: net %d rate %d", l.Periods[0].Net, l.TaxRate)
	}
	if got := l.NetMonthly - l.CostMonthly; got != l.LeftMonthly {
		t.Errorf("left over %d, want take-home minus costs %d", l.LeftMonthly, got)
	}
}
