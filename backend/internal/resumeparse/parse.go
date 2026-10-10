package resumeparse

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/parser"
)

// Draft is a proposed master resume. Nothing is saved until the user applies it.
type Draft struct {
	Profile     models.Profile      `json:"profile"`
	Experiences []models.Experience `json:"experiences"`
	Education   []models.Education  `json:"education"`
	Skills      []string            `json:"skills"`
	Warnings    []string            `json:"warnings"`
}

type section string

const (
	secHeader     section = "header"
	secSummary    section = "summary"
	secExperience section = "experience"
	secEducation  section = "education"
	secSkills     section = "skills"
	secCerts      section = "certs"
	secActivities section = "activities" // research, projects, leadership: parsed like experience
	secOther      section = "other"
)

var headings = map[string]section{
	"summary": secSummary, "professional summary": secSummary, "profile": secSummary, "professional profile": secSummary, "about": secSummary,
	"about me": secSummary, "objective": secSummary, "career objective": secSummary, "overview": secSummary, "career summary": secSummary,
	"experience": secExperience, "work experience": secExperience, "professional experience": secExperience, "employment": secExperience,
	"employment history": secExperience, "work history": secExperience, "relevant experience": secExperience, "career history": secExperience,
	"professional history": secExperience, "experience highlights": secExperience,
	"education": secEducation, "academic background": secEducation, "education and training": secEducation, "academics": secEducation,
	"education and certifications": secEducation,
	"skills":                       secSkills, "technical skills": secSkills, "core competencies": secSkills, "skills and tools": secSkills, "technologies": secSkills,
	"key skills": secSkills, "competencies": secSkills, "tools": secSkills, "skills and technologies": secSkills, "technical proficiencies": secSkills,
	"skills and abilities": secSkills, "areas of expertise": secSkills, "expertise": secSkills, "skills and interests": secSkills,
	"certifications": secCerts, "licenses": secCerts, "certifications and licenses": secCerts, "licenses and certifications": secCerts,
	"clearance": secCerts, "security clearance": secCerts, "certificates": secCerts, "certifications and training": secCerts,
	"statement of purpose": secSummary, "personal statement": secSummary, "qualifications summary": secSummary,
	"projects": secActivities, "personal projects": secActivities, "research": secActivities, "research experience": secActivities,
	"volunteer": secActivities, "volunteer experience": secActivities, "leadership": secActivities, "activities": secActivities,
	"extracurricular": secActivities, "extracurriculars": secActivities, "extracurricular activities": secActivities, "involvement": secActivities,
	"campus involvement": secActivities, "leadership and activities": secActivities, "academic projects": secActivities,
	"awards": secOther, "honors and awards": secOther, "publications": secOther, "interests": secOther,
	"references": secOther, "languages": secOther, "additional information": secOther, "achievements": secOther, "relevant coursework": secOther,
	"coursework": secOther,
}

// headingWords classifies headings not in the table by their key word, for lines of a few words.
var headingWords = []struct {
	re  *regexp.Regexp
	sec section
}{
	{regexp.MustCompile(`\b(volunteer|research|project|projects|leadership|extracurricular|activities|involvement)\b`), secActivities},
	{regexp.MustCompile(`\b(experience|employment|work history)\b`), secExperience},
	{regexp.MustCompile(`\beducation\b`), secEducation},
	{regexp.MustCompile(`\b(skills|technologies|competencies|proficiencies|toolkit)\b`), secSkills},
	{regexp.MustCompile(`\b(summary|objective|profile|purpose)\b`), secSummary},
	{regexp.MustCompile(`\b(certifications?|licenses?|clearance)\b`), secCerts},
	{regexp.MustCompile(`\b(awards?|honors|publications?|interests|coursework|references|languages)\b`), secOther},
}

