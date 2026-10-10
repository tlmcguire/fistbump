package api

import (
	"errors"
	"net/http"
	"os"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
)

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, 200, ai.List(s.cfg.DataDir, s.st.SettingString("ai.selected_model")))
	return nil
}

func (s *Server) findModel(r *http.Request) (ai.ModelInfo, error) {
	for _, m := range ai.List(s.cfg.DataDir, s.st.SettingString("ai.selected_model")) {
		if m.ID == r.PathValue("id") {
			return m, nil
		}
	}
	return ai.ModelInfo{}, notFound("model")
}

func (s *Server) getModel(w http.ResponseWriter, r *http.Request) error {
	m, err := s.findModel(r)
	if err != nil {
		return err
	}
	writeJSON(w, 200, m)
	return nil
}

func (s *Server) startDownload(w http.ResponseWriter, r *http.Request) error {
	m, err := s.findModel(r)
	if err != nil {
		return err
	}
	if m.Installed {
		return conflict("model is already installed")
	}
	id, err := s.downloads.Start(m.CatalogModel)
	if errors.Is(err, ai.ErrBusy) {
		return conflict("model is already downloading")
	}
	if err != nil {
		return err
	}
	writeJSON(w, 202, map[string]any{"download_id": id})
	return nil
}

func (s *Server) getDownload(w http.ResponseWriter, r *http.Request) error {
	st, ok := s.downloads.Status(r.PathValue("download_id"))
	if !ok {
		return notFound("download")
	}
	writeJSON(w, 200, st)
	return nil
}

func (s *Server) cancelDownload(w http.ResponseWriter, r *http.Request) error {
	if !s.downloads.Cancel(r.PathValue("download_id")) {
		return notFound("download")
	}
	return noContent(w)
}

func (s *Server) deleteModel(w http.ResponseWriter, r *http.Request) error {
	m, err := s.findModel(r)
	if err != nil {
		return err
	}
	s.engines.Local.StopIfModel(m.ID)
	if err := os.Remove(ai.ModelPath(s.cfg.DataDir, m.CatalogModel)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if m.Selected {
		if err := s.st.SetSetting("ai.selected_model", ""); err != nil {
			return err
		}
	}
	return noContent(w)
}

func (s *Server) putModelToken(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Token == "" {
		return invalid("token", "token is required")
	}
	s.downloads.SetToken(in.Token)
	return noContent(w)
}

func (s *Server) deleteModelToken(w http.ResponseWriter, r *http.Request) error {
	s.downloads.SetToken("")
	return noContent(w)
}
