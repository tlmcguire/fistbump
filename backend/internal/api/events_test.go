package api

import "testing"

func TestReadEventChecksTimesAndKind(t *testing.T) {
	ok := eventBody{Title: "Interview", StartsAt: "2026-12-01T10:00:00-05:00", EndsAt: "2026-12-01T11:00:00-05:00"}
	in, err := readEvent(ok)
	if err != nil || in.Kind != "interview" {
		t.Fatalf("a plain interview should be read, got %v %v", in, err)
	}
	for name, b := range map[string]eventBody{
		"no title":         {StartsAt: ok.StartsAt, EndsAt: ok.EndsAt},
		"ends before":      {Title: "x", StartsAt: ok.EndsAt, EndsAt: ok.StartsAt},
		"unknown kind":     {Title: "x", Kind: "party", StartsAt: ok.StartsAt, EndsAt: ok.EndsAt},
		"not a time":       {Title: "x", StartsAt: "tomorrow", EndsAt: ok.EndsAt},
		"longer than week": {Title: "x", StartsAt: ok.StartsAt, EndsAt: "2026-12-20T11:00:00-05:00"},
	} {
		if _, err := readEvent(b); err == nil {
			t.Errorf("%s: should be refused", name)
		}
	}
}
