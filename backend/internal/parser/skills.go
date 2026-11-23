// The skill dictionary: the one list of skills that parsing, skill-gap analysis, search ranking and
// the guard against invented skills all share.

package parser

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// techSkills maps a canonical skill to its other spellings. The dictionary is weighted toward tech
// roles; widening it to other fields is planned after the MVP (build-findings 8.2).
var techSkills = map[string][]string{
	"go":                  {"golang", "go lang"},
	"python":              {"python3"},
	"java":                {},
	"javascript":          {"js", "ecmascript"},
	"typescript":          {"ts"},
	"c++":                 {"cpp"},
	"c#":                  {"csharp", "c sharp"},
	"c":                   {},
	"rust":                {},
	"ruby":                {"ruby on rails", "rails"},
	"php":                 {},
	"swift":               {},
	"kotlin":              {},
	"scala":               {},
	"sql":                 {},
	"postgresql":          {"postgres"},
	"mysql":               {},
	"sqlite":              {},
	"mongodb":             {"mongo"},
	"redis":               {},
	"elasticsearch":       {"opensearch"},
	"kafka":               {},
	"rabbitmq":            {},
	"graphql":             {},
	"rest":                {"restful", "rest api", "rest apis"},
	"grpc":                {},
	"react":               {"react.js", "reactjs"},
	"vue":                 {"vue.js", "vuejs"},
	"angular":             {"angularjs"},
	"svelte":              {},
	"node.js":             {"nodejs", "node"},
	"django":              {},
	"flask":               {},
	"fastapi":             {},
	"spring":              {"spring boot"},
	".net":                {"dotnet", "asp.net"},
	"html":                {"html5"},
	"css":                 {"css3", "sass", "scss"},
	"docker":              {"containers", "containerization"},
	"kubernetes":          {"k8s"},
	"terraform":           {},
	"ansible":             {},
	"aws":                 {"amazon web services"},
	"gcp":                 {"google cloud", "google cloud platform"},
	"azure":               {},
	"linux":               {"unix"},
	"git":                 {"github", "gitlab"},
	"ci/cd":               {"cicd", "continuous integration", "continuous delivery", "continuous deployment", "jenkins", "github actions"},
	"microservices":       {"microservice"},
	"distributed systems": {},
	"machine learning":    {"ml"},
	"deep learning":       {},
	"pytorch":             {},
	"tensorflow":          {},
	"llm":                 {"llms", "large language models", "large language model"},
	"nlp":                 {"natural language processing"},
	"data engineering":    {},
	"spark":               {"apache spark", "pyspark"},
	"airflow":             {},
	"dbt":                 {},
	"snowflake":           {},
	"pandas":              {},
	"numpy":               {},
	"tableau":             {},
	"power bi":            {"powerbi", "microsoft power bi"},
	"excel":               {"microsoft excel", "ms excel", "pivot tables", "vlookup"},
	"r":                   {},
	"matlab":              {},
	"bash":                {"shell scripting", "shell"},
	"powershell":          {},
	"selenium":            {},
	"cypress":             {},
	"jest":                {},
	"pytest":              {},
	"junit":               {},
	"agile":               {"scrum", "kanban"},
	"jira":                {},
	"figma":               {},
	"security":            {"infosec", "information security", "cybersecurity"},
	"siem":                {"splunk"},
	"penetration testing": {"pentest", "pen testing"},
	"iam":                 {"identity and access management"},
	"owasp":               {},
	"oauth":               {"oauth2", "openid connect", "oidc"},
	"networking":          {"tcp/ip", "dns", "bgp"},
	"embedded":            {"firmware", "rtos"},
	"fpga":                {"verilog", "vhdl"},
	"ios":                 {},
	"android":             {},
	"react native":        {},
	"flutter":             {},
	"salesforce":          {"sfdc", "salesforce crm"},
	"sap":                 {},
	"hipaa":               {},
	"fhir":                {"hl7"},
	"gdpr":                {},
	"soc 2":               {"soc2"},
	"observability":       {"prometheus", "grafana", "datadog", "opentelemetry"},
	"electron":            {},
	"webassembly":         {"wasm"},
	"oop":                 {"object-oriented", "object oriented"},
	"tdd":                 {"test-driven development"},
	"etl":                 {},
	"data modeling":       {},
	"a/b testing":         {"ab testing", "experimentation"},

	// Analyst and office skills, added after the full build.
	"sql server":         {"microsoft sql server", "ms sql", "mssql", "t-sql", "tsql"},
	"microsoft office":   {"ms office", "office 365", "microsoft 365"},
	"microsoft word":     {"ms word"},
	"powerpoint":         {"microsoft powerpoint", "ms powerpoint"},
	"microsoft access":   {"ms access"},
	"sharepoint":         {},
	"visio":              {"microsoft visio"},
	"vba":                {"excel vba"},
	"power query":        {},
	"google sheets":      {},
	"google workspace":   {"g suite", "gsuite"},
	"google analytics":   {"ga4"},
	"looker":             {"looker studio", "google data studio"},
	"qlik":               {"qlikview", "qlik sense"},
	"alteryx":            {},
	"sas":                {},
	"spss":               {"ibm spss"},
	"stata":              {},
	"data analysis":      {"data analytics"},
	"data visualization": {"data viz", "dashboards", "dashboarding"},
	"statistics":         {"statistical analysis"},
	"financial modeling": {"financial modelling"},
	"forecasting":        {},
	"crm":                {"customer relationship management"},
	"hubspot":            {},
	"quickbooks":         {},
	"smartsheet":         {},
	"asana":              {},
	"confluence":         {},

	// Customer-facing and AI roles, added after a pre-sales posting went mostly unread.
	"solutions architecture":       {"solutions architect", "solutions architects", "solution architect", "solution architecture"},
	"sales engineering":            {"sales engineer", "sales engineers", "pre-sales", "presales"},
	"technical account management": {"technical account manager", "technical account managers"},
	"cloud computing":              {"cloud architecture", "cloud architectures", "cloud infrastructure", "cloud platforms", "cloud services"},
	"data science":                 {"data scientist", "data scientists"},
	"api":                          {"apis", "api design", "api integration", "api integrations"},
	"llm evaluation":               {"evals", "llm evals", "model evaluation", "evaluation frameworks"},
	"prompt engineering":           {"prompting", "prompt design"},
}

