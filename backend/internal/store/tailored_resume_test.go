package store

import (
	"context"
	"errors"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func TestTailoredResumeRoundTrip(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	rev := newRevision(t, s)
	created, err := s.CreateTailored(ctx, models.TailoredResume{
		JobID: rev.JobID, RevisionID: &rev.ID, Content: "new", BaseContent: "old",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Content != "new" || created.BaseContent != "old" || created.RevisionID == nil || created.CreatedAt == "" {
		t.Errorf("created = %+v", created)
	}

	list, err := s.ListTailored(ctx, rev.JobID)
	if err != nil || len(list) != 1 || list[0].Content != "" || list[0].BaseContent != "" {
		t.Errorf("a list leaves out the text: %+v, %v", list, err)
	}
	if got, _ := s.GetTailored(ctx, created.ID); got.Content != "new" {
		t.Errorf("get = %+v", got)
	}
}

func TestTailoredResumeBaseContentMayBeMissing(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	rev := newRevision(t, s)
	if _, err := s.root.Exec(`INSERT INTO tailored_resumes (job_id, content) VALUES (?, 'x')`, rev.JobID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListTailored(ctx, rev.JobID)
	got, err := s.GetTailored(ctx, list[0].ID)
	if err != nil || got.BaseContent != "" {
		t.Errorf("a NULL base_content reads as empty: %+v, %v", got, err)
	}
}

func TestTailoredResumeNeedsJob(t *testing.T) {
	s := newStore(t)
	if _, err := s.CreateTailored(context.Background(), models.TailoredResume{JobID: 99, Content: "x"}); !errors.Is(err, ErrConflict) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
}

func TestDeleteTailored(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	rev := newRevision(t, s)
	created, _ := s.CreateTailored(ctx, models.TailoredResume{JobID: rev.JobID, Content: "x"})
	if err := s.DeleteTailored(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTailored(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting again = %v, want ErrNotFound", err)
	}
}
