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
