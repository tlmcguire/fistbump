package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestCostCompareAPI(t *testing.T) {
	s := newTestServer(t, &fakeFetcher{})

	rec := apiJSON(t, s, http.MethodPost, "/v1/cost/compare", map[string]any{
		"offers": []map[string]any{
			{
				"label":         "Offer A (Austin)",
				"title":         "Software Engineer",
				"location":      "Austin, TX",
				"work_mode":     "Hybrid",
				"pay_min":       150000,
				"pay_max":       150000,
				"bonus":         15000,
				"annual_equity": 20000,
			},
			{
				"label":     "Offer B (San Francisco)",
				"title":     "Software Engineer",
				"location":  "San Francisco, CA",
				"work_mode": "Hybrid",
				"pay_min":   185000,
				"pay_max":   185000,
			},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("compare status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Offers []struct {
			Label     string `json:"label"`
			TotalComp int64  `json:"total_comp"`
		} `json:"offers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode compare: %v", err)
	}
	if len(out.Offers) != 2 || out.Offers[0].TotalComp != 185000 || out.Offers[1].TotalComp != 185000 {
		t.Fatalf("unexpected compare result: %+v", out)
	}
}
