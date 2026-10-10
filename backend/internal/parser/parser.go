// Package parser cleans posting text and extracts skills, requirements and basic metadata.
package parser

import (
	"html"
	"regexp"
	"sort"
	"strings"
)

// Result is the parsed form of a posting.
type Result struct {
	Description  string   `json:"description"`
	ReqTech      []string `json:"req_tech_skills"`
	PrefTech     []string `json:"pref_tech_skills"`
	ReqSoft      []string `json:"req_soft_skills"`
	PrefSoft     []string `json:"pref_soft_skills"`
	Requirements []string `json:"requirements"`
	Inferred     Inferred `json:"inferred"`
}

// Inferred holds metadata guessed from the text. Empty strings mean unknown.
type Inferred struct {
	CompanyName   string `json:"company_name"`
	PositionTitle string `json:"position_title"`
	Location      string `json:"location"`
	WorkMode      string `json:"work_mode"`
}

var (
	tagRe       = regexp.MustCompile(`(?s)<[^>]*>`)
	liRe        = regexp.MustCompile(`(?i)<li[^>]*>`)
	blockRe     = regexp.MustCompile(`(?i)</?(p|div|br|li|ul|ol|h[1-6]|tr|section)[^>]*>`)
	spaceRe     = regexp.MustCompile(`[ \t\x{00a0}]+`)
	blankRe     = regexp.MustCompile(`\n{3,}`)
	bulletRe    = regexp.MustCompile(`^\s*(?:[-*•·▪◦]|\d+[.)])\s+`)
	labelRe     = regexp.MustCompile(`(?i)^\s*(job title|title|position|role|company|employer|location|where|work mode|work type)\s*[:\-]\s*(.+)$`)
	atCompanyRe = regexp.MustCompile(`(?i)(?:^|\s)at\s+([A-Z][\w&.\- ]{1,40}?)(?:[,.\n]|\s+(?:is|we|are|you|in)\b|$)`)
	aboutRe     = regexp.MustCompile(`(?i)^\s*about\s+(?:us\s*[:\-]\s*)?([A-Z][\w&.\- ]{1,40})\s*$`)
	// "Title — Company", "Title | Company", "Title at Company" on a header line.
	// A plain hyphen is left out: "Software Engineer - Backend" is a title.
	titleCompanyRe = regexp.MustCompile(`^(.+?)\s+(?:[—–|]|at)\s+(.+)$`)
)

// genericAbout lists first words of "About ..." headings that name a section,
// not a company ("About the role", "About you").
var genericAbout = map[string]bool{
	"the": true, "this": true, "you": true, "your": true, "our": true, "us": true,
	"role": true, "job": true, "position": true, "team": true, "company": true, "me": true,
}

func companyName(s string) string {
	s = strings.TrimSpace(s)
	first, _, _ := strings.Cut(s, " ")
	if s == "" || genericAbout[strings.ToLower(first)] {
		return ""
	}
	return s
}

// Clean strips HTML, decodes entities and normalizes whitespace.
func Clean(raw string) string {
	s := raw
	if strings.Contains(s, "&lt;") && !strings.Contains(s, "<p") && !strings.Contains(s, "<div") {
		s = html.UnescapeString(s) // Greenhouse returns escaped HTML
	}
	s = liRe.ReplaceAllString(s, "\n- ")
	s = blockRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(spaceRe.ReplaceAllString(l, " "))
	}
	s = strings.Join(lines, "\n")
	s = blankRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

type section int

const (
	secGeneral section = iota // intro, about us, benefits: context, not requirements
	secRequired
	secPreferred
	secDuties // responsibilities: what the job uses, so counted as required
)

var (
	reqHeaderWords  = []string{"requirements", "qualifications", "must have", "what you'll need", "what you will need", "what we're looking for", "who you are", "minimum", "you have", "basic qualifications", "required"}
	prefHeaderWords = []string{"preferred", "nice to have", "bonus", "a plus", "extra credit", "ideal", "desired"}
	dutyHeaderWords = []string{"responsibilities", "what you'll do", "what you will do", "the role", "your role", "day to day", "day-to-day", "in this role", "you will", "what you'll be doing", "duties"}
	genHeaderWords  = []string{"about", "benefits", "perks", "compensation", "overview", "our team", "why join", "how we work", "who we are", "the team", "salary", "pay range"}
)

