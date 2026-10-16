// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"math"
	"strings"

	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-reporting/backend/charts"
	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-reporting/backend/pdf"
)

// The PDF layout engine.
//
// A report is laid out twice. The first pass discovers which page each
// heading lands on; the second inserts a contents page and re-lays the
// document with the page numbers shifted to account for it. That is the
// only honest way to produce a contents list with real page numbers
// without a second rendering system.

// Table metrics.
//
// A cell wraps rather than truncates: a report table carries image refs,
// mount paths and finding titles, and half a digest tells the reader
// nothing. The caps below keep one long value from owning a page — and
// the row is clamped again to whatever an empty page can hold, so a table
// can never produce a row taller than the paper.
const (
	tableCellPadX     float64 = 5
	tableLineLead     float64 = 1.35
	maxTableCellLines         = 10
	maxTableHeadLines         = 3
)

type tocEntry struct {
	title string
	depth int
	page  int
}

type pdfRenderer struct {
	doc  *pdf.Doc
	page *pdf.Page

	reg, semi, bold, mono *pdf.Font
	logoWord, logoMark    *pdf.Image

	meta     Meta
	paper    pageSize
	contentW float64
	y        float64

	toc       []tocEntry
	tocOffset int
	// bookmarksOn is false during the measuring pass so outline entries
	// are only recorded once.
	bookmarksOn bool
}

// renderPDF turns a document into a PDF file.
func renderPDF(d *Document, a *assets, ps pageSize) ([]byte, error) {
	// Pass one: lay the body out with no contents page, to learn where
	// each heading falls.
	measure, err := newPDFRenderer(d.Meta, a, ps)
	if err != nil {
		return nil, err
	}
	measure.renderBody(d)
	entries := measure.toc

	// How long the contents list runs is measured rather than estimated.
	// It used to be a constant — "about 34 lines" — which was true of A4
	// and of nothing else; on Legal the same guess puts every page number
	// in the list one page short. The measuring renderer is about to be
	// thrown away, so the list is laid out into the end of it and the
	// pages it added are counted. No extra document, no extra font parse.
	before := measure.doc.PageCount()
	measure.renderContents(entries, 1)
	tocPages := measure.doc.PageCount() - before
	if tocPages < 1 {
		tocPages = 1
	}

	final, err := newPDFRenderer(d.Meta, a, ps)
	if err != nil {
		return nil, err
	}
	final.bookmarksOn = true
	final.tocOffset = tocPages
	final.renderCover(d)
	final.renderContents(entries, tocPages)
	final.renderBody(d)
	return final.doc.Bytes()
}

func newPDFRenderer(meta Meta, a *assets, ps pageSize) (*pdfRenderer, error) {
	doc := pdf.New(ps.Width, ps.Height)
	doc.Title = meta.Title
	doc.Author = meta.Organisation
	doc.Subject = meta.Environment + " · " + meta.ReportType
	doc.Keywords = strings.Join([]string{meta.ReportType, meta.Environment, meta.Classification}, ", ")

	r := &pdfRenderer{doc: doc, meta: meta, paper: ps, contentW: ps.Width - 2*pageMargin}

	var err error
	if r.reg, err = doc.AddFont("InterTight-Regular", a.fontRegular); err != nil {
		return nil, err
	}
	if r.semi, err = doc.AddFont("InterTight-SemiBold", a.fontSemiBold); err != nil {
		return nil, err
	}
	if r.bold, err = doc.AddFont("InterTight-Bold", a.fontBold); err != nil {
		return nil, err
	}
	if r.mono, err = doc.AddFont("JetBrainsMono-Regular", a.fontMono); err != nil {
		return nil, err
	}
	// A missing logo must not fail an export; the cover simply sets the
	// wordmark as type instead.
	r.logoWord, _ = doc.AddImagePNG(a.logoWordmark)
	r.logoMark, _ = doc.AddImagePNG(a.logoMark)
	return r, nil
}

// --- page furniture --------------------------------------------------

func (r *pdfRenderer) newPage() {
	r.page = r.doc.NewPage()
	r.y = pageMargin
	r.drawHeader()
	r.drawFooter()
}

func (r *pdfRenderer) drawHeader() {
	if r.page.Index() == 0 {
		return
	}
	x := float64(pageMargin)
	if r.logoMark != nil {
		size := 9.0
		r.page.DrawImage(r.logoMark, x, headerBaselineY-size+1, size, size)
		x += size + 6
	}
	r.page.Text(r.semi, sizeCaption, colMuted, x, headerBaselineY, r.meta.Title)
	right := r.meta.Environment
	if !r.meta.PeriodStart.IsZero() {
		right += " · " + fmtPeriod(Period{Start: r.meta.PeriodStart, End: r.meta.PeriodEnd,
			PointInTime: r.meta.PeriodStart.Equal(r.meta.PeriodEnd)})
	}
	r.page.TextAligned(r.reg, sizeCaption, colFaint, pageMargin, r.contentW, headerBaselineY, right, pdf.AlignRight)
	r.page.Line(pageMargin, headerBaselineY+headerRuleGap, r.doc.W-pageMargin,
		headerBaselineY+headerRuleGap, colHairline, 0.5)
}

