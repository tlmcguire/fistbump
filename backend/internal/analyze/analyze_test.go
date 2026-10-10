package analyze

import (
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func str(s string) *string { return &s }

func TestRun(t *testing.T) {
	master := models.Master{
		Profile: &models.Profile{ClearanceCerts: []string{"Security+"}},
		Experiences: []models.Experience{
			{ID: 2, JobTitle: "Engineer", Skills: []string{"golang", "sql"}, Description: str("Built services with Docker.")},
			{ID: 5, JobTitle: "Dev", Skills: []string{"go"}, Impact: str("Mentored 3 engineers")},
		},
	}
	job := models.Job{ID: 9,
		ReqTech: []string{"go", "kubernetes", "docker"}, ReqSoft: []string{"mentoring"},
		PrefTech: []string{"terraform"}}
	r := Run(job, master)
	if r.Score != 0.75 {
		t.Fatalf("score = %v, want 0.75 (go, docker, mentoring of 4)", r.Score)
	}
	if len(r.Required.Tech.Missing) != 1 || r.Required.Tech.Missing[0] != "kubernetes" {
		t.Fatalf("missing = %v", r.Required.Tech.Missing)
	}
	var goIDs []int64
	for _, m := range r.Required.Tech.Matched {
		if m.Skill == "go" {
			goIDs = m.ExperienceIDs
		}
	}
	if len(goIDs) != 2 || goIDs[0] != 2 || goIDs[1] != 5 {
		t.Fatalf("go evidence = %v", goIDs)
	}
	if len(r.Preferred.Tech.Missing) != 1 {
		t.Fatalf("preferred missing = %v", r.Preferred.Tech.Missing)
	}
}

func TestLatencyBudget(t *testing.T) {
	var exps []models.Experience
	for i := 0; i < 50; i++ {
		exps = append(exps, models.Experience{ID: int64(i), JobTitle: "x", Skills: []string{"go", "sql"}, Description: str("lorem ipsum dolor sit amet docker kubernetes")})
	}
	job := models.Job{ReqTech: []string{"go", "sql", "docker", "kubernetes", "terraform", "aws", "python"}, ReqSoft: []string{"communication", "leadership"}}
	start := time.Now()
	Run(job, models.Master{Experiences: exps})
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Fatalf("analysis took %v, budget is 1.5s", d)
	}
}