func headerKind(line string) (section, bool) {
	l := strings.ToLower(strings.TrimRight(strings.TrimSpace(line), ":"))
	if l == "" || len(l) > 70 || bulletRe.MatchString(line) || strings.HasSuffix(l, ".") {
		return secGeneral, false
	}
	for _, w := range prefHeaderWords {
		if strings.Contains(l, w) {
			return secPreferred, true
		}
	}
	for _, w := range reqHeaderWords {
		if strings.Contains(l, w) {
			return secRequired, true
		}
	}
	for _, w := range dutyHeaderWords {
		if strings.HasPrefix(l, w) || strings.Contains(l, w) && len(l) < 40 {
			return secDuties, true
		}
	}
	for _, w := range genHeaderWords {
		if strings.HasPrefix(l, w) || strings.Contains(l, w) && len(l) < 40 {
			return secGeneral, true
		}
	}
	return secGeneral, false
}

var inlinePrefRe = regexp.MustCompile(`(?i)\b(preferred|nice to have|a plus|bonus|is a plus|are a plus|ideally)\b`)

// Parse extracts structured data from posting text.
func Parse(raw string) Result {
	text := Clean(raw)
	res := Result{Description: text, ReqTech: []string{}, PrefTech: []string{}, ReqSoft: []string{}, PrefSoft: []string{}, Requirements: []string{}}
	reqT, prefT, reqS, prefS := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}

	lines := strings.Split(text, "\n")
	// With an explicit requirements section, skills elsewhere (intro, responsibilities) are context, not
	// requirements: tech skills there count as preferred and soft skills are ignored. Without one,
	// everything mentioned is treated as required.
	hasReq := false
	for _, line := range lines {
		if k, ok := headerKind(line); ok && k == secRequired {
			hasReq = true
		}
	}
	cur := secGeneral
	for _, line := range lines {
		if line == "" {
			continue
		}
		if k, ok := headerKind(line); ok {
			cur = k
			continue
		}
		sec := cur
		if inlinePrefRe.MatchString(line) {
			sec = secPreferred
		}
		low := strings.ToLower(line)
		for _, c := range findSkills(low, line, techSkills) {
			if sec == secPreferred || sec == secGeneral && hasReq {
				prefT[c] = true
			} else {
				reqT[c] = true
			}
		}
		for _, c := range findSkills(low, line, softSkills) {
			switch {
			case sec == secGeneral && hasReq:
			case sec == secPreferred:
				prefS[c] = true
			default:
				reqS[c] = true
			}
		}
		if cur == secRequired && bulletRe.MatchString(line) && len(res.Requirements) < 25 {
			res.Requirements = append(res.Requirements, strings.TrimSpace(bulletRe.ReplaceAllString(line, "")))
		}
	}
	// A skill required anywhere is not also listed as preferred.
	for c := range reqT {
		delete(prefT, c)
	}
	for c := range reqS {
		delete(prefS, c)
	}
	res.ReqTech, res.PrefTech, res.ReqSoft, res.PrefSoft = keys(reqT), keys(prefT), keys(reqS), keys(prefS)
	res.Inferred = infer(text)
	return res
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// caseSensitive lists skills whose canonical name is an ordinary word or letter. They only match
// in original case ("Go", "R", "C"), so "go to market" is not a skill.
var caseSensitive = map[string]string{"go": "Go", "r": "R", "c": "C"}

var listSep = regexp.MustCompile(`^[\s,/;()]*$`)

// singleLetterSkill matches languages named by one letter (C, R) only as a list item next to other
// skills, so "U.S.C.", "R&D" and "Plan B" do not count.
func singleLetterSkill(line, letter string) bool {
	for from := 0; from < len(line); {
		i := strings.Index(line[from:], letter)
		if i < 0 {
			return false
		}
		at := from + i
		from = at + 1
		var before, after byte = ' ', ' '
		if at > 0 {
			before = line[at-1]
		}
		if at+1 < len(line) {
			after = line[at+1]
		}
		if !strings.ContainsRune(" ,/;(", rune(before)) || !strings.ContainsRune(" ,/;)", rune(after)) {
			continue
		}
		if after == ' ' && at+2 < len(line) && (line[at+2] == '+' || line[at+2] == '#' || line[at+2] == '.') {
			continue
		}
		// Another skill must sit in the same list: "C, C++ or Rust", "Python/R".
		rest := strings.ToLower(line[:at] + " " + line[at+1:])
		others := 0
		for canon, aliases := range techSkills {
			if _, single := caseSensitive[canon]; single {
				continue
			}
			for _, a := range append([]string{canon}, aliases...) {
				if ContainsTerm(rest, a) {
					others++
					break
				}
			}
			if others > 0 {
				break
			}
		}
		if others > 0 {
			return true
		}
	}
	return false
}

func dictIsSoft(dict map[string][]string) bool {
	_, ok := dict["communication"]
	return ok
}

