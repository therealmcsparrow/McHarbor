// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package docx writes WordprocessingML (.docx) documents with no
// external dependencies.
//
// Why hand-built: the maintained Go OOXML libraries are either
// commercially licensed or template-driven, and neither gives the control
// this needs — named styles that survive into Word's style gallery, table
// header rows that repeat across pages, a real TOC field, and header /
// footer parts carrying the McHarbor mark. The subset of OOXML a report
// needs is small, and writing it directly keeps the plugin's module
// dependency-free, which is what makes the plugin build reliably.
package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"
)

// Unit helpers. OOXML measures in twips (1/20 pt) for layout and EMUs
// (1/914400 inch) for drawings.
func twips(pt float64) int { return int(pt*20 + 0.5) }
func emu(pt float64) int   { return int(pt*12700 + 0.5) }

// halfPoints is the unit for <w:sz>.
func halfPoints(pt float64) int { return int(pt*2 + 0.5) }

// Run is a span of text with character formatting.
type Run struct {
	Text      string
	Bold      bool
	Italic    bool
	Mono      bool
	Size      float64 // points; 0 = inherit from the paragraph style
	Color     string  // "RRGGBB"; empty = inherit
	Highlight string  // "RRGGBB" shading behind the run
	Break     bool    // emit a line break before this run
	Tab       bool    // emit a tab before this run
}

// T is the common case: a plain run.
func T(s string) Run { return Run{Text: s} }

// B is a bold run.
func B(s string) Run { return Run{Text: s, Bold: true} }

// Align values for paragraphs and cells.
const (
	Left   = "left"
	Center = "center"
	Right  = "right"
)

// Cell is one table cell.
type Cell struct {
	Runs     []Run
	Align    string
	Shade    string // "RRGGBB" background
	Bold     bool   // convenience: bold every run
	Color    string // convenience: colour every run
	ColSpan  int    // 0 or 1 = no span
	NoWrap   bool
	VMerge   string // "restart" | "continue" | ""
	PadLeft  float64
	Baseline bool // vertical-align bottom rather than centre
}

// Row is one table row.
type Row struct {
	Cells  []Cell
	Header bool    // repeat on every page and keep with next
	Height float64 // minimum height in points; 0 = auto
	Shade  string  // row-level background, overridden per cell
}

// Table is a laid-out table. Widths are in points and are honoured
// exactly (fixed layout), because a report table that reflows differently
// in Word than in the PDF is a table nobody trusts.
type Table struct {
	Widths  []float64
	Rows    []Row
	Borders bool
	Grid    string // hairline colour "RRGGBB"
	Indent  float64
}

// Image is a registered picture part.
type Image struct {
	relID   string
	wPx     int
	hPx     int
	data    []byte
	name    string
	docPrID int
}

// Aspect is width/height.
func (i *Image) Aspect() float64 {
	if i == nil || i.hPx == 0 {
		return 1
	}
	return float64(i.wPx) / float64(i.hPx)
}

type relationship struct {
	id     string
	typ    string
	target string
}

// Doc is a document under construction.
type Doc struct {
	Title, Author, Subject, Keywords, Company string

	// PageWidth/Height/Margins are in points.
	PageWidth, PageHeight          float64
	MarginTop, MarginRight         float64
	MarginBottom, MarginLeft       float64
	HeaderDistance, FooterDistance float64

	// HeaderLeft/HeaderRight and FooterLeft are plain strings rendered
	// into the running header and footer parts.
	HeaderLeft, HeaderRight string
	FooterLeft              string
	HeaderMark              *Image

	body     bytes.Buffer
	rels     []relationship
	images   []*Image
	embedded []embeddedFont
	nextID   int
}

// New starts a portrait document of the given size, in points, with the
// report's margins: 1.27 cm — 36 pt — on all four sides. The running
// header and footer sit inside that margin, so the body still begins and
// ends exactly 36 pt from the sheet edge.
func New(pageWidth, pageHeight float64) *Doc {
	d := &Doc{
		PageWidth:      pageWidth,
		PageHeight:     pageHeight,
		MarginTop:      36,
		MarginRight:    36,
		MarginBottom:   36,
		MarginLeft:     36,
		HeaderDistance: 16,
		FooterDistance: 16,
		nextID:         1,
	}
	d.rels = []relationship{
		{"rId1", "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles", "styles.xml"},
		{"rId2", "http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering", "numbering.xml"},
		{"rId3", "http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings", "settings.xml"},
		{"rId4", "http://schemas.openxmlformats.org/officeDocument/2006/relationships/header", "header1.xml"},
		{"rId5", "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer", "footer1.xml"},
		{"rId6", "http://schemas.openxmlformats.org/officeDocument/2006/relationships/fontTable", "fontTable.xml"},
	}
	return d
}

// ContentWidth is the usable width in points.
func (d *Doc) ContentWidth() float64 {
	return d.PageWidth - d.MarginLeft - d.MarginRight
}

func (d *Doc) id() int {
	d.nextID++
	return d.nextID
}

// AddImagePNG registers a PNG and returns a handle usable with Picture.
func (d *Doc) AddImagePNG(data []byte, wPx, hPx int) *Image {
	img := &Image{
		relID:   fmt.Sprintf("rIdImg%d", len(d.images)+1),
		wPx:     wPx,
		hPx:     hPx,
		data:    data,
		name:    fmt.Sprintf("media/image%d.png", len(d.images)+1),
		docPrID: d.id(),
	}
	d.images = append(d.images, img)
	d.rels = append(d.rels, relationship{
		img.relID,
		"http://schemas.openxmlformats.org/officeDocument/2006/relationships/image",
		img.name,
	})
	return img
}

