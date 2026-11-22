package analyze

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/parser"
)

func ptr(s string) *string { return &s }

func master() models.Master {
	return models.Master{
		Profile: &models.Profile{ID: 1, Skills: []string{"Excel", "Golang"}, ClearanceCerts: []string{"Secret clearance", "AWS"}},
		Experiences: []models.Experience{
			{ID: 10, Skills: []string{"Postgres"}, Description: ptr("Built dashboards in Tableau for the sales team."),
				Impact: ptr("Mentored two analysts.")},
			{ID: 20, Skills: []string{"SQL"}, Description: ptr("Wrote SQL and Python jobs; go-to person for reporting.")},
			{ID: 30, Skills: nil, Description: nil, Impact: ptr("Cut report time 40% with Power BI")},
		},
	}
}

func matched(g Group) map[string][]int64 {
	out := map[string][]int64{}
	for _, m := range g.Matched {
		out[m.Skill] = m.ExperienceIDs
	}
	return out
}

func TestRun(t *testing.T) {
	job := models.Job{
		ID:             7,
		ReqTechSkills:  []string{"sql", "postgresql", "go", "aws", "kubernetes", "tableau"},
		ReqSoftSkills:  []string{"mentoring", "leadership"},
		PrefTechSkills: []string{"power bi", "excel", "python", "rust"},
		PrefSoftSkills: []string{},
	}
	r := Run(job, master())
	if r.JobID != 7 {
		t.Errorf("job_id = %d", r.JobID)
	}

	got := matched(r.Required.Tech)
	want := map[string][]int64{
		"sql":        {20}, // experience skill list and text
		"postgresql": {10}, // alias "Postgres" in a skill list
		"go":         {},   // profile skill "Golang"; "go-to" in text is not Go
		"aws":        {},   // a certification
		"tableau":    {10}, // found in a description
	}
	if len(got) != len(want) {
		t.Errorf("required tech matched = %v, want %v", got, want)
	}
	for k, ids := range want {
		if g, ok := got[k]; !ok || !slices.Equal(g, ids) {
			t.Errorf("required tech %q evidence = %v (matched %v), want %v", k, g, ok, ids)
		}
	}
	if !slices.Equal(r.Required.Tech.Missing, []string{"kubernetes"}) {
		t.Errorf("required tech missing = %v", r.Required.Tech.Missing)
	}
	if m := matched(r.Required.Soft); len(m) != 1 || !slices.Equal(m["mentoring"], []int64{10}) {
		t.Errorf("required soft matched = %v (mentoring from \"Mentored\" in impact)", m)
	}
	// A soft skill the resume does not name is not missing: a resume rarely lists them.
	if len(r.Required.Soft.Missing) != 0 || !slices.Equal(r.Required.Soft.Unmentioned, []string{"leadership"}) {
		t.Errorf("required soft missing = %v, unmentioned = %v", r.Required.Soft.Missing, r.Required.Soft.Unmentioned)
	}

	pref := matched(r.Preferred.Tech)
	if !slices.Equal(pref["power bi"], []int64{30}) || !slices.Equal(pref["excel"], []int64{}) || !slices.Equal(pref["python"], []int64{20}) {
		t.Errorf("preferred tech matched = %v", pref)
	}
	if !slices.Equal(r.Preferred.Tech.Missing, []string{"rust"}) {
		t.Errorf("preferred tech missing = %v", r.Preferred.Tech.Missing)
	}
	if r.Preferred.Soft.Matched == nil || r.Preferred.Soft.Missing == nil {
		t.Error("empty groups must not be nil")
	}

	// 5 of the 6 required technical skills. The soft skills do not count against the person.
	if want := 5.0 / 6.0; r.Score != want {
		t.Errorf("score = %v, want %v", r.Score, want)
	}
}

func TestJobSkillsAreCanonicalized(t *testing.T) {
	r := Run(models.Job{ReqTechSkills: []string{"Postgres", "K8s"}}, master())
	if m := matched(r.Required.Tech); !slices.Equal(m["Postgres"], []int64{10}) {
		t.Errorf("matched = %v; the job's own spelling is reported, matched by canonical name", m)
	}
	if !slices.Equal(r.Required.Tech.Missing, []string{"K8s"}) || r.Score != 0.5 {
		t.Errorf("missing = %v, score = %v", r.Required.Tech.Missing, r.Score)
	}
}

func TestNoRequiredSkills(t *testing.T) {
	r := Run(models.Job{PrefTechSkills: []string{"rust"}}, master())
	if r.Score != 1 {
		t.Errorf("score = %v, want 1 when nothing is required", r.Score)
	}
}

