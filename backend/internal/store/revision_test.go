package store

import (
	"context"
	"errors"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

// newRevision creates a job and a queued revision for it.
func newRevision(t *testing.T, s *Store) models.Revision {
	t.Helper()
	ctx := context.Background()
	job, err := s.CreateJob(ctx, models.Job{CompanyName: "Acme", PositionTitle: "Analyst"})
	if err != nil {
		t.Fatal(err)
	}
	rev, err := s.CreateRevision(ctx, job.ID, nil, "rules")
	if err != nil {
		t.Fatal(err)
	}
	return rev
}

func TestRevisionLifecycle(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	rev := newRevision(t, s)
	if rev.Status != "queued" || rev.Engine != "rules" || rev.CompletedAt != nil {
		t.Fatalf("new revision = %+v", rev)
	}

	if err := s.StartRevision(ctx, rev.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.StartRevision(ctx, rev.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("starting twice = %v, want ErrConflict", err)
	}

	list := []models.Suggestion{{Section: "skills", Engine: "rules", OriginalText: "a, b", ProposedText: "b, a"}}
	if err := s.CompleteRevision(ctx, rev.ID, "local", list); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRevision(ctx, rev.ID)
	if got.Status != "done" || got.Engine != "local" || got.CompletedAt == nil {
		t.Errorf("completed revision = %+v", got)
	}
	if sg, _ := s.ListSuggestions(ctx, rev.ID); len(sg) != 1 || sg[0].State != "pending" {
		t.Errorf("suggestions = %+v", sg)
	}
}

func TestCanceledRevisionStaysCanceled(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	rev := newRevision(t, s)
	s.StartRevision(ctx, rev.ID)
	if err := s.FinishRevision(ctx, rev.ID, "canceled", ""); err != nil {
		t.Fatal(err)
	}
	list := []models.Suggestion{{Section: "skills", Engine: "rules", OriginalText: "a", ProposedText: "b"}}
	if err := s.CompleteRevision(ctx, rev.ID, "rules", list); !errors.Is(err, ErrConflict) {
		t.Errorf("completing a canceled revision = %v, want ErrConflict", err)
	}
	if err := s.FinishRevision(ctx, rev.ID, "failed", "late"); !errors.Is(err, ErrConflict) {
		t.Errorf("failing a canceled revision = %v, want ErrConflict", err)
	}
	if sg, _ := s.ListSuggestions(ctx, rev.ID); len(sg) != 0 {
		t.Errorf("a refused completion must store no suggestions, got %d", len(sg))
	}
	if got, _ := s.GetRevision(ctx, rev.ID); got.Status != "canceled" {
		t.Errorf("status = %q", got.Status)
	}
}

func TestFinishRevisionRecordsError(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	rev := newRevision(t, s)
	if err := s.FinishRevision(ctx, rev.ID, "failed", "model crashed"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRevision(ctx, rev.ID)
	if got.Status != "failed" || got.Error == nil || *got.Error != "model crashed" {
		t.Errorf("revision = %+v", got)
	}
}

func TestFailInterrupted(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	queued := newRevision(t, s)
	running := newRevision(t, s)
	s.StartRevision(ctx, running.ID)
	done := newRevision(t, s)
	s.StartRevision(ctx, done.ID)
	s.CompleteRevision(ctx, done.ID, "rules", nil)

	n, err := s.FailInterrupted(ctx)
	if err != nil || n != 2 {
		t.Fatalf("FailInterrupted = %d, %v, want 2", n, err)
	}
	for _, id := range []int64{queued.ID, running.ID} {
		got, _ := s.GetRevision(ctx, id)
		if got.Status != "failed" || got.Error == nil || *got.Error != "interrupted" {
			t.Errorf("revision %d = %+v", id, got)
		}
	}
	if got, _ := s.GetRevision(ctx, done.ID); got.Status != "done" {
		t.Errorf("a finished revision must not change, got %q", got.Status)
	}
}

func TestListRevisionsNewestFirst(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	first := newRevision(t, s)
	second, _ := s.CreateRevision(ctx, first.JobID, nil, "rules")
	list, err := s.ListRevisions(ctx, first.JobID)
	if err != nil || len(list) != 2 || list[0].ID != second.ID {
		t.Errorf("list = %+v, %v", list, err)
	}
	if other, _ := s.ListRevisions(ctx, first.JobID+99); other == nil || len(other) != 0 {
		t.Errorf("an unknown job lists as empty, not nil: %v", other)
	}
}

func TestCreateRevisionNeedsJob(t *testing.T) {
	s := newStore(t)
	if _, err := s.CreateRevision(context.Background(), 99, nil, "rules"); !errors.Is(err, ErrConflict) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
}

func TestDeleteRevisionCascades(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	rev := newRevision(t, s)
	s.StartRevision(ctx, rev.ID)
	s.CompleteRevision(ctx, rev.ID, "rules", []models.Suggestion{{Section: "skills", Engine: "rules", OriginalText: "a", ProposedText: "b"}})
	tailored, err := s.CreateTailored(ctx, models.TailoredResume{JobID: rev.JobID, RevisionID: &rev.ID, Content: "x"})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteRevision(ctx, rev.ID); err != nil {
		t.Fatal(err)
	}
	if sg, _ := s.ListSuggestions(ctx, rev.ID); len(sg) != 0 {
		t.Error("suggestions must be deleted with their revision")
	}
	if got, _ := s.GetTailored(ctx, tailored.ID); got.RevisionID != nil {
		t.Errorf("a tailored resume keeps its row with a null revision, got %v", got.RevisionID)
	}
	if err := s.DeleteRevision(ctx, rev.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting again = %v, want ErrNotFound", err)
	}
}
