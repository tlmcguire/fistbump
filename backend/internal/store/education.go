package store

import (
	"context"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

const educationCols = `id, profile_id, institution, degree_level, discipline, graduation_date, gpa, honors, details, sort_order`

func scanEducation(row scanner) (models.Education, error) {
	var e models.Education
	err := row.Scan(&e.ID, &e.ProfileID, &e.Institution, &e.DegreeLevel, &e.Discipline, &e.GraduationDate, &e.GPA,
		&e.Honors, &e.Details, &e.SortOrder)
	return e, mapErr(err)
}

// ListEducation returns the education entries by sort_order, then id.
func (s *Store) ListEducation(ctx context.Context) ([]models.Education, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+educationCols+` FROM education ORDER BY sort_order, id`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.Education{}
	for rows.Next() {
		e, err := scanEducation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, mapErr(rows.Err())
}

// GetEducation returns one entry or ErrNotFound.
func (s *Store) GetEducation(ctx context.Context, id int64) (models.Education, error) {
	return scanEducation(s.db.QueryRowContext(ctx, `SELECT `+educationCols+` FROM education WHERE id = ?`, id))
}

// CreateEducation adds an entry to the profile. A zero SortOrder becomes the largest plus one.
// ErrConflict means no profile exists.
func (s *Store) CreateEducation(ctx context.Context, e models.Education) (models.Education, error) {
	var out models.Education
	err := s.WithTx(ctx, func(tx *Store) error {
		p, err := tx.GetProfile(ctx)
		if err == ErrNotFound {
			return ErrConflict
		} else if err != nil {
			return err
		}
		if e.SortOrder == 0 {
			if err := tx.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM education`).Scan(&e.SortOrder); err != nil {
				return mapErr(err)
			}
		}
		res, err := tx.db.ExecContext(ctx, `INSERT INTO education (profile_id, institution, degree_level, discipline,
  graduation_date, gpa, honors, details, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, e.Institution, e.DegreeLevel, e.Discipline, e.GraduationDate, e.GPA, e.Honors, e.Details, e.SortOrder)
		if err != nil {
			return mapErr(err)
		}
		id, _ := res.LastInsertId()
		out, err = tx.GetEducation(ctx, id)
		return err
	})
	return out, err
}

// UpdateEducation replaces every editable field of entry id.
func (s *Store) UpdateEducation(ctx context.Context, id int64, e models.Education) (models.Education, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE education SET institution = ?, degree_level = ?, discipline = ?,
  graduation_date = ?, gpa = ?, honors = ?, details = ?, sort_order = ? WHERE id = ?`,
		e.Institution, e.DegreeLevel, e.Discipline, e.GraduationDate, e.GPA, e.Honors, e.Details, e.SortOrder, id)
	if err != nil {
		return e, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return e, ErrNotFound
	}
	return s.GetEducation(ctx, id)
}

// DeleteEducation removes an entry or returns ErrNotFound.
func (s *Store) DeleteEducation(ctx context.Context, id int64) error {
	return s.deleteByID(ctx, `DELETE FROM education WHERE id = ?`, id)
}

// GetMaster reads the profile, experiences and education in one transaction, so the master
// resume is consistent. A missing profile gives a Master with a nil Profile.
func (s *Store) GetMaster(ctx context.Context) (models.Master, error) {
	var m models.Master
	err := s.WithTx(ctx, func(tx *Store) error {
		p, err := tx.GetProfile(ctx)
		if err == nil {
			m.Profile = &p
		} else if err != ErrNotFound {
			return err
		}
		if m.Experiences, err = tx.ListExperiences(ctx); err != nil {
			return err
		}
		m.Education, err = tx.ListEducation(ctx)
		return err
	})
	return m, err
}
