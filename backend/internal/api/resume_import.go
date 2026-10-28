package api

// resume_import.go: POST /v1/resume/import and POST /v1/resume/import/apply.
//
// It expects these from resumeparse (steps 15 to 17):
//
//	type Hidden struct { Words int; Reasons []string; Sample string }  // json: words, reasons, sample
//	func (h Hidden) Merge(o Hidden) Hidden  // sums words, unions reasons, keeps the first 200 chars of sample
//	func (h Hidden) Empty() bool
//	func CleanText(text string) (string, Hidden)  // hidden.go: textclean.StripInvisible, reason "invisible characters"
//	type Extracted struct { Text string; Hidden Hidden }
//	func TextFromPDF(data []byte) (Extracted, error)  // extract.go: drops pieces isHidden reports
//	type Draft struct { Profile, Experiences, Education, Skills, Warnings }
//	func Parse(text string) Draft  // parse.go

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/tlmcguire/fistbump/backend/internal/models"
	"github.com/tlmcguire/fistbump/backend/internal/resumeparse"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

const (
	// maxImportBody fits a resumeparse.MaxFileBytes file in base64 (a third larger) plus the
	// filename and JSON around it.
	maxImportBody = (resumeparse.MaxFileBytes+2)/3*4 + 64<<10

	maxPastedChars = 100000
	maxApplyBody   = 4 << 20 // a draft plus a saved copy of up to 1 MB

	// Past these an import still works but the response says what it costs. Most resumes are
	// well under 1 MiB; template PDFs grow from embedded fonts and images, not from text.
	largeFileBytes = 1 << 20
	// About four dense pages. The text is what a saved copy stores and what a local model reads.
	longTextChars = 12000
)

const importNotice = "Review every field before applying. Nothing is saved yet."

type importBody struct {
	Filename   string  `json:"filename"`
	DataBase64 *string `json:"data_base64"`
	Text       *string `json:"text"`
	Engine     string  `json:"engine"`
}

type importResponse struct {
	Text         string              `json:"text"`
	Draft        resumeparse.Draft   `json:"draft"`
	HiddenText   *resumeparse.Hidden `json:"hidden_text"`
	SizeWarnings []string            `json:"size_warnings"`
	Engine       string              `json:"engine"`
	Notice       string              `json:"notice"`
}

