package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) (*sql.DB, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(file)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn, file
}

func TestOpenAppliesMigrations(t *testing.T) {
	conn, _ := openTemp(t)

	var version int
	if err := conn.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	ms, err := listMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if want := ms[len(ms)-1].version; version != want {
		t.Errorf("user_version = %d, want %d", version, want)
	}

	for _, table := range []string{"profile", "experiences", "education", "resumes", "jobs",
		"revisions", "suggestions", "tailored_resumes", "applications", "settings"} {
		var n int
		err := conn.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&n)
		if err != nil || n != 1 {
			t.Errorf("table %s missing (n=%d, err=%v)", table, n, err)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	conn, file := openTemp(t)
	if _, err := conn.Exec("UPDATE settings SET value='local' WHERE key='ai.mode'"); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	again, err := Open(file)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer again.Close()
	var mode string
	if err := again.QueryRow("SELECT value FROM settings WHERE key='ai.mode'").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "local" {
		t.Errorf("ai.mode = %q after reopen, want local (migration must not rerun)", mode)
	}
}

func TestSettingsDefaults(t *testing.T) {
	conn, _ := openTemp(t)
	want := map[string]string{
		"ai.mode":                       "auto",
		"ai.selected_model":             "",
		"ai.idle_minutes":               "5",
		"jobs.retention_days":           "30",
		"jobs.trash_days":               "7",
		"connectors.greenhouse.enabled": "true",
		"connectors.greenhouse.boards":  "[]",
	}
	for k, v := range want {
		var got string
		if err := conn.QueryRow("SELECT value FROM settings WHERE key=?", k).Scan(&got); err != nil {
			t.Errorf("%s: %v", k, err)
			continue
		}
		if got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	conn, _ := openTemp(t)
	_, err := conn.Exec("INSERT INTO experiences (profile_id, company_name, job_title) VALUES (999, 'x', 'y')")
	if err == nil {
		t.Error("insert with missing profile succeeded, foreign keys are off")
	}
}

func TestChecksEnforced(t *testing.T) {
	conn, _ := openTemp(t)
	if _, err := conn.Exec("INSERT INTO jobs (company_name, position_title, source) VALUES ('a', 'b', 'linkedin')"); err == nil {
		t.Error("invalid jobs.source accepted")
	}
	if _, err := conn.Exec("INSERT INTO jobs (company_name, position_title, req_tech_skills) VALUES ('a', 'b', 'not json')"); err == nil {
		t.Error("invalid JSON accepted in req_tech_skills")
	}
}

func TestJobExternalIDUnique(t *testing.T) {
	conn, _ := openTemp(t)
	ins := "INSERT INTO jobs (company_name, position_title, source, external_id) VALUES ('a', 'b', 'greenhouse', ?)"
	if _, err := conn.Exec(ins, "123"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ins, "123"); err == nil {
		t.Error("duplicate (source, external_id) accepted")
	}
	// Pasted jobs have no external id and must not collide.
	paste := "INSERT INTO jobs (company_name, position_title) VALUES ('a', 'b')"
	for i := 0; i < 2; i++ {
		if _, err := conn.Exec(paste); err != nil {
			t.Fatalf("pasted job %d: %v", i, err)
		}
	}
}

func TestApplicationRestrictsJobDelete(t *testing.T) {
	conn, _ := openTemp(t)
	mustExec(t, conn, "INSERT INTO profile (full_name) VALUES ('p')")
	mustExec(t, conn, "INSERT INTO jobs (company_name, position_title) VALUES ('a', 'b')")
	mustExec(t, conn, "INSERT INTO applications (job_id, profile_id) VALUES (1, 1)")

	if _, err := conn.Exec("DELETE FROM jobs WHERE id=1"); err == nil {
		t.Error("deleted a job that has an application")
	}
	mustExec(t, conn, "DELETE FROM applications WHERE id=1")
	mustExec(t, conn, "DELETE FROM jobs WHERE id=1")
}

func TestOneApplicationPerJob(t *testing.T) {
	conn, _ := openTemp(t)
	mustExec(t, conn, "INSERT INTO profile (full_name) VALUES ('p')")
	mustExec(t, conn, "INSERT INTO jobs (company_name, position_title) VALUES ('a', 'b')")
	mustExec(t, conn, "INSERT INTO applications (job_id, profile_id) VALUES (1, 1)")
	if _, err := conn.Exec("INSERT INTO applications (job_id, profile_id) VALUES (1, 1)"); err == nil {
		t.Error("second application for the same job accepted")
	}
}

func TestApplicationStatuses(t *testing.T) {
	conn, _ := openTemp(t)
	mustExec(t, conn, "INSERT INTO profile (full_name) VALUES ('p')")
	mustExec(t, conn, "INSERT INTO jobs (company_name, position_title) VALUES ('a', 'b')")
	mustExec(t, conn, "INSERT INTO applications (job_id, profile_id, status) VALUES (1, 1, 'Rejected')")
	// Archiving is archived_at, not a status.
	if _, err := conn.Exec("UPDATE applications SET status = 'Archived' WHERE id = 1"); err == nil {
		t.Error("status 'Archived' accepted")
	}
}

func TestJobDeleteCascades(t *testing.T) {
	conn, _ := openTemp(t)
	mustExec(t, conn, "INSERT INTO jobs (company_name, position_title) VALUES ('a', 'b')")
	mustExec(t, conn, "INSERT INTO revisions (job_id, engine) VALUES (1, 'rules')")
	mustExec(t, conn, "INSERT INTO suggestions (revision_id, section, original_text, proposed_text) VALUES (1, 'summary', 'o', 'p')")
	mustExec(t, conn, "DELETE FROM jobs WHERE id=1")

	var n int
	if err := conn.QueryRow("SELECT count(*) FROM suggestions").Scan(&n); err != nil || n != 0 {
		t.Errorf("suggestions remaining = %d, err = %v", n, err)
	}
}

func mustExec(t *testing.T, conn *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := conn.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func TestListMigrationsOrdered(t *testing.T) {
	ms, err := listMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 || ms[0].version != 1 {
		t.Fatalf("migrations = %+v, want first version 1", ms)
	}
	for i := 1; i < len(ms); i++ {
		if ms[i].version <= ms[i-1].version {
			t.Errorf("migrations out of order: %+v", ms)
		}
	}
}
