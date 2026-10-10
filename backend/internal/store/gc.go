package store

import "time"

// CountJobs returns the number of stored jobs.
func (s *Store) CountJobs() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n)
	return n, err
}

func cutoff(days int) string {
	return time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02T15:04:05Z")
}

// OldJobCount counts jobs GC would remove: stale searched_at and no application.
func (s *Store) OldJobCount(days int) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE searched_at < ? AND id NOT IN (SELECT job_id FROM applications)`, cutoff(days)).Scan(&n)
	return n, err
}

// CollectOldJobs deletes stale jobs that have no application and returns how many were removed.
func (s *Store) CollectOldJobs(days int) (int, error) {
	res, err := s.DB.Exec(`DELETE FROM jobs WHERE searched_at < ? AND id NOT IN (SELECT job_id FROM applications)`, cutoff(days))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Vacuum compacts the file. It cannot run inside a transaction, so it always uses the root handle.
func (s *Store) Vacuum() error {
	if s.root == nil {
		return nil
	}
	_, err := s.root.Exec(`VACUUM`)
	return err
}
