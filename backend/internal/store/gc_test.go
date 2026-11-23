package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func TestPurgeTrashedJobs(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	old := time.Now().UTC().AddDate(0, 0, -10).Format(stampLayout)
	recent := time.Now().UTC().AddDate(0, 0, -2).Format(stampLayout)

	kept, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "live"})
	archived, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "archived"})
	inTrash, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "old"})
	fresh, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "recent"})
	tracked, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "tracked"})

	s.db.ExecContext(ctx, `UPDATE jobs SET archived_at = ? WHERE id = ?`, now(), archived.ID)
	s.db.ExecContext(ctx, `UPDATE jobs SET trashed_at = ? WHERE id = ?`, old, inTrash.ID)
	s.db.ExecContext(ctx, `UPDATE jobs SET trashed_at = ? WHERE id = ?`, recent, fresh.ID)
	s.db.ExecContext(ctx, `UPDATE jobs SET trashed_at = ? WHERE id = ?`, old, tracked.ID)

	//A job with an application is kept, even when it has been in the Trash a long time.
	withProfile(t, s)
	if _, err := s.CreateApplication(ctx, models.Application{JobID: tracked.ID, Status: "Applied"}); err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}

	n, err := s.PurgeTrashedJobs(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("purged = %d, want 1", n)
	}
	if _, err := s.GetJob(ctx, inTrash.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the old trashed job = %v, want it deleted", err)
	}
	for _, job := range []models.Job{kept, archived, fresh, tracked} {
		if _, err := s.GetJob(ctx, job.ID); err != nil {
			t.Errorf("job %q = %v, want it kept", job.PositionTitle, err)
		}
	}
}

func TestPurgeTrashedJobsKeepsWhatItCannotDelete(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	job, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "old"})
	s.db.ExecContext(ctx, `UPDATE jobs SET trashed_at = ? WHERE id = ?`,
		time.Now().UTC().Add(-2*time.Hour).Format(stampLayout), job.ID)

	//A day of grace is the shortest span, so days below one must not purge a job that is younger.
	if n, err := s.PurgeTrashedJobs(ctx, 0); err != nil || n != 0 {
		t.Errorf("purge = %d, %v; want nothing purged", n, err)
	}
	if _, err := s.GetJob(ctx, job.ID); err != nil {
		t.Errorf("get = %v, want the job kept", err)
	}
}

