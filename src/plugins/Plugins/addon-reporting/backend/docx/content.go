// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docx

import (
	"fmt"
	"strings"
)

// Para appends a paragraph in the named style.
func (d *Doc) Para(style, align string, runs ...Run) {
	d.body.WriteString(d.paraXML(style, align, 0, "", runs))
}

// Text is the common case: a body paragraph of plain text.
func (d *Doc) Text(s string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	d.Para("Normal", Left, T(s))
}

// Heading appends a heading. Levels are clamped to 1..3 because those
// are the levels the TOC field collects.
func (d *Doc) Heading(level int, text string) {
	if level < 1 {
		level = 1
	}
	if level > 3 {
		level = 3
	}
	d.Para(fmt.Sprintf("Heading%d", level), Left, T(text))
}

// Eyebrow is the small tracked label above a section.
func (d *Doc) Eyebrow(text string) { d.Para("Eyebrow", Left, T(text)) }

// Caption is the muted note under a table or chart.
func (d *Doc) Caption(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	d.Para("Caption", Left, T(text))
}

// Bullet appends a bulleted list item at the given indent level.
func (d *Doc) Bullet(level int, runs ...Run) {
	d.body.WriteString(d.paraXML("ListParagraph", Left, 1, listPr(1, level), runs))
}

// Numbered appends an ordered list item.
func (d *Doc) Numbered(level int, runs ...Run) {
	d.body.WriteString(d.paraXML("ListParagraph", Left, 2, listPr(2, level), runs))
}

func listPr(numID, level int) string {
	if level < 0 {
		level = 0
	}
	if level > 2 {
		level = 2
	}
	return fmt.Sprintf(`<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr>`, level, numID)
}

// Spacer adds vertical space without an empty styled paragraph, which
// would otherwise inherit the surrounding style's spacing too.
func (d *Doc) Spacer(pt float64) {
	fmt.Fprintf(&d.body,
		`<w:p><w:pPr><w:spacing w:before="0" w:after="0" w:line="%d" w:lineRule="exact"/></w:pPr></w:p>`,
		twips(pt))
}

// PageBreak starts a new page.
func (d *Doc) PageBreak() {
	d.body.WriteString(`<w:p><w:pPr><w:spacing w:after="0"/></w:pPr><w:r><w:br w:type="page"/></w:r></w:p>`)
}

// Rule draws a horizontal hairline.
func (d *Doc) Rule() {
	fmt.Fprintf(&d.body,
		`<w:p><w:pPr><w:spacing w:before="80" w:after="160"/><w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="%s"/></w:pBdr></w:pPr></w:p>`,
		cHairline)
}

// TOC inserts a table-of-contents field. Word populates it on open
// because settings.xml sets updateFields; the placeholder text is what a
// reader sees if they decline.
func (d *Doc) TOC(placeholder string) {
	var b strings.Builder
	b.WriteString(`<w:p><w:pPr><w:pStyle w:val="Normal"/></w:pPr>`)
	b.WriteString(`<w:r><w:fldChar w:fldCharType="begin" w:dirty="true"/></w:r>`)
	b.WriteString(`<w:r><w:instrText xml:space="preserve"> TOC \o &quot;1-3&quot; \h \z \u </w:instrText></w:r>`)
	b.WriteString(`<w:r><w:fldChar w:fldCharType="separate"/></w:r>`)
	b.WriteString(`<w:r><w:rPr><w:color w:val="` + cInkMuted + `"/></w:rPr><w:t xml:space="preserve">` + esc(placeholder) + `</w:t></w:r>`)
	b.WriteString(`<w:r><w:fldChar w:fldCharType="end"/></w:r>`)
	b.WriteString(`</w:p>`)
	d.body.WriteString(b.String())
}

// Picture places an image at the given width in points, preserving
// aspect ratio.
func (d *Doc) Picture(img *Image, widthPt float64, align string) {
	if img == nil || widthPt <= 0 {
		return
	}
	heightPt := widthPt / img.Aspect()
	var b strings.Builder
	b.WriteString(`<w:p><w:pPr><w:spacing w:before="0" w:after="120"/>`)
	if align != "" && align != Left {
		b.WriteString(`<w:jc w:val="` + align + `"/>`)
	}
	b.WriteString(`</w:pPr><w:r><w:drawing>`)
	fmt.Fprintf(&b, `<wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/>`,
		emu(widthPt), emu(heightPt))
	b.WriteString(`<wp:effectExtent l="0" t="0" r="0" b="0"/>`)
	fmt.Fprintf(&b, `<wp:docPr id="%d" name="Picture %d"/>`, img.docPrID, img.docPrID)
	b.WriteString(`<wp:cNvGraphicFramePr><a:graphicFrameLocks noChangeAspect="1"/></wp:cNvGraphicFramePr>`)
	b.WriteString(`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic>`)
	fmt.Fprintf(&b, `<pic:nvPicPr><pic:cNvPr id="%d" name="Picture %d"/><pic:cNvPicPr/></pic:nvPicPr>`,
		img.docPrID, img.docPrID)
	fmt.Fprintf(&b, `<pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`, img.relID)
	fmt.Fprintf(&b, `<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm>`,
		emu(widthPt), emu(heightPt))
	b.WriteString(`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`)
	b.WriteString(`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`)
	d.body.WriteString(b.String())
}