// resumeImport reads a file or pasted text into a draft. Hidden text is left out of the draft
// and reported. Nothing is saved.
func (s *Server) resumeImport(w http.ResponseWriter, r *http.Request) error {
	var body importBody
	if err := decodeJSONLimit(w, r, &body, maxImportBody); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return parseError(&resumeparse.Error{Reason: resumeparse.ReasonTooLarge,
				Message: fmt.Sprintf("The file is larger than %s. Export it again without images, or paste the text instead.", sizeLabel(resumeparse.MaxFileBytes))})
		}
		return err
	}
	switch body.Engine {
	case "", "rules":
	case "auto":
		return validationError("engine", "Reading with AI is not available yet")
	default:
		return validationError("engine", "engine must be rules or auto")
	}
	hasFile, hasText := body.DataBase64 != nil, body.Text != nil
	switch {
	case hasFile && hasText:
		return validationError("text", "give data_base64 or text, not both")
	case !hasFile && !hasText:
		return validationError("data_base64", "data_base64 or text is required")
	}

	var raw string
	var hidden resumeparse.Hidden
	var warnings []string
	if hasText {
		if n := utf8.RuneCountInString(*body.Text); n > maxPastedChars {
			return validationError("text", fmt.Sprintf("Pasted text is limited to %d characters", maxPastedChars))
		}
		raw = *body.Text
	} else {
		data, err := base64.StdEncoding.DecodeString(*body.DataBase64)
		if err != nil {
			return parseError(&resumeparse.Error{Reason: resumeparse.ReasonInvalidEncoding,
				Message: "The file is empty or could not be read."})
		}
		kind, err := resumeparse.Detect(body.Filename, data)
		if err != nil {
			return parseError(err)
		}
		if len(data) > largeFileBytes {
			warnings = append(warnings, fmt.Sprintf("This file is %s. Only its text is read and kept, "+
				"but larger files take longer to import. Exporting without images or embedded fonts makes it smaller.", sizeLabel(len(data))))
		}
		switch kind {
		case resumeparse.KindPDF:
			ex, err := resumeparse.TextFromPDF(data)
			if err != nil {
				return parseError(err)
			}
			raw, hidden = ex.Text, ex.Hidden
		case resumeparse.KindText:
			raw = string(data)
		}
	}
	raw = strings.TrimPrefix(raw, "\uFEFF") // a leading byte order mark is not hidden text

	// Every path goes through CleanText, so invisible characters are dropped from PDF text too.
	text, invisible := resumeparse.CleanText(raw)
	hidden = hidden.Merge(invisible)
	if strings.TrimSpace(text) == "" {
		return parseError(&resumeparse.Error{Reason: resumeparse.ReasonNoText,
			Message: "No selectable text was found. Paste the text of your resume instead."})
	}
	if n := utf8.RuneCountInString(text); n > longTextChars {
		warnings = append(warnings, fmt.Sprintf("This resume has about %d pages of text. You can import it, "+
			"but a saved copy takes more storage and a local model will take longer to read it and may miss details. "+
			"A master resume of one to three pages works best.", (n+2999)/3000))
	}

	out := importResponse{
		Text:         text,
		Draft:        resumeparse.Parse(text),
		SizeWarnings: warnings,
		Engine:       "rules",
		Notice:       importNotice,
	}
	if out.SizeWarnings == nil {
		out.SizeWarnings = []string{}
	}
	if !hidden.Empty() {
		out.HiddenText = &hidden
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

type applyBody struct {
	Draft struct {
		Profile     profileBody      `json:"profile"`
		Experiences []experienceBody `json:"experiences"`
		Education   []educationBody  `json:"education"`
		Skills      []string         `json:"skills"`
		Warnings    []string         `json:"warnings"`
	} `json:"draft"`
	Replace  bool `json:"replace"`
	SaveCopy *struct {
		VersionLabel string `json:"version_label"`
		Content      string `json:"content"`
	} `json:"save_copy"`
}

// resumeImportApply writes a reviewed draft to the master resume in one transaction. Empty
// profile fields keep their current values; replace deletes experience and education first.
func (s *Server) resumeImportApply(w http.ResponseWriter, r *http.Request) error {
	var body applyBody
	if err := decodeJSONLimit(w, r, &body, maxApplyBody); err != nil {
		return err
	}
	d := body.Draft
	exps := make([]models.Experience, len(d.Experiences))
	for i, b := range d.Experiences {
		e, err := b.validate()
		if err != nil {
			return atIndex(err, "experience_index", i)
		}
		e.SortOrder = int64(i)
		exps[i] = e
	}
	edus := make([]models.Education, len(d.Education))
	for i, b := range d.Education {
		e, err := b.validate()
		if err != nil {
			return atIndex(err, "education_index", i)
		}
		e.SortOrder = int64(i)
		edus[i] = e
	}
	var copyRV *models.ResumeVersion
	if c := body.SaveCopy; c != nil {
		label, err := requiredText("save_copy.version_label", c.VersionLabel)
		if err != nil {
			return err
		}
		if strings.TrimSpace(c.Content) == "" || len(c.Content) > 1<<20 {
			return validationError("save_copy.content", "save_copy.content must be 1 byte to 1 MB")
		}
		copyRV = &models.ResumeVersion{VersionLabel: label, Content: c.Content}
	}

	ctx := r.Context()
	err := s.store.WithTx(ctx, func(tx *store.Store) error {
		cur, err := tx.GetMaster(ctx)
		if err != nil {
			return err
		}
		p, err := mergeProfile(cur.Profile, d.Profile, d.Skills)
		if err != nil {
			return err
		}
		if _, err := tx.UpsertProfile(ctx, p); err != nil {
			return err
		}
		if body.Replace {
			for _, e := range cur.Experiences {
				if err := tx.DeleteExperience(ctx, e.ID); err != nil {
					return err
				}
			}
			for _, e := range cur.Education {
				if err := tx.DeleteEducation(ctx, e.ID); err != nil {
					return err
				}
			}
		}
		// Appended entries go after the ones kept.
		offset := int64(0)
		if !body.Replace {
			offset = int64(len(cur.Experiences))
		}
		for _, e := range exps {
			e.SortOrder += offset
			if _, err := tx.CreateExperience(ctx, e); err != nil {
				return err
			}
		}
		if !body.Replace {
			offset = int64(len(cur.Education))
		}
		for _, e := range edus {
			e.SortOrder += offset
			if _, err := tx.CreateEducation(ctx, e); err != nil {
				return err
			}
		}
		if copyRV != nil {
			if _, err := tx.CreateResume(ctx, *copyRV); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			return apiErr
		}
		return storeError(err, "Resume")
	}
	return s.resumeGet(w, r)
}

// mergeProfile fills the draft's empty fields from the current profile, then validates. Draft
// skills are added to the profile's skills.
func mergeProfile(cur *models.Profile, draft profileBody, skills []string) (models.Profile, error) {
	if cur != nil {
		keep := func(v **string, have *string) {
			if (*v == nil || strings.TrimSpace(**v) == "") && have != nil {
				*v = have
			}
		}
		if strings.TrimSpace(draft.FullName) == "" {
			draft.FullName = cur.FullName
		}
		keep(&draft.Email, cur.Email)
		keep(&draft.Phone, cur.Phone)
		keep(&draft.HomeLocation, cur.HomeLocation)
		keep(&draft.PreferredMode, cur.PreferredMode)
		keep(&draft.Summary, cur.Summary)
		if draft.MinDesiredPay == nil {
			draft.MinDesiredPay = cur.MinDesiredPay
		}
		for _, f := range []struct {
			v    *[]string
			have []string
		}{
			{&draft.TargetPositions, cur.TargetPositions}, {&draft.ClearanceCerts, cur.ClearanceCerts},
			{&draft.Skills, cur.Skills}, {&draft.Links, cur.Links},
		} {
			if len(*f.v) == 0 {
				*f.v = f.have
			}
		}
	}
	draft.Skills = models.Master{Profile: &models.Profile{Skills: append(append([]string{}, draft.Skills...), skills...)}}.Skills()
	return draft.validate()
}

// atIndex adds the failing entry's position to a 422 so the review screen can mark it.
func atIndex(err error, key string, i int) error {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		if apiErr.Details == nil {
			apiErr.Details = map[string]any{}
		}
		apiErr.Details[key] = i
	}
	return err
}

// parseError turns a resumeparse refusal into a 422 with details.reason; anything else is a 500.
func parseError(err error) error {
	var pe *resumeparse.Error
	if !errors.As(err, &pe) {
		return err
	}
	e := validationFailed(pe.Message)
	e.Details = map[string]any{"reason": pe.Reason}
	if pe.Detected != "" {
		e.Details["detected"] = pe.Detected
	}
	return e
}

// sizeLabel writes a byte count the way the UI shows it ("640 KB", "2.3 MB").
func sizeLabel(n int) string {
	if n < 1<<20 {
		return fmt.Sprintf("%d KB", (n+1023)>>10)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}
