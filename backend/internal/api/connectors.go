package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
	"github.com/tlmcguire/fistbump/backend/internal/connectors/greenhouse"
	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/parser"
)

const notice = "Search covers a curated list of companies that publish jobs on Greenhouse, plus any boards you add. Many large employers use other systems: paste those postings instead."

func (s *Server) connectorEntry(id string) (map[string]any, error) {
	if id != "greenhouse" {
		return nil, notFound("connector")
	}
	return map[string]any{"id": "greenhouse", "name": "Greenhouse", "enabled": s.st.SettingBool("connectors.greenhouse.enabled")}, nil
}

func (s *Server) listConnectors(w http.ResponseWriter, r *http.Request) error {
	e, _ := s.connectorEntry("greenhouse")
	writeJSON(w, 200, []any{e})
	return nil
}

func (s *Server) putConnector(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.connectorEntry(r.PathValue("id")); err != nil {
		return err
	}
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Enabled == nil {
		return invalid("enabled", "enabled is required")
	}
	if err := s.st.SetSetting("connectors.greenhouse.enabled", *in.Enabled); err != nil {
		return err
	}
	e, _ := s.connectorEntry("greenhouse")
	writeJSON(w, 200, e)
	return nil
}

type fetchQuery struct {
	Boards     []string `json:"boards"`
	Categories []string `json:"categories"`
	Keywords   []string `json:"keywords"`
	Location   string   `json:"location"`
	WorkMode   string   `json:"work_mode"`
	Limit      int      `json:"limit"`
}

// resolveQuery turns a request query into a connector query. With no boards or categories given, it
// searches every curated board plus the user's custom boards: listings are cheap and cached, so the user
// does not have to pick boards. Keywords default to the profile's target positions, then the most recent
// job title on the resume.
func (s *Server) resolveQuery(q fetchQuery) (connectors.Query, []string, error) {
	out := connectors.Query{Location: strings.TrimSpace(q.Location), WorkMode: q.WorkMode, Limit: q.Limit}
	if out.WorkMode != "" && !slices.Contains([]string{"Remote", "Hybrid", "On-site", "Any"}, out.WorkMode) {
		return out, nil, invalid("work_mode", "work_mode is not valid")
	}
	if out.Limit < 0 || out.Limit > 500 {
		return out, nil, invalid("limit", "limit must be between 1 and 500")
	}
	for _, c := range q.Categories {
		if _, ok := greenhouse.FindCategory(c); !ok {
			return out, nil, invalid("categories", "unknown category %q", c)
		}
	}
	tokens := append([]string{}, q.Boards...)
	switch {
	case len(q.Boards) == 0 && len(q.Categories) == 0:
		tokens = append(tokens, s.st.SettingList("connectors.greenhouse.boards")...)
		tokens = append(tokens, greenhouse.Tokens()...)
	default:
		for _, b := range greenhouse.BoardsFor(q.Categories) {
			tokens = append(tokens, b.Token)
		}
		if len(q.Categories) > 0 {
			tokens = append(tokens, s.st.SettingList("connectors.greenhouse.boards")...)
		}
	}
	seen := map[string]bool{}
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if !greenhouse.ValidToken(t) {
			return out, nil, invalid("boards", "%q is not a valid board token", t)
		}
		if !seen[strings.ToLower(t)] {
			seen[strings.ToLower(t)] = true
			out.Boards = append(out.Boards, t)
		}
	}
	m, err := s.st.GetMaster()
	if err != nil {
		return out, nil, err
	}
	out.Keywords = cleanList(q.Keywords, false)
	if len(out.Keywords) == 0 && m.Profile != nil {
		out.Keywords = m.Profile.TargetPositions
	}
	if len(out.Keywords) == 0 && len(m.Experiences) > 0 {
		out.Keywords = []string{latestTitle(m.Experiences)}
	}
	if len(out.Keywords) == 0 {
		return out, nil, invalid("keywords", "enter a job title to search for, or add target positions to your resume")
	}
	if out.WorkMode == "" && m.Profile != nil && m.Profile.PreferredMode != nil {
		out.WorkMode = *m.Profile.PreferredMode
	}
	out.ResumeSkills = m.Skills()
	if m.Profile != nil && m.Profile.HomeLocation != nil {
		out.HomeLocation = *m.Profile.HomeLocation
	}
	if m.Profile != nil {
		out.ResumeSkills = append(out.ResumeSkills, m.Profile.ClearanceCerts...)
	}
	return out, out.Keywords, nil
}

