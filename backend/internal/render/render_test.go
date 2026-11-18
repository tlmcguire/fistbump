package render

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func ptr[T any](v T) *T { return &v }

func fullMaster() models.Master {
	return models.Master{
		Profile: &models.Profile{
			FullName: "Ada Lovelace", Email: ptr("ada@example.com"), HomeLocation: ptr("Charleston, SC"),
			Links: []string{"github.com/ada"}, Summary: ptr("Analyst with five years of experience."),
			Skills: []string{"sql", "excel"},
		},
		Experiences: []models.Experience{{
			CompanyName: "Acme", JobTitle: "Data Analyst", StartDate: ptr("2022-06-01"), Location: ptr("Charleston, SC"),
			Description: ptr("Built weekly reports for finance."), Impact: ptr("Cut reporting time by 40%."),
			Skills: []string{"SQL", "python"},
		}},
		Education: []models.Education{{
			Institution: "College of Charleston", DegreeLevel: ptr("BS"), Discipline: ptr("Computer Science"),
			GraduationDate: ptr("2022-05-01"), GPA: ptr(3.8),
		}},
	}
}

func TestMasterGolden(t *testing.T) {
	want, err := os.ReadFile("../../testdata/render/master.md")
	if err != nil {
		t.Fatal(err)
	}
	// Git on Windows may check the golden file out with CRLF line endings.
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if got := Master(fullMaster()); got != string(want) {
		t.Errorf("render differs from testdata/render/master.md:\n%s", got)
	}
}

func TestEmptySectionsLeftOut(t *testing.T) {
	got := Master(models.Master{Profile: &models.Profile{FullName: "Ada"}})
	if got != "# Ada\n" {
		t.Errorf("got %q, want only the name", got)
	}
	if Master(models.Master{}) != "" {
		t.Error("no profile and no entries should render nothing")
	}
}

func TestDescriptionVerbatim(t *testing.T) {
	m := fullMaster()
	desc := "Led a team of 4.\nShipped the data warehouse   on time."
	m.Experiences[0].Description = &desc
	if !strings.Contains(Master(m), desc) {
		t.Error("description must appear exactly as stored")
	}
}

func TestEscapesLineStarts(t *testing.T) {
	m := fullMaster()
	m.Experiences[0].Description = ptr("# not a heading\nplain")
	if got := Master(m); !strings.Contains(got, "\\# not a heading\nplain") {
		t.Errorf("leading # not escaped:\n%s", got)
	}
}

func TestOnlyOneTrailingNewline(t *testing.T) {
	got := Master(fullMaster())
	if !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("must end with exactly one newline: %q", got[len(got)-5:])
	}
}

func TestApplyReplacesFirstOccurrence(t *testing.T) {
	base := "## Skills\n\nexcel, sql\n\n## Other\n\nexcel, sql\n"
	got, missed := Apply(base, []Change{{Original: "excel, sql", Final: "sql, excel"}})
	want := "## Skills\n\nsql, excel\n\n## Other\n\nexcel, sql\n"
	if got != want || len(missed) != 0 {
		t.Errorf("got %q, missed %v", got, missed)
	}
}

func TestApplyInsertsNewSummaryBeforeFirstSection(t *testing.T) {
	base := "# Ada\n\nada@example.com\n\n## Experience\n\n### Analyst, Acme\n"
	got, missed := Apply(base, []Change{{Original: "", Final: "Analyst with SQL."}})
	want := "# Ada\n\nada@example.com\n\n## Summary\n\nAnalyst with SQL.\n\n## Experience\n\n### Analyst, Acme\n"
	if got != want || len(missed) != 0 {
		t.Errorf("got %q, missed %v", got, missed)
	}
}

func TestApplyNewSummaryWithoutSections(t *testing.T) {
	got, _ := Apply("# Ada\n", []Change{{Final: "Analyst."}})
	if got != "# Ada\n\n## Summary\n\nAnalyst.\n" {
		t.Errorf("got %q", got)
	}
}

func TestApplyReportsChangesNotFound(t *testing.T) {
	base := "## Skills\n\nsql\n"
	changes := []Change{
		{Original: "gone text", Final: "new"},
		{Original: "sql", Final: "sql, excel"},
	}
	got, missed := Apply(base, changes)
	if got != "## Skills\n\nsql, excel\n" {
		t.Errorf("found changes must still apply, got %q", got)
	}
	if len(missed) != 1 || missed[0].Original != "gone text" {
		t.Errorf("missed = %v, want the one absent change", missed)
	}
}

func TestApplyKeepsExistingSummary(t *testing.T) {
	base := "## Summary\n\nOld.\n"
	got, missed := Apply(base, []Change{{Final: "New."}})
	if got != base || len(missed) != 1 {
		t.Errorf("a base with a summary must not get a second one: %q, %v", got, missed)
	}
}

func TestApplyWithNoChanges(t *testing.T) {
	if got, missed := Apply("text\n", nil); got != "text\n" || missed != nil {
		t.Errorf("got %q, %v", got, missed)
	}
}

func TestApplyPrefersTheWholeBlock(t *testing.T) {
	base := "### Analyst, Acme\n\nUsed excel, sql daily.\n\n## Skills\n\nexcel, sql\n"
	got, missed := Apply(base, []Change{{Original: "excel, sql", Final: "sql, excel"}})
	want := "### Analyst, Acme\n\nUsed excel, sql daily.\n\n## Skills\n\nsql, excel\n"
	if got != want || len(missed) != 0 {
		t.Errorf("the skills block must change, not the sentence before it:\n%q", got)
	}
}

func TestApplyFallsBackToALineThenAnyText(t *testing.T) {
	line := "Built reports.\nexcel, sql\nShipped.\n"
	if got, _ := Apply("Used excel, sql daily.\n\n"+line, []Change{{Original: "excel, sql", Final: "X"}}); !strings.Contains(got, "Built reports.\nX\nShipped.") {
		t.Errorf("a whole line beats the middle of a sentence: %q", got)
	}
	if got, _ := Apply("Used excel, sql daily.\n", []Change{{Original: "excel, sql", Final: "X"}}); got != "Used X daily.\n" {
		t.Errorf("with nothing better the first occurrence is used: %q", got)
	}
}
