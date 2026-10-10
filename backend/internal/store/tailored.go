package store

import "github.com/tlmcguire/fistbump/backend/internal/models"

func scanTailored(s scanner, withContent bool) (*models.TailoredResume, error) {
	var t models.TailoredResume
	var content string
	if err := s.Scan(&t.ID, &t.JobID, &t.RevisionID, &t.BaseResumeID, &content, &t.CreatedAt); err != nil {
		return nil, mapErr(err)
	}
	if withContent {
		t.Content = &content
	}
	return &t, nil
}

const tailoredCols = `id, job_id, revision_id, base_resume_id, content, created_at`

func (s *Store) CreateTailored(jobID int64, revID, baseID *int64, content string) (*models.TailoredResume, error) {
	res, err := s.DB.Exec(`INSERT INTO tailored_resumes (job_id, revision_id, base_resume_id, content) VALUES (?,?,?,?)`, jobID, nint(revID), nint(baseID), content)
	if err != nil {
		return nil, mapErr(err)
	}
	id, _ := res.LastInsertId()
	return s.GetTailored(id)
}

func (s *Store) GetTailored(id int64) (*models.TailoredResume, error) {
	return scanTailored(s.DB.QueryRow(`SELECT `+tailoredCols+` FROM tailored_resumes WHERE id=?`, id), true)
}

func (s *Store) ListTailored(jobID int64) ([]models.TailoredResume, error) {
	q, args := `SELECT `+tailoredCols+` FROM tailored_resumes`, []any{}
	if jobID > 0 {
		q += ` WHERE job_id=?`
		args = append(args, jobID)
	}
	rows, err := s.DB.Query(q+` ORDER BY created_at DESC, id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TailoredResume{}
	for rows.Next() {
		t, err := scanTailored(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *Store) DeleteTailored(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM tailored_resumes WHERE id=?`, id)
	if err != nil {
		return mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
