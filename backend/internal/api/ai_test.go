package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
	"github.com/tlmcguire/fistbump/backend/internal/db"
)

const secretKey = "sk-live-do-not-leak-0123456789"

func newAIServer(t *testing.T) (*Server, *sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "fistbump.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return New(Config{DB: conn, DataDir: dir, Token: testToken, Version: "test"}), conn, dir
}

func status(t *testing.T, s *Server) aiStatus {
	t.Helper()
	rec := call(t, s, "GET", "/v1/ai/status", "")
	expect(t, rec, 200)
	var st aiStatus
	json.Unmarshal(rec.Body.Bytes(), &st)
	return st
}

func TestAIStatusDefaults(t *testing.T) {
	s, _, _ := newAIServer(t)
	st := status(t, s)
	if st.Mode != "auto" || st.ActiveEngine != "rules" || st.Remote.Configured || st.Local.Installed || st.Local.Running {
		t.Errorf("status = %+v", st)
	}
}

func TestRemoteSetAndClear(t *testing.T) {
	s, conn, _ := newAIServer(t)

	// Ollama on this computer: no key needed, and auto now uses it.
	rec := call(t, s, "PUT", "/v1/ai/remote", `{"base_url":"http://127.0.0.1:11434/v1/","model":"llama3.2"}`)
	expect(t, rec, 200)
	st := status(t, s)
	if !st.Remote.Configured || st.Remote.NeedsKey || st.Remote.HasKey || st.Remote.BaseURL != "http://127.0.0.1:11434/v1" || st.ActiveEngine != "remote" {
		t.Errorf("after Ollama: %+v", st)
	}

	// A hosted provider with a key: the key is never echoed, stored or shown in settings.
	rec = call(t, s, "PUT", "/v1/ai/remote", `{"base_url":"https://api.example.com/v1","model":"m","api_key":"`+secretKey+`"}`)
	expect(t, rec, 200)
	if strings.Contains(rec.Body.String(), secretKey) {
		t.Error("the response echoes the key")
	}
	st = status(t, s)
	if !st.Remote.HasKey || st.Remote.NeedsKey {
		t.Errorf("after hosted: %+v", st)
	}
	if body := call(t, s, "GET", "/v1/settings", "").Body.String(); strings.Contains(body, secretKey) || !strings.Contains(body, "https://api.example.com/v1") {
		t.Errorf("settings: %s", body)
	}
	var n int
	conn.QueryRow(`SELECT COUNT(*) FROM settings WHERE value LIKE ? OR key LIKE ?`, "%"+secretKey+"%", "%key%").Scan(&n)
	if n != 0 {
		t.Error("the key reached the database")
	}

	expect(t, call(t, s, "DELETE", "/v1/ai/remote", ""), 204)
	st = status(t, s)
	if st.Remote.Configured || st.Remote.HasKey || st.ActiveEngine != "rules" {
		t.Errorf("after clear: %+v", st)
	}
}

func TestRemoteValidation(t *testing.T) {
	s, _, _ := newAIServer(t)
	rec := call(t, s, "PUT", "/v1/ai/remote", `{"base_url":"http://api.example.com/v1","model":"x","api_key":"k"}`)
	expectField(t, rec, "base_url")
	if !strings.Contains(rec.Body.String(), "https unless the host is this computer") {
		t.Errorf("message: %s", rec.Body)
	}
	expectField(t, call(t, s, "PUT", "/v1/ai/remote", `{"base_url":"https://api.example.com/v1"}`), "model")
	expectField(t, call(t, s, "PUT", "/v1/settings", `{"ai.remote.base_url":"http://api.example.com/v1"}`), "ai.remote.base_url")
}

func TestHostedProviderWithoutKeyIsSkipped(t *testing.T) {
	s, _, _ := newAIServer(t)
	expect(t, call(t, s, "PUT", "/v1/ai/remote", `{"base_url":"https://api.example.com/v1","model":"m"}`), 200)
	st := status(t, s)
	if !st.Remote.NeedsKey || st.ActiveEngine != "rules" {
		t.Errorf("status = %+v", st)
	}
	expect(t, call(t, s, "PUT", "/v1/ai/mode", `{"mode":"remote"}`), 200)
	if st := status(t, s); st.Mode != "remote" || st.ActiveEngine != "" {
		t.Errorf("forced remote without a key: %+v", st)
	}
}

