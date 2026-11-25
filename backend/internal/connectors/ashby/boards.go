package ashby

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
			{"openai", "OpenAI"}, {"cohere", "Cohere"}, {"perplexity", "Perplexity"},
			{"elevenlabs", "ElevenLabs"}, {"harvey", "Harvey"}, {"sierra", "Sierra"},
			{"cognition", "Cognition"}, {"cursor", "Cursor"}, {"poolside", "Poolside"},
			{"adept", "Adept"}, {"character", "Character.AI"}, {"runway", "Runway"},
			{"pika", "Pika"}, {"luma", "Luma AI"}, {"assemblyai", "AssemblyAI"},
			{"deepgram", "Deepgram"}, {"pinecone", "Pinecone"}, {"langchain", "LangChain"},
			{"llamaindex", "LlamaIndex"}, {"baseten", "Baseten"}, {"modal", "Modal"},
			{"together", "Together AI"}, {"fireworks", "Fireworks AI"}, {"hebbia", "Hebbia"},
			{"writer", "Writer"}, {"synthesia", "Synthesia"}, {"imbue", "Imbue"},
			{"decagon", "Decagon"}, {"cartesia", "Cartesia"}, {"suno", "Suno"},
			{"cradle", "Cradle"}, {"cleanlab", "Cleanlab"}, {"contextual-ai", "Contextual AI"},
		},
	},
	{
		ID: "developer_tools", Parent: ptr("technical"),
		Boards: []Board{
			{"linear", "Linear"}, {"vercel", "Vercel"}, {"supabase", "Supabase"},
			{"replit", "Replit"}, {"raycast", "Raycast"}, {"posthog", "PostHog"},
			{"resend", "Resend"}, {"railway", "Railway"}, {"render", "Render"},
			{"clerk", "Clerk"}, {"workos", "WorkOS"}, {"stytch", "Stytch"},
			{"stainless", "Stainless"}, {"mintlify", "Mintlify"}, {"fern", "Fern"},
			{"graphite", "Graphite"}, {"coder", "Coder"}, {"warp", "Warp"},
			{"zed", "Zed"}, {"liveblocks", "Liveblocks"}, {"inngest", "Inngest"},
			{"trigger", "Trigger.dev"}, {"temporal", "Temporal"}, {"browserbase", "Browserbase"},
			{"e2b", "E2B"}, {"sentry", "Sentry"},
		},
	},
	{
		ID: "cloud_devops", Parent: ptr("technical"),
		Boards: []Board{
			{"fly", "Fly.io"}, {"depot", "Depot"}, {"buildkite", "Buildkite"},
			{"dagger", "Dagger"}, {"pulumi", "Pulumi"}, {"tailscale", "Tailscale"},
			{"oxide", "Oxide Computer"}, {"spacelift", "Spacelift"},
		},
	},
	{
		ID: "design_creative", Parent: ptr("technical"),
		Boards: []Board{
			{"framer", "Framer"}, {"spline", "Spline"}, {"rive", "Rive"},
			{"descript", "Descript"}, {"gamma", "Gamma"}, {"krea", "Krea"},
		},
	},
	{
		ID: "data_infrastructure", Parent: ptr("technical"),
		Boards: []Board{
			{"motherduck", "MotherDuck"}, {"clickhouse", "ClickHouse"}, {"tinybird", "Tinybird"},
			{"hex", "Hex"}, {"census", "Census"}, {"hightouch", "Hightouch"},
			{"airbyte", "Airbyte"}, {"dagster", "Dagster Labs"}, {"prefect", "Prefect"},
			{"neon", "Neon"}, {"turso", "Turso"}, {"convex", "Convex"},
			{"planetscale", "PlanetScale"}, {"qdrant", "Qdrant"}, {"weaviate", "Weaviate"},
		},
	},
	{
		ID: "enterprise_software", Parent: ptr("technical"),
		Boards: []Board{
			{"notion", "Notion"}, {"vanta", "Vanta"}, {"deel", "Deel"},
			{"ashby", "Ashby"}, {"attio", "Attio"}, {"clay", "Clay"},
			{"ironclad", "Ironclad"}, {"zapier", "Zapier"}, {"commonroom", "Common Room"},
			{"cal", "Cal.com"}, {"superhuman", "Superhuman"}, {"plain", "Plain"},
			{"pylon", "Pylon"}, {"unify", "Unify"},
		},
	},
	{
		ID: "finance",
		Boards: []Board{
			{"ramp", "Ramp"}, {"mercury", "Mercury"}, {"modern-treasury", "Modern Treasury"},
			{"column", "Column"}, {"increase", "Increase"}, {"lithic", "Lithic"},
			{"moov", "Moov"}, {"sardine", "Sardine"}, {"persona", "Persona"},
			{"parafin", "Parafin"}, {"pulley", "Pulley"}, {"monarch", "Monarch Money"},
			{"kalshi", "Kalshi"}, {"polymarket", "Polymarket"},
		},
	},
	{
		ID: "accounting_compliance",
		Boards: []Board{
			{"numeral", "Numeral"}, {"anrok", "Anrok"}, {"campfire", "Campfire"},
			{"puzzle", "Puzzle"}, {"rillet", "Rillet"}, {"drata", "Drata"},
		},
	},
	{
		ID: "cybersecurity",
		Boards: []Board{
			{"chainguard", "Chainguard"}, {"semgrep", "Semgrep"}, {"1password", "1Password"},
			{"sublime", "Sublime Security"}, {"material-security", "Material Security"}, {"socket", "Socket"},
			{"endorlabs", "Endor Labs"}, {"aikido", "Aikido Security"}, {"runzero", "runZero"},
			{"panther", "Panther"}, {"tines", "Tines"}, {"lakera", "Lakera"},
		},
	},
	{
		ID: "medical",
		Boards: []Board{
			{"abridge", "Abridge"}, {"hippocratic-ai", "Hippocratic AI"}, {"nabla", "Nabla"},
			{"openevidence", "OpenEvidence"}, {"candidhealth", "Candid Health"}, {"healthie", "Healthie"},
			{"pomelo", "Pomelo Care"}, {"summerhealth", "Summer Health"}, {"solace", "Solace Health"},
		},
	},
	{
		ID: "pharma_biotech",
		Boards: []Board{
			{"latch", "LatchBio"}, {"biorender", "BioRender"},
			{"chai-discovery", "Chai Discovery"}, {"evolutionaryscale", "EvolutionaryScale"},
		},
	},
	{
		ID: "energy_climate",
		Boards: []Board{
			{"watershed", "Watershed"}, {"basepower", "Base Power"}, {"crusoe", "Crusoe"},
			{"antora", "Antora Energy"}, {"fervo", "Fervo Energy"}, {"span", "SPAN"},
			{"quilt", "Quilt"}, {"runwise", "Runwise"},
		},
	},
	{
		ID: "robotics", Parent: ptr("technical"),
		Boards: []Board{
			{"figure", "Figure AI"}, {"1x", "1X Technologies"}, {"physicalintelligence", "Physical Intelligence"},
			{"apptronik", "Apptronik"}, {"hadrian", "Hadrian"}, {"nominal", "Nominal"},
		},
	},
	{
		ID: "aerospace_defense",
		Boards: []Board{
			{"saronic", "Saronic"}, {"epirus", "Epirus"}, {"castelion", "Castelion"}, {"chaos", "CHAOS Industries"},
		},
	},
	{
		ID: "space",
		Boards: []Board{
			{"varda", "Varda Space Industries"}, {"apex", "Apex Space"}, {"k2space", "K2 Space"},
			{"muonspace", "Muon Space"}, {"albedo", "Albedo"}, {"trueanomaly", "True Anomaly"},
		},
	},
	{
		ID: "legal_tech",
		Boards: []Board{
			{"evenup", "EvenUp"}, {"spellbook", "Spellbook"}, {"norm-ai", "Norm AI"},
			{"darrow", "Darrow"}, {"paxton", "Paxton AI"},
		},
	},
	{
		ID: "consumer_internet", Parent: ptr("technical"),
		Boards: []Board{
			{"whatnot", "Whatnot"}, {"substack", "Substack"}, {"beehiiv", "beehiiv"},
			{"partiful", "Partiful"}, {"beli", "Beli"},
		},
	},
	{
		ID: "sports_fitness",
		Boards: []Board{
			{"strava", "Strava"}, {"oura", "Oura"}, {"eight-sleep", "Eight Sleep"},
		},
	},
	{
		ID: "crypto_web3",
		Boards: []Board{
			{"phantom", "Phantom"}, {"paradigm", "Paradigm"}, {"privy", "Privy"},
			{"dynamic", "Dynamic"}, {"bridge", "Bridge"}, {"uniswap", "Uniswap Labs"},
			{"dydx", "dYdX"}, {"mystenlabs", "Mysten Labs"},
		},
	},
	{
		ID: "hr_workforce",
		Boards: []Board{
			{"remote", "Remote"}, {"oyster", "Oyster"}, {"gem", "Gem"},
			{"brighthire", "BrightHire"}, {"metaview", "Metaview"}, {"pave", "Pave"},
		},
	},
	{
		ID: "supply_chain",
		Boards: []Board{
			{"flexport", "Flexport"}, {"zip", "Zip"}, {"altana", "Altana"}, {"cargo", "Cargo"},
		},
	},
	{
		ID: "construction_infrastructure",
		Boards: []Board{
			{"buildots", "Buildots"}, {"trunktools", "Trunk Tools"}, {"join", "Join"},
		},
	},
	{
		ID: "real_estate",
		Boards: []Board{
			{"bilt", "Bilt Rewards"}, {"wander", "Wander"}, {"flex", "Flex"},
		},
	},
	{
		ID: "insurance",
		Boards: []Board{
			{"vouch", "Vouch"}, {"coalition", "Coalition"}, {"jerry", "Jerry"},
		},
	},
	{
		ID: "consumer_brands",
		Boards: []Board{
			{"faire", "Faire"}, {"hexclad", "HexClad"}, {"ridge", "Ridge"},
		},
	},
	{
		ID: "hospitality_events",
		Boards: []Board{
			{"luma-events", "Luma"}, {"blackbird", "Blackbird"},
		},
	},
}

// BoardsFor returns the de-duplicated Ashby boards for the given category IDs.
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

// BoardCount returns the unique Ashby board count for a category ID.
func BoardCount(id string) int { return len(BoardsFor([]string{id})) }

// Tokens returns every curated Ashby board token, sorted.
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

// CompanyNames returns every curated Ashby company name, sorted and deduplicated.
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
