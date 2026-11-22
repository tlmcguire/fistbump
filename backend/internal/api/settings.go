package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

func (s *Server) settingsGet(w http.ResponseWriter, r *http.Request) error {
	values, err := s.store.GetSettings(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, values)
	return nil
}

// settingsPut validates every value, writes them in one transaction and returns the full map.
// A selected model must be in the catalog and downloaded, and a remote base URL must pass the
// same check as PUT /v1/ai/remote.
func (s *Server) settingsPut(w http.ResponseWriter, r *http.Request) error {
	var body map[string]json.RawMessage
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	if err := store.ValidateSettings(body); err != nil {
		return settingError(err)
	}
	var selected, baseURL *string
	if raw, ok := body["ai.selected_model"]; ok {
		var id string
		json.Unmarshal(raw, &id) // validated above
		if id != "" {
			if _, known := ai.FindModel(id); !known {
				return validationError("ai.selected_model", "ai.selected_model is not a model in the catalog")
			}
			if !ai.Installed(s.dataDir, id) {
				return validationError("ai.selected_model", "Download this model before selecting it")
			}
		}
		selected = &id
	}
	if raw, ok := body["ai.remote.base_url"]; ok {
		var u string
		json.Unmarshal(raw, &u)
		if u != "" {
			clean, err := ai.ValidateBaseURL(u)
			if err != nil {
				return validationError("ai.remote.base_url", err.Error())
			}
			body["ai.remote.base_url"] = mustJSON(clean)
			u = clean
		}
		baseURL = &u
	}

	before, err := s.store.GetSettings(r.Context())
	if err != nil {
		return err
	}
	if err := s.store.PutSettings(r.Context(), body); err != nil {
		return settingError(err)
	}

	// A new model selection stops the server, so the next request loads the new model.
	if selected != nil && *selected != before["ai.selected_model"] && s.local != nil {
		s.local.Stop()
	}
	// Keep the remote client in step with its settings. Configure drops the key if the host changed.
	if _, ok := body["ai.remote.model"]; ok || baseURL != nil {
		after, err := s.store.GetSettings(r.Context())
		if err != nil {
			return err
		}
		u, _ := after["ai.remote.base_url"].(string)
		m, _ := after["ai.remote.model"].(string)
		if u == "" {
			s.remote.Clear()
		} else if err := s.remote.Configure(u, m, ""); err != nil {
			return validationError("ai.remote.base_url", err.Error())
		}
	}
	return s.settingsGet(w, r)
}

func settingError(err error) error {
	var bad *store.SettingError
	if errors.As(err, &bad) {
		return validationError(bad.Key, bad.Message)
	}
	return err
}