func (r *pdfRenderer) drawFooter() {
	if r.page.Index() == 0 {
		return
	}
	r.page.Line(pageMargin, r.bottom()+footerRuleGap, r.doc.W-pageMargin,
		r.bottom()+footerRuleGap, colHairline, 0.5)
	r.page.Text(r.reg, sizeCaption, colFaint, pageMargin, r.footerY(),
		r.meta.Classification+" · "+r.meta.Product)
	// The page number is stamped at the end, once the total is known.
}

// stampPageNumbers writes "n / total" into every page's footer. Called
// after the body is complete, because a running total cannot be known
// while pages are still being added.
func (r *pdfRenderer) stampPageNumbers() {
	total := r.doc.PageCount()
	for i := 1; i < total; i++ {
		p := r.doc.Page(i)
		if p == nil {
			continue
		}
		label := fmtInt(i) + " / " + fmtInt(total-1)
		p.TextAligned(r.reg, sizeCaption, colFaint, pageMargin, r.contentW, r.footerY(), label, pdf.AlignRight)
	}
}

// bottom is the last usable y for body content: the sheet edge less the
// 1.27 cm margin. The footer rule and label sit below it, inside the
// margin, the way a Word footer sits inside `w:pgMar`.
func (r *pdfRenderer) bottom() float64 { return r.doc.H - pageMargin }

// footerY is the baseline of the running footer.
func (r *pdfRenderer) footerY() float64 { return r.bottom() + footerBaselineGap }

// bodyHeight is the tallest a single block can be and still fit a page.
func (r *pdfRenderer) bodyHeight() float64 { return r.bottom() - pageMargin }

// ensure guarantees `h` points of room, starting a page if not.
func (r *pdfRenderer) ensure(h float64) {
	if r.page == nil {
		r.newPage()
		return
	}
	if r.y+h > r.bottom() {
		r.newPage()
	}
}

func (r *pdfRenderer) space(h float64) { r.y += h }

// --- cover -----------------------------------------------------------

func (r *pdfRenderer) renderCover(d *Document) {
	r.page = r.doc.NewPage()
	r.y = 0

	// A teal rule across the head of the cover, echoing the app's accent.
	r.page.Rect(0, 0, r.doc.W, 6, &colTeal, nil, 0)

	y := 96.0
	if r.logoWord != nil {
		w := 132.0
		r.page.DrawImage(r.logoWord, pageMargin, y, w, w/r.logoWord.Aspect())
		y += w/r.logoWord.Aspect() + 54
	} else {
		r.page.Text(r.bold, 26, colBrand, pageMargin, y+20, "McHarbor")
		y += 62
	}

	r.page.TextTracked(r.semi, sizeEyebrow, colTeal, pageMargin, y, 1.1,
		strings.ToUpper(r.meta.Organisation))
	y += 26

	for _, line := range r.reg.Wrap(d.Meta.Title, sizeCoverTitle, r.contentW) {
		r.page.Text(r.bold, sizeCoverTitle, colInkStrong, pageMargin, y+sizeCoverTitle*0.75, line)
		y += sizeCoverTitle * 1.16
	}
	y += 10

	if d.Meta.Subtitle != "" {
		for _, line := range r.reg.Wrap(d.Meta.Subtitle, sizeCoverSub, r.contentW*0.8) {
			r.page.Text(r.reg, sizeCoverSub, colMuted, pageMargin, y+sizeCoverSub*0.8, line)
			y += sizeCoverSub * 1.4
		}
	}

	y += 30
	r.page.Line(pageMargin, y, pageMargin+150, y, colTeal, 2)
	y += 26

	// The cover facts, as a two-column grid — the questions a reader
	// asks before reading anything: about what, when, and who says so.
	facts := [][2]string{
		{"Environment", d.Meta.Environment},
		{"Period", fmtPeriod(Period{Start: d.Meta.PeriodStart, End: d.Meta.PeriodEnd,
			PointInTime: d.Meta.PeriodStart.Equal(d.Meta.PeriodEnd)})},
		{"Prepared", fmtDateTime(d.Meta.GeneratedAt)},
		{"Prepared by", d.Meta.GeneratedBy},
		{"Sections", levelsLabel(d.Meta.Levels)},
		{"Classification", d.Meta.Classification},
	}
	// A narrowed report says so on its cover. Every figure inside it is a
	// subset, and a reader who does not know that reads them as estate
	// figures — so the scope is a cover fact, not a footnote.
	if d.Meta.SubjectLabel != "" {
		facts = append(facts, [2]string{
			"Scope", titleCase(string(d.Meta.Subject)) + ": " + d.Meta.SubjectLabel,
		})
	}
	colW := r.contentW / 2
	for i, f := range facts {
		cx := pageMargin + colW*float64(i%2)
		cy := y + 40*float64(i/2)
		r.page.TextTracked(r.semi, sizeMicro, colFaint, cx, cy, 0.8, strings.ToUpper(f[0]))
		r.page.Text(r.reg, 10.5, colInk, cx, cy+15,
			r.reg.Truncate(f[1], 10.5, colW-16))
	}
	y += 40*float64((len(facts)+1)/2) + 10

	if d.Scorecard != nil && d.Scorecard.Overall >= 0 {
		r.renderCoverScore(d.Scorecard, y)
	}

	// Confidentiality notice, pinned to the foot of the cover.
	notice := d.Meta.Classification + " — prepared for internal use. " +
		"Figures reflect the platform's recorded state at the time of generation."
	ny := r.bottom() - 58
	r.page.Line(pageMargin, ny-14, r.doc.W-pageMargin, ny-14, colHairline, 0.5)
	for _, line := range r.reg.Wrap(notice, sizeCaption, r.contentW) {
		r.page.Text(r.reg, sizeCaption, colFaint, pageMargin, ny, line)
		ny += sizeCaption * 1.4
	}
}

