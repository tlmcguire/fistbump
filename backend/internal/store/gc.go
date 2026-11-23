package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PurgeTrashedJobs deletes the jobs that have been in the Trash for more than days. The Trash is
// the way out of the app, so unlike the archive it does not keep the job: a job stays there only
// until it is deleted for good. A job an application still points at is kept, because the tracker
// needs it. It returns how many jobs were deleted.
func (s *Store) PurgeTrashedJobs(ctx context.Context, days int) (int64, error) {
	return s.TrashedJobs(ctx, days, false)
}

// OldJobs removes the jobs that were last seen more than days ago. A job is only garbage when
// nothing points at it: an application keeps its job, and an archived or trashed job is the
// user's, not ours to throw away. With dryRun it only counts the jobs it would remove.
// It returns how many jobs were removed.
func (s *Store) OldJobs(ctx context.Context, days int, dryRun bool) (int64, error) {
	if days < 1 {
		days = 1
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(stampLayout)
	return s.removeJobs(ctx, "searched_at < ? AND archived_at IS NULL AND trashed_at IS NULL", cutoff, dryRun)
}

// TrashedJobs removes the jobs that have been in the Trash for more than days. A job an
// application still points at is kept. With dryRun it only counts the jobs it would remove.
// It returns how many jobs were removed.
func (s *Store) TrashedJobs(ctx context.Context, days int, dryRun bool) (int64, error) {
	if days < 1 {
		days = 1
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(stampLayout)
	return s.removeJobs(ctx, "trashed_at IS NOT NULL AND trashed_at < ?", cutoff, dryRun)
}

// removeJobs deletes the jobs matching where, or counts them when dryRun is true. The condition
// never touches a job an application points at: applications.job_id is RESTRICT, and the tracker
// still needs the job. Deleting a job takes its revisions, suggestions and tailored resumes with
// it, because the schema cascades those.
func (s *Store) removeJobs(ctx context.Context, where, cutoff string, dryRun bool) (int64, error) {
	cond := where + " AND id NOT IN (SELECT job_id FROM applications)"
	if dryRun {
		var n int64
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE "+cond, cutoff).Scan(&n); err != nil {
			return 0, mapErr(err)
		}
		return n, nil
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM jobs WHERE "+cond, cutoff)
	if err != nil {
		return 0, mapErr(err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Vacuum rewrites the database file, so the pages of deleted rows go back to the disk. SQLite
// refuses to VACUUM inside a transaction, so it runs on the pool instead of the transaction.
func (s *Store) Vacuum(ctx context.Context) error {
	if _, inTx := s.db.(*sql.Tx); inTx {
		return fmt.Errorf("vacuum cannot run inside a transaction")
	}
	if _, err := s.root.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	return nil
}