func TestNothingMatches(t *testing.T) {
	r := Run(models.Job{ReqTechSkills: []string{"rust", "haskell"}}, models.Master{})
	if r.Score != 0 || len(r.Required.Tech.Matched) != 0 || !slices.Equal(r.Required.Tech.Missing, []string{"rust", "haskell"}) {
		t.Errorf("result = %+v", r.Required.Tech)
	}
}

func TestMatchedByListedSkill(t *testing.T) {
	m := models.Master{Profile: &models.Profile{}, Experiences: []models.Experience{{ID: 1, Skills: []string{"Docker"}}}}
	r := Run(models.Job{ReqTechSkills: []string{"docker"}}, m)
	if got := matched(r.Required.Tech); !slices.Equal(got["docker"], []int64{1}) {
		t.Errorf("matched = %v", got)
	}
}

func TestMatchedByExperienceTextOnly(t *testing.T) {
	m := models.Master{Profile: &models.Profile{}, Experiences: []models.Experience{
		{ID: 1, Description: ptr("Ran nightly ETL jobs in Airflow.")},
		{ID: 2, Impact: ptr("Moved reporting to Airflow, saving 10 hours a week.")},
		{ID: 3, Description: ptr("Answered support tickets.")},
	}}
	r := Run(models.Job{ReqTechSkills: []string{"airflow"}}, m)
	if got := matched(r.Required.Tech); !slices.Equal(got["airflow"], []int64{1, 2}) {
		t.Errorf("matched = %v; want evidence from both the description and the impact", got)
	}
}

func TestMissing(t *testing.T) {
	m := models.Master{Profile: &models.Profile{Skills: []string{"SQL"}}}
	r := Run(models.Job{ReqTechSkills: []string{"sql", "rust"}, ReqSoftSkills: []string{"leadership"}}, m)
	if !slices.Equal(r.Required.Tech.Missing, []string{"rust"}) {
		t.Errorf("missing = %v", r.Required.Tech.Missing)
	}
	// The soft skill is reported apart, not as something missing.
	if len(r.Required.Soft.Missing) != 0 || !slices.Equal(r.Required.Soft.Unmentioned, []string{"leadership"}) {
		t.Errorf("soft missing = %v, unmentioned = %v", r.Required.Soft.Missing, r.Required.Soft.Unmentioned)
	}
	// The score counts the technical skills only.
	if want := 1.0 / 2.0; r.Score != want {
		t.Errorf("score = %v, want %v", r.Score, want)
	}
}

// A soft skill the resume does not name is no evidence against the person: it is reported, but it
// never counts as missing and never lowers the score.
func TestSoftSkillsAreNotMissing(t *testing.T) {
	m := models.Master{Profile: &models.Profile{Skills: []string{"Python"}}}
	r := Run(models.Job{ReqTechSkills: []string{"python"}, ReqSoftSkills: []string{"attention to detail", "communication"}}, m)
	if len(r.Required.Soft.Missing) != 0 {
		t.Errorf("missing = %v, want none", r.Required.Soft.Missing)
	}
	if !slices.Equal(r.Required.Soft.Unmentioned, []string{"attention to detail", "communication"}) {
		t.Errorf("unmentioned = %v", r.Required.Soft.Unmentioned)
	}
	if r.Score != 1 {
		t.Errorf("score = %v, want 1: the soft skills do not count", r.Score)
	}
}

// A resume that shows one skill shows the skills it covers: "rest" covers "api".
func TestCoveredSkillMatches(t *testing.T) {
	m := models.Master{Profile: &models.Profile{Skills: []string{"REST"}}}
	r := Run(models.Job{ReqTechSkills: []string{"api", "kubernetes"}}, m)
	if got := matched(r.Required.Tech); len(got) != 1 || !slices.Equal(got["api"], []int64{}) {
		t.Errorf("matched = %v, want the api covered by rest", got)
	}
	if !slices.Equal(r.Required.Tech.Missing, []string{"kubernetes"}) {
		t.Errorf("missing = %v", r.Required.Tech.Missing)
	}
}

// A resume that shows a longer skill shows the shorter one inside it: "google analytics" shows
// "analytics".
func TestSkillInsideALongerOneMatches(t *testing.T) {
	m := models.Master{Profile: &models.Profile{Skills: []string{"Google Analytics"}}}
	r := Run(models.Job{ReqTechSkills: []string{"analytics"}}, m)
	if got := matched(r.Required.Tech); len(got) != 1 {
		t.Errorf("matched = %v, want analytics", got)
	}
}

