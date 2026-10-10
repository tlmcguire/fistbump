package resumeparse

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/pdf"
	"github.com/tlmcguire/fistbump/backend/internal/render"
)

func sp(s string) *string { return &s }

func val(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// A resume exported by the app must import back with the same content: Unicode text, links,
// certifications, company suffixes, entries on a second page, and user lines that start with "#".
func TestExportImportRoundTrip(t *testing.T) {
	gpa := 3.62
	m := models.Master{
		Profile: &models.Profile{FullName: "Zoë Łukasiewicz-Núñez", Email: sp("zoe.ln@example.com"), Phone: sp("(843) 555-0199"), HomeLocation: sp("Charleston, SC"),
			Links:   []string{"linkedin.com/in/zoeln", "github.com/zoeln", "zoe.dev"},
			Summary: sp("Engineer who “ships” reliable systems → fewer pages."), ClearanceCerts: []string{"AWS Certified Developer – Associate"}},
		Experiences: []models.Experience{
			{ID: 1, JobTitle: "Senior Software Engineer", CompanyName: "Blue Ridge Labs, Inc.", StartDate: sp("2022-03"), Location: sp("Remote"),
				Description: sp("- Built Go services handling 2M requests/day\n- # of incidents cut in half\n- Docs at https://example.com/a/very/long/path/that/should/wrap/somewhere/in/the/pdf/output/index.html"),
				Impact:      sp("- Reduced p99 latency 40%"), Skills: []string{"go"}},
		},
		Education: []models.Education{{ID: 1, Institution: "College of Charleston", DegreeLevel: sp("B.S."), Discipline: sp("Computer Science"), GraduationDate: sp("2019-05"), GPA: &gpa, Honors: sp("Magna Cum Laude")}},
	}
	for i := 0; i < 7; i++ { // spill onto a second page
		m.Experiences = append(m.Experiences, models.Experience{ID: int64(10 + i), JobTitle: fmt.Sprintf("Research Assistant %d", i), CompanyName: "Some University",
			StartDate: sp(fmt.Sprintf("201%d-01", i)), EndDate: sp(fmt.Sprintf("201%d-12", i)), Description: sp("- Studied distributed systems\n- Presented at a regional conference")})
	}
	data, err := pdf.Render(render.Master(m))
	if err != nil {
		t.Fatal(err)
	}
	text, err := TextFromPDF(data)
	if err != nil {
		t.Fatal(err)
	}
	d := Parse(text)
	p := d.Profile
	if p.FullName != m.Profile.FullName || val(p.Email) != "zoe.ln@example.com" || val(p.Phone) != "(843) 555-0199" || val(p.HomeLocation) != "Charleston, SC" {
		t.Errorf("profile = %q %q %q %q", p.FullName, val(p.Email), val(p.Phone), val(p.HomeLocation))
	}
	if !slices.Equal(p.Links, m.Profile.Links) {
		t.Errorf("links = %q", p.Links)
	}
	if val(p.Summary) != val(m.Profile.Summary) {
		t.Errorf("summary = %q", val(p.Summary))
	}
	if !slices.Equal(p.ClearanceCerts, m.Profile.ClearanceCerts) {
		t.Errorf("certs = %q", p.ClearanceCerts)
	}
	if len(d.Experiences) != len(m.Experiences) {
		t.Fatalf("experiences = %d, want %d", len(d.Experiences), len(m.Experiences))
	}
	e := d.Experiences[0]
	if e.JobTitle != "Senior Software Engineer" || e.CompanyName != "Blue Ridge Labs, Inc." || val(e.StartDate) != "2022-03" || e.EndDate != nil || val(e.Location) != "Remote" {
		t.Errorf("first role = %+v", e)
	}
	if !strings.Contains(val(e.Description), "# of incidents") || !strings.Contains(val(e.Description), "/output/index.html") || slices.Contains(e.Skills, "html") {
		t.Errorf("description = %q skills = %v", val(e.Description), e.Skills)
	}
	last := d.Experiences[len(d.Experiences)-1]
	if last.JobTitle != "Research Assistant 6" || val(last.StartDate) != "2016-01" {
		t.Errorf("last role = %q %q", last.JobTitle, val(last.StartDate))
	}
	ed := d.Education[0]
	if ed.Institution != "College of Charleston" || val(ed.Honors) != "Magna Cum Laude" || ed.GPA == nil || *ed.GPA != gpa {
		t.Errorf("education = %+v honors=%q", ed, val(ed.Honors))
	}
	if len(d.Warnings) != 0 {
		t.Errorf("warnings = %v", d.Warnings)
	}
}