func (d *Doc) paraXML(style, align string, numID int, extraPr string, runs []Run) string {
	var b strings.Builder
	b.WriteString(`<w:p><w:pPr>`)
	if style != "" {
		b.WriteString(`<w:pStyle w:val="` + style + `"/>`)
	}
	b.WriteString(extraPr)
	if align != "" && align != Left {
		b.WriteString(`<w:jc w:val="` + align + `"/>`)
	}
	b.WriteString(`</w:pPr>`)
	for _, r := range runs {
		b.WriteString(runXML(r))
	}
	b.WriteString(`</w:p>`)
	_ = numID
	return b.String()
}

// zeroWidthSpace is a break opportunity Word honours and a reader never
// sees. It is how a fixed-layout column is made to fold a value that has
// no space in it. Written as an escape rather than the character itself,
// because an invisible literal in source is a trap for the next reader.
const zeroWidthSpace = "\u200b"

// breakLongTokens seeds break opportunities into runs of text that carry
// no whitespace — a digest, an image reference, a mount path, a URL.
//
// Word only breaks a line at a space, so inside a `tblLayout=fixed`
// column such a token draws straight through the cell border and over
// its neighbour. Marking the natural seams (and, failing those, a fixed
// stride) lets Word wrap the value inside its own column instead. Short
// tokens are left alone: a break opportunity in "running" buys nothing
// and would show up in a copy-paste.
func breakLongTokens(s string) string {
	const (
		threshold = 14 // runes; below this a token fits any real column
		stride    = 8  // forced break interval inside a seamless run
	)
	if s == "" {
		return s
	}
	var out strings.Builder
	out.Grow(len(s) + len(s)/4)

	flush := func(tok []rune) {
		if len(tok) < threshold {
			out.WriteString(string(tok))
			return
		}
		since := 0
		for _, r := range tok {
			out.WriteRune(r)
			since++
			switch r {
			case '/', '\\', '-', '_', '.', ':', ';', ',', '=', '?', '&', '@', '+', '|', '>':
				out.WriteString(zeroWidthSpace)
				since = 0
			default:
				if since >= stride {
					out.WriteString(zeroWidthSpace)
					since = 0
				}
			}
		}
	}

	var tok []rune
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			flush(tok)
			tok = tok[:0]
			out.WriteRune(r)
			continue
		}
		tok = append(tok, r)
	}
	flush(tok)
	return out.String()
}

func runXML(r Run) string {
	var b strings.Builder
	b.WriteString(`<w:r>`)
	rPr := runProps(r)
	if rPr != "" {
		b.WriteString(`<w:rPr>` + rPr + `</w:rPr>`)
	}
	if r.Break {
		b.WriteString(`<w:br/>`)
	}
	if r.Tab {
		b.WriteString(`<w:tab/>`)
	}
	if r.Text != "" {
		b.WriteString(`<w:t xml:space="preserve">` + esc(r.Text) + `</w:t>`)
	}
	b.WriteString(`</w:r>`)
	return b.String()
}

func runProps(r Run) string {
	var b strings.Builder
	if r.Mono {
		b.WriteString(`<w:rFonts w:ascii="JetBrains Mono" w:hAnsi="JetBrains Mono" w:cs="JetBrains Mono"/>`)
	}
	if r.Bold {
		b.WriteString(`<w:b/>`)
	}
	if r.Italic {
		b.WriteString(`<w:i/>`)
	}
	if r.Size > 0 {
		fmt.Fprintf(&b, `<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, halfPoints(r.Size), halfPoints(r.Size))
	}
	if r.Color != "" {
		b.WriteString(`<w:color w:val="` + r.Color + `"/>`)
	}
	if r.Highlight != "" {
		b.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="` + r.Highlight + `"/>`)
	}
	return b.String()
}