// findSkills returns canonical skills from dict whose canonical name or alias occurs in the line.
// low is the lower-cased line, orig the line as written.
func findSkills(low, orig string, dict map[string][]string) []string {
	var out []string
	for canon, aliases := range dict {
		if cs, ok := caseSensitive[canon]; ok {
			if len(cs) == 1 && singleLetterSkill(orig, cs) || len(cs) > 1 && ContainsTerm(orig, cs) {
				out = append(out, canon)
				continue
			}
		} else if ContainsTerm(low, canon) {
			out = append(out, canon)
			continue
		}
		soft := dictIsSoft(dict)
		for _, a := range aliases {
			if ContainsTerm(low, a) || soft && containsStem(low, a) {
				out = append(out, canon)
				break
			}
		}
	}
	return out
}

// FindSkills returns canonical tech and soft skills mentioned in text, for ranking and evidence.
func FindSkills(text string) (tech, soft []string) {
	clean := Clean(text)
	low := strings.ToLower(clean)
	tech, soft = findSkills(low, clean, techSkills), findSkills(low, clean, softSkills)
	sort.Strings(tech)
	sort.Strings(soft)
	return
}

var modeRe = []struct {
	re   *regexp.Regexp
	mode string
}{
	{regexp.MustCompile(`(?i)\bhybrid\b`), "Hybrid"},
	{regexp.MustCompile(`(?i)\b(fully remote|remote[- ]first|100% remote|work from home|remote)\b`), "Remote"},
	{regexp.MustCompile(`(?i)\b(on-?site|in[- ]office|in[- ]person)\b`), "On-site"},
}

// InferWorkMode returns Remote, Hybrid, On-site or "".
func InferWorkMode(text string) string {
	for _, m := range modeRe {
		if m.re.MatchString(text) {
			return m.mode
		}
	}
	return ""
}

func infer(text string) Inferred {
	var in Inferred
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		if m := labelRe.FindStringSubmatch(l); m != nil {
			v := strings.TrimSpace(m[2])
			switch strings.ToLower(m[1]) {
			case "job title", "title", "position", "role":
				if in.PositionTitle == "" {
					in.PositionTitle = v
				}
			case "company", "employer":
				if in.CompanyName == "" {
					in.CompanyName = v
				}
			case "location", "where":
				if in.Location == "" {
					in.Location = v
				}
			case "work mode", "work type":
				if in.WorkMode == "" {
					in.WorkMode = InferWorkMode(v)
				}
			}
		}
	}
	header, headerCompany := "", ""
	if in.PositionTitle == "" {
		for _, l := range lines {
			if l != "" && len(l) <= 100 && !strings.HasSuffix(l, ".") {
				header, in.PositionTitle = l, l
				if m := titleCompanyRe.FindStringSubmatch(l); m != nil {
					in.PositionTitle, headerCompany = strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
				}
				break
			}
		}
	}
	if in.CompanyName == "" {
		for _, l := range lines {
			if m := aboutRe.FindStringSubmatch(l); m != nil {
				if c := companyName(m[1]); c != "" {
					in.CompanyName = c
					break
				}
			}
		}
	}
	if in.CompanyName == "" {
		in.CompanyName = headerCompany
	}
	if in.CompanyName == "" {
		head := text
		if len(head) > 600 {
			head = head[:600]
		}
		if m := atCompanyRe.FindStringSubmatch(head); m != nil {
			in.CompanyName = companyName(m[1])
		}
	}
	// A header split whose right side is not the company ("Engineer — Payments"
	// at Stripe) was part of the title after all.
	if headerCompany != "" && !strings.EqualFold(headerCompany, in.CompanyName) {
		in.PositionTitle = header
	}
	if in.WorkMode == "" {
		in.WorkMode = InferWorkMode(text)
	}
	return in
}

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an and are as at be by for from has have in is it its of on or our that the their this to we will with you your
		who what when where which about all any can may more most not other such than then they these those through using use work working team
		experience years year ability strong including etc must should would also new well high role job company position`) {
		stopwords[w] = true
	}
}

var wordRe = regexp.MustCompile(`[a-z][a-z0-9+#.]*[a-z0-9+#]|[a-z]`)

// Keywords returns the most frequent non-stopword unigrams and bigrams of text, most frequent first.
func Keywords(text string, limit int) []string {
	words := wordRe.FindAllString(strings.ToLower(Clean(text)), -1)
	counts := map[string]int{}
	var prev string
	for _, w := range words {
		if len(w) < 2 || stopwords[w] {
			prev = ""
			continue
		}
		counts[w]++
		if prev != "" {
			counts[prev+" "+w]++
		}
		prev = w
	}
	type kv struct {
		k string
		n int
	}
	var all []kv
	for k, n := range counts {
		if strings.Contains(k, " ") && n < 2 {
			continue
		}
		all = append(all, kv{k, n})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return all[i].k < all[j].k
	})
	out := []string{}
	for _, e := range all {
		if len(out) >= limit {
			break
		}
		out = append(out, e.k)
	}
	return out
}
