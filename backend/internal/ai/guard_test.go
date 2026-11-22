package ai

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/analyze"
	"github.com/tlmcguire/fistbump/backend/internal/models"
)

const testResume = `# Ana Lima

## Summary

Data analyst who builds reporting in SQL and Excel.

## Experience

### Data Analyst, Acme

- Built weekly sales dashboards in Tableau for the regional team
- Cleaned and joined order data with SQL
- Automated a monthly report with Python

## Skills

excel, sql, python, tableau`

func guardRequest() Request {
	a := analyze.Result{}
	a.Required.Tech.Matched = []analyze.Match{{Skill: "sql"}, {Skill: "python"}}
	a.Required.Tech.Missing = []string{"kubernetes", "aws"}
	return Request{
		Job:      models.Job{PositionTitle: "Business Analyst", CompanyName: "Globex"},
		Analysis: a,
		Profile:  &models.Profile{Skills: []string{"excel", "sql", "python", "tableau"}},
		Resume:   testResume,
	}
}

var experienceTarget = Target{
	Section: "experience",
	Label:   "Data Analyst at Acme (responsibilities)",
	Original: `- Built weekly sales dashboards in Tableau for the regional team
- Cleaned and joined order data with SQL
- Automated a monthly report with Python`,
}

func TestGuardDropsAMissingSkill(t *testing.T) {
	proposed := `- Cleaned and joined order data with SQL on AWS
- Built weekly sales dashboards in Tableau for the regional team
- Automated a monthly report with Python`
	if reason := Check(experienceTarget, proposed, guardRequest()); !strings.Contains(reason, "skill") {
		t.Errorf("Check = %q, want a skill drop", reason)
	}
}

func TestGuardDropsSkillsLineWithAnExtraItem(t *testing.T) {
	skills := Target{Section: "skills", Label: "Skills", Original: "excel, sql, python, tableau"}
	if reason := Check(skills, "sql, python, excel, tableau, kubernetes", guardRequest()); reason == "" {
		t.Error("an added item must be dropped")
	}
	if reason := Check(skills, "sql, python", guardRequest()); reason == "" {
		t.Error("a removed item must be dropped")
	}
}

func TestGuardPassesAReorder(t *testing.T) {
	skills := Target{Section: "skills", Label: "Skills", Original: "excel, sql, python, tableau"}
	if reason := Check(skills, "SQL, python, excel, tableau", guardRequest()); reason != "" {
		t.Errorf("a reorder was dropped: %q", reason)
	}
	reordered := `- Cleaned and joined order data with SQL
- Automated a monthly report with Python
- Built weekly sales dashboards in Tableau for the regional team`
	if reason := Check(experienceTarget, reordered, guardRequest()); reason != "" {
		t.Errorf("reordered lines were dropped: %q", reason)
	}
}

func TestGuardDropsPoorlyGroundedRewording(t *testing.T) {
	// No dictionary skill in it, so only the grounding check can catch it.
	proposed := `- Drove enterprise transformation programs delivering synergistic value
- Revamped quarterly planning frameworks across regions
- Championed bold paradigms`
	if reason := Check(experienceTarget, proposed, guardRequest()); !strings.Contains(reason, "words") {
		t.Errorf("Check = %q, want a grounding drop", reason)
	}
}

func TestGuardPassesAGroundedRewrite(t *testing.T) {
	proposed := `- Joined and cleaned order data with SQL
- Automated the monthly report with Python
- Built sales dashboards in Tableau for the regional team`
	if reason := Check(experienceTarget, proposed, guardRequest()); reason != "" {
		t.Errorf("a grounded rewrite was dropped: %q", reason)
	}
}