func TestCertificationsCount(t *testing.T) {
	m := models.Master{Profile: &models.Profile{ClearanceCerts: []string{"HIPAA", "SOC2"}}}
	r := Run(models.Job{ReqTechSkills: []string{"hipaa", "soc 2"}}, m)
	if len(r.Required.Tech.Matched) != 2 || r.Score != 1 {
		t.Errorf("result = %+v, score %v; certifications must count as resume skills", r.Required.Tech, r.Score)
	}
	if ids := r.Required.Tech.Matched[0].ExperienceIDs; ids == nil || len(ids) != 0 {
		t.Errorf("evidence = %v, want an empty list (no experience shows it)", ids)
	}
}

// words returns n words of resume-like text that mention a few skills.
func words(n int, seed int) string {
	vocab := strings.Fields("built maintained reporting pipelines with Python SQL and Airflow for the finance team " +
		"improved dashboards in Tableau reduced costs led weekly reviews with stakeholders mentored analysts " +
		"migrated services to Kubernetes on AWS wrote tests documented processes partnered across teams")
	out := make([]string, n)
	for i := range out {
		out[i] = vocab[(i*7+seed)%len(vocab)]
		if i%15 == 14 {
			out[i] += "."
		}
	}
	return strings.Join(out, " ")
}

// TestLatencyNFR3 checks NFR-3: parsing a 10,000-word posting and analyzing it against a master
// with 30 experiences of 300 words each takes under 1.5 s. Run with -v to see the margin.
func TestLatencyNFR3(t *testing.T) {
	m := models.Master{Profile: &models.Profile{Skills: []string{"Excel"}, ClearanceCerts: []string{"AWS"}}}
	for i := range 30 {
		desc, impact := words(200, i), words(100, i+3)
		m.Experiences = append(m.Experiences, models.Experience{ID: int64(i + 1), Skills: []string{"SQL"}, Description: &desc, Impact: &impact})
	}
	var b strings.Builder
	b.WriteString("Senior Data Engineer\nAcme Data\nCharleston, SC\n\nRequirements\n")
	for i := 0; i < 500; i++ {
		b.WriteString("- " + words(20, i) + "\n")
	}
	posting := b.String()
	if n := len(strings.Fields(posting)); n < 10000 {
		t.Fatalf("posting has %d words, want at least 10,000", n)
	}

	start := time.Now()
	res := parser.Parse(posting)
	job := models.Job{ReqTechSkills: res.ReqTechSkills, PrefTechSkills: res.PrefTechSkills,
		ReqSoftSkills: res.ReqSoftSkills, PrefSoftSkills: res.PrefSoftSkills}
	r := Run(job, m)
	d := time.Since(start)

	t.Logf("NFR-3: parse + analyze took %v (limit 1.5s, %.0fx margin); analyze alone %d ms", d, 1.5/d.Seconds(), r.DurationMS)
	if d >= 1500*time.Millisecond {
		t.Errorf("parse + analyze took %v, over the 1.5 s NFR-3 limit", d)
	}
	if len(res.ReqTechSkills) == 0 || r.Score == 0 {
		t.Errorf("the latency test found nothing to compare: %v, score %v", res.ReqTechSkills, r.Score)
	}
}

// NFR-3 allows 1.5 s. Search ranking also runs this once per result, so it must stay far below that.
func TestRunSpeed(t *testing.T) {
	m := master()
	long := strings.Repeat("Led a team building Go, Kubernetes and PostgreSQL services with strong communication. ", 60)
	for i := range 40 {
		m.Experiences = append(m.Experiences, models.Experience{ID: int64(100 + i), Description: &long, Impact: &long})
	}
	job := models.Job{ReqTechSkills: []string{"go", "kubernetes", "postgresql", "rust", "sql"}, ReqSoftSkills: []string{"communication", "leadership"},
		PrefTechSkills: []string{"terraform", "aws"}}
	// A run takes about 15 ms alone but can take ten times that when other test packages and the race
	// detector share the machine. The fastest of a few runs shows what the code costs, not the load.
	fastest := time.Hour
	for range 5 {
		start := time.Now()
		r := Run(job, m)
		elapsed := time.Since(start)
		fastest = min(fastest, elapsed)
		if r.DurationMS < 0 || r.DurationMS > elapsed.Milliseconds() {
			t.Errorf("duration_ms = %d for a run of %v", r.DurationMS, elapsed)
		}
	}
	// The race detector and shared CI runners slow this loop by 10x or more, so the limit is looser then.
	limit := 200 * time.Millisecond
	if raceEnabled {
		limit = 1500 * time.Millisecond
	}
	if fastest > limit {
		t.Errorf("the fastest of 5 runs took %v, over %v", fastest, limit)
	}
}
