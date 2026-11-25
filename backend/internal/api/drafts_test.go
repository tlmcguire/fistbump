package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/ai"
)

type fakeChatter struct {
	name  string
	ready bool
	calls int
	reply string
}

func (f *fakeChatter) Name() string { return f.name }
func (f *fakeChatter) Ready() bool  { return f.ready }
func (f *fakeChatter) Suggest(context.Context, ai.Request) ([]ai.Draft, error) {
	return nil, nil
}
func (f *fakeChatter) Chat(_ context.Context, _ []ai.Message, _ int, _ bool) (string, error) {
	f.calls++
	return f.reply, nil
}

func TestDraftsAIAndDeterministicModes(t *testing.T) {
	chatter := &fakeChatter{
		name:  ai.EngineLocal,
		ready: true,
		reply: "AI-generated draft text grounded in resume.",
	}
	s := newRoundServer(t, ai.Engines{Local: chatter, Rules: ai.Rules{}})

	rec := apiJSON(t, s, http.MethodPut, "/v1/resume/profile", map[string]any{
		"full_name": "Ada Lovelace",
		"skills":    []string{"go", "sql"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("put profile: %d %s", rec.Code, rec.Body.String())
	}
	rec = apiJSON(t, s, http.MethodPost, "/v1/jobs", map[string]any{
		"company_name":   "Acme",
		"position_title": "Backend Engineer",
		"source":         "pasted",
		"raw_text":       "Required: Go, SQL, Docker",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create job: %d %s", rec.Code, rec.Body.String())
	}
	var job struct{ ID int64 }
	_ = json.Unmarshal(rec.Body.Bytes(), &job)
	path := "/v1/jobs/" + strconv.FormatInt(job.ID, 10) + "/drafts"

	// 1. Deterministic mode must NOT call the AI chatter and must report engine "rules".
	rec = apiJSON(t, s, http.MethodPost, path, map[string]any{
		"kind":     "cover_letter",
		"prompt":   "Cover letter",
		"mode":     "deterministic",
		"generate": true,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("deterministic cover letter: %d %s", rec.Code, rec.Body.String())
	}
	if chatter.calls != 0 {
		t.Fatalf("expected 0 AI calls in deterministic mode, got %d", chatter.calls)
	}
	var detResp struct {
		Content string `json:"content"`
		Engine  string `json:"engine"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &detResp)
	if detResp.Engine != ai.EngineRules {
		t.Fatalf("expected engine rules, got %q", detResp.Engine)
	}

	// 2. AI mode must call the AI chatter for cover_letter, interview_prep, qa, and iteration, returning engine "local".
	for _, tc := range []map[string]any{
		{"kind": "cover_letter", "prompt": "Cover letter", "mode": "ai", "generate": true},
		{"kind": "interview_prep", "prompt": "Interview prep", "mode": "ai", "generate": true},
		{"kind": "qa", "prompt": "Why Acme?", "mode": "ai", "generate": true},
		{"kind": "cover_letter", "prompt": "Cover letter", "base_content": "Old text", "instruction": "Make it shorter", "mode": "ai", "generate": true},
	} {
		before := chatter.calls
		rec = apiJSON(t, s, http.MethodPost, path, tc)
		if rec.Code != http.StatusCreated {
			t.Fatalf("ai draft %v: %d %s", tc["kind"], rec.Code, rec.Body.String())
		}
		if chatter.calls != before+1 {
			t.Fatalf("expected AI chatter to be called for %v", tc["kind"])
		}
		var resp struct {
			Content string `json:"content"`
			Engine  string `json:"engine"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.Engine != ai.EngineLocal || !strings.Contains(resp.Content, "AI-generated") {
			t.Fatalf("unexpected AI draft response: %+v", resp)
		}
	}

	// 3. AI questions must call the AI chatter and return parsed questions with engine "local".
	chatter.reply = "Why are you excited about Acme's backend architecture?\nHow have you scaled Go services with SQL databases?\nHow would you ramp up on Docker in your first 30 days?"
	rec = apiJSON(t, s, http.MethodPost, path, map[string]any{
		"kind":     "questions",
		"mode":     "ai",
		"generate": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("ai questions: %d %s", rec.Code, rec.Body.String())
	}
	var qResp struct {
		Questions []string `json:"questions"`
		Engine    string   `json:"engine"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &qResp)
	if qResp.Engine != ai.EngineLocal || len(qResp.Questions) != 3 {
		t.Fatalf("unexpected AI questions response: %+v", qResp)
	}

	// 4. When no AI engine is ready, mode "ai" must return 503 ai_unavailable (never silently pretend AI ran).
	chatter.ready = false
	rec = apiJSON(t, s, http.MethodPost, path, map[string]any{
		"kind":     "cover_letter",
		"prompt":   "Cover letter",
		"mode":     "ai",
		"generate": true,
	})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when AI unavailable, got %d %s", rec.Code, rec.Body.String())
	}
}