// An old job nobody tracks is garbage. A job with an application, an archived job and a
// trashed job are the user's, so they stay.
func TestOldJobs(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	old := time.Now().UTC().AddDate(0, 0, -40).Format(stampLayout)

	stale, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "stale"})
	fresh, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "fresh"})
	archived, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "archived"})
	trashed, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "trashed"})
	tracked, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "tracked"})

	s.db.ExecContext(ctx, `UPDATE jobs SET searched_at = ? WHERE id = ?`, old, stale.ID)
	s.db.ExecContext(ctx, `UPDATE jobs SET searched_at = ? WHERE id = ?`, old, archived.ID)
	s.db.ExecContext(ctx, `UPDATE jobs SET searched_at = ? WHERE id = ?`, old, trashed.ID)
	s.db.ExecContext(ctx, `UPDATE jobs SET searched_at = ? WHERE id = ?`, old, tracked.ID)
	s.db.ExecContext(ctx, `UPDATE jobs SET archived_at = ? WHERE id = ?`, now(), archived.ID)
	s.db.ExecContext(ctx, `UPDATE jobs SET trashed_at = ? WHERE id = ?`, now(), trashed.ID)

	withProfile(t, s)
	if _, err := s.CreateApplication(ctx, models.Application{JobID: tracked.ID, Status: "Applied"}); err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}

	// The revision, suggestion and tailored resume of the stale job go with it.
	rev, err := s.CreateRevision(ctx, stale.ID, nil, "rules")
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO suggestions (revision_id, section, original_text, proposed_text)
  VALUES (?, 'summary', 'a', 'b')`, rev.ID); err != nil {
		t.Fatalf("insert suggestion: %v", err)
	}
	if _, err := s.CreateTailored(ctx, models.TailoredResume{
		JobID: stale.ID, RevisionID: &rev.ID, Content: "c", BaseContent: "b"}); err != nil {
		t.Fatalf("CreateTailored: %v", err)
	}

	n, err := s.OldJobs(ctx, 30, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("removed = %d, want 1", n)
	}
	if _, err := s.GetJob(ctx, stale.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the old job = %v, want it deleted", err)
	}
	for _, job := range []models.Job{fresh, archived, trashed, tracked} {
		if _, err := s.GetJob(ctx, job.ID); err != nil {
			t.Errorf("job %q = %v, want it kept", job.PositionTitle, err)
		}
	}
	for _, table := range []string{"revisions", "suggestions", "tailored_resumes"} {
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s left behind = %d, want 0", table, count)
		}
	}
}

// A trashed job is deleted once it has been in the Trash for longer than the trash days.
func TestTrashedJobs(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	old := time.Now().UTC().AddDate(0, 0, -10).Format(stampLayout)
	recent := time.Now().UTC().AddDate(0, 0, -3).Format(stampLayout)

	inTrash, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "old"})
	fresh, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "recent"})
	tracked, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "tracked"})
	s.db.ExecContext(ctx, `UPDATE jobs SET trashed_at = ? WHERE id = ?`, old, inTrash.ID)
	s.db.ExecContext(ctx, `UPDATE jobs SET trashed_at = ? WHERE id = ?`, recent, fresh.ID)
	s.db.ExecContext(ctx, `UPDATE jobs SET trashed_at = ? WHERE id = ?`, old, tracked.ID)

	withProfile(t, s)
	if _, err := s.CreateApplication(ctx, models.Application{JobID: tracked.ID, Status: "Applied"}); err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}

	// Three days in the Trash is not yet the seven days the setting allows.
	n, err := s.TrashedJobs(ctx, 7, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("removed = %d, want 1", n)
	}
	if _, err := s.GetJob(ctx, inTrash.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the old trashed job = %v, want it deleted", err)
	}
	for _, job := range []models.Job{fresh, tracked} {
		if _, err := s.GetJob(ctx, job.ID); err != nil {
			t.Errorf("job %q = %v, want it kept", job.PositionTitle, err)
		}
	}
}

// A dry run counts the jobs it would remove and removes none.
func TestOldJobsDryRun(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	old := time.Now().UTC().AddDate(0, 0, -40).Format(stampLayout)
	job, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "stale"})
	s.db.ExecContext(ctx, `UPDATE jobs SET searched_at = ? WHERE id = ?`, old, job.ID)

	n, err := s.OldJobs(ctx, 30, true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("counted = %d, want 1", n)
	}
	if _, err := s.GetJob(ctx, job.ID); err != nil {
		t.Errorf("the job = %v, want it kept by the dry run", err)
	}

	n, err = s.OldJobs(ctx, 30, false)
	if err != nil || n != 1 {
		t.Errorf("removed = %d, %v; want 1", n, err)
	}
	if _, err := s.GetJob(ctx, job.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the old job = %v, want it deleted", err)
	}
}

// A day of grace is the shortest span, so days below one must not remove a job that is younger.
func TestOldJobsKeepsWhatItCannotDelete(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	job, _ := s.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "young"})
	s.db.ExecContext(ctx, `UPDATE jobs SET searched_at = ? WHERE id = ?`,
		time.Now().UTC().Add(-2*time.Hour).Format(stampLayout), job.ID)

	if n, err := s.OldJobs(ctx, 0, false); err != nil || n != 0 {
		t.Errorf("removed = %d, %v; want nothing removed", n, err)
	}
	if _, err := s.GetJob(ctx, job.ID); err != nil {
		t.Errorf("the job = %v, want it kept", err)
	}
}
