package store

import (
	"context"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

const suggestionCols = `id, revision_id, section, target_id, engine, original_text, proposed_text, reason, state, edited_text`

func scanSuggestion(row scanner) (models.Suggestion, error) {
	var s models.Suggestion
	err := row.Scan(&s.ID, &s.RevisionID, &s.Section, &s.TargetID, &s.Engine, &s.OriginalText, &s.ProposedText, &s.Reason, &s.State, &s.EditedText)
	return s, mapErr(err)
}

// insertSuggestions adds pending suggestions to a revision. Callers wrap it in a transaction.
func (s *Store) insertSuggestions(ctx context.Context, revisionID int64, list []models.Suggestion) error {
	for _, sg := range list {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO suggestions (revision_id, section, target_id, engine, original_text, proposed_text, reason)
  VALUES (?, ?, ?, ?, ?, ?, ?)`, revisionID, sg.Section, sg.TargetID, sg.Engine, sg.OriginalText, sg.ProposedText, sg.Reason); err != nil {
			return mapErr(err)
		}
	}
	return nil
}

// ListSuggestions returns a revision's suggestions in the order they were written. It never returns nil.
func (s *Store) ListSuggestions(ctx context.Context, revisionID int64) ([]models.Suggestion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+suggestionCols+` FROM suggestions WHERE revision_id = ? ORDER BY id`, revisionID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.Suggestion{}
	for rows.Next() {
		sg, err := scanSuggestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sg)
	}
	return out, mapErr(rows.Err())
}

// GetSuggestion returns a suggestion of the given revision, or ErrNotFound.
func (s *Store) GetSuggestion(ctx context.Context, revisionID, id int64) (models.Suggestion, error) {
	return scanSuggestion(s.db.QueryRowContext(ctx,
		`SELECT `+suggestionCols+` FROM suggestions WHERE id = ? AND revision_id = ?`, id, revisionID))
}

// DecideSuggestion records the user's decision: state is accepted, rejected or edited, and edited
// carries the user's text for an edit. ErrNotFound means no such suggestion in the revision.
func (s *Store) DecideSuggestion(ctx context.Context, revisionID, id int64, state string, edited *string) (models.Suggestion, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE suggestions SET state = ?, edited_text = ? WHERE id = ? AND revision_id = ?`,
		state, edited, id, revisionID)
	if err != nil {
		return models.Suggestion{}, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Suggestion{}, ErrNotFound
	}
	return s.GetSuggestion(ctx, revisionID, id)
}
