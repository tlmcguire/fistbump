package pdf

import (
	"bytes"
	"regexp"
	"strings"
)

// AuditResult is a deterministic 1-page fit and ATS readiness report for a resume.
type AuditResult struct {
	PageCount       int      `json:"page_count"`
	WordCount       int      `json:"word_count"`
	BulletCount     int      `json:"bullet_count"`
	QuantifiedCount int      `json:"quantified_count"`
	Warnings        []string `json:"warnings"`
}

var (
	pdfPageMarker = []byte("/Type /Page\n")
	pdfPageAlt    = []byte("/Type /Page ")
	metricRe      = regexp.MustCompile(`(?i)(\d+%|\$\d+|\b\d+[kKmMbB+]\b|\b\d+\s*(?:years?|months?|days?|hours?|ms|x|users?|customers?|engineers?|teams?))`)
	emailRe       = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
)

// Audit renders the resume with the chosen PDF style to measure exact page count and runs
// deterministic ATS checks (contact info, quantified impact, summary length, 1-page fit).
func Audit(markdown, style string) AuditResult {
	res := AuditResult{Warnings: []string{}}
	trimmed := strings.TrimSpace(markdown)
	if trimmed == "" {
		res.Warnings = append(res.Warnings, "Resume is empty.")
		return res
	}
	res.WordCount = len(strings.Fields(trimmed))

	if data, err := RenderStyled(markdown, "Resume.pdf", style); err == nil && len(data) > 0 {
		res.PageCount = countPDFPages(data)
	}
	if res.PageCount == 0 {
		res.PageCount = 1
	}

	if !emailRe.MatchString(trimmed) {
		res.Warnings = append(res.Warnings, "Add an email address in your profile so recruiters can reach you.")
	}

	inSummary := false
	summaryWords := 0
	for _, raw := range strings.Split(trimmed, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "## ") {
			inSummary = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "## ")), "summary")
			continue
		}
		if inSummary && line != "" {
			summaryWords += len(strings.Fields(line))
		}
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, `\- `) {
			res.BulletCount++
			if metricRe.MatchString(line) {
				res.QuantifiedCount++
			}
		}
	}

	if summaryWords > 85 {
		res.Warnings = append(res.Warnings, "Your summary is long (over 85 words); trimming it to 2–3 sentences improves scanability.")
	}
	if res.BulletCount >= 3 && res.QuantifiedCount == 0 {
		res.Warnings = append(res.Warnings, "None of your bullets include measurable metrics (%, $, time saved, or scale).")
	}
	if res.PageCount > 1 {
		res.Warnings = append(res.Warnings, "Resume spans multiple pages; switch to Compact style or trim older bullets to fit 1 page.")
	}
	return res
}

func countPDFPages(data []byte) int {
	// gofpdf writes "/Type /Page" (not "/Type /Pages") for each page object.
	count := 0
	idx := 0
	needle := []byte("/Type /Page")
	for {
		i := bytes.Index(data[idx:], needle)
		if i < 0 {
			break
		}
		pos := idx + i + len(needle)
		if pos >= len(data) || data[pos] != 's' {
			count++
		}
		idx = pos
	}
	if count == 0 && (bytes.Contains(data, pdfPageMarker) || bytes.Contains(data, pdfPageAlt)) {
		return 1
	}
	return count
}
