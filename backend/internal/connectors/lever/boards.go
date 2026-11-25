package lever

import (
	"sort"
	"strings"
)

type Board struct {
	Token   string `json:"token"`
	Company string `json:"company"`
}

type Category struct {
	ID     string  `json:"id"`
	Parent *string `json:"parent,omitempty"`
	Boards []Board `json:"boards"`
}

func ptr(s string) *string { return &s }

var catalog = []Category{
	{
		ID: "ai_ml", Parent: ptr("technical"),
		Boards: []Board{
			{"palantir", "Palantir"}, {"mistral", "Mistral AI"}, {"snorkelai", "Snorkel AI"},
			{"labelbox", "Labelbox"}, {"weightsandbiases", "Weights & Biases"}, {"reka", "Reka AI"},
			{"modular", "Modular"}, {"anyscale", "Anyscale"},
		},
	},
	{
		ID: "data_infrastructure", Parent: ptr("technical"),
		Boards: []Board{
			{"imply", "Imply"}, {"starburst", "Starburst"}, {"dremio", "Dremio"},
			{"matillion", "Matillion"}, {"tecton", "Tecton"}, {"montecarlodata", "Monte Carlo"},
		},
	},
	{
		ID: "developer_tools", Parent: ptr("technical"),
		Boards: []Board{
			{"postman", "Postman"}, {"netlify", "Netlify"}, {"circleci", "CircleCI"},
			{"launchdarkly", "LaunchDarkly"}, {"pagerduty", "PagerDuty"}, {"sourcegraph", "Sourcegraph"},
			{"hasura", "Hasura"}, {"kong", "Kong"}, {"prisma", "Prisma"},
		},
	},
	{
		ID: "cloud_devops", Parent: ptr("technical"),
		Boards: []Board{
			{"grafana", "Grafana Labs"}, {"gitpod", "Gitpod"}, {"harness", "Harness"},
			{"chronosphere", "Chronosphere"}, {"honeycomb", "Honeycomb"},
		},
	},
	{
		ID: "design_creative", Parent: ptr("technical"),
		Boards: []Board{
			{"canva", "Canva"}, {"webflow", "Webflow"}, {"mural", "Mural"},
			{"lucid", "Lucid Software"}, {"typeform", "Typeform"},
		},
	},
	{
		ID: "enterprise_software", Parent: ptr("technical"),
		Boards: []Board{
			{"atlassian", "Atlassian"}, {"box", "Box"}, {"docusign", "DocuSign"},
			{"outreach", "Outreach"}, {"highspot", "Highspot"}, {"seismic", "Seismic"},
			{"clari", "Clari"}, {"gainsight", "Gainsight"}, {"pendo", "Pendo"},
			{"amplitude", "Amplitude"}, {"mixpanel", "Mixpanel"}, {"braze", "Braze"},
			{"klaviyo", "Klaviyo"}, {"iterable", "Iterable"},
		},
	},
	{
		ID: "finance",
		Boards: []Board{
			{"wealthfront", "Wealthfront"}, {"betterment", "Betterment"}, {"klarna", "Klarna"},
			{"marqeta", "Marqeta"}, {"flywire", "Flywire"}, {"payoneer", "Payoneer"},
			{"navan", "Navan"}, {"expensify", "Expensify"}, {"justworks", "Justworks"},
			{"carta", "Carta"}, {"dwolla", "Dwolla"}, {"unit", "Unit"}, {"synctera", "Synctera"},
		},
	},
	{
		ID: "accounting_compliance",
		Boards: []Board{
			{"pilot", "Pilot"}, {"bench", "Bench"}, {"avalara", "Avalara"}, {"taxbit", "TaxBit"},
		},
	},
	{
		ID: "cybersecurity",
		Boards: []Board{
			{"snyk", "Snyk"}, {"veracode", "Veracode"}, {"recordedfuture", "Recorded Future"},
			{"flashpoint", "Flashpoint"}, {"arcticwolf", "Arctic Wolf"}, {"expel", "Expel"},
			{"huntress", "Huntress"}, {"redcanary", "Red Canary"}, {"dragos", "Dragos"},
			{"claroty", "Claroty"}, {"armis", "Armis"},
		},
	},
	{
		ID: "medical",
		Boards: []Board{
			{"color", "Color Health"}, {"ro", "Ro"}, {"noom", "Noom"},
			{"omada", "Omada Health"}, {"zocdoc", "Zocdoc"}, {"carbonhealth", "Carbon Health"},
			{"cityblock", "Cityblock Health"}, {"komodohealth", "Komodo Health"},
		},
	},
	{
		ID: "pharma_biotech",
		Boards: []Board{
			{"veeva", "Veeva Systems"}, {"twistbioscience", "Twist Bioscience"},
			{"synthego", "Synthego"}, {"insitro", "insitro"},
		},
	},
	{
		ID: "consumer_internet", Parent: ptr("technical"),
		Boards: []Board{
			{"netflix", "Netflix"}, {"spotify", "Spotify"}, {"eventbrite", "Eventbrite"},
			{"rover", "Rover"}, {"thumbtack", "Thumbtack"}, {"taskrabbit", "TaskRabbit"},
		},
	},
	{
		ID: "gaming", Parent: ptr("technical"),
		Boards: []Board{
			{"niantic", "Niantic"}, {"scopely", "Scopely"}, {"jamcity", "Jam City"}, {"bungie", "Bungie"},
		},
	},
	{
		ID: "retail_ecommerce",
		Boards: []Board{
			{"poshmark", "Poshmark"}, {"thredup", "thredUP"}, {"therealreal", "The RealReal"},
			{"stockx", "StockX"}, {"goat", "GOAT Group"},
		},
	},
	{
		ID: "consumer_brands",
		Boards: []Board{
			{"warbyparker", "Warby Parker"}, {"allbirds", "Allbirds"}, {"glossier", "Glossier"},
			{"brooklinen", "Brooklinen"}, {"away", "Away"},
		},
	},
	{
		ID: "transportation_logistics",
		Boards: []Board{
			{"lime", "Lime"}, {"turo", "Turo"}, {"shipbob", "ShipBob"}, {"shippo", "Shippo"},
		},
	},
	{
		ID: "supply_chain",
		Boards: []Board{
			{"project44", "project44"}, {"fourkites", "FourKites"}, {"coupa", "Coupa"},
		},
	},
	{
		ID: "crypto_web3",
		Boards: []Board{
			{"kraken", "Kraken"}, {"circle", "Circle"}, {"chainalysis", "Chainalysis"},
			{"fireblocks", "Fireblocks"}, {"anchorage", "Anchorage Digital"}, {"bitgo", "BitGo"},
			{"ledger", "Ledger"}, {"offchainlabs", "Offchain Labs"},
		},
	},
	{
		ID: "energy_climate",
		Boards: []Board{
			{"redwoodmaterials", "Redwood Materials"}, {"sila", "Sila Nanotechnologies"},
			{"palmetto", "Palmetto"}, {"arcadia", "Arcadia"},
		},
	},
	{
		ID: "autonomous_vehicles", Parent: ptr("technical"),
		Boards: []Board{
			{"zoox", "Zoox"}, {"nuro", "Nuro"}, {"joby", "Joby Aviation"}, {"archer", "Archer Aviation"},
		},
	},
	{
		ID: "legal_tech",
		Boards: []Board{
			{"clio", "Clio"}, {"everlaw", "Everlaw"}, {"linksquares", "LinkSquares"},
		},
	},
	{
		ID: "education",
		Boards: []Board{
			{"coursera", "Coursera"}, {"udemy", "Udemy"}, {"masterclass", "MasterClass"},
		},
	},
	{
		ID: "sports_fitness",
		Boards: []Board{
			{"peloton", "Peloton"}, {"whoop", "WHOOP"}, {"zwift", "Zwift"},
			{"calm", "Calm"}, {"headspace", "Headspace"},
		},
	},
	{
		ID: "hospitality_events",
		Boards: []Board{
			{"seatgeek", "SeatGeek"}, {"fever", "Fever"}, {"sonder", "Sonder"},
		},
	},
}

