package greenhouse

import "testing"

// TestLocationCities runs many requested places against postings with the same or a confusable
// name. score is 1 for the city, stateScore for a posting that only shares the state, 0 for no match.
func TestLocationCities(t *testing.T) {
	cases := []struct {
		want, posting string
		score         float64
	}{
		// Columbia, SC was matching "British Columbia".
		{"Columbia, SC", "Columbia, SC", 1},
		{"Columbia, SC", "Columbia, South Carolina", 1},
		{"Columbia, SC", "Columbia, SC or Remote", 1},
		{"Columbia, SC", "Charleston, SC", stateScore},
		{"Columbia, SC", "South Carolina", stateScore},
		{"Columbia, SC", "British Columbia, Canada", 0},
		{"Columbia, SC", "Vancouver, British Columbia", 0},
		{"Columbia, SC", "Columbia, MD", 0},
		{"Columbia, SC", "Columbia, Maryland", 0},
		{"Columbia, SC", "Columbia, MO", 0},
		{"Columbia, SC", "Washington, DC", 0},
		{"Columbia, SC", "District of Columbia", 0},
		{"Columbia, SC", "Columbia, Canada", 0},
		{"Columbia, SC", "Remote", 0},
		{"Columbia, South Carolina", "Columbia, SC", 1},
		// Same city name in another state.
		{"Charleston, SC", "Charleston, SC", 1},
		{"Charleston, SC", "Charleston, WV", 0},
		{"Charleston, SC", "Charleston, West Virginia", 0},
		{"Charleston, SC", "Greenville, SC", stateScore},
		{"Charleston, SC", "North Charleston, SC", 1},
		{"Charleston, WV", "Charleston, SC", 0},
		{"Portland, OR", "Portland, Oregon", 1},
		{"Portland, OR", "Portland, ME", 0},
		{"Portland, OR", "Portland, Maine", 0},
		{"Portland, OR", "Portland", 1},
		{"Portland, OR", "Salem, OR", stateScore},
		{"Springfield, IL", "Springfield, MA", 0},
		{"Springfield, IL", "Springfield, IL", 1},
		{"Kansas City, MO", "Kansas City, KS", 0},
		{"Kansas City, MO", "Kansas City, MO", 1},
		{"Kansas City, MO", "Kansas City, Kansas", 0},
		{"Austin, TX", "Austin, Texas", 1},
		{"Austin, TX", "Austin, MN", 0},
		{"Richmond, VA", "Richmond, British Columbia", 0},
		{"Richmond, VA", "Richmond, VA", 1},
		{"Richmond, VA", "Richmond, KY", 0},
		{"Birmingham, AL", "Birmingham, UK", 0},
		{"Birmingham, AL", "Birmingham, AL", 1},
		{"Portland, ME", "Portland, OR", 0},
		{"Columbus, OH", "Columbus, GA", 0},
		{"Columbus, OH", "Columbus, Ohio", 1},
		{"Athens, GA", "Athens, Greece", 0},
		{"Athens, GA", "Athens, GA", 1},
		{"Cambridge, MA", "Cambridge, UK", 0},
		{"Cambridge, MA", "Cambridge, MA", 1},
		{"Cambridge, MA", "Boston, MA", stateScore},
		{"Rochester, NY", "Rochester, MN", 0},
		{"Rochester, NY", "Rochester, New York", 1},
		{"Jackson, MS", "Jackson, WY", 0},
		{"Charlotte, NC", "Charlotte, NC", 1},
		{"Charlotte, NC", "Charlottesville, VA", 0}, // a longer name is another place
		{"Charlotte, NC", "Raleigh, NC", stateScore},
		{"Charlotte, NC", "Charleston, SC", 0},
		{"Lincoln, NE", "Lincolnshire, IL", 0},
		{"Phoenix, AZ", "Phoenix, AZ", 1},
		{"Denver, CO", "Denver, Colorado", 1},
		{"Denver, CO", "Denver, NC", 0},
		// A state or a bare city. A state asked for is matched in full.
		{"South Carolina", "Columbia, SC", 1},
		{"South Carolina", "Charleston, South Carolina", 1},
		{"South Carolina", "Charleston, WV", 0},
		{"Virginia", "Reston, VA", 1},
		{"Virginia", "Charleston, West Virginia", 0},
		{"Virginia", "Richmond, Virginia", 1},
		{"West Virginia", "Charleston, West Virginia", 1},
		{"Georgia", "Atlanta, GA", 1},
		{"Washington", "Seattle, WA", 1},
		{"Indiana", "Berlin, Germany", 0},
		{"Oregon", "Oakland, CA or Remote", 0},
		{"Texas", "Austin, TX", 1},
		{"Austin", "Austin, TX", 1},
		{"Denver", "Denver, CO", 1},
		{"Seattle", "Seattle, WA", 1},
		{"Seattle", "Remote", 0},
		// Places with other names.
		{"Washington, DC", "Washington, DC", 1},
		{"Washington, DC", "Washington, D.C.", 1},
		{"Washington, DC", "Washington DC Metro", 1},
		{"Washington, DC", "Seattle, WA", 0},
		{"Washington, DC", "Washington, WA", 0},
		{"NYC", "New York, NY", 1},
		{"New York", "Brooklyn, NY", stateScore},
		{"New York, NY", "New York City", 1},
		{"SF", "San Francisco, CA", 1},
		{"San Francisco, CA", "Oakland, CA", stateScore},
		{"Bay Area", "Palo Alto, CA", 1},
		{"Los Angeles, CA", "Los Angeles, CA", 1},
		{"Los Angeles, CA", "Los Angeles, Chile", 0},
		{"Boston, MA", "Boston, MA", 1},
		{"Miami, FL", "Miami, OH", 0},
		{"Salt Lake City, UT", "Salt Lake City, Utah", 1},
		{"Nashville, TN", "Memphis, TN", stateScore},
		{"Raleigh, NC", "Raleigh-Durham, NC", 1},
		{"Honolulu, HI", "Honolulu, Hawaii", 1},
		{"Madison, WI", "Madison, AL", 0},
		{"Berlin, NH", "Berlin, Germany", 0},
		{"Paris, TX", "Paris, France", 0},
		{"London, KY", "London, UK", 0},
		{"London", "London, UK", 1},
	}
	for _, c := range cases {
		terms := locationTerms(c.want)
		if got := LocationScore(c.posting, "On-site", terms); got != c.score {
			t.Errorf("%q vs %q: score %v, want %v", c.want, c.posting, got, c.score)
		}
		if ok := LocationOK(c.posting, "On-site", terms, "Any"); ok != (c.score > 0) {
			t.Errorf("%q vs %q: LocationOK = %v", c.want, c.posting, ok)
		}
	}
}

// A remote posting for another country is not for a Columbia, SC user, and a remote US one is.
func TestColumbiaRemote(t *testing.T) {
	terms := locationTerms("Columbia, SC")
	if LocationOK("Remote - Canada", "Remote", terms, "Any") || LocationOK("Remote - British Columbia", "Remote", terms, "Any") {
		t.Error("foreign remote passed")
	}
	if !LocationOK("Remote - US", "Remote", terms, "Any") || !LocationOK("Remote", "Remote", terms, "Any") {
		t.Error("US remote was dropped")
	}
}
