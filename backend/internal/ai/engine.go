package ai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// Engines holds one provider per engine. Local and Remote are nil when not built in (tests).
type Engines struct {
	Local  Provider
	Remote Provider
	Rules  Provider
}

func (e Engines) get(name string) Provider {
	switch name {
	case EngineLocal:
		return e.Local
	case EngineRemote:
		return e.Remote
	case EngineRules:
		return e.Rules
	}
	return nil
}

// Order lists the engines to try for a mode, leaving out any that are missing or not ready.
// "auto" tries local, then remote, then rules; any other mode names a single engine.
func (e Engines) Order(mode string) []string {
	candidates := []string{mode}
	if mode == "auto" {
		candidates = []string{EngineLocal, EngineRemote, EngineRules}
	}
	var out []string
	for _, name := range candidates {
		if p := e.get(name); p != nil && p.Ready() {
			out = append(out, name)
		}
	}
	return out
}

// Chatter is implemented by AI providers (Local and Remote) that support open-ended chat completions.
type Chatter interface {
	Chat(ctx context.Context, msgs []Message, maxTokens int, jsonMode bool) (string, error)
}

// Chat sends a chat completion to the first ready AI model (Local or Remote) allowed by mode.
// Unlike Run, it never falls back to the rules provider; callers handle deterministic fallback explicitly.
func (e Engines) Chat(ctx context.Context, mode string, msgs []Message, maxTokens int, jsonMode bool) (string, string, error) {
	if mode == "" || mode == EngineRules {
		mode = "auto"
	}
	order := e.Order(mode)
	var lastErr error
	tried := 0
	for _, name := range order {
		if name == EngineRules {
			continue
		}
		chatter, ok := e.get(name).(Chatter)
		if !ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		tried++
		reply, err := chatter.Chat(ctx, msgs, maxTokens, jsonMode)
		if err != nil {
			lastErr = err
			slog.Warn("ai chat failed", "engine", name, "error", err)
			if mode != "auto" {
				return "", "", err
			}
			continue
		}
		return strings.TrimSpace(reply), name, nil
	}
	if tried == 0 {
		return "", "", ErrUnavailable
	}
	return "", "", fmt.Errorf("all AI engines failed: %w", lastErr)
}

// Run asks the engines for a mode in order and returns the drafts with the label of the engine
// that wrote them. In auto mode an engine that fails is skipped; a forced engine's error is
// returned. ErrUnavailable means no engine could run.
func (e Engines) Run(ctx context.Context, mode string, r Request) ([]Draft, string, error) {
	order := e.Order(mode)
	if len(order) == 0 {
		return nil, "", ErrUnavailable
	}
	var lastErr error
	for _, name := range order {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		drafts, err := e.get(name).Suggest(ctx, r)
		if err != nil {
			lastErr = err
			slog.Warn("ai engine failed", "engine", name, "error", err)
			if mode != "auto" {
				return nil, "", err
			}
			continue
		}
		return e.fillGaps(ctx, r, drafts, name)
	}
	return nil, "", fmt.Errorf("all engines failed: %w", lastErr)
}

// fillGaps gives every target the engine skipped a rules draft, so a model round never leaves
// part of the resume without a suggestion. A round where the engine wrote nothing is labeled rules.
func (e Engines) fillGaps(ctx context.Context, r Request, drafts []Draft, engine string) ([]Draft, string, error) {
	if engine == EngineRules {
		return drafts, engine, nil
	}
	covered := map[string]bool{}
	for _, d := range drafts {
		covered[targetKey(d.Section, d.TargetID, d.Original)] = true
	}
	var missing []Target
	for _, t := range r.Targets {
		if !covered[targetKey(t.Section, t.TargetID, t.Original)] {
			missing = append(missing, t)
		}
	}
	out := drafts
	if len(missing) > 0 && e.Rules != nil {
		gaps, err := e.Rules.Suggest(ctx, Request{Job: r.Job, Analysis: r.Analysis, Profile: r.Profile, Targets: missing, Language: r.Language})
		if err != nil {
			return nil, "", err
		}
		out = append(out, gaps...)
	}
	// The round is labeled with the engine that wrote most of its suggestions; a tie goes to the
	// model. A round where the model contributed nothing is labeled rules.
	if len(drafts) == 0 || len(out)-len(drafts) > len(drafts) {
		engine = EngineRules
	}
	return out, engine, nil
}

// targetKey identifies a target by section, row id and text.
func targetKey(section string, id *int64, original string) string {
	n := int64(-1)
	if id != nil {
		n = *id
	}
	return fmt.Sprintf("%s|%d|%s", section, n, original)
}
