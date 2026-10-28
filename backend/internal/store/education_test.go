package store

import (
	"context"
	"errors"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func TestEducationCRUD(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if _, err := s.CreateEducation(ctx, models.Education{Institution: "CofC"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("create without profile = %v, want ErrConflict", err)
	}
	withProfile(t, s)
	a, err := s.CreateEducation(ctx, models.Education{Institution: "CofC", GPA: ptr(3.8)})
	if err != nil || a.SortOrder != 1 || *a.GPA != 3.8 {
		t.Fatalf("create = %+v, %v", a, err)
	}
	b, _ := s.CreateEducation(ctx, models.Education{Institution: "MIT"})
	list, _ := s.ListEducation(ctx)
	if len(list) != 2 || list[1].ID != b.ID {
		t.Errorf("list order = %+v", list)
	}
	got, err := s.UpdateEducation(ctx, a.ID, models.Education{Institution: "CofC", DegreeLevel: ptr("BS"), SortOrder: 1})
	if err != nil || got.GPA != nil || *got.DegreeLevel != "BS" {
		t.Errorf("update = %+v, %v (a full replace clears gpa)", got, err)
	}
	if _, err := s.UpdateEducation(ctx, 99, got); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing = %v", err)
	}
	if err := s.DeleteEducation(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEducation(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete = %v", err)
	}
}