func (r *pdfRenderer) renderCoverScore(sc *Scorecard, y float64) {
	h := 92.0
	tint := pdf.Tint(colBrand, 0.06)
	r.page.RoundRect(pageMargin, y, r.contentW, h, 10, &tint, &colHairline, 0.6)

	r.page.TextTracked(r.semi, sizeMicro, colFaint, pageMargin+22, y+26, 0.8, "OVERALL POSTURE")
	r.page.Text(r.bold, 40, colBrand, pageMargin+22, y+68, fmtInt(sc.Overall))
	w := r.bold.Width(fmtInt(sc.Overall), 40)
	r.page.Text(r.reg, 12, colMuted, pageMargin+26+w, y+68, "/ 100")

	badge := bandLabel(sc.Band)
	bs := statusColor(bandStatus(sc.Band))
	bw := r.semi.Width(badge, sizeSmall) + 18
	bx := pageMargin + 150
	bg := pdf.Tint(bs, 0.14)
	r.page.RoundRect(bx, y+38, bw, 20, 10, &bg, nil, 0)
	r.page.TextAligned(r.semi, sizeSmall, bs, bx, bw, y+52, badge, pdf.AlignCenter)

	// Domain scores as compact bars down the right-hand side.
	gx := pageMargin + 250
	gw := r.contentW - 250 - 22
	rows := 0
	for _, d := range sc.Domains {
		if d.Score < 0 {
			continue
		}
		rows++
	}
	if rows == 0 {
		return
	}
	rowH := math.Min(13, (h-24)/float64(rows))
	i := 0
	for _, d := range sc.Domains {
		if d.Score < 0 {
			continue
		}
		cy := y + 16 + rowH*float64(i)
		r.page.Text(r.reg, sizeMicro, colMuted, gx, cy+6, d.Label)
		barX := gx + 62
		barW := gw - 62 - 20
		r.page.RoundRect(barX, cy+1.5, barW, 5, 2.5, &colTrack, nil, 0)
		fill := statusColor(bandStatus(d.Band))
		r.page.RoundRect(barX, cy+1.5, barW*float64(d.Score)/100, 5, 2.5, &fill, nil, 0)
		r.page.TextAligned(r.semi, sizeMicro, colInk, barX+barW+2, 18, cy+6, fmtInt(d.Score), pdf.AlignRight)
		i++
	}
}

func levelsLabel(levels []Level) string {
	if len(levels) == 0 {
		return "—"
	}
	names := make([]string, 0, len(levels))
	for _, l := range levels {
		names = append(names, levelTitle(l))
	}
	return strings.Join(names, " · ")
}

// --- contents --------------------------------------------------------

func (r *pdfRenderer) renderContents(entries []tocEntry, pages int) {
	if len(entries) == 0 {
		return
	}
	r.newPage()
	r.headingText(1, "Contents", false)

	for _, e := range entries {
		r.ensure(16)
		indent := float64((e.depth - 1) * 14)
		font, size, col := r.semi, sizeBody, colInk
		if e.depth > 1 {
			font, size, col = r.reg, sizeSmall, colMuted
		}
		baseline := r.y + size
		label := font.Truncate(e.title, size, r.contentW-indent-40)
		r.page.Text(font, size, col, pageMargin+indent, baseline, label)

		// The measuring pass laid the body out with no cover and no
		// contents, so a heading it recorded at index N sits at
		// (1 cover + `pages` contents + N) in the final document — and
		// the footer stamps a page's label as its own index. The two
		// must agree, or the contents list points a page short.
		num := fmtInt(e.page + pages + 1)
		numW := r.reg.Width(num, size)
		lw := font.Width(label, size)
		r.dotsLeader(pageMargin+indent+lw+6, r.doc.W-pageMargin-numW-6, baseline)
		r.page.TextAligned(r.reg, size, colMuted, pageMargin, r.contentW, baseline, num, pdf.AlignRight)
		r.y += size * 1.85
	}
}

// dotsLeader draws the run of dots between a contents entry and its page
// number. Drawn as evenly spaced marks rather than repeated glyphs so
// the rhythm does not drift with the entry's width.
func (r *pdfRenderer) dotsLeader(x1, x2, y float64) {
	if x2-x1 < 8 {
		return
	}
	const step = 4.0
	for x := x1; x < x2; x += step {
		r.page.Circle(x, y-2, 0.45, &colHairline, nil, 0)
	}
}

// --- body ------------------------------------------------------------

func (r *pdfRenderer) renderBody(d *Document) {
	for _, sec := range d.Sections {
		r.renderSection(d, sec)
	}
	r.stampPageNumbers()
}

