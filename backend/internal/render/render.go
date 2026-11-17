// Package render turns the master resume into Markdown, the text format later phases tailor,
// diff and print.
package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

// Master renders the master resume. Description, impact and the skills line appear exactly as
// stored, so suggestions can find and replace them by exact text. Empty sections are left out
// and the output ends with one newline.
func Master(m models.Master) string {
	var blocks []string
	if p := m.Profile; p != nil {
		if name := strings.TrimSpace(p.FullName); name != "" {
			blocks = append(blocks, "# "+Escape(name))
		}
		var contact []string
		for _, v := range []*string{p.HomeLocation, p.Email, p.Phone} {
			if v != nil && strings.TrimSpace(*v) != "" {
				contact = append(contact, strings.TrimSpace(*v))
			}
		}
		contact = append(contact, p.Links...)
		if len(contact) > 0 {
			blocks = append(blocks, strings.Join(contact, " · "))
		}
		if p.Summary != nil && strings.TrimSpace(*p.Summary) != "" {
			blocks = append(blocks, "## Summary", Escape(strings.TrimSpace(*p.Summary)))
		}
	}
	if len(m.Experiences) > 0 {
		blocks = append(blocks, "## Experience")
		for _, e := range m.Experiences {
			blocks = append(blocks, "### "+Escape(e.JobTitle+", "+e.CompanyName))
			meta := []string{}
			if dates := dateRange(e.StartDate, e.EndDate); dates != "" {
				meta = append(meta, dates)
			}
			if e.Location != nil && strings.TrimSpace(*e.Location) != "" {
				meta = append(meta, strings.TrimSpace(*e.Location))
			}
			if len(meta) > 0 {
				blocks = append(blocks, strings.Join(meta, " | "))
			}
			for _, text := range []*string{e.Description, e.Impact} {
				if text != nil && strings.TrimSpace(*text) != "" {
					blocks = append(blocks, Escape(*text))
				}
			}
		}
	}
	if len(m.Education) > 0 {
		blocks = append(blocks, "## Education")
		for _, e := range m.Education {
			heading := e.Institution
			var degree []string
			for _, v := range []*string{e.DegreeLevel, e.Discipline} {
				if v != nil && strings.TrimSpace(*v) != "" {
					degree = append(degree, strings.TrimSpace(*v))
				}
			}
			if len(degree) == 2 {
				heading = degree[0] + " in " + degree[1] + ", " + e.Institution
			} else if len(degree) == 1 {
				heading = degree[0] + ", " + e.Institution
			}
			blocks = append(blocks, "### "+Escape(heading))
			meta := []string{}
			if e.GraduationDate != nil {
				if d := formatDate(*e.GraduationDate); d != "" {
					meta = append(meta, d)
				}
			}
			if e.GPA != nil {
				meta = append(meta, fmt.Sprintf("GPA %.2f", *e.GPA))
			}
			if len(meta) > 0 {
				blocks = append(blocks, strings.Join(meta, " | "))
			}
			for _, text := range []*string{e.Honors, e.Details} {
				if text != nil && strings.TrimSpace(*text) != "" {
					blocks = append(blocks, Escape(*text))
				}
			}
		}
	}
	if skills := m.Skills(); len(skills) > 0 {
		blocks = append(blocks, "## Skills", strings.Join(skills, ", "))
	}
	if len(blocks) == 0 {
		return ""
	}
	return strings.Join(blocks, "\n\n") + "\n"
}

// escape backslash-escapes a leading #, - or * on each line, the characters that would turn
// plain text into a heading or list when the Markdown is read.
func Escape(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed != "" && strings.ContainsRune("#-*", rune(trimmed[0])) {
			lines[i] = line[:len(line)-len(trimmed)] + `\` + trimmed
		}
	}
	return strings.Join(lines, "\n")
}

// formatDate turns YYYY-MM-DD (or YYYY-MM) into "Jun 2022".
func formatDate(s string) string {
	for _, layout := range []string{"2006-01-02", "2006-01"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("Jan 2006")
		}
	}
	return strings.TrimSpace(s)
}

func dateRange(start, end *string) string {
	var from, to string
	if start != nil {
		from = formatDate(*start)
	}
	if end != nil {
		to = formatDate(*end)
	}
	switch {
	case from == "" && to == "":
		return ""
	case from == "":
		return to
	case to == "":
		return from + " to Present"
	}
	return from + " to " + to
}

// Change is one decided suggestion: the text to find and the text to put there.
type Change struct {
	Original string
	Final    string
}

// Apply returns base with each change applied in order, plus the changes it could not place.
// A change replaces the first occurrence of its Original. A change with an empty Original is a
// new summary and goes under a "## Summary" heading before the first section.
//
// A change is not placed when its Original is missing from base, which happens when the base is
// a saved resume the user edited by hand after the suggestions were written.
func Apply(base string, changes []Change) (string, []Change) {
	var notFound []Change
	for _, c := range changes {
		if c.Original == "" {
			var ok bool
			if base, ok = insertSummary(base, c.Final); !ok {
				notFound = append(notFound, c)
			}
			continue
		}
		if !strings.Contains(base, c.Original) {
			notFound = append(notFound, c)
			continue
		}
		base = strings.Replace(base, c.Original, c.Final, 1)
	}
	return base, notFound
}

// insertSummary adds a summary section before the first "## " heading, or at the end when there
// is none. It reports false when the base already has a summary, or there is nothing to add.
func insertSummary(base, summary string) (string, bool) {
	summary = strings.TrimSpace(summary)
	if summary == "" || strings.HasPrefix(base, "## Summary\n") || strings.Contains(base, "\n## Summary\n") {
		return base, false
	}
	section := "## Summary\n\n" + summary + "\n\n"
	if strings.HasPrefix(base, "## ") {
		return section + base, true
	}
	if i := strings.Index(base, "\n## "); i >= 0 {
		return base[:i+1] + section + base[i+1:], true
	}
	return strings.TrimRight(base, "\n") + "\n\n## Summary\n\n" + summary + "\n", true
}
