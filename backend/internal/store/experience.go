package store

import (
	"context"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

const experienceCols = `id, profile_id, company_name, job_title, start_date, end_date, location, description, impact, skills, sort_order`

func scanExperience(row scanner) (models.Experience, error) {
	var e models.Experience
	var skills string
	if err := row.Scan(&e.ID, &e.ProfileID, &e.CompanyName, &e.JobTitle, &e.StartDate, &e.EndDate, &e.Location,
		&e.Description, &e.Impact, &skills, &e.SortOrder); err != nil {
		return e, mapErr(err)
	}
	e.Skills = decodeList(skills)
	return e, nil
}

// ListExperiences returns the experiences by sort_order, then id.
func (s *Store) ListExperiences(ctx context.Context) ([]models.Experience, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+experienceCols+` FROM experiences ORDER BY sort_order, id`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.Experience{}
	for rows.Next() {
		e, err := scanExperience(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, mapErr(rows.Err())
}

// GetExperience returns one experience or ErrNotFound.
func (s *Store) GetExperience(ctx context.Context, id int64) (models.Experience, error) {
	return scanExperience(s.db.QueryRowContext(ctx, `SELECT `+experienceCols+` FROM experiences WHERE id = ?`, id))
}

// CreateExperience adds an experience to the profile. A zero SortOrder becomes the largest plus one.
// ErrConflict means no profile exists.
func (s *Store) CreateExperience(ctx context.Context, e models.Experience) (models.Experience, error) {
	var out models.Experience
	err := s.WithTx(ctx, func(tx *Store) error {
		p, err := tx.GetProfile(ctx)
		if err == ErrNotFound {
			return ErrConflict
		} else if err != nil {
			return err
		}
		if e.SortOrder == 0 {
			if err := tx.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM experiences`).Scan(&e.SortOrder); err != nil {
				return mapErr(err)
			}
		}
		res, err := tx.db.ExecContext(ctx, `INSERT INTO experiences (profile_id, company_name, job_title, start_date, end_date,
  location, description, impact, skills, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, e.CompanyName, e.JobTitle, e.StartDate, e.EndDate, e.Location, e.Description, e.Impact,
			encodeList(cleanList(e.Skills)), e.SortOrder)
		if err != nil {
			return mapErr(err)
		}
		id, _ := res.LastInsertId()
		out, err = tx.GetExperience(ctx, id)
		return err
	})
	return out, err
}

// UpdateExperience replaces every editable field of experience id.
func (s *Store) UpdateExperience(ctx context.Context, id int64, e models.Experience) (models.Experience, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE experiences SET company_name = ?, job_title = ?, start_date = ?, end_date = ?,
  location = ?, description = ?, impact = ?, skills = ?, sort_order = ? WHERE id = ?`,
		e.CompanyName, e.JobTitle, e.StartDate, e.EndDate, e.Location, e.Description, e.Impact,
		encodeList(cleanList(e.Skills)), e.SortOrder, id)
	if err != nil {
		return e, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return e, ErrNotFound
	}
	return s.GetExperience(ctx, id)
}

// DeleteExperience removes an experience or returns ErrNotFound.
func (s *Store) DeleteExperience(ctx context.Context, id int64) error {
	return s.deleteByID(ctx, `DELETE FROM experiences WHERE id = ?`, id)
}

// deleteByID runs a one-row DELETE and returns ErrNotFound when nothing matched.
func (s *Store) deleteByID(ctx context.Context, query string, id int64) error {
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
