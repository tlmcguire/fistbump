package store

import (
	"encoding/json"
	"fmt"
	"slices"
)

// SettingDef describes a known settings key.
type SettingDef struct {
	Kind    string // string, int, bool, list
	Default any
	Enum    []string
	Min     int // for int settings
}

var SettingDefs = map[string]SettingDef{
	"ai.mode":                          {Kind: "string", Default: "auto", Enum: []string{"auto", "local", "remote", "rules"}},
	"ai.selected_model":                {Kind: "string", Default: ""},
	"ai.idle_minutes":                  {Kind: "int", Default: 5, Min: 1},
	"ai.remote.base_url":               {Kind: "string", Default: ""},
	"ai.remote.model":                  {Kind: "string", Default: ""},
	"jobs.retention_days":              {Kind: "int", Default: 30, Min: 1}, // 0 would make cleanup remove every untracked job
	"connectors.greenhouse.enabled":    {Kind: "bool", Default: true},
	"connectors.greenhouse.boards":     {Kind: "list", Default: []string{}},
	"connectors.greenhouse.categories": {Kind: "list", Default: []string{}},
}

// Settings returns every known key as a typed value with defaults filled in.
func (s *Store) Settings() (map[string]any, error) {
	rows, err := s.DB.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	raw := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		raw[k] = v
	}
	out := map[string]any{}
	for k, def := range SettingDefs {
		v, ok := raw[k]
		if !ok {
			out[k] = def.Default
			continue
		}
		out[k] = decode(def, v)
	}
	return out, rows.Err()
}

func decode(def SettingDef, v string) any {
	switch def.Kind {
	case "int":
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
			return def.Default
		}
		return n
	case "bool":
		return v == "true"
	case "list":
		return parseList(v)
	}
	return v
}

func encode(def SettingDef, v any) (string, error) {
	switch def.Kind {
	case "string":
		str, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("must be a string")
		}
		if len(def.Enum) > 0 && !slices.Contains(def.Enum, str) {
			return "", fmt.Errorf("must be one of %v", def.Enum)
		}
		return str, nil
	case "int":
		f, ok := v.(float64)
		if !ok || f != float64(int(f)) || int(f) < def.Min {
			return "", fmt.Errorf("must be a whole number of at least %d", def.Min)
		}
		return fmt.Sprint(int(f)), nil
	case "bool":
		b, ok := v.(bool)
		if !ok {
			return "", fmt.Errorf("must be a boolean")
		}
		return fmt.Sprint(b), nil
	case "list":
		arr, ok := v.([]any)
		if !ok {
			return "", fmt.Errorf("must be an array of strings")
		}
		strs := []string{}
		for _, e := range arr {
			es, ok := e.(string)
			if !ok {
				return "", fmt.Errorf("must be an array of strings")
			}
			strs = append(strs, es)
		}
		b, _ := json.Marshal(strs)
		return string(b), nil
	}
	return "", fmt.Errorf("unknown kind")
}

// ValidateSetting checks a value for a known key without writing it.
func ValidateSetting(key string, v any) error {
	def, ok := SettingDefs[key]
	if !ok {
		return fmt.Errorf("unknown setting")
	}
	_, err := encode(def, v)
	return err
}

// SetSetting validates and stores one known key. v is a decoded JSON value.
func (s *Store) SetSetting(key string, v any) error {
	def, ok := SettingDefs[key]
	if !ok {
		return fmt.Errorf("unknown setting")
	}
	text, err := encode(def, v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO settings (key, value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, text)
	return err
}

func (s *Store) SettingString(key string) string {
	m, err := s.Settings()
	if err != nil {
		return ""
	}
	str, _ := m[key].(string)
	return str
}

func (s *Store) SettingList(key string) []string {
	m, err := s.Settings()
	if err != nil {
		return []string{}
	}
	l, _ := m[key].([]string)
	return l
}

func (s *Store) SettingBool(key string) bool {
	m, _ := s.Settings()
	b, _ := m[key].(bool)
	return b
}

func (s *Store) SettingInt(key string) int {
	m, _ := s.Settings()
	n, _ := m[key].(int)
	return n
}

func (s *Store) ClearSetting(key string) error {
	_, err := s.DB.Exec(`DELETE FROM settings WHERE key=?`, key)
	return err
}
