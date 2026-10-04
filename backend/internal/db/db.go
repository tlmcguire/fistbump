// Package db opens the SQLite database and applies embedded migrations.
package db

import (
	"database/sql"
	"embed"
	"fmt"
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
	return conn, nil
}

// dsn enables foreign keys on every pooled connection, which SQLite leaves off by default.
func dsn(file string) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
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
