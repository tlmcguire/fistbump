package ai

import (
	_ "embed"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

//go:embed catalog.json
var catalogJSON []byte

type CatalogModel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Repo      string `json:"repo"`
	File      string `json:"file"`
	Revision  string `json:"revision"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	MinRAMGB  int    `json:"min_ram_gb"`
}

// ModelInfo is the API view of a catalog entry.
type ModelInfo struct {
	CatalogModel
	Installed       bool `json:"installed"`
	FitsThisMachine bool `json:"fits_this_machine"`
	Selected        bool `json:"selected"`
}

func Catalog() []CatalogModel {
	var c struct {
		Models []CatalogModel `json:"models"`
	}
	_ = json.Unmarshal(catalogJSON, &c)
	return c.Models
}

func FindModel(id string) (CatalogModel, bool) {
	for _, m := range Catalog() {
		if m.ID == id {
			return m, true
		}
	}
	return CatalogModel{}, false
}

func ModelsDir(dataDir string) string { return filepath.Join(dataDir, "models") }

func ModelPath(dataDir string, m CatalogModel) string {
	return filepath.Join(ModelsDir(dataDir), m.File)
}

func IsInstalled(dataDir string, m CatalogModel) bool {
	st, err := os.Stat(ModelPath(dataDir, m))
	return err == nil && !st.IsDir()
}

// List returns the catalog with installed and fit state.
func List(dataDir, selected string) []ModelInfo {
	ram := TotalRAMGB()
	out := []ModelInfo{}
	for _, m := range Catalog() {
		out = append(out, ModelInfo{CatalogModel: m, Installed: IsInstalled(dataDir, m),
			FitsThisMachine: ram == 0 || ram >= float64(m.MinRAMGB), Selected: m.ID == selected})
	}
	return out
}

// TotalRAMGB returns physical memory in GB, or 0 when it cannot be determined.
func TotalRAMGB() float64 {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		n, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		return n / (1 << 30)
	case "linux":
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				if f := strings.Fields(line); len(f) >= 2 {
					kb, _ := strconv.ParseFloat(f[1], 64)
					return kb / (1 << 20)
				}
			}
		}
	case "windows":
		out, err := exec.Command("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory").Output()
		if err != nil {
			return 0
		}
		n, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		return n / (1 << 30)
	}
	return 0
}
