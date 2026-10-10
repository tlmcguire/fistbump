package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
)

func (s *Server) aiStatusBody() map[string]any {
	mode := s.st.SettingString("ai.mode")
	selected := s.st.SettingString("ai.selected_model")
	installed := false
	if m, ok := ai.FindModel(selected); ok {
		installed = ai.IsInstalled(s.cfg.DataDir, m)
	}
	base, model, hasKey := s.engines.Remote.Config()
	_, haveBin := s.engines.Local.FindBinary()
	return map[string]any{
		"mode":          mode,
		"active_engine": s.engines.Active(mode),
		"local": map[string]any{"model": selected, "installed": installed, "binary_found": haveBin,
			"running": s.engines.Local.Running()},
		"remote": map[string]any{"configured": s.engines.Remote.Configured(), "needs_key": s.engines.Remote.NeedsKey(), "base_url": base, "model": model, "has_key": hasKey},
	}
}

func (s *Server) aiStatus(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, 200, s.aiStatusBody())
	return nil
}

func (s *Server) putAIMode(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Mode string `json:"mode"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := s.st.SetSetting("ai.mode", in.Mode); err != nil {
		return invalid("mode", "mode must be auto, local, remote or rules")
	}
	writeJSON(w, 200, s.aiStatusBody())
	return nil
}

func (s *Server) putAIRemote(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		BaseURL string `json:"base_url"`
		Model   string `json:"model"`
		APIKey  string `json:"api_key"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	in.BaseURL, in.Model = strings.TrimSpace(in.BaseURL), strings.TrimSpace(in.Model)
	if err := ai.ValidateBaseURL(in.BaseURL); err != nil {
		return invalid("base_url", "%s", err.Error())
	}
	if in.Model == "" {
		return invalid("model", "model is required")
	}
	if err := s.st.SetSetting("ai.remote.base_url", in.BaseURL); err != nil {
		return err
	}
	if err := s.st.SetSetting("ai.remote.model", in.Model); err != nil {
		return err
	}
	s.engines.Remote.Configure(in.BaseURL, in.Model, in.APIKey)
	writeJSON(w, 200, s.aiStatusBody())
	return nil
}

func (s *Server) deleteAIRemote(w http.ResponseWriter, r *http.Request) error {
	s.engines.Remote.Clear()
	_ = s.st.ClearSetting("ai.remote.base_url")
	_ = s.st.ClearSetting("ai.remote.model")
	return noContent(w)
}

func (s *Server) aiTest(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Engine string `json:"engine"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	var test func(context.Context) (string, error)
	switch in.Engine {
	case "local":
		test = s.engines.Local.Test
	case "remote":
		test = s.engines.Remote.Test
	default:
		return invalid("engine", "engine must be local or remote")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	start := time.Now()
	sample, err := test(ctx)
	out := map[string]any{"ok": err == nil, "latency_ms": time.Since(start).Milliseconds(), "sample": nil, "error": nil}
	if err != nil {
		out["error"] = err.Error()
	} else {
		out["sample"] = strings.TrimSpace(sample)
	}
	writeJSON(w, 200, out)
	return nil
}
