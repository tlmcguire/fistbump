package greenhouse

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
)

//go:embed boards.json
var boardsJSON []byte

type Board struct {
	Token   string `json:"token"`
	Company string `json:"company"`
	// Website is the company's home page, so a reader can see what it does. Optional; https only.
	Website string `json:"website,omitempty"`
}

type Category struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Parent *string `json:"parent"`
	Boards []Board `json:"boards"`
}

type boardFile struct {
	Version    int        `json:"version"`
	VerifiedAt string     `json:"verified_at"`
	Categories []Category `json:"categories"`
}

var file boardFile

func init() {
	if err := json.Unmarshal(boardsJSON, &file); err != nil {
		panic("greenhouse boards.json: " + err.Error())
	}
	tech := "technical"
	extras := []Category{
		{
			ID: "cloud_devops", Name: "Cloud and DevOps", Parent: &tech,
			Boards: []Board{
				{Token: "cloudflare", Company: "Cloudflare", Website: "https://www.cloudflare.com"},
				{Token: "datadog", Company: "Datadog", Website: "https://www.datadoghq.com"},
				{Token: "digitalocean98", Company: "DigitalOcean", Website: "https://www.digitalocean.com"},
				{Token: "fastly", Company: "Fastly", Website: "https://www.fastly.com"},
				{Token: "backblaze", Company: "Backblaze", Website: "https://www.backblaze.com"},
				{Token: "grafanalabs", Company: "Grafana Labs", Website: "https://grafana.com"},
				{Token: "harnessinc", Company: "Harness", Website: "https://www.harness.io"},
				{Token: "honeycomb", Company: "Honeycomb.io", Website: "https://www.honeycomb.io"},
				{Token: "tailscale", Company: "Tailscale", Website: "https://tailscale.com"},
				{Token: "newrelic", Company: "New Relic", Website: "https://newrelic.com"},
				{Token: "sumologic", Company: "Sumo Logic", Website: "https://www.sumologic.com"},
				{Token: "aviatrix", Company: "Aviatrix", Website: "https://aviatrix.ai"},
				{Token: "akuity", Company: "Akuity", Website: "https://akuity.io"},
				{Token: "canonical", Company: "Canonical", Website: "https://canonical.com"},
				{Token: "wasabi", Company: "Wasabi Technologies", Website: "https://wasabi.com"},
			},
		},
		{
			ID: "design_creative", Name: "Design and creative tools", Parent: &tech,
			Boards: []Board{
				{Token: "figma", Company: "Figma", Website: "https://www.figma.com"},
				{Token: "miro", Company: "Miro", Website: "https://miro.com"},
				{Token: "squarespace", Company: "Squarespace", Website: "https://www.squarespace.com"},
				{Token: "webflow", Company: "Webflow", Website: "https://webflow.com"},
				{Token: "descript", Company: "Descript", Website: "https://descript.com"},
				{Token: "picsart", Company: "Picsart", Website: "https://picsart.com"},
				{Token: "lucidsoftware", Company: "Lucid Software", Website: "https://lucid.co"},
				{Token: "ideo", Company: "IDEO", Website: "https://www.ideo.com"},
			},
		},
		{
			ID: "embedded_iot", Name: "Embedded systems and IoT", Parent: &tech,
			Boards: []Board{
				{Token: "verkada", Company: "Verkada", Website: "https://www.verkada.com"},
				{Token: "samsara", Company: "Samsara", Website: "https://www.samsara.com"},
				{Token: "ubiquiti", Company: "Ubiquiti", Website: "https://ubiquiti.com"},
				{Token: "simplisafe", Company: "SimpliSafe", Website: "https://simplisafe.com"},
				{Token: "sonatus", Company: "Sonatus", Website: "https://www.sonatus.com"},
				{Token: "armada", Company: "Armada", Website: "https://www.armada.ai"},
				{Token: "eridan", Company: "Eridan", Website: "https://eridan.io"},
				{Token: "ambiqmicroinc", Company: "Ambiq Micro", Website: "https://ambiq.com"},
				{Token: "alarmcom", Company: "Alarm.com", Website: "https://alarm.com"},
				{Token: "geotab", Company: "Geotab", Website: "https://www.geotab.com"},
			},
		},
		{
			ID: "productivity_collaboration", Name: "Productivity and collaboration", Parent: &tech,
			Boards: []Board{
				{Token: "asana", Company: "Asana", Website: "https://asana.com"},
				{Token: "airtable", Company: "Airtable", Website: "https://www.airtable.com"},
				{Token: "dropbox", Company: "Dropbox", Website: "https://www.dropbox.com"},
				{Token: "figma", Company: "Figma", Website: "https://www.figma.com"},
				{Token: "smartsheet", Company: "Smartsheet", Website: "https://www.smartsheet.com"},
				{Token: "calendly", Company: "Calendly", Website: "https://calendly.com"},
				{Token: "wrike", Company: "Wrike", Website: "https://www.wrike.com"},
				{Token: "mattermost", Company: "Mattermost", Website: "https://mattermost.com"},
				{Token: "boxinc", Company: "Box", Website: "https://www.box.com"},
			},
		},
		{
			ID: "social_creator", Name: "Social and creator platforms", Parent: &tech,
			Boards: []Board{
				{Token: "discord", Company: "Discord", Website: "https://discord.com"},
				{Token: "reddit", Company: "Reddit", Website: "https://www.reddit.com"},
				{Token: "pinterest", Company: "Pinterest", Website: "https://www.pinterest.com"},
				{Token: "twitch", Company: "Twitch", Website: "https://www.twitch.tv"},
				{Token: "medium", Company: "Medium", Website: "https://medium.com"},
				{Token: "ghost", Company: "Ghost", Website: "https://ghost.org"},
				{Token: "distrokid", Company: "DistroKid", Website: "https://distrokid.com"},
				{Token: "nextdoor", Company: "Nextdoor", Website: "https://nextdoor.com"},
			},
		},
		{
			ID: "customer_experience", Name: "Customer experience and support", Parent: &tech,
			Boards: []Board{
				{Token: "intercom", Company: "Fin (Intercom)", Website: "https://www.intercom.com"},
				{Token: "five9", Company: "Five9", Website: "https://www.five9.com"},
				{Token: "dialpad", Company: "Dialpad", Website: "https://www.dialpad.com"},
				{Token: "nice", Company: "NICE", Website: "https://www.nice.com"},
				{Token: "gongio", Company: "Gong", Website: "https://www.gong.io"},
				{Token: "twilio", Company: "Twilio", Website: "https://www.twilio.com"},
				{Token: "nextiva", Company: "Nextiva", Website: "https://www.nextiva.com"},
				{Token: "cresta", Company: "Cresta", Website: "https://cresta.com"},
				{Token: "polyai", Company: "PolyAI", Website: "https://poly.ai"},
			},
		},
		{
			ID: "payments_banking", Name: "Payments and banking",
			Boards: []Board{
				{Token: "stripe", Company: "Stripe", Website: "https://stripe.com"},
				{Token: "adyen", Company: "Adyen", Website: "https://www.adyen.com"},
				{Token: "affirm", Company: "Affirm", Website: "https://www.affirm.com"},
				{Token: "chime", Company: "Chime", Website: "https://www.chime.com"},
				{Token: "sofi", Company: "SoFi", Website: "https://www.sofi.com"},
				{Token: "brex", Company: "Brex", Website: "https://www.brex.com"},
				{Token: "mercury", Company: "Mercury", Website: "https://mercury.com"},
				{Token: "monzo", Company: "Monzo", Website: "https://monzo.com"},
				{Token: "n26", Company: "N26", Website: "https://n26.com"},
				{Token: "gocardless", Company: "GoCardless", Website: "https://gocardless.com"},
				{Token: "lithic", Company: "Lithic", Website: "https://www.lithic.com"},
				{Token: "crossriverbank", Company: "Cross River", Website: "https://www.crossriver.com"},
				{Token: "block", Company: "Block", Website: "https://block.xyz"},
			},
		},
		{
			ID: "venture_investing", Name: "Venture capital and investing",
			Boards: []Board{
				{Token: "generalatlantic", Company: "General Atlantic", Website: "https://www.generalatlantic.com"},
				{Token: "point72", Company: "Point72", Website: "https://point72.com"},
				{Token: "aqr", Company: "AQR", Website: "https://www.aqr.com"},
				{Token: "bridgewater89", Company: "Bridgewater Associates", Website: "https://www.bridgewater.com"},
				{Token: "gcmgrosvenor", Company: "GCM Grosvenor", Website: "https://www.gcmgrosvenor.com"},
				{Token: "janestreet", Company: "Jane Street", Website: "https://www.janestreet.com"},
				{Token: "jumptrading", Company: "Jump Trading", Website: "https://www.jumptrading.com"},
				{Token: "cais", Company: "CAIS", Website: "https://www.caisgroup.com"},
				{Token: "carta", Company: "Carta", Website: "https://carta.com"},
			},
		},
		{
			ID: "supply_chain", Name: "Supply chain and procurement",
			Boards: []Board{
				{Token: "samsara", Company: "Samsara", Website: "https://www.samsara.com"},
				{Token: "gomotive", Company: "Motive", Website: "https://gomotive.com"},
				{Token: "flexport", Company: "Flexport", Website: "https://www.flexport.com"},
				{Token: "project44", Company: "project44", Website: "https://www.project44.com"},
				{Token: "fourkites", Company: "FourKites", Website: "https://www.fourkites.com"},
				{Token: "ivalua", Company: "Ivalua", Website: "https://www.ivalua.com"},
				{Token: "fairmarkit", Company: "Fairmarkit", Website: "https://www.fairmarkit.com"},
				{Token: "relex", Company: "RELEX Solutions", Website: "https://www.relexsolutions.com"},
				{Token: "afresh", Company: "Afresh", Website: "https://www.afresh.com"},
			},
		},
		{
			ID: "accounting_compliance", Name: "Accounting, tax and compliance",
			Boards: []Board{
				{Token: "carta", Company: "Carta", Website: "https://carta.com"},
				{Token: "gusto", Company: "Gusto", Website: "https://gusto.com"},
				{Token: "billcom", Company: "Bill", Website: "https://www.bill.com"},
				{Token: "highradius", Company: "HighRadius", Website: "https://www.highradius.com"},
				{Token: "avidxchangeinc", Company: "AvidXchange", Website: "https://www.avidxchange.com"},
				{Token: "thoropass", Company: "Thoropass", Website: "https://www.thoropass.com"},
				{Token: "hyperproof", Company: "Hyperproof", Website: "https://hyperproof.io"},
				{Token: "complyadvantage", Company: "ComplyAdvantage", Website: "https://complyadvantage.com"},
			},
		},
		{
			ID: "consumer_brands", Name: "Consumer goods and CPG",
			Boards: []Board{
				{Token: "peloton", Company: "Peloton", Website: "https://www.onepeloton.com"},
				{Token: "yeti", Company: "YETI", Website: "https://www.yeti.com"},
				{Token: "glossier", Company: "Glossier", Website: "https://www.glossier.com"},
				{Token: "harrys", Company: "Harry's", Website: "https://www.harrys.com"},
				{Token: "olipop", Company: "OLIPOP", Website: "https://drinkolipop.com"},
				{Token: "sharkninjaoperatingllc", Company: "SharkNinja", Website: "https://www.sharkninja.com"},
				{Token: "drsquatch", Company: "Dr. Squatch", Website: "https://www.drsquatch.com"},
				{Token: "bombas", Company: "Bombas", Website: "https://bombas.com"},
				{Token: "supergoop", Company: "Supergoop!", Website: "https://supergoop.com"},
			},
		},
		{
			ID: "hospitality_events", Name: "Events, dining and venues",
			Boards: []Board{
				{Token: "airbnb", Company: "Airbnb", Website: "https://www.airbnb.com"},
				{Token: "toast", Company: "Toast", Website: "https://pos.toasttab.com"},
				{Token: "opentable", Company: "OpenTable", Website: "https://www.opentable.com"},
				{Token: "seatgeek", Company: "SeatGeek", Website: "https://seatgeek.com"},
				{Token: "aegworldwide", Company: "AEG Worldwide", Website: "https://aegworldwide.com"},
				{Token: "axs", Company: "AXS", Website: "https://axs.com"},
				{Token: "dorsia", Company: "Dorsia", Website: "https://www.dorsia.com"},
				{Token: "sohohouseco", Company: "Soho House & Co.", Website: "https://www.sohohouse.com"},
			},
		},
		{
			ID: "data_analytics_bi", Name: "Data analytics and BI", Parent: &tech,
			Boards: []Board{
				{Token: "sigmacomputing", Company: "Sigma Computing", Website: "https://www.sigmacomputing.com"},
				{Token: "hex", Company: "Hex", Website: "https://hex.tech"},
				{Token: "thoughtspot", Company: "ThoughtSpot", Website: "https://www.thoughtspot.com"},
				{Token: "domo", Company: "Domo", Website: "https://www.domo.com"},
				{Token: "metabase", Company: "Metabase", Website: "https://www.metabase.com"},
				{Token: "omni", Company: "Omni", Website: "https://omni.co"},
				{Token: "preset", Company: "Preset", Website: "https://preset.io"},
				{Token: "amplitude", Company: "Amplitude", Website: "https://amplitude.com"},
				{Token: "mixpanel", Company: "Mixpanel", Website: "https://mixpanel.com"},
				{Token: "pendo", Company: "Pendo", Website: "https://www.pendo.io"},
				{Token: "heap", Company: "Heap", Website: "https://www.heap.io"},
				{Token: "fullstory", Company: "FullStory", Website: "https://www.fullstory.com"},
			},
		},
		{
			ID: "ecommerce_enablement", Name: "E-commerce platforms and tech", Parent: &tech,
			Boards: []Board{
				{Token: "shopify", Company: "Shopify", Website: "https://www.shopify.com"},
				{Token: "bigcommerce", Company: "BigCommerce", Website: "https://www.bigcommerce.com"},
				{Token: "commercetools", Company: "commercetools", Website: "https://commercetools.com"},
				{Token: "klaviyo", Company: "Klaviyo", Website: "https://www.klaviyo.com"},
				{Token: "attentive", Company: "Attentive", Website: "https://www.attentive.com"},
				{Token: "postscript", Company: "Postscript", Website: "https://postscript.io"},
				{Token: "yotpo", Company: "Yotpo", Website: "https://www.yotpo.com"},
				{Token: "mirakl", Company: "Mirakl", Website: "https://www.mirakl.com"},
				{Token: "faire", Company: "Faire", Website: "https://www.faire.com"},
				{Token: "bloomreach", Company: "Bloomreach", Website: "https://www.bloomreach.com"},
				{Token: "stackline", Company: "Stackline", Website: "https://www.stackline.com"},
			},
		},
		{
			ID: "marketplace_gig", Name: "Marketplaces and gig economy", Parent: &tech,
			Boards: []Board{
				{Token: "airbnb", Company: "Airbnb", Website: "https://www.airbnb.com"},
				{Token: "doordash", Company: "DoorDash", Website: "https://www.doordash.com"},
				{Token: "instacart", Company: "Instacart", Website: "https://www.instacart.com"},
				{Token: "lyft", Company: "Lyft", Website: "https://www.lyft.com"},
				{Token: "uber", Company: "Uber", Website: "https://www.uber.com"},
				{Token: "etsy", Company: "Etsy", Website: "https://www.etsy.com"},
				{Token: "thumbtack", Company: "Thumbtack", Website: "https://www.thumbtack.com"},
				{Token: "taskrabbit", Company: "TaskRabbit", Website: "https://www.taskrabbit.com"},
				{Token: "rover", Company: "Rover", Website: "https://www.rover.com"},
				{Token: "instawork", Company: "Instawork", Website: "https://www.instawork.com"},
				{Token: "offerup", Company: "OfferUp", Website: "https://offerup.com"},
				{Token: "poshmark", Company: "Poshmark", Website: "https://poshmark.com"},
				{Token: "whatnot", Company: "Whatnot", Website: "https://www.whatnot.com"},
			},
		},
		{
			ID: "pet_veterinary", Name: "Pet care and veterinary",
			Boards: []Board{
				{Token: "chewy", Company: "Chewy", Website: "https://www.chewy.com"},
				{Token: "rover", Company: "Rover", Website: "https://www.rover.com"},
				{Token: "butternutbox", Company: "Butternut Box", Website: "https://butternutbox.com"},
				{Token: "farmersdog", Company: "The Farmer's Dog", Website: "https://www.thefarmersdog.com"},
				{Token: "bark", Company: "BARK", Website: "https://bark.co"},
				{Token: "embarkvet", Company: "Embark Veterinary", Website: "https://embarkvet.com"},
				{Token: "splootvet", Company: "Sploot Veterinary Care", Website: "https://www.splootvets.com"},
				{Token: "modernanimal", Company: "Modern Animal", Website: "https://www.modernanimal.com"},
				{Token: "bondvet", Company: "Bond Vet", Website: "https://bondvet.com"},
				{Token: "dutchpet", Company: "Dutch", Website: "https://www.dutch.com"},
			},
		},
		{
			ID: "architecture_civil", Name: "Architecture and civil engineering",
			Boards: []Board{
				{Token: "autodesk", Company: "Autodesk", Website: "https://www.autodesk.com"},
				{Token: "bentley", Company: "Bentley Systems", Website: "https://www.bentley.com"},
				{Token: "procore", Company: "Procore", Website: "https://www.procore.com"},
				{Token: "trimble", Company: "Trimble", Website: "https://www.trimble.com"},
				{Token: "cannondesign", Company: "CannonDesign", Website: "https://www.cannondesign.com"},
				{Token: "bgeinc", Company: "BGE, Inc", Website: "https://www.bgeinc.com"},
				{Token: "brph", Company: "BRPH", Website: "https://www.brph.com"},
				{Token: "atwellgroup", Company: "Atwell, LLC", Website: "https://www.atwell-group.com"},
				{Token: "gensler", Company: "Gensler", Website: "https://www.gensler.com"},
				{Token: "aecom", Company: "AECOM", Website: "https://aecom.com"},
			},
		},
		{
			ID: "aviation_maritime", Name: "Aviation and maritime",
			Boards: []Board{
				{Token: "joby", Company: "Joby Aviation", Website: "https://www.jobyaviation.com"},
				{Token: "boom", Company: "Boom Supersonic", Website: "https://boomsupersonic.com"},
				{Token: "beta", Company: "BETA Technologies", Website: "https://www.beta.team"},
				{Token: "heartaerospace", Company: "Heart Aerospace", Website: "https://www.heartaerospace.com"},
				{Token: "wisk", Company: "Wisk Aero", Website: "https://wisk.aero"},
				{Token: "regent", Company: "Regent", Website: "https://www.regentcraft.com"},
				{Token: "saronic", Company: "Saronic", Website: "https://www.saronic.com"},
				{Token: "skyryse", Company: "Skyryse", Website: "https://www.skyryse.com"},
				{Token: "flyzipline", Company: "Zipline", Website: "https://www.zipline.com"},
				{Token: "saildrone", Company: "Saildrone", Website: "https://www.saildrone.com"},
				{Token: "shone", Company: "Orca AI", Website: "https://www.orca-ai.io"},
			},
		},
	}
	existing := map[string]bool{}
	for _, c := range file.Categories {
		existing[c.ID] = true
	}
	for _, c := range extras {
		if !existing[c.ID] {
			file.Categories = append(file.Categories, c)
		}
	}
}