func (r *pdfRenderer) renderSection(d *Document, sec Section) {
	// Each altitude starts on its own page: the split between strategic,
	// tactical and operational is the document's spine, and a reader
	// should be able to hand one section to someone without the rest.
	r.newPage()

	r.page.TextTracked(r.semi, sizeEyebrow, colTeal, pageMargin, r.y+sizeEyebrow, 1.2,
		strings.ToUpper(levelTitle(sec.Level)))
	r.y += sizeEyebrow + 12

	r.headingText(1, sec.Title, true)
	if sec.Intent != "" {
		r.y += 2
		r.paragraph(sec.Intent, sizeSmall, colMuted, false)
		r.space(8)
	}

	for _, b := range sec.Blocks {
		r.renderBlock(d, b)
	}
}

func (r *pdfRenderer) renderBlock(d *Document, b Block) {
	switch b.Kind {
	case BlockHeading:
		r.headingText(b.Depth, b.Text, true)
	case BlockParagraph:
		size, col := sizeBody, colInk
		if b.Lead {
			size = sizeLead
		}
		r.paragraph(b.Text, size, col, b.Lead)
		r.space(6)
	case BlockList:
		r.list(b)
	case BlockKPIs:
		r.kpiRow(b.KPIs)
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
			r.scorecard(d.Scorecard)
		}
	case BlockDivider:
		r.ensure(18)
		r.page.Line(pageMargin, r.y+6, r.doc.W-pageMargin, r.y+6, colHairline, 0.5)
		r.space(16)
	case BlockPageBreak:
		r.newPage()
	}
}

func (r *pdfRenderer) headingText(depth int, text string, record bool) {
	if text == "" {
		return
	}
	var size float64
	var font *pdf.Font
	switch depth {
	case 1:
		size, font = sizeH1, r.bold
	case 2:
		size, font = sizeH2, r.bold
	default:
		size, font = sizeH3, r.semi
	}
	r.ensure(size*2.4 + 24)
	if depth > 1 {
		r.space(10)
	}
	col := colInkStrong
	if depth >= 3 {
		col = colBrand
	}
	for _, line := range font.Wrap(text, size, r.contentW) {
		r.page.Text(font, size, col, pageMargin, r.y+size*0.82, line)
		r.y += size * 1.22
	}
	if depth == 1 {
		r.y += 3
		r.page.Line(pageMargin, r.y, r.doc.W-pageMargin, r.y, colHairline, 0.6)
		r.y += 9
	} else {
		r.y += 5
	}
	if record {
		r.toc = append(r.toc, tocEntry{title: text, depth: depth, page: r.page.Index()})
		if r.bookmarksOn && depth <= 2 {
			r.doc.AddBookmark(text, depth-1, r.page.Index(), r.y)
		}
	}
}

func (r *pdfRenderer) paragraph(text string, size float64, col pdf.Color, lead bool) {
	if strings.TrimSpace(text) == "" {
		return
	}
	font := r.reg
	leading := size * leadBody
	for _, line := range font.Wrap(text, size, r.contentW) {
		r.ensure(leading + 2)
		r.page.Text(font, size, col, pageMargin, r.y+font.Ascent(size), line)
		r.y += leading
	}
	_ = lead
}

func (r *pdfRenderer) list(b Block) {
	for i, item := range b.Items {
		marker := "•"
		if b.Ordered {
			marker = fmtInt(i+1) + "."
		}
		markerW := 16.0
		lines := r.reg.Wrap(item, sizeBody, r.contentW-markerW-6)
		r.ensure(float64(len(lines))*sizeBody*leadBody + 4)
		first := true
		for _, line := range lines {
			base := r.y + r.reg.Ascent(sizeBody)
			if first {
				mf, mc := r.semi, colTeal
				if b.Ordered {
					mc = colBrand
				}
				r.page.Text(mf, sizeBody, mc, pageMargin, base, marker)
				first = false
			}
			r.page.Text(r.reg, sizeBody, colInk, pageMargin+markerW, base, line)
			r.y += sizeBody * leadBody
		}
		r.y += 3
	}
	r.space(6)
}

// kpiRow lays KPI cards out in rows of up to four. Four is the practical
// limit at A4 width before the value type has to shrink past the point
// where it reads as a headline.
func (r *pdfRenderer) kpiRow(items []KPI) {
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
	gap := 10.0
	cardH := 62.0

	for start := 0; start < len(items); start += perRow {
		end := start + perRow
		if end > len(items) {
			end = len(items)
		}
		row := items[start:end]
		w := (r.contentW - gap*float64(perRow-1)) / float64(perRow)
		r.ensure(cardH + 12)
		for i, k := range row {
			x := pageMargin + (w+gap)*float64(i)
			r.kpiCard(k, x, r.y, w, cardH)
		}
		r.y += cardH + 10
	}
	r.space(4)
}

func (r *pdfRenderer) kpiCard(k KPI, x, y, w, h float64) {
	accent := statusColor(k.Status)
	bg := colSurfaceAlt
	if k.Status != StatusNeutral {
		bg = pdf.Tint(accent, 0.08)
	}
	r.page.RoundRect(x, y, w, h, 7, &bg, &colHairline, 0.5)
	// A status stripe down the leading edge carries the judgement even
	// in greyscale.
	if k.Status != StatusNeutral {
		r.page.RoundRect(x, y+8, 2.5, h-16, 1.25, &accent, nil, 0)
	}

	px := x + 12
	pw := w - 20
	r.page.TextTracked(r.semi, sizeKPILabel, colFaint, px, y+16, 0.7,
		strings.ToUpper(r.semi.Truncate(k.Label, sizeKPILabel, pw)))

	valCol := colInkStrong
	if k.Status != StatusNeutral {
		valCol = accent
	}
	size := sizeKPIValue
	for size > 11 && r.bold.Width(k.Value, size) > pw {
		size -= 1
	}
	r.page.Text(r.bold, size, valCol, px, y+16+size*1.1, k.Value)

	sub := k.Target
	if k.Hint != "" {
		sub = k.Hint
	}
	if k.Target != "" && k.Hint != "" {
		sub = "vs " + k.Target + " · " + k.Hint
	} else if k.Target != "" {
		sub = "vs " + k.Target
	}
	if sub != "" {
		r.page.Text(r.reg, sizeMicro, colMuted, px, y+h-9, r.reg.Truncate(sub, sizeMicro, pw))
	}
}

