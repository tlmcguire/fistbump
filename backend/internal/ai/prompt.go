package ai

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const systemPrompt = `You help tailor a resume to a job posting. Rules: never invent employers, dates, numbers, degrees or skills the resume does not already show. Keep each rewrite close to the original length. Use plain text. Reply with JSON only.`

// buildPrompt returns the user message. Only master resume text and job text are included.
func buildPrompt(r Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Job: %s at %s\n", r.Job.PositionTitle, r.Job.CompanyName)
	fmt.Fprintf(&b, "Required skills: %s\n", strings.Join(append(append([]string{}, r.Job.ReqTech...), r.Job.ReqSoft...), ", "))
	fmt.Fprintf(&b, "Preferred skills: %s\n", strings.Join(append(append([]string{}, r.Job.PrefTech...), r.Job.PrefSoft...), ", "))
	fmt.Fprintf(&b, "Skills the resume already shows that match: %s\n", strings.Join(r.Analysis.Matched(), ", "))
	b.WriteString("\nTask: rewrite each item so it speaks to this job. Keep every fact, number, employer and date exactly as written. ")
	b.WriteString("Do not add skills, tools or claims that are not in the item. Keep about the same length and the same format (keep \"- \" bullets). ")
	b.WriteString("A summary item with empty text needs new text: 2-3 sentences built only from facts in the other items. A skills item may only be reordered so the most relevant come first. ")
	b.WriteString("proposed_text holds only the rewritten text, never the item's kind or context.\n\nItems (JSON):\n")
	type item struct {
		Index   int    `json:"index"`
		Kind    string `json:"kind"`
		Context string `json:"context"`
		Text    string `json:"text"`
	}
	items := make([]item, len(r.Targets))
	for i, t := range r.Targets {
		items[i] = item{i, t.Section, t.Label, t.Original}
	}
	data, _ := json.MarshalIndent(items, "", "  ")
	b.Write(data)
	fmt.Fprintf(&b, "\n\nReply with one entry per item, %d in total: {\"suggestions\":[{\"index\":0,\"proposed_text\":\"...\"}]}", len(r.Targets))
	return b.String()
}

var stop = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`the and for with that this from into over across using used use while their your have has were was been being
		also more most than then them they which when where what will would could should about such each other into through role team teams work
		experience experienced years year professional skilled strong proven including within based focused highly`) {
		stop[w] = true
	}
}

// groundedEnough reports whether the content words of a proposal mostly appear in the source text
// (the resume plus the job's skill names). A model that adds new claims fails this check.
func groundedEnough(proposal, source string) bool {
	total, hit := 0, 0
	for _, w := range strings.Fields(strings.ToLower(proposal)) {
		w = strings.Trim(w, ".,;:()!?\"'…-–—")
		if len(w) < 4 || stop[w] {
			continue
		}
		total++
		stem := w
		if len(stem) > 6 {
			stem = stem[:len(stem)-2] // "optimized" ~ "optimize", "engineers" ~ "engineer"
		}
		if strings.Contains(source, stem) {
			hit++
		}
	}
	return total == 0 || float64(hit)/float64(total) >= 0.75
}

// parseDrafts reads the model reply, tolerating code fences and surrounding prose.
func parseDrafts(reply string, targets []Target) ([]Draft, error) {
	return parseDraftsGrounded(reply, targets, "", nil)
}

// parseDraftsGrounded is parseDrafts that also drops proposals not grounded in source (when non-empty)
// and proposals that claim skills outside have (when non-nil).
func parseDraftsGrounded(reply string, targets []Target, source string, have SkillSet) ([]Draft, error) {
	s := strings.TrimSpace(reply)
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		s = s[i : j+1]
	}
	var parsed struct {
		Suggestions []struct {
			Index    int    `json:"index"`
			Proposed string `json:"proposed_text"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return nil, fmt.Errorf("model reply was not valid JSON")
	}
	var out []Draft
	seen := map[int]bool{}
	for _, p := range parsed.Suggestions {
		p.Proposed = strings.TrimSpace(p.Proposed)
		if p.Index < 0 || p.Index >= len(targets) || seen[p.Index] || p.Proposed == "" {
			continue
		}
		t := targets[p.Index]
		p.Proposed = strings.TrimSpace(strings.TrimLeft(stripEcho(p.Proposed, t), ".…"))
		if source != "" && !groundedEnough(p.Proposed, source) {
			continue
		}
		if have != nil {
			if reason := checkDraft(t, p.Proposed, have); reason != "" {
				dropped(t, reason)
				continue
			}
		}
		if p.Proposed == strings.TrimSpace(t.Original) {
			continue
		}
		seen[p.Index] = true
		out = append(out, Draft{Section: t.Section, TargetID: t.TargetID, Original: t.Original, Proposed: p.Proposed})
	}
	return out, nil
}

// stripEcho removes an item label a small model copied in front of its answer, as a prefix
// ("Professional summary: ...", "Professional summary - ...") or as a whole first line.
func stripEcho(text string, t Target) string {
	text = strings.TrimSpace(text)
	for _, label := range []string{t.Label, t.Section} {
		if label == "" || len(text) <= len(label) || !strings.EqualFold(text[:len(label)], label) {
			continue
		}
		rest := text[len(label):]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 && strings.TrimSpace(rest[:nl]) == "" || nl >= 0 && strings.HasPrefix(strings.TrimSpace(rest[:nl]), "(") {
			return strings.TrimSpace(rest[nl+1:])
		}
		if r := strings.TrimLeft(rest, " "); len(r) > 0 && strings.ContainsRune(":-–—", []rune(r)[0]) {
			return strings.TrimSpace(strings.TrimLeft(r, ":-–— "))
		}
	}
	return text
}

// dropped logs a rejected proposal to stderr (never to the UI or database).
var dropped = func(t Target, reason string) {
	fmt.Fprintf(os.Stderr, "dropped %s suggestion: %s\n", t.Section, reason)
}

// groundingSource is the text model proposals must stay within: every target plus the skills the user
// already matched. Skills the job wants but the resume lacks are deliberately left out.
func groundingSource(r Request) string {
	var b strings.Builder
	for _, t := range r.Targets {
		b.WriteString(t.Label + "\n" + t.Original + "\n")
	}
	b.WriteString(strings.Join(r.Analysis.Matched(), " ") + "\n")
	b.WriteString(r.Job.PositionTitle + " " + r.Job.CompanyName)
	return strings.ToLower(b.String())
}
