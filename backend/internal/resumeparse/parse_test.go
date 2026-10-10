package resumeparse

import (
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/pdf"
)

const sample = `Jordan Rivera
jordan.rivera@example.com | (512) 555-0142 | Austin, TX | linkedin.com/in/jrivera

SUMMARY
Backend engineer with 6 years building payment systems.

EXPERIENCE
Senior Software Engineer | Initech | Jan 2021 – Present
Austin, TX
• Designed Go microservices handling card payments
• Cut p99 latency by 40% by adding Redis caching
  across checkout services
• Mentored four engineers

Globex Corporation
Software Developer    06/2017 - 12/2020
- Built REST APIs in Python and PostgreSQL
- Migrated 120 services to Kubernetes

EDUCATION
University of Texas at Austin
B.S. in Computer Science, May 2017, GPA: 3.7
Cum Laude

SKILLS
Languages: Go, Python, SQL
Tools: Docker, Kubernetes, Terraform, Redis

CERTIFICATIONS
AWS Certified Solutions Architect
`

func TestParseSample(t *testing.T) {
	d := Parse(sample)
	p := d.Profile
	if p.FullName != "Jordan Rivera" || p.Email == nil || *p.Email != "jordan.rivera@example.com" || p.Phone == nil || p.HomeLocation == nil || *p.HomeLocation != "Austin, TX" {
		t.Fatalf("profile = %+v", p)
	}
	if p.Summary == nil || !strings.Contains(*p.Summary, "payment systems") {
		t.Fatalf("summary = %v", p.Summary)
	}
	if len(d.Experiences) != 2 {
		t.Fatalf("experiences = %+v", d.Experiences)
	}
	e := d.Experiences[0]
	if e.JobTitle != "Senior Software Engineer" || e.CompanyName != "Initech" || *e.StartDate != "2021-01" || e.EndDate != nil || e.Location == nil {
		t.Fatalf("exp0 = %+v", e)
	}
	if e.Impact == nil || !strings.Contains(*e.Impact, "40% by adding Redis caching across checkout services") {
		t.Fatalf("impact (wrapped bullet) = %v", e.Impact)
	}
	if e.Description == nil || !strings.Contains(*e.Description, "Mentored four engineers") {
		t.Fatalf("description = %v", e.Description)
	}
	e = d.Experiences[1]
	if e.JobTitle != "Software Developer" || e.CompanyName != "Globex Corporation" || *e.StartDate != "2017-06" || *e.EndDate != "2020-12" {
		t.Fatalf("exp1 = %+v", e)
	}
	if !containsFold(e.Skills, "python") || !containsFold(e.Skills, "kubernetes") {
		t.Fatalf("exp1 skills = %v", e.Skills)
	}
	if len(d.Education) != 1 {
		t.Fatalf("education = %+v", d.Education)
	}
	ed := d.Education[0]
	if ed.Institution != "University of Texas at Austin" || ed.DegreeLevel == nil || *ed.DegreeLevel != "B.S." || ed.Discipline == nil || *ed.Discipline != "Computer Science" ||
		ed.GPA == nil || *ed.GPA != 3.7 || ed.GraduationDate == nil || *ed.GraduationDate != "2017-05" || ed.Honors == nil {
		t.Fatalf("education = %+v discipline=%v", ed, ed.Discipline)
	}
	for _, s := range []string{"go", "python", "sql", "docker", "terraform"} {
		if !containsFold(d.Skills, s) {
			t.Errorf("skills missing %q: %v", s, d.Skills)
		}
	}
	if len(p.ClearanceCerts) != 1 || p.TargetPositions[0] != "Senior Software Engineer" {
		t.Fatalf("certs=%v targets=%v", p.ClearanceCerts, p.TargetPositions)
	}
}

func TestPDFRoundTrip(t *testing.T) {
	md := "# Jordan Rivera\n\njordan@example.com | Austin, TX\n\n## Experience\n\n### Software Engineer, Initech\n\n2021-01 – Present | Austin, TX\n\n- Built Go services\n- Cut latency 40%\n\n## Skills\n\ngo, sql\n"
	data, err := pdf.Render(md)
	if err != nil {
		t.Fatal(err)
	}
	text, err := TextFromPDF(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Jordan Rivera", "jordan@example.com", "Built Go services", "EXPERIENCE"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in extracted text:\n%s", want, text)
		}
	}
	d := Parse(text)
	if d.Profile.FullName != "Jordan Rivera" || len(d.Experiences) != 1 || !containsFold(d.Skills, "go") {
		t.Fatalf("draft = %+v\ntext:\n%s", d, text)
	}
}

func TestScannedPDF(t *testing.T) {
	if _, err := TextFromPDF([]byte("%PDF-1.4 not really")); err == nil {
		t.Fatal("expected an error")
	}
}

