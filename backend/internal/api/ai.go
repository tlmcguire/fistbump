package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
)

// aiStatus is GET /v1/ai/status. It never holds the remote key, only whether one is set.
type aiStatus struct {
	Mode         string `json:"mode"`
	ActiveEngine string `json:"active_engine"` // the first engine the mode would use now, or ""
	Local        struct {
		Model     string `json:"model"`
		Installed bool   `json:"installed"`
		Running   bool   `json:"running"`
		Available bool   `json:"available"` // llama-server is bundled with this build
	} `json:"local"`
	Remote struct {
		Configured bool   `json:"configured"`
		NeedsKey   bool   `json:"needs_key"`
		BaseURL    string `json:"base_url"`
		Model      string `json:"model"`
		HasKey     bool   `json:"has_key"`
	} `json:"remote"`
}

// aiMode reads ai.mode from settings.
func (s *Server) aiMode(ctx context.Context) (string, error) {
	values, err := s.store.GetSettings(ctx)
	if err != nil {
		return "", err
	}
	mode, _ := values["ai.mode"].(string)
	if mode == "" {
		mode = "auto"
	}
	return mode, nil
}

func (s *Server) aiStatusGet(w http.ResponseWriter, r *http.Request) error {
	st, err := s.buildAIStatus(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, st)
	return nil
}

func (s *Server) buildAIStatus(ctx context.Context) (aiStatus, error) {
	var st aiStatus
	mode, err := s.aiMode(ctx)
	if err != nil {
		return st, err
	}
	st.Mode = mode
	if order := s.rounds.engines.Order(mode); len(order) > 0 {
		st.ActiveEngine = order[0]
	}
	values, err := s.store.GetSettings(ctx)
	if err != nil {
		return st, err
	}
	st.Local.Model, _ = values["ai.selected_model"].(string)
	st.Local.Installed = st.Local.Model != "" && ai.Installed(s.dataDir, st.Local.Model)
	if s.local != nil {
		st.Local.Running = s.local.Running()
		st.Local.Available = s.local.Binary != ""
	}
	c := s.remote.Config()
	st.Remote.Configured = c.Configured()
	st.Remote.NeedsKey = c.NeedsKey()
	st.Remote.BaseURL = c.BaseURL
	st.Remote.Model = c.Model
	st.Remote.HasKey = c.HasKey
	return st, nil
}

// aiModePut handles PUT /v1/ai/mode.
func (s *Server) aiModePut(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Mode string `json:"mode"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	switch body.Mode {
	case "auto", ai.EngineLocal, ai.EngineRemote, ai.EngineRules:
	default:
		return validationError("mode", "mode must be auto, local, remote or rules")
	}
	if err := s.putSetting(r.Context(), "ai.mode", body.Mode); err != nil {
		return err
	}
	return s.aiStatusGet(w, r)
}

// aiRemotePut handles PUT /v1/ai/remote. The key is held in memory only: base URL and model go to
// settings, the key goes nowhere but the Remote client. Electron sends it again at every launch.
func (s *Server) aiRemotePut(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		BaseURL string `json:"base_url"`
		Model   string `json:"model"`
		APIKey  string `json:"api_key"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	clean, err := ai.ValidateBaseURL(body.BaseURL)
	if err != nil {
		return validationError("base_url", err.Error())
	}
	model := strings.TrimSpace(body.Model)
	if model == "" {
		return validationError("model", "model is required")
	}
	if strings.ContainsAny(body.APIKey, "\r\n") {
		return validationError("api_key", "api_key must be a single line")
	}
	if err := s.remote.Configure(clean, model, strings.TrimSpace(body.APIKey)); err != nil {
		return validationError("base_url", err.Error())
	}
	values := map[string]json.RawMessage{
		"ai.remote.base_url": mustJSON(clean),
		"ai.remote.model":    mustJSON(model),
	}
	if err := s.store.PutSettings(r.Context(), values); err != nil {
		return err
	}
	return s.aiStatusGet(w, r)
}

// aiRemoteDelete handles DELETE /v1/ai/remote: it forgets the provider and drops the key.
func (s *Server) aiRemoteDelete(w http.ResponseWriter, r *http.Request) error {
	s.remote.Clear()
	values := map[string]json.RawMessage{
		"ai.remote.base_url": mustJSON(""),
		"ai.remote.model":    mustJSON(""),
	}
	if err := s.store.PutSettings(r.Context(), values); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// testTimeout bounds POST /v1/ai/test. A local test may include loading the model.
const testTimeout = 90 * time.Second

// aiTest handles POST /v1/ai/test. A failure is still 200, with ok false and the error.
func (s *Server) aiTest(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Engine string `json:"engine"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	var chat func(context.Context, []ai.Message, int, bool) (string, error)
	var ready bool
	switch body.Engine {
	case ai.EngineLocal:
		if s.local != nil {
			chat, ready = s.local.Chat, s.local.Ready()
		}
	case ai.EngineRemote:
		chat, ready = s.remote.Chat, s.remote.Ready()
	default:
		return validationError("engine", "engine must be local or remote")
	}
	result := struct {
		OK        bool    `json:"ok"`
		LatencyMS int64   `json:"latency_ms"`
		Sample    string  `json:"sample"`
		Error     *string `json:"error"`
	}{}
	fail := func(msg string) {
		result.Error = &msg
	}
	if !ready {
		if body.Engine == ai.EngineLocal {
			fail("The local model is not ready: download and select a model first")
		} else if s.remote.Config().NeedsKey() {
			fail("The remote provider needs an API key")
		} else {
			fail("No remote provider is set up")
		}
		writeJSON(w, http.StatusOK, result)
		return nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), testTimeout)
	defer cancel()
	start := time.Now()
	reply, err := chat(ctx, []ai.Message{
		{Role: "system", Content: "You are a connection test. Follow the instruction exactly."},
		{Role: "user", Content: "Reply with the single word: pong"},
	}, 8, false)
	result.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		fail(err.Error())
	} else {
		result.OK = true
		result.Sample = clipText(strings.TrimSpace(reply), 100)
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// putSetting writes one setting through the store's validation.
func (s *Server) putSetting(ctx context.Context, key string, value any) error {
	return s.store.PutSettings(ctx, map[string]json.RawMessage{key: mustJSON(value)})
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func clipText(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
