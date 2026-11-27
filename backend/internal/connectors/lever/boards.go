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
			{"resy", "Resy"}, {"sevenrooms", "SevenRooms"},
		},
	},
	{
		ID: "embedded_iot", Parent: ptr("technical"),
		Boards: []Board{
			{"particle", "Particle"}, {"wyze", "Wyze"}, {"latch", "Latch"},
			{"eero", "eero"}, {"samsara", "Samsara"},
		},
	},
	{
		ID: "productivity_collaboration", Parent: ptr("technical"),
		Boards: []Board{
			{"atlassian", "Atlassian"}, {"box", "Box"}, {"docusign", "DocuSign"},
			{"lucid", "Lucid Software"}, {"mural", "Mural"}, {"canva", "Canva"},
			{"typeform", "Typeform"}, {"coda", "Coda"}, {"loom", "Loom"},
		},
	},
	{
		ID: "social_creator", Parent: ptr("technical"),
		Boards: []Board{
			{"patreon", "Patreon"}, {"kickstarter", "Kickstarter"}, {"vimeo", "Vimeo"},
			{"soundcloud", "SoundCloud"}, {"spotify", "Spotify"}, {"netflix", "Netflix"},
		},
	},
	{
		ID: "customer_experience", Parent: ptr("technical"),
		Boards: []Board{
			{"front", "Front"}, {"gorgias", "Gorgias"}, {"kustomer", "Kustomer"},
			{"gladly", "Gladly"}, {"gainsight", "Gainsight"}, {"pendo", "Pendo"},
		},
	},
	{
		ID: "payments_banking",
		Boards: []Board{
			{"marqeta", "Marqeta"}, {"klarna", "Klarna"}, {"flywire", "Flywire"},
			{"payoneer", "Payoneer"}, {"dwolla", "Dwolla"}, {"unit", "Unit"},
			{"synctera", "Synctera"}, {"wealthfront", "Wealthfront"}, {"betterment", "Betterment"},
		},
	},
	{
		ID: "venture_investing",
		Boards: []Board{
			{"carta", "Carta"}, {"angellist", "AngelList"}, {"pitchbook", "PitchBook"},
			{"affinity", "Affinity"},
		},
	},
	{
		ID: "hr_workforce",
		Boards: []Board{
			{"justworks", "Justworks"}, {"rippling", "Rippling"}, {"lever", "Lever"},
			{"breezy", "Breezy HR"}, {"namely", "Namely"},
		},
	},
	{
		ID: "robotics", Parent: ptr("technical"),
		Boards: []Board{
			{"symbotic", "Symbotic"}, {"covariant", "Covariant"}, {"bearrobotics", "Bear Robotics"},
			{"dexterity", "Dexterity"},
		},
	},
	{
		ID: "real_estate",
		Boards: []Board{
			{"compass", "Compass"}, {"opendoor", "Opendoor"}, {"divvyhomes", "Divvy Homes"},
			{"flyhomes", "Flyhomes"},
		},
	},
	{
		ID: "insurance",
		Boards: []Board{
			{"lemonade", "Lemonade"}, {"root", "Root Insurance"}, {"nextinsurance", "Next Insurance"},
			{"kin", "Kin Insurance"}, {"hippo", "Hippo Insurance"}, {"metromile", "Metromile"},
			{"clearcover", "Clearcover"}, {"branchinsurance", "Branch"}, {"bestow", "Bestow"},
		},
	},
	{
		ID: "media_entertainment",
		Boards: []Board{
			{"netflix", "Netflix"}, {"spotify", "Spotify"}, {"vimeo", "Vimeo"},
			{"soundcloud", "SoundCloud"}, {"patreon", "Patreon"}, {"kickstarter", "Kickstarter"},
			{"nytimes", "The New York Times"}, {"theathletic", "The Athletic"}, {"morningbrew", "Morning Brew"},
			{"bandcamp", "Bandcamp"}, {"letterboxd", "Letterboxd"}, {"mubi", "MUBI"},
			{"genius", "Genius"}, {"bleacherreport", "Bleacher Report"}, {"complex", "Complex"},
		},
	},
	{
		ID: "consulting_services",
		Boards: []Board{
			{"slalom", "Slalom"}, {"pariveda", "Pariveda"}, {"westmonroe", "West Monroe"},
			{"publicissapient", "Publicis Sapient"}, {"frogdesign", "frog"}, {"workandco", "Work & Co"},
			{"ustwo", "ustwo"}, {"pivotal", "Pivotal Labs"}, {"carbonfive", "Carbon Five"},
			{"thoughtbot", "thoughtbot"}, {"8thlight", "8th Light"}, {"testdouble", "Test Double"},
		},
	},
	{
		ID: "marketing_advertising",
		Boards: []Board{
			{"klaviyo", "Klaviyo"}, {"braze", "Braze"}, {"iterable", "Iterable"},
			{"hubspot", "HubSpot"}, {"mailchimp", "Mailchimp"}, {"activecampaign", "ActiveCampaign"},
			{"buffer", "Buffer"}, {"later", "Later"}, {"sproutsocial", "Sprout Social"},
			{"semrush", "Semrush"}, {"moz", "Moz"}, {"optimizely", "Optimizely"},
			{"vwo", "VWO"}, {"6sense", "6sense"}, {"demandbase", "Demandbase"},
			{"rollworks", "RollWorks"}, {"adroll", "AdRoll"}, {"moloco", "Moloco"},
		},
	},
	{
		ID: "semiconductors_hardware",
		Boards: []Board{
			{"sifive", "SiFive"}, {"cerebras", "Cerebras"}, {"groq", "Groq"},
			{"sambanova", "SambaNova"}, {"mythic", "Mythic"}, {"lightmatter", "Lightmatter"},
			{"ayarlabs", "Ayar Labs"}, {"framework", "Framework"}, {"nothing", "Nothing"},
			{"analog", "Analog Devices"}, {"lattice-semi", "Lattice Semiconductor"},
		},
	},
	{
		ID: "manufacturing",
		Boards: []Board{
			{"desktopmetal", "Desktop Metal"}, {"carbon3d", "Carbon"}, {"velo3d", "Velo3D"},
			{"protolabs", "Protolabs"}, {"fictiv", "Fictiv"}, {"xometry", "Xometry"},
			{"fast-radius", "Fast Radius"}, {"brightmachines", "Bright Machines"}, {"instrumental", "Instrumental"},
			{"sightmachine", "Sight Machine"}, {"machinemetrics", "MachineMetrics"}, {"tulip", "Tulip"},
		},
	},
	{
		ID: "aerospace_defense",
		Boards: []Board{
			{"shieldai", "Shield AI"}, {"skydio", "Skydio"}, {"hermeus", "Hermeus"},
			{"boom", "Boom Supersonic"}, {"beta", "BETA Technologies"}, {"wisk", "Wisk Aero"},
			{"firefly", "Firefly Aerospace"}, {"relativity", "Relativity Space"}, {"plannet", "Planet Labs"},
			{"hawkeye360", "HawkEye 360"}, {"rebellion", "Rebellion Defense"}, {"secondfront", "Second Front"},
		},
	},
	{
		ID: "space",
		Boards: []Board{
			{"firefly", "Firefly Aerospace"}, {"stokespace", "Stoke Space"}, {"impulse", "Impulse Space"},
			{"axiomspace", "Axiom Space"}, {"astranis", "Astranis"}, {"spire", "Spire Global"},
			{"blacksky", "BlackSky"}, {"capellaspace", "Capella Space"}, {"planet", "Planet Labs"},
		},
	},
	{
		ID: "government_civic",
		Boards: []Board{
			{"usds", "U.S. Digital Service"}, {"nava", "Nava PBC"}, {"adhoc", "Ad Hoc"},
			{"fearless", "Fearless"}, {"coforma", "Coforma"}, {"truss", "Truss"},
			{"opengov", "OpenGov"}, {"granicus", "Granicus"}, {"tylertech", "Tyler Technologies"},
			{"polco", "Polco"}, {"fiscalnote", "FiscalNote"}, {"quorum", "Quorum"},
		},
	},
	{
		ID: "nonprofit",
		Boards: []Board{
			{"eff", "Electronic Frontier Foundation"}, {"signal", "Signal"}, {"torproject", "Tor Project"},
			{"internetarchive", "Internet Archive"}, {"khanacademy", "Khan Academy"}, {"codeorg", "Code.org"},
			{"girlswhocode", "Girls Who Code"}, {"charitywater", "charity: water"}, {"kiva", "Kiva"},
			{"watsi", "Watsi"}, {"mercycorps", "Mercy Corps"}, {"path", "PATH"},
		},
	},
	{
		ID: "food_agriculture",
		Boards: []Board{
			{"impossiblefoods", "Impossible Foods"}, {"beyondmeat", "Beyond Meat"}, {"eatjust", "Eat Just"},
			{"upsidefoods", "UPSIDE Foods"}, {"boweryfarming", "Bowery Farming"}, {"plenty", "Plenty"},
			{"aerofarms", "AeroFarms"}, {"granulardata", "Granular"}, {"farmersbusinessnetwork", "FBN"},
			{"apeel", "Apeel Sciences"}, {"imperfectfoods", "Imperfect Foods"}, {"misfitsmarket", "Misfits Market"},
		},
	},
	{
		ID: "travel_hospitality",
		Boards: []Board{
			{"hopper", "Hopper"}, {"kayak", "KAYAK"}, {"skyscanner", "Skyscanner"},
			{"getyourguide", "GetYourGuide"}, {"viator", "Viator"}, {"klook", "Klook"},
			{"avantstay", "AvantStay"}, {"vacasa", "Vacasa"}, {"sonder", "Sonder"},
			{"selina", "Selina"}, {"mews", "Mews"}, {"guesty", "Guesty"},
		},
	},
	{
		ID: "fashion_beauty",
		Boards: []Board{
			{"glossier", "Glossier"}, {"warbyparker", "Warby Parker"}, {"allbirds", "Allbirds"},
			{"everlane", "Everlane"}, {"reformation", "Reformation"}, {"mejuri", "Mejuri"},
			{"outdoorvoices", "Outdoor Voices"}, {"rhode", "rhode"}, {"merit", "MERIT"},
			{"ilitia", "ILIA Beauty"}, {"fentybeauty", "Fenty Beauty"}, {"sephora", "Sephora"},
		},
	},
	{
		ID: "telecom",
		Boards: []Board{
			{"twilio", "Twilio"}, {"telnyx", "Telnyx"}, {"plivo", "Plivo"},
			{"sinch", "Sinch"}, {"infobip", "Infobip"}, {"aircall", "Aircall"},
			{"openphone", "OpenPhone"}, {"ringcentral", "RingCentral"}, {"8x8", "8x8"},
			{"starlink", "Starlink"},
		},
	},
	{
		ID: "automotive",
		Boards: []Board{
			{"rivian", "Rivian"}, {"lucidmotors", "Lucid Motors"}, {"polestar", "Polestar"},
			{"canoo", "Canoo"}, {"fisker", "Fisker"}, {"waymo", "Waymo"},
			{"cruise", "Cruise"}, {"aurora", "Aurora"}, {"tuSimple", "TuSimple"},
			{"tekion", "Tekion"}, {"acvauctions", "ACV Auctions"}, {"vroom", "Vroom"},
		},
	},
	{
		ID: "labs",
		Boards: []Board{
			{"chanzuckerberg", "Chan Zuckerberg Initiative"}, {"broadinstitute", "Broad Institute"},
			{"allenai", "Allen Institute for AI (AI2)"}, {"arcinstitute", "Arc Institute"},
			{"newscience", "New Science"}, {"biocommons", "BioCommons"}, {"benchling", "Benchling"},
			{"recursion", "Recursion"}, {"insitro", "insitro"}, {"schrodinger", "Schrödinger"},
		},
	},
	{
		ID: "construction_infrastructure",
		Boards: []Board{
			{"procore", "Procore"}, {"plangrid", "PlanGrid"}, {"buildertrend", "Buildertrend"},
			{"houzz", "Houzz"}, {"equipmentshare", "EquipmentShare"}, {"bigrentz", "BigRentz"},
			{"kojo", "Kojo"}, {"briq", "Briq"}, {"doxel", "Doxel"}, {"dustyrobotics", "Dusty Robotics"},
		},
	},
	{
		ID: "data_analytics_bi", Parent: ptr("technical"),
		Boards: []Board{
			{"amplitude", "Amplitude"}, {"mixpanel", "Mixpanel"}, {"pendo", "Pendo"},
			{"mode", "Mode"}, {"sisense", "Sisense"}, {"looker", "Looker"},
			{"chartio", "Chartio"}, {"periscopedata", "Periscope Data"}, {"fivetran", "Fivetran"},
			{"segment", "Segment"}, {"heap", "Heap"},
		},
	},
	{
		ID: "ecommerce_enablement", Parent: ptr("technical"),
		Boards: []Board{
			{"klaviyo", "Klaviyo"}, {"gorgias", "Gorgias"}, {"recharge", "Recharge"},
			{"loopreturns", "Loop Returns"}, {"returnly", "Returnly"}, {"bolt", "Bolt"},
			{"fast", "Fast"}, {"narvar", "Narvar"}, {"route", "Route"},
			{"triplewhale", "Triple Whale"}, {"northbeam", "Northbeam"},
		},
	},
	{
		ID: "marketplace_gig", Parent: ptr("technical"),
		Boards: []Board{
			{"rover", "Rover"}, {"thumbtack", "Thumbtack"}, {"taskrabbit", "TaskRabbit"},
			{"turo", "Turo"}, {"poshmark", "Poshmark"}, {"stockx", "StockX"},
			{"goat", "GOAT Group"}, {"upwork", "Upwork"}, {"fiverr", "Fiverr"},
			{"toptal", "Toptal"}, {"cameo", "Cameo"}, {"hipcamp", "Hipcamp"},
		},
	},
	{
		ID: "pet_veterinary",
		Boards: []Board{
			{"rover", "Rover"}, {"wag", "Wag!"}, {"fi", "Fi"},
			{"whisker", "Whisker"}, {"ollie", "Ollie"}, {"nomnom", "Nom Nom"},
			{"fuzzy", "Fuzzy Pet Health"}, {"trupanion", "Trupanion"}, {"lemonadepet", "Lemonade Pet"},
		},
	},
	{
		ID: "architecture_civil",
		Boards: []Board{
			{"testfit", "TestFit"}, {"hypar", "Hypar"}, {"cove-tool", "cove.tool"},
			{"upcodes", "UpCodes"}, {"monograph", "Monograph"}, {"joinbuild", "Join"},
			{"matterport", "Matterport"}, {"hover", "HOVER"},
		},
	},
	{
		ID: "aviation_maritime",
		Boards: []Board{
			{"joby", "Joby Aviation"}, {"archer", "Archer Aviation"}, {"blade", "BLADE"},
			{"wheelsup", "Wheels Up"}, {"foreflight", "ForeFlight"}, {"skydio", "Skydio"},
			{"saildrone", "Saildrone"}, {"nautiluslabs", "Nautilus Labs"}, {"windward", "Windward"},
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
