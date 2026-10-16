// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docx

import (
	"fmt"
	"strings"
)

const xmlDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

// nsAttrs is the namespace set every WordprocessingML part in this
// package declares. Header and footer parts need the drawing namespaces
// too, because the McHarbor mark can be placed in the header.
const nsAttrs = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
	`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" ` +
	`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
	`xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"`

const docRoot = `<w:document ` + nsAttrs + `>`

const packageRels = xmlDecl +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
	`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>` +
	`</Relationships>`

// updateFields makes Word offer to build the table of contents when the
// document is first opened, which is the only way a TOC field can carry
// real page numbers without us paginating in Word's place.
func settingsXML(embedFonts bool) string {
	s := xmlDecl + `<w:settings ` + nsAttrs + `>` +
		`<w:zoom w:percent="100"/>` +
		`<w:updateFields w:val="true"/>` +
		`<w:defaultTabStop w:val="720"/>`
	if embedFonts {
		// saveSubsetFonts=false keeps the full face in the package, so
		// the recipient can edit the document and still get the brand
		// type for characters we never happened to emit.
		s += `<w:embedTrueTypeFonts/><w:saveSubsetFonts w:val="false"/>`
	}
	s += `<w:characterSpacingControl w:val="doNotCompress"/>` +
		`</w:settings>`
	return s
}

// fontTableOr supplies a minimal font table when nothing is embedded;
// the part is referenced from document.xml.rels either way.
func fontTableOr(table string) string {
	if table != "" {
		return table
	}
	return xmlDecl + `<w:fonts ` + nsAttrs + `>` +
		`<w:font w:name="Calibri"><w:panose1 w:val="020F0502020204030204"/><w:charset w:val="00"/><w:family w:val="swiss"/><w:pitch w:val="variable"/></w:font>` +
		`</w:fonts>`
}

// Brand palette, duplicated here as hex because OOXML has no notion of
// the OKLCH tokens the UI uses. Kept in sync with theme.go by hand — the
// values come from the logo the default theme was derived from.
const (
	cInk      = "22303F"
	cInkMuted = "5B6B7C"
	cBrand    = "276AA2"
	cTeal     = "26B0C6"
	cHairline = "DDE4EB"
)

// stylesXML defines the named styles the exporter uses. Naming them
// (rather than formatting runs directly) means a recipient can restyle
// the whole document from Word's style gallery, which is what makes an
// exported report usable as the basis for a board pack rather than a
// dead artefact.
var stylesXML = xmlDecl + `<w:styles ` + nsAttrs + `>` +
	`<w:docDefaults><w:rPrDefault><w:rPr>` +
	`<w:rFonts w:ascii="Inter Tight" w:hAnsi="Inter Tight" w:cs="Inter Tight"/>` +
	`<w:sz w:val="19"/><w:szCs w:val="19"/><w:color w:val="` + cInk + `"/>` +
	`</w:rPr></w:rPrDefault>` +
	`<w:pPrDefault><w:pPr><w:spacing w:after="120" w:line="276" w:lineRule="auto"/></w:pPr></w:pPrDefault>` +
	`</w:docDefaults>` +
	style("Normal", "Normal", "", true, `<w:spacing w:after="120" w:line="276" w:lineRule="auto"/>`, ``) +
	style("Title", "McHarbor Title", "Normal", false,
		`<w:spacing w:before="0" w:after="120"/>`,
		`<w:sz w:val="60"/><w:b/><w:color w:val="`+cInk+`"/>`) +
	style("Subtitle", "McHarbor Subtitle", "Normal", false,
		`<w:spacing w:before="0" w:after="360"/>`,
		`<w:sz w:val="24"/><w:color w:val="`+cBrand+`"/>`) +
	style("Heading1", "heading 1", "Normal", false,
		`<w:keepNext/><w:outlineLvl w:val="0"/><w:spacing w:before="360" w:after="140"/>`+
			`<w:pBdr><w:bottom w:val="single" w:sz="6" w:space="6" w:color="`+cHairline+`"/></w:pBdr>`,
		`<w:sz w:val="34"/><w:b/><w:color w:val="`+cInk+`"/>`) +
	style("Heading2", "heading 2", "Normal", false,
		`<w:keepNext/><w:outlineLvl w:val="1"/><w:spacing w:before="280" w:after="100"/>`,
		`<w:sz w:val="26"/><w:b/><w:color w:val="`+cInk+`"/>`) +
	style("Heading3", "heading 3", "Normal", false,
		`<w:keepNext/><w:outlineLvl w:val="2"/><w:spacing w:before="200" w:after="80"/>`,
		`<w:sz w:val="22"/><w:b/><w:color w:val="`+cBrand+`"/>`) +
	style("Eyebrow", "McHarbor Eyebrow", "Normal", false,
		`<w:spacing w:before="240" w:after="40"/>`,
		`<w:sz w:val="15"/><w:b/><w:caps/><w:spacing w:val="30"/><w:color w:val="`+cTeal+`"/>`) +
	style("Caption", "McHarbor Caption", "Normal", false,
		`<w:spacing w:before="40" w:after="200"/>`,
		`<w:sz w:val="15"/><w:color w:val="`+cInkMuted+`"/>`) +
	style("Lead", "McHarbor Lead", "Normal", false,
		`<w:spacing w:after="200"/>`,
		`<w:sz w:val="22"/><w:color w:val="`+cInk+`"/>`) +
	style("KpiValue", "McHarbor KPI Value", "Normal", false,
		`<w:spacing w:before="20" w:after="0"/>`,
		`<w:sz w:val="34"/><w:b/><w:color w:val="`+cBrand+`"/>`) +
	style("KpiLabel", "McHarbor KPI Label", "Normal", false,
		`<w:spacing w:before="0" w:after="40"/>`,
		`<w:sz w:val="14"/><w:caps/><w:spacing w:val="20"/><w:color w:val="`+cInkMuted+`"/>`) +
	style("Mono", "McHarbor Mono", "Normal", false, `<w:spacing w:after="80"/>`,
		`<w:rFonts w:ascii="JetBrains Mono" w:hAnsi="JetBrains Mono" w:cs="JetBrains Mono"/><w:sz w:val="16"/>`) +
	style("TableHeader", "McHarbor Table Header", "Normal", false,
		`<w:spacing w:before="30" w:after="30"/>`,
		`<w:sz w:val="15"/><w:b/><w:caps/><w:spacing w:val="16"/><w:color w:val="`+cInkMuted+`"/>`) +
	style("TableCell", "McHarbor Table Cell", "Normal", false,
		`<w:spacing w:before="30" w:after="30" w:line="240" w:lineRule="auto"/>`,
		`<w:sz w:val="17"/>`) +
	style("HeaderFooter", "McHarbor Header Footer", "Normal", false,
		`<w:spacing w:before="0" w:after="0"/>`,
		`<w:sz w:val="15"/><w:color w:val="`+cInkMuted+`"/>`) +
	style("ListParagraph", "List Paragraph", "Normal", false,
		`<w:spacing w:after="60"/><w:ind w:left="360"/><w:contextualSpacing/>`, ``) +
	style("TOCEntry", "toc 1", "Normal", false, `<w:spacing w:after="60"/>`, ``) +
	`</w:styles>`

