package store

import (
	"context"
	"errors"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func TestProfileUpsert(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if _, err := s.GetProfile(ctx); err != ErrNotFound {
		t.Fatalf("GetProfile on empty db = %v, want ErrNotFound", err)
	}
	p, err := s.UpsertProfile(ctx, models.Profile{FullName: "Ada", Skills: []string{" SQL", "sql", "Excel", ""}})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Skills; len(got) != 2 || got[0] != "SQL" || got[1] != "Excel" {
		t.Errorf("skills = %v, want [SQL Excel] (trimmed, de-duplicated, as written)", got)
	}
	if p.TargetPositions == nil || p.Links == nil || p.ClearanceCerts == nil {
		t.Error("list fields must be empty slices, not nil")
	}
	p2, err := s.UpsertProfile(ctx, models.Profile{FullName: "Ada L", Email: ptr("ada@example.com")})
	if err != nil {
		t.Fatal(err)
	}
	if p2.ID != p.ID || p2.FullName != "Ada L" || p2.CreatedAt != p.CreatedAt {
		t.Errorf("second upsert changed identity: %+v", p2)
	}
	var n int
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM profile").Scan(&n)
	if n != 1 {
		t.Errorf("profile rows = %d, want 1", n)
	}
}

func TestProfileBadMode(t *testing.T) {
	s := newStore(t)
	_, err := s.UpsertProfile(context.Background(), models.Profile{FullName: "Ada", PreferredMode: ptr("Sideways")})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("err = %v, want ErrConflict from the CHECK constraint", err)
	}
}
