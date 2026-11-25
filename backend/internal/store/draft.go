package store

import (
	"context"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func (s *Store) ListDrafts(ctx context.Context, jobID int64) ([]models.ApplicationDraft, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, job_id, kind, prompt, content, created_at, updated_at
		FROM application_drafts
		WHERE job_id = ?
		ORDER BY updated_at DESC, id DESC`, jobID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.ApplicationDraft{}
	for rows.Next() {
		var d models.ApplicationDraft
		if err := rows.Scan(&d.ID, &d.JobID, &d.Kind, &d.Prompt, &d.Content, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, d)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) GetDraft(ctx context.Context, jobID, id int64) (models.ApplicationDraft, error) {
	var d models.ApplicationDraft
	err := s.db.QueryRowContext(ctx, `
		SELECT id, job_id, kind, prompt, content, created_at, updated_at
		FROM application_drafts WHERE id = ? AND job_id = ?`, id, jobID).Scan(
		&d.ID, &d.JobID, &d.Kind, &d.Prompt, &d.Content, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return models.ApplicationDraft{}, mapErr(err)
	}
	return d, nil
}

func (s *Store) CreateDraft(ctx context.Context, jobID int64, kind, prompt, content string) (models.ApplicationDraft, error) {
	ts := now()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO application_drafts (job_id, kind, prompt, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		jobID, strings.TrimSpace(kind), strings.TrimSpace(prompt), strings.TrimSpace(content), ts, ts)
	if err != nil {
		return models.ApplicationDraft{}, mapErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return models.ApplicationDraft{}, mapErr(err)
	}
	return s.GetDraft(ctx, jobID, id)
}

func (s *Store) UpdateDraft(ctx context.Context, jobID, id int64, prompt, content string) (models.ApplicationDraft, error) {
	ts := now()
	res, err := s.db.ExecContext(ctx, `
		UPDATE application_drafts SET prompt = ?, content = ?, updated_at = ?
		WHERE id = ? AND job_id = ?`,
		strings.TrimSpace(prompt), strings.TrimSpace(content), ts, id, jobID)
	if err != nil {
		return models.ApplicationDraft{}, mapErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.ApplicationDraft{}, ErrNotFound
	}
	return s.GetDraft(ctx, jobID, id)
}

func (s *Store) DeleteDraft(ctx context.Context, jobID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM application_drafts WHERE id = ? AND job_id = ?`, id, jobID)
	if err != nil {
		return mapErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
