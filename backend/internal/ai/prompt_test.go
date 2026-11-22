package ai

import (
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/analyze"
	"github.com/tlmcguire/fistbump/backend/internal/models"
)

// promptRequest has a job whose posting wants skills the resume does not have.
func promptRequest() Request {
	a := analyze.Result{}
	a.Required.Tech.Matched = []analyze.Match{{Skill: "sql"}, {Skill: "python"}}
	a.Required.Tech.Missing = []string{"kubernetes", "terraform"}
	return Request{
		Job:      models.Job{PositionTitle: "Business Analyst", CompanyName: "Globex", RawText: strPtr("secret posting instructions")},
		Analysis: a,
		Profile:  &models.Profile{Skills: []string{"excel", "sql", "python"}},
		Resume:   testResume,
		Language: "en",
	}
}

func strPtr(s string) *string { return &s }

func TestBuildPromptNeverSendsMissingSkills(t *testing.T) {
	target := Target{Section: "experience", Label: "Data Analyst at Acme", Original: "- Cleaned and joined order data with SQL"}
	messages := BuildPrompt(target, promptRequest())
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want a system and a user message", len(messages))
	}
	all := messages[0].Content + "\n" + messages[1].Content
	for _, missing := range []string{"kubernetes", "terraform"} {
		if strings.Contains(strings.ToLower(all), missing) {
			t.Errorf("the prompt names the missing skill %q", missing)
		}
	}
	//the skills the resume does have are sent, so the model can work with them
	if !strings.Contains(all, "sql") {
		t.Error("the prompt leaves out the matched skills")
	}
}

func TestBuildPromptNeverSendsThePosting(t *testing.T) {
	target := Target{Section: "summary", Label: "Summary", Original: "Data analyst who builds reporting in SQL and Excel."}
	messages := BuildPrompt(target, promptRequest())
	all := messages[0].Content + "\n" + messages[1].Content
	if strings.Contains(all, "secret posting instructions") {
		t.Error("the prompt sends the posting's raw text")
	}
}

func TestParseReplyWithTextAroundTheJSON(t *testing.T) {
	cases := []struct {
		name, reply, proposed, reason string
		ok                            bool
	}{
		{"plain json", `{"proposed": "Analyst who builds reporting", "reason": "Shorter for this job"}`,
			"Analyst who builds reporting", "Shorter for this job", true},
		{"prose around it", "Sure! Here is the change:\n{\"proposed\": \"Analyst who builds reporting\", \"reason\": \"Shorter\"}\nHope it helps.",
			"Analyst who builds reporting", "Shorter", true},
		{"in a code fence", "```json\n{\"proposed\": \"Analyst who builds reporting\"}\n```",
			"Analyst who builds reporting", "", true},
		{"no json", "I am not sure what to write here.", "", "", false},
		{"empty proposal", `{"proposed": "   "}`, "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			proposed, reason, ok := ParseReplyWithReason(c.reply)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if proposed != c.proposed {
				t.Errorf("proposed = %q, want %q", proposed, c.proposed)
			}
			if reason != c.reason {
				t.Errorf("reason = %q, want %q", reason, c.reason)
			}
		})
	}
}
