package api

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
)

func dirSize(path string) int64 {
	var n int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

func fileCount(path string) int {
	n := 0
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func (s *Server) dbBytes() int64 {
	var n int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if st, err := os.Stat(filepath.Join(s.cfg.DataDir, DBFile+suffix)); err == nil {
			n += st.Size()
		}
	}
	return n
}

// DBFile is the database file name inside the data directory.
const DBFile = "fistbump.db"

func (s *Server) cacheDir() string { return filepath.Join(s.cfg.DataDir, "cache") }

func (s *Server) installedModelBytes(onlyUnused bool) (int64, []string) {
	selected := s.st.SettingString("ai.selected_model")
	var n int64
	var paths []string
	for _, m := range ai.Catalog() {
		if onlyUnused && m.ID == selected {
			continue
		}
		p := ai.ModelPath(s.cfg.DataDir, m)
		if st, err := os.Stat(p); err == nil {
			n += st.Size()
			paths = append(paths, p)
		}
	}
	return n, paths
}

func (s *Server) getStorage(w http.ResponseWriter, r *http.Request) error {
	models, _ := s.installedModelBytes(false)
	cache, partial := dirSize(s.cacheDir()), ai.PartialBytes(s.cfg.DataDir)
	jobs, err := s.st.CountJobs()
	if err != nil {
		return err
	}
	db := s.dbBytes()
	writeJSON(w, 200, map[string]any{
		"data_dir": s.cfg.DataDir, "db_bytes": db, "models_bytes": models, "cache_bytes": cache,
		"partial_downloads_bytes": partial, "job_count": jobs, "total_bytes": db + models + cache + partial,
	})
	return nil
}

func (s *Server) cleanupStorage(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Targets   []string `json:"targets"`
		OlderThan *int     `json:"older_than_days"`
		DryRun    bool     `json:"dry_run"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	days := s.st.SettingInt("jobs.retention_days")
	if in.OlderThan != nil {
		if *in.OlderThan < 1 {
			return invalid("older_than_days", "older_than_days must be at least 1")
		}
		days = *in.OlderThan
	}
	for _, t := range in.Targets {
		if !slices.Contains([]string{"cache", "partial_downloads", "old_jobs", "unused_models"}, t) {
			return invalid("targets", "unknown target %q", t)
		}
	}
	var freed int64
	removedJobs, removedFiles := 0, 0
	for _, t := range slices.Compact(slices.Sorted(slices.Values(in.Targets))) {
		switch t {
		case "cache":
			freed += dirSize(s.cacheDir())
			removedFiles += fileCount(s.cacheDir())
			if !in.DryRun {
				_ = os.RemoveAll(s.cacheDir())
				s.gh.ClearCache()
			}
		case "partial_downloads":
			parts, _ := filepath.Glob(filepath.Join(ai.ModelsDir(s.cfg.DataDir), "*.part"))
			for _, p := range parts {
				if st, err := os.Stat(p); err == nil {
					freed += st.Size()
					removedFiles++
					if !in.DryRun {
						_ = os.Remove(p)
					}
				}
			}
		case "unused_models":
			n, paths := s.installedModelBytes(true)
			freed += n
			removedFiles += len(paths)
			if !in.DryRun {
				for _, p := range paths {
					_ = os.Remove(p)
				}
			}
		case "old_jobs":
			if in.DryRun {
				n, err := s.st.OldJobCount(days)
				if err != nil {
					return err
				}
				removedJobs = n
				continue
			}
			before := s.dbBytes()
			n, err := s.st.CollectOldJobs(days)
			if err != nil {
				return err
			}
			removedJobs = n
			if n > 0 {
				_ = s.st.Vacuum()
				if after := s.dbBytes(); before > after {
					freed += before - after
				}
			}
		}
	}
	writeJSON(w, 200, map[string]any{"freed_bytes": freed, "removed": map[string]any{"old_jobs": removedJobs, "files": removedFiles}, "dry_run": in.DryRun})
	return nil
}