var (
	bulletRe = regexp.MustCompile(`^\s*(?:[•●▪◦‣∙·*\-–]|\x{f0b7}|o\s)\s*`)
	emailRe  = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)
	phoneRe  = regexp.MustCompile(`(?:\+?1[\s.-]?)?\(?\d{3}\)?[\s.-]?\d{3}[\s.-]?\d{4}`)
	urlRe    = regexp.MustCompile(`(?i)\b(?:https?://|www\.|linkedin\.com|github\.com)\S*`)
	// City, ST (two-letter state or a state name). "Inc., Charleston, SC" yields "Charleston, SC".
	cityRe     = regexp.MustCompile(`\b([A-Z][a-zA-Z.]+(?: [A-Z][a-zA-Z.]+){0,2},\s*(?:[A-Z]{2}|Alabama|Alaska|Arizona|Arkansas|California|Colorado|Connecticut|Delaware|Florida|Georgia|Hawaii|Idaho|Illinois|Indiana|Iowa|Kansas|Kentucky|Louisiana|Maine|Maryland|Massachusetts|Michigan|Minnesota|Mississippi|Missouri|Montana|Nebraska|Nevada|New Hampshire|New Jersey|New Mexico|New York|North Carolina|North Dakota|Ohio|Oklahoma|Oregon|Pennsylvania|Rhode Island|South Carolina|South Dakota|Tennessee|Texas|Utah|Vermont|Virginia|Washington|West Virginia|Wisconsin|Wyoming|Canada|UK|England|India|Germany))\b`)
	month      = `(?:Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|June?|July?|Aug(?:ust)?|Sep(?:t(?:ember)?)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?|Spring|Summer|Fall|Autumn|Winter)\.?`
	datePart   = `(?:` + month + `\s+\d{4}|\d{1,2}/\d{4}|\d{4}-\d{2}|\d{4})`
	rangeRe    = regexp.MustCompile(`(?i)(` + datePart + `)\s*(?:–|—|-|to|until)\s*(` + datePart + `|present|current|now|today)`)
	singleRe   = regexp.MustCompile(`(?i)(?:expected\s+)?(` + month + `\s+\d{4}|\d{1,2}/\d{4}|\b(?:19|20)\d{2}\b)`)
	quantRe    = regexp.MustCompile(`\d+(?:\.\d+)?\s*(?:%|x\b|k\b|m\b|\+)|\$\s?\d|\b\d{2,}\b`)
	gpaRe      = regexp.MustCompile(`(?i)GPA[:\s]*([0-4]\.\d{1,2})`)
	degreeRe   = regexp.MustCompile(`(?i)\b(Ph\.?\s?D\.?|Doctor(?:ate)?|M\.?B\.?A\.?|Master(?:'s)?(?: of [A-Z][a-z]+)?|M\.?S\.?c?\.?|M\.?A\.?|M\.?Eng\.?|Bachelor(?:'s)?(?: of [A-Z][a-z]+)?|B\.?S\.?c?\.?|B\.?A\.?|B\.?Eng\.?|Associate(?:'s)?(?: of [A-Z][a-z]+)?|A\.?A\.?S?\.?|A\.?S\.?)(?:\s|,|$)`)
	schoolRe   = regexp.MustCompile(`(?i)\b(university|college|institute|school|academy|polytechnic|conservatory)\b`)
	honorsRe   = regexp.MustCompile(`(?i)(summa|magna)? ?cum laude|honors|dean'?s list|valedictorian`)
	clearRe    = regexp.MustCompile(`(?i)\b(TS/SCI(?: with (?:full scope |CI )?poly(?:graph)?)?|top secret(?: clearance)?|secret clearance|public trust)\b`)
	titleWords = regexp.MustCompile(`(?i)\b(engineer|developer|manager|analyst|intern|director|specialist|consultant|designer|scientist|lead|architect|administrator|coordinator|associate|officer|technician|assistant|representative|president|founder|programmer|researcher|teacher|nurse|accountant|head|vp|chief|owner|supervisor|instructor|fellow|editor|writer|strategist|producer|recruiter|advisor|agent|operator|trainer|clerk|attorney|paralegal|counsel|executive|member)\b`)
	schoolSep  = regexp.MustCompile(`\s*\|\s*|\s+[—–-]\s+|,\s+`)
	sepRe      = regexp.MustCompile(`\s+(?:\||—|–|-|@|at)\s+|\s*\|\s*|,\s+`)
	monthNums  = map[string]string{"jan": "01", "feb": "02", "mar": "03", "apr": "04", "may": "05", "jun": "06", "jul": "07", "aug": "08", "sep": "09", "oct": "10", "nov": "11", "dec": "12",
		"spr": "05", "sum": "08", "fal": "12", "aut": "12", "win": "12"}
	dateLineRe = regexp.MustCompile(`(?i)^\(?(?:expected\s+|anticipated\s+)?(` + month + `\s+\d{4}|\d{1,2}/\d{4}|\d{4}-\d{2}|(?:19|20)\d{2})\)?(?:\s*\(.*\))?$`)
)

