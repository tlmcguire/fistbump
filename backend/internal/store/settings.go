package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type settingKind int

const (
	kindString settingKind = iota
	kindInt
	kindStrings
)

// settingDef describes one known key. Allowed limits a string to a set; Min bounds an integer.
type settingDef struct {
	kind    settingKind
	def     any
	min     int
	allowed []string
}

// knownSettings is every row of the settings table in docs/api.md.
var knownSettings = map[string]settingDef{
	"ai.mode":                          {kind: kindString, def: "auto", allowed: []string{"auto", "local", "remote", "rules"}},
	"ai.selected_model":                {kind: kindString, def: ""},
	"ai.idle_minutes":                  {kind: kindInt, def: 5, min: 1},
	"ai.remote.base_url":               {kind: kindString, def: ""},
	"ai.remote.model":                  {kind: kindString, def: ""},
	"jobs.retention_days":              {kind: kindInt, def: 30, min: 1},
	"jobs.trash_days":                  {kind: kindInt, def: 7, min: 1},
	"connectors.greenhouse.boards":     {kind: kindStrings, def: []string{}},
	"connectors.greenhouse.categories": {kind: kindStrings, def: []string{}},
}

// SettingError names the key that failed validation.
type SettingError struct {
	Key, Message string
}

func (e *SettingError) Error() string { return e.Message }

// secretWords mark a key that must never be stored, so secrets never land in SQLite.
var secretWords = []string{"key", "token", "secret", "password"}

// parseSetting checks a JSON value against def and returns the typed value and its stored text.
func parseSetting(key string, def settingDef, raw json.RawMessage) (any, string, error) {
	bad := func(format string, args ...any) error {
		return &SettingError{Key: key, Message: key + " " + fmt.Sprintf(format, args...)}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	switch def.kind {
	case kindString:
		var v string
		if err := dec.Decode(&v); err != nil {
			return nil, "", bad("must be a string")
		}
		if def.allowed != nil {
			ok := false
			for _, a := range def.allowed {
				ok = ok || a == v
			}
			if !ok {
				return nil, "", bad("must be one of %s", strings.Join(def.allowed, ", "))
			}
		}
		return v, v, nil
	case kindInt:
		var v int
		if err := dec.Decode(&v); err != nil {
			return nil, "", bad("must be a whole number")
		}
		if v < def.min {
			return nil, "", bad("must be at least %d", def.min)
		}
		return v, strconv.Itoa(v), nil
	default:
		var v []string
		if err := dec.Decode(&v); err != nil || v == nil {
			return nil, "", bad("must be a list of strings")
		}
		b, _ := json.Marshal(v)
		return v, string(b), nil
	}
}

// ValidateSettings checks every key and value without writing. Errors are *SettingError.
func ValidateSettings(values map[string]json.RawMessage) error {
	for key, raw := range values {
		lower := strings.ToLower(key)
		for _, w := range secretWords {
			if strings.Contains(lower, w) {
				return &SettingError{Key: key, Message: key + " looks like a secret and cannot be stored"}
			}
		}
		def, ok := knownSettings[key]
		if !ok {
			return &SettingError{Key: key, Message: key + " is not a known setting"}
		}
		if _, _, err := parseSetting(key, def, raw); err != nil {
			return err
		}
	}
	return nil
}

// GetSettings returns every known key with its typed value, filling defaults for absent rows.
func (s *Store) GetSettings(ctx context.Context) (map[string]any, error) {
	out := make(map[string]any, len(knownSettings))
	for key, def := range knownSettings {
		out[key] = def.def
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, text string
		if err := rows.Scan(&key, &text); err != nil {
			return nil, mapErr(err)
		}
		def, ok := knownSettings[key]
		if !ok {
			continue
		}
		switch def.kind {
		case kindInt:
			if n, err := strconv.Atoi(text); err == nil && n >= def.min {
				out[key] = n
			}
		case kindStrings:
			if list := decodeList(text); list != nil {
				out[key] = list
			}
		default:
			out[key] = text
		}
	}
	return out, mapErr(rows.Err())
}

// PutSettings validates every value first, then writes them all in one transaction.
func (s *Store) PutSettings(ctx context.Context, values map[string]json.RawMessage) error {
	if err := ValidateSettings(values); err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *Store) error {
		for key, raw := range values {
			_, text, err := parseSetting(key, knownSettings[key], raw)
			if err != nil {
				return err
			}
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
  ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, text); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
}