// BoardsFor returns the de-duplicated Lever boards for the given category IDs.
func BoardsFor(ids []string) []Board {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	seen := map[string]bool{}
	var out []Board
	for _, c := range catalog {
		in := want[c.ID] || (c.Parent != nil && want[*c.Parent])
		if !in {
			continue
		}
		for _, b := range c.Boards {
			key := strings.ToLower(b.Token)
			if !seen[key] {
				seen[key] = true
				out = append(out, b)
			}
		}
	}
	return out
}

// BoardCount returns the unique Lever board count for a category ID.
func BoardCount(id string) int { return len(BoardsFor([]string{id})) }

// Tokens returns every curated Lever board token, sorted.
func Tokens() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range catalog {
		for _, b := range c.Boards {
			key := strings.ToLower(b.Token)
			if !seen[key] {
				seen[key] = true
				out = append(out, b.Token)
			}
		}
	}
	sort.Strings(out)
	return out
}

func lookup(token string) (Board, bool) {
	for _, c := range catalog {
		for _, b := range c.Boards {
			if strings.EqualFold(b.Token, token) {
				return b, true
			}
		}
	}
	return Board{}, false
}

// CompanyFor returns the curated company name for a token, or "".
func CompanyFor(token string) string {
	b, _ := lookup(token)
	return b.Company
}

// CompanyNames returns every curated Lever company name, sorted and deduplicated.
func CompanyNames() []string {
	seen := map[string]bool{}
	var names []string
	for _, c := range catalog {
		for _, b := range c.Boards {
			if !seen[b.Company] {
				seen[b.Company] = true
				names = append(names, b.Company)
			}
		}
	}
	sort.Strings(names)
	return names
}
