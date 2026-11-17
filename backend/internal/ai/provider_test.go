package ai

import (
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/render"
)

func ptr[T any](v T) *T { return &v }

func testMaster() models.Master {
	return models.Master{
		Profile: &models.Profile{FullName: "Ada", Skills: []string{"excel", "python", "sql"}},
		Experiences: []models.Experience{{
			ID: 7, CompanyName: "Acme", JobTitle: "Data Analyst",
			Description: ptr("Built dashboards.\nWrote SQL."), Impact: ptr("# Cut reporting time 40%"),
		}},
	}
}

func TestBuildTargets(t *testing.T) {
	got := BuildTargets(testMaster())
	if len(got) != 4 {
		t.Fatalf("got %d targets, want summary, description, impact and skills: %+v", len(got), got)
	}
	if got[0].Section != "summary" || got[0].Original != "" {
		t.Errorf("summary target = %+v", got[0])
	}
	if d := got[1]; d.TargetID == nil || *d.TargetID != 7 || d.Label != "Data Analyst at Acme (responsibilities)" {
		t.Errorf("description target = %+v", d)
	}
	if got[2].Original != `\# Cut reporting time 40%` {
		t.Errorf("impact must be escaped as render prints it, got %q", got[2].Original)
	}
	if got[3].Section != "skills" || got[3].Original != "excel, python, sql" {
		t.Errorf("skills target = %+v", got[3])
	}
}

func TestBuildTargetsSkipsBlankText(t *testing.T) {
	m := testMaster()
	m.Experiences[0].Description, m.Experiences[0].Impact = ptr("  "), nil
	m.Profile.Skills = nil
	got := BuildTargets(m)
	if len(got) != 1 || got[0].Section != "summary" {
		t.Errorf("only the summary target should remain, got %+v", got)
	}
}

// Every target's text must appear in the rendered resume, or render.Apply could not find it.
func TestTargetsAppearInRender(t *testing.T) {
	m := testMaster()
	m.Profile.Summary = ptr("  Analyst.  ")
	rendered := render.Master(m)
	for _, target := range BuildTargets(m) {
		if !strings.Contains(rendered, target.Original) {
			t.Errorf("%s: %q not found in the rendered resume", target.Label, target.Original)
		}
	}
}
