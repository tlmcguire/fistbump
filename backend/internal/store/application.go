package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

// AppConflictError reports that a job already has an application. It matches ErrConflict with
// errors.Is and carries the existing row's id, so callers can name it in details.application_id.
type AppConflictError struct{ ApplicationID int64 }

func (e *AppConflictError) Error() string {
	return fmt.Sprintf("job already has application %d", e.ApplicationID)
}

// Is makes errors.Is(err, ErrConflict) true for an *AppConflictError.
func (e *AppConflictError) Is(target error) bool { return target == ErrConflict }

const appCols = `a.id, a.job_id, a.profile_id, a.base_resume_id, a.tailored_resume_id, a.status, a.date_applied,
  a.next_step_date, a.exported_pdf_path, a.notes, a.archived_at, a.created_at, a.updated_at, j.company_name, j.position_title`

const appFrom = ` FROM applications a JOIN jobs j ON j.id = a.job_id`

func scanApplication(row scanner) (models.Application, error) {
	var a models.Application
	err := row.Scan(&a.ID, &a.JobID, &a.ProfileID, &a.BaseResumeID, &a.TailoredResumeID, &a.Status, &a.DateApplied,
		&a.NextStepDate, &a.ExportedPDFPath, &a.Notes, &a.ArchivedAt, &a.CreatedAt, &a.UpdatedAt, &a.CompanyName, &a.PositionTitle)
	return a, mapErr(err)
}

// today is the local calendar date, the format date_applied uses.
func today() string { return time.Now().Format("2006-01-02") }

// ApplicationFilter narrows ListApplications. Archived is "" (hide archived), "include" or
// "only". The api layer validates the values.
type ApplicationFilter struct {
	Status   string // exact pipeline stage
	Q        string // substring of the job's company or title
	Archived string
}

// ListApplications returns applications most recently updated first, each with its job's company
// and title.
func (s *Store) ListApplications(ctx context.Context, f ApplicationFilter) ([]models.Application, error) {
	where, args := []string{"1 = 1"}, []any{}
	switch f.Archived {
	case "include":
	case "only":
		where = append(where, "a.archived_at IS NOT NULL")
	default:
		where = append(where, "a.archived_at IS NULL")
	}
	if f.Status != "" {
		where = append(where, "a.status = ?")
		args = append(args, f.Status)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
		where = append(where, `(j.company_name LIKE ? ESCAPE '\' OR j.position_title LIKE ? ESCAPE '\')`)
		args = append(args, like, like)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+appCols+appFrom+` WHERE `+strings.Join(where, " AND ")+
		` ORDER BY a.updated_at DESC, a.id DESC`, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.Application{}
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, mapErr(rows.Err())
}

// GetApplication returns one application or ErrNotFound.
func (s *Store) GetApplication(ctx context.Context, id int64) (models.Application, error) {
	return scanApplication(s.db.QueryRowContext(ctx, `SELECT `+appCols+appFrom+` WHERE a.id = ?`, id))
}

// CreateApplication inserts an application for the current profile. Status defaults to Saved.
// With no profile it returns ErrConflict; when the job already has an application it returns an
// *AppConflictError (which also matches ErrConflict).
func (s *Store) CreateApplication(ctx context.Context, a models.Application) (models.Application, error) {
	p, err := s.GetProfile(ctx)
	if errors.Is(err, ErrNotFound) {
		return a, fmt.Errorf("%w: no profile", ErrConflict)
	}
	if err != nil {
		return a, err
	}
	if a.Status == "" {
		a.Status = "Saved"
	}
	ts := now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO applications (job_id, profile_id, base_resume_id, tailored_resume_id,
  status, date_applied, next_step_date, exported_pdf_path, notes, created_at, updated_at)
  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, a.JobID, p.ID, a.BaseResumeID, a.TailoredResumeID, a.Status,
		a.DateApplied, a.NextStepDate, a.ExportedPDFPath, a.Notes, ts, ts)
	if err != nil {
		err = mapErr(err)
		if errors.Is(err, ErrConflict) {
			if id, lookErr := s.JobApplicationID(ctx, a.JobID); lookErr == nil {
				return a, &AppConflictError{ApplicationID: id}
			}
		}
		return a, err
	}
	id, _ := res.LastInsertId()
	return s.GetApplication(ctx, id)
}

