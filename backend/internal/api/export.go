package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/tlmcguire/fistbump/backend/internal/docx"
	"github.com/tlmcguire/fistbump/backend/internal/pdf"
	"github.com/tlmcguire/fistbump/backend/internal/render"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

// maxFileNameRunes caps the name part of an export file name, before the extension.
const maxFileNameRunes = 120

// exportFormats maps each accepted format to the content type and file extension it is sent with.
var exportFormats = map[string]struct{ contentType, ext string }{
	"pdf":  {"application/pdf", "pdf"},
	"docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx"},
	"md":   {"text/markdown; charset=utf-8", "md"},
}

// exportRequest is the body of both export routes.
type exportRequest struct {
	Format string `json:"format"`
	Style  string `json:"style"`
}

// resumeExport handles POST /v1/resume/export: the master resume as a PDF or Markdown file.
func (s *Server) resumeExport(w http.ResponseWriter, r *http.Request) error {
	format, style, err := readFormatAndStyle(w, r)
	if err != nil {
		return err
	}
	master, err := s.store.GetMaster(r.Context())
	if err != nil {
		return err
	}
	if master.Profile == nil {
		return notFound("Create a profile before exporting a resume")
	}
	return s.sendExport(w, format, style, render.Master(master), fileName(master.Profile.FullName, "Resume"))
}

// resumeAudit handles GET /v1/resume/audit?style=...: deterministic 1-page PDF fit and ATS checks.
func (s *Server) resumeAudit(w http.ResponseWriter, r *http.Request) error {
	style := strings.TrimSpace(r.URL.Query().Get("style"))
	if !pdf.ValidStyle(style) {
		return validationError("style", "style must be classic, modern, minimal, compact, or executive")
	}
	if style == "" {
		style = pdf.StyleClassic
	}
	master, err := s.store.GetMaster(r.Context())
	if err != nil {
		return err
	}
	if master.Profile == nil {
		writeJSON(w, http.StatusOK, pdf.Audit("", style))
		return nil
	}
	writeJSON(w, http.StatusOK, pdf.Audit(render.Master(master), style))
	return nil
}

// tailoredExport handles POST /v1/tailored-resumes/{id}/export.
func (s *Server) tailoredExport(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	format, style, err := readFormatAndStyle(w, r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	tailored, err := s.store.GetTailored(ctx, id)
	if err != nil {
		return storeError(err, "Tailored resume")
	}
	job, err := s.store.GetJob(ctx, tailored.JobID)
	if err != nil {
		return storeError(err, "Job")
	}

	// The profile is missing only if it was never saved; the file is still worth sending.
	name := ""
	if profile, err := s.store.GetProfile(ctx); err == nil {
		name = profile.FullName
	} else if err != store.ErrNotFound {
		return err
	}
	return s.sendExport(w, format, style, tailored.Content, fileName(name, job.CompanyName, job.PositionTitle))
}

// readFormatAndStyle reads and checks the "format" and optional "style" of an export request.
func readFormatAndStyle(w http.ResponseWriter, r *http.Request) (string, string, error) {
	var req exportRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return "", "", err
	}
	if _, ok := exportFormats[req.Format]; !ok {
		return "", "", validationError("format", "format must be pdf, docx or md")
	}
	style := strings.TrimSpace(req.Style)
	if !pdf.ValidStyle(style) {
		return "", "", validationError("style", "style must be classic, modern, minimal, compact, or executive")
	}
	if style == "" {
		style = pdf.StyleClassic
	}
	return req.Format, style, nil
}

// sendExport writes markdown as a file download in the given format. Go only returns the bytes;
// the Tauri host asks the user where to save them.
func (s *Server) sendExport(w http.ResponseWriter, format, style, markdown, name string) error {
	body := []byte(markdown)
	switch format {
	case "pdf":
		pdfBytes, err := s.makePDF(markdown, name+"."+exportFormats[format].ext, style)
		if err != nil {
			slog.Error("pdf export failed", "error", err)
			return pdfFailed()
		}
		body = pdfBytes
	case "docx":
		docxBytes, err := docx.RenderStyled(markdown, style)
		if err != nil {
			slog.Error("docx export failed", "error", err)
			return internal()
		}
		body = docxBytes
	}

	kind := exportFormats[format]
	h := w.Header()
	h.Set("Content-Type", kind.contentType)
	h.Set("Content-Disposition", contentDisposition(name+"."+kind.ext))
	h.Set("Content-Length", strconv.Itoa(len(body)))
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		slog.Error("writing export", "error", err)
	}
	return nil
}

// makePDF runs the PDF renderer. A renderer that is missing, fails or panics is an error, so the
// caller can offer Markdown instead.
func (s *Server) makePDF(markdown, fileName, style string) (out []byte, err error) {
	if s.renderPDFStyled == nil && s.renderPDF == nil {
		return nil, fmt.Errorf("no PDF renderer is configured")
	}
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("pdf renderer panicked: %v", v)
		}
	}()
	if s.renderPDFStyled != nil {
		return s.renderPDFStyled(markdown, fileName, style)
	}
	return s.renderPDF(markdown, fileName)
}

// pdfFailed is the 500 for a failed PDF. details.fallback tells the app to offer Markdown.
func pdfFailed() *apiError {
	e := internal()
	e.Message = "The PDF could not be made"
	e.Details = map[string]any{"fallback": "md"}
	return e
}

// fileName joins the non-empty parts as "Ada Lovelace - Acme - Data Analyst", made safe for any
// platform. It never returns an empty name.
func fileName(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p = safeFileName(p); p != "" {
			kept = append(kept, p)
		}
	}
	name := cutRunes(strings.Join(kept, " - "), maxFileNameRunes)
	// Cutting can leave a dangling space or dash, and Windows rejects a trailing dot or space.
	if name = strings.TrimRight(name, " .-"); name == "" {
		return "Resume"
	}
	return name
}

// safeFileName removes the characters that file systems on any platform refuse, along with
// control characters, and collapses runs of spaces.
func safeFileName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func cutRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// contentDisposition builds an attachment header with a plain ASCII filename for old clients and
// the exact UTF-8 name in filename* (RFC 6266, RFC 5987).
func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, percentEncode(name))
}

// percentEncode escapes every byte that RFC 5987 does not allow in an extended value.
func percentEncode(s string) string {
	const allowed = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$&+-.^_`|~"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(allowed, s[i]) >= 0 {
			b.WriteByte(s[i])
		} else {
			fmt.Fprintf(&b, "%%%02X", s[i])
		}
	}
	return b.String()
}
