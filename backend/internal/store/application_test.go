package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

// newJob saves a manual job and returns its id.
func newJob(t *testing.T, s *Store, company string) int64 {
	t.Helper()
	j, err := s.CreateJob(context.Background(), models.Job{CompanyName: company, PositionTitle: "Analyst", Source: "manual"})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	return j.ID
}

func TestApplicationNeedsProfile(t *testing.T) {
	s := newStore(t)
	_, err := s.CreateApplication(context.Background(), models.Application{JobID: newJob(t, s, "Acme")})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("create without profile = %v, want ErrConflict", err)
	}
}

func TestApplicationCRUD(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	p := withProfile(t, s)
	job := newJob(t, s, "Acme")

	a, err := s.CreateApplication(ctx, models.Application{JobID: job, Notes: ptr("referral")})
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "Saved" || a.ProfileID != p.ID || a.CompanyName != "Acme" || a.PositionTitle != "Analyst" || a.ArchivedAt != nil {
		t.Errorf("created = %+v", a)
	}

	_, err = s.CreateApplication(ctx, models.Application{JobID: job})
	var dup *AppConflictError
	if !errors.As(err, &dup) || dup.ApplicationID != a.ID || !errors.Is(err, ErrConflict) {
		t.Errorf("second application = %v, want AppConflictError naming %d", err, a.ID)
	}

	a.Status, a.NextStepDate = "Interviewing", ptr("2026-12-01")
	got, err := s.UpdateApplication(ctx, a.ID, a)
	if err != nil || got.Status != "Interviewing" || *got.NextStepDate != "2026-12-01" || *got.Notes != "referral" {
		t.Errorf("update = %+v, %v", got, err)
	}
	if _, err := s.UpdateApplication(ctx, 999, a); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing = %v", err)
	}
	if err := s.DeleteApplication(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetApplication(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get deleted = %v", err)
	}
}

