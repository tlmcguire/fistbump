package resumeparse

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

// zipOf builds a ZIP archive holding the named files, each with the given content.
func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// refusal runs Detect and returns the *Error it gave, failing the test if there was none.
func refusal(t *testing.T, name string, data []byte) *Error {
	t.Helper()
	_, err := Detect(name, data)
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("Detect(%q) = %v, want an *Error", name, err)
	}
	return e
}

func TestDetectAcceptsPDFAndText(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want Kind
	}{
		{"resume.pdf", []byte("%PDF-1.7\n..."), KindPDF},
		{"noext", []byte("%PDF-1.4"), KindPDF}, // the content decides, not the file name
		{"resume.docx", zipOf(t, map[string]string{"word/document.xml": "<w/>"}), KindDOCX},
		{"resume.txt", []byte("Jane Doe\nGo, SQL"), KindText},
		{"resume.md", []byte("# Jane Doe\n\n## Skills\n"), KindText},
		{"résumé.txt", []byte("Łukasz Ñandú"), KindText},
		{"renamed.pdf", []byte("just text, whatever the name says"), KindText},
		{"", []byte("no file name at all"), KindText},
	}
	for _, c := range cases {
		if got, err := Detect(c.name, c.data); err != nil || got != c.want {
			t.Errorf("Detect(%q) = %v, %v, want %v", c.name, got, err, c.want)
		}
	}
}

func TestDetectFindsPDFAfterJunkWithinTheFirstKilobyte(t *testing.T) {
	junk := bytes.Repeat([]byte{' '}, 900)
	if got, err := Detect("a.pdf", append(junk, []byte("%PDF-1.4")...)); err != nil || got != KindPDF {
		t.Errorf("a header near the start should still count: %v, %v", got, err)
	}
	late := append(bytes.Repeat([]byte{' '}, 2000), []byte("%PDF-1.4")...)
	if got, err := Detect("a.pdf", late); err != nil || got != KindText {
		t.Errorf("a header far into the file is only text: %v, %v", got, err)
	}
}

func TestDetectRefusesEmptyAndTooLarge(t *testing.T) {
	if e := refusal(t, "a.pdf", nil); e.Reason != ReasonEmpty {
		t.Errorf("empty: %+v", e)
	}
	if e := refusal(t, "a.pdf", make([]byte, MaxFileBytes+1)); e.Reason != ReasonTooLarge {
		t.Errorf("too large: %+v", e)
	}
	if _, err := Detect("a.txt", bytes.Repeat([]byte("a"), MaxFileBytes)); err != nil {
		t.Errorf("exactly the limit is allowed: %v", err)
	}
}

func TestDetectRefusesOtherFormatsWithAdvice(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		detected string
		advice   string
	}{
		{"cv.doc", append([]byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"), make([]byte, 20)...), "doc", "Save it as .docx or PDF"},
		{"cv.pages", zipOf(t, map[string]string{"Index/Document.iwa": "x"}), "pages", "File > Export To > PDF"},
		{"cv.odt", zipOf(t, map[string]string{"mimetype": "application/vnd.oasis.opendocument.text"}), "odt", "File > Export as PDF"},
		{"cv.zip", zipOf(t, map[string]string{"a.txt": "x"}), "zip", "Save your resume as a PDF"},
		{"broken.zip", []byte("PK\x03\x04 not really a zip"), "zip", "Save your resume as a PDF"},
		{"scan.png", []byte("\x89PNG\r\n\x1A\n...."), "png", "paste the text"},
		{"scan.jpg", []byte("\xFF\xD8\xFF\xE0...."), "jpeg", "paste the text"},
		{"scan.gif", []byte("GIF89a...."), "gif", "paste the text"},
		{"scan.webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "webp", "paste the text"},
		{"photo.heic", []byte("\x00\x00\x00\x18ftypheic...."), "heic", "paste the text"},
		{"photo.avif", []byte("\x00\x00\x00\x18ftypavif...."), "avif", "paste the text"},
	}
	for _, c := range cases {
		e := refusal(t, c.name, c.data)
		if e.Reason != ReasonUnsupportedFormat || e.Detected != c.detected {
			t.Errorf("%s: reason %q, detected %q, want %q", c.name, e.Reason, e.Detected, c.detected)
		}
		if !strings.Contains(e.Message, c.advice) || !strings.HasPrefix(e.Message, c.name) {
			t.Errorf("%s: message %q should name the file and say %q", c.name, e.Message, c.advice)
		}
	}
}

// The file's bytes decide what it is, whatever its name says.
func TestDetectIgnoresTheFileExtension(t *testing.T) {
	if got, err := Detect("resume.pdf", zipOf(t, map[string]string{"word/document.xml": "<w/>"})); err != nil || got != KindDOCX {
		t.Errorf("a DOCX file renamed .pdf = %v, %v; want KindDOCX", got, err)
	}
	if e := refusal(t, "resume.txt", []byte("\x89PNG\r\n\x1A\n....")); e.Detected != "png" {
		t.Errorf("an image renamed .txt: %+v", e)
	}
}

func TestDetectRefusesUTF16AndBinary(t *testing.T) {
	for _, bom := range [][]byte{{0xFF, 0xFE, 'a', 0}, {0xFE, 0xFF, 0, 'a'}} {
		if e := refusal(t, "cv.txt", bom); e.Detected != "utf16" || !strings.Contains(e.Message, "UTF-8") {
			t.Errorf("UTF-16: %+v", e)
		}
	}
	for name, data := range map[string][]byte{
		"invalid UTF-8": {0xC3, 0x28, 'a', 'b'},
		"NUL byte":      []byte("text\x00more text"),
	} {
		if e := refusal(t, "cv.txt", data); e.Detected != "unknown" || e.Reason != ReasonUnsupportedFormat {
			t.Errorf("%s: %+v", name, e)
		}
	}
}

func TestDetectNamesTheFileInMessages(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1A\n....")
	cases := map[string]string{
		"scan.png":               "scan.png",
		`C:\Users\me\scan.png`:   "scan.png", // a Windows path gives its base name
		"/home/me/docs/scan.png": "scan.png",
		"":                       "This file",
		".":                      "This file",
	}
	for in, want := range cases {
		if e := refusal(t, in, png); !strings.HasPrefix(e.Message, want+" is an image") {
			t.Errorf("Detect(%q) message = %q, want it to start with %q", in, e.Message, want)
		}
	}
}

func TestErrorMessage(t *testing.T) {
	e := &Error{Reason: ReasonNoText, Message: "nothing here"}
	if e.Error() != "nothing here" {
		t.Errorf("Error() = %q", e.Error())
	}
}
