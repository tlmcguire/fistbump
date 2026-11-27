package models

import (
	"regexp"
	"strings"
)

const (
	EducationLayoutSplit = "split"
	UnknownInstitution   = "Unknown institution"
)

var educationDegree = regexp.MustCompile(`(?i)\b(Ph\.?\s?D\.?|Doctor(?:ate)?(?: of [A-Z][a-z]+)?|J\.?D\.?|M\.?D\.?|M\.?B\.?A\.?|Master(?:'s)?(?: of [A-Z][a-z]+(?: [A-Z][a-z]+)?)?|M\.?S\.?c?\.?|M\.?A\.?|M\.?Eng\.?|Bachelor(?:'s)?(?: of [A-Z][a-z]+(?: [A-Z][a-z]+)?)?|B\.?S\.?c?\.?|B\.?A\.?|B\.?Eng\.?|Associate(?:'s)?(?: of [A-Z][a-z]+(?: [A-Z][a-z]+)?)?|A\.?A\.?S?\.?|A\.?S\.?|High School Diploma|Certificate)(?:\s|,|$)`)

// FindEducationDegree returns a degree match and the captured level.
func FindEducationDegree(text string) []string {
	return educationDegree.FindStringSubmatch(text)
}

func CountEducationDegrees(text string) int {
	return len(educationDegree.FindAllStringIndex(text, -1))
}

// SplitEducation reads older entries that stored several degrees in one field.
func SplitEducation(e Education) []Education {
	level, discipline := educationText(e.DegreeLevel), educationText(e.Discipline)
	field := discipline
	if field == "" || !strings.Contains(field, ";") {
		field = level
	}
	parts := strings.Split(field, ";")
	if len(parts) == 1 {
		return []Education{e}
	}
	out := make([]Education, 0, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return []Education{e}
		}
		row := e
		if field == discipline {
			row.Discipline = &part
		} else {
			row.DegreeLevel = &part
		}
		if match := FindEducationDegree(part); match != nil && strings.HasPrefix(part, match[0]) {
			major := strings.TrimSpace(part[len(match[0]):])
			major = strings.TrimPrefix(strings.TrimPrefix(major, "in "), "of ")
			row.DegreeLevel = &match[1]
			if field == discipline || major != "" {
				row.Discipline = nil
				if major != "" {
					row.Discipline = &major
				}
			}
		} else if i > 0 && field == level {
			return []Education{e}
		}
		out = append(out, row)
	}
	return out
}

func educationText(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func educationWords(raw string) string {
	return strings.ToLower(strings.Join(strings.Fields(raw), " "))
}

func educationLevelKey(level *string) string {
	return strings.NewReplacer(".", "", " ", "").Replace(educationWords(educationText(level)))
}

// SameEducationTerm groups rows at the same known school when the recorded date and GPA agree
// or one entry omits them. Conflicting dates or GPAs stay attached to their own rows.
func SameEducationTerm(a, b Education) bool {
	school := educationWords(a.Institution)
	if school == "" || school == educationWords(UnknownInstitution) || school != educationWords(b.Institution) {
		return false
	}
	dateA, dateB := educationText(a.GraduationDate), educationText(b.GraduationDate)
	sameMonth := len(dateA) >= 7 && len(dateB) >= 7 && dateA[:7] == dateB[:7]
	return (dateA == "" || dateB == "" || dateA == dateB || sameMonth) &&
		(a.GPA == nil || b.GPA == nil || *a.GPA == *b.GPA)
}

// SameEducationDegree compares equivalent credential rows, ignoring case and degree punctuation.
func SameEducationDegree(a, b Education) bool {
	return SameEducationTerm(a, b) &&
		educationLevelKey(a.DegreeLevel) == educationLevelKey(b.DegreeLevel) &&
		educationWords(educationText(a.Discipline)) == educationWords(educationText(b.Discipline))
}

// MergeEducationDetails fills missing metadata and preserves distinct notes in order.
func MergeEducationDetails(a, b Education) Education {
	if a.GraduationDate == nil {
		a.GraduationDate = b.GraduationDate
	}
	if a.GPA == nil {
		a.GPA = b.GPA
	}
	a.Honors = mergeEducationLines(a.Honors, b.Honors)
	a.Details = mergeEducationLines(a.Details, b.Details)
	return a
}

func mergeEducationLines(first, second *string) *string {
	if educationText(second) == "" {
		return first
	}
	if educationText(first) == "" {
		return second
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(*first, "\n") {
		seen[educationWords(line)] = true
	}
	result := *first
	for _, line := range strings.Split(*second, "\n") {
		key := educationWords(line)
		if key != "" && !seen[key] {
			result += "\n" + strings.TrimSpace(line)
			seen[key] = true
		}
	}
	if result == *first {
		return first
	}
	return &result
}

// UniqueEducation normalizes older bundled entries and merges duplicates without modifying stored rows.
func UniqueEducation(rows []Education) []Education {
	out := make([]Education, 0, len(rows))
	for _, row := range rows {
		for _, degree := range SplitEducation(row) {
			found := false
			for i, existing := range out {
				if SameEducationDegree(existing, degree) {
					out[i] = MergeEducationDetails(existing, degree)
					found = true
					break
				}
			}
			if !found {
				out = append(out, degree)
			}
		}
	}
	return out
}
