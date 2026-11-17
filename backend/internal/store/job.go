package store

import (
	"context"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

const jobCols = `id, company_name, position_title, listing_url, source, external_id, board, company_url, work_mode,
  employment_type, location, pay_min, pay_max, description, raw_text, req_tech_skills, pref_tech_skills,
  req_soft_skills, pref_soft_skills, requirements, close_date, created_at, searched_at, archived_at, trashed_at`

// jobListCols is jobCols without the two large text columns, which list responses leave out.
var jobListCols = strings.NewReplacer("description,", "NULL,", "raw_text,", "NULL,").Replace(jobCols)

func scanJob(row scanner) (models.Job, error) {
	var j models.Job
	var reqTech, prefTech, reqSoft, prefSoft, reqs string
	if err := row.Scan(&j.ID, &j.CompanyName, &j.PositionTitle, &j.ListingURL, &j.Source, &j.ExternalID, &j.Board,
		&j.CompanyURL, &j.WorkMode, &j.EmploymentType, &j.Location, &j.PayMin, &j.PayMax, &j.Description, &j.RawText,
		&reqTech, &prefTech, &reqSoft, &prefSoft, &reqs, &j.CloseDate, &j.CreatedAt, &j.SearchedAt, &j.ArchivedAt,
		&j.TrashedAt); err != nil {
		return j, mapErr(err)
	}
	j.ReqTechSkills, j.PrefTechSkills = decodeList(reqTech), decodeList(prefTech)
	j.ReqSoftSkills, j.PrefSoftSkills = decodeList(reqSoft), decodeList(prefSoft)
	j.Requirements = decodeList(reqs)
	return j, nil
}

// JobFilter narrows ListJobs. Archived is "" (hide archived), "include" or "only"; Trashed is ""
// (hide trashed) or "only". The api layer validates them.
type JobFilter struct {
	Q        string // substring of the company or title
	Source   string
	Archived string
	Trashed  string
	Limit    int // 0 means 50
	Offset   int
}

// ListJobs returns jobs newest searched_at first, without description and raw_text.
func (s *Store) ListJobs(ctx context.Context, f JobFilter) ([]models.Job, error) {
	where, args := []string{}, []any{}
	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
		where = append(where, `(company_name LIKE ? ESCAPE '\' OR position_title LIKE ? ESCAPE '\')`)
		args = append(args, like, like)
	}
	if f.Source != "" {
		where = append(where, "source = ?")
		args = append(args, f.Source)
	}
	switch f.Archived {
	case "":
		where = append(where, "archived_at IS NULL")
	case "only":
		where = append(where, "archived_at IS NOT NULL")
	}
	if f.Trashed == "only" {
		where = append(where, "trashed_at IS NOT NULL")
	} else {
		where = append(where, "trashed_at IS NULL")
	}
	if f.Limit == 0 {
		f.Limit = 50
	}
	query := `SELECT ` + jobListCols + ` FROM jobs WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY searched_at DESC, id DESC LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, query, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, mapErr(rows.Err())
}

// GetJob returns one job or ErrNotFound.
func (s *Store) GetJob(ctx context.Context, id int64) (models.Job, error) {
	return scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE id = ?`, id))
}

// jobArgs are the writable columns in the order CreateJob and UpdateJob use them.
func jobArgs(j models.Job) []any {
	return []any{j.CompanyName, j.PositionTitle, j.ListingURL, j.Board, j.CompanyURL, j.WorkMode, j.EmploymentType,
		j.Location, j.PayMin, j.PayMax, j.Description, j.RawText, encodeList(cleanList(j.ReqTechSkills)),
		encodeList(cleanList(j.PrefTechSkills)), encodeList(cleanList(j.ReqSoftSkills)),
		encodeList(cleanList(j.PrefSoftSkills)), encodeList(cleanList(j.Requirements)), j.CloseDate}
}

const jobSet = `company_name = ?, position_title = ?, listing_url = ?, board = ?, company_url = ?, work_mode = ?,
  employment_type = ?, location = ?, pay_min = ?, pay_max = ?, description = ?, raw_text = ?, req_tech_skills = ?,
  pref_tech_skills = ?, req_soft_skills = ?, pref_soft_skills = ?, requirements = ?, close_date = ?`

// CreateJob inserts a job with source and external_id as given. A duplicate (source, external_id)
// is ErrConflict.
func (s *Store) CreateJob(ctx context.Context, j models.Job) (models.Job, error) {
	if j.Source == "" {
		j.Source = "pasted"
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO jobs (source, external_id, company_name, position_title, listing_url,
  board, company_url, work_mode, employment_type, location, pay_min, pay_max, description, raw_text, req_tech_skills,
  pref_tech_skills, req_soft_skills, pref_soft_skills, requirements, close_date, created_at, searched_at)
  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		append(append([]any{j.Source, j.ExternalID}, jobArgs(j)...), now(), now())...)
	if err != nil {
		return j, mapErr(err)
	}
	id, _ := res.LastInsertId()
	return s.GetJob(ctx, id)
}

// UpdateJob replaces every writable field of job id. Source, external_id and the timestamps stay.
func (s *Store) UpdateJob(ctx context.Context, id int64, j models.Job) (models.Job, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET `+jobSet+` WHERE id = ?`, append(jobArgs(j), id)...)
	if err != nil {
		return j, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return j, ErrNotFound
	}
	return s.GetJob(ctx, id)
}

// UpsertGreenhouseJob saves a Greenhouse posting on (source, external_id): a new row, or the
// existing one with new text, skills and searched_at. created reports which.
func (s *Store) UpsertGreenhouseJob(ctx context.Context, j models.Job) (out models.Job, created bool, err error) {
	j.Source = "greenhouse"
	err = s.WithTx(ctx, func(tx *Store) error {
		var id int64
		err := tx.db.QueryRowContext(ctx, `SELECT id FROM jobs WHERE source = 'greenhouse' AND external_id = ?`, j.ExternalID).Scan(&id)
		switch err = mapErr(err); err {
		case ErrNotFound:
			created = true
			out, err = tx.CreateJob(ctx, j)
			return err
		case nil:
		default:
			return err
		}
		if _, err := tx.db.ExecContext(ctx, `UPDATE jobs SET `+jobSet+`, searched_at = ? WHERE id = ?`,
			append(jobArgs(j), now(), id)...); err != nil {
			return mapErr(err)
		}
		out, err = tx.GetJob(ctx, id)
		return err
	})
	return out, created, err
}

// DeleteJob removes a job. ErrConflict means an application references it; JobApplicationID names it.
func (s *Store) DeleteJob(ctx context.Context, id int64) error {
	return s.deleteByID(ctx, `DELETE FROM jobs WHERE id = ?`, id)
}

// JobApplicationID returns the id of an application for job id, or ErrNotFound when there is none.
func (s *Store) JobApplicationID(ctx context.Context, jobID int64) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM applications WHERE job_id = ? ORDER BY id LIMIT 1`, jobID).Scan(&id)
	return id, mapErr(err)
}