func (r *pdfRenderer) callout(b Block) {
	accent := statusColor(b.Tone)
	if b.Tone == StatusNeutral {
		accent = colBrand
	}
	bg := pdf.Tint(accent, 0.07)

	titleH := 0.0
	if b.Title != "" {
		titleH = sizeSmall * 1.5
	}
	lines := r.reg.Wrap(b.Text, sizeSmall, r.contentW-38)
	h := 18 + titleH + float64(len(lines))*sizeSmall*leadBody + 6

	r.ensure(h + 10)
	r.page.RoundRect(pageMargin, r.y, r.contentW, h, 7, &bg, nil, 0)
	r.page.RoundRect(pageMargin, r.y+7, 3, h-14, 1.5, &accent, nil, 0)

	ty := r.y + 16
	if b.Title != "" {
		r.page.Text(r.semi, sizeSmall, accent, pageMargin+18, ty, b.Title)
		ty += sizeSmall * 1.5
	}
	for _, line := range lines {
		r.page.Text(r.reg, sizeSmall, colInk, pageMargin+18, ty, line)
		ty += sizeSmall * leadBody
	}
	r.y += h + 10
}

func (r *pdfRenderer) keyValue(b Block) {
	if b.Title != "" {
		r.headingText(3, b.Title, false)
	}
	keyW := r.contentW * 0.3
	for _, p := range b.Pairs {
		font := r.reg
		if p.Mono {
			font = r.mono
		}
		size := sizeSmall
		lines := font.Wrap(p.V, size, r.contentW-keyW-10)
		h := math.Max(float64(len(lines))*size*leadTight, size*leadTight) + 6
		r.ensure(h)
		r.page.Text(r.semi, size, colMuted, pageMargin, r.y+size, r.semi.Truncate(p.K, size, keyW-8))
		ty := r.y + size
		for _, line := range lines {
			r.page.Text(font, size, colInk, pageMargin+keyW, ty, line)
			ty += size * leadTight
		}
		r.y += h
		r.page.Line(pageMargin, r.y-3, r.doc.W-pageMargin, r.y-3, colHairline, 0.4)
	}
	r.space(10)
}

func (r *pdfRenderer) findings(b Block) {
	for _, f := range b.Findings {
		st := severityStatus(f.Severity)
		accent := statusColor(st)

		titleLines := r.semi.Wrap(f.Title, sizeBody, r.contentW-96)
		detailLines := r.reg.Wrap(f.Detail, sizeSmall, r.contentW-24)
		recLines := r.reg.Wrap("Recommended: "+f.Recommendation, sizeSmall, r.contentW-24)

		h := 16 + float64(len(titleLines))*sizeBody*1.3
		if f.Detail != "" {
			h += float64(len(detailLines))*sizeSmall*leadTight + 4
		}
		if f.Recommendation != "" {
			h += float64(len(recLines))*sizeSmall*leadTight + 4
		}
		h += 12

		r.ensure(h + 8)
		r.page.RoundRect(pageMargin, r.y, r.contentW, h, 6, &colSurfaceAlt, &colHairline, 0.5)
		r.page.RoundRect(pageMargin, r.y+6, 3, h-12, 1.5, &accent, nil, 0)

		// Severity chip, right-aligned so titles of any length line up.
		chip := strings.ToUpper(string(f.Severity))
		cw := r.semi.Width(chip, sizeMicro) + 14
		cbg := pdf.Tint(accent, 0.16)
		r.page.RoundRect(r.doc.W-pageMargin-cw-12, r.y+11, cw, 13, 6.5, &cbg, nil, 0)
		r.page.TextAligned(r.semi, sizeMicro, accent, r.doc.W-pageMargin-cw-12, cw, r.y+20, chip, pdf.AlignCenter)

		ty := r.y + 12 + sizeBody*0.85
		for _, line := range titleLines {
			r.page.Text(r.semi, sizeBody, colInkStrong, pageMargin+14, ty, line)
			ty += sizeBody * 1.3
		}
		if f.Detail != "" {
			ty += 2
			for _, line := range detailLines {
				r.page.Text(r.reg, sizeSmall, colMuted, pageMargin+14, ty, line)
				ty += sizeSmall * leadTight
			}
		}
		if f.Recommendation != "" {
			ty += 4
			for i, line := range recLines {
				font := r.reg
				if i == 0 {
					font = r.semi
				}
				r.page.Text(font, sizeSmall, colInk, pageMargin+14, ty, line)
				ty += sizeSmall * leadTight
			}
		}
		if f.Owner != "" {
			r.page.TextAligned(r.reg, sizeMicro, colFaint, pageMargin, r.contentW-14,
				r.y+h-8, "Owner: "+f.Owner, pdf.AlignRight)
		}
		r.y += h + 8
	}
	r.space(2)
}

