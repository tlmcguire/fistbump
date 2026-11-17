package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/analyze"
	"github.com/tlmcguire/fistbump/backend/internal/models"
)

// matching builds an analysis in which the resume shows the given required tech skills.
func matching(skills ...string) analyze.Result {
	var r analyze.Result
	for _, s := range skills {
		r.Required.Tech.Matched = append(r.Required.Tech.Matched, analyze.Match{Skill: s})
	}
	return r
}

func suggest(t *testing.T, a analyze.Result, targets ...Target) []Draft {
	t.Helper()
	req := Request{Job: models.Job{PositionTitle: "Data Analyst"}, Analysis: a, Targets: targets}
	drafts, err := Rules{}.Suggest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return drafts
}

func TestRulesReorderSkills(t *testing.T) {
	got := suggest(t, matching("sql"), Target{Section: "skills", Original: "excel, python, sql"})
	if len(got) != 1 || got[0].Proposed != "sql, excel, python" || got[0].Engine != EngineRules {
		t.Errorf("got %+v", got)
	}
}

func TestRulesKeepSkillItems(t *testing.T) {
	got := suggest(t, matching("python", "sql"), Target{Section: "skills", Original: "excel, sql, go, python"})
	items := strings.Split(got[0].Proposed, ", ")
	if len(items) != 4 || items[0] != "python" || items[1] != "sql" {
		t.Errorf("got %q: same four items, matched ones first in the job's order", got[0].Proposed)
	}
}

func TestRulesReorderExperienceLines(t *testing.T) {
	id := int64(7)
	got := suggest(t, matching("sql"), Target{
		Section: "experience", TargetID: &id,
		Original: "Built dashboards.\nWrote SQL.\nPresented to execs.",
	})
	if len(got) != 1 || got[0].Proposed != "Wrote SQL.\nBuilt dashboards.\nPresented to execs." {
		t.Fatalf("got %+v", got)
	}
	if got[0].TargetID == nil || *got[0].TargetID != 7 {
		t.Errorf("draft must keep the target id, got %v", got[0].TargetID)
	}
}

func TestRulesSummary(t *testing.T) {
	appended := suggest(t, matching("sql", "excel"), Target{Section: "summary", Original: "Analyst with five years."})
	if want := "Analyst with five years. Relevant strengths for this role: sql, excel."; appended[0].Proposed != want {
		t.Errorf("appended = %q", appended[0].Proposed)
	}
	created := suggest(t, matching("sql"), Target{Section: "summary"})
	if want := "Data Analyst with hands-on experience in sql."; created[0].Proposed != want {
		t.Errorf("created = %q", created[0].Proposed)
	}
}

func TestRulesSummaryUsesFiveSkillsAtMost(t *testing.T) {
	got := suggest(t, matching("a", "b", "c", "d", "e", "f", "g"), Target{Section: "summary"})
	if !strings.HasSuffix(got[0].Proposed, "a, b, c, d, e.") {
		t.Errorf("got %q", got[0].Proposed)
	}
}

func TestRulesProposeNothingWithoutAChange(t *testing.T) {
	targets := []Target{
		{Section: "skills", Original: "sql, excel"},              // already in order
		{Section: "experience", Original: "One line about SQL."}, // single line
		{Section: "experience", Original: "Wrote SQL.\nOther."},  // matched line already first
		{Section: "summary", Original: "Analyst."},               // nothing matched below
	}
	if got := suggest(t, matching("sql"), targets[:3]...); len(got) != 0 {
		t.Errorf("got %+v, want no drafts", got)
	}
	if got := suggest(t, matching(), targets...); len(got) != 0 {
		t.Errorf("with no matched skills got %+v, want no drafts", got)
	}
}

func TestRulesStopWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Rules{}.Suggest(ctx, Request{Targets: []Target{{Section: "skills", Original: "a, b"}}})
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
