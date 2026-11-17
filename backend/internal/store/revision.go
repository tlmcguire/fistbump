package store

import (
	"context"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

const revisionCols = `id, job_id, base_resume_id, engine, status, error, created_at, completed_at`

func scanRevision(row scanner) (models.Revision, error) {
	var r models.Revision
	err := row.Scan(&r.ID, &r.JobID, &r.BaseResumeID, &r.Engine, &r.Status, &r.Error, &r.CreatedAt, &r.CompletedAt)
	return r, mapErr(err)
}

// CreateRevision adds a queued revision. ErrConflict means the job or base resume does not exist.
func (s *Store) CreateRevision(ctx context.Context, jobID int64, baseResumeID *int64, engine string) (models.Revision, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO revisions (job_id, base_resume_id, engine, created_at) VALUES (?, ?, ?, ?)`,
		jobID, baseResumeID, engine, now())
	if err != nil {
		return models.Revision{}, mapErr(err)
	}
	id, _ := res.LastInsertId()
	return s.GetRevision(ctx, id)
}

// GetRevision returns one revision, or ErrNotFound.
func (s *Store) GetRevision(ctx context.Context, id int64) (models.Revision, error) {
	return scanRevision(s.db.QueryRowContext(ctx, `SELECT `+revisionCols+` FROM revisions WHERE id = ?`, id))
}

// ListRevisions returns a job's revisions, newest first.
func (s *Store) ListRevisions(ctx context.Context, jobID int64) ([]models.Revision, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+revisionCols+` FROM revisions WHERE job_id = ? ORDER BY id DESC`, jobID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.Revision{}
	for rows.Next() {
		r, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, mapErr(rows.Err())
}

// StartRevision moves a queued revision to running. ErrConflict means it is no longer queued, for
// example because the user canceled it while it waited.
func (s *Store) StartRevision(ctx context.Context, id int64) error {
	return s.moveRevision(ctx, `UPDATE revisions SET status = 'running' WHERE id = ? AND status = 'queued'`, id)
}

// FinishRevision ends a queued or running revision as failed or canceled. A failure records msg.
// ErrConflict means the revision had already finished, so a canceled round is never overwritten.
func (s *Store) FinishRevision(ctx context.Context, id int64, status, msg string) error {
	var errText *string
	if msg != "" {
		errText = &msg
	}
	return s.moveRevision(ctx, `UPDATE revisions SET status = ?, error = ?, completed_at = ?
  WHERE id = ? AND status IN ('queued', 'running')`, status, errText, now(), id)
}

// CompleteRevision stores the suggestions and marks a running revision done, naming the engine
// that wrote them. Both happen together, or ErrConflict is returned if the revision is no longer running.
func (s *Store) CompleteRevision(ctx context.Context, id int64, engine string, suggestions []models.Suggestion) error {
	return s.WithTx(ctx, func(tx *Store) error {
		if err := tx.moveRevision(ctx, `UPDATE revisions SET status = 'done', engine = ?, completed_at = ?
  WHERE id = ? AND status = 'running'`, engine, now(), id); err != nil {
			return err
		}
		return tx.insertSuggestions(ctx, id, suggestions)
	})
}

// moveRevision runs a status update and returns ErrConflict when no row was in the expected status.
// The id is always the last argument.
func (s *Store) moveRevision(ctx context.Context, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrConflict
	}
	return nil
}

// DeleteRevision removes a revision and its suggestions. Tailored resumes built from it keep their
// rows with revision_id set to null.
func (s *Store) DeleteRevision(ctx context.Context, id int64) error {
	return s.deleteByID(ctx, `DELETE FROM revisions WHERE id = ?`, id)
}

// FailInterrupted marks rounds left queued or running by a previous launch as failed. It returns
// how many it changed.
func (s *Store) FailInterrupted(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE revisions SET status = 'failed', error = 'interrupted', completed_at = ?
  WHERE status IN ('queued', 'running')`, now())
	if err != nil {
		return 0, mapErr(err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