func (r *pdfRenderer) scorecard(sc *Scorecard) {
	if sc.Overall < 0 {
		return
	}
	h := 78.0
	r.ensure(h + 12)
	bg := pdf.Tint(colBrand, 0.05)
	r.page.RoundRect(pageMargin, r.y, r.contentW, h, 8, &bg, &colHairline, 0.5)

	r.page.TextTracked(r.semi, sizeMicro, colFaint, pageMargin+18, r.y+20, 0.8, "OVERALL POSTURE")
	r.page.Text(r.bold, 30, colBrand, pageMargin+18, r.y+54, fmtInt(sc.Overall))
	vw := r.bold.Width(fmtInt(sc.Overall), 30)
	r.page.Text(r.reg, 10, colMuted, pageMargin+22+vw, r.y+54, "/ 100")

	badge := bandLabel(sc.Band)
	bs := statusColor(bandStatus(sc.Band))
	bw := r.semi.Width(badge, sizeSmall) + 18
	bbg := pdf.Tint(bs, 0.14)
	r.page.RoundRect(pageMargin+18, r.y+58, bw, 15, 7.5, &bbg, nil, 0)
	r.page.TextAligned(r.semi, sizeSmall, bs, pageMargin+18, bw, r.y+69, badge, pdf.AlignCenter)

	gx := pageMargin + 140
	gw := r.contentW - 140 - 18
	n := 0
	for _, d := range sc.Domains {
		if d.Score >= 0 {
			n++
		}
	}
	if n > 0 {
		rowH := (h - 18) / float64(n)
		i := 0
		for _, d := range sc.Domains {
			if d.Score < 0 {
				continue
			}
			cy := r.y + 12 + rowH*float64(i)
			r.page.Text(r.reg, sizeMicro, colMuted, gx, cy+6, d.Label)
			barX := gx + 66
			barW := gw - 66 - 22
			r.page.RoundRect(barX, cy+2, barW, 5, 2.5, &colTrack, nil, 0)
			fill := statusColor(bandStatus(d.Band))
			r.page.RoundRect(barX, cy+2, barW*float64(d.Score)/100, 5, 2.5, &fill, nil, 0)
			r.page.TextAligned(r.semi, sizeMicro, colInk, barX+barW+2, 20, cy+6, fmtInt(d.Score), pdf.AlignRight)
			i++
		}
	}
	r.y += h + 12
}

// --- tables ----------------------------------------------------------

