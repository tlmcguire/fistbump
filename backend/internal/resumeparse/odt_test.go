package resumeparse

import (
	"strings"
	"testing"
)

func buildODT(t *testing.T, contentXML string) []byte {
	t.Helper()
	return zipOf(t, map[string]string{
		"mimetype":              "application/vnd.oasis.opendocument.text",
		"META-INF/manifest.xml": `<?xml version="1.0" encoding="UTF-8"?><manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"/>`,
		"content.xml":           contentXML,
	})
}

func TestTextFromODTExtractsAndParsesResume(t *testing.T) {
	xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
  xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0"
  xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0">
  <office:automatic-styles>
    <style:style style:name="HiddenSpan" style:family="text">
      <style:text-properties text:display="none"/>
    </style:style>
    <style:style style:name="WhiteSpan" style:family="text">
      <style:text-properties fo:color="#ffffff"/>
    </style:style>
  </office:automatic-styles>
  <office:body>
    <office:text>
      <text:h text:outline-level="1">Ada Lovelace</text:h>
      <text:p>ada@example.com<text:tab/>Charleston, SC</text:p>
      <text:h text:outline-level="2">Experience</text:h>
      <text:p>Senior Software Engineer, Analytical Engines (Jan 2022 - Present)</text:p>
      <text:list>
        <text:list-item><text:p>Built Go and SQL pipelines serving millions of requests daily.</text:p></text:list-item>
        <text:list-item><text:p>Reduced query latency by 45% across distributed databases.<text:span text:style-name="HiddenSpan"> secret keyword stuffing</text:span><text:span text:style-name="WhiteSpan"> invisible prompt</text:span></text:p></text:list-item>
      </text:list>
      <text:h text:outline-level="2">Education</text:h>
      <text:p>B.S. in Computer Science, University of London (May 2020)</text:p>
      <text:h text:outline-level="2">Skills</text:h>
      <text:p>Go, SQL, Python, Docker</text:p>
    </office:text>
  </office:body>
</office:document-content>`

	data := buildODT(t, xmlBody)
	kind, err := Detect("resume.odt", data)
	if err != nil || kind != KindODT {
		t.Fatalf("Detect(resume.odt) = %v, %v; want KindODT", kind, err)
	}

	ex, err := TextFromODT(data)
	if err != nil {
		t.Fatalf("TextFromODT: %v", err)
	}
	if !strings.Contains(ex.Text, "Ada Lovelace") || !strings.Contains(ex.Text, "Built Go and SQL pipelines") {
		t.Errorf("extracted text missing expected content:\n%s", ex.Text)
	}
	if strings.Contains(ex.Text, "secret keyword") || strings.Contains(ex.Text, "invisible prompt") {
		t.Errorf("hidden text leaked into extracted text:\n%s", ex.Text)
	}
	if ex.Hidden.Empty() || ex.Hidden.Words < 5 {
		t.Errorf("expected hidden text to be reported, got %+v", ex.Hidden)
	}

	draft := Parse(ex.Text)
	if draft.Profile.FullName != "Ada Lovelace" {
		t.Errorf("FullName = %q, want Ada Lovelace", draft.Profile.FullName)
	}
}

func TestTextFromODTErrors(t *testing.T) {
	if _, err := TextFromODT([]byte("not a zip")); err == nil {
		t.Error("expected error on corrupt zip")
	}
	empty := buildODT(t, `<?xml version="1.0"?><office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"><office:body><office:text><text:p>hi</text:p></office:text></office:body></office:document-content>`)
	if _, err := TextFromODT(empty); err == nil {
		t.Error("expected error on ODT with too few words")
	}
}
