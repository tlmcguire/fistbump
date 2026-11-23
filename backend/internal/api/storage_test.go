package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
	"github.com/tlmcguire/fistbump/backend/internal/models"
)

// stampLayout is the timestamp format the schema uses.
const stampLayout = "2006-01-02T15:04:05Z"

// cleanupResult is the answer of POST /v1/storage/cleanup.
type cleanupResult struct {
	FreedBytes int64          `json:"freed_bytes"`
	Removed    map[string]int `json:"removed"`
	DryRun     bool           `json:"dry_run"`
}

// setSetting writes one setting.
func setSetting(t *testing.T, s *Server, key string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal the setting: %v", err)
	}
	if err := s.store.PutSettings(context.Background(), map[string]json.RawMessage{key: raw}); err != nil {
		t.Fatalf("PutSettings: %v", err)
	}
}

// ageJob sets the searched_at stamp of a job, which is what the cleanup looks at.
func ageJob(t *testing.T, s *Server, id int64, stamp string) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(),
		"UPDATE jobs SET searched_at = ? WHERE id = ?", stamp, id); err != nil {
		t.Fatalf("age the job: %v", err)
	}
}

// trashJob puts a job in the Trash.
func trashJob(t *testing.T, s *Server, id int64, stamp string) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(),
		"UPDATE jobs SET trashed_at = ? WHERE id = ?", stamp, id); err != nil {
		t.Fatalf("trash the job: %v", err)
	}
}

// The report says how much space the app uses and where it lives.
func TestStorageReportSizes(t *testing.T) {
	s, conn, _, dir := storageServer(t)
	if _, err := conn.Exec(`INSERT INTO greenhouse_description_cache (board, job_id, posting, fetched_at)
  VALUES ('acme', '1', json_object('raw_text', ?), '2026-10-24T00:00:00Z')`, strings.Repeat("x", 5000)); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "models"), 0o700)
	os.WriteFile(filepath.Join(dir, "models", "a.gguf"), make([]byte, 300), 0o600)
	os.WriteFile(filepath.Join(dir, "models", "b.gguf.part"), make([]byte, 100), 0o600)
	if _, err := s.store.CreateJob(context.Background(), models.Job{CompanyName: "a", PositionTitle: "b"}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	var report storageReport
	rec := call(t, s, "GET", "/v1/storage", "")
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &report) != nil {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if report.DataDir != dir {
		t.Errorf("data_dir = %q, want %q", report.DataDir, dir)
	}
	if report.DBBytes == 0 {
		t.Error("db_bytes = 0, want the size of the database file")
	}
	if report.ModelsBytes != 400 {
		t.Errorf("models_bytes = %d, want 400", report.ModelsBytes)
	}
	if report.PartialDownloadsBytes != 100 {
		t.Errorf("partial_downloads_bytes = %d, want 100", report.PartialDownloadsBytes)
	}
	if report.CacheBytes < 5000 {
		t.Errorf("cache_bytes = %d, want the size of the cache tables", report.CacheBytes)
	}
	if report.JobCount != 1 {
		t.Errorf("job_count = %d, want 1", report.JobCount)
	}
	if report.TotalBytes != report.DBBytes+report.ModelsBytes {
		t.Errorf("total_bytes = %d, want %d", report.TotalBytes, report.DBBytes+report.ModelsBytes)
	}
}

// The cache is emptied, and the bytes it held are reported as freed.
func TestStorageCleanupCache(t *testing.T) {
	s, conn, cache, _ := storageServer(t)
	if _, err := conn.Exec(`INSERT INTO greenhouse_description_cache (board, job_id, posting, fetched_at)
  VALUES ('acme', '1', json_object('raw_text', ?), '2026-10-24T00:00:00Z')`, strings.Repeat("x", 5000)); err != nil {
		t.Fatal(err)
	}
	var report storageReport
	if rec := call(t, s, "GET", "/v1/storage", ""); rec.Code != http.StatusOK ||
		json.Unmarshal(rec.Body.Bytes(), &report) != nil {
		t.Fatalf("the storage report failed: status %d, body %q", rec.Code, rec.Body.String())
	}

	rec := call(t, s, "POST", "/v1/storage/cleanup", `{"targets":["cache"]}`)
	var result cleanupResult
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if cache.cleared != 1 {
		t.Errorf("the cache was cleared %d times, want 1", cache.cleared)
	}
	if result.FreedBytes != report.CacheBytes {
		t.Errorf("freed_bytes = %d, want %d", result.FreedBytes, report.CacheBytes)
	}
}

// A .part file with no download running is removed. A finished model stays.
func TestStorageCleanupPartialDownloads(t *testing.T) {
	s, _, _, dir := storageServer(t)
	os.MkdirAll(filepath.Join(dir, "models"), 0o700)
	os.WriteFile(filepath.Join(dir, "models", "a.gguf.part"), make([]byte, 120), 0o600)
	os.WriteFile(filepath.Join(dir, "models", "a.gguf"), make([]byte, 300), 0o600)

	rec := call(t, s, "POST", "/v1/storage/cleanup", `{"targets":["partial_downloads"]}`)
	var result cleanupResult
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if result.Removed["files"] != 1 {
		t.Errorf("removed files = %d, want 1", result.Removed["files"])
	}
	if result.FreedBytes != 120 {
		t.Errorf("freed_bytes = %d, want 120", result.FreedBytes)
	}
	if _, err := os.Stat(filepath.Join(dir, "models", "a.gguf.part")); !os.IsNotExist(err) {
		t.Error("the partial download is still there")
	}
	if _, err := os.Stat(filepath.Join(dir, "models", "a.gguf")); err != nil {
		t.Error("cleanup removed a finished model")
	}
}

