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
		"skills":     "Skills the job asks for are moved to the front so they are seen first. Nothing was added or removed.",
		"lines":      "Lines that show skills this job asks for are moved up. The wording is unchanged.",
		"summary":    "Adds the skills you already have that this job asks for (%s), so the summary speaks to the posting.",
		"newsummary": "Your summary was empty. This one is built only from skills already on your resume (%s).",
		"rewrite":    "The wording was adjusted to read better for this job. Check it against your own record before you accept it.",
	},
	"es": {
		"skills":     "Las habilidades que pide el puesto pasan al principio para verse primero. No se añadió ni se quitó nada.",
		"lines":      "Las líneas que muestran habilidades que pide el puesto suben de lugar. La redacción no cambia.",
		"summary":    "Añade las habilidades que ya tienes y que pide el puesto (%s), para que el resumen responda a la oferta.",
		"newsummary": "Tu resumen estaba vacío. Este se construye solo con habilidades que ya están en tu currículum (%s).",
		"rewrite":    "Se ajustó la redacción para que encaje mejor con este empleo. Compárala con tu historial antes de aceptarla.",
	},
	"fr": {
		"skills":     "Les compétences demandées par le poste passent en tête pour être vues en premier. Rien n'a été ajouté ni retiré.",
		"lines":      "Les lignes qui montrent des compétences demandées par le poste remontent. La formulation ne change pas.",
		"summary":    "Ajoute les compétences que vous avez déjà et que le poste demande (%s), pour que le résumé réponde à l'annonce.",
		"newsummary": "Votre résumé était vide. Celui-ci repose uniquement sur des compétences déjà présentes dans votre CV (%s).",
		"rewrite":    "La formulation a été ajustée pour mieux correspondre à cet emploi. Comparez-la à votre parcours avant de l'accepter.",
	},
	"de": {
		"skills":     "Die Fähigkeiten, die die Stelle verlangt, stehen jetzt vorn, damit sie zuerst auffallen. Es wurde nichts hinzugefügt oder entfernt.",
		"lines":      "Zeilen, die Fähigkeiten für diese Stelle zeigen, rücken nach oben. Der Wortlaut bleibt gleich.",
		"summary":    "Ergänzt Fähigkeiten, die Sie schon haben und die die Stelle verlangt (%s), damit die Zusammenfassung zur Anzeige passt.",
		"newsummary": "Ihre Zusammenfassung war leer. Diese entsteht nur aus Fähigkeiten, die schon in Ihrem Lebenslauf stehen (%s).",
		"rewrite":    "Der Wortlaut wurde angepasst, damit er besser zu dieser Stelle passt. Prüfen Sie ihn an Ihrem Werdegang, bevor Sie zustimmen.",
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
