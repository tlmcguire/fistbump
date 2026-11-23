package store

import (
	"context"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

// A tailored resume's base_content can be NULL, which reads as an empty string.
const tailoredCols = `id, job_id, revision_id, base_resume_id, content, COALESCE(base_content, ''), created_at`

func scanTailored(row scanner) (models.TailoredResume, error) {
	var t models.TailoredResume
	err := row.Scan(&t.ID, &t.JobID, &t.RevisionID, &t.BaseResumeID, &t.Content, &t.BaseContent, &t.CreatedAt)
	return t, mapErr(err)
}

// CreateTailored saves a tailored resume. ErrConflict means a referenced job, revision or resume is missing.
func (s *Store) CreateTailored(ctx context.Context, t models.TailoredResume) (models.TailoredResume, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO tailored_resumes (job_id, revision_id, base_resume_id, content, base_content, created_at)
  VALUES (?, ?, ?, ?, ?, ?)`, t.JobID, t.RevisionID, t.BaseResumeID, t.Content, t.BaseContent, now())
	if err != nil {
		return models.TailoredResume{}, mapErr(err)
	}
	id, _ := res.LastInsertId()
	return s.GetTailored(ctx, id)
}

// GetTailored returns one tailored resume with its content, or ErrNotFound.
func (s *Store) GetTailored(ctx context.Context, id int64) (models.TailoredResume, error) {
	return scanTailored(s.db.QueryRowContext(ctx, `SELECT `+tailoredCols+` FROM tailored_resumes WHERE id = ?`, id))
}

// UpdateTailoredContent replaces a tailored resume's text with the user's own edit. The base text
// stays as it was, so the changes view still compares against the resume it started from.
func (s *Store) UpdateTailoredContent(ctx context.Context, id int64, content string) (models.TailoredResume, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE tailored_resumes SET content = ? WHERE id = ?`, content, id)
	if err != nil {
		return models.TailoredResume{}, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.TailoredResume{}, ErrNotFound
	}
	return s.GetTailored(ctx, id)
}

// ListTailored returns a job's tailored resumes, newest first, without their text.
func (s *Store) ListTailored(ctx context.Context, jobID int64) ([]models.TailoredResume, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, job_id, revision_id, base_resume_id, '', '', created_at
  FROM tailored_resumes WHERE job_id = ? ORDER BY id DESC`, jobID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.TailoredResume{}
	for rows.Next() {
		t, err := scanTailored(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, mapErr(rows.Err())
}

// DeleteTailored removes a tailored resume; applications that used it keep their rows.
func (s *Store) DeleteTailored(ctx context.Context, id int64) error {
	return s.deleteByID(ctx, `DELETE FROM tailored_resumes WHERE id = ?`, id)
}