func (r *pdfRenderer) table(b Block) {
	if len(b.Columns) == 0 {
		return
	}
	widths := columnWidths(b.Columns, r.contentW)

	if b.Caption != "" {
		r.ensure(20)
		r.page.TextTracked(r.semi, sizeMicro, colFaint, pageMargin, r.y+sizeMicro, 0.8,
			strings.ToUpper(b.Caption))
		r.y += sizeMicro + 8
	}

	// Header labels wrap too. A narrow column with a two-word heading used
	// to lose the second word to an ellipsis, which is the one place in a
	// table where the reader cannot infer what was cut.
	headLines := make([][]string, len(b.Columns))
	headRows := 1
	for i, c := range b.Columns {
		w := r.semi.Wrap(strings.ToUpper(c.Label), sizeTableHead, widths[i]-2*tableCellPadX)
		if len(w) > maxTableHeadLines {
			w = w[:maxTableHeadLines]
		}
		headLines[i] = w
		if len(w) > headRows {
			headRows = len(w)
		}
	}
	headH := float64(headRows)*sizeTableHead*tableLineLead + 12

	drawHeader := func() {
		r.ensure(headH + 24)
		r.page.Rect(pageMargin, r.y, r.contentW, headH, &colSurfaceAlt, nil, 0)
		x := float64(pageMargin)
		for i := range b.Columns {
			align := alignOf(b.Columns[i].Align)
			ty := r.y + 6 + sizeTableHead*0.85
			for _, line := range headLines[i] {
				r.page.TextAligned(r.semi, sizeTableHead, colMuted, x+tableCellPadX,
					widths[i]-2*tableCellPadX, ty, line, align)
				ty += sizeTableHead * tableLineLead
			}
			x += widths[i]
		}
		r.y += headH
		r.page.Line(pageMargin, r.y, r.doc.W-pageMargin, r.y, colHairline, 0.6)
	}
	drawHeader()

	// The tallest a row may become on this paper: an empty page, less the
	// repeated header and the row's own padding. Clamping here rather than
	// at draw time means the measure and the draw always agree.
	cellCap := int((r.bodyHeight() - headH - 9) / (sizeTableCell * tableLineLead))
	if cellCap > maxTableCellLines {
		cellCap = maxTableCellLines
	}
	if cellCap < 1 {
		cellCap = 1
	}

	// The trailing note is measured up front so the last row can reserve
	// room for it. Without that, a table which happens to fill a page
	// exactly pushes its own note onto a page of its own, where it reads
	// as a stray sentence belonging to nothing.
	var noteLines []string
	noteH := 0.0
	if b.Note != "" {
		noteLines = r.reg.Wrap(b.Note, sizeCaption, r.contentW)
		noteH = float64(len(noteLines))*sizeCaption*1.4 + 6
	}

	for ri, row := range b.Rows {
		// Measure the row before committing to it, so a tall row moves
		// whole rather than splitting across a page boundary.
		lines := 1
		wrapped := make([][]string, len(b.Columns))
		for ci := range b.Columns {
			if ci >= len(row) {
				wrapped[ci] = []string{""}
				continue
			}
			cellFont := r.reg
			if row[ci].Mono || b.Columns[ci].Mono {
				cellFont = r.mono
			}
			// `Wrap` hard-splits anything unbreakable — a digest, a
			// container id, a mount path — so a long value folds inside
			// its column instead of bleeding across the next one.
			w := cellFont.Wrap(row[ci].Text, sizeTableCell, widths[ci]-2*tableCellPadX)
			// A cap still exists, because one pathological cell should not
			// own a page; beyond it the tail is ellipsised on the last
			// visible line rather than dropped without a mark.
			if len(w) > cellCap {
				keep := w[:cellCap]
				keep[cellCap-1] = cellFont.Truncate(
					strings.Join(w[cellCap-1:], " "), sizeTableCell,
					widths[ci]-2*tableCellPadX)
				w = keep
			}
			wrapped[ci] = w
			if len(w) > lines {
				lines = len(w)
			}
		}
		rowH := float64(lines)*sizeTableCell*tableLineLead + 9

		need := rowH
		if ri == len(b.Rows)-1 {
			need += noteH
		}
		// A row moves whole; `cellCap` guarantees it can always fit one.
		if r.y+need > r.bottom() {
			r.newPage()
			drawHeader()
		}
		if ri%2 == 1 {
			r.page.Rect(pageMargin, r.y, r.contentW, rowH, &colSurfaceAlt, nil, 0)
		}

		x := float64(pageMargin)
		for ci, col := range b.Columns {
			if ci >= len(row) {
				x += widths[ci]
				continue
			}
			c := row[ci]
			cellFont := r.reg
			if c.Mono || col.Mono {
				cellFont = r.mono
			}
			align := alignOf(col.Align)
			ty := r.y + 5 + sizeTableCell*0.85
			cellW := widths[ci] - 2*tableCellPadX

			if c.Pill && c.Text != "" {
				st := statusColor(c.Status)
				pw := r.semi.Width(c.Text, sizeMicro) + 12
				px := x + tableCellPadX
				switch align {
				case pdf.AlignCenter:
					px = x + (widths[ci]-pw)/2
				case pdf.AlignRight:
					px = x + widths[ci] - tableCellPadX - pw
				}
				bg := pdf.Tint(st, 0.15)
				r.page.RoundRect(px, r.y+4, pw, 13, 6.5, &bg, nil, 0)
				r.page.TextAligned(r.semi, sizeMicro, st, px, pw, r.y+13, c.Text, pdf.AlignCenter)
				x += widths[ci]
				continue
			}

			col2 := colInk
			font := cellFont
			if c.Status != StatusNeutral {
				col2 = statusColor(c.Status)
				font = r.semi
				if c.Mono || b.Columns[ci].Mono {
					font = cellFont
				}
			}
			for _, line := range wrapped[ci] {
				r.page.TextAligned(font, sizeTableCell, col2, x+tableCellPadX, cellW, ty, line, align)
				ty += sizeTableCell * tableLineLead
			}
			x += widths[ci]
		}
		r.y += rowH
		r.page.Line(pageMargin, r.y, r.doc.W-pageMargin, r.y, colHairline, 0.35)
	}

	r.y += 6
	for _, line := range noteLines {
		r.page.Text(r.reg, sizeCaption, colFaint, pageMargin, r.y+sizeCaption, line)
		r.y += sizeCaption * 1.4
	}
	r.space(12)
}

func columnWidths(cols []Column, total float64) []float64 {
	sum := 0.0
	for _, c := range cols {
		w := c.Width
		if w <= 0 {
			w = 1
		}
		sum += w
	}
	out := make([]float64, len(cols))
	for i, c := range cols {
		w := c.Width
		if w <= 0 {
			w = 1
		}
		out[i] = total * w / sum
	}
	return out
}

func alignOf(a string) pdf.Align {
	switch a {
	case "right":
		return pdf.AlignRight
	case "center":
		return pdf.AlignCenter
	default:
		return pdf.AlignLeft
	}
}

// --- charts ----------------------------------------------------------

func (r *pdfRenderer) chart(b Block) {
	if b.Chart == nil || len(b.Chart.Series) == 0 {
		return
	}
	h := b.Chart.Height
	if h <= 0 {
		h = 132
	}
	h = math.Min(h, 300)

	titleH := 0.0
	if b.Chart.Title != "" {
		titleH = sizeMicro + 10
	}
	captionH := 0.0
	var captionLines []string
	if b.Chart.Caption != "" {
		captionLines = r.reg.Wrap(b.Chart.Caption, sizeCaption, r.contentW)
		captionH = float64(len(captionLines))*sizeCaption*1.4 + 4
	}

	r.ensure(h + titleH + captionH + 14)
	if b.Chart.Title != "" {
		r.page.TextTracked(r.semi, sizeMicro, colFaint, pageMargin, r.y+sizeMicro, 0.8,
			strings.ToUpper(b.Chart.Title))
		r.y += titleH
	}

	spec := chartSpec(*b.Chart)
	layout := charts.Build(spec, r.contentW, h, r.measurer(), chartPalette())
	r.drawChart(layout, pageMargin, r.y)
	r.y += h + 6

	for _, line := range captionLines {
		r.page.Text(r.reg, sizeCaption, colFaint, pageMargin, r.y+sizeCaption, line)
		r.y += sizeCaption * 1.4
	}
	r.space(10)
}

