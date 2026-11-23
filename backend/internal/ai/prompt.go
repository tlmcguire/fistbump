package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/render"
)

// systemPrompt keeps a small model on one narrow task. The posting's raw text is never sent: only
// the parsed title, company and matched skills, which come from text already stripped of hidden
// HTML and invisible characters, so instructions hidden in a posting cannot reach the model.
const systemPrompt = `You edit one piece of a resume so it reads well for a specific job.
Rules:
- Keep every fact. Do not add skills, tools, numbers, job titles, employers, dates or achievements that are not in the text you are given.
- Do not mention the job posting or the company you are applying to.
- Write in the same voice and tense as the original.
- Plain text only, because the resume is printed from it: no Markdown (no **bold**, no # headings, no backticks, no tables), no emoji, no quotation marks around the text, no placeholders such as [Company].
- Keep the original's layout. Bullet lines start with "- " and stay bullets; a paragraph stays a paragraph. Do not add a heading, a name, a section title or a contact line.
- Keep the original's dates, job titles and punctuation style. Spell out nothing that is abbreviated and abbreviate nothing that is spelled out.
- Write short, concrete lines that start with a verb where the original does. No first person ("I", "my"), no filler such as "responsible for" or "passionate", and no buzzword stuffing.
- Also give a "reason": one short sentence (at most 25 words) telling the person why this change helps them for this job, for example why skills were reordered or why a line was reworded. Do not claim anything the original does not show.
- Reply with JSON only, in exactly this form: {"proposed": "the rewritten text", "reason": "why you changed it"}`

// sectionRule is the extra instruction for each kind of target.
func sectionRule(t Target) string {
	switch t.Section {
	case "skills":
		return "This is a comma-separated skills line. Only reorder the items so the ones that matter most for the job come first. Keep every item exactly as written, and add or remove none."
	case "summary":
		return "This is the resume summary. Rewrite it in at most 3 sentences."
	default:
		n := len(strings.Split(strings.TrimRight(plainText(t.Original), "\n"), "\n"))
		if n == 1 {
			return "This is a line from the experience section. Rewrite it as one line."
		}
		return fmt.Sprintf("These are %d lines from the experience section, one item per line. Rewrite them as exactly %d lines, one per original line, separated by \\n. Every line keeps its leading \"- \" bullet and every number and fact in it. Do not merge, split or drop lines.", n, n)
	}
}

// BuildPrompt writes the messages for one target. Missing skills are not sent, so the model is
// not tempted to add them.
func BuildPrompt(t Target, r Request) []Message {
	var b strings.Builder
	fmt.Fprintf(&b, "Job title: %s\n", oneLine(r.Job.PositionTitle))
	fmt.Fprintf(&b, "Company: %s\n", oneLine(r.Job.CompanyName))
	if skills := matchedSkills(r.Analysis); len(skills) > 0 {
		fmt.Fprintf(&b, "Skills the job wants that this resume shows: %s\n", strings.Join(skills, ", "))
	}
	fmt.Fprintf(&b, "Part of the resume: %s\n", oneLine(t.Label))
	b.WriteString(sectionRule(t))
	fmt.Fprintf(&b, "\nWrite the \"reason\" in %s. Keep \"proposed\" in the language of the original text.", languageRule(r.Language))
	b.WriteString("\n\nOriginal text:\n")
	b.WriteString(plainText(t.Original))
	system := systemPrompt
	if lang := languageRule(r.Language); lang != "English" {
		//a small model follows this far better in the system message, and with the key spelled out
		system += fmt.Sprintf("\n- The \"reason\" must be written in %s, not in English, even though the resume text is in another language.", lang)
	}
	return []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: b.String()},
	}
}

// ParseReply reads {"proposed": "..."} from a reply. Small models often wrap the JSON in prose or
// a code fence, so the first {...} in the text is tried when the whole reply does not parse.
func ParseReply(reply string) (string, bool) {
	p, _, ok := ParseReplyWithReason(reply)
	return p, ok
}

// ParseReplyWithReason is ParseReply that also gives the model's short explanation, empty when it sent none.
func ParseReplyWithReason(reply string) (string, string, bool) {
	if p, why, ok := decodeProposed(reply); ok {
		return p, why, true
	}
	start := strings.Index(reply, "{")
	if start < 0 {
		return "", "", false
	}
	// Find the end of the first complete object, respecting strings.
	depth, inString, escaped := 0, false, false
	for i := start; i < len(reply); i++ {
		c := reply[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return decodeProposed(reply[start : i+1])
			}
		}
	}
	return "", "", false
}

func decodeProposed(s string) (string, string, bool) {
	var v struct {
		Proposed *string `json:"proposed"`
		Reason   string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &v); err != nil || v.Proposed == nil {
		return "", "", false
	}
	p := strings.TrimSpace(*v.Proposed)
	return p, cleanReason(v.Reason), p != ""
}

// plainText removes the backslash render.Escape puts before a leading #, - or *, so the model
// sees the text as the user wrote it.
func plainText(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if len(trimmed) > 1 && trimmed[0] == '\\' && strings.ContainsRune("#-*", rune(trimmed[1])) {
			lines[i] = line[:len(line)-len(trimmed)] + trimmed[1:]
		}
	}
	return strings.Join(lines, "\n")
}

// toResumeText puts a proposal in the form the target's Original has, so render.Apply can place it. Small
// models drop or swap the "- " bullets, so each line gets the marker its original line had.
func toResumeText(t Target, proposed string) string {
	proposed = strings.TrimSpace(strings.ReplaceAll(proposed, "\r\n", "\n"))
	if t.Section == "skills" {
		return proposed // the skills line is not escaped
	}
	return render.Escape(matchBullets(plainText(t.Original), proposed))
}

var bulletPrefix = []string{"- ", "* ", "• ", "– ", "— "}

func hasBullet(line string) bool {
	line = strings.TrimLeft(line, " \t")
	for _, p := range bulletPrefix {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

func withoutBullet(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	for _, p := range bulletPrefix {
		if strings.HasPrefix(trimmed, p) {
			return strings.TrimLeft(strings.TrimPrefix(trimmed, p), " \t")
		}
	}
	return strings.TrimSpace(line)
}

// matchBullets makes line i of the proposal a "- " bullet when line i of the original is one, and plain
// text when it is not. It leaves the proposal alone when the line counts differ, since lines cannot be paired.
func matchBullets(original, proposed string) string {
	orig := strings.Split(strings.TrimRight(original, "\n"), "\n")
	lines := strings.Split(proposed, "\n")
	if len(orig) != len(lines) {
		return proposed
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if hasBullet(orig[i]) {
			lines[i] = "- " + withoutBullet(line)
		} else if hasBullet(line) {
			lines[i] = withoutBullet(line)
		}
	}
	return strings.Join(lines, "\n")
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
