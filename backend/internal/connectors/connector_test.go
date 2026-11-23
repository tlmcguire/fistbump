package connectors

import "testing"

func TestFitFilters(t *testing.T) {
	two := 2
	zero := 0
	f := &Fit{Percent: 60, Matched: 3, Required: 5, Missing: []string{"go", "aws"}}
	cases := []struct {
		name string
		q    Query
		fit  *Fit
		want bool
	}{
		{"no filter", Query{}, f, true},
		{"min fit met", Query{MinFit: 60}, f, true},
		{"min fit not met", Query{MinFit: 70}, f, false},
		{"missing within limit", Query{MaxMissing: &two}, f, true},
		{"missing over limit", Query{MaxMissing: &zero}, f, false},
		{"both must hold", Query{MinFit: 50, MaxMissing: &zero}, f, false},
		{"unrated posting is kept", Query{MinFit: 90, MaxMissing: &zero}, nil, true},
	}
	for _, c := range cases {
		if got := c.q.FitOK(c.fit); got != c.want {
			t.Errorf("%s: FitOK = %v, want %v", c.name, got, c.want)
		}
	}
	if (Query{}).HasFit() || !(Query{MinFit: 1}).HasFit() || !(Query{MaxMissing: &zero}).HasFit() {
		t.Error("HasFit")
	}
}

func TestFitFilterValidation(t *testing.T) {
	base := Query{Limit: 20, WorkMode: WorkModeAny}
	bad := -1
	big := 51
	for name, q := range map[string]Query{
		"min fit over 100": {Limit: 20, WorkMode: WorkModeAny, MinFit: 101},
		"min fit negative": {Limit: 20, WorkMode: WorkModeAny, MinFit: -1},
		"max missing neg":  {Limit: 20, WorkMode: WorkModeAny, MaxMissing: &bad},
		"max missing big":  {Limit: 20, WorkMode: WorkModeAny, MaxMissing: &big},
	} {
		if q.Validate() == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	base.MinFit = 70
	if err := base.Validate(); err != nil {
		t.Errorf("valid fit filter rejected: %v", err)
	}
}

func TestRatingFilters(t *testing.T) {
	good := &Ratings{Pay: "good", Benefits: "fair", Living: "good"}
	if !(Query{}).RatingsOK(nil) {
		t.Error("no filter should pass an unrated posting")
	}
	q := Query{MinBenefits: "good"}
	if q.RatingsOK(good) || q.RatingsOK(nil) {
		t.Error("fair benefits and unrated benefits should fail a good-benefits filter")
	}
	q = Query{MinBenefits: "fair", MinPayRating: "good"}
	if !q.RatingsOK(good) {
		t.Error("should pass")
	}
	off := false
	if (Query{MinBenefits: "good", UseRatings: &off}).HasRatingFilter() {
		t.Error("ratings off means no rating filter")
	}
	if (Query{Limit: 5, WorkMode: "Any", MinLiving: "great"}).Validate() == nil {
		t.Error("a bad rating word should be refused")
	}
}