// Categories returns the curated categories in file order.
func Categories() []Category { return file.Categories }

// VerifiedAt is the date the lists were last checked against the live API.
func VerifiedAt() string { return file.VerifiedAt }

func FindCategory(id string) (Category, bool) {
	for _, c := range file.Categories {
		if c.ID == id {
			return c, true
		}
	}
	return Category{}, false
}

// BoardCount counts the unique boards a category covers when searched, including its subsets.
func BoardCount(id string) int { return len(BoardsFor([]string{id})) }

// BoardsFor returns the de-duplicated boards of the given categories. A parent category includes
// the boards of every subset. A subset covers only itself.
func BoardsFor(ids []string) []Board {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	seen := map[string]bool{}
	var out []Board
	for _, c := range file.Categories {
		in := want[c.ID] || c.Parent != nil && want[*c.Parent]
		if !in {
			continue
		}
		for _, b := range c.Boards {
			if !seen[b.Token] {
				seen[b.Token] = true
				out = append(out, b)
			}
		}
	}
	return out
}

// lookup returns the curated entry for a token. Tokens are matched without regard to case,
// as Greenhouse does.
func lookup(token string) (Board, bool) {
	for _, c := range file.Categories {
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

// WebsiteFor returns the curated company website for a token, or "".
func WebsiteFor(token string) string {
	b, _ := lookup(token)
	return b.Website
}

// Tokens returns every curated token, sorted.
func Tokens() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range file.Categories {
		for _, b := range c.Boards {
			if !seen[b.Token] {
				seen[b.Token] = true
				out = append(out, b.Token)
			}
		}
	}
	sort.Strings(out)
	return out
}

// CompanyNames lists the curated company names, sorted and without duplicates.
func CompanyNames() []string {
	seen := map[string]bool{}
	names := []string{}
	for _, c := range file.Categories {
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
