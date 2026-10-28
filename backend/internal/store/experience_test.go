package store

import (
	"context"
	"errors"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func TestExperienceCRUDAndOrder(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if _, err := s.CreateExperience(ctx, models.Experience{CompanyName: "Acme", JobTitle: "Analyst"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("create without profile = %v, want ErrConflict", err)
	}
	withProfile(t, s)
	a, _ := s.CreateExperience(ctx, models.Experience{CompanyName: "Acme", JobTitle: "Analyst", Skills: []string{"sql"}})
	b, _ := s.CreateExperience(ctx, models.Experience{CompanyName: "Beta", JobTitle: "Intern"})
	if a.SortOrder != 1 || b.SortOrder != 2 {
		t.Fatalf("sort orders = %d, %d, want 1, 2", a.SortOrder, b.SortOrder)
	}
	b.SortOrder = 1
	a.SortOrder = 2
	s.UpdateExperience(ctx, b.ID, b)
	s.UpdateExperience(ctx, a.ID, a)
	list, err := s.ListExperiences(ctx)
	if err != nil || len(list) != 2 || list[0].ID != b.ID {
		t.Fatalf("list = %+v, %v; want Beta first", list, err)
	}
	got, err := s.UpdateExperience(ctx, a.ID, models.Experience{CompanyName: "Acme 2", JobTitle: "Senior", SortOrder: 2})
	if err != nil || got.CompanyName != "Acme 2" || len(got.Skills) != 0 || got.Skills == nil {
		t.Errorf("update = %+v, %v", got, err)
	}
	if _, err := s.UpdateExperience(ctx, 999, got); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing = %v, want ErrNotFound", err)
	}
	if err := s.DeleteExperience(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteExperience(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete = %v, want ErrNotFound", err)
	}
	if _, err := s.GetExperience(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get deleted = %v, want ErrNotFound", err)
	}
}

func TestWithTxRollback(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	withProfile(t, s)
	boom := errors.New("boom")
	err := s.WithTx(ctx, func(tx *Store) error {
		if _, err := tx.CreateExperience(ctx, models.Experience{CompanyName: "Acme", JobTitle: "Analyst"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if list, _ := s.ListExperiences(ctx); len(list) != 0 {
		t.Errorf("rollback left %d rows", len(list))
	}
}

func TestGetMasterSkills(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	m, err := s.GetMaster(ctx)
	if err != nil || m.Profile != nil || m.Experiences == nil || m.Education == nil {
		t.Fatalf("empty master = %+v, %v", m, err)
	}
	s.UpsertProfile(ctx, models.Profile{FullName: "Ada", Skills: []string{"sql", "excel"}})
	s.CreateExperience(ctx, models.Experience{CompanyName: "A", JobTitle: "B", Skills: []string{"SQL", "python"}})
	m, _ = s.GetMaster(ctx)
	if got := m.Skills(); len(got) != 3 || got[0] != "sql" || got[2] != "python" {
		t.Errorf("Skills() = %v, want [sql excel python]", got)
	}
}
