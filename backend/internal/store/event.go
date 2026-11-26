package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

// EventInput is what a caller may set on an event.
type EventInput struct {
	JobID    *int64
	Kind     string
	Title    string
	StartsAt string
	EndsAt   string
	Location string
	Notes    string
}

const eventColumns = `e.id, e.job_id, e.kind, e.title, e.starts_at, e.ends_at, e.location, e.notes, e.created_at, e.updated_at,
	COALESCE(j.company_name, ''), COALESCE(j.position_title, '')`

func scanEvent(row interface{ Scan(...any) error }) (models.Event, error) {
	var e models.Event
	var job sql.NullInt64
	if err := row.Scan(&e.ID, &job, &e.Kind, &e.Title, &e.StartsAt, &e.EndsAt, &e.Location, &e.Notes, &e.CreatedAt, &e.UpdatedAt,
		&e.CompanyName, &e.PositionTitle); err != nil {
		return models.Event{}, err
	}
	if job.Valid {
		e.JobID = &job.Int64
	}
	return e, nil
}

// ListEvents returns the events that overlap [from, to); either bound may be empty. Times compare as text, which
// is right for RFC 3339 in one offset and close enough across one (the page asks with a margin).
func (s *Store) ListEvents(ctx context.Context, from, to string) ([]models.Event, error) {
	q := `SELECT ` + eventColumns + ` FROM events e LEFT JOIN jobs j ON j.id = e.job_id WHERE 1 = 1`
	args := []any{}
	if from != "" {
		q += ` AND e.ends_at > ?`
		args = append(args, from)
	}
	if to != "" {
		q += ` AND e.starts_at < ?`
		args = append(args, to)
	}
	q += ` ORDER BY e.starts_at, e.id`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []models.Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, mapErr(err)
		}
		out = append(out, e)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) GetEvent(ctx context.Context, id int64) (models.Event, error) {
	e, err := scanEvent(s.db.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM events e LEFT JOIN jobs j ON j.id = e.job_id WHERE e.id = ?`, id))
	if err != nil {
		return models.Event{}, mapErr(err)
	}
	return e, nil
}

func (s *Store) CreateEvent(ctx context.Context, in EventInput) (models.Event, error) {
	ts := now()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO events (job_id, kind, title, starts_at, ends_at, location, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.JobID, in.Kind, strings.TrimSpace(in.Title), in.StartsAt, in.EndsAt, strings.TrimSpace(in.Location), strings.TrimSpace(in.Notes), ts, ts)
	if err != nil {
		return models.Event{}, mapErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return models.Event{}, mapErr(err)
	}
	return s.GetEvent(ctx, id)
}

func (s *Store) UpdateEvent(ctx context.Context, id int64, in EventInput) (models.Event, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE events SET job_id = ?, kind = ?, title = ?, starts_at = ?, ends_at = ?, location = ?, notes = ?, updated_at = ?
		WHERE id = ?`,
		in.JobID, in.Kind, strings.TrimSpace(in.Title), in.StartsAt, in.EndsAt, strings.TrimSpace(in.Location), strings.TrimSpace(in.Notes), now(), id)
	if err != nil {
		return models.Event{}, mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Event{}, ErrNotFound
	}
	return s.GetEvent(ctx, id)
}

func (s *Store) DeleteEvent(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id = ?`, id)
	if err != nil {
		return mapErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