// impliedSkills maps a skill to the skills that cover it. A resume that shows "rest" shows
// "api", and a resume that shows "sql server" shows "sql". The rules are a list, never a guess:
// a skill not named here only matches itself.
var impliedSkills = map[string][]string{
	"api":              {"rest", "graphql", "grpc"},
	"sql":              {"sql server", "postgresql", "mysql", "sqlite", "snowflake", "dbt"},
	"cloud computing":  {"aws", "gcp", "azure"},
	"data analysis":    {"data visualization", "statistics", "excel", "tableau", "power bi", "spss", "stata"},
	"data engineering": {"etl", "spark", "airflow", "dbt", "sql"},
	"data science":     {"machine learning", "statistics", "pandas", "nlp"},
	"machine learning": {"pytorch", "tensorflow", "deep learning", "llm"},
	"deep learning":    {"pytorch", "tensorflow", "machine learning"},
	"nlp":              {"llm", "machine learning"},
	"docker":           {"kubernetes"},
	"microservices":    {"docker", "kubernetes", "rest", "api"},
	"javascript":       {"node.js", "react", "vue", "angular", "svelte", "typescript"},
	"typescript":       {"javascript"},
	"rest":             {"api"},
	"etl":              {"airflow", "spark", "dbt"},
	"crm":              {"salesforce", "hubspot"},
	"excel":            {"vba", "power query"},
	"reporting":        {"tableau", "power bi", "looker", "qlik", "excel"},
	"data modeling":    {"sql", "etl"},
	"problem solving":  {"analytical"},
	"analytics":        {"data analysis", "data visualization", "tableau", "power bi", "spss", "statistics"},
	"cloud":            {"aws", "gcp", "azure"},
	"testing":          {"pytest", "jest", "cypress", "selenium", "junit", "tdd"},
}

// relatedFamilies group skills that someone who has one very likely has used or can pick up fast:
// a resume that shows PostgreSQL probably knows MySQL. They are weaker evidence than impliedSkills, so
// the analysis counts them as likely, for half credit, and never as a skill the resume shows.
var relatedFamilies = [][]string{
	{"postgresql", "mysql", "sql server", "sqlite", "oracle", "sql"},
	{"aws", "gcp", "azure", "cloud", "cloud computing"},
	{"react", "vue", "angular", "svelte", "javascript", "typescript", "html", "css"},
	{"python", "pandas", "numpy", "jupyter"},
	{"java", "kotlin", "spring", "scala"},
	{"docker", "kubernetes", "terraform", "ci/cd", "linux", "bash"},
	{"git", "github", "gitlab", "ci/cd"},
	{"tableau", "power bi", "looker", "qlik", "excel", "data visualization", "reporting"},
	{"machine learning", "pytorch", "tensorflow", "scikit-learn", "deep learning", "statistics", "python"},
	{"jira", "confluence", "agile", "scrum", "project management"},
	{"salesforce", "hubspot", "crm"},
	{"pytest", "jest", "junit", "selenium", "cypress", "testing"},
	{"rest", "api", "graphql", "grpc", "microservices"},
	{"airflow", "spark", "dbt", "etl", "snowflake", "data engineering"},
}

