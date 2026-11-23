package parser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// expected is a posting's .json file in testdata/postings: what Parse must find in the .txt beside it.
type expected struct {
	ReqTech          []string `json:"req_tech_skills"`
	PrefTech         []string `json:"pref_tech_skills"`
	ReqSoft          []string `json:"req_soft_skills"`
	PrefSoft         []string `json:"pref_soft_skills"`
	Requirements     int      `json:"requirements"`
	HiddenCharacters int      `json:"hidden_characters"`
	Inferred         Inferred `json:"inferred"`
}

// TestPostings runs Parse over every posting in testdata/postings and compares it with its .json.
func TestPostings(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "testdata", "postings", "*.txt"))
	if err != nil || len(files) < 6 {
		t.Fatalf("found %d postings (%v); want at least 6", len(files), err)
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".txt")
		t.Run(name, func(t *testing.T) {
			text, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(strings.TrimSuffix(f, ".txt") + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var want expected
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			r := Parse(string(text))
			for _, c := range []struct {
				field     string
				got, want []string
			}{{"req_tech_skills", r.ReqTechSkills, want.ReqTech}, {"pref_tech_skills", r.PrefTechSkills, want.PrefTech},
				{"req_soft_skills", r.ReqSoftSkills, want.ReqSoft}, {"pref_soft_skills", r.PrefSoftSkills, want.PrefSoft}} {
				if !slices.Equal(c.got, c.want) {
					t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
				}
			}
			if len(r.Requirements) != want.Requirements {
				t.Errorf("requirements = %d %q, want %d", len(r.Requirements), r.Requirements, want.Requirements)
			}
			if r.Hidden != want.HiddenCharacters {
				t.Errorf("hidden = %d, want %d", r.Hidden, want.HiddenCharacters)
			}
			if r.Inferred != want.Inferred {
				t.Errorf("inferred = %+v\nwant       %+v", r.Inferred, want.Inferred)
			}
		})
	}
}

// The hidden-text posting spells a skill list in Unicode tag characters, wrapped in zero-width
// characters. None of those skills may be found, and the visible ones must still be.
func TestHiddenSkillListIsNotDetected(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("..", "..", "testdata", "postings", "hidden-text.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.ContainsRune(string(text), 0xE0001) || !strings.ContainsRune(string(text), 0x200B) {
		t.Fatal("hidden-text.txt lost its tag or zero-width characters")
	}
	r := Parse(string(text))
	all := append(append(append(r.ReqTechSkills, r.PrefTechSkills...), r.ReqSoftSkills...), r.PrefSoftSkills...)
	for _, hidden := range []string{"kubernetes", "terraform", "rust", "scala", "leadership"} {
		if slices.Contains(all, hidden) {
			t.Errorf("found hidden skill %q in %v", hidden, all)
		}
	}
	if r.Hidden == 0 || strings.ContainsRune(r.Description, 0xE0001) {
		t.Errorf("hidden = %d; the hidden characters must be counted and removed", r.Hidden)
	}
}

func TestWholeWordMatching(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"go is not good or go-to", "Good judgment, go-to-market sense; we go above and beyond. Django and MongoDB.", []string{"django", "mongodb"}},
		{"Go as a language", "Backend services in Go and Golang", []string{"go"}},
		{"c next to other languages", "C, C++ or Rust", []string{"c", "c++", "rust"}},
		{"r next to python", "R and Python", []string{"python", "r"}},
		{"lone letters are not languages", "Series C funding, an R&D lab, Plan B and vitamin C", []string{}},
		{"c-suite", "Report to the C-suite", []string{}},
		{"java is not javascript", "JavaScript and TypeScript", []string{"javascript", "typescript"}},
		{"symbols", "C#, .NET and Node.js.", []string{".net", "c#", "node.js"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Parse(c.text).ReqTechSkills; !slices.Equal(got, c.want) {
				t.Errorf("Parse(%q) tech = %v, want %v", c.text, got, c.want)
			}
		})
	}
}

