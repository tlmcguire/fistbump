package resumeparse

import (
	"os"
	"slices"
	"strings"
	"testing"
)

const plainResume = `Jane Q. Public
jane@example.org | (843) 555-0100 | Charleston, SC

SUMMARY
Backend developer.

EXPERIENCE
Software Engineer, Globex Inc.
Jan 2020 - Present
- Built APIs in Go
- Cut latency 30%

EDUCATION
B.S. Computer Science, College of Charleston
May 2019

SKILLS
Go, SQL, Docker

CERTIFICATIONS
Security+`

func TestParsePlainResume(t *testing.T) {
	d := Parse(plainResume)
	p := d.Profile

	if p.FullName != "Jane Q. Public" || *p.Email != "jane@example.org" || *p.Phone != "(843) 555-0100" || *p.HomeLocation != "Charleston, SC" {
		t.Errorf("header = %q %v %v %v", p.FullName, p.Email, p.Phone, p.HomeLocation)
	}
	if p.Summary == nil || *p.Summary != "Backend developer." {
		t.Errorf("summary = %v", p.Summary)
	}
	if len(d.Experiences) != 1 {
		t.Fatalf("experiences = %+v", d.Experiences)
	}
	e := d.Experiences[0]
	if e.JobTitle != "Software Engineer" || e.CompanyName != "Globex Inc." || *e.StartDate != "2020-01-01" || e.EndDate != nil {
		t.Errorf("experience = %q at %q, %v to %v", e.JobTitle, e.CompanyName, e.StartDate, e.EndDate)
	}
	if text := *e.Description + " " + *e.Impact; !strings.Contains(text, "Built APIs in Go") || !strings.Contains(text, "Cut latency 30%") {
		t.Errorf("both bullets must be kept: %q / %q", *e.Description, *e.Impact)
	}
	if len(d.Education) != 1 || d.Education[0].Institution != "College of Charleston" ||
		*d.Education[0].DegreeLevel != "B.S." || *d.Education[0].Discipline != "Computer Science" || *d.Education[0].GraduationDate != "2019-05-01" {
		t.Errorf("education = %+v", d.Education)
	}
	if !slices.Equal(d.Skills, []string{"Go", "SQL", "Docker"}) || !slices.Equal(p.Skills, d.Skills) {
		t.Errorf("skills = %q, profile skills = %q", d.Skills, p.Skills)
	}
	if !slices.Equal(p.ClearanceCerts, []string{"Security+"}) || !slices.Equal(p.TargetPositions, []string{"Software Engineer"}) {
		t.Errorf("certs = %q, target positions = %q", p.ClearanceCerts, p.TargetPositions)
	}
	if len(d.Warnings) != 0 {
		t.Errorf("a complete resume should give no warnings: %q", d.Warnings)
	}
}

// What render.Master writes must import cleanly, so a resume exported as Markdown can come back.
func TestParseMarkdownResume(t *testing.T) {
	md, err := os.ReadFile("../../testdata/render/master.md")
	if err != nil {
		t.Fatal(err)
	}
	d := Parse(string(md))
	p := d.Profile

	if p.FullName != "Ada Lovelace" || *p.Email != "ada@example.com" || *p.HomeLocation != "Charleston, SC" || !slices.Equal(p.Links, []string{"github.com/ada"}) {
		t.Errorf("header = %q %v %v %q", p.FullName, p.Email, p.HomeLocation, p.Links)
	}
	if *p.Summary != "Analyst with five years of experience." {
		t.Errorf("summary = %q", *p.Summary)
	}
	if len(d.Experiences) != 1 || d.Experiences[0].JobTitle != "Data Analyst" || d.Experiences[0].CompanyName != "Acme" ||
		*d.Experiences[0].StartDate != "2022-06-01" || *d.Experiences[0].Location != "Charleston, SC" {
		t.Errorf("experience = %+v", d.Experiences)
	}
	if len(d.Education) != 1 || d.Education[0].Institution != "College of Charleston" || *d.Education[0].GPA != 3.8 {
		t.Errorf("education = %+v", d.Education)
	}
	if !slices.Equal(d.Skills, []string{"sql", "excel", "python"}) {
		t.Errorf("skills = %q", d.Skills)
	}
	if len(d.Warnings) != 0 || strings.Contains(p.FullName+d.Experiences[0].JobTitle, "#") {
		t.Errorf("warnings = %q; heading marks must not reach the data", d.Warnings)
	}
}