func style(id, name, basedOn string, isDefault bool, pPr, rPr string) string {
	var b strings.Builder
	b.WriteString(`<w:style w:type="paragraph" w:styleId="` + id + `"`)
	if isDefault {
		b.WriteString(` w:default="1"`)
	}
	b.WriteString(`><w:name w:val="` + name + `"/>`)
	if basedOn != "" {
		b.WriteString(`<w:basedOn w:val="` + basedOn + `"/>`)
	}
	b.WriteString(`<w:qFormat/>`)
	if pPr != "" {
		b.WriteString(`<w:pPr>` + pPr + `</w:pPr>`)
	}
	if rPr != "" {
		b.WriteString(`<w:rPr>` + rPr + `</w:rPr>`)
	}
	b.WriteString(`</w:style>`)
	return b.String()
}

// numberingXML defines one bullet list (numId 1) and one ordered list
// (numId 2), each with three indent levels.
var numberingXML = xmlDecl + `<w:numbering ` + nsAttrs + `>` +
	abstractNum(0, []string{"bullet", "bullet", "bullet"}, []string{"", "o", "▪"}) +
	abstractNum(1, []string{"decimal", "lowerLetter", "lowerRoman"}, []string{"%1.", "%2.", "%3."}) +
	`<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>` +
	`<w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num>` +
	`</w:numbering>`