func exactHeading(line string) bool {
	l := strings.ToLower(strings.TrimSpace(strings.TrimRight(line, ":")))
	_, ok := headings[strings.Join(strings.Fields(strings.NewReplacer("&", "and", "/", " and ").Replace(l)), " ")]
	return ok
}

func headingOf(line string) (section, bool) {
	l := strings.ToLower(strings.TrimSpace(strings.TrimRight(line, ":")))
	l = strings.Join(strings.Fields(strings.NewReplacer("&", "and", "/", " and ").Replace(l)), " ")
	if len(l) > 40 || bulletRe.MatchString(line) {
		return "", false
	}
	if s, ok := headings[l]; ok {
		return s, true
	}
	// "Work and Leadership Experience", "TECHNICAL SKILLS & TOOLS": short, no digits or sentence punctuation.
	if n := len(strings.Fields(l)); n == 0 || n > 5 || strings.ContainsAny(l, "0123456789.,;@|") || titleWords.MatchString(l) {
		return "", false
	}
	for _, h := range headingWords {
		if h.re.MatchString(l) {
			return h.sec, true
		}
	}
	return "", false
}

// normDate turns "Jan 2020", "01/2020" or "2020" into YYYY-MM or YYYY. Present-like words return "".
func normDate(s string) string {
	s = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(s), "."))
	switch s {
	case "present", "current", "now", "today":
		return ""
	}
	if m := regexp.MustCompile(`^(\d{1,2})/(\d{4})$`).FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		if n >= 1 && n <= 12 {
			return m[2] + "-" + strconv.Itoa(100 + n)[1:]
		}
		return m[2]
	}
	if m := regexp.MustCompile(`^([a-z]+)\.?\s+(\d{4})$`).FindStringSubmatch(s); m != nil && len(m[1]) >= 3 {
		if mm, ok := monthNums[m[1][:3]]; ok {
			return m[2] + "-" + mm
		}
	}
	if regexp.MustCompile(`^\d{4}(-\d{2})?$`).MatchString(s) {
		return s
	}
	return ""
}

// findCity returns the location in a header line. Matches that start inside a company name
// ("Security Inc., Charleston, SC") are skipped in favor of a later match.
func findCity(s string) string {
	for from := 0; from < len(s); {
		loc := cityRe.FindStringIndex(s[from:])
		if loc == nil {
			return ""
		}
		m := s[from+loc[0] : from+loc[1]]
		head := m[:strings.Index(m, ",")]
		if !regexp.MustCompile(`(?i)\b(inc|llc|ltd|corp|co|company|group|university|college|institute)\.?$`).MatchString(head) && !titleWords.MatchString(head) {
			return m
		}
		from += loc[0] + strings.Index(m, ",") + 1
	}
	return ""
}

// isDateLine reports whether a line is a date or date range on its own (allowing a short label).
func isDateLine(l string) bool {
	l = strings.TrimSpace(l)
	if m := rangeRe.FindString(l); m != "" && len(l)-len(m) < 25 {
		return true
	}
	return dateLineRe.MatchString(l)
}

var legalSuffix = regexp.MustCompile(`(?i)^(inc|llc|l\.l\.c|ltd|corp|co|llp|plc|gmbh|s\.a|pbc|lp)\.?$`)