// latestTitle returns the title of the current role, or of the role that ended last.
func latestTitle(exps []models.Experience) string {
	best, bestEnd := exps[0].JobTitle, ""
	for _, e := range exps {
		end := "9999"
		if e.EndDate != nil {
			end = *e.EndDate
		}
		if end > bestEnd {
			best, bestEnd = e.JobTitle, end
		}
	}
	return best
}

func (s *Server) requireEnabled(id string) error {
	if id != "greenhouse" {
		return notFound("connector")
	}
	if !s.st.SettingBool("connectors.greenhouse.enabled") {
		return conflict("the Greenhouse connector is turned off in settings")
	}
	return nil
}

func (s *Server) fetchConnector(w http.ResponseWriter, r *http.Request) error {
	if err := s.requireEnabled(r.PathValue("id")); err != nil {
		return err
	}
	var in struct {
		Query fetchQuery `json:"query"`
	}
	if err := decodeOptional(r, &in); err != nil {
		return err
	}
	cq, used, err := s.resolveQuery(in.Query)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	res, err := s.gh.FetchPostings(ctx, cq)
	if err != nil {
		return upstream("%v", err)
	}
	// Every board failing means Greenhouse (or the network) is down. Partial failures are listed in errors.
	if res.BoardsSearched == 0 && len(res.Errors) > 0 {
		return upstream("could not reach Greenhouse: %s", res.Errors[0].Message)
	}
	// Mark postings already saved (and archived) so the list can show them or hide archived ones.
	type marked struct {
		connectors.Posting
		SavedJobID *int64 `json:"saved_job_id"`
		Archived   bool   `json:"archived"`
	}
	ids := make([]string, len(res.Postings))
	for i, p := range res.Postings {
		ids[i] = p.ExternalID
	}
	saved, err := s.st.ConnectorJobs("greenhouse", ids)
	if err != nil {
		return err
	}
	postings := make([]marked, len(res.Postings))
	for i, p := range res.Postings {
		postings[i] = marked{Posting: p}
		if j, ok := saved[p.ExternalID]; ok {
			id := j.ID
			postings[i].SavedJobID, postings[i].Archived = &id, j.ArchivedAt != nil
		}
	}
	writeJSON(w, 200, map[string]any{
		"postings": postings, "boards_searched": res.BoardsSearched, "boards_total": res.BoardsTotal,
		"postings_scanned": res.PostingsScanned, "matched": res.Matched, "errors": res.Errors,
		"keywords_used": used, "work_mode_used": cq.WorkMode,
	})
	return nil
}

