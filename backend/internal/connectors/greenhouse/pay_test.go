package greenhouse

import "testing"

func TestParsePay(t *testing.T) {
	cases := []struct {
		text    string
		low, hi int
		ok      bool
	}{
		{"The range is $120,000 - $150,000 per year.", 120000, 150000, true},
		{"Pay: $60k to $75k", 60000, 75000, true},
		{"$100 - 150k depending on experience", 100000, 150000, true},
		{"$45 - $55 per hour", 93600, 114400, true},
		{"US: $140,000-$170,000. Canada: $130,000 – $160,000", 130000, 170000, true},
		{"Raised $50 - $60 million in funding", 0, 0, false}, // below a salary once read as dollars
		{"A $5,000 - $10,000 signing bonus", 0, 0, false},
		{"No pay listed", 0, 0, false},
	}
	for _, c := range cases {
		low, hi, ok := ParsePay(c.text)
		if ok != c.ok || low != c.low || hi != c.hi {
			t.Errorf("ParsePay(%q) = %d, %d, %v; want %d, %d, %v", c.text, low, hi, ok, c.low, c.hi, c.ok)
		}
	}
}

func TestPayOK(t *testing.T) {
	cases := []struct {
		lo, hi, wantMin, wantMax int
		require, ok              bool
	}{
		{100000, 130000, 120000, 0, false, true},  // overlaps
		{100000, 110000, 120000, 0, false, false}, // tops out under the minimum
		{150000, 170000, 0, 140000, false, false}, // starts over the maximum
		{0, 0, 120000, 0, false, true},            // lists no pay, kept
		{0, 0, 120000, 0, true, false},            // lists no pay, dropped
	}
	for _, c := range cases {
		if got := PayOK(c.lo, c.hi, c.wantMin, c.wantMax, c.require); got != c.ok {
			t.Errorf("PayOK(%+v) = %v", c, got)
		}
	}
}

func TestLevelAndExclude(t *testing.T) {
	levels := map[string]string{
		"Software Engineering Intern": "Intern",
		"Junior Data Analyst":         "Entry",
		"Data Analyst":                "Mid",
		"Senior Data Analyst":         "Senior",
		"Staff Engineer":              "Lead",
		"Engineering Manager":         "Lead",
		"Data Analyst III":            "Senior",
	}
	for title, want := range levels {
		if got := Level(title); got != want {
			t.Errorf("Level(%q) = %q, want %q", title, got, want)
		}
	}
	if ExcludeOK("Sales Engineer", []string{"sales"}) || !ExcludeOK("Wholesale Buyer", []string{"sales"}) {
		t.Error("exclude matches whole words only")
	}
	if ExcludeOK("Customer Success Manager", []string{"success manager"}) {
		t.Error("exclude matches phrases")
	}
}