// Related returns the skills a resume that shows the given skill probably has too, other than the
// skill itself. The names are canonical.
func Related(skill string) []string {
	c := Canon(skill)
	out := []string{}
	seen := map[string]bool{c: true}
	for _, fam := range relatedFamilies {
		in := false
		for _, m := range fam {
			if Canon(m) == c {
				in = true
			}
		}
		if !in {
			continue
		}
		for _, m := range fam {
			if m = Canon(m); !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out
}

var softSkills = map[string][]string{
	"communication":          {"communication skills", "written and verbal communication", "communicate", "written communication", "verbal communication"},
	"leadership":             {"lead teams", "team lead", "technical leadership"},
	"collaboration":          {"collaborate", "collaborative", "cross-functional", "teamwork", "team player"},
	"mentoring":              {"mentor", "mentorship", "coaching"},
	"problem solving":        {"problem-solving", "troubleshooting", "analytical"},
	"adaptability":           {"adaptable", "flexible", "fast-paced"},
	"ownership":              {"self-starter", "self-motivated", "autonomy", "take ownership", "initiative"},
	"time management":        {"prioritization", "prioritize", "deadlines"},
	"critical thinking":      {},
	"creativity":             {"creative", "innovative"},
	"stakeholder management": {"stakeholders", "stakeholder"},
	"presentation":           {"presenting", "public speaking"},
	"attention to detail":    {"detail-oriented"},
	"curiosity":              {"curious", "continuous learning", "eager to learn"},
	"empathy":                {"customer-focused", "customer focus", "user-centric"},
	"project management":     {"program management", "project planning", "roadmap", "pmp"},
	"customer-facing":        {"customer facing", "client-facing", "client facing"},
	"relationship building":  {"build relationships", "building relationships", "relationship-building", "relationship management"},
	"teaching":               {"teach"},
	"negotiation":            {"negotiate", "negotiating"},
}

// languageSkills are spoken languages, listed with the soft skills because they describe the person,
// not a tool. They are matched only when capitalized ("Japanese", never "japanese maple"), and
// they take no inflections.
var languageSkills = map[string][]string{
	"english":    {},
	"spanish":    {},
	"french":     {},
	"german":     {},
	"italian":    {},
	"portuguese": {},
	"dutch":      {},
	"russian":    {},
	"japanese":   {},
	"korean":     {},
	"chinese":    {"mandarin", "cantonese"},
	"hindi":      {},
	"arabic":     {},
	"turkish":    {},
	"hebrew":     {},
	"swedish":    {},
	"vietnamese": {},
	"thai":       {},
	"indonesian": {},
	"tagalog":    {},
}

// exactCase lists one-word skills that are also common English words or letters. As a single token
// they match only when written in one of these forms, and never when joined to a neighbor by a
// hyphen: "Go" but not "go to market" or "Go-to-market", "C" but not "C-suite", "REST" but not "the
// rest of the team". Longer aliases ("golang", "rest api") match in any case.
var exactCase = map[string][]string{
	"go":   {"Go", "GO"},
	"c":    {"C"},
	"r":    {"R"},
	"rest": {"REST"},
}

// inflections are endings accepted on the last word of a soft skill, which resumes use as verbs
// ("mentored", "collaborated"). A final "e" is dropped first as well ("communicate", "communicating").
var inflections = []string{"s", "es", "d", "ed", "ing", "er", "ers", "ion", "ions"}

type entry struct {
	canon string
	soft  bool
	lang  bool // a spoken language: matched only when capitalized, never inflected
}

var (
	// aliases maps every spelling in the dictionary, normalized by key, to its skill. Canon uses it.
	aliases map[string]entry
	// index is aliases plus inflected soft skills. Scanning text looks up n-grams in it.
	index map[string]entry
	// maxWords is the longest key in index, in words.
	maxWords int
	// canonical is every technical skill with its words, for the implied-skill rules.
	canonical [][]string
)

func init() {
	aliases, index = map[string]entry{}, map[string]entry{}
	add := func(dict map[string][]string, soft, lang bool) {
		for c, as := range dict {
			for _, a := range append([]string{c}, as...) {
				aliases[key(a)] = entry{c, soft, lang}
			}
		}
	}
	add(techSkills, false, false)
	add(softSkills, true, false)
	add(languageSkills, true, true)
	for k, e := range aliases {
		index[k] = e
		maxWords = max(maxWords, strings.Count(k, " ")+1)
	}
	for c := range techSkills {
		canonical = append(canonical, strings.Fields(key(c)))
	}
	for k, e := range aliases {
		if !e.soft || e.lang {
			continue
		}
		stems := []string{k}
		if strings.HasSuffix(k, "e") {
			stems = append(stems, k[:len(k)-1])
		}
		for _, s := range stems {
			for _, suf := range inflections {
				if _, taken := index[s+suf]; !taken {
					index[s+suf] = e
				}
			}
		}
	}
}

// key normalizes a skill name the way text is tokenized: lower case, words joined by one space, so
// "Object-Oriented", "object oriented" and "object  oriented" are the same key.
func key(s string) string {
	toks := tokenize(s)
	words := make([]string, len(toks))
	for i, t := range toks {
		words[i] = t.text
	}
	return strings.Join(words, " ")
}

// Canon returns the canonical name for a skill or any of its aliases, or the trimmed, lower-case
// input if the dictionary does not know it.
func Canon(s string) string {
	if e, ok := aliases[key(s)]; ok {
		return e.canon
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// Aliases returns every spelling of a skill, canonical name first.
func Aliases(skill string) []string {
	c := Canon(skill)
	if as, ok := techSkills[c]; ok {
		return append([]string{c}, as...)
	}
	if as, ok := softSkills[c]; ok {
		return append([]string{c}, as...)
	}
	if as, ok := languageSkills[c]; ok {
		return append([]string{c}, as...)
	}
	return []string{c}
}

// Implied returns the skills that cover a skill: a resume that shows "rest" shows "api", and a
// resume that shows "google analytics" shows "analytics". The first rule is the dictionary; the
// second holds when every word of the skill is in the words of the other one. A skill never covers
// itself, and a one-letter skill covers nothing.
func Implied(skill string) []string {
	c := Canon(skill)
	if len(c) < 2 {
		return nil
	}
	want := strings.Fields(c)
	out := []string{}
	seen := map[string]bool{c: true}
	for _, cover := range impliedSkills[c] {
		cover = Canon(cover)
		if cover == "" || seen[cover] {
			continue
		}
		seen[cover] = true
		out = append(out, cover)
	}
	for _, words := range canonical {
		cover := strings.Join(words, " ")
		if seen[cover] || len(words) <= len(want) {
			continue
		}
		if !containsAll(words, want) {
			continue
		}
		seen[cover] = true
		out = append(out, cover)
	}
	return out
}

// containsAll reports whether every word of want is one of the words.
func containsAll(words, want []string) bool {
	for _, w := range want {
		found := false
		for _, x := range words {
			if x == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// IsSoft reports whether skill is a known soft skill or spoken language.
func IsSoft(skill string) bool {
	c := Canon(skill)
	_, soft := softSkills[c]
	_, lang := languageSkills[c]
	return soft || lang
}

// FindSkills returns the canonical tech and soft skills mentioned in text, sorted. Pass text as
// written, not lower-cased: "Go", "C", "R", "REST" and spoken languages ("Japanese") are recognized by
// their capitals.
func FindSkills(text string) (tech, soft []string) {
	seenT, seenS := map[string]bool{}, map[string]bool{}
	for _, h := range scan(text) {
		if h.soft && !seenS[h.canon] {
			seenS[h.canon] = true
			soft = append(soft, h.canon)
		} else if !h.soft && !seenT[h.canon] {
			seenT[h.canon] = true
			tech = append(tech, h.canon)
		}
	}
	slices.Sort(tech)
	slices.Sort(soft)
	return tech, soft
}

// ContainsSkill reports whether text mentions skill under any of its spellings. As with FindSkills,
// pass text as written. A longer skill wins over one inside it, so "React Native" contains "react
// native" but not "react".
func ContainsSkill(text, skill string) bool {
	c := Canon(skill)
	for _, h := range scan(text) {
		if h.canon == c {
			return true
		}
	}
	return false
}

// ContainsTerm reports whether term occurs in text as whole words, ignoring case. It does not use
// the dictionary, so it works for skills the dictionary does not know.
func ContainsTerm(text, term string) bool {
	want := tokenize(term)
	if len(want) == 0 {
		return false
	}
	toks := tokenize(text)
	for i := 0; i+len(want) <= len(toks); i++ {
		ok := true
		for j, w := range want {
			if toks[i+j].text != w.text || j > 0 && toks[i+j].brk {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

type hit struct {
	canon string
	soft  bool
	line  int
}

// scan tokenizes text once and looks up n-grams of up to maxWords words, longest first. A match
// consumes its words, so "ruby on rails" is one hit and "rails" is not looked up again.
func scan(text string) []hit {
	toks := tokenize(text)
	var hits []hit
	for i := 0; i < len(toks); {
		n := 1
		for ; n < maxWords && i+n < len(toks) && !toks[i+n].brk; n++ {
		}
		matched := false
		for ; n >= 1; n-- {
			words := make([]string, n)
			for j := range n {
				words[j] = toks[i+j].text
			}
			k := strings.Join(words, " ")
			e, ok := index[k]
			if !ok || n == 1 && !caseOK(k, e, toks[i]) {
				continue
			}
			hits = append(hits, hit{e.canon, e.soft, toks[i].line})
			i += n
			matched = true
			break
		}
		if !matched {
			i++
		}
	}
	return withoutLoneLetters(hits)
}

func caseOK(k string, e entry, t token) bool {
	if e.lang {
		r, _ := utf8.DecodeRuneInString(t.orig)
		return unicode.IsUpper(r) // "Japanese-speaking" counts; "japanese maple" does not
	}
	forms, ok := exactCase[k]
	return !ok || !t.hyphen && slices.Contains(forms, t.orig)
}

// withoutLoneLetters drops one-letter skills (C, R) unless another tech skill is on the same line,
// so "C, C++ or Rust" and "R and Python" count but "Series C" and "Plan C" do not.
func withoutLoneLetters(hits []hit) []hit {
	company := map[int]bool{}
	for _, h := range hits {
		if !h.soft && len(h.canon) > 1 {
			company[h.line] = true
		}
	}
	return slices.DeleteFunc(hits, func(h hit) bool {
		return !h.soft && len(h.canon) == 1 && !company[h.line]
	})
}

type token struct {
	text   string // lower case
	orig   string // as written
	line   int    // 0-based line number
	brk    bool   // punctuation or a line break comes before it, so no n-gram spans into it
	hyphen bool   // joined to a neighbor by a hyphen with no spaces ("Go-to-market")
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// tokenize splits text into words. A word is a run of letters, digits and underscores, which may
// also hold "." or "&" between word characters ("node.js", "asp.net", "R&D"), start with "." before
// a letter (".net"), and end in "+" or "#" ("c++", "c#"). Spaces, hyphens and slashes separate
// words within a phrase, so "object-oriented" and "CI/CD" are two words each; any other character
// ends the phrase.
func tokenize(s string) []token {
	var toks []token
	line, prevEnd := 0, 0
	at := func(i int) rune {
		if i < 0 || i >= len(s) {
			return 0
		}
		r, _ := utf8.DecodeRuneInString(s[i:])
		return r
	}
	before := func(i int) rune {
		if i <= 0 {
			return 0
		}
		r, _ := utf8.DecodeLastRuneInString(s[:i])
		return r
	}
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		dotStart := r == '.' && unicode.IsLetter(at(i+1)) && !isWordRune(before(i))
		if !isWordRune(r) && !dotStart {
			i += w
			continue
		}
		start := i
		i += w
		for i < len(s) {
			r, w := utf8.DecodeRuneInString(s[i:])
			if isWordRune(r) || (r == '.' || r == '&') && isWordRune(at(i+1)) {
				i += w
				continue
			}
			break
		}
		if j := i; at(j) == '+' || at(j) == '#' {
			for at(j) == '+' || at(j) == '#' {
				j++
			}
			if !isWordRune(at(j)) {
				i = j
			}
		}
		gap := s[prevEnd:start]
		t := token{orig: s[start:i], line: line}
		t.text = strings.ToLower(t.orig)
		if nl := strings.Count(gap, "\n"); nl > 0 {
			t.line += nl
			line += nl
		}
		t.brk = len(toks) > 0 && strings.ContainsFunc(gap, func(r rune) bool {
			return r != '-' && r != '/' && (r == '\n' || !unicode.IsSpace(r))
		})
		if gap == "-" && len(toks) > 0 {
			t.hyphen = true
			toks[len(toks)-1].hyphen = true
		}
		toks = append(toks, t)
		prevEnd = i
	}
	return toks
}
