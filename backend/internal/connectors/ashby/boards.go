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
			{"luma-events", "Luma"}, {"blackbird", "Blackbird"}, {"partiful", "Partiful"},
			{"beli", "Beli"}, {"wander", "Wander"},
		},
	},
	{
		ID: "embedded_iot", Parent: ptr("technical"),
		Boards: []Board{
			{"oxide", "Oxide Computer"}, {"span", "SPAN"}, {"hadrian", "Hadrian"},
			{"nominal", "Nominal"}, {"eight-sleep", "Eight Sleep"}, {"oura", "Oura"},
			{"quilt", "Quilt"}, {"runwise", "Runwise"},
		},
	},
	{
		ID: "productivity_collaboration", Parent: ptr("technical"),
		Boards: []Board{
			{"notion", "Notion"}, {"linear", "Linear"}, {"superhuman", "Superhuman"},
			{"raycast", "Raycast"}, {"cal", "Cal.com"}, {"framer", "Framer"},
			{"attio", "Attio"}, {"zapier", "Zapier"}, {"gamma", "Gamma"},
		},
	},
	{
		ID: "social_creator", Parent: ptr("technical"),
		Boards: []Board{
			{"substack", "Substack"}, {"beehiiv", "beehiiv"}, {"whatnot", "Whatnot"},
			{"partiful", "Partiful"}, {"suno", "Suno"}, {"descript", "Descript"},
			{"krea", "Krea"}, {"runway", "Runway"},
		},
	},
	{
		ID: "customer_experience", Parent: ptr("technical"),
		Boards: []Board{
			{"sierra", "Sierra"}, {"decagon", "Decagon"}, {"plain", "Plain"},
			{"pylon", "Pylon"}, {"commonroom", "Common Room"}, {"clay", "Clay"},
		},
	},
	{
		ID: "payments_banking",
		Boards: []Board{
			{"ramp", "Ramp"}, {"mercury", "Mercury"}, {"modern-treasury", "Modern Treasury"},
			{"column", "Column"}, {"increase", "Increase"}, {"lithic", "Lithic"},
			{"moov", "Moov"}, {"bridge", "Bridge"}, {"parafin", "Parafin"},
		},
	},
	{
		ID: "venture_investing",
		Boards: []Board{
			{"paradigm", "Paradigm"}, {"pulley", "Pulley"}, {"kalshi", "Kalshi"},
			{"polymarket", "Polymarket"}, {"carta", "Carta"},
		},
	},
	{
		ID: "education",
		Boards: []Board{
			{"replit", "Replit"}, {"brilliant", "Brilliant"}, {"synth", "Synthesis"},
			{"maven", "Maven"},
		},
	},
	{
		ID: "gaming", Parent: ptr("technical"),
		Boards: []Board{
			{"discord", "Discord"}, {"inworld", "Inworld AI"}, {"overwolf", "Overwolf"},
		},
	},
	{
		ID: "autonomous_vehicles", Parent: ptr("technical"),
		Boards: []Board{
			{"waabi", "Waabi"}, {"appliedintuition", "Applied Intuition"}, {"wayve", "Wayve"},
			{"kodiak", "Kodiak"}, {"maymobility", "May Mobility"}, {"gatik", "Gatik"},
			{"torc", "Torc Robotics"}, {"ouster", "Ouster"}, {"aeva", "Aeva"},
		},
	},
	{
		ID: "retail_ecommerce",
		Boards: []Board{
			{"faire", "Faire"}, {"whatnot", "Whatnot"}, {"hexclad", "HexClad"},
			{"ridge", "Ridge"}, {"quince", "Quince"}, {"italic", "Italic"},
			{"evergreen", "Evergreen"}, {"shopmy", "ShopMy"}, {"flagship", "Flagship"},
			{"canal", "Canal"}, {"violet", "Violet"}, {"rye", "Rye"},
		},
	},
	{
		ID: "transportation_logistics",
		Boards: []Board{
			{"flexport", "Flexport"}, {"altana", "Altana"}, {"cargo", "Cargo"},
			{"terminal", "Terminal Industries"}, {"happyrobot", "HappyRobot"}, {"augusto", "Augment"},
			{"vroom", "Vroom"}, {"flocargo", "Flock Freight"}, {"mothership", "Mothership"},
			{"stord", "Stord"}, {"ware2go", "Ware2Go"}, {"warpfreight", "Warp"},
		},
	},
	{
		ID: "media_entertainment",
		Boards: []Board{
			{"substack", "Substack"}, {"beehiiv", "beehiiv"}, {"suno", "Suno"},
			{"runway", "Runway"}, {"pika", "Pika"}, {"luma", "Luma AI"},
			{"elevenlabs", "ElevenLabs"}, {"descript", "Descript"}, {"captions", "Captions"},
			{"opusclip", "OpusClip"}, {"heygen", "HeyGen"}, {"hedras", "Hedra"},
			{"krea", "Krea"}, {"ideogram", "Ideogram"}, {"midjourney", "Midjourney"},
		},
	},
	{
		ID: "marketing_advertising",
		Boards: []Board{
			{"clay", "Clay"}, {"unify", "Unify"}, {"commonroom", "Common Room"},
			{"warmly", "Warmly"}, {"koala", "Koala"}, {"default", "Default"},
			{"vector", "Vector"}, {"rb2b", "RB2B"}, {"mutiny", "Mutiny"},
			{"jasper", "Jasper"}, {"typeface", "Typeface"}, {"pencil", "Pencil"},
			{"smartly", "Smartly"}, {"triplewhale", "Triple Whale"}, {"northbeam", "Northbeam"},
		},
	},
	{
		ID: "food_agriculture",
		Boards: []Board{
			{"monarchtractor", "Monarch Tractor"}, {"carbonrobotics", "Carbon Robotics"},
			{"verdantrobotics", "Verdant Robotics"}, {"bountiful", "Bountiful"},
			{"soundag", "Sound Agriculture"}, {"nitricity", "Nitricity"},
			{"milli", "Milli"}, {"slice", "Slice"}, {"owner", "Owner.com"},
			{"snackpass", "Snackpass"}, {"sunday", "sunday"}, {"popmenu", "Popmenu"},
		},
	},
	{
		ID: "travel_hospitality",
		Boards: []Board{
			{"wander", "Wander"}, {"blackbird", "Blackbird"}, {"beli", "Beli"},
			{"mindtrip", "Mindtrip"}, {"fora", "Fora Travel"}, {"duffel", "Duffel"},
			{"amadeus", "Amadeus"}, {"canary", "Canary Technologies"}, {"mews", "Mews"},
			{"spotnana", "Spotnana"}, {"navan", "Navan"}, {"engine", "Engine"},
		},
	},
	{
		ID: "fashion_beauty",
		Boards: []Board{
			{"daydream", "Daydream"}, {"photoroom", "Photoroom"}, {"shopmy", "ShopMy"},
			{"glossier", "Glossier"}, {"summerfridays", "Summer Fridays"}, {"kosas", "Kosas"},
			{"dieux", "Dieux"}, {"saie", "Saie"}, {"jonesroad", "Jones Road Beauty"},
		},
	},
	{
		ID: "semiconductors_hardware",
		Boards: []Board{
			{"oxide", "Oxide Computer"}, {"etched", "Etched"}, {"matx", "MatX"},
			{"extropic", "Extropic"}, {"normalcomputing", "Normal Computing"}, {"rain", "Rain AI"},
			{"positrion", "Positron AI"}, {"rivos", "Rivos"}, {"aheadcomputing", "AheadComputing"},
			{"lightmatter", "Lightmatter"}, {"ayarlabs", "Ayar Labs"}, {"celestialai", "Celestial AI"},
		},
	},
	{
		ID: "manufacturing",
		Boards: []Board{
			{"hadrian", "Hadrian"}, {"nominal", "Nominal"}, {"dirac", "Dirac"},
			{"senra", "Senra Systems"}, {"atomic", "Atomic Industries"}, {"machina", "Machina Labs"},
			{"freeform", "Freeform"}, {"rangeenergy", "Range Energy"}, {"bedrock", "Bedrock Robotics"},
			{"standardbots", "Standard Bots"}, {"brightmachines", "Bright Machines"},
		},
	},
	{
		ID: "labs",
		Boards: []Board{
			{"evolutionaryscale", "EvolutionaryScale"}, {"chai-discovery", "Chai Discovery"},
			{"cradle", "Cradle"}, {"latch", "LatchBio"}, {"biorender", "BioRender"},
			{"futurehouse", "FutureHouse"}, {"edisonscientific", "Edison Scientific"},
			{"periodic", "Periodic Labs"}, {"lila", "Lila Sciences"}, {"orbitalmaterials", "Orbital Materials"},
			{"radicalai", "Radical AI"}, {"cuspai", "CuspAI"}, {"boltz", "Boltz"},
		},
	},
	{
		ID: "quantum_computing",
		Boards: []Board{
			{"psiquantum", "PsiQuantum"}, {"quera", "QuEra"}, {"atlanticquantum", "Atlantic Quantum"},
			{"aliceandbob", "Alice & Bob"}, {"nordquantique", "Nord Quantique"}, {"photonic", "Photonic"},
			{"sandboxaq", "SandboxAQ"}, {"classiq", "Classiq"},
		},
	},
	{
		ID: "nonprofit",
		Boards: []Board{
			{"epochai", "Epoch AI"}, {"redwoodresearch", "Redwood Research"},
			{"apollo-research", "Apollo Research"}, {"metr", "METR"},
			{"rethinkpriorities", "Rethink Priorities"}, {"openphilanthropy", "Open Philanthropy"},
			{"arcinstitute", "Arc Institute"}, {"renaissancephilanthropy", "Renaissance Philanthropy"},
		},
	},
	{
		ID: "government_civic",
		Boards: []Board{
			{"govly", "Govly"}, {"usg", "CivicTech"}, {"peregrine", "Peregrine"},
			{"flock", "Flock Safety"}, {"skydio", "Skydio"}, {"usdigitalresponse", "U.S. Digital Response"},
			{"coda-gov", "Polimorphic"}, {"civicplus", "CivicPlus"}, {"opengov", "OpenGov"},
		},
	},
	{
		ID: "telecom",
		Boards: []Board{
			{"livekit", "LiveKit"}, {"daily", "Daily"}, {"vapi", "Vapi"},
			{"retell", "Retell AI"}, {"bland", "Bland AI"}, {"openphone", "OpenPhone"},
			{"quo", "Quo"}, {"tarana", "Tarana Wireless"}, {"astspacemobile", "AST SpaceMobile"},
		},
	},
	{
		ID: "automotive",
		Boards: []Board{
			{"waabi", "Waabi"}, {"wayve", "Wayve"}, {"appliedintuition", "Applied Intuition"},
			{"harbinger", "Harbinger"}, {"slate", "Slate Auto"}, {"telomotors", "Telo Trucks"},
			{"rivian", "Rivian"}, {"scout", "Scout Motors"}, {"tekion", "Tekion"},
		},
	},
	{
		ID: "consulting_services",
		Boards: []Board{
			{"distyl", "Distyl AI"}, {"cinder", "Cinder"}, {"scale", "Scale AI"},
			{"mercor", "Mercor"}, {"surge", "Surge AI"}, {"turing", "Turing"},
			{"micro1", "micro1"}, {"handshake-ai", "Handshake AI"},
		},
	},
	{
		ID: "data_analytics_bi", Parent: ptr("technical"),
		Boards: []Board{
			{"hex", "Hex"}, {"motherduck", "MotherDuck"}, {"clickhouse", "ClickHouse"},
			{"tinybird", "Tinybird"}, {"omni", "Omni"}, {"evidence", "Evidence"},
			{"lightdash", "Lightdash"}, {"rill", "Rill Data"}, {"equals", "Equals"},
			{"posthog", "PostHog"}, {"statsig", "Statsig"}, {"eppo", "Eppo"},
		},
	},
	{
		ID: "ecommerce_enablement", Parent: ptr("technical"),
		Boards: []Board{
			{"faire", "Faire"}, {"medusa", "Medusa"}, {"swell", "Swell"},
			{"fermat", "FERMÀT"}, {"replay", "Rebuy"}, {"loop", "Loop Returns"},
			{"rye", "Rye"}, {"canal", "Canal"}, {"violet", "Violet"},
			{"disco-network", "Disco"}, {"blackcrow", "Black Crow AI"},
		},
	},
	{
		ID: "marketplace_gig", Parent: ptr("technical"),
		Boards: []Board{
			{"whatnot", "Whatnot"}, {"faire", "Faire"}, {"mercor", "Mercor"},
			{"contra", "Contra"}, {"braintrust", "Braintrust"}, {"paraform", "Paraform"},
			{"partiful", "Partiful"}, {"wander", "Wander"}, {"bilt", "Bilt Rewards"},
		},
	},
	{
		ID: "pet_veterinary",
		Boards: []Board{
			{"sploot", "Sploot Veterinary Care"}, {"modernanimal", "Modern Animal"},
			{"smalls", "Smalls"}, {"petfolk", "Petfolk"}, {"scribenote", "Scribenote"},
			{"vetspire", "Vetspire"}, {"dutch", "Dutch"},
		},
	},
	{
		ID: "architecture_civil",
		Boards: []Board{
			{"buildots", "Buildots"}, {"trunktools", "Trunk Tools"}, {"join", "Join"},
			{"snaptrude", "Snaptrude"}, {"rayon", "Rayon"}, {"maket", "Maket"},
			{"gendo", "Gendo"}, {"kailo", "Kailo"}, {"giraffetech", "Giraffe"},
		},
	},
	{
		ID: "aviation_maritime",
		Boards: []Board{
			{"saronic", "Saronic"}, {"mach", "Mach Industries"}, {"heron", "Hermeus"},
			{"boom", "Boom Supersonic"}, {"regent", "Regent"}, {"saildrone", "Saildrone"},
			{"orca-ai", "Orca AI"}, {"shone", "Shone"}, {"pyka", "Pyka"},
			{"reliable-robotics", "Reliable Robotics"}, {"merlinlabs", "Merlin Labs"},
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
