package render

import (
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
