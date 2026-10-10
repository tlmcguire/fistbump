package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/db"
	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return New(conn)
}

func TestWithTxRollsBackEveryWrite(t *testing.T) {
	s := newStore(t)
	if _, err := s.UpsertProfile(models.Profile{FullName: "Ann"}); err != nil {
		t.Fatal(err)
	}
	keep, _ := s.CreateExperience(models.Experience{CompanyName: "Keep", JobTitle: "Engineer"})
	boom := errors.New("fail after writes")
	err := s.WithTx(func(tx *Store) error {
		if err := tx.DeleteExperience(keep.ID); err != nil {
			return err
		}
		if _, err := tx.CreateExperience(models.Experience{CompanyName: "New", JobTitle: "Dev"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	exps, _ := s.ListExperiences()
	if len(exps) != 1 || exps[0].CompanyName != "Keep" {
		t.Fatalf("writes survived a rolled-back transaction: %+v", exps)
	}
}

func TestSettingMinimums(t *testing.T) {
	s := newStore(t)
	if err := s.SetSetting("jobs.retention_days", float64(0)); err == nil {
		t.Fatal("retention of 0 days accepted")
	}
	if err := ValidateSetting("jobs.retention_days", float64(7)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSetting("nope", "x"); err == nil {
		t.Fatal("unknown key accepted")
	}
}
