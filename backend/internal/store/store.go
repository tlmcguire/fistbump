// Package store reads and writes the SQLite tables. The api package calls it and never sees a
// driver error: constraint failures and missing rows are mapped to ErrConflict and ErrNotFound.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrNotFound means no row matched.
	ErrNotFound = errors.New("not found")
	// ErrConflict means a unique, foreign key or check constraint refused the write.
	ErrConflict = errors.New("conflict")
)

// DBTX is what *sql.DB and *sql.Tx share, so every method works inside or outside a transaction.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store is the data access layer.
type Store struct {
	db   DBTX
	root *sql.DB
}

// New returns a Store on db.
func New(db *sql.DB) *Store {
	return &Store{db: db, root: db}
}

// WithTx runs fn on a Store bound to one transaction and commits if fn returns nil, else rolls back.
// Calls nested inside fn reuse the same transaction.
func (s *Store) WithTx(ctx context.Context, fn func(*Store) error) error {
	if _, nested := s.db.(*sql.Tx); nested {
		return fn(s)
	}
	tx, err := s.root.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(&Store{db: tx, root: s.root}); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return mapErr(err)
	}
	return nil
}

// mapErr turns driver errors into ErrNotFound and ErrConflict. Other errors pass through.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	// modernc.org/sqlite reports constraint failures as "constraint failed: ... (1555)" style
	// text; matching the message avoids importing the driver's error type here.
	msg := err.Error()
	if strings.Contains(msg, "constraint failed") || strings.Contains(msg, "CHECK constraint") ||
		strings.Contains(msg, "UNIQUE constraint") || strings.Contains(msg, "FOREIGN KEY constraint") {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}

// now is the current time as RFC 3339 UTC, the format the schema uses.
func now() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

// encodeList stores a string list as a JSON array; nil becomes "[]".
func encodeList(list []string) string {
	if list == nil {
		list = []string{}
	}
	b, _ := json.Marshal(list)
	return string(b)
}

// decodeList reads a JSON array column; it never returns nil.
func decodeList(s string) []string {
	var list []string
	if err := json.Unmarshal([]byte(s), &list); err != nil || list == nil {
		return []string{}
	}
	return list
}

// cleanList trims entries, drops blanks and removes case-insensitive duplicates, keeping the
// first spelling.
func cleanList(list []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, item := range list {
		item = strings.TrimSpace(item)
		key := strings.ToLower(item)
		if item == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}
