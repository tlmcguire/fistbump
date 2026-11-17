package store

import (
	"context"
	"errors"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

// doneRevision returns a finished revision holding two pending suggestions.
func doneRevision(t *testing.T, s *Store) (models.Revision, []models.Suggestion) {
	t.Helper()
	ctx := context.Background()
	rev := newRevision(t, s)
	s.StartRevision(ctx, rev.ID)
	id := int64(7)
	list := []models.Suggestion{
		{Section: "skills", Engine: "rules", OriginalText: "a, b", ProposedText: "b, a"},
		{Section: "experience", TargetID: &id, Engine: "rules", OriginalText: "x\ny", ProposedText: "y\nx"},
	}
	if err := s.CompleteRevision(ctx, rev.ID, "rules", list); err != nil {
		t.Fatal(err)
	}
	stored, err := s.ListSuggestions(ctx, rev.ID)
	if err != nil || len(stored) != 2 {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	return rev, stored
}

func TestSuggestionsKeepTheirFields(t *testing.T) {
	s := newStore(t)
	_, got := doneRevision(t, s)
	if got[0].Section != "skills" || got[0].TargetID != nil || got[0].State != "pending" {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].TargetID == nil || *got[1].TargetID != 7 || got[1].ProposedText != "y\nx" {
		t.Errorf("second = %+v", got[1])
	}
}

func TestDecideSuggestion(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	rev, list := doneRevision(t, s)

	accepted, err := s.DecideSuggestion(ctx, rev.ID, list[0].ID, "accepted", nil)
	if err != nil || accepted.State != "accepted" || accepted.Final() != "b, a" {
		t.Errorf("accepted = %+v, %v", accepted, err)
	}
	edit := "mine"
	edited, err := s.DecideSuggestion(ctx, rev.ID, list[1].ID, "edited", &edit)
	if err != nil || edited.State != "edited" || edited.Final() != "mine" {
		t.Errorf("edited = %+v, %v", edited, err)
	}
	// Changing a mind clears the earlier edit.
	rejected, _ := s.DecideSuggestion(ctx, rev.ID, list[1].ID, "rejected", nil)
	if rejected.EditedText != nil || rejected.Final() != "x\ny" {
		t.Errorf("rejected = %+v", rejected)
	}
}

func TestDecideSuggestionMustBelongToRevision(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	rev, list := doneRevision(t, s)
	if _, err := s.DecideSuggestion(ctx, rev.ID+1, list[0].ID, "accepted", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("wrong revision = %v, want ErrNotFound", err)
	}
	if _, err := s.DecideSuggestion(ctx, rev.ID, 999, "accepted", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown suggestion = %v, want ErrNotFound", err)
	}
	if _, err := s.DecideSuggestion(ctx, rev.ID, list[0].ID, "maybe", nil); !errors.Is(err, ErrConflict) {
		t.Errorf("unknown state = %v, want ErrConflict from the CHECK constraint", err)
	}
}

func TestListSuggestionsEmptyIsNotNil(t *testing.T) {
	s := newStore(t)
	rev := newRevision(t, s)
	if got, err := s.ListSuggestions(context.Background(), rev.ID); err != nil || got == nil || len(got) != 0 {
		t.Errorf("got %v, %v", got, err)
	}
}
