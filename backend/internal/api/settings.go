package api

import (
	"encoding/json"
	"errors"
	"net/http"

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
func (s *Server) settingsPut(w http.ResponseWriter, r *http.Request) error {
	var body map[string]json.RawMessage
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	if err := s.store.PutSettings(r.Context(), body); err != nil {
		var bad *store.SettingError
		if errors.As(err, &bad) {
			return validationError(bad.Key, bad.Message)
		}
		return err
	}
	return s.settingsGet(w, r)
}