// Table appends a table. Widths are honoured exactly (fixed layout) so
// the Word output matches the PDF column by column.
func (d *Doc) Table(t Table) {
	if len(t.Rows) == 0 || len(t.Widths) == 0 {
		return
	}
	grid := t.Grid
	if grid == "" {
		grid = cHairline
	}
	total := 0.0
	for _, w := range t.Widths {
		total += w
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<w:tbl><w:tblPr><w:tblW w:w="%d" w:type="dxa"/><w:tblLayout w:type="fixed"/>`, twips(total))
	if t.Indent > 0 {
		fmt.Fprintf(&b, `<w:tblInd w:w="%d" w:type="dxa"/>`, twips(t.Indent))
	}
	if t.Borders {
		fmt.Fprintf(&b, `<w:tblBorders>`+
			`<w:top w:val="single" w:sz="4" w:space="0" w:color="%[1]s"/>`+
			`<w:bottom w:val="single" w:sz="4" w:space="0" w:color="%[1]s"/>`+
			`<w:insideH w:val="single" w:sz="4" w:space="0" w:color="%[1]s"/>`+
			`</w:tblBorders>`, grid)
	}
	b.WriteString(`<w:tblCellMar><w:top w:w="60" w:type="dxa"/><w:bottom w:w="60" w:type="dxa"/>` +
		`<w:left w:w="96" w:type="dxa"/><w:right w:w="96" w:type="dxa"/></w:tblCellMar>`)
	b.WriteString(`</w:tblPr><w:tblGrid>`)
	for _, w := range t.Widths {
		fmt.Fprintf(&b, `<w:gridCol w:w="%d"/>`, twips(w))
	}
	b.WriteString(`</w:tblGrid>`)

	for _, row := range t.Rows {
		b.WriteString(rowXML(row, t.Widths))
	}
	b.WriteString(`</w:tbl>`)
	// Word needs a paragraph between consecutive tables, and one after
	// the last table, or it merges them.
	b.WriteString(`<w:p><w:pPr><w:spacing w:before="0" w:after="0" w:line="60" w:lineRule="exact"/></w:pPr></w:p>`)
	d.body.WriteString(b.String())
}

func rowXML(row Row, widths []float64) string {
	var b strings.Builder
	b.WriteString(`<w:tr><w:trPr>`)
	if row.Header {
		// tblHeader repeats the row on every page; cantSplit stops a
		// single row breaking across the page boundary.
		b.WriteString(`<w:tblHeader/><w:cantSplit/>`)
	}
	if row.Height > 0 {
		fmt.Fprintf(&b, `<w:trHeight w:val="%d" w:hRule="atLeast"/>`, twips(row.Height))
	}
	b.WriteString(`</w:trPr>`)

	col := 0
	for _, c := range row.Cells {
		span := c.ColSpan
		if span < 1 {
			span = 1
		}
		w := 0.0
		for i := col; i < col+span && i < len(widths); i++ {
			w += widths[i]
		}
		col += span

		shade := c.Shade
		if shade == "" {
			shade = row.Shade
		}
		style := "TableCell"
		if row.Header {
			style = "TableHeader"
		}

		b.WriteString(`<w:tc><w:tcPr>`)
		fmt.Fprintf(&b, `<w:tcW w:w="%d" w:type="dxa"/>`, twips(w))
		if span > 1 {
			fmt.Fprintf(&b, `<w:gridSpan w:val="%d"/>`, span)
		}
		if c.VMerge != "" {
			if c.VMerge == "restart" {
				b.WriteString(`<w:vMerge w:val="restart"/>`)
			} else {
				b.WriteString(`<w:vMerge/>`)
			}
		}
		if shade != "" {
			b.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="` + shade + `"/>`)
		}
		if c.NoWrap {
			b.WriteString(`<w:noWrap/>`)
		}
		if c.Baseline {
			b.WriteString(`<w:vAlign w:val="bottom"/>`)
		} else {
			b.WriteString(`<w:vAlign w:val="center"/>`)
		}
		b.WriteString(`</w:tcPr>`)

		runs := c.Runs
		if len(runs) == 0 {
			runs = []Run{T("")}
		}
		// A cell that has asked not to wrap keeps its text intact; every
		// other cell gets break opportunities so a long value folds inside
		// its own column rather than running through the one beside it.
		if !c.NoWrap {
			wrapped := make([]Run, len(runs))
			for i, r := range runs {
				r.Text = breakLongTokens(r.Text)
				wrapped[i] = r
			}
			runs = wrapped
		}
		if c.Bold || c.Color != "" {
			decorated := make([]Run, len(runs))
			for i, r := range runs {
				if c.Bold {
					r.Bold = true
				}
				if c.Color != "" && r.Color == "" {
					r.Color = c.Color
				}
				decorated[i] = r
			}
			runs = decorated
		}

		b.WriteString(`<w:p><w:pPr><w:pStyle w:val="` + style + `"/>`)
		if c.Align != "" && c.Align != Left {
			b.WriteString(`<w:jc w:val="` + c.Align + `"/>`)
		}
		b.WriteString(`</w:pPr>`)
		for _, r := range runs {
			b.WriteString(runXML(r))
		}
		b.WriteString(`</w:p></w:tc>`)
	}
	b.WriteString(`</w:tr>`)
	return b.String()
}
