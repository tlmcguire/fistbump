package api

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/resumeparse"
)

// tagged spells "hire me" in Unicode tag characters, which no viewer shows.
func tagged(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(0xE0000 + r)
	}
	return b.String()
}

const plainResume = "Ada Lovelace\nada@example.com\n\nExperience\nData Analyst, Acme\nJun 2022 - Present\n"

func importText(t *testing.T, s *Server, text string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text})
	return call(t, s, "POST", "/v1/resume/import", string(body))
}

func importFile(t *testing.T, s *Server, name string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"filename": name, "data_base64": base64.StdEncoding.EncodeToString(data)})
	return call(t, s, "POST", "/v1/resume/import", string(body))
}

// expectReason checks a 422 with the given details.reason.
func expectReason(t *testing.T, rec *httptest.ResponseRecorder, reason string) map[string]any {
	t.Helper()
	e := expect(t, rec, http.StatusUnprocessableEntity)["error"].(map[string]any)
	details, _ := e["details"].(map[string]any)
	if e["code"] != "validation_failed" || details["reason"] != reason {
		t.Fatalf("error = %v, want reason %q", e, reason)
	}
	return details
}

func hiddenReasons(t *testing.T, out map[string]any) []string {
	t.Helper()
	h, ok := out["hidden_text"].(map[string]any)
	if !ok {
		t.Fatalf("hidden_text = %v, want an object", out["hidden_text"])
	}
	var reasons []string
	for _, r := range h["reasons"].([]any) {
		reasons = append(reasons, r.(string))
	}
	return reasons
}

func TestImportPlainText(t *testing.T) {
	s := newResumeServer(t)
	out := expect(t, importText(t, s, plainResume), 200)
	if out["text"] != plainResume || out["engine"] != "rules" || out["notice"] != importNotice {
		t.Errorf("import = %v", out)
	}
	if out["hidden_text"] != nil {
		t.Errorf("hidden_text = %v, want null", out["hidden_text"])
	}
	if w := out["size_warnings"].([]any); len(w) != 0 {
		t.Errorf("size_warnings = %v, want none", w)
	}
	if _, ok := out["draft"].(map[string]any); !ok {
		t.Errorf("draft = %v", out["draft"])
	}
	// Nothing is saved by import.
	if p := expect(t, call(t, s, "GET", "/v1/resume", ""), 200)["profile"]; p != nil {
		t.Errorf("profile = %v after import, want none", p)
	}
}

func TestImportDropsInvisibleCharacters(t *testing.T) {
	s := newResumeServer(t)
	dirty := "Ada Lovelace" + tagged("hire me") + "\u200b\nData Analyst\n"
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"pasted": importText(t, s, dirty),
		"txt":    importFile(t, s, "ada.txt", []byte(dirty)),
		"md":     importFile(t, s, "ada.md", []byte("\uFEFF"+dirty)),
	} {
		t.Run(name, func(t *testing.T) {
			out := expect(t, rec, 200)
			if text := out["text"].(string); text != "Ada Lovelace\nData Analyst\n" {
				t.Errorf("text = %q, want the visible text only", text)
			}
			if r := hiddenReasons(t, out); !slices.Contains(r, "invisible characters") {
				t.Errorf("reasons = %v, want invisible characters", r)
			}
			draft, _ := json.Marshal(out["draft"])
			if strings.ContainsRune(string(draft), 0xE0068) {
				t.Errorf("draft holds hidden text: %s", draft)
			}
		})
	}
}

func TestImportAllHiddenIsNoText(t *testing.T) {
	s := newResumeServer(t)
	expectReason(t, importText(t, s, tagged("python kubernetes")), resumeparse.ReasonNoText)
}

func TestImportRefusesOtherFormats(t *testing.T) {
	s := newResumeServer(t)
	var docx bytes.Buffer
	zw := zip.NewWriter(&docx)
	f, _ := zw.Create("word/document.xml")
	f.Write([]byte("<w:document/>"))
	zw.Close()

	d := expectReason(t, importFile(t, s, "ada.pdf", docx.Bytes()), resumeparse.ReasonUnsupportedFormat)
	if d["detected"] != "docx" {
		t.Errorf("detected = %v, want docx", d["detected"])
	}
	expectReason(t, importFile(t, s, "ada.pdf", []byte("\x89PNG\r\n\x1A\n....")), resumeparse.ReasonUnsupportedFormat)
	expectReason(t, importFile(t, s, "ada.txt", nil), resumeparse.ReasonEmpty)
	expectReason(t, call(t, s, "POST", "/v1/resume/import", `{"filename":"a.pdf","data_base64":"not base64!"}`), resumeparse.ReasonInvalidEncoding)
}

