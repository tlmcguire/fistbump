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
	}
	seenSec := map[string]bool{}
	var finalOrder []string
	for _, k := range m.SectionOrder {
		k = strings.ToLower(strings.TrimSpace(k))
		if k != "" && !seenSec[k] {
			seenSec[k] = true
			finalOrder = append(finalOrder, k)
		}
	}
	for _, def := range models.DefaultResumeSectionOrder() {
		if !seenSec[def] {
			finalOrder = append(finalOrder, def)
		}
	}

	for _, sec := range finalOrder {
		switch sec {
		case "summary":
			if p := m.Profile; p != nil && p.Summary != nil && strings.TrimSpace(*p.Summary) != "" {
				blocks = append(blocks, "## Summary", Escape(strings.TrimSpace(*p.Summary)))
			}
		case "experience":
			if len(m.Experiences) > 0 {
				blocks = append(blocks, "## Experience")
				for _, e := range m.Experiences {
					blocks = append(blocks, "### "+Escape(e.JobTitle+", "+e.CompanyName))
					meta := []string{}
					if !e.DateNA {
						if dates := dateRange(e.StartDate, e.EndDate); dates != "" {
							meta = append(meta, dates)
						}
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
		case "education":
			blocks = append(blocks, educationBlocks(m.Education, m.EducationLayout == models.EducationLayoutSplit)...)
		case "skills":
			if skills := m.Skills(); len(skills) > 0 {
				blocks = append(blocks, "## Skills", strings.Join(skills, ", "))
			}
		case "custom":
			for _, cs := range m.CustomSections {
				if !cs.Visible {
					continue
				}
				title := strings.TrimSpace(cs.Title)
				content := strings.TrimSpace(cs.Content)
				if title == "" || content == "" {
					continue
				}
				blocks = append(blocks, "## "+Escape(title), content)
			}
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	return strings.Join(blocks, "\n\n") + "\n"
}

func educationBlocks(rows []models.Education, split bool) []string {
	if len(rows) == 0 {
		return nil
	}
	type group struct {
		meta    models.Education
		degrees []string
	}
	var groups []group
	for _, row := range models.UniqueEducation(rows) {
		i := 0
		for i < len(groups) && !models.SameEducationTerm(groups[i].meta, row) {
			i++
		}
		if i == len(groups) {
			groups = append(groups, group{meta: row})
		}
		g := &groups[i]
		g.meta = models.MergeEducationDetails(g.meta, row)
		var degree []string
		for _, field := range []*string{row.DegreeLevel, row.Discipline} {
			if field != nil && strings.TrimSpace(*field) != "" {
				degree = append(degree, strings.TrimSpace(*field))
			}
		}
		if len(degree) > 0 {
			g.degrees = append(g.degrees, strings.Join(degree, " in "))
		}
	}

	blocks := []string{"## Education"}
	for _, g := range groups {
		e := g.meta
		switch {
		case len(g.degrees) == 0:
			blocks = append(blocks, "### "+Escape(e.Institution))
		case split:
			for _, degree := range g.degrees {
				blocks = append(blocks, "### "+Escape(degree+", "+e.Institution))
			}
		default:
			blocks = append(blocks, "### "+Escape(strings.Join(g.degrees, "; ")+", "+e.Institution))
		}
		var meta []string
		if e.GraduationDate != nil {
			if date := formatDate(*e.GraduationDate); date != "" {
				meta = append(meta, date)
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
	return blocks
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
// A change replaces its Original where it stands as a block, else its first occurrence. A change with an empty Original is a
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
		i := find(base, c.Original)
		if i < 0 {
			notFound = append(notFound, c)
			continue
		}
		base = base[:i] + c.Final + base[i+len(c.Original):]
	}
	return base, notFound
}

// find returns where original sits in text, or -1. It prefers a spot where original is a whole
// block (the way Master writes it), then a spot where it is whole lines, then the first occurrence.
// Without this, a skills line such as "excel, sql" would be replaced inside an earlier description
// that happens to contain the same words.
func find(text, original string) int {
	first, line := -1, -1
	for from := 0; from <= len(text); {
		i := strings.Index(text[from:], original)
		if i < 0 {
			break
		}
		i += from
		end := i + len(original)
		if (i == 0 || text[i-1] == '\n') && (end == len(text) || text[end] == '\n') {
			blockStart := i == 0 || strings.HasSuffix(text[:i], "\n\n")
			blockEnd := end == len(text) || strings.HasPrefix(text[end:], "\n\n") || text[end:] == "\n"
			if blockStart && blockEnd {
				return i
			}
			if line < 0 {
				line = i
			}
		}
		if first < 0 {
			first = i
		}
		from = i + 1
	}
	if line >= 0 {
		return line
	}
	return first
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
