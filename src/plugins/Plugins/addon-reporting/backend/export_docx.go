// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"image/color"
	"image/png"
	"math"
	"strings"

	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-reporting/backend/charts"
	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-reporting/backend/docx"
	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-reporting/backend/fontkit"
)

// The Word exporter.
//
// The same document model as the PDF, emitted as WordprocessingML.
// Everything that can be native Word is native Word — headings carry
// outline levels so the TOC field works, tables are real tables with
// repeating header rows, and the brand fonts are embedded so the file
// looks the same on a machine that has never heard of Inter Tight.
//
// Charts are the one exception. Word has no vector primitive we can emit
// without embedding a spreadsheet part, so charts are rendered from the
// identical geometry the PDF draws and carried as pictures at 4×.

type docxRenderer struct {
	doc    *docx.Doc
	meta   Meta
	assets *assets
	fonts  charts.Fonts
}

func renderDOCX(d *Document, a *assets, ps pageSize) ([]byte, error) {
	doc := docx.New(ps.Width, ps.Height)
	doc.Title = d.Meta.Title
	doc.Author = d.Meta.Organisation
	doc.Subject = d.Meta.Environment + " · " + d.Meta.ReportType
	doc.Company = d.Meta.Organisation
	doc.Keywords = strings.Join([]string{d.Meta.ReportType, d.Meta.Environment, d.Meta.Classification}, ", ")
	doc.HeaderLeft = d.Meta.Title
	doc.HeaderRight = d.Meta.Environment + " · " + fmtPeriod(Period{
		Start: d.Meta.PeriodStart, End: d.Meta.PeriodEnd,
		PointInTime: d.Meta.PeriodStart.Equal(d.Meta.PeriodEnd),
	})
	doc.FooterLeft = d.Meta.Classification + " · " + d.Meta.Product + " " + d.Meta.Version

	doc.EmbedFont("Inter Tight", "020B0604020202020204", a.fontRegular, a.fontBold)
	doc.EmbedFont("JetBrains Mono", "020B0609020204030204", a.fontMono, nil)

	r := &docxRenderer{doc: doc, meta: d.Meta, assets: a}
	if a.faceRegular != nil {
		r.fonts = charts.Fonts{Regular: a.faceRegular, Bold: a.faceSemiBold}
	}

	r.cover(d)
	r.contents()
	for _, sec := range d.Sections {
		r.section(d, sec)
	}
	return doc.Bytes()
}

func (r *docxRenderer) cover(d *Document) {
	if img := r.assets.logoWordmarkImage(r.doc); img != nil {
		r.doc.Picture(img, 132, docx.Left)
		r.doc.Spacer(18)
	}
	r.doc.Para("Eyebrow", docx.Left, docx.T(strings.ToUpper(d.Meta.Organisation)))
	r.doc.Para("Title", docx.Left, docx.T(d.Meta.Title))
	if d.Meta.Subtitle != "" {
		r.doc.Para("Subtitle", docx.Left, docx.T(d.Meta.Subtitle))
	}

	period := fmtPeriod(Period{Start: d.Meta.PeriodStart, End: d.Meta.PeriodEnd,
		PointInTime: d.Meta.PeriodStart.Equal(d.Meta.PeriodEnd)})
	w := r.doc.ContentWidth()
	rows := []docx.Row{
		coverRow("Environment", d.Meta.Environment, "Period", period),
		coverRow("Prepared", fmtDateTime(d.Meta.GeneratedAt), "Prepared by", d.Meta.GeneratedBy),
		coverRow("Sections", levelsLabel(d.Meta.Levels), "Classification", d.Meta.Classification),
	}
	// A narrowed report says so on its cover; see the note in export_pdf.go.
	if d.Meta.SubjectLabel != "" {
		rows = append(rows, coverRow(
			"Scope", titleCase(string(d.Meta.Subject))+": "+d.Meta.SubjectLabel, "", ""))
	}
	r.doc.Table(docx.Table{
		Widths: []float64{w * 0.22, w * 0.28, w * 0.22, w * 0.28},
		Rows:   rows,
	})

	if d.Scorecard != nil && d.Scorecard.Overall >= 0 {
		r.doc.Spacer(10)
		r.scorecardTable(d.Scorecard)
	}
	r.doc.PageBreak()
}

func coverRow(k1, v1, k2, v2 string) docx.Row {
	return docx.Row{Cells: []docx.Cell{
		{Runs: []docx.Run{{Text: strings.ToUpper(k1), Size: 7, Color: hexMuted}}},
		{Runs: []docx.Run{{Text: v1, Size: 10, Color: hexInk}}},
		{Runs: []docx.Run{{Text: strings.ToUpper(k2), Size: 7, Color: hexMuted}}},
		{Runs: []docx.Run{{Text: v2, Size: 10, Color: hexInk}}},
	}}
}

