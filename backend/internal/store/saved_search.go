package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func (s *Store) ListSavedSearches(ctx context.Context) ([]models.SavedSearch, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, query_json, last_run_at, seen_keys_json, created_at
		FROM saved_searches
		ORDER BY id DESC`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.SavedSearch{}
	for rows.Next() {
		item, err := scanSavedSearch(rows)
		if err != nil {
			return nil, mapErr(err)
		}
		out = append(out, item)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) GetSavedSearch(ctx context.Context, id int64) (models.SavedSearch, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, query_json, last_run_at, seen_keys_json, created_at
		FROM saved_searches WHERE id = ?`, id)
	item, err := scanSavedSearch(row)
	if err != nil {
		return models.SavedSearch{}, mapErr(err)
	}
	return item, nil
}

func (s *Store) CreateSavedSearch(ctx context.Context, name string, queryJSON string, seenKeys []string) (models.SavedSearch, error) {
	name = strings.TrimSpace(name)
	if seenKeys == nil {
		seenKeys = []string{}
	}
	seenRaw, _ := json.Marshal(seenKeys)
	ts := now()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO saved_searches (name, query_json, last_run_at, seen_keys_json, created_at)
		VALUES (?, ?, ?, ?, ?)`, name, queryJSON, ts, string(seenRaw), ts)
	if err != nil {
		return models.SavedSearch{}, mapErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return models.SavedSearch{}, mapErr(err)
	}
	return s.GetSavedSearch(ctx, id)
}

func (s *Store) UpdateSavedSearchRun(ctx context.Context, id int64, seenKeys []string) (models.SavedSearch, error) {
	if seenKeys == nil {
		seenKeys = []string{}
	}
	seenRaw, _ := json.Marshal(seenKeys)
	ts := now()
	res, err := s.db.ExecContext(ctx, `
		UPDATE saved_searches SET last_run_at = ?, seen_keys_json = ? WHERE id = ?`,
		ts, string(seenRaw), id)
	if err != nil {
		return models.SavedSearch{}, mapErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.SavedSearch{}, ErrNotFound
	}
	return s.GetSavedSearch(ctx, id)
}

func (s *Store) DeleteSavedSearch(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM saved_searches WHERE id = ?`, id)
	if err != nil {
		return mapErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanSavedSearch(sc scanner) (models.SavedSearch, error) {
	var item models.SavedSearch
	var lastRun sql.NullString
	var seenRaw string
	if err := sc.Scan(&item.ID, &item.Name, &item.QueryJSON, &lastRun, &seenRaw, &item.CreatedAt); err != nil {
		return models.SavedSearch{}, err
	}
	if lastRun.Valid {
		item.LastRunAt = &lastRun.String
	}
	item.SeenKeys = decodeList(seenRaw)
	if item.SeenKeys == nil {
		item.SeenKeys = []string{}
	}
	return item, nil
}