func TestAIModeValidation(t *testing.T) {
	s, _, _ := newAIServer(t)
	expectField(t, call(t, s, "PUT", "/v1/ai/mode", `{"mode":"fast"}`), "mode")
	expect(t, call(t, s, "PUT", "/v1/ai/mode", `{"mode":"rules"}`), 200)
	if status(t, s).Mode != "rules" {
		t.Error("mode not saved")
	}
}

func TestAITestRemote(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "pong"}}}})
	}))
	defer upstream.Close()
	s, _, _ := newAIServer(t)

	out := expect(t, call(t, s, "POST", "/v1/ai/test", `{"engine":"remote"}`), 200)
	if out["ok"] != false || out["error"] == nil {
		t.Errorf("unconfigured test: %v", out)
	}
	expect(t, call(t, s, "PUT", "/v1/ai/remote", `{"base_url":"`+upstream.URL+`/v1","model":"m"}`), 200)
	out = expect(t, call(t, s, "POST", "/v1/ai/test", `{"engine":"remote"}`), 200)
	if out["ok"] != true || out["sample"] != "pong" || out["error"] != nil {
		t.Errorf("test: %v", out)
	}
	out = expect(t, call(t, s, "POST", "/v1/ai/test", `{"engine":"local"}`), 200)
	if out["ok"] != false {
		t.Errorf("local with no llama-server: %v", out)
	}
	expectField(t, call(t, s, "POST", "/v1/ai/test", `{"engine":"rules"}`), "engine")
}

func installFakeModel(t *testing.T, dir string, m ai.Model) {
	t.Helper()
	os.MkdirAll(ai.ModelsDir(dir), 0o700)
	f, err := os.Create(m.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(m.SizeBytes) // sparse, so it is instant
	f.Close()
}

func TestModelsListSelectAndDelete(t *testing.T) {
	s, _, dir := newAIServer(t)
	rec := call(t, s, "GET", "/v1/models", "")
	expect(t, rec, 200)
	var list []modelEntry
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != len(ai.Catalog()) || list[0].ID != "qwen2.5-1.5b-instruct-q4" || list[0].Installed || list[0].Selected {
		t.Fatalf("list = %+v", list)
	}
	id := list[0].ID

	expectField(t, call(t, s, "PUT", "/v1/settings", `{"ai.selected_model":"`+id+`"}`), "ai.selected_model")
	expectField(t, call(t, s, "PUT", "/v1/settings", `{"ai.selected_model":"nope"}`), "ai.selected_model")

	installFakeModel(t, dir, ai.Catalog()[0])
	expect(t, call(t, s, "PUT", "/v1/settings", `{"ai.selected_model":"`+id+`"}`), 200)
	got := expect(t, call(t, s, "GET", "/v1/models/"+id, ""), 200)
	if got["installed"] != true || got["selected"] != true {
		t.Errorf("model = %v", got)
	}
	if st := status(t, s); st.Local.Model != id || !st.Local.Installed {
		t.Errorf("status local = %+v", st.Local)
	}

	expect(t, call(t, s, "POST", "/v1/models/"+id+"/download", ""), 409) // already installed
	expect(t, call(t, s, "DELETE", "/v1/models/"+id, ""), 204)
	if _, err := os.Stat(ai.Catalog()[0].Path(dir)); !os.IsNotExist(err) {
		t.Error("the file should be deleted")
	}
	if st := status(t, s); st.Local.Model != "" {
		t.Errorf("selection should be cleared: %+v", st.Local)
	}
	expect(t, call(t, s, "DELETE", "/v1/models/"+id, ""), 404)
}

func TestModelRoutesNotFound(t *testing.T) {
	s, _, _ := newAIServer(t)
	expect(t, call(t, s, "GET", "/v1/models/nope", ""), 404)
	expect(t, call(t, s, "POST", "/v1/models/nope/download", ""), 404)
	expect(t, call(t, s, "GET", "/v1/models/downloads/d99", ""), 404)
	expect(t, call(t, s, "DELETE", "/v1/models/downloads/d99", ""), 404)
	expect(t, call(t, s, "PUT", "/v1/models/token", `{"token":"hf_x"}`), 501)
}
