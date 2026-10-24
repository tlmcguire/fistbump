package store

import (
	"context"
	"errors"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func TestResumeLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if _, err := s.CreateResume(ctx, models.ResumeVersion{VersionLabel: "v1", Content: "x"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("create without profile = %v, want ErrConflict", err)
	}
	withProfile(t, s)
	a, err := s.CreateResume(ctx, models.ResumeVersion{VersionLabel: "v1", Content: "one"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.CreateResume(ctx, models.ResumeVersion{VersionLabel: "v2", TargetPosition: ptr("Analyst"), Content: "two"})
	list, _ := s.ListResumes(ctx)
	if len(list) != 2 || list[0].ID != b.ID || list[0].Content != "" {
		t.Errorf("list = %+v, want newest first without content", list)
	}
	got, err := s.UpdateResume(ctx, b.ID, ResumePatch{VersionLabel: ptr("renamed")})
	if err != nil || got.VersionLabel != "renamed" || got.Content != "two" || got.TargetPosition == nil {
		t.Errorf("rename = %+v, %v (other fields must stay)", got, err)
	}
	got, _ = s.UpdateResume(ctx, b.ID, ResumePatch{SetTarget: true})
	if got.TargetPosition != nil {
		t.Error("SetTarget with nil should clear target_position")
	}
	if _, err := s.UpdateResume(ctx, 99, ResumePatch{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing = %v", err)
	}
	if err := s.DeleteResume(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetResume(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get deleted = %v", err)
	}
}
