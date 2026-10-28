package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func raw(m map[string]string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range m {
		out[k] = json.RawMessage(v)
	}
	return out
}

func TestSettingsDefaultsAndWrite(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	got, err := s.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(knownSettings) {
		t.Errorf("got %d keys, want %d", len(got), len(knownSettings))
	}
	if got["ai.mode"] != "auto" || got["ai.idle_minutes"] != 5 || got["ai.remote.model"] != "" {
		t.Errorf("defaults wrong: %v", got)
	}
	if err := s.PutSettings(ctx, raw(map[string]string{"ai.idle_minutes": "9", "connectors.greenhouse.boards": `["acme"]`, "ai.mode": `"rules"`})); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSettings(ctx)
	if got["ai.idle_minutes"] != 9 || got["ai.mode"] != "rules" || got["connectors.greenhouse.boards"].([]string)[0] != "acme" {
		t.Errorf("after put: %v", got)
	}
}

func TestSettingsBadValueWritesNothing(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	err := s.PutSettings(ctx, raw(map[string]string{"ai.idle_minutes": "9", "jobs.trash_days": "0"}))
	var bad *SettingError
	if !errors.As(err, &bad) || bad.Key != "jobs.trash_days" {
		t.Fatalf("err = %v, want SettingError on jobs.trash_days", err)
	}
	if got, _ := s.GetSettings(ctx); got["ai.idle_minutes"] != 5 {
		t.Errorf("a valid key was written despite a bad one: %v", got["ai.idle_minutes"])
	}
}

func TestSettingsRejects(t *testing.T) {
	for name, in := range map[string]map[string]string{
		"unknown key":     {"nope": `1`},
		"secret key":      {"ai.remote.api_key": `"sk-123"`},
		"token key":       {"models.token": `"x"`},
		"wrong int type":  {"ai.idle_minutes": `"five"`},
		"fractional int":  {"ai.idle_minutes": `1.5`},
		"bad enum":        {"ai.mode": `"magic"`},
		"string not list": {"connectors.greenhouse.boards": `"acme"`},
		"null list":       {"connectors.greenhouse.boards": `null`},
		"below minimum":   {"jobs.retention_days": `0`},
	} {
		if err := ValidateSettings(raw(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
