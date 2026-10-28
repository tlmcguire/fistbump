package store

import (
	"context"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

const profileCols = `id, full_name, email, phone, home_location, target_positions, preferred_mode,
  min_desired_pay, clearance_certs, summary, skills, links, created_at, updated_at`

// scanner is what *sql.Row and *sql.Rows share.
type scanner interface{ Scan(dest ...any) error }

func scanProfile(row scanner) (models.Profile, error) {
	var p models.Profile
	var targets, certs, skills, links string
	err := row.Scan(&p.ID, &p.FullName, &p.Email, &p.Phone, &p.HomeLocation, &targets, &p.PreferredMode,
		&p.MinDesiredPay, &certs, &p.Summary, &skills, &links, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, mapErr(err)
	}
	p.TargetPositions, p.ClearanceCerts, p.Skills, p.Links = decodeList(targets), decodeList(certs), decodeList(skills), decodeList(links)
	return p, nil
}

// GetProfile returns the single profile, or ErrNotFound when none exists yet.
func (s *Store) GetProfile(ctx context.Context) (models.Profile, error) {
	return scanProfile(s.db.QueryRowContext(ctx, `SELECT `+profileCols+` FROM profile ORDER BY id LIMIT 1`))
}

// UpsertProfile updates the single profile or creates it, and returns the stored row. The
// read-only fields on p are ignored.
func (s *Store) UpsertProfile(ctx context.Context, p models.Profile) (models.Profile, error) {
	var out models.Profile
	err := s.WithTx(ctx, func(tx *Store) error {
		args := []any{p.FullName, p.Email, p.Phone, p.HomeLocation, encodeList(cleanList(p.TargetPositions)), p.PreferredMode,
			p.MinDesiredPay, encodeList(cleanList(p.ClearanceCerts)), p.Summary, encodeList(cleanList(p.Skills)), encodeList(cleanList(p.Links))}
		existing, err := tx.GetProfile(ctx)
		switch {
		case err == ErrNotFound:
			_, err = tx.db.ExecContext(ctx, `INSERT INTO profile (full_name, email, phone, home_location, target_positions,
  preferred_mode, min_desired_pay, clearance_certs, summary, skills, links, created_at, updated_at)
  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, append(args, now(), now())...)
		case err != nil:
			return err
		default:
			_, err = tx.db.ExecContext(ctx, `UPDATE profile SET full_name = ?, email = ?, phone = ?, home_location = ?,
  target_positions = ?, preferred_mode = ?, min_desired_pay = ?, clearance_certs = ?, summary = ?, skills = ?,
  links = ?, updated_at = ? WHERE id = ?`, append(args, now(), existing.ID)...)
		}
		if err != nil {
			return mapErr(err)
		}
		out, err = tx.GetProfile(ctx)
		return err
	})
	return out, err
}
