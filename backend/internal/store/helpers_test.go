package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/db"
	"github.com/tlmcguire/fistbump/backend/internal/models"
)

// newStore opens a fresh migrated database in a temporary directory.
func newStore(t *testing.T) *Store {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return New(conn)
}

// withProfile creates the profile every other table hangs from.
func withProfile(t *testing.T, s *Store) models.Profile {
	t.Helper()
	p, err := s.UpsertProfile(context.Background(), models.Profile{FullName: "Ada Lovelace"})
	if err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}
	return p
}

func ptr[T any](v T) *T { return &v }
