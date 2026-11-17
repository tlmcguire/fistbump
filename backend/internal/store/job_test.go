package store

import (
	"context"
	"errors"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func TestJobLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	a, err := s.CreateJob(ctx, models.Job{CompanyName: "Acme", PositionTitle: "Data Analyst", RawText: ptr("text"),
		Description: ptr("desc"), ReqTechSkills: []string{"sql", " SQL ", "excel"}, CompanyURL: ptr("https://acme.com")})
	if err != nil {
		t.Fatal(err)
	}
	if a.Source != "pasted" || len(a.ReqTechSkills) != 2 || a.PrefSoftSkills == nil || *a.CompanyURL != "https://acme.com" {
		t.Errorf("created = %+v", a)
	}
	b, _ := s.CreateJob(ctx, models.Job{CompanyName: "Globex", PositionTitle: "Engineer", Source: "manual"})

	list, err := s.ListJobs(ctx, JobFilter{})
	if err != nil || len(list) != 2 || list[0].ID != b.ID {
		t.Fatalf("list = %+v, %v; want newest first", list, err)
	}
	if list[1].RawText != nil || list[1].Description != nil {
		t.Error("list must leave out raw_text and description")
	}
	if got, _ := s.ListJobs(ctx, JobFilter{Q: "acm"}); len(got) != 1 || got[0].ID != a.ID {
		t.Errorf("q filter = %+v", got)
	}
	if got, _ := s.ListJobs(ctx, JobFilter{Q: "%"}); len(got) != 0 {
		t.Errorf("q must match %% literally, got %d", len(got))
	}
	if got, _ := s.ListJobs(ctx, JobFilter{Source: "manual"}); len(got) != 1 || got[0].ID != b.ID {
		t.Errorf("source filter = %+v", got)
	}
	if got, _ := s.ListJobs(ctx, JobFilter{Limit: 1, Offset: 1}); len(got) != 1 || got[0].ID != a.ID {
		t.Errorf("paging = %+v", got)
	}

	a.PositionTitle = "Senior Data Analyst"
	a.CompanyURL = nil
	got, err := s.UpdateJob(ctx, a.ID, a)
	if err != nil || got.PositionTitle != "Senior Data Analyst" || got.CompanyURL != nil || *got.RawText != "text" {
		t.Errorf("update = %+v, %v", got, err)
	}
	if _, err := s.UpdateJob(ctx, 99, a); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing = %v", err)
	}
	if err := s.DeleteJob(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetJob(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get deleted = %v", err)
	}
}

func TestListJobsArchivedAndTrashed(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	live, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "live"})
	archived, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "archived"})
	trashed, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "trashed"})
	s.db.ExecContext(ctx, `UPDATE jobs SET archived_at = ? WHERE id = ?`, now(), archived.ID)
	s.db.ExecContext(ctx, `UPDATE jobs SET trashed_at = ? WHERE id = ?`, now(), trashed.ID)

	ids := func(f JobFilter) []int64 {
		list, err := s.ListJobs(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		for _, j := range list {
			out = append(out, j.ID)
		}
		return out
	}
	if got := ids(JobFilter{}); len(got) != 1 || got[0] != live.ID {
		t.Errorf("default = %v, want only the live job", got)
	}
	if got := ids(JobFilter{Archived: "include"}); len(got) != 2 {
		t.Errorf("include archived = %v", got)
	}
	if got := ids(JobFilter{Archived: "only"}); len(got) != 1 || got[0] != archived.ID {
		t.Errorf("only archived = %v", got)
	}
	if got := ids(JobFilter{Trashed: "only"}); len(got) != 1 || got[0] != trashed.ID {
		t.Errorf("only trashed = %v", got)
	}
}

func TestUpsertGreenhouseJob(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	j := models.Job{CompanyName: "Acme", PositionTitle: "Analyst", ExternalID: ptr("123"), Board: ptr("acme"),
		RawText: ptr("old"), ReqTechSkills: []string{"sql"}}
	first, created, err := s.UpsertGreenhouseJob(ctx, j)
	if err != nil || !created || first.Source != "greenhouse" || *first.Board != "acme" {
		t.Fatalf("first = %+v, %v, %v", first, created, err)
	}
	j.RawText, j.ReqTechSkills = ptr("new"), []string{"python"}
	second, created, err := s.UpsertGreenhouseJob(ctx, j)
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("second = %+v, %v, %v; want the same row", second, created, err)
	}
	if *second.RawText != "new" || second.ReqTechSkills[0] != "python" || second.CreatedAt != first.CreatedAt {
		t.Errorf("refreshed = %+v", second)
	}
	// A pasted job with the same external id is a different row.
	if _, err := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "b", ExternalID: ptr("123")}); err != nil {
		t.Errorf("pasted job with the same id: %v", err)
	}
}

func TestDeleteJobWithApplication(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	p := withProfile(t, s)
	j, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "b"})
	if _, err := s.JobApplicationID(ctx, j.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("no application = %v", err)
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO applications (job_id, profile_id) VALUES (?, ?)`, j.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	appID, _ := res.LastInsertId()
	if err := s.DeleteJob(ctx, j.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("delete with application = %v, want ErrConflict", err)
	}
	if got, err := s.JobApplicationID(ctx, j.ID); err != nil || got != appID {
		t.Errorf("application id = %d, %v; want %d", got, err, appID)
	}
}
