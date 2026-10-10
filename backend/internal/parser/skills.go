package parser

import "strings"

// techSkills maps a canonical skill to its aliases (the canonical name is always an alias).
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
	"power bi":            {"powerbi"},
	"excel":               {},
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
	"salesforce":          {},
	"sap":                 {},
	"hipaa":               {},
	"fhir":                {"hl7"},
	"gdpr":                {},
	"soc 2":               {"soc2"},
	"observability":       {"prometheus", "grafana", "datadog", "opentelemetry"},
	"electron":            {},
	"webassembly":         {"wasm"},
	"oop":                 {"object-oriented", "object oriented"},
	"tdd":                 {"test-driven development", "test driven development"},
	"etl":                 {},
	"data modeling":       {},
	"a/b testing":         {"ab testing", "experimentation"},
}

var softSkills = map[string][]string{
	"communication":          {"communicate", "written communication", "verbal communication"},
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
	"attention to detail":    {"detail-oriented", "detail oriented"},
	"curiosity":              {"curious", "continuous learning", "eager to learn"},
	"empathy":                {"customer-focused", "customer focus", "user-centric"},
	"project management":     {"program management", "roadmap"},
}

// alias -> canonical, built once.
var techCanon, softCanon map[string]string

func init() {
	techCanon, softCanon = map[string]string{}, map[string]string{}
	for c, as := range techSkills {
		techCanon[c] = c
		for _, a := range as {
			techCanon[a] = c
		}
	}
	for c, as := range softSkills {
		softCanon[c] = c
		for _, a := range as {
			softCanon[a] = c
		}
	}
}

// Canon returns the canonical form of a skill: aliases map to their canonical name, anything else is trimmed and lower-cased.
func Canon(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if c, ok := techCanon[s]; ok {
		return c
	}
	if c, ok := softCanon[s]; ok {
		return c
	}
	return s
}

// Aliases returns every spelling of a canonical skill, including itself.
func Aliases(canon string) []string {
	canon = Canon(canon)
	out := []string{canon}
	if as, ok := techSkills[canon]; ok {
		return append(out, as...)
	}
	if as, ok := softSkills[canon]; ok {
		return append(out, as...)
	}
	return out
}

func isWordChar(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' || b >= 0x80
}

// ContainsTerm reports whether term occurs in text (both lower-case) on word boundaries.
// Terms made only of symbols and letters such as "c++", "c#" and ".net" are handled by checking
// the characters adjacent to each match, not by regexp.
func ContainsTerm(text, term string) bool {
	if term == "" {
		return false
	}
	for from := 0; from < len(text); {
		i := strings.Index(text[from:], term)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(term)
		okL := start == 0 || !isWordChar(text[start-1])
		okR := end == len(text) || !isWordChar(text[end])
		// "c" must not match inside "c++" or "c#".
		if okR && end < len(text) && (text[end] == '+' || text[end] == '#') && isWordChar(term[len(term)-1]) {
			okR = false
		}
		if okL && okR {
			return true
		}
		from = start + 1
	}
	return false
}

var inflections = []string{"s", "es", "d", "ed", "ing", "er", "ers", "ion", "ions"}

// containsStem is ContainsTerm that also accepts common inflections of the term's last word
// ("mentor" matches "mentored", "collaborate" matches "collaborated"). Used for soft skills, which
// appear as verbs in resume text.
func containsStem(text, term string) bool {
	if ContainsTerm(text, term) {
		return true
	}
	for _, suf := range inflections {
		if ContainsTerm(text, term+suf) {
			return true
		}
		if strings.HasSuffix(term, "e") && ContainsTerm(text, term[:len(term)-1]+suf) {
			return true
		}
	}
	return false
}

// IsSoft reports whether canon is a known soft skill.
func IsSoft(canon string) bool { _, ok := softSkills[Canon(canon)]; return ok }

// ContainsSkill reports whether lower-cased text mentions the skill under any of its aliases.
func ContainsSkill(text, skill string) bool {
	soft := IsSoft(skill)
	for _, a := range Aliases(skill) {
		if ContainsTerm(text, a) || soft && containsStem(text, a) {
			return true
		}
	}
	return false
}
