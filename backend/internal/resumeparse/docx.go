package resumeparse

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"sort"
	"strings"
)

// maxDOCXEntryBytes caps any single decompressed XML file inside a .docx archive to prevent zip bombs.
const maxDOCXEntryBytes = 10 << 20

var (
	errDOCXUnreadable = &Error{Reason: ReasonUnreadable,
		Message: "The Word document (.docx) could not be read. Try saving it again or exporting as PDF."}
	errDOCXNoText = &Error{Reason: ReasonNoText,
		Message: "No readable text was found in this Word document. Paste the text of your resume instead."}
)

// TextFromDOCX extracts visible text from a .docx file in reading order (headers, body, footers).
// Runs marked hidden (<w:vanish/>, <w:webHidden/>), white-on-white text, or sub-2pt text are dropped
// and reported in Hidden, matching TextFromPDF's hidden-text protection.
func TextFromDOCX(data []byte) (Extracted, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Extracted{}, errDOCXUnreadable
	}

	var headers, bodies, footers []*zip.File
	for _, f := range zr.File {
		switch {
		case f.Name == "word/document.xml":
			bodies = append(bodies, f)
		case strings.HasPrefix(f.Name, "word/header") && strings.HasSuffix(f.Name, ".xml"):
			headers = append(headers, f)
		case strings.HasPrefix(f.Name, "word/footer") && strings.HasSuffix(f.Name, ".xml"):
			footers = append(footers, f)
		}
	}
	if len(bodies) == 0 {
		return Extracted{}, errDOCXUnreadable
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })
	sort.Slice(footers, func(i, j int) bool { return footers[i].Name < footers[j].Name })

	log := &hiddenLog{}
	var sections []string
	for _, group := range [][]*zip.File{headers, bodies, footers} {
		for _, f := range group {
			txt, err := extractDOCXPart(f, log)
			if err != nil {
				return Extracted{}, errDOCXUnreadable
			}
			if txt != "" {
				sections = append(sections, txt)
			}
		}
	}

	text := stripInvisible(strings.TrimSpace(strings.Join(sections, "\n\n")), log)
	if len(strings.Fields(text)) < minWords {
		return Extracted{}, errDOCXNoText
	}
	return Extracted{Text: text, Hidden: log.report()}, nil
}

func extractDOCXPart(f *zip.File, log *hiddenLog) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	raw, err := io.ReadAll(io.LimitReader(rc, maxDOCXEntryBytes))
	if err != nil {
		return "", err
	}

	dec := xml.NewDecoder(bytes.NewReader(raw))
	var (
		lines      []string
		inPara     bool
		inRun      bool
		inRunProps bool
		inText     bool
		inTableRow bool
		paraHasNum bool
		runHidden  string
		paraBuf    strings.Builder
		cellBufs   []string
	)

	flushPara := func() {
		line := strings.TrimSpace(paraBuf.String())
		paraBuf.Reset()
		if line == "" {
			if !inTableRow && len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			return
		}
		if paraHasNum && !isBullet(line) {
			line = "- " + line
		}
		if inTableRow {
			cellBufs = append(cellBufs, line)
		} else {
			lines = append(lines, line)
		}
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "tr":
				inTableRow = true
				cellBufs = nil
			case "p":
				inPara = true
				paraHasNum = false
				paraBuf.Reset()
			case "numPr":
				if inPara {
					paraHasNum = true
				}
			case "r":
				if inPara {
					inRun = true
					runHidden = ""
				}
			case "rPr":
				if inRun {
					inRunProps = true
				}
			case "vanish", "webHidden":
				if inRunProps && !isExplicitFalse(t.Attr) {
					runHidden = "hidden formatting"
				}
			case "color":
				if inRunProps {
					val := strings.ToUpper(strings.TrimSpace(attrVal(t.Attr, "val")))
					if val == "FFFFFF" || val == "FEFEFE" || val == "FDFDFD" {
						runHidden = "same color as the background"
					}
				}
			case "sz":
				if inRunProps {
					// OOXML w:sz is in half-points; 4 half-points = 2pt.
					val := strings.TrimSpace(attrVal(t.Attr, "val"))
					if val == "1" || val == "2" || val == "3" || val == "4" {
						runHidden = "tiny text"
					}
				}
			case "t":
				if inRun {
					inText = true
				}
			case "tab":
				if inRun && runHidden == "" {
					paraBuf.WriteString("  |  ")
				}
			case "br", "cr":
				if inRun && runHidden == "" {
					paraBuf.WriteByte('\n')
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "rPr":
				inRunProps = false
			case "r":
				inRun = false
				runHidden = ""
			case "p":
				if inPara {
					flushPara()
					inPara = false
					paraHasNum = false
				}
			case "tr":
				if inTableRow {
					if len(cellBufs) > 0 {
						lines = append(lines, strings.Join(cellBufs, "  |  "))
					}
					cellBufs = nil
					inTableRow = false
				}
			}
		case xml.CharData:
			if inText {
				s := string(t)
				if runHidden != "" {
					if trimmed := strings.TrimSpace(s); trimmed != "" {
						log.gap()
						log.add(runHidden, trimmed, len([]rune(trimmed)))
					}
				} else {
					paraBuf.WriteString(s)
				}
			}
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func attrVal(attrs []xml.Attr, local string) string {
	for _, a := range attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func isExplicitFalse(attrs []xml.Attr) bool {
	v := strings.ToLower(strings.TrimSpace(attrVal(attrs, "val")))
	return v == "0" || v == "false" || v == "off"
}
