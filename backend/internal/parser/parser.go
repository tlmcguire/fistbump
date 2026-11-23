// Package parser turns job posting text into skills, requirements and inferred fields. It is
// deterministic and does no I/O. The skill dictionary is in skills.go.
package parser

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/textclean"
)

// Limits on what Parse returns.
const (
	maxRequirements   = 30
	maxDescription    = 2000 // characters
	maxTitle          = 80   // characters; a longer line is a sentence, not a title
	maxHeading        = 60   // characters
	maxLabelValue     = 100  // characters in the value of a "Location:" style line
	maxModeLineLength = 60   // characters in an intro line read for the work mode
)

// Inferred holds the fields guessed from the posting text. Each is empty when not found.
type Inferred struct {
	CompanyName   string `json:"company_name"`
	PositionTitle string `json:"position_title"`
	Location      string `json:"location"`
	WorkMode      string `json:"work_mode"` // Remote, Hybrid, On-site, or empty
}

// Result is what POST /v1/jobs/parse returns. Lists are never nil.
type Result struct {
	Description    string   `json:"description"`
	ReqTechSkills  []string `json:"req_tech_skills"`
	PrefTechSkills []string `json:"pref_tech_skills"`
	ReqSoftSkills  []string `json:"req_soft_skills"`
	PrefSoftSkills []string `json:"pref_soft_skills"`
	Requirements   []string `json:"requirements"`
	Inferred       Inferred `json:"inferred"`
	Hidden         int      `json:"-"` // invisible characters removed while cleaning
}

// Parse reads a posting: it cleans the text, splits it into sections, and fills every field of
// Result from them.
func Parse(raw string) Result {
	text, hidden := Clean(raw)
	lines := strings.Split(text, "\n")
	secs := sections(lines)

	hasReq := slices.Contains(secs, secRequired)
	// Text copied from a web page often loses its bullet symbols. When no requirement line has one,
	// the plain lines of the required section are the requirements.
	plainReqs := !reqHasBullets(lines, secs)
	reqT, prefT, reqS, prefS := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	requirements := []string{}
	firstHeading := len(lines)
	for i, line := range lines {
		sec := secs[i]
		if sec == secRequired || sec == secPreferred {
			firstHeading = min(firstHeading, i)
		}
		if h, rest, ok := heading(line); ok {
			if h == secRequired || h == secPreferred {
				firstHeading = min(firstHeading, i)
			}
			if line = rest; line == "" {
				continue
			}
		}
		var tDst, sDst map[string]bool
		switch {
		case sec == secRequired, sec != secPreferred && !hasReq:
			tDst, sDst = reqT, reqS
			// A line that calls its own skills a bonus ("Kafka is a plus") is preferred, wherever it sits.
			if softenedRe.MatchString(line) {
				tDst, sDst = prefT, prefS
			}
		case sec == secPreferred:
			tDst, sDst = prefT, prefS
		default:
			continue // context (intro, duties, benefits) when the posting has its own requirements
		}
		tech, soft := FindSkills(line)
		for _, s := range tech {
			tDst[s] = true
		}
		for _, s := range soft {
			sDst[s] = true
		}
		if sec == secRequired && len(requirements) < maxRequirements && (isBullet(line) || plainReqs && isRequirementLine(line)) {
			requirements = append(requirements, strings.TrimPrefix(line, "- "))
		}
	}
	// A skill required anywhere is not also listed as preferred.
	for s := range reqT {
		delete(prefT, s)
	}
	for s := range reqS {
		delete(prefS, s)
	}
	intro := lines[:firstHeading]
	return Result{
		Description:    clip(strings.TrimSpace(strings.Join(intro, "\n")), maxDescription),
		ReqTechSkills:  sorted(reqT),
		PrefTechSkills: sorted(prefT),
		ReqSoftSkills:  sorted(reqS),
		PrefSoftSkills: sorted(prefS),
		Requirements:   requirements,
		Inferred:       infer(lines, intro, text),
		Hidden:         hidden,
	}
}