func TestMarkdownLine(t *testing.T) {
	cases := map[string]string{
		"# Ada Lovelace":        "Ada Lovelace",
		"### Analyst, Acme":     "Analyst, Acme",
		"###### Deep":           "Deep",
		`\- Cut reporting time`: "- Cut reporting time", // render.Escape puts a backslash before a leading - # or *
		`\# not a heading`:      "# not a heading",
		`\* starred`:            "* starred",
		"#hashtag":              "#hashtag", // no space after the mark: not a heading
		"C# developer":          "C# developer",
		`\n stays`:              `\n stays`,
		"- a real bullet":       "- a real bullet",
		"":                      "",
	}
	for in, want := range cases {
		if got := markdownLine(in); got != want {
			t.Errorf("markdownLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseEmptyText(t *testing.T) {
	d := Parse("")
	if d.Experiences == nil || d.Education == nil || d.Skills == nil || d.Warnings == nil ||
		d.Profile.TargetPositions == nil || d.Profile.ClearanceCerts == nil {
		t.Error("lists must be empty, never nil, so the API sends [] and not null")
	}
	want := []string{"Could not find your name", "No work experience", "No skills section"}
	for _, w := range want {
		if !slices.ContainsFunc(d.Warnings, func(s string) bool { return strings.Contains(s, w) }) {
			t.Errorf("warnings %q should mention %q", d.Warnings, w)
		}
	}
}

func TestParseSeveralJobsKeepTheirOrder(t *testing.T) {
	d := Parse("Jane Doe\n\nEXPERIENCE\nSenior Developer, Initech\nMar 2021 - Present\nLed a team of four.\n\nDeveloper, Globex\nJun 2018 - Feb 2021\nShipped features.\n")
	if len(d.Experiences) != 2 {
		t.Fatalf("experiences = %+v", d.Experiences)
	}
	first, second := d.Experiences[0], d.Experiences[1]
	if first.CompanyName != "Initech" || second.CompanyName != "Globex" || first.SortOrder != 0 || second.SortOrder != 1 {
		t.Errorf("order = %s (%d), %s (%d)", first.CompanyName, first.SortOrder, second.CompanyName, second.SortOrder)
	}
	if *second.StartDate != "2018-06-01" || *second.EndDate != "2021-02-01" || first.EndDate != nil {
		t.Errorf("dates = %v to %v, %v to %v", first.StartDate, first.EndDate, second.StartDate, second.EndDate)
	}
}

func TestParseEducationDetails(t *testing.T) {
	d := Parse("Jane Doe\n\nEDUCATION\nCollege of Charleston\nB.S. Computer Science, May 2019\nGPA: 3.85\nMagna Cum Laude\n")
	if len(d.Education) != 1 {
		t.Fatalf("education = %+v", d.Education)
	}
	e := d.Education[0]
	if e.Institution != "College of Charleston" || *e.DegreeLevel != "B.S." || *e.Discipline != "Computer Science" ||
		*e.GraduationDate != "2019-05-01" || *e.GPA != 3.85 || *e.Honors != "Magna Cum Laude" {
		t.Errorf("education = %+v", e)
	}
}

func TestParseSkillsSplitOnEverySeparator(t *testing.T) {
	d := Parse("Jane Doe\n\nSKILLS\n• Go, SQL; Docker | Kubernetes\n- Python\nProgramming: Rust\n")
	want := []string{"Go", "SQL", "Docker", "Kubernetes", "Python", "Rust"}
	if !slices.Equal(d.Skills, want) {
		t.Errorf("skills = %q, want %q", d.Skills, want)
	}
}

func TestParseActivitiesAreImported(t *testing.T) {
	d := Parse("Jane Doe\n\nEXPERIENCE\nAnalyst, Acme\n2019 - 2021\n- Did things\n\nPROJECTS\nInventory Tracker\nMar 2021 - Jun 2021\n- Built a tracker in Go\n\nEXTRACURRICULAR\nCybersecurity Club \u2014 State College, Charleston, SC | August 2023 \u2013 Present\n- Defense team member\n")
	if len(d.Experiences) != 3 || d.Experiences[0].JobTitle != "Analyst" {
		t.Fatalf("experiences = %+v", d.Experiences)
	}
	if e := d.Experiences[1]; e.JobTitle != "Inventory Tracker" || e.CompanyName != "Projects" {
		t.Errorf("project = %+v", e)
	}
	if e := d.Experiences[2]; e.JobTitle != "Cybersecurity Club" || e.CompanyName != "State College" || e.StartDate == nil || *e.StartDate != "2023-08-01" {
		t.Errorf("club = %+v", e)
	}
	for _, w := range d.Warnings {
		if strings.Contains(w, "Inventory") || strings.Contains(w, "Cybersecurity") {
			t.Errorf("activity text was reported as unplaced: %q", w)
		}
	}
}

func TestParseFindsClearances(t *testing.T) {
	d := Parse("Jane Doe\nActive TS/SCI clearance\n\nEXPERIENCE\nAnalyst, Acme\n2019 - 2021\n- Did things\n")
	if !slices.Equal(d.Profile.ClearanceCerts, []string{"TS/SCI"}) {
		t.Errorf("certs = %q", d.Profile.ClearanceCerts)
	}
}

func TestParseLineEndings(t *testing.T) {
	if d := Parse("Jane Doe\r\n\r\nSKILLS\r\nGo\r\n"); d.Profile.FullName != "Jane Doe" || !slices.Equal(d.Skills, []string{"Go"}) {
		t.Errorf("CRLF text: name %q, skills %q", d.Profile.FullName, d.Skills)
	}
}

func TestIsJobTitle(t *testing.T) {
	for _, s := range []string{"Data Analyst", "Software Engineer", "Intern", "Senior Product Manager"} {
		if !IsJobTitle(s) {
			t.Errorf("IsJobTitle(%q) = false", s)
		}
	}
	for _, s := range []string{"Charleston, SC", "Acme Inc."} {
		if IsJobTitle(s) {
			t.Errorf("IsJobTitle(%q) = true", s)
		}
	}
}
