package api

import (
	"errors"
	"net/http"
	"os"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
)

// modelEntry is one model in GET /v1/models.
type modelEntry struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Repo            string `json:"repo"`
	File            string `json:"file"`
	SizeBytes       int64  `json:"size_bytes"`
	MinRAMGB        int    `json:"min_ram_gb"`
	License         string `json:"license"`
	Installed       bool   `json:"installed"`
	FitsThisMachine bool   `json:"fits_this_machine"`
	Selected        bool   `json:"selected"`
	Downloading     bool   `json:"downloading"`
}

func (s *Server) selectedModel(r *http.Request) (string, error) {
	values, err := s.store.GetSettings(r.Context())
	if err != nil {
		return "", err
	}
	id, _ := values["ai.selected_model"].(string)
	return id, nil
}

func (s *Server) entry(m ai.Model, selected string) modelEntry {
	return modelEntry{
		ID: m.ID, Name: m.Name, Repo: m.Repo, File: m.File, SizeBytes: m.SizeBytes,
		MinRAMGB: m.MinRAMGB, License: m.License,
		Installed:       ai.Installed(s.dataDir, m.ID),
		FitsThisMachine: ai.FitsThisMachine(m),
		Selected:        m.ID == selected,
		Downloading:     s.downloads.Downloading(m.ID),
	}
}

func (s *Server) modelList(w http.ResponseWriter, r *http.Request) error {
	selected, err := s.selectedModel(r)
	if err != nil {
		return err
	}
	out := []modelEntry{}
	for _, m := range ai.Catalog() {
		out = append(out, s.entry(m, selected))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func pathModel(r *http.Request) (ai.Model, error) {
	m, ok := ai.FindModel(r.PathValue("id"))
	if !ok {
		return m, notFound("Model not found")
	}
	return m, nil
}

func (s *Server) modelGet(w http.ResponseWriter, r *http.Request) error {
	m, err := pathModel(r)
	if err != nil {
		return err
	}
	selected, err := s.selectedModel(r)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, s.entry(m, selected))
	return nil
}

// modelDownload handles POST /v1/models/{id}/download. One download runs at a time.
func (s *Server) modelDownload(w http.ResponseWriter, r *http.Request) error {
	m, err := pathModel(r)
	if err != nil {
		return err
	}
	id, err := s.downloads.Start(m)
	switch {
	case errors.Is(err, ai.ErrInstalled):
		return conflict("This model is already downloaded")
	case errors.Is(err, ai.ErrBusy):
		return conflict("Another model is downloading. Wait for it or cancel it first")
	case err != nil:
		return err
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"download_id": id})
	return nil
}

func (s *Server) downloadStatus(w http.ResponseWriter, r *http.Request) error {
	st, ok := s.downloads.Status(r.PathValue("download_id"))
	if !ok {
		return notFound("Download not found")
	}
	writeJSON(w, http.StatusOK, st)
	return nil
}

// downloadCancel stops a transfer and keeps the .part file so a later download resumes it.
func (s *Server) downloadCancel(w http.ResponseWriter, r *http.Request) error {
	if !s.downloads.Cancel(r.PathValue("download_id")) {
		return notFound("Download not found")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// modelDelete removes a downloaded model. The local server is stopped first if it has the model
// loaded, and the selection is cleared if this model was selected.
func (s *Server) modelDelete(w http.ResponseWriter, r *http.Request) error {
	m, err := pathModel(r)
	if err != nil {
		return err
	}
	if s.downloads.Downloading(m.ID) {
		return conflict("This model is downloading. Cancel the download first")
	}
	if !ai.Installed(s.dataDir, m.ID) {
		return notFound("This model is not downloaded")
	}
	if s.local != nil {
		s.local.StopIfLoaded(m.ID)
	}
	if err := os.Remove(m.Path(s.dataDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	selected, err := s.selectedModel(r)
	if err != nil {
		return err
	}
	if selected == m.ID {
		if err := s.putSetting(r.Context(), "ai.selected_model", ""); err != nil {
			return err
		}
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
