package cost

import (
	"slices"
	"strings"
	"testing"
)

// Wording that is not a benefit must not count as one: these phrases are in nearly every posting.
func TestBenefitsIgnoreBoilerplate(t *testing.T) {
	text := `Our vision is to build the best product. We are a healthcare technology company and a leader in the
healthcare industry. We value diversity, equity and inclusion. We provide reasonable accommodations to
applicants with a disability. Certification in AWS is required. Acme Holdings LTD is an equal opportunity employer.
Ira will reach out. You will report to the VP and need to be vision-driven.`
	if b := benefits(text); len(b.Found) != 0 || b.Rating != "unlisted" {
		t.Errorf("found %v, rating %s; want nothing", b.Found, b.Rating)
	}
}

func TestBenefitsDetectReal(t *testing.T) {
	cases := []struct {
		name, text string
		want       []string
		not        []string
	}{
		{"full package", `Benefits: Medical, dental and vision insurance; 401(k) with 4% company match; 20 days paid time off and
12 paid holidays; 16 weeks paid parental leave; stock options; annual bonus; $1,500 learning budget; wellness stipend;
life and disability insurance.`,
			[]string{"Health insurance", "Dental", "Vision", "Retirement plan with a match", "Paid time off", "Paid holidays", "Parental leave", "Equity or stock", "Bonus", "Learning budget", "Wellness", "Life and disability insurance", "Stipends and commuter"}, nil},
		{"bare essentials", "We offer health insurance, dental coverage and a 401k.", []string{"Health insurance", "Dental", "Retirement plan"}, []string{"Vision", "Paid time off"}},
		{"unlimited pto", "Unlimited PTO and flexible hours.", []string{"Paid time off"}, []string{"Health insurance"}},
		{"healthcare coverage wording", "Comprehensive healthcare coverage for you and your family.", []string{"Health insurance"}, nil},
		{"vision list", "Benefits include medical, dental, and vision plans.", []string{"Health insurance", "Dental", "Vision"}, nil},
		{"rsus", "Benefits include RSUs.", []string{"Equity or stock"}, nil},
		{"hsa", "We offer HSA and FSA options.", []string{"Stipends and commuter"}, nil},
		{"pension", "We offer a defined benefit pension plan.", []string{"Retirement plan"}, nil},
	}
	for _, c := range cases {
		b := benefits(c.text)
		for _, w := range c.want {
			if !slices.Contains(b.Found, w) {
				t.Errorf("%s: missing %q in %v", c.name, w, b.Found)
			}
		}
		for _, n := range c.not {
			if slices.Contains(b.Found, n) {
				t.Errorf("%s: %q should not be found (%v)", c.name, n, b.Found)
			}
		}
	}
}

func TestBenefitsRatings(t *testing.T) {
	full := `Benefits: Medical, dental, vision; 401(k) match; paid time off; paid parental leave; stock options; bonus; learning budget.`
	if r := benefits(full).Rating; r != "strong" {
		t.Errorf("full package = %s", r)
	}
	// plenty of perks but no health insurance cannot be strong
	perks := `We offer a 401(k) match, unlimited PTO, paid parental leave, stock options, annual bonus, learning budget, wellness stipend, paid holidays.`
	if r := benefits(perks).Rating; r == "strong" {
		t.Errorf("no health insurance should not be strong, got %s (score %d)", r, benefits(perks).Score)
	}
	if r := benefits("We offer a 401(k).").Rating; r != "basic" {
		t.Errorf("one item = %s", r)
	}
	if r := benefits("We offer health insurance, dental and a 401(k) match.").Rating; r != "decent" && r != "strong" {
		t.Errorf("core three = %s", r)
	}
	if b := benefits(""); b.Rating != "unlisted" || len(b.Found) != 0 {
		t.Errorf("empty = %+v", b)
	}
	if b := benefits("We offer medical, dental, vision and a 401(k) match."); !slices.Contains(b.Missing, "Paid time off") || slices.Contains(b.Missing, "Dental") {
		t.Errorf("missing = %v", b.Missing)
	}
}

// Wording that only sounds like a benefit, with no offer around it, is never counted.
func TestBenefitsNeverInvented(t *testing.T) {
	for _, text := range []string{
		"Experience with health insurance claims processing is a plus.",
		"You will build dental practice management software.",
		"Familiarity with 401(k) administration and pension systems.",
		"We are a life insurance company serving millions.",
		"Our culture of equity and belonging.",
		"The company offers insurance products to customers.",
		"Bonus points if you know Go. Gym membership not included.",
	} {
		if b := benefits(text); len(b.Found) != 0 {
			t.Errorf("%q: invented %v", text, b.Found)
		}
	}
}

func TestBenefitsHeadingScope(t *testing.T) {
	text := "About the role\nYou will build health insurance claims software.\n\nBenefits\nMedical, dental, vision\n401(k) match\n"
	b := benefits(text)
	for _, want := range []string{"Health insurance", "Dental", "Vision", "Retirement plan with a match"} {
		if !slices.Contains(b.Found, want) {
			t.Errorf("under a heading: missing %q in %v", want, b.Found)
		}
	}
	// the same words above the heading do not count
	above := "You will work on dental software and 401(k) systems.\n\nBenefits\nPaid time off\n"
	if b := benefits(above); slices.Contains(b.Found, "Dental") || slices.Contains(b.Found, "Retirement plan") || !slices.Contains(b.Found, "Paid time off") {
		t.Errorf("heading scope: %v", b.Found)
	}
}

func TestBenefitsSaySoWhenUnreadable(t *testing.T) {
	cases := map[string]string{
		"We offer a comprehensive benefits package and a great culture.": "unclear",
		"Benefits\nCompetitive and comprehensive. Ask us for details.":   "unclear",
		"Join our team! You will write code and ship product.":           "unlisted",
		"":       "unlisted",
		"   \n ": "unlisted",
	}
	for text, want := range cases {
		b := benefits(text)
		if b.Rating != want {
			t.Errorf("%q: rating %s; want %s", text, b.Rating, want)
		}
		if len(b.Found) != 0 || len(b.Missing) != 0 || b.Score != 0 {
			t.Errorf("%q: nothing should be found or listed missing: %+v", text, b)
		}
		if !strings.Contains(b.Headline, "not rated") && !strings.Contains(b.Headline, "unknown") {
			t.Errorf("%q: headline should say it is not rated: %q", text, b.Headline)
		}
	}
	// unreadable benefits rate nothing, so they cannot help or hurt a filter
	r := Analyze(Input{Title: "Software Engineer", Location: "Austin, TX", Text: "A comprehensive benefits package."})
	if got := Rate(r); got.Benefits != "" {
		t.Errorf("unclear benefits rated %q", got.Benefits)
	}
}
