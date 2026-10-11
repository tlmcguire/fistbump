package api

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/tlmcguire/fistbump/backend/internal/connectors/greenhouse"
)

// Board tokens are lowercase letters, digits, - and _. Job ids are digits. The length caps sit well
// above real values (the longest curated board is 23 characters, ids are about 10 digits) and only
// reject junk before it goes into an outbound URL.
var (
	boardPattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
	jobIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)
)

// maxLinkLen caps the pasted link before it is parsed.
const maxLinkLen = 2048

// validateIDs checks a board and job id. Both routes use it, including for the values ParseLink
// reads from a pasted link, so they accept exactly the same boards and jobs.
func validateIDs(board, jobID string) error {
	if !boardPattern.MatchString(board) {
		return validationFailed("Board token is not valid")
	}
	if !jobIDPattern.MatchString(jobID) {
		return validationFailed("Job id is not valid")
	}
	return nil
}

// greenhouseJob returns a listing the app already knows, by board and job id. It serves reloads
// and, later, re-pulling a saved job; a 404 means the posting has closed.
func (s *Server) greenhouseJob(w http.ResponseWriter, r *http.Request) error {
	board, jobID := r.PathValue("board"), r.PathValue("job_id")
	if err := validateIDs(board, jobID); err != nil {
		return err
	}
	return s.writePosting(w, r, board, jobID)
}

// greenhouseLink opens a pasted job link: it reads the board and job id from the link, then
// fetches the posting. This is how a listing first enters the app.
func (s *Server) greenhouseLink(w http.ResponseWriter, r *http.Request) error {
	raw := r.URL.Query().Get("url")
	if raw == "" {
		return validationFailed("Enter a Greenhouse job link")
	}
	if len(raw) > maxLinkLen {
		return validationFailed("The job link is too long")
	}

	// ParseLink's error text is written for the user (for example, the explanation for links
	// that carry only gh_jid), so it is passed through as the 422 message.
	board, jobID, err := greenhouse.ParseLink(raw)
	if err != nil {
		return validationFailed(err.Error())
	}
	if err := validateIDs(board, jobID); err != nil {
		return err
	}
	return s.writePosting(w, r, board, jobID)
}

// writePosting fetches one posting and writes it as JSON. Both routes share it so they cannot
// drift apart. The request's context cancels the Greenhouse call if the app stops waiting.
func (s *Server) writePosting(w http.ResponseWriter, r *http.Request, board, jobID string) error {
	p, err := s.gh.FetchJob(r.Context(), board, jobID)
	if err != nil {
		return greenhouseError(err)
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

// greenhouseError maps the Greenhouse client's errors to API errors. The messages are shown to
// the user. Any other error is returned unchanged and becomes a 500.
func greenhouseError(err error) error {
	switch {
	case errors.Is(err, greenhouse.ErrNotFound):
		return notFound("This job is closed or does not exist.")
	case errors.Is(err, greenhouse.ErrUpstream):
		return upstreamError("Greenhouse did not respond. Try again.")
	default:
		return err
	}
}
