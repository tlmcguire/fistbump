package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const testKey = "sk-test-0123456789abcdef"

// openAIServer is a fake OpenAI-compatible API. handle answers each request; it records them.
type openAIServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recorded
	handle   func(w http.ResponseWriter, body chatRequest, call int)
}

type recorded struct {
	auth string
	body chatRequest
}

func newOpenAIServer(t *testing.T, handle func(w http.ResponseWriter, body chatRequest, call int)) *openAIServer {
	t.Helper()
	s := &openAIServer{handle: handle}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var body chatRequest
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &body)
		s.mu.Lock()
		s.requests = append(s.requests, recorded{auth: r.Header.Get("Authorization"), body: body})
		call := len(s.requests)
		s.mu.Unlock()
		s.handle(w, body, call)
	}))
	t.Cleanup(s.Close)
	return s
}

func reply(w http.ResponseWriter, content string) {
	json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": content}}},
	})
}

func TestValidateBaseURL(t *testing.T) {
	ok := []string{"https://api.example.com/v1", "http://127.0.0.1:11434/v1", "http://localhost:8080/v1/", "http://[::1]:9000/v1"}
	for _, u := range ok {
		if _, err := ValidateBaseURL(u); err != nil {
			t.Errorf("ValidateBaseURL(%q) = %v, want ok", u, err)
		}
	}
	bad := []string{"http://api.example.com/v1", "ftp://example.com", "api.example.com", "https://user:pw@example.com/v1", "https://example.com/v1?x=1", ""}
	for _, u := range bad {
		if _, err := ValidateBaseURL(u); err == nil {
			t.Errorf("ValidateBaseURL(%q) = ok, want an error", u)
		}
	}
	if got, _ := ValidateBaseURL("http://localhost:8080/v1/"); got != "http://localhost:8080/v1" {
		t.Errorf("trailing slash kept: %q", got)
	}
	if _, err := ValidateBaseURL("http://api.example.com/v1"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("http for a hosted provider: %v", err)
	}
}

func TestChatSendsKeyAndModel(t *testing.T) {
	s := newOpenAIServer(t, func(w http.ResponseWriter, _ chatRequest, _ int) { reply(w, "pong") })
	r := NewRemote()
	if err := r.Configure(s.URL+"/v1", "m1", testKey); err != nil {
		t.Fatal(err)
	}
	got, err := r.Chat(context.Background(), []Message{{Role: "user", Content: "ping"}}, 8, false)
	if err != nil || got != "pong" {
		t.Fatalf("Chat = %q, %v", got, err)
	}
	req := s.requests[0]
	if req.auth != "Bearer "+testKey || req.body.Model != "m1" || req.body.ResponseFormat != nil {
		t.Errorf("request = %+v", req)
	}
}

func TestChatWithoutKeySendsNoAuthorization(t *testing.T) {
	s := newOpenAIServer(t, func(w http.ResponseWriter, _ chatRequest, _ int) { reply(w, "ok") })
	r := NewRemote()
	r.Configure(s.URL+"/v1", "llama3.2", "")
	if _, err := r.Chat(context.Background(), nil, 8, false); err != nil {
		t.Fatal(err)
	}
	if s.requests[0].auth != "" {
		t.Errorf("Authorization = %q, want none", s.requests[0].auth)
	}
	if !r.Ready() {
		t.Error("a loopback provider with no key should be ready")
	}
}

func TestChatRetriesWithoutJSONModeOn400(t *testing.T) {
	s := newOpenAIServer(t, func(w http.ResponseWriter, body chatRequest, call int) {
		if body.ResponseFormat != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"message":"response_format is not supported"}}`))
			return
		}
		reply(w, `{"proposed":"x"}`)
	})
	r := NewRemote()
	r.Configure(s.URL+"/v1", "m", "")
	got, err := r.Chat(context.Background(), nil, 8, true)
	if err != nil || got != `{"proposed":"x"}` {
		t.Fatalf("Chat = %q, %v", got, err)
	}
	if len(s.requests) != 2 || s.requests[0].body.ResponseFormat == nil || s.requests[1].body.ResponseFormat != nil {
		t.Errorf("want one JSON-mode request and one retry without it, got %+v", s.requests)
	}
}

func TestChatRetriesWithoutJSONModeOn501(t *testing.T) {
	s := newOpenAIServer(t, func(w http.ResponseWriter, body chatRequest, call int) {
		if body.ResponseFormat != nil {
			w.WriteHeader(http.StatusNotImplemented)
			w.Write([]byte(`{"error":{"message":"structured output is unavailable"}}`))
			return
		}
		reply(w, `{"proposed":"x","reason":"y"}`)
	})
	r := NewRemote()
	r.Configure(s.URL+"/v1", "m", "")
	got, err := r.Chat(context.Background(), nil, 8, true)
	if err != nil || got != `{"proposed":"x","reason":"y"}` || len(s.requests) != 2 {
		t.Fatalf("Chat = %q, %v after %d requests", got, err, len(s.requests))
	}
}

func TestRedirectToAnotherHostIsRefused(t *testing.T) {
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = true
		reply(w, "stolen")
	}))
	defer other.Close()
	s := newOpenAIServer(t, func(w http.ResponseWriter, _ chatRequest, _ int) {
		// 127.0.0.1 and localhost are different hosts to the client.
		w.Header().Set("Location", strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+"/v1/chat/completions")
		w.WriteHeader(http.StatusTemporaryRedirect)
	})
	r := NewRemote()
	r.Configure(s.URL+"/v1", "m", testKey)
	if _, err := r.Chat(context.Background(), nil, 8, false); err == nil {
		t.Fatal("a redirect to another host should fail")
	}
	if leaked {
		t.Error("the request reached the other host")
	}
}

func TestErrorsNeverIncludeTheKey(t *testing.T) {
	s := newOpenAIServer(t, func(w http.ResponseWriter, _ chatRequest, _ int) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Incorrect API key provided: ` + testKey + strings.Repeat(" padding", 80) + `"}}`))
	})
	r := NewRemote()
	r.Configure(s.URL+"/v1", "m", testKey)
	_, err := r.Chat(context.Background(), nil, 8, false)
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("error leaks the key: %v", err)
	}
	if n := len([]rune(err.Error())); n > maxErrMessage+1 {
		t.Errorf("error is %d characters, want at most %d", n, maxErrMessage+1)
	}
}

func TestHostedProviderNeedsKey(t *testing.T) {
	r := NewRemote()
	r.Configure("https://api.example.com/v1", "m", "")
	if r.Ready() || !r.Config().NeedsKey() {
		t.Error("a hosted provider with no key must not be ready")
	}
	r.Configure("https://api.example.com/v1", "m", testKey)
	if !r.Ready() || !r.Config().HasKey {
		t.Error("with a key it is ready")
	}
}

func TestConfigureKeepsKeyOnlyForTheSameHost(t *testing.T) {
	r := NewRemote()
	r.Configure("https://api.example.com/v1", "a", testKey)
	r.Configure("https://api.example.com/v1", "b", "") // model change
	if !r.Config().HasKey {
		t.Error("changing only the model should keep the key")
	}
	r.Configure("https://other.example.com/v1", "b", "")
	if r.Config().HasKey {
		t.Error("a new host must not get the old key")
	}
}
