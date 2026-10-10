package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/parser"
	"github.com/tlmcguire/fistbump/backend/internal/resumeparse"
)

// Completer is implemented by engines that can answer a free-form prompt.
// jsonMode asks the engine for a JSON object reply.
type Completer interface {
	Complete(ctx context.Context, system, user string, maxTokens int, jsonMode bool) (string, error)
}

func (r *Remote) Complete(ctx context.Context, system, user string, maxTokens int, jsonMode bool) (string, error) {
	if !r.Ready() {
		return "", ErrUnavailable
	}
	msgs := []chatMessage{{"system", system}, {"user", user}}
	if jsonMode {
		return r.ChatJSON(ctx, msgs, maxTokens)
	}
	return r.Chat(ctx, msgs, maxTokens)
}

func (l *Local) Complete(ctx context.Context, system, user string, maxTokens int, jsonMode bool) (string, error) {
	if err := l.ensure(ctx); err != nil {
		return "", err
	}
	msgs := []chatMessage{{"system", system}, {"user", user}}
	if jsonMode {
		return l.client.ChatJSON(ctx, msgs, maxTokens)
	}
	return l.client.Chat(ctx, msgs, maxTokens)
}

// Complete runs the prompt on the first model engine the mode allows. It never uses the rules engine,
// and returns ErrUnavailable when no model can run.
func (e *Engines) Complete(ctx context.Context, mode, system, user string, maxTokens int, jsonMode bool) (string, string, error) {
	var last error = ErrUnavailable
	for _, name := range e.Order(mode) {
		var c Completer
		switch name {
		case "local":
			c = e.Local
		case "remote":
			c = e.Remote
		default:
			continue
		}
		out, err := c.Complete(ctx, system, user, maxTokens, jsonMode)
		if err == nil {
			return out, name, nil
		}
		if ctx.Err() != nil {
			return "", name, ctx.Err()
		}
		last = fmt.Errorf("%s: %w", name, err)
	}
	return "", "", last
}

const structureSystem = `You convert resume text into JSON. Copy facts exactly as written. Never invent employers, titles, dates, numbers, schools or skills. Use null for anything missing. Reply with JSON only.`

const structureSchema = `{"full_name":"","email":null,"phone":null,"location":null,"summary":null,
"experiences":[{"job_title":"","company_name":"","location":null,"start_date":"YYYY-MM or YYYY","end_date":"YYYY-MM, YYYY, or null if current","bullets":[""]}],
"education":[{"institution":"","degree_level":null,"discipline":null,"graduation_date":null,"gpa":null,"honors":null}],
"skills":[""],"certifications":[""]}`

type aiResume struct {
	FullName    string  `json:"full_name"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	Location    *string `json:"location"`
	Summary     *string `json:"summary"`
	Experiences []struct {
		JobTitle    string   `json:"job_title"`
		CompanyName string   `json:"company_name"`
		Location    *string  `json:"location"`
		StartDate   *string  `json:"start_date"`
		EndDate     *string  `json:"end_date"`
		Bullets     []string `json:"bullets"`
	} `json:"experiences"`
	Education []struct {
		Institution    string  `json:"institution"`
		DegreeLevel    *string `json:"degree_level"`
		Discipline     *string `json:"discipline"`
		GraduationDate *string `json:"graduation_date"`
		GPA            flexNum `json:"gpa"`
		Honors         *string `json:"honors"`
	} `json:"education"`
	Skills         []string `json:"skills"`
	Certifications []string `json:"certifications"`
}

var (
	quant    = regexp.MustCompile(`\d+(?:\.\d+)?\s*(?:%|x\b|k\b|m\b|\+)|\$\s?\d|\b\d{2,}\b`)
	dateOK   = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2})?)?$`)
	maxInput = 14000
)

func clean(p *string) *string {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(*p)
	if s == "" || strings.EqualFold(s, "null") {
		return nil
	}
	return &s
}

func cleanDate(p *string) *string {
	p = clean(p)
	if p == nil || !dateOK.MatchString(*p) {
		return nil
	}
	return p
}

// flexNum accepts a JSON number, a numeric string, or null. Small models often quote numbers.
type flexNum struct{ v *float64 }

func (f *flexNum) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	var n float64
	if _, err := fmt.Sscan(s, &n); err == nil {
		f.v = &n
	}
	return nil
}

// grounded keeps a value only when it appears in the source text, so the model cannot add facts.
func grounded(p *string, src string) *string {
	p = clean(p)
	if p == nil || !strings.Contains(src, strings.ToLower(*p)) {
		return nil
	}
	return p
}

