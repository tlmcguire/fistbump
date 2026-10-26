// Determines what has been uploaded by size (or lack thereof); text the reader would not see is left out and reported, and nothing is saved here.
package resumeparse

import (
	"archive/zip"
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// file type that can be imported
type Kind int

const (
	KindPDF Kind = iota + 1
	KindText
)

// Largest file that can be imported (5MB)
const MaxFileBytes = 5 << 20

// Reasons a file is refused. The API reports them as details.reason.
const (
	ReasonEmpty             = "empty"
	ReasonInvalidEncoding   = "invalid_encoding"
	ReasonTooLarge          = "too_large"
	ReasonUnsupportedFormat = "unsupported_format"
	ReasonEncrypted         = "encrypted"
	ReasonNoText            = "no_text"
	ReasonUnreadable        = "unreadable"
)

type Error struct {
	Reason   string
	Detected string
	Message  string
}

func (e *Error) Error() string { return e.Message }

// known PDF readers accept a PDF within the first 1024 bytes
const pdfSearch = 1024

// detects what was uploaded
func Detect(filename string, data []byte) (Kind, error) {
	name := "This file"
	if base := filepath.Base(strings.ReplaceAll(filename, `\`, "/")); filename != "" && base != "." && base != "/" {
		name = base
	}
	if len(data) == 0 {
		return 0, &Error{Reason: ReasonEmpty, Message: "The file is empty or could not be read."}
	}
	if len(data) > MaxFileBytes {
		return 0, &Error{Reason: ReasonTooLarge, Message: "The file size is too large, it must be less than 5MB."}
	}
	if bytes.Contains(data[:min(len(data), pdfSearch+5)], []byte("%PDF-")) {
		return KindPDF, nil
	}
	if detected, what := magic(data); detected != "" {
		return 0, unsupported(name, detected, what)
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) || bytes.HasPrefix(data, []byte{0xFE, 0xFF}) {
		return 0, &Error{Reason: ReasonUnsupportedFormat, Detected: "utf16",
			Message: name + " is text saved as UTF-16. Save it as UTF-8 text, or as a PDF, and import that."}
	}
	if utf8.Valid(data) && bytes.IndexByte(data, 0) < 0 {
		return KindText, nil
	}
	return 0, &Error{Reason: ReasonUnsupportedFormat, Detected: "unknown",
		Message: name + " is not a PDF or a text file. Only PDF, .txt and .md files can be imported."}
}

// file signatures for what was detected
func magic(data []byte) (detected, what string) {
	has := func(prefix string) bool { return bytes.HasPrefix(data, []byte(prefix)) }
	switch {
	case has("PK\x03\x04"):
		return zipKind(data)
	case has("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"):
		return "doc", "a Word document"
	case has("\x89PNG\r\n\x1A\n"):
		return "png", "an image"
	case has("\xFF\xD8\xFF"):
		return "jpeg", "an image"
	case has("GIF87a"), has("GIF89a"):
		return "gif", "an image"
	case len(data) >= 12 && has("RIFF") && string(data[8:12]) == "WEBP":
		return "webp", "an image"
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		switch string(data[8:12]) {
		case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1":
			return "heic", "an image"
		case "avif", "avis":
			return "avif", "an image"
		}
	}
	return "", ""
}

// zipKind looks inside a ZIP container: Word, OpenDocument and Pages files are all ZIP archives.
func zipKind(data []byte) (detected, what string) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "zip", "a ZIP archive"
	}
	for _, f := range zr.File {
		switch {
		case strings.HasPrefix(f.Name, "word/"):
			return "docx", "a Word document"
		case f.Name == "Index/Document.iwa" || f.Name == "index.xml" || strings.HasSuffix(f.Name, ".iwa"):
			return "pages", "a Pages document"
		case f.Name == "mimetype":
			if rc, err := f.Open(); err == nil {
				buf := make([]byte, 64)
				n, _ := rc.Read(buf)
				rc.Close()
				if strings.HasPrefix(string(buf[:n]), "application/vnd.oasis.opendocument") {
					return "odt", "an OpenDocument file"
				}
			}
		}
	}
	return "zip", "a ZIP archive"
}

// User tried to import a non-PDF
func unsupported(name, detected, what string) *Error {
	var msg string
	switch {
	case what == "an image":
		msg = fmt.Sprintf("%s is %s. Import a PDF with selectable text, or paste the text instead.", name, what)
	case detected == "docx" || detected == "doc":
		msg = fmt.Sprintf("%s is %s. Save it as a PDF (File > Save As > PDF) and import that.", name, what)
	case detected == "pages":
		msg = fmt.Sprintf("%s is %s. Export it as a PDF (File > Export To > PDF) and import that.", name, what)
	case detected == "odt":
		msg = fmt.Sprintf("%s is %s. Export it as a PDF (File > Export as PDF) and import that.", name, what)
	default:
		msg = fmt.Sprintf("%s is %s. Save your resume as a PDF and import that.", name, what)
	}
	return &Error{Reason: ReasonUnsupportedFormat, Detected: detected, Message: msg}
}
