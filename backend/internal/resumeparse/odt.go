package resumeparse

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
)

// maxODTEntryBytes caps any single decompressed XML file inside an .odt archive to prevent zip bombs.
const maxODTEntryBytes = 10 << 20

var (
	errODTUnreadable = &Error{Reason: ReasonUnreadable,
		Message: "The OpenDocument file (.odt) could not be read. Try saving it again in LibreOffice or exporting as PDF."}
	errODTNoText = &Error{Reason: ReasonNoText,
		Message: "No readable text was found in this OpenDocument file. Paste the text of your resume instead."}
)

// TextFromODT extracts visible text from a LibreOffice / OpenDocument (.odt) file in reading order.
// Styles marked hidden (text:display="none"), white-on-white text, or sub-2pt text are dropped and
// reported in Hidden, matching TextFromPDF and TextFromDOCX.
func TextFromODT(data []byte) (Extracted, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Extracted{}, errODTUnreadable
	}

	var contentFile, stylesFile *zip.File
	for _, f := range zr.File {
		switch f.Name {
		case "content.xml":
			contentFile = f
		case "styles.xml":
			stylesFile = f
		}
	}
	if contentFile == nil {
		return Extracted{}, errODTUnreadable
	}

	hiddenStyles := map[string]string{}
	if stylesFile != nil {
		if raw, err := readZipEntry(stylesFile, maxODTEntryBytes); err == nil {
			collectODTHiddenStyles(raw, hiddenStyles)
		}
	}

	raw, err := readZipEntry(contentFile, maxODTEntryBytes)
	if err != nil {
		return Extracted{}, errODTUnreadable
	}
	collectODTHiddenStyles(raw, hiddenStyles)

	log := &hiddenLog{}
	txt, err := extractODTBody(raw, hiddenStyles, log)
	if err != nil {
		return Extracted{}, errODTUnreadable
	}

	text := stripInvisible(strings.TrimSpace(txt), log)
	if len(strings.Fields(text)) < minWords {
		return Extracted{}, errODTNoText
	}
	return Extracted{Text: text, Hidden: log.report()}, nil
}

func readZipEntry(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, limit))
}

// collectODTHiddenStyles records style names whose text-properties hide text.
func collectODTHiddenStyles(raw []byte, out map[string]string) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	curStyle := ""
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "style":
				curStyle = strings.TrimSpace(attrVal(t.Attr, "name"))
			case "text-properties":
				if curStyle == "" {
					continue
				}
				if reason := odtHiddenReason(t.Attr); reason != "" {
					out[curStyle] = reason
				}
			}
		case xml.EndElement:
			if t.Name.Local == "style" {
				curStyle = ""
			}
		}
	}
}

func odtHiddenReason(attrs []xml.Attr) string {
	if strings.EqualFold(strings.TrimSpace(attrVal(attrs, "display")), "none") {
		return "hidden formatting"
	}
	color := strings.ToUpper(strings.TrimSpace(attrVal(attrs, "color")))
	if color == "#FFFFFF" || color == "#FEFEFE" || color == "#FDFDFD" || color == "WHITE" {
		return "same color as the background"
	}
	fs := strings.ToLower(strings.TrimSpace(attrVal(attrs, "font-size")))
	if strings.HasSuffix(fs, "pt") {
		if v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(fs, "pt")), 64); err == nil && v > 0 && v <= 2.0 {
			return "tiny text"
		}
	}
	return ""
}

func extractODTBody(raw []byte, hiddenStyles map[string]string, log *hiddenLog) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var (
		lines       []string
		inBody      bool
		inPara      bool
		inListItem  bool
		firstInItem bool
		inTableRow  bool
		paraBuf     strings.Builder
		cellBufs    []string
		hiddenStack []string
	)

	currentHidden := func() string {
		for i := len(hiddenStack) - 1; i >= 0; i-- {
			if hiddenStack[i] != "" {
				return hiddenStack[i]
			}
		}
		return ""
	}

	flushPara := func() {
		line := strings.TrimSpace(paraBuf.String())
		paraBuf.Reset()
		if line == "" {
			if !inTableRow && len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			return
		}
		if inListItem && firstInItem && !isBullet(line) {
			line = "- " + line
		}
		firstInItem = false
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
			local := t.Name.Local
			if local == "text" && t.Name.Space != "" && strings.Contains(t.Name.Space, "office") {
				inBody = true
			}
			if !inBody && local == "body" {
				inBody = true
			}
			styleName := strings.TrimSpace(attrVal(t.Attr, "style-name"))
			hiddenStack = append(hiddenStack, hiddenStyles[styleName])
			if !inBody {
				continue
			}
			switch local {
			case "table-row":
				inTableRow = true
				cellBufs = nil
			case "list-item":
				inListItem = true
				firstInItem = true
			case "p", "h":
				inPara = true
				paraBuf.Reset()
			case "tab":
				if inPara && currentHidden() == "" {
					paraBuf.WriteString("  |  ")
				}
			case "line-break":
				if inPara && currentHidden() == "" {
					paraBuf.WriteByte('\n')
				}
			case "s":
				if inPara && currentHidden() == "" {
					count := 1
					if c, err := strconv.Atoi(strings.TrimSpace(attrVal(t.Attr, "c"))); err == nil && c > 0 && c <= 32 {
						count = c
					}
					paraBuf.WriteString(strings.Repeat(" ", count))
				}
			}
		case xml.EndElement:
			if len(hiddenStack) > 0 {
				hiddenStack = hiddenStack[:len(hiddenStack)-1]
			}
			local := t.Name.Local
			if local == "body" {
				inBody = false
			}
			if !inBody {
				continue
			}
			switch local {
			case "p", "h":
				if inPara {
					flushPara()
					inPara = false
				}
			case "list-item":
				inListItem = false
				firstInItem = false
			case "table-row":
				if inTableRow {
					if len(cellBufs) > 0 {
						lines = append(lines, strings.Join(cellBufs, "  |  "))
					}
					cellBufs = nil
					inTableRow = false
				}
			}
		case xml.CharData:
			if !inBody || !inPara {
				continue
			}
			s := string(t)
			if reason := currentHidden(); reason != "" {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					log.gap()
					log.add(reason, trimmed, len([]rune(trimmed)))
				}
			} else {
				paraBuf.WriteString(s)
			}
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}