func (r *pdfRenderer) measurer() charts.Measure {
	return func(text string, size float64, bold bool) float64 {
		if bold {
			return r.semi.Width(text, size)
		}
		return r.reg.Width(text, size)
	}
}

// chartSpec maps the document's chart block onto the charts package's
// specification, resolving statuses to colours here so the charts
// package stays independent of the report's semantics.
func chartSpec(c Chart) charts.Spec {
	spec := charts.Spec{
		Kind:        charts.Kind(c.Kind),
		Unit:        c.Unit,
		Max:         c.Max,
		Target:      c.Target,
		TargetLabel: c.TargetLabel,
		HasTarget:   c.HasTarget,
		ShowLegend:  c.ShowLegend,
		ShowValues:  c.ShowValues,
	}
	for _, s := range c.Series {
		cs := charts.Series{Name: s.Name, Color: chartStatusRGB(s.Status)}
		for _, p := range s.Points {
			cs.Points = append(cs.Points, charts.Point{
				Label: p.Label, Value: p.Value, Color: chartStatusRGB(p.Status),
			})
		}
		spec.Series = append(spec.Series, cs)
	}
	return spec
}

// drawChart replays chart geometry as PDF vectors. Charts stay vector in
// the PDF so they remain crisp at any zoom and print cleanly.
func (r *pdfRenderer) drawChart(l charts.Layout, ox, oy float64) {
	toColor := func(c charts.RGB) pdf.Color { return pdf.RGB(c.R, c.G, c.B) }

	for _, sh := range l.Shapes {
		alpha := sh.Alpha
		if alpha == 0 {
			alpha = 1
		}
		var fill, stroke *pdf.Color
		if sh.Fill != nil {
			c := pdf.Tint(toColor(*sh.Fill), alpha)
			fill = &c
		}
		if sh.Stroke != nil {
			c := pdf.Tint(toColor(*sh.Stroke), alpha)
			stroke = &c
		}
		lw := sh.LineWidth
		if lw <= 0 {
			lw = 0.5
		}
		switch sh.Kind {
		case charts.ShapeRect:
			r.page.Rect(ox+sh.X, oy+sh.Y, sh.W, sh.H, fill, stroke, lw)
		case charts.ShapeRoundRect:
			r.page.RoundRect(ox+sh.X, oy+sh.Y, sh.W, sh.H, sh.R, fill, stroke, lw)
		case charts.ShapeCircle:
			r.page.Circle(ox+sh.X, oy+sh.Y, sh.R, fill, stroke, lw)
		case charts.ShapeArc:
			if fill != nil {
				r.page.Arc(ox+sh.X, oy+sh.Y, sh.R, sh.RInner, sh.Start, sh.Sweep, *fill)
			}
		case charts.ShapeLine:
			if len(sh.Pts) >= 2 && stroke != nil {
				r.drawChartLine(ox, oy, sh, *stroke, lw)
			}
		case charts.ShapePath:
			pts := make([]pdf.Pt, len(sh.Pts))
			for i, p := range sh.Pts {
				pts[i] = pdf.Pt{X: ox + p.X, Y: oy + p.Y}
			}
			if fill != nil {
				r.page.Polygon(pts, fill, nil, 0)
			}
			if stroke != nil {
				r.page.Polyline(pts, *stroke, lw)
			}
		}
	}

	for _, lb := range l.Labels {
		font := r.reg
		if lb.Bold {
			font = r.semi
		}
		col := toColor(lb.Color)
		switch lb.Align {
		case charts.AlignCenter:
			r.page.Text(font, lb.Size, col, ox+lb.X-font.Width(lb.Text, lb.Size)/2, oy+lb.Y, lb.Text)
		case charts.AlignRight:
			r.page.Text(font, lb.Size, col, ox+lb.X-font.Width(lb.Text, lb.Size), oy+lb.Y, lb.Text)
		default:
			r.page.Text(font, lb.Size, col, ox+lb.X, oy+lb.Y, lb.Text)
		}
	}
}

// drawChartLine handles the dashed reference line, which PDF has no
// primitive for at this level — it is drawn as a run of short segments.
func (r *pdfRenderer) drawChartLine(ox, oy float64, sh charts.Shape, col pdf.Color, lw float64) {
	a, b := sh.Pts[0], sh.Pts[1]
	if !sh.Dashed {
		r.page.Line(ox+a.X, oy+a.Y, ox+b.X, oy+b.Y, col, lw)
		return
	}
	const dash, gap = 3.2, 2.4
	length := math.Hypot(b.X-a.X, b.Y-a.Y)
	if length <= 0 {
		return
	}
	ux, uy := (b.X-a.X)/length, (b.Y-a.Y)/length
	for d := 0.0; d < length; d += dash + gap {
		e := math.Min(d+dash, length)
		r.page.Line(ox+a.X+ux*d, oy+a.Y+uy*d, ox+a.X+ux*e, oy+a.Y+uy*e, col, lw)
	}
}
