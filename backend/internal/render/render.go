// Package render turns the structured master resume into Markdown text.
package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

var months = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// FormatDate shows stored dates the way resumes write them: "2022-03" and "2022-03-15" become "Mar 2022".
func FormatDate(d string) string {
	if len(d) >= 7 && d[4] == '-' {
		if m, err := strconv.Atoi(d[5:7]); err == nil && m >= 1 && m <= 12 {
			return months[m-1] + " " + d[:4]
		}
	}
	return d
}

// DateRange formats "Jan 2022 – Present" style ranges from stored date text.
func DateRange(start, end *string) string {
	s, e := FormatDate(deref(start)), FormatDate(deref(end))
	if s == "" && e == "" {
		return ""
	}
	if e == "" {
		e = "Present"
	}
	if s == "" {
		return e
	}
	return s + " – " + e
}

// escape protects user text from being read as Markdown structure: a line that starts with "#"
// ("# of incidents cut in half") would otherwise become a heading. The PDF renderer removes the "\".
// Each escaped line keeps its exact text otherwise, so suggestions still match it.
func escape(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		t := strings.TrimLeft(l, " ")
		if strings.HasPrefix(t, "#") {
			lines[i] = `\` + t
		}
	}
	return strings.Join(lines, "\n")
}

// SkillsLine is the comma-joined skills text used for the Skills section and skills suggestions.
func SkillsLine(skills []string) string { return strings.Join(skills, ", ") }

// Master renders the master resume as Markdown. Description, impact and the skills line appear verbatim
// so suggestions can replace them by exact text.
func Master(m models.Master) string {
	var b strings.Builder
	if m.Profile != nil {
		fmt.Fprintf(&b, "# %s\n\n", m.Profile.FullName)
		// Contact line in the order resumes usually use: place, email, phone, then links as written.
		var contact []string
		for _, p := range []*string{m.Profile.HomeLocation, m.Profile.Email, m.Profile.Phone} {
			if v := deref(p); v != "" {
				contact = append(contact, v)
			}
		}
		contact = append(contact, m.Profile.Links...)
		if len(contact) > 0 {
			b.WriteString(strings.Join(contact, " · ") + "\n\n")
		}
		if s := deref(m.Profile.Summary); s != "" {
			b.WriteString("## Summary\n\n" + escape(s) + "\n\n")
		}
	}
	if len(m.Experiences) > 0 {
		b.WriteString("## Experience\n\n")
		for _, e := range m.Experiences {
			fmt.Fprintf(&b, "### %s, %s\n\n", e.JobTitle, e.CompanyName)
			var meta []string
			if r := DateRange(e.StartDate, e.EndDate); r != "" {
				meta = append(meta, r)
			}
			if l := deref(e.Location); l != "" {
				meta = append(meta, l)
			}
			if len(meta) > 0 {
				b.WriteString(strings.Join(meta, " | ") + "\n\n")
			}
			if d := deref(e.Description); d != "" {
				b.WriteString(escape(d) + "\n\n")
			}
			if i := deref(e.Impact); i != "" {
				b.WriteString(escape(i) + "\n\n")
			}
		}
	}
	if len(m.Education) > 0 {
		b.WriteString("## Education\n\n")
		for _, e := range m.Education {
			head := e.Institution
			var deg []string
			if d := deref(e.DegreeLevel); d != "" {
				deg = append(deg, d)
			}
			if d := deref(e.Discipline); d != "" {
				deg = append(deg, d)
			}
			if len(deg) > 0 {
				head = strings.Join(deg, " in ") + ", " + e.Institution
			}
			fmt.Fprintf(&b, "### %s\n\n", head)
			var meta []string
			if d := deref(e.GraduationDate); d != "" {
				meta = append(meta, FormatDate(d))
			}
			if e.GPA != nil {
				meta = append(meta, fmt.Sprintf("GPA %.2f", *e.GPA))
			}
			if h := deref(e.Honors); h != "" {
				meta = append(meta, h)
			}
			if len(meta) > 0 {
				b.WriteString(strings.Join(meta, " | ") + "\n\n")
			}
			if d := deref(e.Details); d != "" {
				b.WriteString(escape(d) + "\n\n")
			}
		}
	}
	if sk := m.Skills(); len(sk) > 0 {
		b.WriteString("## Skills\n\n" + SkillsLine(sk) + "\n\n")
	}
	if m.Profile != nil && len(m.Profile.ClearanceCerts) > 0 {
		b.WriteString("## Certifications\n\n")
		for _, c := range m.Profile.ClearanceCerts {
			b.WriteString("- " + c + "\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}

// Change is one resolved suggestion to apply to a base resume text.
type Change struct {
	Original string // empty for inserted text (summary)
	Final    string
}

// sectionHeading finds the first line that starts with "## " (not "### ") and returns its offset, or -1.
func sectionHeading(text string) int {
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], "## ") {
			return i
		}
		nl := strings.IndexByte(text[i:], '\n')
		if nl < 0 {
			return -1
		}
		i += nl + 1
	}
	return -1
}

func summaryBlock(final string) string { return "## Summary\n\n" + final + "\n\n" }

// Apply returns base with each change applied. A change with an Original replaces its first occurrence.
// Text with no Original becomes a "## Summary" section before the first section heading.
func Apply(base string, changes []Change) string {
	text := base
	for _, c := range changes {
		if c.Final == "" {
			continue
		}
		if c.Original != "" {
			text = strings.Replace(text, c.Original, c.Final, 1)
			continue
		}
		if i := sectionHeading(text); i >= 0 {
			text = text[:i] + summaryBlock(c.Final) + text[i:]
		} else {
			text = strings.TrimRight(text, "\n") + "\n\n" + summaryBlock(c.Final)
		}
	}
	return text
}

// Revert undoes Apply on tailored text, recovering the base it was built from. Changes are undone in
// reverse order. Text the user changed afterwards cannot be recovered and is left as is.
func Revert(tailored string, changes []Change) string {
	text := tailored
	for i := len(changes) - 1; i >= 0; i-- {
		c := changes[i]
		if c.Final == "" {
			continue
		}
		if c.Original != "" {
			text = strings.Replace(text, c.Final, c.Original, 1)
			continue
		}
		block := summaryBlock(c.Final)
		if strings.Contains(text, block) {
			text = strings.Replace(text, block, "", 1)
		} else {
			text = strings.Replace(text, "\n\n"+block, "\n", 1)
		}
	}
	return text
}
