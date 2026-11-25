package store

import (
	"context"
	"testing"
)

func TestSavedSearchCRUD(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	created, err := st.CreateSavedSearch(ctx, "Go Remote", `{"keywords":["golang"],"work_mode":"Remote"}`, []string{"stripe/1", "stripe/2"})
	if err != nil {
		t.Fatalf("CreateSavedSearch failed: %v", err)
	}
	if created.Name != "Go Remote" || len(created.SeenKeys) != 2 {
		t.Fatalf("unexpected created saved search: %+v", created)
	}

	list, err := st.ListSavedSearches(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListSavedSearches: len=%d err=%v", len(list), err)
	}

	updated, err := st.UpdateSavedSearchRun(ctx, created.ID, []string{"stripe/1", "stripe/2", "stripe/3"})
	if err != nil || len(updated.SeenKeys) != 3 {
		t.Fatalf("UpdateSavedSearchRun failed: %+v err=%v", updated, err)
	}

	if err := st.DeleteSavedSearch(ctx, created.ID); err != nil {
		t.Fatalf("DeleteSavedSearch failed: %v", err)
	}
	list, err = st.ListSavedSearches(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("expected empty list after delete, got %d err=%v", len(list), err)
	}
}
