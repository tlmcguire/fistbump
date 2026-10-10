package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/diff"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/pdf"
	"github.com/tlmcguire/fistbump/backend/internal/render"
)

// finalText is the text a suggestion contributes: edited, accepted or (for rejected) the original.
func finalText(g models.Suggestion) string {
	switch g.State {
	case "edited":
		if g.EditedText != nil {
			return *g.EditedText
		}
	case "accepted":
		return g.ProposedText
	}
	return g.OriginalText
}

func changes(sugs []models.Suggestion) []render.Change {
	var out []render.Change
	for _, g := range sugs {
		if g.State == "accepted" || g.State == "edited" {
			out = append(out, render.Change{Original: g.OriginalText, Final: finalText(g)})
		}
	}
	return out
}

func (s *Server) createTailored(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		RevisionID int64 `json:"revision_id"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	rev, err := s.st.GetRevision(in.RevisionID, true)
	if err != nil {
		return invalid("revision_id", "unknown revision_id")
	}
	if rev.Status != "done" {
		return conflict("revision is %s, not done", rev.Status).with("status", rev.Status)
	}
	pending := 0
	for _, g := range rev.Suggestions {
		if g.State == "pending" {
			pending++
		}
	}
	if pending > 0 {
		return conflict("%d suggestions are still pending", pending).with("pending", pending)
	}
	base, err := s.baseText(rev.BaseResumeID)
	if err != nil {
		return err
	}
	content := render.Apply(base, changes(rev.Suggestions))
	out, err := s.st.CreateTailored(rev.JobID, &rev.ID, rev.BaseResumeID, content)
	if err != nil {
		return err
	}
	writeJSON(w, 201, out)
	return nil
}

func (s *Server) listTailored(w http.ResponseWriter, r *http.Request) error {
	out, err := s.st.ListTailored(int64(queryInt(r, "job_id")))
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) getTailored(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	t, err := s.st.GetTailored(id)
	if err != nil {
		return fromStore(err, "tailored resume")
	}
	writeJSON(w, 200, t)
	return nil
}

func (s *Server) diffTailored(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	t, err := s.st.GetTailored(id)
	if err != nil {
		return fromStore(err, "tailored resume")
	}
	// The base is recovered by undoing the applied suggestions, so later edits to the master resume
	// do not show up as changes. Without a revision, fall back to the current base text.
	var hints []diff.Hint
	var base string
	if t.RevisionID != nil {
		sugs, err := s.st.ListSuggestions(*t.RevisionID)
		if err != nil {
			return err
		}
		for _, g := range sugs {
			if g.State == "accepted" || g.State == "edited" {
				hints = append(hints, diff.Hint{Section: g.Section, TargetID: g.TargetID, Original: g.OriginalText, Final: finalText(g)})
			}
		}
		base = render.Revert(*t.Content, changes(sugs))
	} else if base, err = s.baseText(t.BaseResumeID); err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"hunks": diff.Hunks(base, *t.Content, hints)})
	return nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func (s *Server) exportTailored(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
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
	t, err := s.st.GetTailored(id)
	if err != nil {
		return fromStore(err, "tailored resume")
	}
	name := "resume"
	if j, err := s.st.GetJob(t.JobID); err == nil {
		name = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(j.CompanyName+" "+j.PositionTitle), "-"), "-")
	}
	if name == "" {
		name = "resume"
	}
	var body []byte
	ctype := "text/markdown; charset=utf-8"
	if in.Format == "pdf" {
		if body, err = pdf.Render(*t.Content); err != nil {
			return errf(500, "internal", "PDF export failed, try md")
		}
		ctype = "application/pdf"
	} else {
		body = []byte(*t.Content)
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+"."+in.Format+`"`)
	_, _ = w.Write(body)
	return nil
}

func (s *Server) deleteTailored(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.st.DeleteTailored(id); err != nil {
		return fromStore(err, "tailored resume")
	}
	return noContent(w)
}
