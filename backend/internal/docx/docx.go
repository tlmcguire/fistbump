// Package docx turns resume Markdown into clean, ATS-safe single-column .docx (OOXML) documents
// using only the Go standard library.
package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

type styleSpec struct {
	font       string
	accentHex  string
	nameHalfPt int
	h2HalfPt   int
	bodyHalfPt int
	afterTwips int
}

func specFor(style string) styleSpec {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "modern":
		return styleSpec{font: "Calibri", accentHex: "1D4ED8", nameHalfPt: 36, h2HalfPt: 22, bodyHalfPt: 20, afterTwips: 80}
	case "minimal":
		return styleSpec{font: "Arial", accentHex: "374151", nameHalfPt: 32, h2HalfPt: 20, bodyHalfPt: 20, afterTwips: 70}
	case "compact":
		return styleSpec{font: "Calibri", accentHex: "111827", nameHalfPt: 30, h2HalfPt: 20, bodyHalfPt: 18, afterTwips: 45}
	case "executive":
		return styleSpec{font: "Georgia", accentHex: "1E3A5F", nameHalfPt: 38, h2HalfPt: 22, bodyHalfPt: 21, afterTwips: 90}
	default: // classic
		return styleSpec{font: "Georgia", accentHex: "111827", nameHalfPt: 34, h2HalfPt: 22, bodyHalfPt: 20, afterTwips: 80}
	}
}

// RenderStyled builds a single-column ATS-safe .docx file from resume Markdown.
func RenderStyled(markdown, style string) ([]byte, error) {
	sp := specFor(style)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	files := []struct {
		name string
		body string
	}{
		{"[Content_Types].xml", contentTypesXML},
		{"_rels/.rels", relsXML},
		{"word/_rels/document.xml.rels", docRelsXML},
		{"word/document.xml", buildDocumentXML(markdown, sp)},
	}
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f.body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func escapeXML(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func unescapeMD(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `\-`) || strings.HasPrefix(s, `\*`) || strings.HasPrefix(s, `\#`) {
		s = s[1:]
	}
	return s
}

func paragraph(text string, sp styleSpec, halfPt int, bold, italic bool, colorHex string, before, after int, bottomRule bool, indentLeft int) string {
	var pPr strings.Builder
	pPr.WriteString("<w:pPr>")
	fmt.Fprintf(&pPr, `<w:spacing w:before="%d" w:after="%d"/>`, before, after)
	if indentLeft > 0 {
		fmt.Fprintf(&pPr, `<w:ind w:left="%d" w:hanging="180"/>`, indentLeft)
	}
	if bottomRule {
		fmt.Fprintf(&pPr, `<w:pBdr><w:bottom w:val="single" w:sz="6" w:space="3" w:color="%s"/></w:pBdr>`, sp.accentHex)
	}
	pPr.WriteString("</w:pPr>")

	var rPr strings.Builder
	rPr.WriteString("<w:rPr>")
	fmt.Fprintf(&rPr, `<w:rFonts w:ascii="%s" w:hAnsi="%s"/>`, sp.font, sp.font)
	if bold {
		rPr.WriteString("<w:b/>")
	}
	if italic {
		rPr.WriteString("<w:i/>")
	}
	if colorHex != "" {
		fmt.Fprintf(&rPr, `<w:color w:val="%s"/>`, colorHex)
	}
	fmt.Fprintf(&rPr, `<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, halfPt, halfPt)
	rPr.WriteString("</w:rPr>")

	return fmt.Sprintf(`<w:p>%s<w:r>%s<w:t xml:space="preserve">%s</w:t></w:r></w:p>`, pPr.String(), rPr.String(), escapeXML(text))
}

func buildDocumentXML(markdown string, sp styleSpec) string {
	var body strings.Builder
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# "):
			text := unescapeMD(strings.TrimPrefix(line, "# "))
			body.WriteString(paragraph(text, sp, sp.nameHalfPt, true, false, sp.accentHex, 0, 60, false, 0))
		case strings.HasPrefix(line, "## "):
			text := strings.ToUpper(unescapeMD(strings.TrimPrefix(line, "## ")))
			body.WriteString(paragraph(text, sp, sp.h2HalfPt, true, false, sp.accentHex, 180, 60, true, 0))
		case strings.HasPrefix(line, "### "):
			text := unescapeMD(strings.TrimPrefix(line, "### "))
			body.WriteString(paragraph(text, sp, sp.bodyHalfPt+2, true, false, "111827", 110, 20, false, 0))
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			text := "• " + unescapeMD(line[2:])
			body.WriteString(paragraph(text, sp, sp.bodyHalfPt, false, false, "1F2937", 0, sp.afterTwips, false, 320))
		default:
			text := unescapeMD(line)
			isMeta := strings.Contains(text, " | ") || strings.Contains(text, " · ")
			color := "1F2937"
			if isMeta {
				color = "4B5563"
			}
			body.WriteString(paragraph(text, sp, sp.bodyHalfPt, false, isMeta, color, 0, sp.afterTwips, false, 0))
		}
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		body.String() +
		`<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="864" w:right="864" w:bottom="864" w:left="864"/></w:sectPr>` +
		`</w:body></w:document>`
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`</Types>`

const relsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`</Relationships>`

const docRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`