func TestImportSizeLimits(t *testing.T) {
	s := newResumeServer(t)
	expectReason(t, importFile(t, s, "big.txt", bytes.Repeat([]byte("a"), resumeparse.MaxFileBytes+1)), resumeparse.ReasonTooLarge)

	// A body past the cap is refused before it is decoded.
	huge := `{"filename":"big.pdf","data_base64":"` + strings.Repeat("A", maxImportBody) + `"}`
	expectReason(t, call(t, s, "POST", "/v1/resume/import", huge), resumeparse.ReasonTooLarge)

	expectField(t, importText(t, s, strings.Repeat("a", maxPastedChars+1)), "text")

	// Large files and long text import, with a warning.
	long := strings.Repeat("Built dashboards in SQL and Tableau for the finance team.\n", longTextChars/50)
	out := expect(t, importText(t, s, long), 200)
	if w := out["size_warnings"].([]any); len(w) != 1 || !strings.Contains(w[0].(string), "pages of text") {
		t.Errorf("size_warnings = %v, want the long-text warning", w)
	}
	padded := append([]byte(plainResume), bytes.Repeat([]byte(" "), largeFileBytes)...)
	out = expect(t, importFile(t, s, "padded.txt", padded), 200)
	if w := out["size_warnings"].([]any); len(w) == 0 || !strings.Contains(w[0].(string), "MB") {
		t.Errorf("size_warnings = %v, want the file-size warning", w)
	}
}

func TestImportRequestValidation(t *testing.T) {
	s := newResumeServer(t)
	expectField(t, call(t, s, "POST", "/v1/resume/import", `{}`), "data_base64")
	expectField(t, call(t, s, "POST", "/v1/resume/import", `{"text":"a","data_base64":"YQ=="}`), "text")
	expectField(t, call(t, s, "POST", "/v1/resume/import", `{"text":"a","engine":"auto"}`), "engine")
	expectField(t, call(t, s, "POST", "/v1/resume/import", `{"text":"a","engine":"magic"}`), "engine")
	expectField(t, call(t, s, "POST", "/v1/resume/import", `{"text":"a","colour":"red"}`), "colour")
}

func TestSizeLabel(t *testing.T) {
	for n, want := range map[int]string{1: "1 KB", 39 << 10: "39 KB", 5 << 20: "5.0 MB", 2400000: "2.3 MB"} {
		if got := sizeLabel(n); got != want {
			t.Errorf("sizeLabel(%d) = %q, want %q", n, got, want)
		}
	}
}

const applyDraft = `{"draft":{
  "profile":{"full_name":"Ada Lovelace","email":"ada@new.example","skills":["python"]},
  "experiences":[{"company_name":"Acme","job_title":"Data Analyst","start_date":"2022-06","skills":[]}],
  "education":[{"institution":"State University","degree_level":"BS"}],
  "skills":["tableau"],
  "warnings":["ignored"]}`

func TestImportApply(t *testing.T) {
	s := newResumeServer(t)
	putProfile(t, s)
	expect(t, call(t, s, "POST", "/v1/resume/experiences", `{"company_name":"Old Co","job_title":"Clerk"}`), 201)

	out := expect(t, call(t, s, "POST", "/v1/resume/import/apply", applyDraft+`,"replace":false,
	  "save_copy":{"version_label":"Imported ada.pdf","content":"Ada Lovelace\n..."}}`), 200)
	p := out["profile"].(map[string]any)
	if p["email"] != "ada@new.example" {
		t.Errorf("email = %v, want the draft's", p["email"])
	}
	skills, _ := json.Marshal(p["skills"])
	if string(skills) != `["python","tableau"]` {
		t.Errorf("skills = %s, want the draft's, then the parsed list", skills)
	}
	exps := out["experiences"].([]any)
	if len(exps) != 2 || exps[0].(map[string]any)["company_name"] != "Old Co" || exps[1].(map[string]any)["start_date"] != "2022-06-01" {
		t.Errorf("experiences = %v, want the old entry kept and the new one after it", exps)
	}
	if len(out["education"].([]any)) != 1 {
		t.Errorf("education = %v", out["education"])
	}
	rec := call(t, s, "GET", "/v1/resumes", "")
	var saved []map[string]any
	json.Unmarshal(rec.Body.Bytes(), &saved)
	if len(saved) != 1 || saved[0]["version_label"] != "Imported ada.pdf" {
		t.Errorf("saved resumes = %v, want the copy", saved)
	}
}

