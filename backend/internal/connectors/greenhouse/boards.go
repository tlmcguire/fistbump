package greenhouse

import (
	_ "embed"
	"encoding/json"
	"sort"
)

//go:embed boards.json
var boardsJSON []byte

type Board struct {
	Token   string `json:"token"`
	Company string `json:"company"`
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

// CompanyFor returns the curated company name for a token, or "".
func CompanyFor(token string) string {
	for _, c := range file.Categories {
		for _, b := range c.Boards {
			if b.Token == token {
				return b.Company
			}
		}
	}
	return ""
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
