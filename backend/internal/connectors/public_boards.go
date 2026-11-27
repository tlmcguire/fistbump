package connectors

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
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
	Version     int                   `json:"version"`
	UpdatedAt   string                `json:"updated_at"`
	RefreshDays int                   `json:"refresh_days"`
	FeedURL     string                `json:"feed_url"`
	Sources     []PublicBoardSource   `json:"sources"`
	Boards      map[string][]PublicBoard `json:"boards"`
}

var (
	publicBoardsMu    sync.RWMutex
	activePublicCat   PublicBoardCatalog
	publicCompanyIdx  = map[string]map[string]string{}
	publicTokenRe     = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
)

func init() {
	var cat PublicBoardCatalog
	if err := json.Unmarshal(bundledPublicBoardsJSON, &cat); err != nil {
		panic("connectors public_boards.json: " + err.Error())
	}
	setPublicCatalog(cat)
}

func setPublicCatalog(cat PublicBoardCatalog) {
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
	activePublicCat = cat
	publicCompanyIdx = idx
	publicBoardsMu.Unlock()
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
	setPublicCatalog(cat)
	return true
}

// ApplyPublicBoardsJSON validates raw JSON, activates it in memory, and caches it to dataDir.
func ApplyPublicBoardsJSON(dataDir string, raw []byte) bool {
	var cat PublicBoardCatalog
	if err := json.Unmarshal(raw, &cat); err != nil || cat.Version != 1 {
		return false
	}
	setPublicCatalog(cat)
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
		feedURL = CurrentPublicBoardCatalog().FeedURL
	}
	if feedURL == "" {
		return false
	}
	if !strings.HasPrefix(feedURL, "https://") {
		return false
	}
	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 || req.URL.Scheme != "https" || !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}
	}
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, feedURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	req.Header.Set("User-Agent", "Fistbump")
	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("public ATS boards refresh skipped", "error", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
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