func TestImportApplyReplace(t *testing.T) {
	s := newResumeServer(t)
	putProfile(t, s)
	expect(t, call(t, s, "POST", "/v1/resume/experiences", `{"company_name":"Old Co","job_title":"Clerk"}`), 201)
	expect(t, call(t, s, "POST", "/v1/resume/education", `{"institution":"Old School"}`), 201)

	out := expect(t, call(t, s, "POST", "/v1/resume/import/apply", applyDraft+`,"replace":true}`), 200)
	exps, edus := out["experiences"].([]any), out["education"].([]any)
	if len(exps) != 1 || exps[0].(map[string]any)["company_name"] != "Acme" || len(edus) != 1 || edus[0].(map[string]any)["institution"] != "State University" {
		t.Errorf("after replace: experiences %v, education %v", exps, edus)
	}
}

func TestImportApplyKeepsCurrentValues(t *testing.T) {
	s := newResumeServer(t)
	putProfile(t, s)
	out := expect(t, call(t, s, "POST", "/v1/resume/import/apply", `{"draft":{"profile":{"full_name":"","email":null,"phone":"555-0100"}}}`), 200)
	p := out["profile"].(map[string]any)
	if p["full_name"] != "Ada Lovelace" || p["email"] != "ada@example.com" || p["phone"] != "555-0100" {
		t.Errorf("profile = %v, want empty draft fields to keep current values", p)
	}
}

func TestImportApplyNeedsANameForANewProfile(t *testing.T) {
	s := newResumeServer(t)
	expectField(t, call(t, s, "POST", "/v1/resume/import/apply", `{"draft":{"profile":{"full_name":""}}}`), "full_name")
}

func TestImportApplyRollsBack(t *testing.T) {
	s := newResumeServer(t)
	putProfile(t, s)
	expect(t, call(t, s, "POST", "/v1/resume/experiences", `{"company_name":"Old Co","job_title":"Clerk"}`), 201)

	bad := `{"draft":{"profile":{"full_name":"Someone Else"},"experiences":[
	  {"company_name":"Acme","job_title":"Analyst"},
	  {"company_name":"Beta","job_title":"Analyst","start_date":"2023-01","end_date":"2022-01"}]},"replace":true}`
	rec := call(t, s, "POST", "/v1/resume/import/apply", bad)
	e := expect(t, rec, http.StatusUnprocessableEntity)["error"].(map[string]any)
	if d := e["details"].(map[string]any); d["experience_index"] != float64(1) || d["field"] != "end_date" {
		t.Errorf("details = %v, want experience_index 1 and field end_date", d)
	}
	rec = call(t, s, "POST", "/v1/resume/import/apply", `{"draft":{"education":[{"institution":"A"},{"institution":"B","gpa":9}]}}`)
	e = expect(t, rec, http.StatusUnprocessableEntity)["error"].(map[string]any)
	if d := e["details"].(map[string]any); d["education_index"] != float64(1) {
		t.Errorf("details = %v, want education_index 1", d)
	}

	out := expect(t, call(t, s, "GET", "/v1/resume", ""), 200)
	if out["profile"].(map[string]any)["full_name"] != "Ada Lovelace" || len(out["experiences"].([]any)) != 1 {
		t.Errorf("resume changed after a refused apply: %v", out)
	}
}

func TestImportApplyRejectsBadBodies(t *testing.T) {
	s := newResumeServer(t)
	putProfile(t, s)
	expectField(t, call(t, s, "POST", "/v1/resume/import/apply", `{"draft":{},"save_copy":{"version_label":"","content":"x"}}`), "save_copy.version_label")
	expectField(t, call(t, s, "POST", "/v1/resume/import/apply", `{"draft":{},"save_copy":{"version_label":"x","content":" "}}`), "save_copy.content")
	expectField(t, call(t, s, "POST", "/v1/resume/import/apply", `{"draft":{},"extra":1}`), "extra")
}
