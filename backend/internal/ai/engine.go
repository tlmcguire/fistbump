package ai

import (
	"context"
	"errors"
	"fmt"
)

// Engines picks which Provider runs a request and falls through to the next one on failure.
type Engines struct {
	Local  *Local
	Remote *Remote
	Rules  Rules
}

// Order returns the provider names to try for a mode. A forced mode returns only that engine.
func (e *Engines) Order(mode string) []string {
	switch mode {
	case "local", "remote", "rules":
		return []string{mode}
	}
	var out []string
	if e.Local.Ready() {
		out = append(out, "local")
	}
	if e.Remote.Ready() {
		out = append(out, "remote")
	}
	return append(out, "rules")
}

func (e *Engines) Get(name string) Provider {
	switch name {
	case "local":
		return e.Local
	case "remote":
		return e.Remote
	}
	return e.Rules
}

// CanRun reports whether a forced engine could run right now.
func (e *Engines) CanRun(name string) bool {
	switch name {
	case "local":
		return e.Local.Ready()
	case "remote":
		return e.Remote.Ready()
	}
	return true
}

// Active is the first engine auto mode would try.
func (e *Engines) Active(mode string) string { return e.Order(mode)[0] }

// Run tries each engine in order and returns the first success with the engine that produced it.
// In a forced mode a failure is returned as-is. In auto mode it falls through to the next engine.
func (e *Engines) Run(ctx context.Context, mode string, r Request) ([]Draft, string, error) {
	var last error
	for _, name := range e.Order(mode) {
		drafts, err := e.Get(name).Suggest(ctx, r)
		if err == nil {
			if name != "rules" {
				if len(drafts) == 0 { // nothing usable from the model: say so instead of labeling rule output as AI
					name = "rules"
				}
				drafts = fillGaps(ctx, drafts, r)
			}
			return drafts, name, nil
		}
		if ctx.Err() != nil {
			return nil, name, ctx.Err()
		}
		last = fmt.Errorf("%s: %w", name, err)
	}
	if last == nil {
		last = errors.New("no engine available")
	}
	return nil, "", last
}

// fillGaps adds rule-based drafts for targets the model left unchanged, so small models that only
// rewrite a few items still produce a useful revision.
func fillGaps(ctx context.Context, drafts []Draft, r Request) []Draft {
	covered := map[string]bool{}
	key := func(section string, id *int64, original string) string {
		k := section + "|" + original
		if id != nil {
			k += fmt.Sprint("|", *id)
		}
		return k
	}
	for _, d := range drafts {
		covered[key(d.Section, d.TargetID, d.Original)] = true
	}
	extra, _ := Rules{}.Suggest(ctx, r)
	for _, d := range extra {
		if !covered[key(d.Section, d.TargetID, d.Original)] {
			drafts = append(drafts, d)
		}
	}
	return drafts
}