var (
	lineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\u2028", "\n", "\u2029", "\n", "\v", "\n", "\f", "\n", "\u0085", "\n")
	// A bullet is a symbol, or a number of one or two digits with "." or ")", at the start of a line.
	// "-", "*", "+" and dashes need a space after them, so "-5°C" and "*required" are not bullets.
	bulletRe = regexp.MustCompile(`^(?:[•·▪◦‣●○■□►▸▹▶➢➤✓✔→]\s*|[-*+–—]\s+|\d{1,2}[.)]\s+)`)
)

// Clean normalizes posting text: invisible characters removed (counted in the second result),
// line endings made "\n", runs of spaces and tabs made one space, lines trimmed, bullets written
// as "- ", and runs of blank lines collapsed to one.
func Clean(raw string) (string, int) {
	raw = strings.TrimPrefix(raw, "\uFEFF") // a leading byte order mark is not hidden text
	s, hidden := textclean.StripInvisible(raw)
	s = lineBreaks.Replace(s)
	var out []string
	blank := true // drops leading blank lines
	for _, line := range strings.Split(s, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if loc := bulletRe.FindStringIndex(line); loc != nil {
			if rest := strings.TrimSpace(line[loc[1]:]); rest != "" {
				line = "- " + rest
			} else {
				line = ""
			}
		}
		if line == "" {
			if !blank {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, line)
		blank = false
	}
	return strings.TrimSpace(strings.Join(out, "\n")), hidden
}

func isBullet(line string) bool { return strings.HasPrefix(line, "- ") }

// reqHasBullets reports whether any line of a required section is a bullet.
func reqHasBullets(lines []string, secs []section) bool {
	for i, l := range lines {
		if secs[i] == secRequired && isBullet(l) {
			return true
		}
	}
	return false
}

// isRequirementLine reports whether a plain line reads as a requirement: a few words, not a heading.
func isRequirementLine(line string) bool {
	if _, _, isHeading := heading(line); isHeading {
		return false
	}
	return len(strings.Fields(line)) >= 4
}

type section int

const (
	secIntro     section = iota // before any heading
	secRequired                 // requirements, qualifications
	secPreferred                // nice to have, bonus
	secOther                    // any other heading: responsibilities, benefits, about us
)

var (
	// Headings whose words would otherwise read as preferred ("Benefits and bonus").
	otherFirstRe = regexp.MustCompile(`\b(benefits|perks|compensation|salary|pay range|pay)\b`)
	preferredRe  = regexp.MustCompile(`\b(preferred|nice[- ]to[- ]haves?|bonus|pluses|desired)\b|^(a |big |huge )?plus$|\ba plus$`)
	requiredRe   = regexp.MustCompile(`\b(requirements|required|qualifications|what you bring|what you'll bring|what you will bring|must[- ]haves?|what you'll need|what you will need|what we're looking for|what we are looking for|who you are)\b`)
	otherRe      = regexp.MustCompile(`\b(responsibilities|duties|what you'll do|what you will do|day[- ]to[- ]day|in this role|the role|your role|about|overview|summary|who we are|why join|our team|the team|culture|values|how to apply|equal opportunity|eeo|location|locations|logistics|how we(?:'re| are) different|come work with us|life at)\b`)
	// softenedRe marks a single line as asking for a skill only as a bonus.
	softenedRe  = regexp.MustCompile(`(?i)\b(is|are|would be|a) (a )?(plus|bonus|nice to have)\b|\bnice[- ]to[- ]haves?\b|\b(preferred|ideally|bonus points)\b`)
	headingTrim = "#*_=: "
)

// heading reports whether line is a section heading, and which section it starts. A heading is
// a short line that is not a bullet or a sentence and names a known section, or ends in ":". A
// line like "Requirements: Python, SQL" is a heading followed by content, returned in rest.
func heading(line string) (sec section, rest string, ok bool) {
	if line == "" || isBullet(line) {
		return 0, "", false
	}
	if head, after, found := strings.Cut(line, ":"); found && strings.TrimSpace(after) != "" {
		// Only a known section name counts here, not the trailing-colon rule: "Note: ..." is a sentence.
		if utf8.RuneCountInString(head) <= 40 {
			if sec, _, ok := heading(head); ok {
				return sec, strings.TrimSpace(after), true
			}
		}
		return 0, "", false
	}
	if utf8.RuneCountInString(line) > maxHeading || strings.ContainsAny(line[len(line)-1:], ".!?,;") {
		return 0, "", false
	}
	l := strings.ToLower(strings.ReplaceAll(strings.Trim(line, headingTrim), "’", "'"))
	switch {
	case l == "":
		return 0, "", false
	case otherFirstRe.MatchString(l):
		return secOther, "", true
	case preferredRe.MatchString(l):
		return secPreferred, "", true
	case requiredRe.MatchString(l):
		return secRequired, "", true
	case otherRe.MatchString(l), strings.HasSuffix(line, ":"):
		return secOther, "", true
	}
	return 0, "", false
}

// sections gives the section each line is in. A heading line belongs to the section it starts.
func sections(lines []string) []section {
	out := make([]section, len(lines))
	cur := secIntro
	for i, line := range lines {
		if sec, _, ok := heading(line); ok {
			cur = sec
		}
		out[i] = cur
	}
	return out
}

var (
	labelRe = regexp.MustCompile(`(?i)^(job title|title|position|role|company|employer|location|locations|based in|office location)\s*[:–—-]\s*(.+)$`)
	// "Software Engineer at Acme" on the title line.
	titleAtRe = regexp.MustCompile(`^(.+?)\s+at\s+([A-Z][\w&'.-]*(?:\s+[A-Z&][\w&'.-]*){0,4})$`)
	aboutRe   = regexp.MustCompile(`^(?i:about)\s+([A-Z][\w&'.-]*(?:\s+[A-Z&][\w&'.-]*){0,4})\s*:?$`)
	suffixRe  = regexp.MustCompile(`\b[A-Z][\w&'.-]*(?:[ \t]+[A-Z&][\w&'.-]*){0,4},?[ \t]+(?:Inc\.|Inc\b|LLC\b|Corp\.|Corp\b|Corporation\b)`)
	atRe      = regexp.MustCompile(`\bat\s+([A-Z][\w&'.-]*(?:\s+[A-Z&][\w&'.-]*){0,3})`)
	// First words of "About ..." and "at ..." that name a section or a thing, not a company.
	notCompany = map[string]bool{"the": true, "this": true, "you": true, "your": true, "our": true, "us": true,
		"role": true, "job": true, "position": true, "team": true, "company": true, "me": true, "we": true,
		"a": true, "an": true, "least": true, "all": true, "any": true, "times": true}
	abbrev = map[string]bool{"Inc": true, "Corp": true, "Co": true, "Ltd": true, "St": true, "Bros": true}

	usStates = `AL|AK|AZ|AR|CA|CO|CT|DE|DC|FL|GA|HI|ID|IL|IN|IA|KS|KY|LA|ME|MD|MA|MI|MN|MS|MO|MT|NE|NV|NH|NJ|NM|NY|NC|ND|OH|OK|OR|PA|RI|SC|SD|TN|TX|UT|VT|VA|WA|WV|WI|WY|` +
		`Alabama|Alaska|Arizona|Arkansas|California|Colorado|Connecticut|Delaware|Florida|Georgia|Hawaii|Idaho|Illinois|Indiana|Iowa|Kansas|Kentucky|Louisiana|Maine|Maryland|Massachusetts|Michigan|Minnesota|Mississippi|Missouri|Montana|Nebraska|Nevada|New Hampshire|New Jersey|New Mexico|New York|North Carolina|North Dakota|Ohio|Oklahoma|Oregon|Pennsylvania|Rhode Island|South Carolina|South Dakota|Tennessee|Texas|Utah|Vermont|Virginia|Washington|West Virginia|Wisconsin|Wyoming`
	countries = `United States|USA|US|Canada|Mexico|United Kingdom|UK|England|Scotland|Ireland|Germany|France|Spain|Portugal|Italy|Netherlands|Belgium|Switzerland|Austria|Poland|Sweden|Norway|Denmark|Finland|Israel|India|Singapore|Japan|China|South Korea|Australia|New Zealand|Brazil|Argentina|Colombia|Philippines`
	// "Charleston, SC", "Austin, Texas", "Toronto, Canada": a capitalized place, a comma, and a
	// state or country. Codes are matched in capitals only, so "in, or" is not a place.
	cityRe = regexp.MustCompile(`\b[A-Z][A-Za-z.'-]+(?:[ -][A-Z][A-Za-z.'-]+){0,3},\s*(?:` + usStates + `|` + countries + `)\b`)
	// A short line that is only "Remote" or starts with it: "Remote - US", "Remote (Canada)".
	remoteLineRe = regexp.MustCompile(`(?i)^remote\b`)
	// A line that opens with the mode: "Hybrid: two days a week in the office", "On-site, Monday to Friday".
	modeLineRe = regexp.MustCompile(`(?i)^(remote|hybrid|on-?site)\b`)
	// The line under the title often names the company, sometimes with more after a separator:
	// "Oakline Software · Remote - US".
	locSepRe         = regexp.MustCompile(`\s+[·|•]\s+`)
	headerSepRe      = regexp.MustCompile(`\s+[·|•—–-]\s+`)
	notCompanyLineRe = regexp.MustCompile(`(?i)\b(full[- ]time|part[- ]time|contract|internship|temporary|remote|hybrid|on-?site|apply|posted)\b|\d`)
	onSiteRe         = regexp.MustCompile(`(?i)\b(on-?site|in[- ]office|in[- ]person)\b`)
)

// infer guesses the title, company, location and work mode. lines is the cleaned posting, intro
// the lines before its first required or preferred heading, text the whole cleaned posting.
func infer(lines, intro []string, text string) Inferred {
	var in Inferred
	titleLine, locLine := "", ""
	for _, line := range lines {
		m := labelRe.FindStringSubmatch(line)
		if m == nil || utf8.RuneCountInString(m[2]) > maxLabelValue {
			continue
		}
		v := strings.TrimSpace(m[2])
		switch strings.ToLower(m[1]) {
		case "job title", "title", "position", "role":
			if in.PositionTitle == "" {
				in.PositionTitle, titleLine = v, line
			}
		case "company", "employer":
			if in.CompanyName == "" {
				in.CompanyName = v
			}
		default:
			if in.Location == "" {
				in.Location, locLine = v, line
			}
		}
	}

	if in.Location == "" {
		for _, line := range lines {
			if m := cityRe.FindString(line); m != "" {
				in.Location, locLine = m, line
				break
			}
			if loc := remoteLocation(line); loc != "" {
				in.Location, locLine = loc, line
				break
			}
		}
	}

	titleAt := -1
	if in.PositionTitle == "" {
		for i, line := range lines {
			if line == "" || isBullet(line) || line == locLine || labelRe.MatchString(line) ||
				utf8.RuneCountInString(line) >= maxTitle {
				continue
			}
			if _, _, ok := heading(line); ok {
				continue
			}
			in.PositionTitle, titleLine, titleAt = line, line, i
			if m := titleAtRe.FindStringSubmatch(line); m != nil && company(m[2]) != "" {
				in.PositionTitle = m[1]
				if in.CompanyName == "" {
					in.CompanyName = company(m[2])
				}
			}
			break
		}
	}

	if in.CompanyName == "" {
		for _, line := range lines {
			if m := aboutRe.FindStringSubmatch(line); m != nil {
				if in.CompanyName = company(m[1]); in.CompanyName != "" {
					break
				}
			}
		}
	}
	if in.CompanyName == "" && titleAt >= 0 {
		in.CompanyName = companyLine(lines[titleAt+1:])
	}
	if in.CompanyName == "" {
		for _, line := range intro {
			if m := atRe.FindStringSubmatch(line); m != nil {
				if in.CompanyName = company(m[1]); in.CompanyName != "" {
					break
				}
			}
		}
	}
	if in.CompanyName == "" {
		if m := suffixRe.FindString(text); m != "" {
			in.CompanyName = strings.TrimSpace(m)
		}
	}

	in.WorkMode = workMode(intro, titleLine, locLine, in.Location, text)
	return in
}

// remoteLocation returns a short "Remote" location from line, which may also stand after a
// separator in a header line ("Oakline Software · Remote - US"), else "".
func remoteLocation(line string) string {
	if isBullet(line) || utf8.RuneCountInString(line) > maxHeading {
		return ""
	}
	for _, part := range locSepRe.Split(line, -1) {
		if utf8.RuneCountInString(part) <= 40 && remoteLineRe.MatchString(part) {
			return part
		}
	}
	return ""
}

// companyLine reads a company name from the first non-empty line under the title, when that
// line is short and is not a heading, a sentence, a place or an employment type.
func companyLine(rest []string) string {
	for _, line := range rest {
		if line == "" {
			continue
		}
		if isBullet(line) || utf8.RuneCountInString(line) > maxHeading || strings.HasSuffix(line, ".") {
			return ""
		}
		if _, _, ok := heading(line); ok {
			return ""
		}
		first := headerSepRe.Split(line, 2)[0]
		if cityRe.MatchString(first) || notCompanyLineRe.MatchString(first) || len(strings.Fields(first)) > 6 ||
			first == "" || !unicode.IsUpper([]rune(first)[0]) {
			return ""
		}
		return company(first)
	}
	return ""
}

// company cleans a candidate company name, or returns "" when its first word shows it is not one.
func company(s string) string {
	s = strings.TrimRight(strings.TrimSpace(s), ",;:")
	// A sentence's full stop is not part of the name, unless the name ends in an abbreviation.
	if f := strings.Fields(s); len(f) > 0 && strings.HasSuffix(s, ".") && !abbrev[strings.TrimSuffix(f[len(f)-1], ".")] {
		s = strings.TrimSuffix(s, ".")
	}
	first, _, _ := strings.Cut(s, " ")
	if s == "" || notCompany[strings.ToLower(first)] {
		return ""
	}
	return s
}

// workMode reads the work mode the way the Greenhouse search does (StatedMode on the location and
// title, then DescribedMode on the text), and also from short intro lines such as "Hybrid". A
// posting that names a place but no mode is On-site, as in search; one that says nothing is "".
func workMode(intro []string, titleLine, locLine, location, text string) string {
	if m := StatedMode(locLine + " " + titleLine); m != "" {
		return m
	}
	for _, line := range intro {
		if !isBullet(line) && utf8.RuneCountInString(line) <= maxModeLineLength {
			if m := StatedMode(line); m != "" {
				return m
			}
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if m := modeLineRe.FindString(line); m != "" {
			if mode := StatedMode(m); mode != "" {
				return mode
			}
			return connectors.WorkModeOnSite
		}
	}
	if m := DescribedMode(text); m != "" {
		return m
	}
	if location != "" || onSiteRe.MatchString(text) {
		return connectors.WorkModeOnSite
	}
	return ""
}

var (
	remoteRe = regexp.MustCompile(`(?i)\bremote\b`)
	hybridRe = regexp.MustCompile(`(?i)\bhybrid\b`)

	describedRemote = regexp.MustCompile(`(?i)\b(fully remote|100% remote|remote[- ]first|this is a remote|remote position|remote role|remote opportunity|work from anywhere|work remotely from)\b`)
	describedHybrid = regexp.MustCompile(`(?i)\b(hybrid (role|position|work|schedule|model|environment|policy)|this is a hybrid|\d days? (a|per) week (in|at) the office|in[- ]office \d days?)\b`)
)

// StatedMode reads the work mode from short text that states it outright, such as a location or a
// title: Remote or Hybrid when the word is there, else "".
func StatedMode(text string) string {
	switch {
	case remoteRe.MatchString(text):
		return connectors.WorkModeRemote
	case hybridRe.MatchString(text):
		return connectors.WorkModeHybrid
	}
	return ""
}

// DescribedMode reads a stricter work mode from a description: "fully remote", "hybrid role" and the
// like. It returns "" when the text does not say, since benefits lists mention "remote" in passing.
func DescribedMode(text string) string {
	switch {
	case describedRemote.MatchString(text):
		return connectors.WorkModeRemote
	case describedHybrid.MatchString(text):
		return connectors.WorkModeHybrid
	}
	return ""
}

// clip shortens s to at most n characters, at a word boundary when one is near the end.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)[:n]
	if i := strings.LastIndexAny(string(r), " \n"); i > len(string(r))-200 && i > 0 {
		return strings.TrimSpace(string(r)[:i])
	}
	return strings.TrimSpace(string(r))
}

func sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