func (r *docxRenderer) contents() {
	r.doc.Heading(1, "Contents")
	r.doc.TOC("Right-click here and choose “Update Field” to build the table of contents.")
	r.doc.PageBreak()
}

func (r *docxRenderer) section(d *Document, sec Section) {
	r.doc.Eyebrow(levelTitle(sec.Level))
	r.doc.Heading(1, sec.Title)
	if sec.Intent != "" {
		r.doc.Para("Caption", docx.Left, docx.T(sec.Intent))
	}
	for _, b := range sec.Blocks {
		r.block(d, b)
	}
	// Each altitude begins on a fresh page, matching the PDF, so a
	// section can be circulated on its own.
	if sec.Level != AnnexLevel {
		r.doc.PageBreak()
	}
}

func (r *docxRenderer) block(d *Document, b Block) {
	switch b.Kind {
	case BlockHeading:
		r.doc.Heading(b.Depth, b.Text)
	case BlockParagraph:
		style := "Normal"
		if b.Lead {
			style = "Lead"
		}
		r.doc.Para(style, docx.Left, docx.T(b.Text))
	case BlockList:
		for i, item := range b.Items {
			if b.Ordered {
				r.doc.Numbered(0, docx.T(item))
			} else {
				r.doc.Bullet(0, docx.T(item))
			}
			_ = i
		}
	case BlockKPIs:
		r.kpis(b.KPIs)
	case BlockTable:
		r.table(b)
	case BlockChart:
		r.chart(b)
	case BlockCallout:
		r.callout(b)
	case BlockKeyValue:
		r.keyValue(b)
	case BlockFindings:
		r.findings(b)
	case BlockScorecard:
		if d.Scorecard != nil {
			r.scorecardTable(d.Scorecard)
		}
	case BlockDivider:
		r.doc.Rule()
	case BlockPageBreak:
		r.doc.PageBreak()
	}
}

// kpis renders the headline numbers as a borderless shaded table — the
// closest Word equivalent of the PDF's cards, and one that survives
// being edited.
func (r *docxRenderer) kpis(items []KPI) {
	if len(items) == 0 {
		return
	}
	perRow := len(items)
	if perRow > 4 {
		perRow = (len(items) + 1) / 2
		if perRow > 4 {
			perRow = 4
		}
	}
	w := r.doc.ContentWidth()

	for start := 0; start < len(items); start += perRow {
		end := start + perRow
		if end > len(items) {
			end = len(items)
		}
		row := items[start:end]
		cw := w / float64(perRow)
		widths := make([]float64, perRow)
		for i := range widths {
			widths[i] = cw
		}

		label := make([]docx.Cell, 0, perRow)
		value := make([]docx.Cell, 0, perRow)
		sub := make([]docx.Cell, 0, perRow)
		for i := 0; i < perRow; i++ {
			if i >= len(row) {
				label = append(label, docx.Cell{})
				value = append(value, docx.Cell{})
				sub = append(sub, docx.Cell{})
				continue
			}
			k := row[i]
			shade := hexSurfaceAlt
			if k.Status != StatusNeutral {
				shade = statusTintHex(k.Status)
			}
			valColor := hexInkStrong
			if k.Status != StatusNeutral {
				valColor = statusHex(k.Status)
			}
			label = append(label, docx.Cell{Shade: shade,
				Runs: []docx.Run{{Text: strings.ToUpper(k.Label), Size: 7, Color: hexMuted, Bold: true}}})
			value = append(value, docx.Cell{Shade: shade,
				Runs: []docx.Run{{Text: k.Value, Size: 17, Bold: true, Color: valColor}}})
			sub = append(sub, docx.Cell{Shade: shade,
				Runs: []docx.Run{{Text: kpiSub(k), Size: 7, Color: hexMuted}}})
		}
		r.doc.Table(docx.Table{
			Widths: widths,
			Rows: []docx.Row{
				{Cells: label}, {Cells: value}, {Cells: sub},
			},
		})
	}
}

func kpiSub(k KPI) string {
	switch {
	case k.Target != "" && k.Hint != "":
		return "vs " + k.Target + " · " + k.Hint
	case k.Target != "":
		return "vs " + k.Target
	default:
		return k.Hint
	}
}

