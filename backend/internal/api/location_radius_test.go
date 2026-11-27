package api

import (
	"reflect"
	"testing"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/connectors"
)

func TestLocationRadiusUsesResumeDefaultAndCustomCenter(t *testing.T) {
	s := newResumeServer(t)
	request := `{"radius_miles":20,"locations":["North Charleston, SC","Seattle, WA","Remote","Boston, MA; Summerville, SC"]}`
	expectField(t, call(t, s, "POST", "/v1/connectors/location-radius", request), "location")
	expect(t, call(t, s, "PUT", "/v1/resume/profile", `{"full_name":"Ada","home_location":"Charleston, SC"}`), 200)
	got := expect(t, call(t, s, "POST", "/v1/connectors/location-radius", request), 200)["matches"]
	if !reflect.DeepEqual(got, []any{true, false, false, true}) {
		t.Fatalf("matches from resume location = %v", got)
	}
	got = expect(t, call(t, s, "POST", "/v1/connectors/location-radius",
		`{"radius_miles":20,"location":"Seattle, Washington","locations":["North Charleston, SC","Seattle, WA"]}`), 200)["matches"]
	if !reflect.DeepEqual(got, []any{false, true}) {
		t.Fatalf("matches from override = %v", got)
	}
}

func TestATSSearchReceivesRadiusBeforePagination(t *testing.T) {
	searcher := &fakeSearcher{}
	s := newSearchServer(t, searcher)
	rec := postFetch(t, s, `{"query":{"radius_miles":25,"location":"Charleston, SC","boards":["sample"]}}`)
	if rec.Code != 200 {
		t.Fatal(rec.Body)
	}
	if q := searcher.gotQuery; q.Location != "" || q.WithinRadius == nil ||
		!q.WithinRadius("Summerville, SC") || q.WithinRadius("Columbia, SC") ||
		customPostingMatches(connectors.Posting{Location: "Columbia, SC", WorkMode: "Remote"}, q, "", time.Time{}) {
		t.Fatal("ATS matcher did not apply the selected radius")
	}
}