// groundedDate keeps a date only when its year appears in the source text.
func groundedDate(p *string, src string) *string {
	p = cleanDate(p)
	if p == nil || !strings.Contains(src, (*p)[:4]) {
		return nil
	}
	return p
}

// StructureResume asks a model to structure resume text and returns a draft. Contact details the rules
// parser found are kept when the model omits them. Callers fall back to the rules draft on error.
func (e *Engines) StructureResume(ctx context.Context, mode, text string, rules resumeparse.Draft) (resumeparse.Draft, string, error) {
	if len(text) > maxInput {
		text = text[:maxInput]
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	reply, engine, err := e.Complete(ctx, mode, structureSystem, "Schema:\n"+structureSchema+"\n\nResume text:\n"+text, 3500, true)
	if err != nil {
		return rules, "", err
	}
	s := strings.TrimSpace(reply)
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		s = s[i : j+1]
	}
	var r aiResume
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return rules, engine, errors.New("the model reply was not valid JSON")
	}
	src := strings.ToLower(strings.Join(strings.Fields(text), " "))
	d := resumeparse.Draft{Experiences: []models.Experience{}, Education: []models.Education{}, Skills: []string{}, Warnings: []string{}}
	d.Profile = models.Profile{FullName: strings.TrimSpace(r.FullName), Email: grounded(r.Email, src), Phone: grounded(r.Phone, src), HomeLocation: grounded(r.Location, src),
		Summary: clean(r.Summary), TargetPositions: []string{}, ClearanceCerts: []string{}}
	if d.Profile.FullName == "" || !strings.Contains(src, strings.ToLower(d.Profile.FullName)) {
		d.Profile.FullName = rules.Profile.FullName
	}
	if d.Profile.HomeLocation == nil {
		d.Profile.HomeLocation = rules.Profile.HomeLocation
	}
	d.Profile.Links = rules.Profile.Links // read verbatim from the header, never from the model
	if d.Profile.Email == nil {
		d.Profile.Email = rules.Profile.Email
	}
	if d.Profile.Phone == nil {
		d.Profile.Phone = rules.Profile.Phone
	}
	for _, s := range append(r.Skills, rules.Skills...) {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" && len(s) <= 40 && !contains(d.Skills, s) && strings.Contains(src, s) {
			d.Skills = append(d.Skills, s)
		}
	}
	for _, c := range r.Certifications {
		if c = strings.TrimSpace(c); c != "" {
			d.Profile.ClearanceCerts = append(d.Profile.ClearanceCerts, c)
		}
	}
	for i, x := range r.Experiences {
		if strings.TrimSpace(x.JobTitle) == "" && strings.TrimSpace(x.CompanyName) == "" {
			continue
		}
		ex := models.Experience{JobTitle: strings.TrimSpace(x.JobTitle), CompanyName: strings.TrimSpace(x.CompanyName), Location: grounded(x.Location, src),
			StartDate: groundedDate(x.StartDate, src), EndDate: groundedDate(x.EndDate, src), SortOrder: i}
		if ex.JobTitle == "" {
			ex.JobTitle = "Unknown title"
		}
		if ex.CompanyName == "" {
			ex.CompanyName = "Unknown company"
		}
		var desc, impact []string
		for _, b := range x.Bullets {
			if b = strings.TrimSpace(strings.TrimLeft(b, "-•* ")); b == "" {
				continue
			}
			// A bullet whose words mostly do not appear in the resume was written by the model: drop it.
			if !mostlyGrounded(b, src) {
				continue
			}
			if quant.MatchString(b) {
				impact = append(impact, "- "+b)
			} else {
				desc = append(desc, "- "+b)
			}
		}
		ex.Description, ex.Impact = clean(ptr(strings.Join(desc, "\n"))), clean(ptr(strings.Join(impact, "\n")))
		all := strings.ToLower(ex.JobTitle + "\n" + strings.Join(x.Bullets, "\n")) // not the company name
		tech, _ := parser.FindSkills(all)
		ex.Skills = tech
		for _, s := range d.Skills {
			if parser.ContainsTerm(all, s) && !contains(ex.Skills, parser.Canon(s)) {
				ex.Skills = append(ex.Skills, s)
			}
		}
		if ex.Skills == nil {
			ex.Skills = []string{}
		}
		d.Experiences = append(d.Experiences, ex)
	}
	for i, x := range r.Education {
		if strings.TrimSpace(x.Institution) == "" {
			continue
		}
		ed := models.Education{Institution: strings.TrimSpace(x.Institution), DegreeLevel: grounded(x.DegreeLevel, src), Discipline: grounded(x.Discipline, src),
			GraduationDate: groundedDate(x.GraduationDate, src), Honors: grounded(x.Honors, src), SortOrder: i}
		if v := x.GPA.v; v != nil && *v >= 0 && *v <= 5 && strings.Contains(src, fmt.Sprint(*v)) {
			ed.GPA = v
		}
		d.Education = append(d.Education, ed)
	}
	// A model that dropped every role did worse than the rules parser.
	if len(d.Experiences) == 0 && len(rules.Experiences) > 0 {
		return rules, engine, errors.New("the model found no work experience")
	}
	d.Profile.Skills = []string{}
	for _, s := range d.Skills { // skills no role mentions are general skills on the profile
		found := false
		for _, ex := range d.Experiences {
			if contains(ex.Skills, s) || contains(ex.Skills, parser.Canon(s)) {
				found = true
			}
		}
		if !found {
			d.Profile.Skills = append(d.Profile.Skills, s)
		}
	}
	if len(d.Experiences) > 0 {
		d.Profile.TargetPositions = []string{d.Experiences[0].JobTitle}
	}
	if d.Profile.FullName == "" {
		d.Warnings = append(d.Warnings, "Could not find your name. Enter it before saving.")
	}
	return d, engine, nil
}

