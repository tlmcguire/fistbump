package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Message is one chat message in the OpenAI format.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RemoteConfig is what the status route may show. It never holds the key.
type RemoteConfig struct {
	BaseURL string
	Model   string
	HasKey  bool
}

// Configured reports whether a base URL and model are set.
func (c RemoteConfig) Configured() bool { return c.BaseURL != "" && c.Model != "" }

// NeedsKey reports a hosted provider with no key in memory. Loopback servers such as Ollama
// need none.
func (c RemoteConfig) NeedsKey() bool {
	return c.Configured() && !c.HasKey && !isLoopbackURL(c.BaseURL)
}

// Remote is a client for any OpenAI-compatible chat API: a hosted provider, Ollama, or the
// local llama-server. The key is held in memory only and never logged or returned.
type Remote struct {
	// Label is the engine name on drafts: EngineRemote, or EngineLocal when Local drives it.
	Label string

	mu      sync.Mutex
	baseURL string
	model   string
	key     string
	client  *http.Client
}

const (
	maxReply      = 1 << 20 // bytes read from a chat response
	maxErrMessage = 300     // characters of an upstream error kept
	chatTimeout   = 2 * time.Minute
)

// NewRemote returns an unconfigured client.
func NewRemote() *Remote {
	r := &Remote{Label: EngineRemote}
	r.client = &http.Client{
		Timeout: chatTimeout,
		// Follow a redirect only to the same host, so the key never goes anywhere else.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme {
				return errors.New("the provider redirected to another host")
			}
			return nil
		},
	}
	return r
}

// ValidateBaseURL checks a provider address and returns it without a trailing slash. It must use
// https, or http only when the host is this computer.
func ValidateBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("base_url must be an http or https address")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("base_url must not contain a user name, query or fragment")
	}
	if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
		return "", errors.New("base_url must use https unless the host is this computer")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && isLoopbackHost(u.Hostname())
}

// Configure sets the provider. An empty key keeps the current one only when the host is
// unchanged, so a key is never sent to a host it was not given for.
func (r *Remote) Configure(baseURL, model, key string) error {
	clean, err := ValidateBaseURL(baseURL)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if key == "" && sameHost(r.baseURL, clean) {
		key = r.key
	}
	r.baseURL, r.model, r.key = clean, strings.TrimSpace(model), key
	return nil
}

func sameHost(a, b string) bool {
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	return a != "" && err1 == nil && err2 == nil && ua.Scheme == ub.Scheme && ua.Host == ub.Host
}

// Clear forgets the provider and drops the key.
func (r *Remote) Clear() {
	r.mu.Lock()
	r.baseURL, r.model, r.key = "", "", ""
	r.mu.Unlock()
}

// Config returns the provider settings without the key.
func (r *Remote) Config() RemoteConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RemoteConfig{BaseURL: r.baseURL, Model: r.model, HasKey: r.key != ""}
}

func (r *Remote) Name() string { return r.Label }

// Ready reports a configured provider that has a key, or needs none.
func (r *Remote) Ready() bool {
	c := r.Config()
	return c.Configured() && !c.NeedsKey()
}

type chatRequest struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	MaxTokens      int       `json:"max_tokens,omitempty"`
	Temperature    float64   `json:"temperature"`
	ResponseFormat any       `json:"response_format,omitempty"`
}

// Chat sends one chat completion and returns the reply text. With jsonMode it asks for a JSON
// object, and retries once without that if the server rejects it with 400.
func (r *Remote) Chat(ctx context.Context, msgs []Message, maxTokens int, jsonMode bool) (string, error) {
	r.mu.Lock()
	baseURL, model, key := r.baseURL, r.model, r.key
	r.mu.Unlock()
	if baseURL == "" || model == "" {
		return "", ErrUnavailable
	}
	reply, status, err := r.post(ctx, baseURL, model, key, msgs, maxTokens, jsonMode)
	if jsonMode && status == http.StatusBadRequest {
		reply, _, err = r.post(ctx, baseURL, model, key, msgs, maxTokens, false)
	}
	return reply, err
}

func (r *Remote) post(ctx context.Context, baseURL, model, key string, msgs []Message, maxTokens int, jsonMode bool) (string, int, error) {
	body := chatRequest{Model: model, Messages: msgs, MaxTokens: maxTokens, Temperature: 0.2}
	if jsonMode {
		body.ResponseFormat = map[string]string{"type": "json_object"}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return "", 0, redact(err.Error(), key)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReply))
	if err != nil {
		return "", resp.StatusCode, redact(err.Error(), key)
	}
	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, redact(fmt.Sprintf("provider answered %s: %s", resp.Status, upstreamMessage(data)), key)
	}
	var out struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil || len(out.Choices) == 0 {
		return "", resp.StatusCode, errors.New("provider sent a reply that is not a chat completion")
	}
	return out.Choices[0].Message.Content, resp.StatusCode, nil
}

// upstreamMessage pulls error.message from an OpenAI-style error body, or uses the raw text.
func upstreamMessage(data []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return strings.TrimSpace(string(data))
}

// redact trims an error message and removes the key if the provider echoed it.
func redact(msg, key string) error {
	if key != "" {
		msg = strings.ReplaceAll(msg, key, "[redacted]")
	}
	if r := []rune(msg); len(r) > maxErrMessage {
		msg = string(r[:maxErrMessage]) + "…"
	}
	return errors.New(msg)
}

// Suggest asks the provider for one rewrite per target.
func (r *Remote) Suggest(ctx context.Context, req Request) ([]Draft, error) {
	if !r.Ready() {
		return nil, ErrUnavailable
	}
	return suggestWith(ctx, r.Label, r.Chat, req)
}

type chatFunc func(ctx context.Context, msgs []Message, maxTokens int, jsonMode bool) (string, error)

// suggestWith runs one model call per target. Each reply is parsed and checked by the guard; a
// reply that fails is dropped, and the rules engine fills that item later. If every call fails,
// the last error is returned so auto mode can try the next engine.
func suggestWith(ctx context.Context, engine string, chat chatFunc, req Request) ([]Draft, error) {
	var out []Draft
	var lastErr error
	calls, failures := 0, 0
	for _, t := range req.Targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if strings.TrimSpace(t.Original) == "" {
			continue // nothing to rewrite, and a model asked to write from nothing would invent
		}
		calls++
		reply, err := chat(ctx, BuildPrompt(t, req), maxTokensFor(t), true)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			failures++
			lastErr = err
			slog.Warn("model call failed", "engine", engine, "target", t.Label, "error", err)
			continue
		}
		proposed, ok := ParseReply(reply)
		if !ok {
			logDrop(engine, t, "reply was not the expected JSON")
			continue
		}
		proposed = toResumeText(t, proposed)
		if reason := Check(t, proposed, req); reason != "" {
			logDrop(engine, t, reason)
			continue
		}
		out = append(out, Draft{Section: t.Section, TargetID: t.TargetID, Original: t.Original, Proposed: proposed, Engine: engine})
	}
	if calls > 0 && failures == calls {
		return nil, lastErr
	}
	return out, nil
}

// maxTokensFor allows about twice the original's length (a token is about 4 characters).
func maxTokensFor(t Target) int {
	return min(max(len(t.Original)/2+64, 128), 1024)
}

// logDrop records why a proposal was dropped, never its text.
func logDrop(engine string, t Target, reason string) {
	slog.Info("model proposal dropped", "engine", engine, "target", t.Label, "reason", reason)
}
