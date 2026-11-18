package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/db"
	"github.com/tlmcguire/fistbump/backend/internal/models"
)

const pdfMagic = "%PDF-fake"

// fakePDF stands in for the pdf package: it returns a marker followed by the Markdown it was given.
func fakePDF(markdown, _ string) ([]byte, error) { return []byte(pdfMagic + markdown), nil }

// newExportServer builds a server whose PDF renderer is render.
func newExportServer(t *testing.T, render func(string, string) ([]byte, error)) *Server {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	s := New(Config{DB: conn, Greenhouse: &fakeFetcher{}, Token: testToken, Version: "test", PDF: render})
	s.parse, s.analyze = fakeParse, fakeAnalyze
	return s
}

// seedTailored saves a profile, a job and a tailored resume, and returns the tailored resume's id.
func seedTailored(t *testing.T, s *Server, fullName, company, position string) string {
	t.Helper()
	expect(t, call(t, s, "PUT", "/v1/resume/profile", fmt.Sprintf(`{"full_name":%q}`, fullName)), 200)
	job := expect(t, call(t, s, "POST", "/v1/jobs",
		fmt.Sprintf(`{"source":"manual","company_name":%q,"position_title":%q,"raw_text":"x"}`, company, position)), 201)
	tailored, err := s.store.CreateTailored(context.Background(), models.TailoredResume{
		JobID: int64(job["id"].(float64)), Content: "# Ada\n\n## Skills\n\nsql, excel\n", BaseContent: "# Ada\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprint(tailored.ID)
}

func TestTailoredExportMarkdown(t *testing.T) {
	s := newExportServer(t, fakePDF)
	id := seedTailored(t, s, "Ada Lovelace", "Acme", "Data Analyst")

	rec := call(t, s, "POST", "/v1/tailored-resumes/"+id+"/export", `{"format":"md"}`)
	if rec.Code != 200 || rec.Body.String() != "# Ada\n\n## Skills\n\nsql, excel\n" {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="Ada Lovelace - Acme - Data Analyst.md"`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if rec.Header().Get("Content-Length") != fmt.Sprint(rec.Body.Len()) {
		t.Errorf("Content-Length = %q for %d bytes", rec.Header().Get("Content-Length"), rec.Body.Len())
	}
}

func TestTailoredExportPDF(t *testing.T) {
	s := newExportServer(t, fakePDF)
	id := seedTailored(t, s, "Ada Lovelace", "Acme", "Data Analyst")

	rec := call(t, s, "POST", "/v1/tailored-resumes/"+id+"/export", `{"format":"pdf"}`)
	if rec.Code != 200 || !strings.HasPrefix(rec.Body.String(), pdfMagic+"# Ada") {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), `filename="Ada Lovelace - Acme - Data Analyst.pdf"`) {
		t.Errorf("Content-Disposition = %q", rec.Header().Get("Content-Disposition"))
	}
}

// The PDF renderer gets the download file name, so it can title the document after it.
func TestPDFGetsDownloadFileName(t *testing.T) {
	var got []string
	s := newExportServer(t, func(markdown, fileName string) ([]byte, error) {
		got = append(got, fileName)
		return fakePDF(markdown, fileName)
	})
	id := seedTailored(t, s, "Ada Lovelace", "Acme", "Data Analyst")
	expect(t, call(t, s, "POST", "/v1/tailored-resumes/"+id+"/export", `{"format":"pdf"}`), 200)
	expect(t, call(t, s, "POST", "/v1/resume/export", `{"format":"pdf"}`), 200)
	want := []string{"Ada Lovelace - Acme - Data Analyst.pdf", "Ada Lovelace - Resume.pdf"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("file names = %q, want %q", got, want)
	}
}

func TestExportFileNameIsSanitized(t *testing.T) {
	s := newExportServer(t, fakePDF)
	id := seedTailored(t, s, "Ada Lovelace", "Acme/Co: Inc", `Data "Analyst"?`)
	rec := call(t, s, "POST", "/v1/tailored-resumes/"+id+"/export", `{"format":"md"}`)
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="Ada Lovelace - AcmeCo Inc - Data Analyst.md"`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

func TestExportNonASCIINameUsesFilenameStar(t *testing.T) {
	s := newExportServer(t, fakePDF)
	id := seedTailored(t, s, "Jan Łukasiewicz", "Acme", "Analyst")
	rec := call(t, s, "POST", "/v1/tailored-resumes/"+id+"/export", `{"format":"pdf"}`)
	cd := rec.Header().Get("Content-Disposition")
	if !strings.Contains(cd, `filename="Jan _ukasiewicz - Acme - Analyst.pdf"`) ||
		!strings.Contains(cd, `filename*=UTF-8''Jan%20%C5%81ukasiewicz%20-%20Acme%20-%20Analyst.pdf`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

func TestMasterExport(t *testing.T) {
	s := newExportServer(t, fakePDF)
	putProfile(t, s)

	md := call(t, s, "POST", "/v1/resume/export", `{"format":"md"}`)
	if md.Code != 200 || !strings.HasPrefix(md.Body.String(), "# Ada Lovelace") {
		t.Fatalf("md: status %d, body %q", md.Code, md.Body)
	}
	if !strings.Contains(md.Header().Get("Content-Disposition"), `filename="Ada Lovelace - Resume.md"`) {
		t.Errorf("Content-Disposition = %q", md.Header().Get("Content-Disposition"))
	}
	pdf := call(t, s, "POST", "/v1/resume/export", `{"format":"pdf"}`)
	if pdf.Code != 200 || !strings.HasPrefix(pdf.Body.String(), pdfMagic+"# Ada Lovelace") ||
		pdf.Header().Get("Content-Type") != "application/pdf" {
		t.Errorf("pdf: status %d, type %q, body %q", pdf.Code, pdf.Header().Get("Content-Type"), pdf.Body)
	}
}

func TestExportErrors(t *testing.T) {
	s := newExportServer(t, fakePDF)
	wantError(t, call(t, s, "POST", "/v1/resume/export", `{"format":"pdf"}`), 404, "not_found") // no profile
	wantError(t, call(t, s, "POST", "/v1/tailored-resumes/99/export", `{"format":"pdf"}`), 404, "not_found")
	wantError(t, call(t, s, "POST", "/v1/tailored-resumes/abc/export", `{"format":"pdf"}`), 404, "not_found")

	id := seedTailored(t, s, "Ada", "Acme", "Analyst")
	for _, body := range []string{`{"format":"docx"}`, `{"format":""}`, `{}`, `{"format":"PDF"}`} {
		expectField(t, call(t, s, "POST", "/v1/tailored-resumes/"+id+"/export", body), "format")
		expectField(t, call(t, s, "POST", "/v1/resume/export", body), "format")
	}
	wantError(t, call(t, s, "POST", "/v1/tailored-resumes/"+id+"/export", `not json`), 400, "bad_request")
	wantError(t, call(t, s, "POST", "/v1/tailored-resumes/"+id+"/export", `{"format":"pdf","path":"/tmp/x"}`), 422, "validation_failed")
}

// A PDF that cannot be made answers 500 with details.fallback "md", and Markdown still works.
func TestPDFFailureOffersMarkdown(t *testing.T) {
	cases := map[string]func(string, string) ([]byte, error){
		"error":     func(string, string) ([]byte, error) { return nil, errors.New("font failed") },
		"panic":     func(string, string) ([]byte, error) { panic("fpdf broke") },
		"not wired": nil,
	}
	for name, render := range cases {
		t.Run(name, func(t *testing.T) {
			s := newExportServer(t, render)
			id := seedTailored(t, s, "Ada", "Acme", "Analyst")

			rec := call(t, s, "POST", "/v1/tailored-resumes/"+id+"/export", `{"format":"pdf"}`)
			body := decodeError(t, rec)
			if rec.Code != http.StatusInternalServerError || body.Error.Code != "internal" || body.Error.Details["fallback"] != "md" {
				t.Fatalf("status %d, error %+v", rec.Code, body.Error)
			}
			if strings.Contains(rec.Body.String(), "font failed") || strings.Contains(rec.Body.String(), "fpdf broke") {
				t.Errorf("the renderer's message must not reach the client: %s", rec.Body)
			}
			if md := call(t, s, "POST", "/v1/tailored-resumes/"+id+"/export", `{"format":"md"}`); md.Code != 200 {
				t.Errorf("Markdown must still export, got %d", md.Code)
			}
			if master := call(t, s, "POST", "/v1/resume/export", `{"format":"pdf"}`); master.Code != 500 {
				t.Errorf("the master export fails the same way, got %d", master.Code)
			}
		})
	}
}

func TestFileName(t *testing.T) {
	long := strings.Repeat("a", 200)
	cases := []struct {
		parts []string
		want  string
	}{
		{[]string{"Ada Lovelace", "Acme", "Data Analyst"}, "Ada Lovelace - Acme - Data Analyst"},
		{[]string{"Ada Lovelace", "Resume"}, "Ada Lovelace - Resume"},
		{[]string{"Ada", "A/B: C*D?", `"E"<F>|G`}, "Ada - AB CD - EFG"},
		{[]string{"  Ada \t Lovelace\n", "", "Analyst"}, "Ada Lovelace - Analyst"},
		{[]string{"Ada", "Acme.", "Analyst."}, "Ada - Acme. - Analyst"}, // no trailing dot: Windows refuses it
		{[]string{"", "", ""}, "Resume"},
		{[]string{"///", ":::"}, "Resume"},
		{[]string{long}, strings.Repeat("a", 120)},
		{[]string{"Jan Łukasiewicz", "Café Ñandú"}, "Jan Łukasiewicz - Café Ñandú"},
		{[]string{"A\x00B\x1fC\x7fD"}, "ABCD"},
	}
	for _, c := range cases {
		if got := fileName(c.parts...); got != c.want {
			t.Errorf("fileName(%q) = %q, want %q", c.parts, got, c.want)
		}
	}
	if got := fileName(strings.Repeat("é", 150)); len([]rune(got)) != 120 {
		t.Errorf("the cap counts characters, not bytes: got %d runes", len([]rune(got)))
	}
}

func TestContentDisposition(t *testing.T) {
	cases := []struct{ name, want string }{
		{"Ada - Resume.pdf", `attachment; filename="Ada - Resume.pdf"; filename*=UTF-8''Ada%20-%20Resume.pdf`},
		{"Łukasz.md", `attachment; filename="_ukasz.md"; filename*=UTF-8''%C5%81ukasz.md`},
		{`a"b\c.pdf`, `attachment; filename="a_b_c.pdf"; filename*=UTF-8''a%22b%5Cc.pdf`},
		{"a,b;c='d'(e).pdf", `attachment; filename="a,b;c='d'(e).pdf"; filename*=UTF-8''a%2Cb%3Bc%3D%27d%27%28e%29.pdf`},
	}
	for _, c := range cases {
		if got := contentDisposition(c.name); got != c.want {
			t.Errorf("contentDisposition(%q)\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}
