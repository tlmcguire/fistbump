package resumeparse

import (
	"strings"
	"testing"
)

func TestTextFromDOCXExtractsVisibleContentAndFiltersHidden(t *testing.T) {
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Jane Q. Public</w:t></w:r></w:p>
    <w:p><w:r><w:t>jane@example.org</w:t><w:tab/><w:t>Charleston, SC</w:t></w:r></w:p>
    <w:p><w:r><w:t>EXPERIENCE</w:t></w:r></w:p>
    <w:p><w:r><w:t>Software Engineer, Globex Inc.</w:t></w:r></w:p>
    <w:p><w:r><w:t>Jan 2021 - Present</w:t></w:r></w:p>
    <w:p>
      <w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr>
      <w:r><w:t>Built distributed Go services handling 10k requests per second.</w:t></w:r>
    </w:p>
    <w:p>
      <w:r><w:rPr><w:vanish/></w:rPr><w:t>secret prompt injection keywords</w:t></w:r>
      <w:r><w:rPr><w:color w:val="FFFFFF"/></w:rPr><w:t>invisible white text</w:t></w:r>
      <w:r><w:rPr><w:sz w:val="2"/></w:rPr><w:t>tiny hidden text</w:t></w:r>
    </w:p>
    <w:p><w:r><w:t>SKILLS</w:t></w:r></w:p>
    <w:p><w:r><w:t>Go, SQL, Docker</w:t></w:r></w:p>
  </w:body>
</w:document>`

	data := zipOf(t, map[string]string{
		"word/document.xml": docXML,
	})

	kind, err := Detect("resume.docx", data)
	if err != nil || kind != KindDOCX {
		t.Fatalf("Detect(resume.docx) = %v, %v; want KindDOCX", kind, err)
	}

	ex, err := TextFromDOCX(data)
	if err != nil {
		t.Fatalf("TextFromDOCX failed: %v", err)
	}
	for _, secret := range []string{"secret prompt injection", "invisible white text", "tiny hidden text"} {
		if strings.Contains(ex.Text, secret) {
			t.Errorf("hidden text leaked into extracted DOCX text: %q", ex.Text)
		}
	}
	if ex.Hidden.Empty() || ex.Hidden.Words < 6 {
		t.Errorf("expected hidden text to be reported, got %+v", ex.Hidden)
	}

	draft := Parse(ex.Text)
	if draft.Profile.FullName != "Jane Q. Public" {
		t.Errorf("FullName = %q, want Jane Q. Public", draft.Profile.FullName)
	}
	if len(draft.Experiences) != 1 || draft.Experiences[0].JobTitle != "Software Engineer" {
		t.Errorf("Experiences = %+v", draft.Experiences)
	}
	if len(draft.Skills) != 3 {
		t.Errorf("Skills = %v, want 3 skills", draft.Skills)
	}
}

func TestTextFromDOCXHandlesEmptyAndCorruptFiles(t *testing.T) {
	emptyDoc := zipOf(t, map[string]string{
		"word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Hi</w:t></w:r></w:p></w:body></w:document>`,
	})
	if _, err := TextFromDOCX(emptyDoc); err == nil {
		t.Errorf("expected error for DOCX with fewer than minWords")
	}
	if _, err := TextFromDOCX([]byte("not a zip")); err == nil {
		t.Errorf("expected error for corrupt DOCX")
	}
}