var abbrevEnd = regexp.MustCompile(`(?i)\b(inc|co|corp|ltd|llc|l\.l\.c|llp|plc|jr|sr|st|bros|intl|dept|univ|assn)\.$`)

// endsSentence reports a line that reads as prose. A final period after a company or name
// abbreviation ("Blue Ridge Labs, Inc.") does not count.
func endsSentence(l string) bool {
	return strings.HasSuffix(l, ".") && !abbrevEnd.MatchString(strings.TrimSpace(l))
}

var urlTail = regexp.MustCompile(`(?i)(?:https?://|www\.)\S+$`)

// joinWrapped joins a line that wrapped in the PDF onto the text before it. A web address broken
// across lines is rejoined without a space.
func joinWrapped(prev, next string) string {
	if urlTail.MatchString(prev) && next != "" && !strings.HasPrefix(next, " ") {
		return prev + next
	}
	return prev + " " + next
}

func isBullet(l string) bool { return bulletRe.MatchString(l) }

func stripBullet(l string) string { return strings.TrimSpace(bulletRe.ReplaceAllString(l, "")) }

func strPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// Parse builds a draft from resume text. It is heuristic: the user reviews every field before saving.
func Parse(text string) Draft {
	d := Draft{Experiences: []models.Experience{}, Education: []models.Education{}, Skills: []string{}, Warnings: []string{}}
	d.Profile.TargetPositions, d.Profile.ClearanceCerts = []string{}, []string{}
	blocks := map[section][]string{}
	type activity struct {
		title string
		lines []string
	}
	var acts []*activity
	cur := secHeader
	all := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	for i := range all {
		all[i] = strings.TrimSpace(all[i])
	}
	for i, l := range all {
		// A known heading always counts; a fuzzy one does not when dates follow it, since that is an
		// entry title such as "Research Assistant".
		if s, ok := headingOf(l); ok && (exactHeading(l) || !headerLine(all, i)) {
			cur = s
			if s == secActivities {
				acts = append(acts, &activity{title: strings.TrimRight(titleCase(l), ":")})
			}
			continue
		}
		if cur == secActivities {
			acts[len(acts)-1].lines = append(acts[len(acts)-1].lines, l)
			continue
		}
		blocks[cur] = append(blocks[cur], l)
	}
	parseHeader(&d, blocks[secHeader])
	if s := joinParagraph(blocks[secSummary]); s != "" {
		d.Profile.Summary = &s
	}
	d.Skills = parseSkills(blocks[secSkills])
	d.Experiences = parseExperiences(blocks[secExperience], d.Skills, "")
	for _, a := range acts { // research, projects and activities follow work experience
		for _, e := range parseExperiences(a.lines, d.Skills, a.title) {
			e.SortOrder = len(d.Experiences)
			d.Experiences = append(d.Experiences, e)
		}
	}
	d.Education = parseEducation(blocks[secEducation])
	for _, l := range blocks[secHeader] {
		if rest, ok := strings.CutPrefix(l, "Clearances and certifications:"); ok {
			for _, c := range strings.Split(rest, ",") {
				if c = strings.TrimSpace(c); c != "" {
					d.Profile.ClearanceCerts = appendUnique(d.Profile.ClearanceCerts, c)
				}
			}
		}
	}
	for _, l := range blocks[secCerts] {
		if l = stripBullet(l); l != "" && len(l) < 120 {
			d.Profile.ClearanceCerts = appendUnique(d.Profile.ClearanceCerts, l)
		}
	}
	for _, m := range clearRe.FindAllString(text, -1) {
		d.Profile.ClearanceCerts = appendUnique(d.Profile.ClearanceCerts, m)
	}
	if len(d.Experiences) > 0 {
		d.Profile.TargetPositions = []string{d.Experiences[0].JobTitle}
	}
	// Skills from the skills section that no role mentions are general skills on the profile, so they
	// are not credited to a job where they were not used.
	d.Profile.Skills = []string{}
	for _, s := range d.Skills {
		found := false
		for _, e := range d.Experiences {
			if containsFold(e.Skills, s) || containsFold(e.Skills, parser.Canon(s)) {
				found = true
			}
		}
		if !found {
			d.Profile.Skills = append(d.Profile.Skills, s)
		}
	}
	if d.Profile.FullName == "" {
		d.Warnings = append(d.Warnings, "Could not find your name. Enter it before saving.")
	}
	if len(d.Experiences) == 0 {
		d.Warnings = append(d.Warnings, "No work experience was recognized. Add entries by hand or check that the resume has an Experience heading.")
	}
	if len(d.Skills) == 0 {
		d.Warnings = append(d.Warnings, "No skills section was found. Skills were taken from your experience text.")
	}
	return d
}

