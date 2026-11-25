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