const studentSample = `Alex Moreno

Columbia, SC, 29201 • alex@example.org • github.com/amoreno

Statement of Purpose
I am seeking software engineering internship opportunities.

Professional Experience
Software Engineering Intern
Palmetto Robotics Inc., Columbia, SC
May 2026–present
Developing Go services for device telemetry and data ingestion.

Research Assistant
University of South Carolina, Columbia, SC
March 2024–present
Built a static analysis pipeline in Python to flag risky dependencies
and visualized results with Dash.

Education
University of South Carolina Honors, Columbia, SC
Expected Spring 2027 (GPA: 3.9)
B.S. Computer Science
B.S. Mathematics
Minors in Spanish

Research
DepScan: Finding Vulnerable Dependencies
Under review–
Combined static analysis and LLMs to find vulnerable packages.

Skills
• Python | • git | • Go
• Java | • Rust | • SQL

Extracurricular
Robotics Club
University of South Carolina, Columbia, SC
August 2023–present
Team lead for the autonomy subteam; former treasurer (Apr 2025–present).

Hackathon
University of South Carolina, Columbia, SC
November 2025
Built a scheduling app with a team of four. Placed 2nd.
`

func TestParseStudentLayout(t *testing.T) {
	d := Parse(studentSample)
	if d.Profile.FullName != "Alex Moreno" || d.Profile.HomeLocation == nil || *d.Profile.HomeLocation != "Columbia, SC" || d.Profile.Summary == nil {
		t.Fatalf("profile = %+v", d.Profile)
	}
	type row struct{ title, org, start, end string }
	want := []row{
		{"Software Engineering Intern", "Palmetto Robotics Inc.", "2026-05", ""},
		{"Research Assistant", "University of South Carolina", "2024-03", ""},
		{"DepScan: Finding Vulnerable Dependencies", "Research", "", ""},
		{"Robotics Club", "University of South Carolina", "2023-08", ""},
		{"Hackathon", "University of South Carolina", "2025-11", "2025-11"},
	}
	if len(d.Experiences) != len(want) {
		for _, e := range d.Experiences {
			t.Logf("got %q | %q", e.JobTitle, e.CompanyName)
		}
		t.Fatalf("experiences = %d, want %d", len(d.Experiences), len(want))
	}
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	for i, w := range want {
		e := d.Experiences[i]
		if e.JobTitle != w.title || e.CompanyName != w.org || str(e.StartDate) != w.start || str(e.EndDate) != w.end {
			t.Errorf("exp %d = %q | %q | %s-%s, want %+v", i, e.JobTitle, e.CompanyName, str(e.StartDate), str(e.EndDate), w)
		}
	}
	// Paragraph descriptions stay paragraphs, wrapped lines joined.
	if got := str(d.Experiences[1].Description); got != "Built a static analysis pipeline in Python to flag risky dependencies and visualized results with Dash." {
		t.Errorf("description = %q", got)
	}
	// Skills: the company name does not make "robotics" or "security" a skill; unused skills go to the profile.
	if !containsFold(d.Experiences[0].Skills, "go") || containsFold(d.Experiences[0].Skills, "java") {
		t.Errorf("intern skills = %v", d.Experiences[0].Skills)
	}
	if !containsFold(d.Profile.Skills, "java") || !containsFold(d.Profile.Skills, "rust") || containsFold(d.Profile.Skills, "python") {
		t.Errorf("profile skills = %v", d.Profile.Skills)
	}
	if len(d.Education) != 2 {
		t.Fatalf("education = %+v", d.Education)
	}
	for i, disc := range []string{"Computer Science", "Mathematics"} {
		ed := d.Education[i]
		if ed.Institution != "University of South Carolina" || str(ed.Discipline) != disc || str(ed.GraduationDate) != "2027-05" || ed.GPA == nil || *ed.GPA != 3.9 || str(ed.Honors) != "Honors College" {
			t.Errorf("edu %d = %+v disc=%q", i, ed, str(ed.Discipline))
		}
	}
	if len(d.Warnings) != 0 {
		t.Errorf("warnings = %v", d.Warnings)
	}
}

func TestHeaderLinksKeptAsWritten(t *testing.T) {
	d := Parse("Tyler McGuire\nCharleston, SC · work@mcguire.one · linkedin.com/in/tlmcguire7 · github.com/tlmcguire · Digital portfolio: mcguire.one\n\nEXPERIENCE\nIntern\nAcme, Charleston, SC\nMay 2026 – Present\n- Built things.")
	want := []string{"linkedin.com/in/tlmcguire7", "github.com/tlmcguire", "mcguire.one"}
	if strings.Join(d.Profile.Links, " ") != strings.Join(want, " ") {
		t.Fatalf("links = %q, want %q", d.Profile.Links, want)
	}
	if d.Profile.Email == nil || *d.Profile.Email != "work@mcguire.one" || d.Profile.HomeLocation == nil || *d.Profile.HomeLocation != "Charleston, SC" {
		t.Fatalf("contact = %v %v", d.Profile.Email, d.Profile.HomeLocation)
	}
	if d.Profile.FullName != "Tyler McGuire" {
		t.Fatalf("name = %q", d.Profile.FullName)
	}
}