func titleCase(s string) string {
	if strings.ToUpper(s) != s {
		return s
	}
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if w != "and" && w != "of" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func appendUnique(list []string, v string) []string {
	if containsFold(list, v) {
		return list
	}
	return append(list, v)
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

func joinParagraph(lines []string) string {
	var parts []string
	for _, l := range lines {
		if l = stripBullet(l); l != "" {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, " ")
}

// linkRe finds web addresses in header text: full URLs, www addresses, and bare domains such as
// "linkedin.com/in/name" or "mcguire.one". Email addresses are removed before it runs.
var linkRe = regexp.MustCompile(`(?i)\b(?:https?://)?(?:www\.)?[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)*\.(?:com|org|net|io|dev|me|co|ai|app|one|edu|gov|us|uk|ca|info|xyz|tech|site|page|design|codes|blog|art|studio|website|online|link|bio|cv|resume|work|pro)\b(?:/[^\s|·•,;)]*)?`)

// headerLinks returns the web addresses in a header line, as written, in order.
func headerLinks(line string) []string {
	clean := emailRe.ReplaceAllString(line, " ")
	var out []string
	for _, m := range linkRe.FindAllString(clean, -1) {
		m = strings.TrimRight(m, "./")
		if strings.Count(m, ".") == 0 {
			continue
		}
		out = append(out, m)
	}
	return out
}

func parseHeader(d *Draft, lines []string) {
	d.Profile.Links = []string{}
	for _, l := range lines {
		if l == "" {
			continue
		}
		for _, link := range headerLinks(l) {
			d.Profile.Links = appendUnique(d.Profile.Links, link)
		}
		if m := emailRe.FindString(l); m != "" && d.Profile.Email == nil {
			d.Profile.Email = strPtr(m)
		}
		clean := linkRe.ReplaceAllString(urlRe.ReplaceAllString(emailRe.ReplaceAllString(l, " "), " "), " ")
		if m := phoneRe.FindString(clean); m != "" && d.Profile.Phone == nil {
			d.Profile.Phone = strPtr(m)
			clean = strings.Replace(clean, m, " ", 1)
		}
		if m := cityRe.FindString(clean); m != "" && d.Profile.HomeLocation == nil && !titleWords.MatchString(m) {
			d.Profile.HomeLocation = strPtr(m)
		}
		if d.Profile.FullName == "" && looksLikeName(l) {
			d.Profile.FullName = strings.TrimSpace(l)
		}
	}
	if d.Profile.FullName == "" { // name and contact details on one line: take the text before the first separator
		for _, l := range lines {
			if l == "" {
				continue
			}
			first := strings.TrimSpace(regexp.MustCompile(`\s*[|•·,]\s*`).Split(l, 2)[0])
			if looksLikeName(first) {
				d.Profile.FullName = first
			}
			break
		}
	}
}

// IsJobTitle reports whether s reads like a job title ("Software Engineering Intern") rather than an
// activity or project ("Cybersecurity Club", "Hackathon").
func IsJobTitle(s string) bool { return titleWords.MatchString(s) }

func looksLikeName(s string) bool {
	s = strings.TrimSpace(s)
	f := strings.Fields(s)
	if len(f) < 2 || len(f) > 5 || emailRe.MatchString(s) || phoneRe.MatchString(s) || urlRe.MatchString(s) || strings.ContainsAny(s, "|@:,0123456789") {
		return false
	}
	particles := map[string]bool{"de": true, "la": true, "del": true, "da": true, "di": true, "do": true, "dos": true, "du": true, "van": true, "von": true, "der": true, "den": true, "le": true, "bin": true, "binti": true, "al": true, "el": true, "y": true}
	caps := 0
	for _, w := range f {
		r := []rune(w)
		switch {
		case unicode.IsUpper(r[0]):
			caps++
		case particles[strings.ToLower(w)]:
		default:
			return false
		}
	}
	if caps < 2 {
		return false
	}
	return !titleWords.MatchString(s)
}

func parseSkills(lines []string) []string {
	out := []string{}
	for _, l := range lines {
		l = stripBullet(l)
		if i := strings.Index(l, ":"); i > 0 && i < 40 {
			l = l[i+1:] // "Languages: Go, Python" drops the category label
		}
		for _, p := range regexp.MustCompile(`[,;|•·]|\s{2,}|\s/\s`).Split(l, -1) {
			p = strings.ToLower(strings.TrimSpace(strings.TrimRight(p, ".")))
			if p != "" && len(p) <= 40 && len(strings.Fields(p)) <= 4 {
				out = appendUnique(out, p)
			}
		}
	}
	return out
}

// headerLine reports whether a non-bullet line starts a new entry: a short line that is, or within
// three lines is followed by, a date line. Sentences ("... (Apr 2025-present).") are not headers.
func headerLine(lines []string, i int) bool {
	l := lines[i]
	if l == "" || isBullet(l) || len(l) > 100 || endsSentence(l) || strings.HasSuffix(l, ";") {
		return false
	}
	if first := []rune(l)[0]; first >= 'a' && first <= 'z' {
		return false
	}
	if isDateLine(l) || rangeRe.MatchString(l) && len(l) <= 100 {
		return true
	}
	// Look a few lines ahead for the dates. PDF text often has a blank line between a title and its
	// date line, so one blank line is allowed; a second ends the block.
	blanks := 0
	for j := i + 1; j < len(lines) && j <= i+5; j++ {
		if lines[j] == "" {
			if blanks++; blanks > 1 {
				break
			}
			continue
		}
		if isBullet(lines[j]) || endsSentence(lines[j]) {
			break
		}
		if isDateLine(lines[j]) || rangeRe.MatchString(lines[j]) {
			return true
		}
	}
	return false
}

type item struct {
	text   string
	bullet bool
}

type entry struct {
	header []string
	body   []item
	dated  bool
}

// splitEntries groups lines into entries: a header block (title, organization, dates) followed by
// bullets or paragraphs. Wrapped lines join the item above them.
func splitEntries(lines []string) []entry {
	var out []entry
	var cur *entry
	inHeader, blank := false, false
	for i, l := range lines {
		if l == "" {
			// A blank line inside a header block (title, blank, dates) keeps the block open until the
			// dates arrive.
			if !(inHeader && cur != nil && !cur.dated) {
				inHeader = false
			}
			blank = true
			continue
		}
		hdr := headerLine(lines, i)
		switch {
		case cur != nil && inHeader && !cur.dated && !isBullet(l) && len(cur.header) < 4 && (isDateLine(l) || rangeRe.MatchString(l) || len(l) <= 90 && !endsSentence(l)):
			cur.header = append(cur.header, l)
			cur.dated = isDateLine(l) || rangeRe.MatchString(l)
		case hdr:
			out = append(out, entry{header: []string{l}, dated: isDateLine(l)})
			cur = &out[len(out)-1]
			inHeader = true
		case cur == nil:
			out = append(out, entry{body: []item{{stripBullet(l), isBullet(l)}}})
			cur = &out[len(out)-1]
		case isBullet(l) || len(cur.body) == 0 || blank:
			inHeader = false
			cur.body = append(cur.body, item{stripBullet(l), isBullet(l)})
		default: // wrapped line of the previous item
			cur.body[len(cur.body)-1].text = joinWrapped(cur.body[len(cur.body)-1].text, l)
		}
		blank = false
	}
	return out
}

// splitBlocks is the fallback for sections without dates (projects, research): each blank-line
// separated block is an entry whose first line is the title.
func splitBlocks(lines []string) []entry {
	var out []entry
	var cur *entry
	for _, l := range lines {
		if l == "" {
			cur = nil
			continue
		}
		if cur == nil {
			out = append(out, entry{header: []string{stripBullet(l)}})
			cur = &out[len(out)-1]
			continue
		}
		if isBullet(l) || len(cur.body) == 0 {
			cur.body = append(cur.body, item{stripBullet(l), isBullet(l)})
		} else {
			cur.body[len(cur.body)-1].text = joinWrapped(cur.body[len(cur.body)-1].text, l)
		}
	}
	return out
}

// parseExperiences turns entries into experience rows. defaultOrg names the organization when an
// entry has none (for example "Research" for an undated research project).
func parseExperiences(lines []string, skillList []string, defaultOrg string) []models.Experience {
	out := []models.Experience{}
	entries := splitEntries(lines)
	dated := 0
	for _, en := range entries {
		if en.dated {
			dated++
		}
	}
	if dated == 0 && defaultOrg != "" {
		entries = splitBlocks(lines)
	}
	for _, en := range entries {
		if len(en.header) == 0 {
			continue
		}
		var e models.Experience
		var pieces []string
		for _, h := range en.header {
			if m := rangeRe.FindStringSubmatch(h); m != nil && e.StartDate == nil {
				e.StartDate, e.EndDate = strPtr(normDate(m[1])), strPtr(normDate(m[2]))
				h = strings.Replace(h, m[0], " ", 1)
			} else if m := dateLineRe.FindStringSubmatch(strings.TrimSpace(h)); m != nil && e.StartDate == nil {
				e.StartDate = strPtr(normDate(m[1])) // a single date: an event or a one-off role
				e.EndDate = e.StartDate
				continue
			}
			if m := findCity(h); m != "" && e.Location == nil {
				e.Location = strPtr(m)
				h = strings.Replace(h, m, " ", 1)
			} else if regexp.MustCompile(`(?i)\bremote\b`).MatchString(h) && e.Location == nil {
				e.Location = strPtr("Remote")
				h = regexp.MustCompile(`(?i)\(?\bremote\b\)?`).ReplaceAllString(h, " ")
			}
			for _, p := range sepRe.Split(h, -1) {
				if p = strings.Trim(strings.TrimSpace(p), "|,()-–— "); p != "" {
					// "Blue Ridge Labs, Inc.": a legal suffix belongs to the name before it.
					if n := len(pieces); n > 0 && legalSuffix.MatchString(p) {
						pieces[n-1] += ", " + p
						continue
					}
					pieces = append(pieces, p)
				}
			}
		}
		// A piece with a job-title word is the title. Everything else fills in by position: the first
		// remaining piece is the title, the next the organization.
		for _, p := range pieces {
			if e.JobTitle == "" && titleWords.MatchString(p) {
				e.JobTitle = p
			}
		}
		for _, p := range pieces { // fill gaps in order when keywords did not decide
			if e.JobTitle == "" && p != e.CompanyName {
				e.JobTitle = p
			} else if e.CompanyName == "" && p != e.JobTitle {
				e.CompanyName = p
			}
		}
		if e.JobTitle == "" && e.CompanyName == "" {
			continue
		}
		if e.CompanyName == "" {
			e.CompanyName = defaultOrg
		}
		if e.CompanyName == "" {
			e.CompanyName = "Unknown company"
		}
		if e.JobTitle == "" {
			e.JobTitle = "Unknown title"
		}
		// Bullets with numbers are results; paragraphs stay together as the description.
		var desc, impact []string
		var bodyText []string
		for _, b := range en.body {
			bodyText = append(bodyText, b.text)
			switch {
			case !b.bullet:
				desc = append(desc, b.text)
			case quantRe.MatchString(b.text):
				impact = append(impact, "- "+b.text)
			default:
				desc = append(desc, "- "+b.text)
			}
		}
		e.Description, e.Impact = strPtr(strings.Join(desc, "\n")), strPtr(strings.Join(impact, "\n"))
		// Skills come from the title and the text, not the company ("Acme Security Inc.") or web
		// addresses ("index.html").
		all := strings.ToLower(e.JobTitle + "\n" + linkRe.ReplaceAllString(urlRe.ReplaceAllString(strings.Join(bodyText, "\n"), " "), " "))
		tech, _ := parser.FindSkills(all)
		e.Skills = tech
		for _, s := range skillList {
			if parser.ContainsTerm(all, s) && !containsFold(e.Skills, parser.Canon(s)) && !containsFold(e.Skills, s) {
				e.Skills = append(e.Skills, s)
			}
		}
		if e.Skills == nil {
			e.Skills = []string{}
		}
		e.SortOrder = len(out)
		out = append(out, e)
	}
	return out
}

func parseEducation(lines []string) []models.Education {
	out := []models.Education{}
	var cur *models.Education
	var details []string
	flush := func() {
		if cur != nil {
			cur.Details = strPtr(strings.Join(details, "\n"))
			cur.SortOrder = len(out)
			out = append(out, *cur)
		}
		cur, details = nil, nil
	}
	for _, l := range lines {
		if l == "" {
			continue
		}
		body := stripBullet(l)
		school := schoolRe.MatchString(body) && !isBullet(l)
		deg := degreeRe.FindStringSubmatch(body)
		// A new school line starts a new entry. A second degree under the same school becomes its own row
		// that shares the school, date, GPA and honors.
		if school && (cur == nil || cur.Institution != "" && cur.Institution != "Unknown institution") {
			flush()
		} else if deg != nil && cur != nil && cur.DegreeLevel != nil && !school {
			prev := *cur
			flush()
			cur = &models.Education{Institution: prev.Institution, GraduationDate: prev.GraduationDate, GPA: prev.GPA, Honors: prev.Honors}
		}
		if cur == nil {
			cur = &models.Education{Institution: "Unknown institution"}
		}
		used := false
		if school && (cur.Institution == "Unknown institution") {
			name := body
			for _, p := range schoolSep.Split(body, -1) {
				if schoolRe.MatchString(p) {
					name = strings.TrimSpace(p)
					break
				}
			}
			if m := regexp.MustCompile(`\s+Honors( College| Program)?$`).FindString(name); m != "" {
				name = strings.TrimSuffix(name, m)
				cur.Honors = strPtr(strings.TrimSpace(m))
				if !strings.Contains(*cur.Honors, " ") {
					cur.Honors = strPtr("Honors College")
				}
			}
			cur.Institution = name
			used = true
		}
		if deg != nil && cur.DegreeLevel == nil {
			cur.DegreeLevel = strPtr(strings.TrimRight(strings.TrimSpace(deg[1]), ", "))
			rest := strings.TrimSpace(body[strings.Index(body, deg[0])+len(deg[0]):])
			rest = strings.TrimPrefix(strings.TrimPrefix(rest, "in "), "of ")
			rest = sepRe.Split(rest, 2)[0]
			rest = singleRe.ReplaceAllString(rest, "")
			if rest = strings.Trim(strings.TrimSpace(rest), ",.|-– "); rest != "" && !schoolRe.MatchString(rest) {
				cur.Discipline = strPtr(rest)
			}
			used = true
		}
		if m := gpaRe.FindStringSubmatch(body); m != nil {
			g, _ := strconv.ParseFloat(m[1], 64)
			cur.GPA = &g
			used = true
		}
		if m := singleRe.FindAllStringSubmatch(body, -1); m != nil && cur.GraduationDate == nil {
			cur.GraduationDate = strPtr(normDate(m[len(m)-1][1]))
			used = true
		}
		if honorsRe.MatchString(body) && cur.Honors == nil {
			h := body
			for _, part := range regexp.MustCompile(`\s*[|•·;]\s*`).Split(body, -1) {
				if honorsRe.MatchString(part) {
					h = part
					break
				}
			}
			cur.Honors = strPtr(h)
			used = true
		}
		if !used {
			details = append(details, body)
		}
	}
	flush()
	return out
}
