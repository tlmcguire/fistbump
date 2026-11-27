package resumeparse

import (
	"strings"
	"testing"
)

func TestResumeParserImprovements(t *testing.T) {
	resume := `Jane Smith, PhD
jane@example.com · Charleston, SC · github.com/janesmith

**Engineering Experience**
Acme Corp · Senior Software Engineer · Austin, TX
Jun '22 – Ongoing
➢ Built distributed data pipelines for real-time develop-
ment and analytics reducing latency by 40%.

--- Open Source ---
Fistbump Assistant
▸ Maintained local-first job application tools in Go.

--- Programming Languages ---
Languages & Tools: Go, TypeScript, SQL, Docker

Education
State University
Bachelor of Science in Computer Science
GPA: 3.92 / 4.0 · 2018 - 2022
`
	d := Parse(resume)
	if d.Profile.FullName != "Jane Smith, PhD" {
		t.Errorf("FullName = %q, want %q", d.Profile.FullName, "Jane Smith, PhD")
	}
	if len(d.Experiences) < 2 {
		t.Fatalf("expected at least 2 experiences (work + open source), got %d", len(d.Experiences))
	}
	work := d.Experiences[0]
	if work.StartDate == nil || *work.StartDate != "2022-06-01" {
		t.Errorf("work.StartDate = %v, want 2022-06-01", work.StartDate)
	}
	if work.EndDate != nil {
		t.Errorf("work.EndDate = %v, want nil for Ongoing", *work.EndDate)
	}
	combined := ""
	if work.Description != nil {
		combined += *work.Description
	}
	if work.Impact != nil {
		combined += *work.Impact
	}
	if !strings.Contains(combined, "development") {
		t.Errorf("expected hyphenated line break to rejoin 'development', got %q", combined)
	}

	oss := d.Experiences[1]
	if !oss.DateNA {
		t.Errorf("expected undated Open Source project to have DateNA=true")
	}

	if len(d.Skills) < 4 {
		t.Errorf("expected at least 4 parsed skills, got %v", d.Skills)
	}
	if len(d.Education) != 1 {
		t.Fatalf("expected 1 education entry, got %d", len(d.Education))
	}
	if d.Education[0].GPA == nil || *d.Education[0].GPA != 3.92 {
		t.Errorf("GPA = %v, want 3.92", d.Education[0].GPA)
	}
}

func TestParseInlineSingleDatesMultiDegreeAndShortYearSeason(t *testing.T) {
	resume := `Tyler McGuire
Charleston, SC · work@mcguire.one

Education
College of Charleston Honors, Charleston, SC  |  Expected Spring 2027
B.S. Computer Science; B.S. Software Engineering — Minors in Data Science, Spanish — GPA: 3.84

Research & Projects
AISAI: Automated Identification of Software Supply Chain Vulnerability  |  Spring '26
• Built a vulnerability detection tool combining static analysis and LLMs.

Extracurricular
Cybersecurity Club — College of Charleston, Charleston, SC  |  August 2023 – Present
• Defense team member.

International Collegiate Programming Competition — College of Charleston  |  November 2025
• Placed 2nd.

Skills
Go, Python, AWS (EKS)
`
	d := Parse(resume)
	if len(d.Education) != 2 {
		t.Fatalf("expected 2 education entries for dual degree, got %d: %+v", len(d.Education), d.Education)
	}
	if d.Education[0].Discipline == nil || *d.Education[0].Discipline != "Computer Science" {
		t.Errorf("edu[0].Discipline = %v, want Computer Science", d.Education[0].Discipline)
	}
	if d.Education[1].Discipline == nil || *d.Education[1].Discipline != "Software Engineering" {
		t.Errorf("edu[1].Discipline = %v, want Software Engineering", d.Education[1].Discipline)
	}
	if d.Education[1].Details == nil || !strings.Contains(*d.Education[1].Details, "Minors in Data Science, Spanish") {
		t.Errorf("expected minors preserved in Details, got %v", d.Education[1].Details)
	}
	if len(d.Experiences) != 3 {
		t.Fatalf("expected 3 activity/project experiences, got %d: %+v", len(d.Experiences), d.Experiences)
	}
	aisai := d.Experiences[0]
	if aisai.StartDate == nil || *aisai.StartDate != "2026-03-01" || aisai.EndDate == nil || *aisai.EndDate != "2026-05-01" {
		t.Errorf("AISAI dates = %v to %v, want 2026-03-01 to 2026-05-01", aisai.StartDate, aisai.EndDate)
	}
	icpc := d.Experiences[2]
	if !strings.Contains(icpc.JobTitle, "International Collegiate Programming Competition") {
		t.Errorf("expected ICPC as separate experience, got %+v", icpc)
	}
	if icpc.StartDate == nil || *icpc.StartDate != "2025-11-01" {
		t.Errorf("ICPC StartDate = %v, want 2025-11-01", icpc.StartDate)
	}
}