func ptr(s string) *string { return &s }

func mostlyGrounded(b, src string) bool {
	words := strings.Fields(strings.ToLower(b))
	hit := 0
	for _, w := range words {
		if strings.Contains(src, strings.Trim(w, ".,;:()")) {
			hit++
		}
	}
	return len(words) > 0 && float64(hit)/float64(len(words)) >= 0.8
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// RulesSummary writes a plain summary from the master resume: latest title, years of experience, top skills.
// Years count only entries whose title reads as a job, so imported clubs, competitions and projects
// (stored as experience entries) do not inflate the number.
func RulesSummary(m models.Master) string {
	if len(m.Experiences) == 0 {
		return ""
	}
	title := m.Experiences[0].JobTitle
	earliest := ""
	for _, e := range m.Experiences {
		if e.StartDate != nil && resumeparse.IsJobTitle(e.JobTitle) && (earliest == "" || *e.StartDate < earliest) {
			earliest = *e.StartDate
		}
	}
	var b strings.Builder
	b.WriteString(title)
	if len(earliest) >= 4 {
		var y int
		fmt.Sscan(earliest[:4], &y)
		if years := time.Now().Year() - y; years >= 2 {
			fmt.Fprintf(&b, " with %d+ years of experience", years)
		}
	}
	if sk := m.Skills(); len(sk) > 0 {
		if len(sk) > 6 {
			sk = sk[:6]
		}
		b.WriteString(" working with " + joinList(sk))
	}
	b.WriteString(".")
	return b.String()
}

const summarySystem = `You write resume summaries. Use only facts in the resume. 2 to 3 sentences, no first person, no buzzwords, no invented numbers. Reply with the summary text only.`

// Summary writes a professional summary with a model when one is available, otherwise with rules.
func (e *Engines) Summary(ctx context.Context, mode string, m models.Master, resumeText string) (string, string, error) {
	if mode != "rules" {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		if len(resumeText) > maxInput {
			resumeText = resumeText[:maxInput]
		}
		out, engine, err := e.Complete(ctx, mode, summarySystem, resumeText, 300, false)
		out = strings.Trim(strings.TrimSpace(out), `"`)
		// The summary may only name skills the resume shows; otherwise use the rules summary.
		have := ResumeSkills([]string{resumeText}, m.Skills())
		if m.Profile != nil {
			have = ResumeSkills([]string{resumeText}, m.Skills(), m.Profile.ClearanceCerts)
		}
		if err == nil && out != "" && len(have.UnsupportedSkills(out)) == 0 && groundedEnough(out, strings.ToLower(resumeText)) {
			return out, engine, nil
		}
		if err == nil {
			err = errors.New("the model's summary named skills or claims not on the resume")
		}
		if mode == "local" || mode == "remote" {
			return "", engine, err
		}
	}
	s := RulesSummary(m)
	if s == "" {
		return "", "rules", errors.New("add at least one experience entry first")
	}
	return s, "rules", nil
}
