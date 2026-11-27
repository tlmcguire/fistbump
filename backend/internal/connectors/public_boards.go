package connectors

import (
	"context"
	_ "embed"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/catalogfeed"
)

//go:embed public_boards.json
var bundledPublicBoardsJSON []byte

// PublicBoard is a company board discovered from an open public ATS dataset.
type PublicBoard struct {
	Token   string `json:"token"`
	Company string `json:"company"`
}

// PublicBoardSource records one upstream dataset checked on a 30-day cadence.
type PublicBoardSource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	UpdatedAt   string `json:"updated_at"`
	BoardsAdded int    `json:"boards_added"`
}

// PublicBoardCatalog is the schema of public_boards.json and the cached <dataDir>/public-ats-boards.json.
type PublicBoardCatalog struct {
	Version     int                      `json:"version"`
	UpdatedAt   string                   `json:"updated_at"`
	RefreshDays int                      `json:"refresh_days"`
	FeedURL     string                   `json:"feed_url"`
	Sources     []PublicBoardSource      `json:"sources"`
	Boards      map[string][]PublicBoard `json:"boards"`
}

var (
	publicBoardsMu      sync.RWMutex
	activePublicCat     PublicBoardCatalog
	publicCompanyIdx    = map[string]map[string]string{}
	publicTokenRe       = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	bundledBoardFeedURL string
)

func init() {
	var cat PublicBoardCatalog
	if err := json.Unmarshal(bundledPublicBoardsJSON, &cat); err != nil {
		panic("connectors public_boards.json: " + err.Error())
	}
	bundledBoardFeedURL = cat.FeedURL
	setPublicCatalog(cat)
}

func setPublicCatalog(cat PublicBoardCatalog) bool {
	if cat.RefreshDays <= 0 {
		cat.RefreshDays = 30
	}
	if cat.Boards == nil {
		cat.Boards = map[string][]PublicBoard{}
	}
	idx := map[string]map[string]string{}
	clean := map[string][]PublicBoard{}
	for _, ats := range []string{"greenhouse", "lever", "ashby"} {
		seen := map[string]bool{}
		idx[ats] = map[string]string{}
		for _, b := range cat.Boards[ats] {
			tok := strings.ToLower(strings.TrimSpace(b.Token))
			if !publicTokenRe.MatchString(tok) || seen[tok] {
				continue
			}
			seen[tok] = true
			comp := strings.TrimSpace(b.Company)
			if comp == "" {
				comp = tok
			}
			clean[ats] = append(clean[ats], PublicBoard{Token: tok, Company: comp})
			idx[ats][tok] = comp
		}
	}
	cat.Boards = clean
	publicBoardsMu.Lock()
	defer publicBoardsMu.Unlock()
	if activePublicCat.UpdatedAt != "" && cat.UpdatedAt < activePublicCat.UpdatedAt {
		return false
	}
	activePublicCat = cat
	publicCompanyIdx = idx
	return true
}

// PublicBoards returns the active public-dataset boards for the given ATS ("greenhouse", "lever", "ashby").
func PublicBoards(ats string) []PublicBoard {
	publicBoardsMu.RLock()
	defer publicBoardsMu.RUnlock()
	list := activePublicCat.Boards[strings.ToLower(strings.TrimSpace(ats))]
	out := make([]PublicBoard, len(list))
	copy(out, list)
	return out
}

// PublicCompanyFor returns the company name for a public-dataset board token, or "".
func PublicCompanyFor(ats, token string) string {
	publicBoardsMu.RLock()
	defer publicBoardsMu.RUnlock()
	if m := publicCompanyIdx[strings.ToLower(strings.TrimSpace(ats))]; m != nil {
		return m[strings.ToLower(strings.TrimSpace(token))]
	}
	return ""
}

// CurrentPublicBoardCatalog returns a snapshot of the active public board catalog metadata.
func CurrentPublicBoardCatalog() PublicBoardCatalog {
	publicBoardsMu.RLock()
	defer publicBoardsMu.RUnlock()
	return activePublicCat
}

// PublicBoardsNeedRefresh reports whether RefreshDays (default 30) have elapsed since UpdatedAt.
func PublicBoardsNeedRefresh(cat PublicBoardCatalog, now time.Time) bool {
	days := cat.RefreshDays
	if days <= 0 {
		days = 30
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(cat.UpdatedAt))
	if err != nil {
		return true
	}
	return now.Sub(t) >= time.Duration(days)*24*time.Hour
}

// LoadCachedPublicBoards loads <dataDir>/public-ats-boards.json if it exists and is valid.
func LoadCachedPublicBoards(dataDir string) bool {
	if dataDir == "" {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, "public-ats-boards.json"))
	if err != nil {
		return false
	}
	var cat PublicBoardCatalog
	if err := json.Unmarshal(raw, &cat); err != nil || cat.Version != 1 {
		return false
	}
	if _, err := time.Parse(time.DateOnly, cat.UpdatedAt); err != nil {
		return false
	}
	return setPublicCatalog(cat)
}

// ApplyPublicBoardsJSON validates raw JSON, activates it in memory, and caches it to dataDir.
func ApplyPublicBoardsJSON(dataDir string, raw []byte) bool {
	var cat PublicBoardCatalog
	if err := json.Unmarshal(raw, &cat); err != nil || cat.Version != 1 {
		return false
	}
	if _, err := time.Parse(time.DateOnly, cat.UpdatedAt); err != nil || !setPublicCatalog(cat) {
		return false
	}
	if dataDir != "" {
		_ = os.MkdirAll(dataDir, 0o700)
		_ = os.WriteFile(filepath.Join(dataDir, "public-ats-boards.json"), raw, 0o600)
	}
	return true
}

// RefreshPublicBoardsNow fetches feedURL (or the catalog's configured FeedURL), validates it,
// activates it in memory, and caches it to <dataDir>/public-ats-boards.json.
func RefreshPublicBoardsNow(ctx context.Context, dataDir, feedURL string, client *http.Client) bool {
	if feedURL == "" {
		feedURL = bundledBoardFeedURL
	}
	if feedURL == "" {
		return false
	}
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body, err := catalogfeed.Get(reqCtx, client, feedURL, 4<<20)
	if err != nil {
		slog.Debug("public ATS boards refresh skipped", "error", err)
		return false
	}
	if !ApplyPublicBoardsJSON(dataDir, body) {
		return false
	}
	cat := CurrentPublicBoardCatalog()
	slog.Info("refreshed public ATS board catalog", "updated_at", cat.UpdatedAt,
		"greenhouse", len(cat.Boards["greenhouse"]), "lever", len(cat.Boards["lever"]), "ashby", len(cat.Boards["ashby"]))
	return true
}

// StartBoardUpdater loads any cached public-ats-boards.json from dataDir and checks in the
// background every 24 hours whether 30 days have elapsed since the last refresh.
func StartBoardUpdater(ctx context.Context, dataDir, feedURL string) {
	LoadCachedPublicBoards(dataDir)
	go func() {
		check := func() {
			if PublicBoardsNeedRefresh(CurrentPublicBoardCatalog(), time.Now().UTC()) {
				RefreshPublicBoardsNow(ctx, dataDir, feedURL, nil)
			}
		}
		check()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				check()
			}
		}
	}()
}
