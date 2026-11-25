package store

import (
	"context"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func TestExperienceDateNAPersistence(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.UpsertProfile(ctx, models.Profile{FullName: "Ada Lovelace"}); err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateExperience(ctx, models.Experience{
		CompanyName: "Open Source",
		JobTitle:    "Maintainer",
		DateNA:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.DateNA {
		t.Fatalf("expected DateNA=true after create")
	}
	got, err := s.GetExperience(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.DateNA {
		t.Fatalf("expected DateNA=true after get")
	}
	got.DateNA = false
	updated, err := s.UpdateExperience(ctx, created.ID, got)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DateNA {
		t.Fatalf("expected DateNA=false after update")
	}
}
