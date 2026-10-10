package api

import (
	"encoding/base64"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/pdf"
	"github.com/tlmcguire/fistbump/backend/internal/render"
	"github.com/tlmcguire/fistbump/backend/internal/resumeparse"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

const maxImportBytes = 5 << 20

// importResume extracts text from an uploaded resume and returns a draft for review. Nothing is saved.
func (s *Server) importResume(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Filename   string `json:"filename"`
		DataBase64 string `json:"data_base64"`
		Text       string `json:"text"`
		Engine     string `json:"engine"` // rules (default) or auto
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Engine == "" {
		in.Engine = "rules"
	}
	if in.Engine != "rules" && in.Engine != "auto" {
		return invalid("engine", "engine must be rules or auto")
	}
	text := in.Text
	if in.DataBase64 != "" {
		data, err := base64.StdEncoding.DecodeString(in.DataBase64)
		if err != nil {
			return invalid("data_base64", "data_base64 is not valid base64")
		}
		if len(data) > maxImportBytes {
			return invalid("data_base64", "the file is larger than 5 MB")
		}
		switch strings.ToLower(filepath.Ext(in.Filename)) {
		case ".pdf":
			if text, err = resumeparse.TextFromPDF(data); err != nil {
				return invalid("data_base64", "%s", err.Error())
			}
		case ".txt", ".md", ".markdown", "":
			text = string(data)
		default:
			return invalid("filename", "use a PDF, .txt or .md file")
		}
	}
	if strings.TrimSpace(text) == "" {
		return invalid("text", "provide a file or paste your resume text")
	}
	draft := resumeparse.Parse(text)
	engine, notice := "rules", ""
	if in.Engine == "auto" {
		if d, used, err := s.engines.StructureResume(r.Context(), s.effectiveMode("auto"), text, draft); err == nil {
			draft, engine = d, used
		} else {
			notice = "The model could not structure this resume (" + err.Error() + "). Showing the rules-based result."
		}
	}
	writeJSON(w, 200, map[string]any{"text": text, "draft": draft, "engine": engine, "notice": notice})
	return nil
}

// applyImport writes a reviewed draft to the master resume. With replace, existing experience and
// education entries are removed first. Every entry is validated before anything is written.
func (s *Server) applyImport(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Draft struct {
			Profile     models.Profile      `json:"profile"`
			Experiences []models.Experience `json:"experiences"`
			Education   []models.Education  `json:"education"`
			Skills      []string            `json:"skills"`   // informational; skills live on experiences
			Warnings    []string            `json:"warnings"` // informational
		} `json:"draft"`
		Replace  bool `json:"replace"`
		SaveCopy *struct {
			Label string `json:"version_label"`
			Text  string `json:"content"`
		} `json:"save_copy"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	cur, err := s.st.GetProfile()
	if err != nil {
		return err
	}
	p := in.Draft.Profile
	if cur != nil { // keep what the draft leaves empty
		if strings.TrimSpace(p.FullName) == "" {
			p.FullName = cur.FullName
		}
		for _, pair := range [][2]**string{{&p.Email, &cur.Email}, {&p.Phone, &cur.Phone}, {&p.HomeLocation, &cur.HomeLocation}, {&p.Summary, &cur.Summary}, {&p.PreferredMode, &cur.PreferredMode}} {
			if *pair[0] == nil || strings.TrimSpace(**pair[0]) == "" {
				*pair[0] = *pair[1]
			}
		}
		if len(p.TargetPositions) == 0 {
			p.TargetPositions = cur.TargetPositions
		}
		if p.MinDesiredPay == nil {
			p.MinDesiredPay = cur.MinDesiredPay
		}
		p.ClearanceCerts = cleanList(append(cur.ClearanceCerts, p.ClearanceCerts...), false)
		if !in.Replace {
			p.Skills = append(append([]string{}, cur.Skills...), p.Skills...)
		}
		if len(p.Links) == 0 {
			p.Links = cur.Links
		}
	}
	p.FullName = strings.TrimSpace(p.FullName)
	if p.FullName == "" {
		return invalid("full_name", "enter your name before saving")
	}
	for _, f := range []**string{&p.Email, &p.Phone, &p.HomeLocation, &p.Summary} {
		blankToNil(f)
	}
	p.TargetPositions, p.ClearanceCerts, p.Skills, p.Links = cleanList(p.TargetPositions, false), cleanList(p.ClearanceCerts, false), cleanList(p.Skills, true), cleanList(p.Links, false)
	if p.PreferredMode != nil && !slices.Contains([]string{"Remote", "Hybrid", "On-site", "Any"}, *p.PreferredMode) {
		p.PreferredMode = nil
	}
	for i := range in.Draft.Experiences {
		if err := validateExperience(&in.Draft.Experiences[i]); err != nil {
			return err.(*apiErr).with("experience_index", i)
		}
	}
	for i := range in.Draft.Education {
		if err := validateEducation(&in.Draft.Education[i]); err != nil {
			return err.(*apiErr).with("education_index", i)
		}
	}
	// One transaction: a failure part way leaves the master resume exactly as it was.
	err = s.st.WithTx(func(tx *store.Store) error {
		if _, err := tx.UpsertProfile(p); err != nil {
			return err
		}
		if in.Replace {
			exps, err := tx.ListExperiences()
			if err != nil {
				return err
			}
			for _, e := range exps {
				if err := tx.DeleteExperience(e.ID); err != nil {
					return err
				}
			}
			edus, err := tx.ListEducation()
			if err != nil {
				return err
			}
			for _, e := range edus {
				if err := tx.DeleteEducation(e.ID); err != nil {
					return err
				}
			}
		}
		for _, e := range in.Draft.Experiences {
			e.SortOrder = 0
			if _, err := tx.CreateExperience(e); err != nil {
				return err
			}
		}
		for _, e := range in.Draft.Education {
			e.SortOrder = 0
			if _, err := tx.CreateEducation(e); err != nil {
				return err
			}
		}
		if in.SaveCopy != nil && strings.TrimSpace(in.SaveCopy.Text) != "" {
			label := strings.TrimSpace(in.SaveCopy.Label)
			if label == "" {
				label = "Imported resume"
			}
			if _, err := tx.CreateResume(label, nil, in.SaveCopy.Text); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.getResume(w, r)
}

func (s *Server) resumeSummary(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Engine string `json:"engine"`
	}
	if err := decodeOptional(r, &in); err != nil {
		return err
	}
	m, err := s.st.GetMaster()
	if err != nil {
		return err
	}
	if m.Profile == nil || len(m.Experiences) == 0 {
		return conflict("add your profile and at least one experience entry first")
	}
	mode := s.effectiveMode(in.Engine)
	text, engine, err := s.engines.Summary(r.Context(), mode, m, render.Master(m))
	if err != nil {
		return errf(503, "ai_unavailable", "%s", err.Error())
	}
	writeJSON(w, 200, map[string]any{"summary": text, "engine": engine})
	return nil
}

func (s *Server) resumePreview(w http.ResponseWriter, r *http.Request) error {
	m, err := s.st.GetMaster()
	if err != nil {
		return err
	}
	if m.Profile == nil {
		return conflict("create your profile first")
	}
	writeJSON(w, 200, map[string]any{"markdown": render.Master(m)})
	return nil
}

func (s *Server) exportMaster(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Format string `json:"format"`
	}
	if err := decodeOptional(r, &in); err != nil {
		return err
	}
	if in.Format == "" {
		in.Format = "pdf"
	}
	if in.Format != "pdf" && in.Format != "md" {
		return invalid("format", "format must be pdf or md")
	}
	m, err := s.st.GetMaster()
	if err != nil {
		return err
	}
	if m.Profile == nil {
		return conflict("create your profile first")
	}
	md := render.Master(m)
	name := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(m.Profile.FullName), "-"), "-")
	if name == "" {
		name = "resume"
	} else {
		name += "-resume"
	}
	body, ctype := []byte(md), "text/markdown; charset=utf-8"
	if in.Format == "pdf" {
		if body, err = pdf.Render(md); err != nil {
			return errf(500, "internal", "PDF export failed, try md")
		}
		ctype = "application/pdf"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+"."+in.Format+`"`)
	_, _ = w.Write(body)
	return nil
}
