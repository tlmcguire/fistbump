package store

import (
	"github.com/tlmcguire/fistbump/backend/internal/models"
)

const profileCols = `id, full_name, email, phone, home_location, target_positions, preferred_mode, min_desired_pay, clearance_certs, summary, skills, links, created_at, updated_at`

func scanProfile(s scanner) (*models.Profile, error) {
	var p models.Profile
	var tp, cc, sk, ln string
	err := s.Scan(&p.ID, &p.FullName, &p.Email, &p.Phone, &p.HomeLocation, &tp, &p.PreferredMode, &p.MinDesiredPay, &cc, &p.Summary, &sk, &ln, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	p.TargetPositions, p.ClearanceCerts, p.Skills, p.Links = parseList(tp), parseList(cc), parseList(sk), parseList(ln)
	return &p, nil
}

// GetProfile returns the single profile, or nil when none exists.
func (s *Store) GetProfile() (*models.Profile, error) {
	p, err := scanProfile(s.DB.QueryRow(`SELECT ` + profileCols + ` FROM profile ORDER BY id LIMIT 1`))
	if err == ErrNotFound {
		return nil, nil
	}
	return p, err
}

// UpsertProfile creates the profile row or updates the existing one.
func (s *Store) UpsertProfile(in models.Profile) (*models.Profile, error) {
	cur, err := s.GetProfile()
	if err != nil {
		return nil, err
	}
	if cur == nil {
		_, err = s.DB.Exec(`INSERT INTO profile (full_name, email, phone, home_location, target_positions, preferred_mode, min_desired_pay, clearance_certs, summary, skills, links)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`, in.FullName, nstr(in.Email), nstr(in.Phone), nstr(in.HomeLocation), jsonList(in.TargetPositions),
			nstr(in.PreferredMode), nint(in.MinDesiredPay), jsonList(in.ClearanceCerts), nstr(in.Summary), jsonList(in.Skills), jsonList(in.Links))
	} else {
		_, err = s.DB.Exec(`UPDATE profile SET full_name=?, email=?, phone=?, home_location=?, target_positions=?, preferred_mode=?,
			min_desired_pay=?, clearance_certs=?, summary=?, skills=?, links=?, updated_at=? WHERE id=?`, in.FullName, nstr(in.Email), nstr(in.Phone), nstr(in.HomeLocation),
			jsonList(in.TargetPositions), nstr(in.PreferredMode), nint(in.MinDesiredPay), jsonList(in.ClearanceCerts), nstr(in.Summary), jsonList(in.Skills), jsonList(in.Links), now(), cur.ID)
	}
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetProfile()
}

const expCols = `id, profile_id, company_name, job_title, start_date, end_date, location, description, impact, skills, sort_order`

func scanExp(s scanner) (*models.Experience, error) {
	var e models.Experience
	var sk string
	if err := s.Scan(&e.ID, &e.ProfileID, &e.CompanyName, &e.JobTitle, &e.StartDate, &e.EndDate, &e.Location, &e.Description, &e.Impact, &sk, &e.SortOrder); err != nil {
		return nil, mapErr(err)
	}
	e.Skills = parseList(sk)
	return &e, nil
}

func (s *Store) ListExperiences() ([]models.Experience, error) {
	rows, err := s.DB.Query(`SELECT ` + expCols + ` FROM experiences ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Experience{}
	for rows.Next() {
		e, err := scanExp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (s *Store) GetExperience(id int64) (*models.Experience, error) {
	return scanExp(s.DB.QueryRow(`SELECT `+expCols+` FROM experiences WHERE id=?`, id))
}

func (s *Store) CreateExperience(e models.Experience) (*models.Experience, error) {
	p, err := s.GetProfile()
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrConflict
	}
	var next int
	_ = s.DB.QueryRow(`SELECT COALESCE(MAX(sort_order)+1,0) FROM experiences WHERE profile_id=?`, p.ID).Scan(&next)
	if e.SortOrder == 0 {
		e.SortOrder = next
	}
	res, err := s.DB.Exec(`INSERT INTO experiences (profile_id, company_name, job_title, start_date, end_date, location, description, impact, skills, sort_order)
		VALUES (?,?,?,?,?,?,?,?,?,?)`, p.ID, e.CompanyName, e.JobTitle, nstr(e.StartDate), nstr(e.EndDate), nstr(e.Location),
		nstr(e.Description), nstr(e.Impact), jsonList(e.Skills), e.SortOrder)
	if err != nil {
		return nil, mapErr(err)
	}
	id, _ := res.LastInsertId()
	return s.GetExperience(id)
}

func (s *Store) UpdateExperience(id int64, e models.Experience) (*models.Experience, error) {
	res, err := s.DB.Exec(`UPDATE experiences SET company_name=?, job_title=?, start_date=?, end_date=?, location=?, description=?, impact=?, skills=?, sort_order=? WHERE id=?`,
		e.CompanyName, e.JobTitle, nstr(e.StartDate), nstr(e.EndDate), nstr(e.Location), nstr(e.Description), nstr(e.Impact), jsonList(e.Skills), e.SortOrder, id)
	if err != nil {
		return nil, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetExperience(id)
}

func (s *Store) DeleteExperience(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM experiences WHERE id=?`, id)
	if err != nil {
		return mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const eduCols = `id, profile_id, institution, degree_level, discipline, graduation_date, gpa, honors, details, sort_order`

func scanEdu(s scanner) (*models.Education, error) {
	var e models.Education
	if err := s.Scan(&e.ID, &e.ProfileID, &e.Institution, &e.DegreeLevel, &e.Discipline, &e.GraduationDate, &e.GPA, &e.Honors, &e.Details, &e.SortOrder); err != nil {
		return nil, mapErr(err)
	}
	return &e, nil
}

func (s *Store) ListEducation() ([]models.Education, error) {
	rows, err := s.DB.Query(`SELECT ` + eduCols + ` FROM education ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Education{}
	for rows.Next() {
		e, err := scanEdu(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (s *Store) GetEducation(id int64) (*models.Education, error) {
	return scanEdu(s.DB.QueryRow(`SELECT `+eduCols+` FROM education WHERE id=?`, id))
}

func (s *Store) CreateEducation(e models.Education) (*models.Education, error) {
	p, err := s.GetProfile()
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrConflict
	}
	var next int
	_ = s.DB.QueryRow(`SELECT COALESCE(MAX(sort_order)+1,0) FROM education WHERE profile_id=?`, p.ID).Scan(&next)
	if e.SortOrder == 0 {
		e.SortOrder = next
	}
	res, err := s.DB.Exec(`INSERT INTO education (profile_id, institution, degree_level, discipline, graduation_date, gpa, honors, details, sort_order)
		VALUES (?,?,?,?,?,?,?,?,?)`, p.ID, e.Institution, nstr(e.DegreeLevel), nstr(e.Discipline), nstr(e.GraduationDate), nfloat(e.GPA), nstr(e.Honors), nstr(e.Details), e.SortOrder)
	if err != nil {
		return nil, mapErr(err)
	}
	id, _ := res.LastInsertId()
	return s.GetEducation(id)
}

func (s *Store) UpdateEducation(id int64, e models.Education) (*models.Education, error) {
	res, err := s.DB.Exec(`UPDATE education SET institution=?, degree_level=?, discipline=?, graduation_date=?, gpa=?, honors=?, details=?, sort_order=? WHERE id=?`,
		e.Institution, nstr(e.DegreeLevel), nstr(e.Discipline), nstr(e.GraduationDate), nfloat(e.GPA), nstr(e.Honors), nstr(e.Details), e.SortOrder, id)
	if err != nil {
		return nil, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetEducation(id)
}

func (s *Store) DeleteEducation(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM education WHERE id=?`, id)
	if err != nil {
		return mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetMaster loads the structured master resume.
func (s *Store) GetMaster() (models.Master, error) {
	var m models.Master
	var err error
	if m.Profile, err = s.GetProfile(); err != nil {
		return m, err
	}
	if m.Experiences, err = s.ListExperiences(); err != nil {
		return m, err
	}
	m.Education, err = s.ListEducation()
	return m, err
}
