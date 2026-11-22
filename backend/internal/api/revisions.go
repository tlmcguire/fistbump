package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/render"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

// roundRunner runs suggestion rounds in the background, one at a time. A round that has to wait
// stays queued. Rounds use the server's base context, not the request's, so they outlive the
// request that started them and stop when the server shuts down.
type roundRunner struct {
	base    context.Context
	engines ai.Engines
	fail    func(id int64, err error) // marks a revision failed
	mu      sync.Mutex
	cancels map[int64]context.CancelFunc
	slot    chan struct{} // holds one token while a round runs
}

func newRoundRunner(base context.Context, engines ai.Engines, fail func(int64, error)) *roundRunner {
	return &roundRunner{
		base:    base,
		engines: engines,
		fail:    fail,
		cancels: map[int64]context.CancelFunc{},
		slot:    make(chan struct{}, 1),
	}
}

// start runs round in a goroutine once no other round is running. An error from round, or a
// panic inside it, marks the revision failed unless the round was canceled.
func (rr *roundRunner) start(id int64, round func(ctx context.Context) error) {
	ctx, cancel := context.WithCancel(rr.base)
	rr.mu.Lock()
	rr.cancels[id] = cancel
	rr.mu.Unlock()

	go func() {
		defer func() {
			if v := recover(); v != nil {
				slog.Error("suggestion round panicked", "revision", id, "panic", v)
				rr.fail(id, errors.New("internal error"))
			}
			rr.mu.Lock()
			delete(rr.cancels, id)
			rr.mu.Unlock()
			cancel()
		}()

		select {
		case rr.slot <- struct{}{}:
		case <-ctx.Done():
			return // canceled while waiting; the cancel route has already updated the status
		}
		defer func() { <-rr.slot }()

		if err := round(ctx); err != nil && ctx.Err() == nil {
			rr.fail(id, err)
		}
	}()
}

// cancel stops a queued or running round. It does nothing for a round that is not in progress.
func (rr *roundRunner) cancel(id int64) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if cancel, ok := rr.cancels[id]; ok {
		cancel()
	}
}

// failRound marks a revision failed. A revision already finished or canceled is left as it is.
func (s *Server) failRound(id int64, cause error) {
	err := s.store.FinishRevision(context.Background(), id, "failed", cause.Error())
	if err != nil && !errors.Is(err, store.ErrConflict) {
		slog.Error("marking revision failed", "revision", id, "error", err)
	}
}

// revisionDetail is a revision with its suggestions, which are always listed, even when empty.
type revisionDetail struct {
	models.Revision
	Suggestions []models.Suggestion `json:"suggestions"`
}

// suggestionID reads the numeric {sid} from the path; anything else is a 404.
func suggestionID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("sid"), 10, 64)
	if err != nil || id < 1 {
		return 0, notFound("Not found")
	}
	return id, nil
}

// jobIDQuery reads the required job_id query parameter.
func jobIDQuery(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.URL.Query().Get("job_id"), 10, 64)
	if err != nil || id < 1 {
		return 0, validationError("job_id", "job_id is required")
	}
	return id, nil
}

