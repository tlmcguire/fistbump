package store

import (
	"context"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

const resumeCols = `id, profile_id, version_label, target_position, content, created_at, updated_at`

func scanResume(row scanner) (models.ResumeVersion, error) {
	var r models.ResumeVersion
	err := row.Scan(&r.ID, &r.ProfileID, &r.VersionLabel, &r.TargetPosition, &r.Content, &r.CreatedAt, &r.UpdatedAt)
	return r, mapErr(err)
}

// ListResumes returns saved resumes newest first, without their content.
func (s *Store) ListResumes(ctx context.Context) ([]models.ResumeVersion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, profile_id, version_label, target_position, '', created_at, updated_at
  FROM resumes ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.ResumeVersion{}
	for rows.Next() {
		r, err := scanResume(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, mapErr(rows.Err())
}

// GetResume returns one saved resume with its content, or ErrNotFound.
func (s *Store) GetResume(ctx context.Context, id int64) (models.ResumeVersion, error) {
	return scanResume(s.db.QueryRowContext(ctx, `SELECT `+resumeCols+` FROM resumes WHERE id = ?`, id))
}

// CreateResume saves a resume under the profile. ErrConflict means no profile exists.
func (s *Store) CreateResume(ctx context.Context, r models.ResumeVersion) (models.ResumeVersion, error) {
	var out models.ResumeVersion
	err := s.WithTx(ctx, func(tx *Store) error {
		p, err := tx.GetProfile(ctx)
		if err == ErrNotFound {
			return ErrConflict
		} else if err != nil {
			return err
		}
		res, err := tx.db.ExecContext(ctx, `INSERT INTO resumes (profile_id, version_label, target_position, content, created_at, updated_at)
  VALUES (?, ?, ?, ?, ?, ?)`, p.ID, r.VersionLabel, r.TargetPosition, r.Content, now(), now())
		if err != nil {
			return mapErr(err)
		}
		id, _ := res.LastInsertId()
		out, err = tx.GetResume(ctx, id)
		return err
	})
	return out, err
}

// ResumePatch lists the fields UpdateResume changes; a nil field keeps its current value.
type ResumePatch struct {
	VersionLabel   *string
	SetTarget      bool // true changes TargetPosition, even to nil (clearing it)
	TargetPosition *string
	Content        *string
}

// UpdateResume applies any subset of fields and sets updated_at.
func (s *Store) UpdateResume(ctx context.Context, id int64, patch ResumePatch) (models.ResumeVersion, error) {
	var out models.ResumeVersion
	err := s.WithTx(ctx, func(tx *Store) error {
		cur, err := tx.GetResume(ctx, id)
		if err != nil {
			return err
		}
		if patch.VersionLabel != nil {
			cur.VersionLabel = *patch.VersionLabel
		}
		if patch.SetTarget {
			cur.TargetPosition = patch.TargetPosition
		}
		if patch.Content != nil {
			cur.Content = *patch.Content
		}
		if _, err := tx.db.ExecContext(ctx, `UPDATE resumes SET version_label = ?, target_position = ?, content = ?, updated_at = ? WHERE id = ?`,
			cur.VersionLabel, cur.TargetPosition, cur.Content, now(), id); err != nil {
			return mapErr(err)
		}
		out, err = tx.GetResume(ctx, id)
		return err
	})
	return out, err
}

// DeleteResume removes a saved resume; dependents keep their rows with the reference set to null.
func (s *Store) DeleteResume(ctx context.Context, id int64) error {
	return s.deleteByID(ctx, `DELETE FROM resumes WHERE id = ?`, id)
}