func (r *docxRenderer) table(b Block) {
	if len(b.Columns) == 0 {
		return
	}
	if b.Caption != "" {
		r.doc.Para("Eyebrow", docx.Left, docx.T(strings.ToUpper(b.Caption)))
	}
	widths := columnWidths(b.Columns, r.doc.ContentWidth())

	header := make([]docx.Cell, len(b.Columns))
	for i, c := range b.Columns {
		header[i] = docx.Cell{
			Runs:  []docx.Run{{Text: strings.ToUpper(c.Label)}},
			Align: docxAlign(c.Align),
			Shade: hexSurfaceAlt,
		}
	}
	rows := []docx.Row{{Cells: header, Header: true}}

	for ri, row := range b.Rows {
		cells := make([]docx.Cell, len(b.Columns))
		for ci, col := range b.Columns {
			if ci >= len(row) {
				cells[ci] = docx.Cell{Align: docxAlign(col.Align)}
				continue
			}
			c := row[ci]
			cell := docx.Cell{Align: docxAlign(col.Align)}
			run := docx.Run{Text: c.Text, Mono: c.Mono || col.Mono}
			switch {
			case c.Pill && c.Text != "":
				run.Bold = true
				run.Color = statusHex(c.Status)
				cell.Shade = statusTintHex(c.Status)
			case c.Status != StatusNeutral:
				run.Bold = true
				run.Color = statusHex(c.Status)
			}
			cell.Runs = []docx.Run{run}
			cells[ci] = cell
		}
		shade := ""
		if ri%2 == 1 {
			shade = hexSurfaceAlt
		}
		rows = append(rows, docx.Row{Cells: cells, Shade: shade})
	}

	r.doc.Table(docx.Table{Widths: widths, Rows: rows, Borders: true})
	if b.Note != "" {
		r.doc.Caption(b.Note)
	}
}

func docxAlign(a string) string {
	switch a {
	case "right":
		return docx.Right
	case "center":
		return docx.Center
	default:
		return docx.Left
	}
}

func (r *docxRenderer) callout(b Block) {
	shade := statusTintHex(b.Tone)
	accent := statusHex(b.Tone)
	if b.Tone == StatusNeutral {
		shade, accent = hexInfoTint, hexBrand
	}
	var runs []docx.Run
	if b.Title != "" {
		runs = append(runs, docx.Run{Text: b.Title, Bold: true, Color: accent})
		runs = append(runs, docx.Run{Text: b.Text, Break: true, Color: hexInk})
	} else {
		runs = append(runs, docx.Run{Text: b.Text, Color: hexInk})
	}
	r.doc.Table(docx.Table{
		Widths: []float64{r.doc.ContentWidth()},
		Rows:   []docx.Row{{Cells: []docx.Cell{{Runs: runs, Shade: shade}}}},
	})
}

func (r *docxRenderer) keyValue(b Block) {
	if b.Title != "" {
		r.doc.Heading(3, b.Title)
	}
	w := r.doc.ContentWidth()
	rows := make([]docx.Row, 0, len(b.Pairs))
	for i, p := range b.Pairs {
		shade := ""
		if i%2 == 1 {
			shade = hexSurfaceAlt
		}
		rows = append(rows, docx.Row{Shade: shade, Cells: []docx.Cell{
			{Runs: []docx.Run{{Text: p.K, Bold: true, Color: hexMuted}}},
			{Runs: []docx.Run{{Text: p.V, Mono: p.Mono}}},
		}})
	}
	r.doc.Table(docx.Table{Widths: []float64{w * 0.3, w * 0.7}, Rows: rows, Borders: true})
}

func (r *docxRenderer) findings(b Block) {
	w := r.doc.ContentWidth()
	for _, f := range b.Findings {
		st := severityStatus(f.Severity)
		var body []docx.Run
		body = append(body, docx.Run{Text: f.Title, Bold: true, Size: 10, Color: hexInkStrong})
		if f.Detail != "" {
			body = append(body, docx.Run{Text: f.Detail, Break: true, Size: 8.5, Color: hexMuted})
		}
		if f.Recommendation != "" {
			body = append(body, docx.Run{Text: "Recommended: ", Break: true, Bold: true, Size: 8.5, Color: hexInk})
			body = append(body, docx.Run{Text: f.Recommendation, Size: 8.5, Color: hexInk})
		}
		if f.Owner != "" {
			body = append(body, docx.Run{Text: "Owner: " + f.Owner, Break: true, Size: 7.5, Color: hexMuted})
		}
		r.doc.Table(docx.Table{
			Widths: []float64{w * 0.16, w * 0.84},
			Rows: []docx.Row{{Cells: []docx.Cell{
				{Shade: statusTintHex(st), Baseline: false,
					Runs: []docx.Run{{Text: strings.ToUpper(string(f.Severity)), Bold: true, Size: 7.5, Color: statusHex(st)}}},
				{Runs: body, Shade: hexSurfaceAlt},
			}}},
		})
	}
}

