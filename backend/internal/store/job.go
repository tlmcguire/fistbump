package store

import (
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

const jobCols = `id, company_name, position_title, listing_url, source, external_id, work_mode, employment_type, location, pay_min, pay_max,
	description, raw_text, req_tech_skills, pref_tech_skills, req_soft_skills, pref_soft_skills, requirements, close_date, created_at, searched_at, archived_at`

func scanJob(s scanner) (*models.Job, error) {
	var j models.Job
	var a, b, c, d, e string
	err := s.Scan(&j.ID, &j.CompanyName, &j.PositionTitle, &j.ListingURL, &j.Source, &j.ExternalID, &j.WorkMode, &j.EmploymentType, &j.Location,
		&j.PayMin, &j.PayMax, &j.Description, &j.RawText, &a, &b, &c, &d, &e, &j.CloseDate, &j.CreatedAt, &j.SearchedAt, &j.ArchivedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	j.ReqTech, j.PrefTech, j.ReqSoft, j.PrefSoft, j.Requirements = parseList(a), parseList(b), parseList(c), parseList(d), parseList(e)
	return &j, nil
}

func (s *Store) GetJob(id int64) (*models.Job, error) {
	return scanJob(s.DB.QueryRow(`SELECT `+jobCols+` FROM jobs WHERE id=?`, id))
}

// ListJobs returns jobs newest first, without raw_text and description. archived is "" (exclude
// archived jobs), "include", or "only".
func (s *Store) ListJobs(q, source, archived string, limit, offset int) ([]models.Job, error) {
	where, args := []string{"1=1"}, []any{}
	switch archived {
	case "include":
	case "only":
		where = append(where, "archived_at IS NOT NULL")
	default:
		where = append(where, "archived_at IS NULL")
	}
	if q != "" {
		where = append(where, `(company_name LIKE ? OR position_title LIKE ?)`)
		args = append(args, "%"+q+"%", "%"+q+"%")
	}
	if source != "" {
		where = append(where, `source=?`)
		args = append(args, source)
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit, offset)
	rows, err := s.DB.Query(`SELECT `+jobCols+` FROM jobs WHERE `+strings.Join(where, " AND ")+` ORDER BY searched_at DESC, id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		j.RawText, j.Description = nil, nil
		out = append(out, *j)
	}
	return out, rows.Err()
}

func (s *Store) CreateJob(j models.Job) (*models.Job, error) {
	res, err := s.DB.Exec(`INSERT INTO jobs (company_name, position_title, listing_url, source, external_id, work_mode, employment_type, location, pay_min, pay_max,
		description, raw_text, req_tech_skills, pref_tech_skills, req_soft_skills, pref_soft_skills, requirements, close_date)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		j.CompanyName, j.PositionTitle, nstr(j.ListingURL), j.Source, nstr(j.ExternalID), nstr(j.WorkMode), nstr(j.EmploymentType), nstr(j.Location),
		nint(j.PayMin), nint(j.PayMax), nstr(j.Description), nstr(j.RawText), jsonList(j.ReqTech), jsonList(j.PrefTech), jsonList(j.ReqSoft),
		jsonList(j.PrefSoft), jsonList(j.Requirements), nstr(j.CloseDate))
	if err != nil {
		return nil, mapErr(err)
	}
	id, _ := res.LastInsertId()
	return s.GetJob(id)
}

func (s *Store) UpdateJob(id int64, j models.Job) (*models.Job, error) {
	res, err := s.DB.Exec(`UPDATE jobs SET company_name=?, position_title=?, listing_url=?, work_mode=?, employment_type=?, location=?, pay_min=?, pay_max=?,
		description=?, raw_text=?, req_tech_skills=?, pref_tech_skills=?, req_soft_skills=?, pref_soft_skills=?, requirements=?, close_date=? WHERE id=?`,
		j.CompanyName, j.PositionTitle, nstr(j.ListingURL), nstr(j.WorkMode), nstr(j.EmploymentType), nstr(j.Location), nint(j.PayMin), nint(j.PayMax),
		nstr(j.Description), nstr(j.RawText), jsonList(j.ReqTech), jsonList(j.PrefTech), jsonList(j.ReqSoft), jsonList(j.PrefSoft), jsonList(j.Requirements),
		nstr(j.CloseDate), id)
	if err != nil {
		return nil, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetJob(id)
}

func (s *Store) DeleteJob(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM jobs WHERE id=?`, id)
	if err != nil {
		return mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertConnectorJob inserts a connector posting or refreshes searched_at on the existing row.
// The returned bool reports whether the row already existed.
func (s *Store) UpsertConnectorJob(j models.Job) (*models.Job, bool, error) {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM jobs WHERE source=? AND external_id=?`, j.Source, *j.ExternalID).Scan(&id)
	if err == nil {
		if _, err := s.DB.Exec(`UPDATE jobs SET searched_at=? WHERE id=?`, now(), id); err != nil {
			return nil, true, err
		}
		got, err := s.GetJob(id)
		return got, true, err
	}
	if mapErr(err) != ErrNotFound {
		return nil, false, err
	}
	got, err := s.CreateJob(j)
	return got, false, err
}

// SetJobArchived archives (stamping archived_at) or restores a job.
func (s *Store) SetJobArchived(id int64, archived bool) (*models.Job, error) {
	var at any
	if archived {
		at = now()
	}
	res, err := s.DB.Exec(`UPDATE jobs SET archived_at=? WHERE id=?`, at, id)
	if err != nil {
		return nil, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetJob(id)
}

// ConnectorJobs maps external ids to stored jobs for one source, to mark search results already saved.
func (s *Store) ConnectorJobs(source string, ids []string) (map[string]models.Job, error) {
	out := map[string]models.Job{}
	if len(ids) == 0 {
		return out, nil
	}
	args := []any{source}
	marks := make([]string, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args = append(args, id)
	}
	rows, err := s.DB.Query(`SELECT `+jobCols+` FROM jobs WHERE source=? AND external_id IN (`+strings.Join(marks, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		j.RawText, j.Description = nil, nil
		out[*j.ExternalID] = *j
	}
	return out, rows.Err()
}
