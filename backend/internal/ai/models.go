package ai

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
)

// Model is one entry of the catalog of models the app can download.
type Model struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Repo      string `json:"repo"`
	File      string `json:"file"`
	Revision  string `json:"revision"` // a commit hash, so the file never changes under us
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	MinRAMGB  int    `json:"min_ram_gb"`
	License   string `json:"license"`
}

//go:embed catalog.json
var catalogJSON []byte

var catalog = mustCatalog()

func mustCatalog() []Model {
	var out []Model
	if err := json.Unmarshal(catalogJSON, &out); err != nil {
		panic("ai: bad catalog.json: " + err.Error())
	}
	return out
}

// Catalog returns a copy of every model the app can download, default first.
func Catalog() []Model {
	return append([]Model(nil), catalog...)
}

// FindModel returns the catalog entry with this id.
func FindModel(id string) (Model, bool) {
	for _, m := range catalog {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// URL is where the model file is downloaded from, pinned to its revision.
func (m Model) URL() string {
	return "https://huggingface.co/" + m.Repo + "/resolve/" + m.Revision + "/" + m.File
}

// ModelsDir is the folder in the data dir that holds downloaded models.
func ModelsDir(dataDir string) string { return filepath.Join(dataDir, "models") }

// Path is where the finished model file lives.
func (m Model) Path(dataDir string) string { return filepath.Join(ModelsDir(dataDir), m.File) }

// Installed reports whether the model file is in place with the catalog size.
func Installed(dataDir, id string) bool {
	m, ok := FindModel(id)
	if !ok {
		return false
	}
	info, err := os.Stat(m.Path(dataDir))
	return err == nil && info.Mode().IsRegular() && info.Size() == m.SizeBytes
}

// FitsThisMachine compares the model's RAM need with this computer's total RAM. When the total
// cannot be read it reports true, so an unknown machine is not locked out.
func FitsThisMachine(m Model) bool {
	total := TotalRAM()
	if total == 0 {
		return true
	}
	return total >= uint64(m.MinRAMGB)<<30
}
