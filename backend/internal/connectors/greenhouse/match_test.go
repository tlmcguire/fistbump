package greenhouse

import (
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
)

func TestTitleScore(t *testing.T) {
	cases := []struct {
		title    string
		keywords []string
		want     float64
	}{
		{"Senior Data Analyst", []string{"data analyst"}, 1},
		{"Sr. Data Analyst II", []string{"Data Analyst"}, 1},
		{"Software Developer", []string{"software engineer"}, 1},
		{"Front End Engineer", []string{"frontend engineer"}, 1},
		{"Back-end Developer", []string{"backend engineer"}, 1},
		{"Data Scientist", []string{"data analyst"}, 0.5},
		{"Marketing Manager", []string{"data analyst"}, 0},
		{"Software Engineering Intern", []string{"software engineer"}, 0.5},
		{"Accountant", []string{"data analyst", "accountant"}, 1},
	}
	for _, c := range cases {
		if got := TitleScore(c.title, c.keywords); got != c.want {
			t.Errorf("TitleScore(%q, %v) = %v, want %v", c.title, c.keywords, got, c.want)
		}
	}
	if TitleScore("Intern", []string{"engineer"}) != 0 {
		t.Error("intern must not be dropped as a seniority word")
	}
}

func TestWorkMode(t *testing.T) {
	cases := []struct{ loc, title, want string }{
		{"Remote - US", "Analyst", "Remote"},
		{"Austin, TX", "Analyst (Remote)", "Remote"},
		{"Hybrid - Boston", "Analyst", "Hybrid"},
		{"Austin, TX", "Analyst", "On-site"},
	}
	for _, c := range cases {
		if got := WorkMode(c.loc, c.title); got != c.want {
			t.Errorf("WorkMode(%q, %q) = %q, want %q", c.loc, c.title, got, c.want)
		}
	}
}

func TestModeOK(t *testing.T) {
	cases := []struct {
		mode, want string
		ok         bool
	}{
		{"Remote", "Any", true},
		{"Remote", "Remote", true},
		{"On-site", "Remote", false},
		{"Hybrid", "Remote", false},
		{"Hybrid", "Hybrid", true},
		{"On-site", "Hybrid", true}, // does not say, so it may be hybrid
		{"Remote", "Hybrid", false},
		{"On-site", "On-site", true},
		{"Hybrid", "On-site", false},
		{"Remote", "On-site", false},
	}
	for _, c := range cases {
		if got := ModeOK(c.mode, c.want); got != c.ok {
			t.Errorf("ModeOK(%q, %q) = %v, want %v", c.mode, c.want, got, c.ok)
		}
	}
}

func TestLocation(t *testing.T) {
	terms := locationTerms("Charleston, SC")
	if !LocationOK("Charleston, SC", "On-site", terms, "Any") {
		t.Error("city match should pass")
	}
	if LocationOK("Berlin, Germany", "On-site", locationTerms("IN"), "Any") {
		t.Error("two-letter term must match a whole word")
	}
	if LocationOK("Denver, CO", "On-site", terms, "Any") {
		t.Error("other city should not pass")
	}
	if !LocationOK("Remote - US", connectors.WorkModeRemote, terms, "Any") {
		t.Error("remote passes a location filter")
	}
	for _, loc := range []string{"Remote - Australia", "Remote - Ireland", "Remote, Poland", "Remote - UK", "Remote Canada"} {
		if LocationOK(loc, connectors.WorkModeRemote, terms, "Any") {
			t.Errorf("%q should not pass for a US location", loc)
		}
	}
	for _, loc := range []string{"Remote", "Remote - United States", "Remote - US", "US-Remote", "Raleigh, North Carolina, USA; Remote, East Coast USA", "Costa Mesa, California, United States; Remote"} {
		if !LocationOK(loc, connectors.WorkModeRemote, terms, "Any") {
			t.Errorf("%q should pass for a US location", loc)
		}
	}
	if !LocationOK("Remote - Australia", connectors.WorkModeRemote, locationTerms("Sydney"), "Any") {
		t.Error("foreign remote is not filtered when the user is not in the US")
	}
	if LocationOK("Remote - US", connectors.WorkModeRemote, terms, "On-site") {
		t.Error("remote does not pass when On-site is requested")
	}
	if !LocationOK("Anywhere", "On-site", nil, "Any") {
		t.Error("no location filter passes everything")
	}
	if LocationScore("Charleston, SC", "On-site", terms) != 1 || LocationScore("Remote", "Remote", terms) != 0.5 || LocationScore("Denver", "On-site", terms) != 0 {
		t.Error("location scores")
	}
	if got := Score(1, 0.5); got != 0.85 {
		t.Errorf("Score = %v, want 0.85", got)
	}
}

func TestLocationAliases(t *testing.T) {
	cases := []struct {
		want, posting string
		ok            bool
	}{
		{"Charleston, South Carolina", "Charleston, SC", true},
		{"South Carolina", "Columbia, SC", true},
		{"Charleston, SC", "South Carolina", true},
		{"NYC", "New York, NY", true},
		{"New York", "NYC", true},
		{"SF", "San Francisco, CA", true},
		{"Bay Area", "Oakland, CA", true},
		{"Oregon", "Oakland, CA or Remote", false}, // "or" is a word, not the state
		{"Oregon", "Portland, OR", true},
		{"Indiana", "Berlin, Germany", false},
		{"Texas", "Austin, TX", true},
	}
	for _, c := range cases {
		if got := locationMatches(c.posting, locationTerms(c.want)); got != c.ok {
			t.Errorf("%q vs %q = %v, want %v", c.want, c.posting, got, c.ok)
		}
	}
}

func TestDescribedMode(t *testing.T) {
	cases := map[string]string{
		"This is a fully remote position.":         "Remote",
		"We are remote-first.":                     "Remote",
		"Hybrid role, 3 days a week in the office": "Hybrid",
		"Benefits include a remote work stipend.":  "",
		"Great team.": "",
	}
	for text, want := range cases {
		if got := DescribedMode(text); got != want {
			t.Errorf("DescribedMode(%q) = %q, want %q", text, got, want)
		}
	}
}