// Bytes serialises the .docx package.
func (d *Doc) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)

	write := func(name, content string) error {
		w, err := z.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}

	fontParts, fontTable, fontRels := d.buildFontParts()

	parts := []struct{ name, content string }{
		{"[Content_Types].xml", d.contentTypes(len(fontParts) > 0)},
		{"_rels/.rels", packageRels},
		{"docProps/core.xml", d.coreProps(time.Now())},
		{"docProps/app.xml", d.appProps()},
		{"word/document.xml", d.documentXML()},
		{"word/styles.xml", stylesXML},
		{"word/numbering.xml", numberingXML},
		{"word/fontTable.xml", fontTableOr(fontTable)},
		{"word/settings.xml", settingsXML(len(fontParts) > 0)},
		{"word/header1.xml", d.headerXML()},
		{"word/footer1.xml", d.footerXML()},
		{"word/_rels/document.xml.rels", d.documentRels()},
	}
	if fontRels != "" {
		parts = append(parts, struct{ name, content string }{"word/_rels/fontTable.xml.rels", fontRels})
	}
	for _, p := range parts {
		if err := write(p.name, p.content); err != nil {
			return nil, err
		}
	}
	for _, img := range d.images {
		w, err := z.Create("word/" + img.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(img.data); err != nil {
			return nil, err
		}
	}
	for _, fp := range fontParts {
		w, err := z.Create("word/" + fp.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(fp.data); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (d *Doc) contentTypes(hasFonts bool) string {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	b.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	b.WriteString(`<Default Extension="png" ContentType="image/png"/>`)
	if hasFonts {
		b.WriteString(`<Default Extension="odttf" ContentType="application/vnd.openxmlformats-officedocument.obfuscatedFont"/>`)
	}
	for _, o := range [][2]string{
		{"/word/document.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"},
		{"/word/styles.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"},
		{"/word/numbering.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"},
		{"/word/fontTable.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.fontTable+xml"},
		{"/word/settings.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"},
		{"/word/header1.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"},
		{"/word/footer1.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"},
		{"/docProps/core.xml", "application/vnd.openxmlformats-package.core-properties+xml"},
		{"/docProps/app.xml", "application/vnd.openxmlformats-officedocument.extended-properties+xml"},
	} {
		fmt.Fprintf(&b, `<Override PartName="%s" ContentType="%s"/>`, o[0], o[1])
	}
	b.WriteString(`</Types>`)
	return b.String()
}

func (d *Doc) documentRels() string {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, r := range d.rels {
		fmt.Fprintf(&b, `<Relationship Id="%s" Type="%s" Target="%s"/>`, r.id, r.typ, r.target)
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

func (d *Doc) coreProps(now time.Time) string {
	ts := now.UTC().Format("2006-01-02T15:04:05Z")
	return xmlDecl + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + esc(d.Title) + `</dc:title>` +
		`<dc:subject>` + esc(d.Subject) + `</dc:subject>` +
		`<dc:creator>` + esc(d.Author) + `</dc:creator>` +
		`<cp:keywords>` + esc(d.Keywords) + `</cp:keywords>` +
		`<cp:lastModifiedBy>McHarbor Reporting</cp:lastModifiedBy>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + ts + `</dcterms:created>` +
		`<dcterms:modified xsi:type="dcterms:W3CDTF">` + ts + `</dcterms:modified>` +
		`</cp:coreProperties>`
}

func (d *Doc) appProps() string {
	return xmlDecl + `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">` +
		`<Application>McHarbor Reporting</Application>` +
		`<Company>` + esc(d.Company) + `</Company>` +
		`</Properties>`
}

func (d *Doc) documentXML() string {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(docRoot)
	b.WriteString(`<w:body>`)
	b.WriteString(d.body.String())
	fmt.Fprintf(&b,
		`<w:sectPr><w:headerReference w:type="default" r:id="rId4"/><w:footerReference w:type="default" r:id="rId5"/>`+
			`<w:pgSz w:w="%d" w:h="%d"/>`+
			`<w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="%d" w:footer="%d" w:gutter="0"/>`+
			`<w:titlePg/><w:docGrid w:linePitch="360"/></w:sectPr>`,
		twips(d.PageWidth), twips(d.PageHeight),
		twips(d.MarginTop), twips(d.MarginRight), twips(d.MarginBottom), twips(d.MarginLeft),
		twips(d.HeaderDistance), twips(d.FooterDistance))
	b.WriteString(`</w:body></w:document>`)
	return b.String()
}

// The running header is a two-column borderless table so the right-hand
// label sits flush right without depending on tab stops a recipient
// might re-flow when they edit the document.
func (d *Doc) headerXML() string {
	return xmlDecl + `<w:hdr ` + nsAttrs + `>` +
		headerRow(d.ContentWidth(), d.HeaderLeft, d.HeaderRight) +
		`</w:hdr>`
}

func (d *Doc) footerXML() string {
	return xmlDecl + `<w:ftr ` + nsAttrs + `>` +
		footerRow(d.ContentWidth(), d.FooterLeft) +
		`</w:ftr>`
}
