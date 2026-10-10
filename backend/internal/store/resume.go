package store

import "github.com/tlmcguire/fistbump/backend/internal/models"

func scanResume(s scanner, withContent bool) (*models.ResumeVersion, error) {
	var r models.ResumeVersion
	var content string
	if err := s.Scan(&r.ID, &r.ProfileID, &r.VersionLabel, &r.TargetPosition, &content, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, mapErr(err)
	}
	if withContent {
		r.Content = &content
	}
	return &r, nil
}

const resumeCols = `id, profile_id, version_label, target_position, content, created_at, updated_at`

func (s *Store) ListResumes() ([]models.ResumeVersion, error) {
	rows, err := s.DB.Query(`SELECT ` + resumeCols + ` FROM resumes ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.ResumeVersion{}
	for rows.Next() {
		r, err := scanResume(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (s *Store) GetResume(id int64) (*models.ResumeVersion, error) {
	return scanResume(s.DB.QueryRow(`SELECT `+resumeCols+` FROM resumes WHERE id=?`, id), true)
}

func (s *Store) CreateResume(label string, target *string, content string) (*models.ResumeVersion, error) {
	p, err := s.GetProfile()
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrConflict
	}
	res, err := s.DB.Exec(`INSERT INTO resumes (profile_id, version_label, target_position, content) VALUES (?,?,?,?)`, p.ID, label, nstr(target), content)
	if err != nil {
		return nil, mapErr(err)
	}
	id, _ := res.LastInsertId()
	return s.GetResume(id)
}

func (s *Store) UpdateResume(id int64, label *string, target *string, content *string) (*models.ResumeVersion, error) {
	cur, err := s.GetResume(id)
	if err != nil {
		return nil, err
	}
	if label != nil {
		cur.VersionLabel = *label
	}
	if target != nil {
		cur.TargetPosition = target
	}
	if content != nil {
		cur.Content = content
	}
	_, err = s.DB.Exec(`UPDATE resumes SET version_label=?, target_position=?, content=?, updated_at=? WHERE id=?`,
		cur.VersionLabel, nstr(cur.TargetPosition), *cur.Content, now(), id)
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetResume(id)
}

func (s *Store) DeleteResume(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM resumes WHERE id=?`, id)
	if err != nil {
		return mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
