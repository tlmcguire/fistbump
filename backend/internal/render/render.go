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
		blocks = append(blocks, "# "+escape(p.FullName))
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
			blocks = append(blocks, "## Summary", escape(strings.TrimSpace(*p.Summary)))
		}
	}
	if len(m.Experiences) > 0 {
		blocks = append(blocks, "## Experience")
		for _, e := range m.Experiences {
			blocks = append(blocks, "### "+escape(e.JobTitle+", "+e.CompanyName))
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
					blocks = append(blocks, escape(*text))
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
			blocks = append(blocks, "### "+escape(heading))
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
					blocks = append(blocks, escape(*text))
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
func escape(s string) string {
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
