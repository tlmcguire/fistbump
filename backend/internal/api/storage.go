package api

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// CacheClearer is a cache the storage route can empty: the Greenhouse cache and the logo cache.
type CacheClearer interface {
	Clear() error
}

// Cleanup targets. Only the ones for features that exist are built; the others belong to later phases.
var (
	builtTargets   = map[string]bool{"cache": true, "partial_downloads": true}
	plannedTargets = map[string]bool{"old_jobs": true, "trash": true, "unused_models": true}
)

// Database files: the database and SQLite's write-ahead log and shared-memory files beside it.
var dbFiles = []string{"fistbump.db", "fistbump.db-wal", "fistbump.db-shm"}

type storageReport struct {
	DataDir               string `json:"data_dir"`
	DBBytes               int64  `json:"db_bytes"`
	ModelsBytes           int64  `json:"models_bytes"`
	CacheBytes            int64  `json:"cache_bytes"`
	PartialDownloadsBytes int64  `json:"partial_downloads_bytes"`
	JobCount              int    `json:"job_count"`
	TotalBytes            int64  `json:"total_bytes"`
}

// storage handles GET /v1/storage: how much space the app uses and where.
func (s *Server) storage(w http.ResponseWriter, r *http.Request) error {
	report, err := s.measure(r)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, report)
	return nil
}

func (s *Server) measure(r *http.Request) (storageReport, error) {
	report := storageReport{DataDir: s.dataDir}
	for _, name := range dbFiles {
		report.DBBytes += fileSize(filepath.Join(s.dataDir, name))
	}
	models := filepath.Join(s.dataDir, "models")
	report.ModelsBytes = dirSize(models, func(string) bool { return true })
	report.PartialDownloadsBytes = dirSize(models, isPartial)

	// The caches live in the database, so their size is the size of what they hold.
	err := s.db.QueryRowContext(r.Context(), `SELECT
		(SELECT COALESCE(SUM(LENGTH(jobs) + LENGTH(board) + LENGTH(etag)), 0) FROM greenhouse_listing_cache) +
		(SELECT COALESCE(SUM(LENGTH(posting) + LENGTH(board) + LENGTH(job_id)), 0) FROM greenhouse_description_cache) +
		(SELECT COALESCE(SUM(COALESCE(LENGTH(data), 0) + LENGTH(host)), 0) FROM logo_cache)`).Scan(&report.CacheBytes)
	if err != nil {
		return report, err
	}
	if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM jobs`).Scan(&report.JobCount); err != nil {
		return report, err
	}
	report.TotalBytes = report.DBBytes + report.ModelsBytes
	return report, nil
}

// storageCleanup handles POST /v1/storage/cleanup. With dry_run it only reports what would be freed.
func (s *Server) storageCleanup(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var body struct {
		Targets       []string `json:"targets"`
		OlderThanDays *int     `json:"older_than_days"`
		DryRun        bool     `json:"dry_run"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil && err != io.EOF {
		return bodyError(err)
	}
	if len(body.Targets) == 0 {
		return validationFailed("Choose what to clean up")
	}
	if body.OlderThanDays != nil && *body.OlderThanDays < 1 {
		return validationFailed("Older than days must be at least 1")
	}
	for _, t := range body.Targets {
		switch {
		case builtTargets[t]:
		case plannedTargets[t]:
			return validationFailed("Cleaning up " + strings.ReplaceAll(t, "_", " ") + " is not available yet")
		default:
			return validationFailed("Unknown cleanup target")
		}
	}

	before, err := s.measure(r)
	if err != nil {
		return err
	}
	result := struct {
		FreedBytes int64          `json:"freed_bytes"`
		Removed    map[string]int `json:"removed"`
		DryRun     bool           `json:"dry_run"`
	}{Removed: map[string]int{}, DryRun: body.DryRun}

	for _, t := range body.Targets {
		switch t {
		case "cache":
			result.FreedBytes += before.CacheBytes
			if body.DryRun {
				continue
			}
			for _, c := range s.caches {
				if err := c.Clear(); err != nil {
					return err
				}
			}
		case "partial_downloads":
			files := partialFiles(filepath.Join(s.dataDir, "models"))
			result.Removed["files"] += len(files)
			result.FreedBytes += before.PartialDownloadsBytes
			if body.DryRun {
				continue
			}
			for _, f := range files {
				os.Remove(f)
			}
		}
	}
	if !body.DryRun {
		// Emptied tables leave free pages in the file; VACUUM gives them back to the disk.
		if _, err := s.db.ExecContext(r.Context(), `VACUUM`); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0
	}
	return info.Size()
}

func isPartial(path string) bool { return strings.HasSuffix(path, ".part") }

// dirSize adds up the files under dir that match keep. A missing dir is empty.
func dirSize(dir string, keep func(string) bool) int64 {
	var total int64
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !keep(path) {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func partialFiles(dir string) []string {
	var out []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && isPartial(path) {
			out = append(out, path)
		}
		return nil
	})
	return out
}
