package api

import (
	"net/http"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/connectors/greenhouse"
	"github.com/tlmcguire/fistbump/backend/internal/cost"
)

// costCompare handles POST /v1/cost/compare: side-by-side compensation & cost-of-living comparison
// for 2 to 5 job offers, incorporating optional annual bonus and equity.
func (s *Server) costCompare(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Offers []struct {
			Label        string `json:"label"`
			Title        string `json:"title"`
			Location     string `json:"location"`
			WorkMode     string `json:"work_mode"`
			PayMin       int64  `json:"pay_min"`
			PayMax       int64  `json:"pay_max"`
			Bonus        int64  `json:"bonus"`
			AnnualEquity int64  `json:"annual_equity"`
			Text         string `json:"text"`
		} `json:"offers"`
	}
	if err := decodeJSONLimit(w, r, &body, maxFetchBody); err != nil {
		return err
	}
	if len(body.Offers) < 2 || len(body.Offers) > 5 {
		return validationError("offers", "Provide 2 to 5 offers to compare")
	}

	home, desired := "", int64(0)
	if m, err := s.store.GetMaster(r.Context()); err == nil && m.Profile != nil {
		if m.Profile.HomeLocation != nil {
			home = *m.Profile.HomeLocation
		}
		if m.Profile.MinDesiredPay != nil {
			desired = *m.Profile.MinDesiredPay
		}
	}

	type comparisonItem struct {
		Label        string        `json:"label"`
		Bonus        int64         `json:"bonus"`
		AnnualEquity int64         `json:"annual_equity"`
		TotalComp    int64         `json:"total_comp"`
		Analysis     cost.Analysis `json:"analysis"`
	}
	results := make([]comparisonItem, 0, len(body.Offers))
	for _, o := range body.Offers {
		if o.PayMin < 0 || o.PayMax < 0 || o.Bonus < 0 || o.AnnualEquity < 0 {
			return validationError("offers", "Compensation amounts cannot be negative")
		}
		payMin, payMax := o.PayMin, o.PayMax
		if payMin == 0 && payMax == 0 && o.Text != "" {
			if lo, hi, ok := greenhouse.ParsePay(o.Text); ok {
				payMin, payMax = int64(lo), int64(hi)
			}
		}
		extra := o.Bonus + o.AnnualEquity
		in := cost.Input{
			Title:         strings.TrimSpace(o.Title),
			Location:      strings.TrimSpace(o.Location),
			WorkMode:      o.WorkMode,
			PayMin:        payMin + extra,
			PayMax:        max(payMax, payMin) + extra,
			Text:          o.Text,
			HomeLocation:  home,
			MinDesiredPay: desired,
		}
		analysis := cost.Analyze(in)
		total := max(payMax, payMin) + extra
		if payMin > 0 && payMax > 0 {
			total = (payMin+payMax)/2 + extra
		}
		label := strings.TrimSpace(o.Label)
		if label == "" {
			label = strings.TrimSpace(o.Title)
		}
		results = append(results, comparisonItem{
			Label:        label,
			Bonus:        o.Bonus,
			AnnualEquity: o.AnnualEquity,
			TotalComp:    total,
			Analysis:     analysis,
		})
	}
	writeJSON(w, http.StatusOK, struct {
		Offers []comparisonItem `json:"offers"`
	}{Offers: results})
	return nil
}