func abstractNum(id int, formats, texts []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<w:abstractNum w:abstractNumId="%d">`, id)
	for lvl := 0; lvl < len(formats); lvl++ {
		text := texts[lvl]
		if formats[lvl] == "bullet" && text == "" {
			text = "•"
		}
		fmt.Fprintf(&b,
			`<w:lvl w:ilvl="%d"><w:start w:val="1"/><w:numFmt w:val="%s"/><w:lvlText w:val="%s"/>`+
				`<w:lvlJc w:val="left"/><w:pPr><w:ind w:left="%d" w:hanging="260"/></w:pPr>`,
			lvl, formats[lvl], esc(text), 360+lvl*360)
		if formats[lvl] == "bullet" {
			b.WriteString(`<w:rPr><w:rFonts w:ascii="Segoe UI Symbol" w:hAnsi="Segoe UI Symbol" w:hint="default"/><w:color w:val="` + cTeal + `"/></w:rPr>`)
		}
		b.WriteString(`</w:lvl>`)
	}
	b.WriteString(`</w:abstractNum>`)
	return b.String()
}

// headerRow renders the running header: title on the left, environment
// on the right, a hairline beneath.
func headerRow(width float64, left, right string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<w:tbl><w:tblPr><w:tblW w:w="%d" w:type="dxa"/><w:tblLayout w:type="fixed"/>`, twips(width))
	b.WriteString(`<w:tblBorders><w:bottom w:val="single" w:sz="4" w:space="0" w:color="` + cHairline + `"/></w:tblBorders>`)
	b.WriteString(`<w:tblCellMar><w:left w:w="0" w:type="dxa"/><w:right w:w="0" w:type="dxa"/><w:top w:w="0" w:type="dxa"/><w:bottom w:w="60" w:type="dxa"/></w:tblCellMar>`)
	b.WriteString(`</w:tblPr><w:tblGrid>`)
	fmt.Fprintf(&b, `<w:gridCol w:w="%d"/><w:gridCol w:w="%d"/>`, twips(width*0.62), twips(width*0.38))
	b.WriteString(`</w:tblGrid><w:tr>`)
	fmt.Fprintf(&b, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr>%s</w:tc>`,
		twips(width*0.62), simplePara("HeaderFooter", Left, left))
	fmt.Fprintf(&b, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr>%s</w:tc>`,
		twips(width*0.38), simplePara("HeaderFooter", Right, right))
	b.WriteString(`</w:tr></w:tbl>`)
	// A zero-height paragraph after a table keeps Word from merging the
	// header table into the first body table.
	b.WriteString(`<w:p><w:pPr><w:spacing w:after="0" w:line="20" w:lineRule="exact"/></w:pPr></w:p>`)
	return b.String()
}

// footerRow renders the running footer: classification on the left,
// "Page X of Y" on the right, using real Word fields.
func footerRow(width float64, left string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<w:tbl><w:tblPr><w:tblW w:w="%d" w:type="dxa"/><w:tblLayout w:type="fixed"/>`, twips(width))
	b.WriteString(`<w:tblBorders><w:top w:val="single" w:sz="4" w:space="0" w:color="` + cHairline + `"/></w:tblBorders>`)
	b.WriteString(`<w:tblCellMar><w:left w:w="0" w:type="dxa"/><w:right w:w="0" w:type="dxa"/><w:top w:w="60" w:type="dxa"/><w:bottom w:w="0" w:type="dxa"/></w:tblCellMar>`)
	b.WriteString(`</w:tblPr><w:tblGrid>`)
	fmt.Fprintf(&b, `<w:gridCol w:w="%d"/><w:gridCol w:w="%d"/>`, twips(width*0.7), twips(width*0.3))
	b.WriteString(`</w:tblGrid><w:tr>`)
	fmt.Fprintf(&b, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr>%s</w:tc>`,
		twips(width*0.7), simplePara("HeaderFooter", Left, left))
	fmt.Fprintf(&b, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr>`, twips(width*0.3))
	b.WriteString(`<w:p><w:pPr><w:pStyle w:val="HeaderFooter"/><w:jc w:val="right"/></w:pPr>`)
	b.WriteString(field("PAGE"))
	b.WriteString(`<w:r><w:t xml:space="preserve"> / </w:t></w:r>`)
	b.WriteString(field("NUMPAGES"))
	b.WriteString(`</w:p></w:tc>`)
	b.WriteString(`</w:tr></w:tbl>`)
	b.WriteString(`<w:p><w:pPr><w:spacing w:after="0" w:line="20" w:lineRule="exact"/></w:pPr></w:p>`)
	return b.String()
}

func field(instr string) string {
	return `<w:r><w:fldChar w:fldCharType="begin"/></w:r>` +
		`<w:r><w:instrText xml:space="preserve"> ` + instr + ` </w:instrText></w:r>` +
		`<w:r><w:fldChar w:fldCharType="separate"/></w:r>` +
		`<w:r><w:t>1</w:t></w:r>` +
		`<w:r><w:fldChar w:fldCharType="end"/></w:r>`
}

func simplePara(style, align, text string) string {
	var b strings.Builder
	b.WriteString(`<w:p><w:pPr>`)
	if style != "" {
		b.WriteString(`<w:pStyle w:val="` + style + `"/>`)
	}
	if align != "" && align != Left {
		b.WriteString(`<w:jc w:val="` + align + `"/>`)
	}
	b.WriteString(`</w:pPr>`)
	if text != "" {
		b.WriteString(`<w:r><w:t xml:space="preserve">` + esc(text) + `</w:t></w:r>`)
	}
	b.WriteString(`</w:p>`)
	return b.String()
}

// esc escapes XML text content and drops the control characters OOXML
// forbids — a container log line can carry them and Word refuses to open
// a document that contains one.
func esc(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		case '\t', '\n':
			b.WriteRune(' ')
		default:
			if r < 0x20 || r == 0x7F || (r >= 0xFFFE && r <= 0xFFFF) {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}