func TestAliases(t *testing.T) {
	r := Parse("Experience with Postgres, K8s, JS, ECMAScript, Golang, MS Excel, PowerBI and T-SQL.\nStrong written and verbal communication.")
	want := []string{"excel", "go", "javascript", "kubernetes", "postgresql", "power bi", "sql server"}
	if !slices.Equal(r.ReqTechSkills, want) || !slices.Equal(r.ReqSoftSkills, []string{"communication"}) {
		t.Errorf("aliases = %v, %v; want %v, [communication]", r.ReqTechSkills, r.ReqSoftSkills, want)
	}
	for in, want := range map[string]string{"Golang": "go", "K8s": "kubernetes", " Postgres ": "postgresql", "Object-Oriented": "oop", "Haskell": "haskell"} {
		if got := Canon(in); got != want {
			t.Errorf("Canon(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDictionary checks that every spelling in the skill dictionary belongs to one skill.
func TestDictionary(t *testing.T) {
	owner := map[string]string{}
	for _, dict := range []map[string][]string{techSkills, softSkills} {
		for c, as := range dict {
			for _, a := range append([]string{c}, as...) {
				k := key(a)
				if prev, ok := owner[k]; ok && prev != c {
					t.Errorf("%q belongs to both %q and %q", a, prev, c)
				}
				owner[k] = c
			}
		}
	}
	if len(techSkills) < 105 || len(softSkills) < 16 {
		t.Errorf("dictionary has %d tech and %d soft skills", len(techSkills), len(softSkills))
	}
}

// TestImpliedSkills checks the rules that let one skill stand in for another: a resume that shows
// "rest" shows "api", and a resume that shows "google analytics" shows "analytics".
func TestImpliedSkills(t *testing.T) {
	cases := []struct {
		skill string
		want  []string
	}{
		{"api", []string{"rest", "graphql", "grpc"}},
		{"sql", []string{"sql server", "postgresql", "mysql", "sqlite", "snowflake", "dbt"}},
		{"docker", []string{"kubernetes"}},
		{"analytics", []string{"data analysis", "tableau", "google analytics"}},
	}
	for _, c := range cases {
		got := Implied(c.skill)
		for _, w := range c.want {
			if !slices.Contains(got, w) {
				t.Errorf("Implied(%q) = %v; want it to include %q", c.skill, got, w)
			}
		}
		if slices.Contains(got, c.skill) {
			t.Errorf("Implied(%q) = %v; a skill never covers itself", c.skill, got)
		}
	}
	// An alias is understood: "APIs" is the skill "api".
	if got := Implied("APIs"); !slices.Contains(got, "rest") {
		t.Errorf("Implied(%q) = %v; want it to include %q", "APIs", got, "rest")
	}
	// One letter covers nothing.
	if got := Implied("c"); len(got) != 0 {
		t.Errorf("Implied(%q) = %v; want nothing", "c", got)
	}
}

const posting = `Senior Data Analyst
Acme Analytics · Charleston, SC (Hybrid)

We are looking for an analyst to join our growing team at Acme Analytics.
You will build dashboards and work with Salesforce data.

Responsibilities
• Build Tableau reports for the sales team
• Partner with stakeholders across the business

Requirements:
1. 3+ years of SQL and Excel
2) Experience with Python or R
- Strong communication skills

Nice to have
* Power BI
* Python scripting

Benefits
- Remote work stipend
- 401(k) match

Acme Analytics, Inc. is an equal opportunity employer.`

func TestParsePosting(t *testing.T) {
	r := Parse(posting)
	check := func(field string, got, want []string) {
		t.Helper()
		if !slices.Equal(got, want) {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	check("req_tech_skills", r.ReqTechSkills, []string{"excel", "python", "r", "sql"})
	check("pref_tech_skills", r.PrefTechSkills, []string{"power bi"})
	check("req_soft_skills", r.ReqSoftSkills, []string{"communication"})
	check("pref_soft_skills", r.PrefSoftSkills, []string{})
	check("requirements", r.Requirements, []string{"3+ years of SQL and Excel", "Experience with Python or R", "Strong communication skills"})

	if !strings.HasPrefix(r.Description, "Senior Data Analyst\n") || !strings.HasSuffix(r.Description, "- Partner with stakeholders across the business") {
		t.Errorf("description = %q", r.Description)
	}
	want := Inferred{CompanyName: "Acme Analytics", PositionTitle: "Senior Data Analyst", Location: "Charleston, SC", WorkMode: "Hybrid"}
	if r.Inferred != want {
		t.Errorf("inferred = %+v, want %+v", r.Inferred, want)
	}
	if r.Hidden != 0 {
		t.Errorf("hidden = %d", r.Hidden)
	}
}

func TestParseListsAreNeverNil(t *testing.T) {
	r := Parse("")
	for name, l := range map[string][]string{"req_tech": r.ReqTechSkills, "pref_tech": r.PrefTechSkills,
		"req_soft": r.ReqSoftSkills, "pref_soft": r.PrefSoftSkills, "requirements": r.Requirements} {
		if l == nil {
			t.Errorf("%s is nil", name)
		}
	}
}

func TestClean(t *testing.T) {
	cases := []struct {
		name, in, want string
		hidden         int
	}{
		{"line endings", "a\r\nb\rc\u2028d", "a\nb\nc\nd", 0},
		{"spaces and tabs", "  Go \t and\u00a0 SQL  ", "Go and SQL", 0},
		{"blank runs", "\n\na\n\n\n\n b \n \n\nc\n\n", "a\n\nb\n\nc", 0},
		{"bullets", "• one\n·two\n* three\n+ four\n– five\n1. six\n12) seven\n✓ eight", "- one\n- two\n- three\n- four\n- five\n- six\n- seven\n- eight", 0},
		{"not bullets", "-5°C freezer\n*required\n2026 plans\n5+ years", "-5°C freezer\n*required\n2026 plans\n5+ years", 0},
		{"empty bullet", "•\nreal", "real", 0},
		{"invisible characters", "Py\u200bthon\u202e and S\U000E0051L", "Python and SL", 3},
		{"leading byte order mark is not hidden", "\uFEFFJob", "Job", 0},
		{"idempotent on cleaned text", "Title\n\n- a\n- b", "Title\n\n- a\n- b", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, n := Clean(c.in)
			if got != c.want || n != c.hidden {
				t.Errorf("Clean(%q) = %q, %d; want %q, %d", c.in, got, n, c.want, c.hidden)
			}
		})
	}
}

func TestHiddenIsReported(t *testing.T) {
	r := Parse("Engineer\nRequirements\n- Go\u200b\u200c and SQL")
	if r.Hidden != 2 {
		t.Errorf("hidden = %d, want 2", r.Hidden)
	}
	if !slices.Equal(r.ReqTechSkills, []string{"go", "sql"}) {
		t.Errorf("skills = %v", r.ReqTechSkills)
	}
}

func TestHeading(t *testing.T) {
	cases := []struct {
		line string
		sec  section
		rest string
		ok   bool
	}{
		{"Requirements", secRequired, "", true},
		{"QUALIFICATIONS:", secRequired, "", true},
		{"## What you bring", secRequired, "", true},
		{"**Must have**", secRequired, "", true},
		{"Minimum Qualifications", secRequired, "", true},
		{"What You’ll Need", secRequired, "", true},
		{"Preferred Qualifications", secPreferred, "", true},
		{"Nice to have", secPreferred, "", true},
		{"Bonus points", secPreferred, "", true},
		{"Pluses", secPreferred, "", true},
		{"Plus", secPreferred, "", true},
		{"Benefits & bonus", secOther, "", true},
		{"Responsibilities", secOther, "", true},
		{"About Acme", secOther, "", true},
		{"Something else:", secOther, "", true},
		{"Requirements: Python, SQL", secRequired, "Python, SQL", true},
		{"Location: Remote", secOther, "Remote", true},
		{"- Requirements", 0, "", false},
		{"You must have 3 years of SQL.", 0, "", false},
		{"Software Engineer, Disney Plus", 0, "", false},
		{"Senior Data Analyst", 0, "", false},
		{"Note: we sponsor visas", 0, "", false},
		{strings.Repeat("requirements ", 6), 0, "", false},
	}
	for _, c := range cases {
		sec, rest, ok := heading(c.line)
		if sec != c.sec || rest != c.rest || ok != c.ok {
			t.Errorf("heading(%q) = %v, %q, %v; want %v, %q, %v", c.line, sec, rest, ok, c.sec, c.rest, c.ok)
		}
	}
}

func TestSkillsWithoutHeadings(t *testing.T) {
	r := Parse("Backend Engineer\nWe use Go, PostgreSQL and Kubernetes. Mentoring juniors is part of the job.")
	if !slices.Equal(r.ReqTechSkills, []string{"go", "kubernetes", "postgresql"}) || !slices.Equal(r.ReqSoftSkills, []string{"mentoring"}) {
		t.Errorf("req = %v, %v", r.ReqTechSkills, r.ReqSoftSkills)
	}
	if len(r.PrefTechSkills) != 0 || len(r.Requirements) != 0 {
		t.Errorf("pref = %v, requirements = %v", r.PrefTechSkills, r.Requirements)
	}
	if r.Description != "Backend Engineer\nWe use Go, PostgreSQL and Kubernetes. Mentoring juniors is part of the job." {
		t.Errorf("description = %q", r.Description)
	}
}

func TestSkillsOutsideSectionsWithRequiredSection(t *testing.T) {
	r := Parse("Engineer\nOur stack includes Kafka.\n\nResponsibilities\n- Run Docker\n\nRequirements\n- Go\n\nPreferred\n- Rust\n- Go")
	if !slices.Equal(r.ReqTechSkills, []string{"go"}) {
		t.Errorf("req = %v (context skills must not count when a required section exists)", r.ReqTechSkills)
	}
	if !slices.Equal(r.PrefTechSkills, []string{"rust"}) {
		t.Errorf("pref = %v (a required skill is not also preferred)", r.PrefTechSkills)
	}
}

func TestPreferredOnly(t *testing.T) {
	r := Parse("Engineer\nWe use Go.\n\nNice to have\n- Rust")
	if !slices.Equal(r.ReqTechSkills, []string{"go"}) || !slices.Equal(r.PrefTechSkills, []string{"rust"}) {
		t.Errorf("req = %v, pref = %v", r.ReqTechSkills, r.PrefTechSkills)
	}
	if r.Description != "Engineer\nWe use Go." {
		t.Errorf("description = %q", r.Description)
	}
}

func TestInlineHeading(t *testing.T) {
	r := Parse("Analyst\nRequirements: SQL, Tableau\nBenefits: Excel training")
	if !slices.Equal(r.ReqTechSkills, []string{"sql", "tableau"}) {
		t.Errorf("req = %v", r.ReqTechSkills)
	}
}

func TestRequirementsLimitAndOrder(t *testing.T) {
	var b strings.Builder
	b.WriteString("Engineer\nRequirements\nSome intro line, not a bullet\n")
	for i := range 40 {
		b.WriteString("- item " + string(rune('A'+i%26)) + "\n")
	}
	b.WriteString("\nBenefits\n- not a requirement\n")
	r := Parse(b.String())
	if len(r.Requirements) != 30 || r.Requirements[0] != "item A" || r.Requirements[1] != "item B" {
		t.Errorf("requirements = %d %q", len(r.Requirements), r.Requirements[:2])
	}
	r = Parse("Engineer\nRequirements\n- Go\n\nBenefits\n- Dental")
	if !slices.Equal(r.Requirements, []string{"Go"}) {
		t.Errorf("requirements = %q", r.Requirements)
	}
}

func TestDescriptionIsClipped(t *testing.T) {
	r := Parse(strings.Repeat("word ", 1000) + "\nRequirements\n- Go")
	if n := len([]rune(r.Description)); n > 2000 || n < 1800 || strings.HasSuffix(r.Description, " ") {
		t.Errorf("description has %d characters", n)
	}
	r = Parse("Requirements\n- Go")
	if r.Description != "" {
		t.Errorf("description = %q, want empty when the posting starts with requirements", r.Description)
	}
}

func TestInferred(t *testing.T) {
	cases := []struct {
		name string
		text string
		want Inferred
	}{
		{"title at company", "Software Engineer at Stripe\nSan Francisco, CA\n\nRequirements\n- Go",
			Inferred{"Stripe", "Software Engineer", "San Francisco, CA", "On-site"}},
		{"about heading", "Product Designer\n\nAbout Figma\nWe make tools.\n\nAbout the role\nDesign things.",
			Inferred{"Figma", "Product Designer", "", ""}},
		{"about the role is not a company", "Designer\n\nAbout the role\nDesign.\n\nAbout you\nYou design.",
			Inferred{"", "Designer", "", ""}},
		{"company suffix", "Staff Accountant\nWe are Harbor Holdings, LLC and we keep books.",
			Inferred{"Harbor Holdings, LLC", "Staff Accountant", "", ""}},
		{"at in the intro", "Data Engineer\nJoin the data team at Lowcountry Health and build pipelines.",
			Inferred{"Lowcountry Health", "Data Engineer", "", ""}},
		{"full stop after the company", "Nurse\nCome work at Roper St. Francis.\nWe care.",
			Inferred{"Roper St. Francis", "Nurse", "", ""}},
		{"abbreviation kept", "Nurse\nCome work at Acme Co.",
			Inferred{"Acme Co.", "Nurse", "", ""}},
		{"at least is not a company", "Data Engineer\nYou need at least 3 years.",
			Inferred{"", "Data Engineer", "", ""}},
		{"labels", "Job Title: Nurse Educator\nCompany: Roper St. Francis\nLocation: Charleston, South Carolina",
			Inferred{"Roper St. Francis", "Nurse Educator", "Charleston, South Carolina", "On-site"}},
		{"remote line", "Backend Engineer\nRemote - US\n\nRequirements\n- Go",
			Inferred{"", "Backend Engineer", "Remote - US", "Remote"}},
		{"remote in the title", "Support Engineer (Remote)\nToronto, Canada",
			Inferred{"", "Support Engineer (Remote)", "Toronto, Canada", "Remote"}},
		{"described mode", "Analyst\nThis is a fully remote position.",
			Inferred{"", "Analyst", "", "Remote"}},
		{"benefit mention is not a mode", "Analyst\nWe offer great pay.\n\nBenefits\n- Remote work stipend",
			Inferred{"", "Analyst", "", ""}},
		{"stated on-site", "Lab Technician\nThis role is on-site five days a week.",
			Inferred{"", "Lab Technician", "", "On-site"}},
		{"title skips headings and long lines", "Requirements\n" + strings.Repeat("x", 90) + "\nQA Engineer",
			Inferred{"", "QA Engineer", "", ""}},
		{"state code needs capitals", "Engineer\nWork in, or near, the office.",
			Inferred{"", "Engineer", "", ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Parse(c.text).Inferred; got != c.want {
				t.Errorf("got %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestStatedMode(t *testing.T) {
	cases := map[string]string{
		"Remote - US":             "Remote",
		"Austin, TX (Remote)":     "Remote",
		"Hybrid - Boston":         "Hybrid",
		"Austin, TX":              "",
		"Remotely located office": "",
	}
	for text, want := range cases {
		if got := StatedMode(text); got != want {
			t.Errorf("StatedMode(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestDescribedMode(t *testing.T) {
	cases := map[string]string{
		"This is a fully remote position.":         "Remote",
		"We are remote-first.":                     "Remote",
		"Hybrid role, 3 days a week in the office": "Hybrid",
		"Benefits include a remote work stipend.":  "",
		"Great team.": "",
	}
	for text, want := range cases {
		if got := DescribedMode(text); got != want {
			t.Errorf("DescribedMode(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestSpokenLanguages(t *testing.T) {
	_, soft := FindSkills("Native-level Japanese fluency and business-level English required")
	if !slices.Equal(soft, []string{"english", "japanese"}) {
		t.Errorf("soft = %q, want english and japanese", soft)
	}
	if _, soft := FindSkills("Japanese-speaking team, Mandarin a plus"); !slices.Equal(soft, []string{"chinese", "japanese"}) {
		t.Errorf("hyphenated and aliased languages: %q", soft)
	}
	// Lower case is a plant, a verb or a part of a phrase, not a language.
	if tech, soft := FindSkills("pruning the japanese maple, plain english"); len(tech)+len(soft) != 0 {
		t.Errorf("lower-case words matched: %q %q", tech, soft)
	}
	if !IsSoft("Japanese") || Canon("Cantonese") != "chinese" || !slices.Contains(Aliases("chinese"), "mandarin") {
		t.Error("languages should answer IsSoft, Canon and Aliases")
	}
}

func TestLanguageRequirementsLandInTheirSections(t *testing.T) {
	r := Parse("Analyst at Acme\nRequirements\n- Fluent Japanese\nPreferred\n- Spanish and Python")
	if !slices.Equal(r.ReqSoftSkills, []string{"japanese"}) || !slices.Equal(r.PrefSoftSkills, []string{"spanish"}) {
		t.Errorf("soft = %q / %q", r.ReqSoftSkills, r.PrefSoftSkills)
	}
}

func TestMoreSectionHeadingsEndPreferred(t *testing.T) {
	text := "Analyst at Acme\nPreferred qualifications\n- Python\nLogistics\nWe use Java in the office.\nHow we're different\nWe also use Rust."
	r := Parse(text)
	if !slices.Equal(r.PrefTechSkills, []string{"python"}) {
		t.Errorf("preferred = %q; Logistics and How we're different are not requirements", r.PrefTechSkills)
	}
}

func TestHybridPolicyIsHybrid(t *testing.T) {
	r := Parse("Analyst at Acme\nTokyo, Japan\nLocation-based hybrid policy: we expect staff in the office 25% of the time.")
	if r.Inferred.WorkMode != "Hybrid" {
		t.Errorf("work mode = %q, want Hybrid", r.Inferred.WorkMode)
	}
}

func TestPlainLinesAreRequirementsWhenBulletsWereLost(t *testing.T) {
	plain := Parse("Analyst at Acme\nMinimum qualifications\n\n    Three years of experience with SQL\n\n    Strong written communication skills\n\n    Python")
	if len(plain.Requirements) != 2 {
		t.Errorf("requirements = %q, want the two lines of four words or more", plain.Requirements)
	}
	// With real bullets, only the bullets count.
	bulleted := Parse("Analyst at Acme\nMinimum qualifications\n- Three years of experience with SQL\nAnd some other plain sentence here")
	if len(bulleted.Requirements) != 1 {
		t.Errorf("requirements = %q, want only the bullet", bulleted.Requirements)
	}
}

func TestManagedKubernetes(t *testing.T) {
	for in, want := range map[string]string{"Amazon EKS": "eks", "AWS EKS": "eks", "Elastic Kubernetes Service": "eks", "Google Kubernetes Engine": "gke", "Azure Kubernetes Service": "aks"} {
		if got := Canon(in); got != want {
			t.Errorf("Canon(%q) = %q, want %q", in, got, want)
		}
	}
	if tech, _ := FindSkills("Run services on Amazon EKS and Kubernetes."); !slices.Contains(tech, "eks") || !slices.Contains(tech, "kubernetes") {
		t.Errorf("FindSkills = %v", tech)
	}
	if !slices.Contains(Related("eks"), "kubernetes") || !slices.Contains(Related("kubernetes"), "eks") {
		t.Errorf("EKS and Kubernetes are not a likely pair: %v / %v", Related("eks"), Related("kubernetes"))
	}
	if !slices.Contains(Implied("aws"), "eks") {
		t.Errorf("EKS does not cover AWS: %v", Implied("aws"))
	}
}

func TestInlinePlusMarksASkillAsPreferred(t *testing.T) {
	r := Parse("Requirements\n- Strong SQL\n- Kafka is a plus\n- Terraform preferred\n")
	if !slices.Equal(r.ReqTechSkills, []string{"sql"}) || !slices.Equal(r.PrefTechSkills, []string{"kafka", "terraform"}) {
		t.Errorf("required %v, preferred %v", r.ReqTechSkills, r.PrefTechSkills)
	}
}
