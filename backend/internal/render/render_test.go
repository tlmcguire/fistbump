package render

import (
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func TestApply(t *testing.T) {
	base := "# Ann\n\nann@x.com\n\n## Experience\n\nWrote code.\n\n## Skills\n\ngo, sql\n"
	got := Apply(base, []Change{
		{Original: "Wrote code.", Final: "Wrote great code."},
		{Original: "", Final: "A summary."},
		{Original: "go, sql", Final: "sql, go"},
	})
	for _, want := range []string{"Wrote great code.", "## Summary\n\nA summary.", "sql, go"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Index(got, "## Summary") > strings.Index(got, "## Experience") {
		t.Error("summary should come before experience")
	}
}

func TestMasterRendersVerbatimText(t *testing.T) {
	d, i := "Built APIs.", "Cut latency 40%."
	m := models.Master{
		Profile:     &models.Profile{FullName: "Ann Lee"},
		Experiences: []models.Experience{{JobTitle: "Dev", CompanyName: "Acme", Description: &d, Impact: &i, Skills: []string{"go", "sql"}}},
	}
	out := Master(m)
	for _, want := range []string{"# Ann Lee", "### Dev, Acme", d, i, "## Skills\n\ngo, sql"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRevertRoundTrip(t *testing.T) {
	base := "# Ann\n\n## Experience\n\nWrote code.\n\n### Sub\n\n## Skills\n\ngo, sql\n"
	ch := []Change{{Original: "Wrote code.", Final: "Wrote great code."}, {Final: "A summary."}, {Original: "go, sql", Final: "sql, go"}}
	tail := Apply(base, ch)
	if got := Revert(tail, ch); got != base {
		t.Fatalf("Revert = %q, want %q", got, base)
	}
}

func TestApplySummaryIgnoresSubheadings(t *testing.T) {
	got := Apply("# Ann\n\n### Only a subheading\n\n## Experience\n", []Change{{Final: "S."}})
	if !strings.Contains(got, "## Summary\n\nS.\n\n## Experience") {
		t.Fatalf("got %q", got)
	}
}

func TestFormatDate(t *testing.T) {
	for in, want := range map[string]string{"2022-03": "Mar 2022", "2022-12-01": "Dec 2022", "2022": "2022", "": ""} {
		if got := FormatDate(in); got != want {
			t.Errorf("FormatDate(%q) = %q", in, got)
		}
	}
}
