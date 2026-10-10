package parser

import (
	"slices"
	"testing"
)

const posting = `Senior Backend Engineer
Company: Acme Corp
Location: Austin, TX (Hybrid)

About Us
Acme builds payment tools.

What you'll do
- Build services in Go and PostgreSQL
- Mentor junior engineers

Requirements
- 5+ years of experience with Go
- Strong SQL and Docker skills
- Excellent communication

Nice to have
- Kubernetes or Terraform experience
- Rust is a plus
`

func TestParsePosting(t *testing.T) {
	r := Parse(posting)
	if r.Inferred.PositionTitle != "Senior Backend Engineer" || r.Inferred.CompanyName != "Acme Corp" {
		t.Fatalf("inferred = %+v", r.Inferred)
	}
	if r.Inferred.Location != "Austin, TX (Hybrid)" || r.Inferred.WorkMode != "Hybrid" {
		t.Fatalf("location/mode = %+v", r.Inferred)
	}
	for _, want := range []string{"go", "sql", "docker", "postgresql"} {
		if !slices.Contains(r.ReqTech, want) {
			t.Errorf("req tech missing %q: %v", want, r.ReqTech)
		}
	}
	for _, want := range []string{"kubernetes", "terraform", "rust"} {
		if !slices.Contains(r.PrefTech, want) {
			t.Errorf("pref tech missing %q: %v", want, r.PrefTech)
		}
	}
	if !slices.Contains(r.ReqSoft, "communication") || !slices.Contains(r.ReqSoft, "mentoring") {
		t.Errorf("req soft = %v", r.ReqSoft)
	}
	if len(r.Requirements) != 3 {
		t.Errorf("requirements = %v", r.Requirements)
	}
}

func TestHeaderTitleAndCompany(t *testing.T) {
	cases := []struct{ text, title, company string }{
		// "About the role" is a section, not a company.
		{"Senior Backend Engineer — Northwind Logistics\nRemote (US)\n\nAbout the role\nYou will run services.", "Senior Backend Engineer", "Northwind Logistics"},
		{"Data Analyst | Bluebird Health\n\nAbout you\nYou like SQL.", "Data Analyst", "Bluebird Health"},
		{"Platform Engineer at Lumen Freight\n\nWhat you'll do", "Platform Engineer", "Lumen Freight"},
		// The right side is a team when the company is named elsewhere.
		{"Senior Engineer — Payments\n\nAbout Stripe\nWe build payments.", "Senior Engineer — Payments", "Stripe"},
		// A plain hyphen stays part of the title.
		{"Software Engineer - Backend\n\nAbout the team\nSmall team.", "Software Engineer - Backend", ""},
	}
	for _, c := range cases {
		in := Parse(c.text).Inferred
		if in.PositionTitle != c.title || in.CompanyName != c.company {
			t.Errorf("%q: got title %q company %q, want %q / %q", c.text[:20], in.PositionTitle, in.CompanyName, c.title, c.company)
		}
	}
}

func TestGoIsCaseSensitive(t *testing.T) {
	r := Parse("Requirements\n- You will go above and beyond and go to market quickly")
	if slices.Contains(r.ReqTech, "go") {
		t.Fatalf("matched 'go' as a skill: %v", r.ReqTech)
	}
}

func TestContainsTerm(t *testing.T) {
	cases := []struct {
		text, term string
		want       bool
	}{
		{"we use c++ daily", "c++", true},
		{"we use c# and .net", "c#", true},
		{"we use c# and .net", ".net", true},
		{"cpp and c++", "c", false},
		{"node.js apps", "node.js", true},
		{"nodejs", "node", false},
		{"restful apis", "rest", false},
		{"rest apis", "rest", true},
	}
	for _, c := range cases {
		if got := ContainsTerm(c.text, c.term); got != c.want {
			t.Errorf("ContainsTerm(%q, %q) = %v, want %v", c.text, c.term, got, c.want)
		}
	}
}

func TestCleanGreenhouseHTML(t *testing.T) {
	got := Clean("&lt;p&gt;Hello &amp;amp; welcome&lt;/p&gt;&lt;ul&gt;&lt;li&gt;Go&lt;/li&gt;&lt;/ul&gt;")
	if got != "Hello & welcome\n\n- Go" {
		t.Fatalf("Clean = %q", got)
	}
}

func TestCanon(t *testing.T) {
	for in, want := range map[string]string{"Golang": "go", "K8s": "kubernetes", " Postgres ": "postgresql", "Haskell": "haskell"} {
		if got := Canon(in); got != want {
			t.Errorf("Canon(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeywords(t *testing.T) {
	k := Keywords("payments payments engineer. payments engineer needed", 3)
	if len(k) == 0 || k[0] != "payments" {
		t.Fatalf("Keywords = %v", k)
	}
}

func TestGreenhouseRequirementsAreBullets(t *testing.T) {
	r := Parse("&lt;h3&gt;Requirements&lt;/h3&gt;&lt;ul&gt;&lt;li&gt;5 years of Go&lt;/li&gt;&lt;li&gt;SQL&lt;/li&gt;&lt;/ul&gt;")
	if len(r.Requirements) != 2 || r.Requirements[0] != "5 years of Go" {
		t.Fatalf("requirements = %q", r.Requirements)
	}
}

func TestSingleLetterLanguages(t *testing.T) {
	cases := map[string]bool{
		"Experience with C, C++ or Rust": true,
		"Python/R for analysis":          true,
		"refugee under 8 U.S.C. § 1157":  false,
		"Our R&D group":                  false,
		"Plan C is a backup":             false,
	}
	for line, want := range cases {
		got := singleLetterSkill(line, "C") || singleLetterSkill(line, "R")
		if got != want {
			t.Errorf("%q: got %v want %v", line, got, want)
		}
	}
}

func TestIntroSkillsAreNotRequirements(t *testing.T) {
	r := Parse("About us\nWe love collaborative engineers who take ownership and use Kubernetes.\nBasic qualifications\n- Go experience\nPreferred\n- Terraform")
	if slices.Contains(r.ReqSoft, "collaboration") || slices.Contains(r.ReqTech, "kubernetes") || !slices.Contains(r.PrefTech, "kubernetes") || !slices.Contains(r.ReqTech, "go") {
		t.Fatalf("req=%v/%v pref=%v", r.ReqTech, r.ReqSoft, r.PrefTech)
	}
}
