package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Remote is an OpenAI-compatible chat client (hosted APIs and Ollama). The API key lives only in memory.
type Remote struct {
	mu      sync.RWMutex
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
	name    string
}

func NewRemote() *Remote { return newClient("remote") }

func newClient(name string) *Remote {
	r := &Remote{name: name}
	r.client = &http.Client{
		Timeout: 180 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Host != via[0].URL.Host {
				return errors.New("cross-host redirect refused")
			}
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
	return r
}

func (r *Remote) Name() string { return r.name }

// ValidateBaseURL requires HTTPS unless the host is loopback.
func ValidateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("base_url is not a valid URL")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && IsLoopback(u.Hostname()) {
		return nil
	}
	return errors.New("base_url must use https unless the host is loopback")
}

func IsLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (r *Remote) Configure(baseURL, model, key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.baseURL, r.model, r.apiKey = strings.TrimRight(baseURL, "/"), model, key
}

// SetKey replaces only the in-memory key.
func (r *Remote) SetKey(key string) {
	r.mu.Lock()
	r.apiKey = key
	r.mu.Unlock()
}

func (r *Remote) Clear() { r.Configure("", "", "") }

func (r *Remote) Config() (baseURL, model string, hasKey bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.baseURL, r.model, r.apiKey != ""
}

// Configured reports whether a base URL and model are set.
func (r *Remote) Configured() bool {
	b, m, _ := r.Config()
	return b != "" && m != ""
}

// NeedsKey reports a hosted provider configured without its key, for example after a restart on a
// system where the key could not be stored. Loopback servers such as Ollama need no key.
func (r *Remote) NeedsKey() bool {
	b, _, hasKey := r.Config()
	if b == "" || hasKey {
		return false
	}
	u, err := url.Parse(b)
	return err != nil || !IsLoopback(u.Hostname())
}

// Ready reports whether a request could be sent: configured, and keyed unless the server is local.
func (r *Remote) Ready() bool { return r.Configured() && !r.NeedsKey() }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Chat sends messages and returns the first reply.
func (r *Remote) Chat(ctx context.Context, msgs []chatMessage, maxTokens int) (string, error) {
	return r.chat(ctx, msgs, maxTokens, false)
}

// ChatJSON asks for a JSON object reply. Servers that reject response_format are retried without it.
func (r *Remote) ChatJSON(ctx context.Context, msgs []chatMessage, maxTokens int) (string, error) {
	out, err := r.chat(ctx, msgs, maxTokens, true)
	if err != nil && strings.Contains(err.Error(), "status 400") {
		return r.chat(ctx, msgs, maxTokens, false)
	}
	return out, err
}

func (r *Remote) chat(ctx context.Context, msgs []chatMessage, maxTokens int, jsonMode bool) (string, error) {
	r.mu.RLock()
	base, model, key := r.baseURL, r.model, r.apiKey
	r.mu.RUnlock()
	if base == "" {
		return "", ErrUnavailable
	}
	payload := map[string]any{"model": model, "messages": msgs, "temperature": 0.2, "max_tokens": maxTokens}
	if jsonMode {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrUnavailable, trimErr(err.Error(), key))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: status %d: %s", ErrUnavailable, resp.StatusCode, trimErr(string(data), key))
	}
	var out struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil || len(out.Choices) == 0 {
		return "", errors.New("unexpected reply format")
	}
	return out.Choices[0].Message.Content, nil
}

// trimErr shortens upstream text and removes the key if it was echoed.
func trimErr(s, key string) string {
	if key != "" {
		s = strings.ReplaceAll(s, key, "[redacted]")
	}
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

func (r *Remote) Suggest(ctx context.Context, req Request) ([]Draft, error) {
	if !r.Ready() {
		return nil, ErrUnavailable
	}
	reply, err := r.ChatJSON(ctx, []chatMessage{{"system", systemPrompt}, {"user", buildPrompt(req)}}, 2048)
	if err != nil {
		return nil, err
	}
	return parseDraftsGrounded(reply, req.Targets, groundingSource(req), requestSkills(req))
}

// Test sends a tiny prompt and returns the sample reply.
func (r *Remote) Test(ctx context.Context) (string, error) {
	return r.Chat(ctx, []chatMessage{{"user", "Reply with the single word: pong"}}, 16)
}
