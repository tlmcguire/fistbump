// Package db opens the SQLite database and applies embedded migrations.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open opens (creating if needed) the database at file and migrates it to the latest version.
func Open(file string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", dsn(file))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}
	if err := ensureCaches(conn); err != nil {
		conn.Close()
		return nil, err
	}
	if err := ensureColumns(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// columnMigrations each add one column with ALTER TABLE, which cannot say IF NOT EXISTS. The column is
// checked on every open and the migration run when it is missing, for the same reason as cacheMigrations:
// a database whose user_version another build moved past them never ran them, and every write of that
// column then fails ("table suggestions has no column named reason").
var columnMigrations = []struct{ table, column, file string }{
	{"suggestions", "reason", "004_suggestion_reason.sql"},
	{"experiences", "date_na", "005_experience_date_na.sql"},
}

func ensureColumns(conn *sql.DB) error {
	for _, c := range columnMigrations {
		has, err := hasColumn(conn, c.table, c.column)
		if err != nil {
			return fmt.Errorf("check %s.%s: %w", c.table, c.column, err)
		}
		if has {
			continue
		}
		body, err := migrationsFS.ReadFile(path.Join("migrations", c.file))
		if err != nil {
			return fmt.Errorf("read %s: %w", c.file, err)
		}
		slog.Warn("adding a column an earlier migration should have added", "table", c.table, "column", c.column)
		if _, err := conn.Exec(string(body)); err != nil {
			return fmt.Errorf("ensure %s: %w", c.file, err)
		}
	}
	return nil
}

func hasColumn(conn *sql.DB, table, column string) (bool, error) {
	// PRAGMA does not accept bound parameters. table comes from columnMigrations above, never from input.
	rows, err := conn.Query("SELECT name FROM pragma_table_info('" + table + "')")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// cacheMigrations only create tables with IF NOT EXISTS, so they are run on every open as well as in
// order. That repairs a database whose user_version another build moved past them (an experimental build
// that shared the data folder did), which would otherwise never get its tables.
var cacheMigrations = []string{"002_greenhouse_cache.sql", "003_logo_cache.sql", "006_saved_searches.sql", "007_application_drafts.sql"}

func ensureCaches(conn *sql.DB) error {
	for _, name := range cacheMigrations {
		body, err := migrationsFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := conn.Exec(string(body)); err != nil {
			return fmt.Errorf("ensure %s: %w", name, err)
		}
	}
	return nil
}

// dsn enables foreign keys on every pooled connection, which SQLite leaves off by default.
func dsn(file string) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "temp_store(MEMORY)")
	q.Add("_pragma", "cache_size(-16000)")
	return "file:" + file + "?" + q.Encode()
}

type migration struct {
	version int
	name    string
}

// migrate applies each migration newer than PRAGMA user_version, one transaction per file.
func migrate(conn *sql.DB) error {
	ms, err := listMigrations()
	if err != nil {
		return err
	}
	var current int
	if err := conn.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if latest := ms[len(ms)-1].version; current > latest {
		slog.Warn("database schema is newer than this build; it was written by another build",
			"schema_version", current, "latest_known", latest)
	}
	for _, m := range ms {
		if m.version <= current {
			continue
		}
		body, err := migrationsFS.ReadFile(path.Join("migrations", m.name))
		if err != nil {
			return fmt.Errorf("read %s: %w", m.name, err)
		}
		if err := apply(conn, m.version, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", m.name, err)
		}
	}
	return nil
}

func apply(conn *sql.DB, version int, body string) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(body); err != nil {
		return err
	}
	// PRAGMA does not accept bound parameters. version comes from an embedded filename parsed as int.
	if _, err := tx.Exec("PRAGMA user_version = " + strconv.Itoa(version)); err != nil {
		return err
	}
	return tx.Commit()
}

// listMigrations returns embedded files named NNN_description.sql in version order.
func listMigrations() ([]migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	var ms []migration
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(name, "_")
		v, err := strconv.Atoi(prefix)
		if !ok || err != nil || v < 1 {
			return nil, fmt.Errorf("migration %q: name must be NNN_description.sql", name)
		}
		ms = append(ms, migration{version: v, name: name})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	for i := 1; i < len(ms); i++ {
		if ms[i].version == ms[i-1].version {
			return nil, fmt.Errorf("duplicate migration version %d", ms[i].version)
		}
	}
	return ms, nil
}