// Old jobs are removed. The setting decides the age, and the request can override it.
func TestStorageCleanupOldJobs(t *testing.T) {
	s, _, _, _ := storageServer(t)
	ctx := context.Background()
	stale, _ := s.store.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "stale"})
	fresh, _ := s.store.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "fresh"})
	old := time.Now().UTC().AddDate(0, 0, -40).Format(stampLayout)
	ageJob(t, s, stale.ID, old)
	setSetting(t, s, "jobs.retention_days", 365)

	// With the setting at a year the job is not old enough yet.
	rec := call(t, s, "POST", "/v1/storage/cleanup", `{"targets":["old_jobs"],"dry_run":true}`)
	var result cleanupResult
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if result.Removed["old_jobs"] != 0 || result.FreedBytes != 0 {
		t.Errorf("dry run = %+v, want nothing removed", result)
	}

	// The request asks for a shorter age, so the job goes.
	rec = call(t, s, "POST", "/v1/storage/cleanup", `{"targets":["old_jobs"],"older_than_days":30}`)
	result = cleanupResult{}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if result.Removed["old_jobs"] != 1 {
		t.Errorf("removed old_jobs = %d, want 1", result.Removed["old_jobs"])
	}
	if result.FreedBytes != 0 {
		t.Errorf("freed_bytes = %d, want 0: the file only shrinks after the vacuum", result.FreedBytes)
	}
	if _, err := s.store.GetJob(ctx, stale.ID); err == nil {
		t.Error("the old job is still there")
	}
	if _, err := s.store.GetJob(ctx, fresh.ID); err != nil {
		t.Errorf("the recent job = %v, want it kept", err)
	}
}

// A trashed job is removed once it has been in the Trash for longer than the trash days.
func TestStorageCleanupTrash(t *testing.T) {
	s, _, _, _ := storageServer(t)
	ctx := context.Background()
	inTrash, _ := s.store.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "old"})
	kept, _ := s.store.CreateJob(ctx, models.Job{CompanyName: "a", PositionTitle: "recent"})
	trashJob(t, s, inTrash.ID, time.Now().UTC().AddDate(0, 0, -10).Format(stampLayout))
	trashJob(t, s, kept.ID, time.Now().UTC().AddDate(0, 0, -3).Format(stampLayout))
	setSetting(t, s, "jobs.trash_days", 7)

	rec := call(t, s, "POST", "/v1/storage/cleanup", `{"targets":["trash"]}`)
	var result cleanupResult
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if result.Removed["trash"] != 1 {
		t.Errorf("removed trash = %d, want 1", result.Removed["trash"])
	}
	if _, err := s.store.GetJob(ctx, inTrash.ID); err == nil {
		t.Error("the trashed job is still there")
	}
	if _, err := s.store.GetJob(ctx, kept.ID); err != nil {
		t.Errorf("the recent trashed job = %v, want it kept", err)
	}
}

// A model other than the selected one is removed. The selected model stays.
func TestStorageCleanupUnusedModels(t *testing.T) {
	s, _, _, dir := storageServer(t)
	catalog := ai.Catalog()
	if len(catalog) < 2 {
		t.Fatal("the catalog needs two models for this test")
	}
	selected, other := catalog[0], catalog[1]
	os.MkdirAll(ai.ModelsDir(dir), 0o700)
	os.WriteFile(selected.Path(dir), make([]byte, 100), 0o600)
	os.WriteFile(other.Path(dir), make([]byte, 250), 0o600)
	setSetting(t, s, "ai.selected_model", selected.ID)

	// A dry run reports the file and keeps it.
	rec := call(t, s, "POST", "/v1/storage/cleanup", `{"targets":["unused_models"],"dry_run":true}`)
	var result cleanupResult
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if result.Removed["files"] != 1 || result.FreedBytes != 250 {
		t.Errorf("dry run = %+v, want one file of 250 bytes", result)
	}
	if _, err := os.Stat(other.Path(dir)); err != nil {
		t.Error("a dry run removed a model")
	}

	rec = call(t, s, "POST", "/v1/storage/cleanup", `{"targets":["unused_models"]}`)
	result = cleanupResult{}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if result.Removed["files"] != 1 || result.FreedBytes != 250 {
		t.Errorf("cleanup = %+v, want one file of 250 bytes", result)
	}
	if _, err := os.Stat(other.Path(dir)); !os.IsNotExist(err) {
		t.Error("the unused model is still there")
	}
	if _, err := os.Stat(selected.Path(dir)); err != nil {
		t.Error("cleanup removed the selected model")
	}
}

// Every target is built, so only an unknown one is a validation error.
func TestStorageCleanupRejectsBadRequests(t *testing.T) {
	s, _, cache, _ := storageServer(t)
	for name, body := range map[string]string{
		"no targets":     `{"targets":[]}`,
		"unknown target": `{"targets":["everything"]}`,
		"zero days":      `{"targets":["cache"],"older_than_days":0}`,
		"negative days":  `{"targets":["cache"],"older_than_days":-1}`,
		"unknown field":  `{"targets":["cache"],"force":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			wantError(t, call(t, s, "POST", "/v1/storage/cleanup", body),
				http.StatusUnprocessableEntity, "validation_failed")
		})
	}
	if cache.cleared != 0 {
		t.Error("a rejected request cleared the cache")
	}
}
