package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
	"github.com/tlmcguire/fistbump/backend/internal/analyze"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/render"
)

// effectiveMode combines the per-request engine with the ai.mode setting.
func (s *Server) effectiveMode(requested string) string {
	if requested != "" && requested != "auto" {
		return requested
	}
	return s.st.SettingString("ai.mode")
}

func (s *Server) createRevision(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		JobID        int64  `json:"job_id"`
		BaseResumeID *int64 `json:"base_resume_id"`
		Engine       string `json:"engine"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	switch in.Engine {
	case "", "auto", "local", "remote", "rules":
	default:
		return invalid("engine", "engine must be auto, local, remote or rules")
	}
	if _, err := s.st.GetJob(in.JobID); err != nil {
		return invalid("job_id", "unknown job_id")
	}
	if in.BaseResumeID != nil {
		if _, err := s.st.GetResume(*in.BaseResumeID); err != nil {
			return invalid("base_resume_id", "unknown base_resume_id")
		}
	}
	if m, err := s.st.GetMaster(); err != nil {
		return err
	} else if m.Profile == nil {
		return conflict("create the master resume before generating revisions")
	}
	mode := s.effectiveMode(in.Engine)
	if mode != "auto" && !s.engines.CanRun(mode) {
		return errf(503, "ai_unavailable", "the %s engine is not available", mode)
	}
	engine := s.engines.Active(mode)
	rev, err := s.st.CreateRevision(in.JobID, in.BaseResumeID, engine)
	if err != nil {
		return fromStore(err, "job")
	}
	ctx, cancel := context.WithCancel(s.background)
	s.mu.Lock()
	s.running[rev.ID] = cancel
	s.mu.Unlock()
	s.wg.Add(1)
	go s.generate(ctx, rev.ID, mode)
	writeJSON(w, 202, rev)
	return nil
}

// generate runs in the background and writes suggestions, then the final status.
func (s *Server) generate(ctx context.Context, revID int64, mode string) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		if c, ok := s.running[revID]; ok {
			c()
			delete(s.running, revID)
		}
		s.mu.Unlock()
	}()
	fail := func(err error) {
		if ctx.Err() != nil {
			return
		}
		msg := err.Error()
		_ = s.st.SetRevisionStatus(revID, "failed", &msg, "")
	}
	rev, err := s.st.GetRevision(revID, false)
	if err != nil {
		return
	}
	_ = s.st.SetRevisionStatus(revID, "running", nil, "")
	job, err := s.st.GetJob(rev.JobID)
	if err != nil {
		fail(err)
		return
	}
	master, err := s.st.GetMaster()
	if err != nil {
		fail(err)
		return
	}
	targets := ai.BuildTargets(master)
	if rev.BaseResumeID != nil {
		base, err := s.st.GetResume(*rev.BaseResumeID)
		if err != nil {
			fail(err)
			return
		}
		targets = filterTargets(targets, *base.Content)
	}
	req := ai.Request{Job: *job, Analysis: analyze.Run(*job, master), Profile: master.Profile, Targets: targets}
	drafts, engine, err := s.engines.Run(ctx, mode, req)
	if err != nil {
		fail(err)
		return
	}
	// Do not overwrite a cancel that landed while the engine ran.
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, err := s.st.GetRevision(revID, false); err != nil || cur.Status == "canceled" {
		return
	}
	_ = s.st.ClearSuggestions(revID)
	for _, d := range drafts {
		_ = s.st.AddSuggestion(models.Suggestion{RevisionID: revID, Section: d.Section, TargetID: d.TargetID, OriginalText: d.Original, ProposedText: d.Proposed})
	}
	_ = s.st.SetRevisionStatus(revID, "done", nil, engine)
}

// filterTargets keeps the summary and any target whose original text appears in the base resume.
func filterTargets(ts []ai.Target, base string) []ai.Target {
	out := []ai.Target{}
	for _, t := range ts {
		if t.Original == "" || strings.Contains(base, t.Original) {
			out = append(out, t)
		}
	}
	return out
}

func (s *Server) listRevisions(w http.ResponseWriter, r *http.Request) error {
	out, err := s.st.ListRevisions(int64(queryInt(r, "job_id")))
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) getRevision(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	rev, err := s.st.GetRevision(id, true)
	if err != nil {
		return fromStore(err, "revision")
	}
	// Always include the list, even when empty, so clients can tell "no suggestions" from "not loaded".
	type withSugs struct {
		*models.Revision
		Suggestions []models.Suggestion `json:"suggestions"`
	}
	sugs := rev.Suggestions
	if sugs == nil {
		sugs = []models.Suggestion{}
	}
	writeJSON(w, 200, withSugs{rev, sugs})
	return nil
}

func (s *Server) cancelRevision(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rev, err := s.st.GetRevision(id, false)
	if err != nil {
		return fromStore(err, "revision")
	}
	if rev.Status != "queued" && rev.Status != "running" {
		return conflict("revision is %s and cannot be canceled", rev.Status)
	}
	if err := s.st.SetRevisionStatus(id, "canceled", nil, ""); err != nil {
		return err
	}
	if c, ok := s.running[id]; ok {
		c()
	}
	rev, _ = s.st.GetRevision(id, false)
	writeJSON(w, 200, rev)
	return nil
}

func (s *Server) deleteRevision(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	s.mu.Lock()
	if c, ok := s.running[id]; ok {
		c()
	}
	s.mu.Unlock()
	if err := s.st.DeleteRevision(id); err != nil {
		return fromStore(err, "revision")
	}
	return noContent(w)
}

func (s *Server) suggestionState(state string) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		rid, err := pathID(r, "id")
		if err != nil {
			return err
		}
		sid, err := pathID(r, "sid")
		if err != nil {
			return err
		}
		out, err := s.st.SetSuggestionState(rid, sid, state, nil)
		if err != nil {
			return fromStore(err, "suggestion")
		}
		writeJSON(w, 200, out)
		return nil
	}
}

func (s *Server) editSuggestion(w http.ResponseWriter, r *http.Request) error {
	rid, err := pathID(r, "id")
	if err != nil {
		return err
	}
	sid, err := pathID(r, "sid")
	if err != nil {
		return err
	}
	var in struct {
		EditedText string `json:"edited_text"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.EditedText) == "" {
		return invalid("edited_text", "edited_text must not be empty")
	}
	out, err := s.st.SetSuggestionState(rid, sid, "edited", &in.EditedText)
	if err != nil {
		return fromStore(err, "suggestion")
	}
	writeJSON(w, 200, out)
	return nil
}

// baseText returns the text a revision or tailored resume starts from: the saved resume or the rendered master.
func (s *Server) baseText(baseID *int64) (string, error) {
	if baseID != nil {
		res, err := s.st.GetResume(*baseID)
		if err != nil {
			return "", fromStore(err, "base resume")
		}
		return *res.Content, nil
	}
	m, err := s.st.GetMaster()
	if err != nil {
		return "", err
	}
	return render.Master(m), nil
}
