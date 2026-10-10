package store

import "github.com/tlmcguire/fistbump/backend/internal/models"

const revCols = `id, job_id, base_resume_id, engine, status, error, created_at, completed_at`

func scanRev(s scanner) (*models.Revision, error) {
	var r models.Revision
	if err := s.Scan(&r.ID, &r.JobID, &r.BaseResumeID, &r.Engine, &r.Status, &r.Error, &r.CreatedAt, &r.CompletedAt); err != nil {
		return nil, mapErr(err)
	}
	return &r, nil
}

func (s *Store) CreateRevision(jobID int64, base *int64, engine string) (*models.Revision, error) {
	res, err := s.DB.Exec(`INSERT INTO revisions (job_id, base_resume_id, engine) VALUES (?,?,?)`, jobID, nint(base), engine)
	if err != nil {
		return nil, mapErr(err)
	}
	id, _ := res.LastInsertId()
	return s.GetRevision(id, false)
}

func (s *Store) GetRevision(id int64, withSuggestions bool) (*models.Revision, error) {
	r, err := scanRev(s.DB.QueryRow(`SELECT `+revCols+` FROM revisions WHERE id=?`, id))
	if err != nil {
		return nil, err
	}
	if withSuggestions {
		if r.Suggestions, err = s.ListSuggestions(id); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (s *Store) ListRevisions(jobID int64) ([]models.Revision, error) {
	q, args := `SELECT `+revCols+` FROM revisions`, []any{}
	if jobID > 0 {
		q += ` WHERE job_id=?`
		args = append(args, jobID)
	}
	rows, err := s.DB.Query(q+` ORDER BY created_at DESC, id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Revision{}
	for rows.Next() {
		r, err := scanRev(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// SetRevisionStatus updates status and error; terminal statuses stamp completed_at.
func (s *Store) SetRevisionStatus(id int64, status string, errMsg *string, engine string) error {
	completed := any(nil)
	switch status {
	case "done", "failed", "canceled":
		completed = now()
	}
	if engine == "" {
		_, err := s.DB.Exec(`UPDATE revisions SET status=?, error=?, completed_at=? WHERE id=?`, status, nstr(errMsg), completed, id)
		return err
	}
	_, err := s.DB.Exec(`UPDATE revisions SET status=?, error=?, completed_at=?, engine=? WHERE id=?`, status, nstr(errMsg), completed, engine, id)
	return err
}

func (s *Store) DeleteRevision(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM revisions WHERE id=?`, id)
	if err != nil {
		return mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AddSuggestion(g models.Suggestion) error {
	_, err := s.DB.Exec(`INSERT INTO suggestions (revision_id, section, target_id, original_text, proposed_text) VALUES (?,?,?,?,?)`,
		g.RevisionID, g.Section, nint(g.TargetID), g.OriginalText, g.ProposedText)
	return err
}

func (s *Store) ClearSuggestions(revisionID int64) error {
	_, err := s.DB.Exec(`DELETE FROM suggestions WHERE revision_id=?`, revisionID)
	return err
}

const sugCols = `id, revision_id, section, target_id, original_text, proposed_text, state, edited_text`

func scanSug(s scanner) (*models.Suggestion, error) {
	var g models.Suggestion
	if err := s.Scan(&g.ID, &g.RevisionID, &g.Section, &g.TargetID, &g.OriginalText, &g.ProposedText, &g.State, &g.EditedText); err != nil {
		return nil, mapErr(err)
	}
	return &g, nil
}

func (s *Store) ListSuggestions(revisionID int64) ([]models.Suggestion, error) {
	rows, err := s.DB.Query(`SELECT `+sugCols+` FROM suggestions WHERE revision_id=? ORDER BY id`, revisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Suggestion{}
	for rows.Next() {
		g, err := scanSug(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func (s *Store) GetSuggestion(revisionID, id int64) (*models.Suggestion, error) {
	return scanSug(s.DB.QueryRow(`SELECT `+sugCols+` FROM suggestions WHERE id=? AND revision_id=?`, id, revisionID))
}

// SetSuggestionState sets the state; edited is non-nil only for the "edited" state.
func (s *Store) SetSuggestionState(revisionID, id int64, state string, edited *string) (*models.Suggestion, error) {
	res, err := s.DB.Exec(`UPDATE suggestions SET state=?, edited_text=? WHERE id=? AND revision_id=?`, state, nstr(edited), id, revisionID)
	if err != nil {
		return nil, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetSuggestion(revisionID, id)
}

func (s *Store) CountPending(revisionID int64) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM suggestions WHERE revision_id=? AND state='pending'`, revisionID).Scan(&n)
	return n, err
}

// FailInterruptedRevisions marks queued or running revisions as failed. Called at startup.
func (s *Store) FailInterruptedRevisions() (int, error) {
	res, err := s.DB.Exec(`UPDATE revisions SET status='failed', error='interrupted: the app closed while generating', completed_at=?
		WHERE status IN ('queued','running')`, now())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