// revisionCreate handles POST /v1/revisions. It queues a round and returns at once; the app polls
// the revision until it is done.
func (s *Server) revisionCreate(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		JobID        int64  `json:"job_id"`
		BaseResumeID *int64 `json:"base_resume_id"`
		Engine       string `json:"engine"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	mode := req.Engine
	if mode == "" {
		// No engine named: use the mode chosen in Settings, auto by default.
		m, err := s.aiMode(r.Context())
		if err != nil {
			return err
		}
		mode = m
	}
	switch mode {
	case "auto", ai.EngineLocal, ai.EngineRemote, ai.EngineRules:
	default:
		return validationError("engine", "engine must be auto, local, remote or rules")
	}
	if req.JobID < 1 {
		return validationError("job_id", "job_id is required")
	}

	ctx := r.Context()
	if _, err := s.store.GetJob(ctx, req.JobID); err != nil {
		return storeError(err, "Job")
	}
	if _, err := s.store.GetProfile(ctx); errors.Is(err, store.ErrNotFound) {
		return conflict("Create a profile before tailoring a resume")
	} else if err != nil {
		return err
	}
	if req.BaseResumeID != nil {
		if _, err := s.store.GetResume(ctx, *req.BaseResumeID); errors.Is(err, store.ErrNotFound) {
			return validationError("base_resume_id", "base_resume_id does not match a saved resume")
		} else if err != nil {
			return err
		}
	}

	// The revision records the first engine that will be tried; it is updated when the round ends.
	order := s.rounds.engines.Order(mode)
	if len(order) == 0 {
		return unavailable(fmt.Sprintf("The %s engine is not available", mode))
	}
	rev, err := s.store.CreateRevision(ctx, req.JobID, req.BaseResumeID, order[0])
	if err != nil {
		return storeError(err, "Revision")
	}
	s.rounds.start(rev.ID, func(ctx context.Context) error { return s.runRound(ctx, rev, mode) })
	writeJSON(w, http.StatusAccepted, rev)
	return nil
}

// runRound writes the suggestions for one revision. It runs in the background.
func (s *Server) runRound(ctx context.Context, rev models.Revision, mode string) error {
	if err := s.store.StartRevision(ctx, rev.ID); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return nil // canceled while it waited
		}
		return err
	}
	job, err := s.store.GetJob(ctx, rev.JobID)
	if err != nil {
		return err
	}
	master, err := s.store.GetMaster(ctx)
	if err != nil {
		return err
	}

	drafts, engine, err := s.rounds.engines.Run(ctx, mode, ai.Request{
		Job:      job,
		Analysis: s.analyze(job, master),
		Profile:  master.Profile,
		Targets:  ai.BuildTargets(master),
		Resume:   render.Master(master),
	})
	if err != nil {
		return err
	}

	suggestions := make([]models.Suggestion, len(drafts))
	for i, d := range drafts {
		if d.Engine == "" {
			d.Engine = engine
		}
		suggestions[i] = models.Suggestion{
			Section:      d.Section,
			TargetID:     d.TargetID,
			Engine:       d.Engine,
			OriginalText: d.Original,
			ProposedText: d.Proposed,
		}
	}
	if err := s.store.CompleteRevision(ctx, rev.ID, engine, suggestions); errors.Is(err, store.ErrConflict) {
		return nil // canceled while it ran
	} else if err != nil {
		return err
	}
	return nil
}

// revisionList handles GET /v1/revisions?job_id=, newest first.
func (s *Server) revisionList(w http.ResponseWriter, r *http.Request) error {
	jobID, err := jobIDQuery(r)
	if err != nil {
		return err
	}
	list, err := s.store.ListRevisions(r.Context(), jobID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

// revisionGet handles GET /v1/revisions/{id}. The app polls it while a round runs.
func (s *Server) revisionGet(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	rev, err := s.store.GetRevision(r.Context(), id)
	if err != nil {
		return storeError(err, "Revision")
	}
	suggestions, err := s.store.ListSuggestions(r.Context(), id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, revisionDetail{Revision: rev, Suggestions: suggestions})
	return nil
}

// revisionCancel handles POST /v1/revisions/{id}/cancel.
func (s *Server) revisionCancel(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if _, err := s.store.GetRevision(r.Context(), id); err != nil {
		return storeError(err, "Revision")
	}
	if err := s.store.FinishRevision(r.Context(), id, "canceled", ""); errors.Is(err, store.ErrConflict) {
		return conflict("Only a queued or running revision can be canceled")
	} else if err != nil {
		return err
	}
	s.rounds.cancel(id)
	rev, err := s.store.GetRevision(r.Context(), id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, rev)
	return nil
}

// revisionDelete handles DELETE /v1/revisions/{id}, stopping its round first if one is running.
func (s *Server) revisionDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	s.rounds.cancel(id)
	if err := s.store.DeleteRevision(r.Context(), id); err != nil {
		return storeError(err, "Revision")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) suggestionAccept(w http.ResponseWriter, r *http.Request) error {
	return s.decideSuggestion(w, r, "accepted", nil)
}

func (s *Server) suggestionReject(w http.ResponseWriter, r *http.Request) error {
	return s.decideSuggestion(w, r, "rejected", nil)
}

// suggestionEdit handles the edit route, which carries the user's text.
func (s *Server) suggestionEdit(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		EditedText string `json:"edited_text"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.EditedText) == "" {
		return validationError("edited_text", "edited_text is required")
	}
	if len(req.EditedText) > maxLongText {
		return validationError("edited_text", "edited_text is too long")
	}
	return s.decideSuggestion(w, r, "edited", &req.EditedText)
}

// decideSuggestion records a decision on one suggestion. Decisions open once the round is done.
func (s *Server) decideSuggestion(w http.ResponseWriter, r *http.Request, state string, edited *string) error {
	revisionID, err := pathID(r)
	if err != nil {
		return err
	}
	id, err := suggestionID(r)
	if err != nil {
		return err
	}
	rev, err := s.store.GetRevision(r.Context(), revisionID)
	if err != nil {
		return storeError(err, "Revision")
	}
	if rev.Status != "done" {
		return conflict("Suggestions can be decided once the revision is done")
	}
	sg, err := s.store.DecideSuggestion(r.Context(), revisionID, id, state, edited)
	if err != nil {
		return storeError(err, "Suggestion")
	}
	writeJSON(w, http.StatusOK, sg)
	return nil
}