func (r *docxRenderer) scorecardTable(sc *Scorecard) {
	w := r.doc.ContentWidth()
	r.doc.Table(docx.Table{
		Widths: []float64{w * 0.34, w * 0.16, w * 0.5},
		Rows: []docx.Row{{Cells: []docx.Cell{
			{Shade: hexInfoTint, Runs: []docx.Run{
				{Text: "OVERALL POSTURE", Size: 7, Bold: true, Color: hexMuted},
				{Text: fmtInt(sc.Overall) + " / 100", Break: true, Size: 22, Bold: true, Color: hexBrand},
			}},
			{Shade: hexInfoTint, Runs: []docx.Run{
				{Text: bandLabel(sc.Band), Bold: true, Size: 11, Color: statusHex(bandStatus(sc.Band))},
			}},
			{Shade: hexInfoTint, Runs: []docx.Run{
				{Text: scorecardSummaryLine(sc), Size: 8, Color: hexInk},
			}},
		}}},
	})

	rows := make([]docx.Row, 0, len(sc.Domains)+1)
	rows = append(rows, docx.Row{Header: true, Cells: []docx.Cell{
		{Runs: []docx.Run{{Text: "DOMAIN"}}, Shade: hexSurfaceAlt},
		{Runs: []docx.Run{{Text: "SCORE"}}, Align: docx.Right, Shade: hexSurfaceAlt},
		{Runs: []docx.Run{{Text: "BAND"}}, Shade: hexSurfaceAlt},
		{Runs: []docx.Run{{Text: "PRINCIPAL DRIVER"}}, Shade: hexSurfaceAlt},
	}})
	for i, d := range sc.Domains {
		score := "—"
		if d.Score >= 0 {
			score = fmtInt(d.Score)
		}
		shade := ""
		if i%2 == 1 {
			shade = hexSurfaceAlt
		}
		rows = append(rows, docx.Row{Shade: shade, Cells: []docx.Cell{
			{Runs: []docx.Run{{Text: d.Label}}},
			{Runs: []docx.Run{{Text: score, Bold: true, Color: statusHex(bandStatus(d.Band))}}, Align: docx.Right},
			{Runs: []docx.Run{{Text: bandLabel(d.Band), Bold: true, Color: statusHex(bandStatus(d.Band))}},
				Shade: statusTintHex(bandStatus(d.Band))},
			{Runs: []docx.Run{{Text: d.Note}}},
		}})
	}
	r.doc.Table(docx.Table{
		Widths:  columnWidths([]Column{{Width: 1.5}, {Width: 0.8}, {Width: 1.2}, {Width: 4.5}}, w),
		Rows:    rows,
		Borders: true,
	})
}

func scorecardSummaryLine(sc *Scorecard) string {
	if w := worstDomain(sc); w != nil {
		return "Lowest-scoring domain: " + w.Label + " at " + fmtInt(w.Score) + ". " + w.Note + "."
	}
	return "All assessed domains are within their expected range."
}

// chart renders the chart geometry to a picture. 4× supersampling puts
// the effective resolution around 290 dpi at print size, which is where
// the difference stops being visible on paper.
func (r *docxRenderer) chart(b Block) {
	if b.Chart == nil || len(b.Chart.Series) == 0 || r.fonts.Regular == nil {
		return
	}
	if b.Chart.Title != "" {
		r.doc.Para("Eyebrow", docx.Left, docx.T(strings.ToUpper(b.Chart.Title)))
	}

	w := r.doc.ContentWidth()
	h := b.Chart.Height
	if h <= 0 {
		h = 132
	}
	h = math.Min(h, 300)

	spec := chartSpec(*b.Chart)
	layout := charts.Build(spec, w, h, r.fonts.Measurer(), chartPalette())

	const scale = 4.0
	img := charts.Render(layout, r.fonts, scale, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		// A chart that cannot be encoded is dropped rather than failing
		// the export; the surrounding tables still carry the numbers.
		return
	}
	handle := r.doc.AddImagePNG(buf.Bytes(), img.Bounds().Dx(), img.Bounds().Dy())
	r.doc.Picture(handle, w, docx.Left)

	if b.Chart.Caption != "" {
		r.doc.Caption(b.Chart.Caption)
	}
}

// assets carries the embedded fonts and logos, parsed once.
type assets struct {
	fontRegular  []byte
	fontSemiBold []byte
	fontBold     []byte
	fontMono     []byte

	faceRegular  *fontkit.Face
	faceSemiBold *fontkit.Face

	logoWordmark []byte
	logoMark     []byte

	// wordmarkW/H are the pixel dimensions, needed by the DOCX drawing.
	wordmarkW, wordmarkH int
}

func (a *assets) logoWordmarkImage(d *docx.Doc) *docx.Image {
	if len(a.logoWordmark) == 0 || a.wordmarkW == 0 {
		return nil
	}
	return d.AddImagePNG(a.logoWordmark, a.wordmarkW, a.wordmarkH)
}