// importConnector stores postings from recent search results. Each posting's full text is fetched if the
// search did not already load it.
func (s *Server) importConnector(w http.ResponseWriter, r *http.Request) error {
	if err := s.requireEnabled(r.PathValue("id")); err != nil {
		return err
	}
	var in struct {
		Query       *fetchQuery `json:"query"` // accepted for compatibility, not needed
		ExternalIDs []string    `json:"external_ids"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if len(in.ExternalIDs) == 0 {
		return invalid("external_ids", "choose at least one posting")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	out := struct {
		Imported  []models.Job `json:"imported"`
		Refreshed []models.Job `json:"refreshed"`
	}{[]models.Job{}, []models.Job{}}
	for _, ext := range cleanList(in.ExternalIDs, false) {
		p, err := s.gh.Lookup(ctx, ext)
		if err != nil {
			return invalid("external_ids", "%s", err.Error())
		}
		raw, id, url := p.RawText, p.ExternalID, p.ListingURL
		j := models.Job{CompanyName: p.CompanyName, PositionTitle: p.PositionTitle, Source: "greenhouse", ExternalID: &id, RawText: &raw}
		if url != "" {
			j.ListingURL = &url
		}
		if p.Location != "" {
			l := p.Location
			j.Location = &l
		}
		parsed := parser.Parse(raw)
		applyParse(&j, parsed)
		mode := p.WorkMode
		if mode == "" {
			mode = parsed.Inferred.WorkMode
		}
		if mode != "" {
			j.WorkMode = &mode
		}
		saved, existed, err := s.st.UpsertConnectorJob(j)
		if err != nil {
			return err
		}
		if existed {
			out.Refreshed = append(out.Refreshed, *saved)
		} else {
			out.Imported = append(out.Imported, *saved)
		}
	}
	writeJSON(w, 200, out)
	return nil
}

// getPosting returns one posting from recent search results with its full text, for reading in the app.
func (s *Server) getPosting(w http.ResponseWriter, r *http.Request) error {
	if err := s.requireEnabled("greenhouse"); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	p, err := s.gh.Lookup(ctx, r.PathValue("external_id"))
	if err != nil {
		return notFound("posting (search again)")
	}
	writeJSON(w, 200, p)
	return nil
}

func (s *Server) categoryEntry(c greenhouse.Category, enabled []string) map[string]any {
	return map[string]any{"id": c.ID, "name": c.Name, "parent": c.Parent, "board_count": greenhouse.BoardCount(c.ID), "enabled": slices.Contains(enabled, c.ID)}
}

func (s *Server) listCategories(w http.ResponseWriter, r *http.Request) error {
	enabled := s.st.SettingList("connectors.greenhouse.categories")
	list := []map[string]any{}
	for _, c := range greenhouse.Categories() {
		list = append(list, s.categoryEntry(c, enabled))
	}
	writeJSON(w, 200, map[string]any{
		"categories": list, "verified_at": greenhouse.VerifiedAt(), "notice": notice,
		"custom_boards": s.st.SettingList("connectors.greenhouse.boards"),
	})
	return nil
}

func (s *Server) putCategory(w http.ResponseWriter, r *http.Request) error {
	c, ok := greenhouse.FindCategory(r.PathValue("category"))
	if !ok {
		return notFound("category")
	}
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Enabled == nil {
		return invalid("enabled", "enabled is required")
	}
	cur := s.st.SettingList("connectors.greenhouse.categories")
	next := slices.DeleteFunc(slices.Clone(cur), func(id string) bool { return id == c.ID })
	if *in.Enabled {
		next = append(next, c.ID)
	}
	if err := s.st.SetSetting("connectors.greenhouse.categories", toAny(next)); err != nil {
		return err
	}
	writeJSON(w, 200, s.categoryEntry(c, next))
	return nil
}

func toAny(in []string) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

func (s *Server) addBoard(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Board string `json:"board"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	token := strings.TrimSpace(in.Board)
	if !greenhouse.ValidToken(token) {
		return invalid("board", "board must be the token from boards.greenhouse.io/<token>")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	company, open, err := s.gh.ValidateBoard(ctx, token)
	if err != nil {
		if errors.Is(err, greenhouse.ErrNoBoard) {
			return invalid("board", "no Greenhouse board named %q", token)
		}
		return upstream("could not check the board: %v", err)
	}
	cur := s.st.SettingList("connectors.greenhouse.boards")
	if !slices.ContainsFunc(cur, func(b string) bool { return strings.EqualFold(b, token) }) {
		if err := s.st.SetSetting("connectors.greenhouse.boards", toAny(append(cur, token))); err != nil {
			return err
		}
	}
	writeJSON(w, 200, map[string]any{"board": token, "company": company, "open_postings": open})
	return nil
}

func (s *Server) removeBoard(w http.ResponseWriter, r *http.Request) error {
	token := r.PathValue("board")
	cur := s.st.SettingList("connectors.greenhouse.boards")
	next := slices.DeleteFunc(slices.Clone(cur), func(b string) bool { return strings.EqualFold(b, token) })
	if len(next) == len(cur) {
		return notFound("board")
	}
	if err := s.st.SetSetting("connectors.greenhouse.boards", toAny(next)); err != nil {
		return err
	}
	return noContent(w)
}
