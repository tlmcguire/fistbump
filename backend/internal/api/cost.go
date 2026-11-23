package api

import (
	"net/http"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/connectors/greenhouse"
	"github.com/tlmcguire/fistbump/backend/internal/cost"
)

// costAnalyze handles POST /v1/cost: how a posting's pay and benefits compare with the market and with the
// cost of living where the user would live. It takes the posting's fields, so it works for a saved job and
// for one still in the search results. The user's home location and minimum pay come from the profile.
func (s *Server) costAnalyze(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Title    string `json:"title"`
		Location string `json:"location"`
		WorkMode string `json:"work_mode"`
		PayMin   int64  `json:"pay_min"`
		PayMax   int64  `json:"pay_max"`
		Text     string `json:"text"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	if len(body.Title) > maxShortText || len(body.Location) > maxShortText {
		return validationError("title", "title or location is too long")
	}
	if body.PayMin < 0 || body.PayMax < 0 || body.PayMin > 100_000_000 || body.PayMax > 100_000_000 {
		return validationError("pay_min", "pay must be a yearly amount between 0 and 100000000")
	}
	in := cost.Input{Title: strings.TrimSpace(body.Title), Location: strings.TrimSpace(body.Location), WorkMode: body.WorkMode,
		PayMin: body.PayMin, PayMax: body.PayMax, Text: body.Text}
	if in.PayMin == 0 && in.PayMax == 0 {
		if lo, hi, ok := greenhouse.ParsePay(body.Text); ok {
			in.PayMin, in.PayMax = int64(lo), int64(hi)
		}
	}
	if m, err := s.store.GetMaster(r.Context()); err == nil && m.Profile != nil {
		if m.Profile.HomeLocation != nil {
			in.HomeLocation = *m.Profile.HomeLocation
		}
		if m.Profile.MinDesiredPay != nil {
			in.MinDesiredPay = *m.Profile.MinDesiredPay
		}
	}
	writeJSON(w, http.StatusOK, cost.Analyze(in))
	return nil
}
