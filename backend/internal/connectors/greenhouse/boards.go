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
				{Token: "digitalocean", Company: "DigitalOcean", Website: "https://www.digitalocean.com"},
				{Token: "fastly", Company: "Fastly", Website: "https://www.fastly.com"},
				{Token: "backblaze", Company: "Backblaze", Website: "https://www.backblaze.com"},
			},
		},
		{
			ID: "design_creative", Name: "Design and creative tools", Parent: &tech,
			Boards: []Board{
				{Token: "figma", Company: "Figma", Website: "https://www.figma.com"},
				{Token: "miro", Company: "Miro", Website: "https://miro.com"},
				{Token: "squarespace", Company: "Squarespace", Website: "https://www.squarespace.com"},
			},
		},
		{
			ID: "supply_chain", Name: "Supply chain and procurement",
			Boards: []Board{
				{Token: "samsara", Company: "Samsara", Website: "https://www.samsara.com"},
				{Token: "motive", Company: "Motive", Website: "https://gomotive.com"},
			},
		},
		{
			ID: "accounting_compliance", Name: "Accounting, tax and compliance",
			Boards: []Board{
				{Token: "carta", Company: "Carta", Website: "https://carta.com"},
				{Token: "gusto", Company: "Gusto", Website: "https://gusto.com"},
			},
		},
		{
			ID: "consumer_brands", Name: "Consumer goods and CPG",
			Boards: []Board{
				{Token: "peloton", Company: "Peloton", Website: "https://www.onepeloton.com"},
				{Token: "yeti", Company: "YETI", Website: "https://www.yeti.com"},
			},
		},
		{
			ID: "hospitality_events", Name: "Events, dining and venues",
			Boards: []Board{
				{Token: "airbnb", Company: "Airbnb", Website: "https://www.airbnb.com"},
				{Token: "toast", Company: "Toast", Website: "https://pos.toasttab.com"},
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