func TestGuardFormatRules(t *testing.T) {
	r := guardRequest()
	if reason := Check(experienceTarget, experienceTarget.Original, r); reason != "no change" {
		t.Errorf("no-op: %q", reason)
	}
	four := experienceTarget.Original + "\n- Cleaned order data"
	if reason := Check(experienceTarget, four, r); !strings.Contains(reason, "lines") {
		t.Errorf("extra line: %q", reason)
	}
	summary := Target{Section: "summary", Label: "Summary", Original: "Data analyst who builds reporting in SQL and Excel."}
	long := "Data analyst. Builds reporting. Uses SQL. Uses Excel."
	if reason := Check(summary, long, r); !strings.Contains(reason, "3 sentences") {
		t.Errorf("long summary: %q", reason)
	}
	if reason := Check(summary, "Data analyst who builds reporting in SQL, Excel and Python.", r); reason != "" {
		t.Errorf("a grounded summary was dropped: %q", reason)
	}
}

func TestPromptsNeverSendMissingSkills(t *testing.T) {
	r := guardRequest()
	for _, target := range []Target{experienceTarget, {Section: "skills", Label: "Skills", Original: "excel, sql"}, {Section: "summary", Label: "Summary", Original: "Analyst."}} {
		for _, m := range BuildPrompt(target, r) {
			low := strings.ToLower(m.Content)
			for _, missing := range r.Analysis.Required.Tech.Missing {
				if strings.Contains(low, missing) {
					t.Errorf("%s prompt mentions missing skill %q", target.Section, missing)
				}
			}
		}
	}
	user := BuildPrompt(experienceTarget, r)[1].Content
	for _, want := range []string{"Business Analyst", "Globex", "sql, python", "3 lines or fewer", "Built weekly sales dashboards"} {
		if !strings.Contains(user, want) {
			t.Errorf("user prompt lacks %q:\n%s", want, user)
		}
	}
}

func TestPromptUnescapesAndOutputIsEscaped(t *testing.T) {
	target := Target{Section: "experience", Label: "x", Original: `\- Built dashboards` + "\n" + `\- Wrote SQL`}
	if user := BuildPrompt(target, guardRequest())[1].Content; strings.Contains(user, `\-`) {
		t.Errorf("the model should see plain text: %s", user)
	}
	if got := toResumeText(target, "- Wrote SQL\n- Built dashboards"); got != `\- Wrote SQL`+"\n"+`\- Built dashboards` {
		t.Errorf("toResumeText = %q", got)
	}
}

