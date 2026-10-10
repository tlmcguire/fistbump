// Package store holds SQL CRUD for every table.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// DBTX is satisfied by both *sql.DB and *sql.Tx, so store methods run inside or outside a transaction.
type DBTX interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Store wraps the database handle.
type Store struct {
	DB   DBTX
	root *sql.DB
}

func New(db *sql.DB) *Store { return &Store{DB: db, root: db} }

// WithTx runs fn with a Store bound to one transaction. Any error rolls everything back.
func (s *Store) WithTx(fn func(tx *Store) error) error {
	if s.root == nil {
		return fn(s) // already inside a transaction
	}
	t, err := s.root.Begin()
	if err != nil {
		return err
	}
	if err := fn(&Store{DB: t}); err != nil {
		_ = t.Rollback()
		return err
	}
	return t.Commit()
}

func now() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

func jsonList(s []string) string {
	if s == nil {
		s = []string{}
	}
	b, _ := json.Marshal(s)
	return string(b)
}

func parseList(s string) []string {
	out := []string{}
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func nstr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func nint(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nfloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func mapErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		return ErrConflict
	}
	return err
}

type scanner interface{ Scan(...any) error }
