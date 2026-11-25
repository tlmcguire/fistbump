package ai

import (
	"fmt"
	"strings"
)

// Languages the app's text and the engines' explanations are written in.
var languageNames = map[string]string{
	"en": "English",
	"es": "Spanish",
	"fr": "French",
	"de": "German",
}

// NormalizeLanguage gives a supported two-letter code, "en" for anything else.
func NormalizeLanguage(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if len(code) > 2 {
		code = code[:2]
	}
	if _, ok := languageNames[code]; ok {
		return code
	}
	return "en"
}

// reasons are the explanations the rules engine gives, one set per language.
var reasons = map[string]map[string]string{
	"en": {
		"skills":       "Skills the job asks for are moved to the front so they are seen first. Nothing was added or removed.",
		"skillsNamed":  "Moved %s to the front of your skills list because this job asks for them. Nothing was added or removed.",
		"lines":        "Lines that show skills this job asks for are moved up. The wording is unchanged.",
		"linesNamed":   "Moved lines highlighting %s to the top so matching experience is seen first. Wording is unchanged.",
		"summary":      "Highlights %s from your resume so your summary speaks directly to what this posting asks for.",
		"newsummary":   "Your summary was empty. Built a concise summary using skills already on your resume (%s).",
		"rewrite":      "Tightened wording to highlight experience relevant to this role. Verify against your record before accepting.",
		"rewriteNamed": "Refined wording to emphasize %s from your experience for this role.",
	},
	"es": {
		"skills":       "Las habilidades que pide el puesto pasan al principio para verse primero. No se añadió ni se quitó nada.",
		"skillsNamed":  "Se movieron %s al principio de tu lista de habilidades porque el puesto las pide. No se añadió ni se quitó nada.",
		"lines":        "Las líneas que muestran habilidades que pide el puesto suben de lugar. La redacción no cambia.",
		"linesNamed":   "Se subieron las líneas que destacan %s para que esa experiencia se vea primero. La redacción no cambia.",
		"summary":      "Destaca %s de tu currículum para que el resumen responda a lo que pide la oferta.",
		"newsummary":   "Tu resumen estaba vacío. Se creó uno breve usando habilidades ya presentes en tu currículum (%s).",
		"rewrite":      "Se ajustó la redacción para destacar mejor tu experiencia para este puesto. Revísala antes de aceptarla.",
		"rewriteNamed": "Se ajustó la redacción para destacar %s de tu experiencia para este puesto.",
	},
	"fr": {
		"skills":       "Les compétences demandées par le poste passent en tête pour être vues en premier. Rien n'a été ajouté ni retiré.",
		"skillsNamed":  "Déplacement de %s en tête de vos compétences car le poste les demande. Rien n'a été ajouté ni retiré.",
		"lines":        "Les lignes qui montrent des compétences demandées par le poste remontent. La formulation ne change pas.",
		"linesNamed":   "Les lignes mettant en avant %s remontent en premier pour être vues tout de suite. La formulation ne change pas.",
		"summary":      "Met en avant %s de votre CV pour que le résumé réponde directement à l'annonce.",
		"newsummary":   "Votre résumé était vide. Un résumé concis a été créé à partir des compétences déjà sur votre CV (%s).",
		"rewrite":      "La formulation a été affinée pour mieux mettre en valeur votre expérience pour ce poste. Vérifiez-la avant d'accepter.",
		"rewriteNamed": "La formulation a été affinée pour mettre en avant %s dans votre expérience pour ce poste.",
	},
	"de": {
		"skills":       "Die Fähigkeiten, die die Stelle verlangt, stehen jetzt vorn, damit sie zuerst auffallen. Es wurde nichts hinzugefügt oder entfernt.",
		"skillsNamed":  "%s wurden an den Anfang Ihrer Fähigkeiten gestellt, da die Stelle sie verlangt. Es wurde nichts hinzugefügt oder entfernt.",
		"lines":        "Zeilen, die Fähigkeiten für diese Stelle zeigen, rücken nach oben. Der Wortlaut bleibt gleich.",
		"linesNamed":   "Zeilen mit %s wurden nach oben gerückt, damit passende Erfahrung zuerst auffällt. Der Wortlaut bleibt gleich.",
		"summary":      "Hebt %s aus Ihrem Lebenslauf hervor, damit die Zusammenfassung direkt zur Anzeige passt.",
		"newsummary":   "Ihre Zusammenfassung war leer. Erstellt eine kurze Zusammenfassung aus Fähigkeiten in Ihrem Lebenslauf (%s).",
		"rewrite":      "Der Wortlaut wurde präzisiert, um passende Erfahrung für diese Stelle hervorzuheben. Prüfen Sie ihn vor dem Übernehmen.",
		"rewriteNamed": "Der Wortlaut wurde präzisiert, um %s aus Ihrer Erfahrung für diese Stelle hervorzuheben.",
	},
}