func TestApplicationListAndArchive(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	withProfile(t, s)
	a, _ := s.CreateApplication(ctx, models.Application{JobID: newJob(t, s, "Acme")})
	b, _ := s.CreateApplication(ctx, models.Application{JobID: newJob(t, s, "Globex"), Status: "Applied"})

	// Most recently updated first: touching a moves it ahead of b.
	if got, _ := s.ListApplications(ctx, ApplicationFilter{}); len(got) != 2 || got[0].ID != b.ID {
		t.Errorf("order = %+v, want newest first", got)
	}
	time.Sleep(1100 * time.Millisecond) // updated_at has one-second resolution
	if _, err := s.UpdateApplication(ctx, a.ID, a); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListApplications(ctx, ApplicationFilter{}); got[0].ID != a.ID || got[0].CompanyName != "Acme" || got[0].PositionTitle != "Analyst" {
		t.Errorf("after update = %+v, want a first with the job's company and title", got)
	}
	if got, _ := s.ListApplications(ctx, ApplicationFilter{Status: "Applied"}); len(got) != 1 || got[0].ID != b.ID {
		t.Errorf("status filter = %+v", got)
	}
	if got, _ := s.ListApplications(ctx, ApplicationFilter{Q: "acm"}); len(got) != 1 || got[0].ID != a.ID {
		t.Errorf("q filter = %+v", got)
	}

	if err := s.ArchiveApplication(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetApplication(ctx, a.ID); got.ArchivedAt == nil || got.Status != "Saved" {
		t.Errorf("archived = %+v, want archived_at set and status kept", got)
	}
	if got, _ := s.ListApplications(ctx, ApplicationFilter{}); len(got) != 1 || got[0].ID != b.ID {
		t.Errorf("default list = %+v, want archived hidden", got)
	}
	if got, _ := s.ListApplications(ctx, ApplicationFilter{Archived: "only"}); len(got) != 1 || got[0].ID != a.ID {
		t.Errorf("archived=only = %+v", got)
	}
	if got, _ := s.ListApplications(ctx, ApplicationFilter{Archived: "include"}); len(got) != 2 {
		t.Errorf("archived=include = %d rows, want 2", len(got))
	}
	if err := s.UnarchiveApplication(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListApplications(ctx, ApplicationFilter{}); len(got) != 2 {
		t.Errorf("after unarchive = %d rows, want 2", len(got))
	}
	if err := s.ArchiveApplication(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("archive missing = %v", err)
	}
}

func TestApplicationBulk(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	withProfile(t, s)
	a, _ := s.CreateApplication(ctx, models.Application{JobID: newJob(t, s, "Acme")})
	b, _ := s.CreateApplication(ctx, models.Application{JobID: newJob(t, s, "Globex")})

	if _, err := s.BulkStatus(ctx, []int64{a.ID, 999}, "Offered"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bulk with unknown id = %v, want ErrNotFound", err)
	}
	if got, _ := s.GetApplication(ctx, a.ID); got.Status != "Saved" {
		t.Errorf("status = %s after failed bulk, want rollback to Saved", got.Status)
	}

	out, err := s.BulkStatus(ctx, []int64{b.ID, a.ID}, "Applied")
	if err != nil || len(out) != 2 || out[0].ID != b.ID {
		t.Fatalf("bulk status = %+v, %v", out, err)
	}
	if today := time.Now().Format("2006-01-02"); out[0].DateApplied == nil || *out[0].DateApplied != today {
		t.Errorf("date_applied = %v, want today", out[0].DateApplied)
	}

	out, err = s.BulkArchive(ctx, []int64{a.ID, b.ID}, true)
	if err != nil || len(out) != 2 || out[0].ArchivedAt == nil || out[1].ArchivedAt == nil {
		t.Fatalf("bulk archive = %+v, %v", out, err)
	}
	out, err = s.BulkArchive(ctx, []int64{a.ID}, false)
	if err != nil || out[0].ArchivedAt != nil {
		t.Errorf("bulk restore = %+v, %v", out, err)
	}
}

func TestJobArchiveAndTrash(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	withProfile(t, s)
	tracked := newJob(t, s, "Acme")
	free := newJob(t, s, "Globex")
	app, _ := s.CreateApplication(ctx, models.Application{JobID: tracked})

	// Archive without the flag leaves the application alone.
	if err := s.ArchiveJob(ctx, tracked, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetApplication(ctx, app.ID); got.ArchivedAt != nil {
		t.Error("ArchiveJob(false) archived the application")
	}
	if err := s.ArchiveJob(ctx, tracked, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetApplication(ctx, app.ID); got.ArchivedAt == nil {
		t.Error("ArchiveJob(true) did not archive the application")
	}
	if got, _ := s.ListJobs(ctx, JobFilter{}); len(got) != 1 || got[0].ID != free {
		t.Errorf("default job list = %+v, want archived hidden", got)
	}
	if err := s.UnarchiveJob(ctx, tracked); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetApplication(ctx, app.ID); got.ArchivedAt != nil {
		t.Error("UnarchiveJob did not restore the application")
	}

	if err := s.TrashJob(ctx, tracked); !errors.Is(err, ErrConflict) {
		t.Errorf("trash tracked job = %v, want ErrConflict", err)
	}
	if err := s.TrashJob(ctx, free); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListJobs(ctx, JobFilter{Archived: "include"}); len(got) != 1 || got[0].ID != tracked {
		t.Errorf("job list = %+v, want trashed hidden", got)
	}
	if got, _ := s.ListJobs(ctx, JobFilter{Trashed: "only"}); len(got) != 1 || got[0].ID != free {
		t.Errorf("trash = %+v", got)
	}
	if err := s.RestoreJob(ctx, free); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetJob(ctx, free); got.TrashedAt != nil {
		t.Error("RestoreJob left trashed_at set")
	}

	for name, err := range map[string]error{
		"archive":   s.ArchiveJob(ctx, 999, false),
		"unarchive": s.UnarchiveJob(ctx, 999),
		"trash":     s.TrashJob(ctx, 999),
		"restore":   s.RestoreJob(ctx, 999),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s missing job = %v, want ErrNotFound", name, err)
		}
	}
}
