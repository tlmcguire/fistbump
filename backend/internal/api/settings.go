package api

import (
	"net/http"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) error {
	m, err := s.st.Settings()
	if err != nil {
		return err
	}
	writeJSON(w, 200, m)
	return nil
}

func secretLooking(k string) bool {
	k = strings.ToLower(k)
	for _, w := range []string{"key", "token", "secret", "password"} {
		if strings.Contains(k, w) {
			return true
		}
	}
	return false
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) error {
	var in map[string]any
	if err := decode(r, &in); err != nil {
		return err
	}
	for k := range in {
		if secretLooking(k) {
			return invalid(k, "settings never hold secrets")
		}
		if _, ok := store.SettingDefs[k]; !ok {
			return invalid(k, "unknown setting %q", k)
		}
	}
	if v, ok := in["ai.selected_model"]; ok {
		if id, _ := v.(string); id != "" {
			if _, found := ai.FindModel(id); !found {
				return invalid("ai.selected_model", "unknown model")
			}
		}
	}
	// Validate every value before writing any, then write them together, so a bad value cannot
	// leave a partial update behind.
	for k, v := range in {
		if err := store.ValidateSetting(k, v); err != nil {
			return invalid(k, "%s %s", k, err.Error())
		}
	}
	if err := s.st.WithTx(func(tx *store.Store) error {
		for k, v := range in {
			if err := tx.SetSetting(k, v); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	// Apply remote settings changes to the live client without touching the key.
	if _, ok := in["ai.remote.base_url"]; ok {
		s.syncRemote()
	} else if _, ok := in["ai.remote.model"]; ok {
		s.syncRemote()
	}
	return s.getSettings(w, r)
}

func (s *Server) syncRemote() {
	base, model, _ := s.engines.Remote.Config()
	nb, nm := s.st.SettingString("ai.remote.base_url"), s.st.SettingString("ai.remote.model")
	if nb == base && nm == model {
		return
	}
	if nb != "" && ai.ValidateBaseURL(nb) != nil {
		return
	}
	s.engines.Remote.Configure(nb, nm, "")
}
