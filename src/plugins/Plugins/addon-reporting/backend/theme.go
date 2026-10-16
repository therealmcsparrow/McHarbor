// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-reporting/backend/charts"
	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-reporting/backend/pdf"
)

// The document palette.
//
// McHarbor's UI is dark-first and defines colour in OKLCH; a report is
// neither. It is printed, photocopied, projected and pasted into decks,
// so it renders on a light ground with ink that survives greyscale. The
// hues come from the same source the default "Tidal" theme was derived
// from — the logo's teal rgb(38,176,198) and deep blue rgb(39,106,162) —
// so the document reads as the same product without pretending a page is
// a screen.
var (
	colBrand      = pdf.RGB(39, 106, 162)  // deep blue — primary
	colTeal       = pdf.RGB(38, 176, 198)  // teal — secondary / accent
	colInk        = pdf.RGB(34, 48, 63)    // body text
	colInkStrong  = pdf.RGB(20, 31, 43)    // headings
	colMuted      = pdf.RGB(91, 107, 124)  // captions, axis labels
	colFaint      = pdf.RGB(140, 155, 169) // page furniture
	colHairline   = pdf.RGB(221, 228, 235) // rules, table lines
	colSurface    = pdf.RGB(255, 255, 255)
	colSurfaceAlt = pdf.RGB(245, 248, 250) // zebra rows, callout grounds
	colTrack      = pdf.RGB(234, 240, 245) // bar tracks

	colGood = pdf.RGB(30, 158, 106)
	colWarn = pdf.RGB(201, 138, 21)
	colBad  = pdf.RGB(196, 72, 60)
	colInfo = pdf.RGB(60, 125, 196)
)

// hex mirrors the palette for the DOCX exporter, which speaks RRGGBB.
const (
	hexBrand      = "276AA2"
	hexTeal       = "26B0C6"
	hexInk        = "22303F"
	hexInkStrong  = "141F2B"
	hexMuted      = "5B6B7C"
	hexHairline   = "DDE4EB"
	hexSurfaceAlt = "F5F8FA"

	hexGood = "1E9E6A"
	hexWarn = "C98A15"
	hexBad  = "C4483C"
	hexInfo = "3C7DC4"

	// Tints for status pills — light enough to keep dark text legible.
	hexGoodTint = "E4F4EC"
	hexWarnTint = "FAF0DC"
	hexBadTint  = "FBE9E7"
	hexInfoTint = "E7F0FA"
)

// statusColor maps a semantic status to ink.
func statusColor(s Status) pdf.Color {
	switch s {
	case StatusGood:
		return colGood
	case StatusWarn:
		return colWarn
	case StatusBad:
		return colBad
	case StatusInfo:
		return colInfo
	case StatusMuted:
		return colFaint
	default:
		return colInk
	}
}

func statusHex(s Status) string {
	switch s {
	case StatusGood:
		return hexGood
	case StatusWarn:
		return hexWarn
	case StatusBad:
		return hexBad
	case StatusInfo:
		return hexInfo
	default:
		return hexInk
	}
}

func statusTintHex(s Status) string {
	switch s {
	case StatusGood:
		return hexGoodTint
	case StatusWarn:
		return hexWarnTint
	case StatusBad:
		return hexBadTint
	case StatusInfo:
		return hexInfoTint
	default:
		return hexSurfaceAlt
	}
}

// bandStatus maps a posture band to the status ramp, so a scorecard and
// a table cell agree on what "at risk" looks like.
func bandStatus(b Band) Status {
	switch b {
	case BandStrong, BandStable:
		return StatusGood
	case BandWatch:
		return StatusWarn
	case BandAtRisk, BandCritical:
		return StatusBad
	default:
		return StatusNeutral
	}
}

func severityStatus(s Severity) Status {
	switch s {
	case SevCritical, SevHigh:
		return StatusBad
	case SevMedium:
		return StatusWarn
	case SevLow:
		return StatusInfo
	default:
		return StatusNeutral
	}
}

// Type scale, in points. Deliberately compressed relative to the UI:
// a page is read at arm's length and a report that sets body copy at
// screen sizes runs to twice the pages it needs.
// Each is typed explicitly: an untyped constant here would infer as int
// at every `size := sizeH1` and quietly refuse to take a fractional
// step later.
const (
	sizeCoverTitle float64 = 30
	sizeCoverSub   float64 = 12
	sizeH1         float64 = 17
	sizeH2         float64 = 12.5
	sizeH3         float64 = 10.5
	sizeLead       float64 = 11
	sizeBody       float64 = 9.3
	sizeSmall      float64 = 8.2
	sizeCaption    float64 = 7.6
	sizeMicro      float64 = 6.8
	sizeEyebrow    float64 = 7.4
	sizeKPIValue   float64 = 19
	sizeKPILabel   float64 = 7
	sizeTableHead  float64 = 7.4
	sizeTableCell  float64 = 8.4
)

// Leading multipliers.
const (
	leadBody  float64 = 1.45
	leadTight float64 = 1.28
)

// Page geometry, in points.
//
// The content margin is 1.27 cm — 0.5 inch, 36 pt exactly — on all four
// sides, on every paper size. The running header and footer live inside
// that margin, the way Word's `w:header` / `w:footer` distances do, so
// the body still begins and ends exactly 36 pt from the sheet edge and a
// table gets the full measure between them.
const (
	pageMargin float64 = 36

	// headerBaselineY is measured down from the top edge; the header rule
	// sits headerRuleGap below that baseline, still above the margin.
	headerBaselineY float64 = 20
	headerRuleGap   float64 = 7

	// footerRuleGap and footerBaselineGap are measured down from the
	// bottom content edge (height - pageMargin).
	footerRuleGap     float64 = 9
	footerBaselineGap float64 = 20
)

// chartPalette is the categorical ramp. Ordered so the first four are
// distinguishable to the most common forms of colour vision deficiency
// and still separate by lightness in greyscale — a report gets printed
// in mono more often than anyone plans for.
func chartPalette() charts.Palette {
	return charts.Palette{
		Ink:        charts.RGB{R: 34, G: 48, B: 63},
		Muted:      charts.RGB{R: 91, G: 107, B: 124},
		Hairline:   charts.RGB{R: 221, G: 228, B: 235},
		SurfaceAlt: charts.RGB{R: 245, G: 248, B: 250},
		Track:      charts.RGB{R: 234, G: 240, B: 245},
		Series: []charts.RGB{
			{R: 39, G: 106, B: 162},  // brand deep blue
			{R: 38, G: 176, B: 198},  // brand teal
			{R: 124, G: 92, B: 190},  // violet
			{R: 201, G: 138, B: 21},  // amber
			{R: 30, G: 158, B: 106},  // green
			{R: 160, G: 176, B: 190}, // slate — the "everything else" bin
		},
	}
}

// chartStatusRGB maps a datum's status onto the chart palette.
func chartStatusRGB(s Status) charts.RGB {
	switch s {
	case StatusGood:
		return charts.RGB{R: 30, G: 158, B: 106}
	case StatusWarn:
		return charts.RGB{R: 201, G: 138, B: 21}
	case StatusBad:
		return charts.RGB{R: 196, G: 72, B: 60}
	case StatusInfo:
		return charts.RGB{R: 60, G: 125, B: 196}
	case StatusMuted:
		return charts.RGB{R: 160, G: 176, B: 190}
	default:
		// Zero value means "no override": the chart takes the next
		// colour from the categorical ramp.
		return charts.RGB{}
	}
}
