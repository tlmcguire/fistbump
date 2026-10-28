package store

import (
	"context"
	"os"
	"testing"
)

// TestSeedLoads keeps testdata/seed.sql valid against the current schema.
func TestSeedLoads(t *testing.T) {
	sql, err := os.ReadFile("../../testdata/seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(t)
	if _, err := s.root.Exec(string(sql)); err != nil {
		t.Fatalf("seed.sql: %v", err)
	}
	m, err := s.GetMaster(context.Background())
	if err != nil || m.Profile == nil || len(m.Experiences) != 3 || len(m.Education) != 2 {
		t.Errorf("seeded master = %+v, %v", m, err)
	}
}
