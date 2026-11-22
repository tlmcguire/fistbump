package store

import (
	"context"
	"time"
)

// PurgeTrashedJobs deletes the jobs that have been in the Trash for more than days. The Trash is
// the way out of the app, so unlike the archive it does not keep the job: a job stays there only
// until it is deleted for good. A job an application still points at is kept, because the tracker
// needs it. It returns how many jobs were deleted.
func (s *Store) PurgeTrashedJobs(ctx context.Context, days int) (int64, error) {
	if days < 1 {
		days = 1
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(stampLayout)
	res, err := s.db.ExecContext(ctx, `DELETE FROM jobs
  WHERE trashed_at IS NOT NULL AND trashed_at < ?
    AND id NOT IN (SELECT job_id FROM applications)`, cutoff)
	if err != nil {
		return 0, mapErr(err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