// reasonFor writes the rules engine's explanation for a kind of change.
func reasonFor(lang, key string, args ...any) string {
	set, ok := reasons[NormalizeLanguage(lang)]
	if !ok {
		set = reasons["en"]
	}
	text := set[key]
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}

// languageRule is the prompt line that asks for the reason in the user's language.
func languageRule(lang string) string {
	return languageNames[NormalizeLanguage(lang)]
}

// maxReasonLen keeps an explanation short even when a model rambles.
const maxReasonLen = 280

func cleanReason(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxReasonLen {
		s = strings.TrimSpace(s[:maxReasonLen]) + "…"
	}
	return s
}

// looksEnglish guesses whether a sentence is English, by its common small words. A small model asked for a
// reason in another language often answers in English anyway; that reason is replaced rather than shown
// in the wrong language.
func looksEnglish(s string) bool {
	hits := 0
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !(r >= 'a' && r <= 'z') }) {
		switch w {
		case "the", "and", "this", "that", "with", "for", "your", "which", "is", "are", "to", "of":
			hits++
		}
	}
	return hits >= 2
}

// localizeReason keeps a model's reason when it is in the asked-for language, and otherwise gives the
// plain reason for a rewrite in that language.
func localizeReason(lang, section, why string) string {
	lang = NormalizeLanguage(lang)
	if why == "" || lang == "en" || !looksEnglish(why) {
		return why
	}
	if section == "skills" {
		return reasonFor(lang, "skills")
	}
	return reasonFor(lang, "rewrite")
}

// verifyReason ensures a model's cited reason is accurate, specific, and in the requested language.
// If the model returned an empty, overly generic, or contradictory reason (for example claiming it
// added a skill to a skills-only reorder), it falls back to a concrete explanation citing the matched
// skills actually present in the proposed text.
func verifyReason(lang, section, original, proposed, why string, matched []string) string {
	why = localizeReason(lang, section, cleanReason(why))
	low := strings.ToLower(why)

	// Detect contradictory or vague model reasons.
	bad := why == "" || len(why) < 12 ||
		(section == "skills" && (strings.Contains(low, "added ") || strings.Contains(low, "removed "))) ||
		(section != "skills" && strings.Contains(low, "reordered skills")) ||
		low == "improved readability." || low == "improved readability" ||
		low == "updated for the job." || low == "updated for the job"

	if !bad {
		return why
	}

	// Build a grounded, specific fallback reason citing matched skills present in the proposal.
	cited := citedSkills(proposed, matched, 4)
	if section == "skills" {
		if len(cited) > 0 {
			return reasonFor(lang, "skillsNamed", strings.Join(cited, ", "))
		}
		return reasonFor(lang, "skills")
	}
	if len(cited) > 0 {
		return reasonFor(lang, "rewriteNamed", strings.Join(cited, ", "))
	}
	return reasonFor(lang, "rewrite")
}

// citedSkills returns up to limit matched skills that appear in text.
func citedSkills(text string, matched []string, limit int) []string {
	low := strings.ToLower(plainText(text))
	var out []string
	for _, m := range matched {
		if strings.Contains(low, strings.ToLower(m)) {
			out = append(out, m)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}
