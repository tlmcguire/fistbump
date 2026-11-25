package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/analyze"
	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func TestCheckDropsNoOpAndTypographyOnlyEdits(t *testing.T) {
	req := Request{
		Resume: "Bachelor’s degree in Computer Science. Built Go services.",
		Job:    models.Job{PositionTitle: "Backend Engineer", CompanyName: "Acme"},
	}

	cases := []struct {
		name     string
		section  string
		original string
		proposed string
	}{
		{"curly vs straight apostrophe", "summary", "Earned a Bachelor’s degree in CS.", "Earned a Bachelor's degree in CS."},
		{"straight vs curly apostrophe", "summary", "Earned a Bachelor's degree in CS.", "Earned a Bachelor’s degree in CS."},
		{"trailing period toggle", "experience", "- Built Go services", "- Built Go services."},
		{"en-dash vs hyphen", "experience", "- Built Go services – reduced latency", "- Built Go services - reduced latency"},
		{"case-only tweak", "summary", "Built Go services for Acme.", "Built go services for Acme."},
		{"extra whitespace only", "summary", "Built Go services.", "Built  Go   services."},
		{"skills same order different spacing", "skills", "Go, SQL, Python", "Go,  SQL, Python"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := Target{Section: tc.section, Original: tc.original}
			if reason := Check(target, tc.proposed, req); reason == "" {
				t.Errorf("Check(%q -> %q) returned empty reason, want rejection", tc.original, tc.proposed)
			}
		})
	}
}

func TestVerifyReasonFixesVagueOrContradictoryReasons(t *testing.T) {
	matched := []string{"Go", "PostgreSQL", "Docker"}

	// Contradictory reason on skills reorder ("Added Go") must be replaced with accurate skillsNamed reason.
	got := verifyReason("en", "skills", "Python, Go, Docker", "Go, Docker, Python", "Added Go and Docker to your skills.", matched)
	if strings.Contains(strings.ToLower(got), "added go") {
		t.Errorf("contradictory reason was not replaced: %q", got)
	}
	if !strings.Contains(got, "Go") || !strings.Contains(got, "Docker") {
		t.Errorf("expected replacement reason to cite moved skills, got %q", got)
	}

	// Overly generic reason must be replaced with a specific grounded reason.
	got = verifyReason("en", "experience", "- Built APIs", "- Built Go APIs with PostgreSQL", "Improved readability.", matched)
	if got == "Improved readability." || !strings.Contains(got, "Go") {
		t.Errorf("generic reason was not replaced with specific skill citation: %q", got)
	}

	// Specific accurate reason is preserved.
	good := "Brings PostgreSQL query tuning to the start of the bullet to match the role."
	if got := verifyReason("en", "experience", "- Tuned queries", "- Tuned PostgreSQL queries", good, matched); got != good {
		t.Errorf("good reason was unexpectedly altered: got %q, want %q", got, good)
	}
}

func TestRulesSuggestCitesExactMatchedSkills(t *testing.T) {
	req := Request{
		Language: "en",
		Job:      models.Job{PositionTitle: "Backend Engineer", CompanyName: "Acme"},
		Analysis: analyze.Result{
			Required: analyze.Split{
				Tech: analyze.Group{
					Matched: []analyze.Match{{Skill: "Go"}, {Skill: "SQL"}},
				},
			},
		},
		Targets: []Target{
			{Section: "skills", Original: "Python, Go, SQL"},
			{Section: "experience", Original: "- Wrote internal docs\n- Built Go microservices with SQL"},
		},
	}

	drafts, err := (Rules{}).Suggest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 2 {
		t.Fatalf("got %d drafts, want 2", len(drafts))
	}
	if !strings.Contains(drafts[0].Reason, "Go") || !strings.Contains(drafts[0].Reason, "SQL") {
		t.Errorf("skills reason should cite Go and SQL, got %q", drafts[0].Reason)
	}
	if !strings.Contains(drafts[1].Reason, "Go") || !strings.Contains(drafts[1].Reason, "SQL") {
		t.Errorf("experience reason should cite Go and SQL, got %q", drafts[1].Reason)
	}
}
