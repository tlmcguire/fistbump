package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCostAnalyze(t *testing.T) {
	s := newTestServer(t, &fakeFetcher{})
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/v1/cost", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		return rec
	}
	rec := post(`{"title":"Software Engineer","location":"Austin, TX","work_mode":"On-site","text":"Salary: $120,000 - $150,000. We offer medical, dental and a 401(k) match."}`)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Supported bool `json:"supported"`
		Pay       struct {
			Listed bool  `json:"listed"`
			Min    int64 `json:"min"`
		} `json:"pay"`
		Lifestyle struct {
			Verdict string `json:"verdict"`
		} `json:"lifestyle"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Supported || !got.Pay.Listed || got.Pay.Min != 120000 || got.Lifestyle.Verdict == "" {
		t.Errorf("unexpected: %s", rec.Body)
	}
	if rec := post(`{"title":"x","bogus":1}`); rec.Code != 422 {
		t.Errorf("unknown field status %d", rec.Code)
	}
}
