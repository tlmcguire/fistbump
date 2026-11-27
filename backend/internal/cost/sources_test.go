package cost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAnalyzeIncludesCitedSourcesAndSchedule(t *testing.T) {
	ResetCatalogForTest()
	res := Analyze(Input{
		Title:    "Software Engineer",
		Location: "Austin, TX",
		PayMin:   130000,
		PayMax:   150000,
		Text:     "We offer health insurance, dental, vision, 401(k) match, and paid time off.",
	})
	if len(res.Sources) < 5 {
		t.Fatalf("expected at least 5 cited sources, got %d", len(res.Sources))
	}
	if res.RefreshDays != 30 {
		t.Errorf("RefreshDays = %d, want 30", res.RefreshDays)
	}
	if res.DataUpdatedAt == "" || res.NextUpdateAt == "" {
		t.Errorf("expected DataUpdatedAt and NextUpdateAt to be populated, got %q / %q", res.DataUpdatedAt, res.NextUpdateAt)
	}
	cats := map[string]bool{}
	for _, s := range res.Sources {
		if s.Name == "" || s.URL == "" {
			t.Errorf("source missing name or URL: %+v", s)
		}
		cats[s.Category] = true
	}
	for _, want := range []string{"cost_of_living", "rent", "pay", "taxes", "benefits"} {
		if !cats[want] {
			t.Errorf("missing source category %q", want)
		}
	}
}

func TestNeedsRefresh30DayWindow(t *testing.T) {
	now := time.Date(2026, 12, 25, 12, 0, 0, 0, time.UTC)
	fresh := ReferenceCatalog{
		UpdatedAt:   "2026-12-10",
		RefreshDays: 30,
		FetchedAt:   now.Add(-29 * 24 * time.Hour).Format(time.RFC3339),
	}
	if NeedsRefresh(fresh, now) {
		t.Errorf("expected 29-day-old catalog not to need refresh")
	}
	stale := fresh
	stale.FetchedAt = now.Add(-31 * 24 * time.Hour).Format(time.RFC3339)
	if !NeedsRefresh(stale, now) {
		t.Errorf("expected 31-day-old catalog to need refresh")
	}
	staleByUpdated := ReferenceCatalog{UpdatedAt: "2026-10-01", RefreshDays: 30}
	if !NeedsRefresh(staleByUpdated, now) {
		t.Errorf("expected catalog with old UpdatedAt and empty FetchedAt to need refresh")
	}
}

func TestRefreshNowUpdatesInMemoryAndPersistsCache(t *testing.T) {
	ResetCatalogForTest()
	defer ResetCatalogForTest()

	payload := ReferenceCatalog{
		UpdatedAt:   "2027-01-01",
		RefreshDays: 30,
		Sources: []Source{
			{Category: "cost_of_living", Name: "BEA RPP 2027", URL: "https://www.bea.gov/data/prices-inflation/regional-price-parities-state-and-metro-area", Description: "Updated index", UpdatedAt: "2027-01-01"},
		},
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	ok := RefreshNow(context.Background(), dir, srv.URL, srv.Client())
	if !ok {
		t.Fatalf("RefreshNow returned false, want true")
	}
	cat, next := CurrentCatalog()
	if cat.UpdatedAt != "2027-01-01" {
		t.Errorf("UpdatedAt = %q, want 2027-01-01", cat.UpdatedAt)
	}
	if next != "2027-01-31" {
		t.Errorf("NextUpdateAt = %q, want 2027-01-31", next)
	}
	if _, err := os.Stat(filepath.Join(dir, "cost-sources.json")); err != nil {
		t.Fatalf("expected cached cost-sources.json in dataDir: %v", err)
	}

	// Reset in-memory catalog and verify LoadCachedCatalog restores the cached version.
	ResetCatalogForTest()
	LoadCachedCatalog(dir)
	reloaded, _ := CurrentCatalog()
	if reloaded.UpdatedAt != "2027-01-01" {
		t.Errorf("reloaded UpdatedAt = %q, want 2027-01-01", reloaded.UpdatedAt)
	}
}

func TestRefreshNowGracefullyIgnoresErrorsAndInvalidJSON(t *testing.T) {
	ResetCatalogForTest()
	defer ResetCatalogForTest()

	badSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badSrv.Close()

	if RefreshNow(context.Background(), t.TempDir(), badSrv.URL, badSrv.Client()) {
		t.Errorf("expected RefreshNow to return false on 500")
	}

	malformedSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"updated_at": "", "sources": []}`))
	}))
	defer malformedSrv.Close()

	if RefreshNow(context.Background(), t.TempDir(), malformedSrv.URL, malformedSrv.Client()) {
		t.Errorf("expected RefreshNow to return false on empty/malformed catalog")
	}
}