func TestParseReply(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{`{"proposed": "New text"}`, "New text", true},
		{"Sure! Here it is:\n```json\n{\"proposed\": \"Line {one}\\nLine two\"}\n```\nHope that helps.", "Line {one}\nLine two", true},
		{`{"proposed": ""}`, "", false},
		{`{"text": "x"}`, "", false},
		{"no json here", "", false},
		{`{"proposed": "unterminated`, "", false},
	}
	for _, c := range cases {
		got, ok := ParseReply(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseReply(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// chatScript answers each call with the next reply or error.
func chatScript(replies ...any) chatFunc {
	i := 0
	return func(context.Context, []Message, int, bool) (string, error) {
		r := replies[i%len(replies)]
		i++
		if err, ok := r.(error); ok {
			return "", err
		}
		return r.(string), nil
	}
}

func TestSuggestWithKeepsGoodDropsBad(t *testing.T) {
	r := guardRequest()
	r.Targets = []Target{
		experienceTarget,
		{Section: "skills", Label: "Skills", Original: "excel, sql, python, tableau"},
		{Section: "summary", Label: "Summary", Original: ""}, // empty: never sent
	}
	chat := chatScript(
		`{"proposed": "- Cleaned and joined order data with SQL\n- Automated a monthly report with Python\n- Built weekly sales dashboards in Tableau for the regional team"}`,
		`{"proposed": "sql, python, excel, tableau, aws"}`, // adds a skill: dropped
	)
	drafts, err := suggestWith(context.Background(), EngineRemote, chat, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 1 || drafts[0].Section != "experience" || drafts[0].Engine != EngineRemote {
		t.Fatalf("drafts = %+v", drafts)
	}
	if !strings.HasPrefix(drafts[0].Proposed, `\- Cleaned`) {
		t.Errorf("proposal should be escaped like the original: %q", drafts[0].Proposed)
	}
}

func TestSuggestWithFailsWhenEveryCallFails(t *testing.T) {
	r := guardRequest()
	r.Targets = []Target{experienceTarget}
	boom := errors.New("connection refused")
	if _, err := suggestWith(context.Background(), EngineRemote, chatScript(boom), r); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the call's error so auto mode falls through", err)
	}
}

func TestModelThatSkipsGetsRulesGapFillWithLabels(t *testing.T) {
	r := guardRequest()
	r.Targets = []Target{experienceTarget, {Section: "skills", Label: "Skills", Original: "excel, tableau, sql, python"}}
	remote := NewRemote()
	s := newOpenAIServer(t, func(w http.ResponseWriter, body chatRequest, call int) {
		if strings.Contains(body.Messages[1].Content, "skills line") {
			reply(w, "I cannot help with that.") // skipped: not JSON
			return
		}
		reply(w, `{"proposed": "- Cleaned and joined order data with SQL\n- Automated a monthly report with Python\n- Built weekly sales dashboards in Tableau for the regional team"}`)
	})
	remote.Configure(s.URL+"/v1", "m", "")
	drafts, engine, err := Engines{Remote: remote, Rules: Rules{}}.Run(context.Background(), "auto", r)
	if err != nil {
		t.Fatal(err)
	}
	if engine != EngineRemote || len(drafts) != 2 {
		t.Fatalf("engine %q, drafts %+v", engine, drafts)
	}
	labels := map[string]string{}
	for _, d := range drafts {
		labels[d.Section] = d.Engine
	}
	if labels["experience"] != EngineRemote || labels["skills"] != EngineRules {
		t.Errorf("labels = %v", labels)
	}
}

func TestParseReplyWithReason(t *testing.T) {
	p, why, ok := ParseReplyWithReason("Sure! ```json\n{\"proposed\": \"Go, SQL\", \"reason\": \"  Puts   SQL first because the job asks for it.  \"}\n```")
	if !ok || p != "Go, SQL" || why != "Puts SQL first because the job asks for it." {
		t.Fatalf("got %q, %q, %v", p, why, ok)
	}
	if _, why, ok := ParseReplyWithReason(`{"proposed": "x"}`); !ok || why != "" {
		t.Fatalf("a reply with no reason still works: %q, %v", why, ok)
	}
	long := strings.Repeat("word ", 200)
	if _, why, _ := ParseReplyWithReason(`{"proposed": "x", "reason": "` + long + `"}`); len(why) > maxReasonLen+4 {
		t.Fatalf("reason not shortened: %d", len(why))
	}
}

func TestRulesExplainTheirChangesInTheUsersLanguage(t *testing.T) {
	if NormalizeLanguage("fr-CA") != "fr" || NormalizeLanguage("xx") != "en" || NormalizeLanguage("") != "en" {
		t.Fatal("NormalizeLanguage")
	}
	for _, lang := range []string{"en", "es", "fr", "de"} {
		for _, key := range []string{"skills", "lines", "summary", "newsummary"} {
			if reasonFor(lang, key, "go") == "" {
				t.Errorf("%s/%s has no text", lang, key)
			}
		}
	}
	if reasonFor("es", "skills") == reasonFor("en", "skills") {
		t.Error("Spanish reason is the English one")
	}
}

func TestAnEnglishReasonIsNotShownInAnotherLanguage(t *testing.T) {
	english := "This change directly addresses the skill requirement and improves the resume for this role."
	if got := localizeReason("es", "experience", english); got != reasonFor("es", "rewrite") {
		t.Errorf("got %q", got)
	}
	if got := localizeReason("es", "skills", english); got != reasonFor("es", "skills") {
		t.Errorf("got %q", got)
	}
	if got := localizeReason("en", "experience", english); got != english {
		t.Errorf("English is kept: %q", got)
	}
	spanish := "Pone SQL primero porque el puesto lo pide."
	if got := localizeReason("es", "skills", spanish); got != spanish {
		t.Errorf("Spanish is kept: %q", got)
	}
	if got := localizeReason("fr", "summary", ""); got != "" {
		t.Errorf("no reason stays none: %q", got)
	}
}
