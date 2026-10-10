package store

import (
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

const appCols = `a.id, a.job_id, a.profile_id, a.base_resume_id, a.tailored_resume_id, a.status, a.date_applied, a.next_step_date,
	a.exported_pdf_path, a.notes, a.created_at, a.updated_at, j.company_name, j.position_title`

func scanApp(s scanner) (*models.Application, error) {
	var a models.Application
	if err := s.Scan(&a.ID, &a.JobID, &a.ProfileID, &a.BaseResumeID, &a.TailoredResumeID, &a.Status, &a.DateApplied, &a.NextStepDate,
		&a.ExportedPDFPath, &a.Notes, &a.CreatedAt, &a.UpdatedAt, &a.CompanyName, &a.PositionTitle); err != nil {
		return nil, mapErr(err)
	}
	return &a, nil
}

func (s *Store) GetApplication(id int64) (*models.Application, error) {
	return scanApp(s.DB.QueryRow(`SELECT `+appCols+` FROM applications a JOIN jobs j ON j.id=a.job_id WHERE a.id=?`, id))
}

func (s *Store) ListApplications(status, q string) ([]models.Application, error) {
	where, args := []string{"1=1"}, []any{}
	if status != "" {
		where = append(where, `a.status=?`)
		args = append(args, status)
	}
	if q != "" {
		where = append(where, `(j.company_name LIKE ? OR j.position_title LIKE ?)`)
		args = append(args, "%"+q+"%", "%"+q+"%")
	}
	rows, err := s.DB.Query(`SELECT `+appCols+` FROM applications a JOIN jobs j ON j.id=a.job_id WHERE `+strings.Join(where, " AND ")+` ORDER BY a.updated_at DESC, a.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Application{}
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (s *Store) CreateApplication(a models.Application) (*models.Application, error) {
	p, err := s.GetProfile()
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrConflict
	}
	if a.Status == "" {
		a.Status = "Saved"
	}
	if a.Status == "Applied" && a.DateApplied == nil {
		d := time.Now().Format("2006-01-02")
		a.DateApplied = &d
	}
	res, err := s.DB.Exec(`INSERT INTO applications (job_id, profile_id, base_resume_id, tailored_resume_id, status, date_applied, next_step_date, notes)
		VALUES (?,?,?,?,?,?,?,?)`, a.JobID, p.ID, nint(a.BaseResumeID), nint(a.TailoredResumeID), a.Status, nstr(a.DateApplied), nstr(a.NextStepDate), nstr(a.Notes))
	if err != nil {
		return nil, mapErr(err)
	}
	id, _ := res.LastInsertId()
	return s.GetApplication(id)
}

// UpdateApplication writes every mutable field of a (callers merge partial updates first).
func (s *Store) UpdateApplication(id int64, a models.Application) (*models.Application, error) {
	if a.Status == "Applied" && a.DateApplied == nil {
		d := time.Now().Format("2006-01-02")
		a.DateApplied = &d
	}
	res, err := s.DB.Exec(`UPDATE applications SET status=?, date_applied=?, next_step_date=?, notes=?, base_resume_id=?, tailored_resume_id=?,
		exported_pdf_path=?, updated_at=? WHERE id=?`, a.Status, nstr(a.DateApplied), nstr(a.NextStepDate), nstr(a.Notes), nint(a.BaseResumeID),
		nint(a.TailoredResumeID), nstr(a.ExportedPDFPath), now(), id)
	if err != nil {
		return nil, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetApplication(id)
}

func (s *Store) DeleteApplication(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM applications WHERE id=?`, id)
	if err != nil {
		return mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