// UpdateApplication writes every writable field of a to application id and sets updated_at. The
// job, profile, archive flag and timestamps stay. The api layer merges a partial PUT onto the
// stored row before calling this.
func (s *Store) UpdateApplication(ctx context.Context, id int64, a models.Application) (models.Application, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE applications SET base_resume_id = ?, tailored_resume_id = ?, status = ?,
  date_applied = ?, next_step_date = ?, exported_pdf_path = ?, notes = ?, updated_at = ? WHERE id = ?`,
		a.BaseResumeID, a.TailoredResumeID, a.Status, a.DateApplied, a.NextStepDate, a.ExportedPDFPath, a.Notes, now(), id)
	if err != nil {
		return a, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return a, ErrNotFound
	}
	return s.GetApplication(ctx, id)
}

// DeleteApplication removes an application.
func (s *Store) DeleteApplication(ctx context.Context, id int64) error {
	return s.deleteByID(ctx, `DELETE FROM applications WHERE id = ?`, id)
}

// setStatus sets one application's status and updated_at. Moving to Applied fills date_applied
// with today's date when it is empty.
func (s *Store) setStatus(ctx context.Context, id int64, status string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE applications SET status = ?,
  date_applied = CASE WHEN ? = 'Applied' THEN COALESCE(date_applied, ?) ELSE date_applied END,
  updated_at = ? WHERE id = ?`, status, status, today(), now(), id)
	if err != nil {
		return mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// setArchived stamps or clears archived_at on one application and sets updated_at. Archiving an
// already archived application keeps its first archived_at.
func (s *Store) setArchived(ctx context.Context, id int64, archived bool) error {
	query, args := `UPDATE applications SET archived_at = NULL, updated_at = ? WHERE id = ?`, []any{now(), id}
	if archived {
		query = `UPDATE applications SET archived_at = COALESCE(archived_at, ?), updated_at = ? WHERE id = ?`
		args = []any{now(), now(), id}
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ArchiveApplication hides an application from the default list. Its status does not change.
func (s *Store) ArchiveApplication(ctx context.Context, id int64) error {
	return s.setArchived(ctx, id, true)
}

// UnarchiveApplication returns an archived application to the default list.
func (s *Store) UnarchiveApplication(ctx context.Context, id int64) error {
	return s.setArchived(ctx, id, false)
}

// bulk runs fn for each id in one transaction and returns the updated rows in ids order. Any
// failure, including an unknown id (ErrNotFound), rolls back every row.
func (s *Store) bulk(ctx context.Context, ids []int64, fn func(tx *Store, id int64) error) ([]models.Application, error) {
	out := []models.Application{}
	err := s.WithTx(ctx, func(tx *Store) error {
		for _, id := range ids {
			if err := fn(tx, id); err != nil {
				if errors.Is(err, ErrNotFound) {
					return fmt.Errorf("application %d: %w", id, ErrNotFound)
				}
				return err
			}
			a, err := tx.GetApplication(ctx, id)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BulkStatus sets one status on many applications, all or nothing.
func (s *Store) BulkStatus(ctx context.Context, ids []int64, status string) ([]models.Application, error) {
	return s.bulk(ctx, ids, func(tx *Store, id int64) error { return tx.setStatus(ctx, id, status) })
}

// BulkArchive archives (archived true) or restores many applications, all or nothing.
func (s *Store) BulkArchive(ctx context.Context, ids []int64, archived bool) ([]models.Application, error) {
	return s.bulk(ctx, ids, func(tx *Store, id int64) error { return tx.setArchived(ctx, id, archived) })
}
