// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package charts turns a chart specification into pure geometry —
// rectangles, arcs, paths and positioned labels — with no knowledge of
// how it will be drawn.
//
// That indirection is the point. The PDF export replays the geometry as
// vectors; the Word export replays the identical geometry into a raster;
// the React viewer replays the same semantics as SVG. One layout pass
// means the three surfaces cannot drift, which is what lets a reader
// trust that the chart on screen is the chart in the board pack.
//
// House style, applied here rather than left to each caller: zero
// baselines on every bar chart, no 3-D, no chartjunk, direct labelling
// in preference to a legend when there are few series, and a status ramp
// that still separates in greyscale for anyone printing in mono.
package charts

import "math"

// Kind selects the chart form.
type Kind string

const (
	Bar        Kind = "bar"        // vertical, grouped by series
	HBar       Kind = "hbar"       // horizontal, one bar per category
	StackedBar Kind = "stackedbar" // vertical, series stacked per category
	Donut      Kind = "donut"      // composition, with legend
	Line       Kind = "line"       // trend over time
	Area       Kind = "area"       // trend with filled body
	Gauge      Kind = "gauge"      // a single score against its bands
)

// RGB is an 8-bit colour.
type RGB struct{ R, G, B uint8 }

// Point is one datum. Color overrides the palette for this datum alone —
// how a status split (healthy / degraded / down) keeps its meaning
// instead of taking whatever the categorical ramp hands it.
type Point struct {
	Label string
	Value float64
	Color RGB
}

// Series is one named set of points.
type Series struct {
	Name   string
	Color  RGB
	Points []Point
}

// Spec describes what to draw.
type Spec struct {
	Kind   Kind
	Series []Series

	// Unit is appended to axis and value labels ("%", "ms", "GB").
	Unit string
	// Max pins the value axis; 0 lets the layout choose a nice round top.
	Max float64
	// Target draws a reference line — an SLO, a policy threshold.
	Target      float64
	TargetLabel string
	HasTarget   bool

	ShowLegend bool
	ShowValues bool
	// Format renders a value for display. nil falls back to a compact
	// default that drops needless decimals.
	Format func(float64) string
}

// Palette carries the document's colours so charts match the page they
// sit on without importing the theme package (which would make this
// package report-specific).
type Palette struct {
	Ink        RGB
	Muted      RGB
	Hairline   RGB
	SurfaceAlt RGB
	Track      RGB
	Series     []RGB
}

// ShapeKind discriminates the geometry union.
type ShapeKind int

const (
	ShapeRect ShapeKind = iota
	ShapeRoundRect
	ShapeLine
	ShapePath
	ShapeArc
	ShapeCircle
)

// Pt is a point in chart-local coordinates: origin top-left, y downward,
// matching both renderers.
type Pt struct{ X, Y float64 }

// Shape is one drawing primitive.
type Shape struct {
	Kind      ShapeKind
	X, Y      float64
	W, H      float64
	R         float64 // corner radius, or outer radius for arc/circle
	RInner    float64 // arc inner radius
	Start     float64 // arc start, degrees clockwise from 12 o'clock
	Sweep     float64 // arc sweep, degrees
	Pts       []Pt
	Fill      *RGB
	Stroke    *RGB
	LineWidth float64
	Alpha     float64 // 0 is treated as 1
	Dashed    bool
}

// Align places a label relative to its anchor point.
type Align int

const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

// Label is positioned text. Y is the baseline.
type Label struct {
	Text  string
	X, Y  float64
	Size  float64
	Color RGB
	Align Align
	Bold  bool
}

// Layout is the finished geometry.
type Layout struct {
	W, H   float64
	Shapes []Shape
	Labels []Label
}

// Measure reports the width of a string, so layout can reserve gutters
// for axis labels without guessing.
type Measure func(text string, size float64, bold bool) float64

// Type sizes inside a chart. Deliberately smaller than body copy: chart
// labels are read in relation to the marks, not as prose.
const (
	axisSize   = 6.5
	catSize    = 6.8
	valueSize  = 6.8
	legendSize = 7.4
)

func (s Spec) format(v float64) string {
	if s.Format != nil {
		return s.Format(v)
	}
	return DefaultFormat(v) + s.Unit
}

// DefaultFormat renders a number compactly: no decimals for whole
// numbers, one for the rest, and thousands separated with a thin space
// so a four-digit count does not read as a year.
func DefaultFormat(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	var s string
	switch {
	case v >= 10000:
		s = groupThousands(int64(math.Round(v)))
	case v == math.Trunc(v):
		s = trimFloat(v, 0)
	case v >= 100:
		s = trimFloat(v, 0)
	case v >= 10:
		s = trimFloat(v, 1)
	default:
		s = trimFloat(v, 1)
	}
	if neg {
		return "−" + s
	}
	return s
}

func trimFloat(v float64, dp int) string {
	mult := math.Pow(10, float64(dp))
	r := math.Round(v*mult) / mult
	out := ""
	if dp == 0 {
		return groupThousands(int64(r))
	}
	whole := int64(math.Trunc(r))
	frac := int64(math.Round(math.Abs(r-float64(whole)) * mult))
	if frac == 0 {
		return groupThousands(whole)
	}
	out = groupThousands(whole) + "." + itoaPad(frac, dp)
	return out
}

func itoaPad(v int64, width int) string {
	s := ""
	for v > 0 {
		s = string(rune('0'+v%10)) + s
		v /= 10
	}
	for len(s) < width {
		s = "0" + s
	}
	if s == "" {
		s = "0"
	}
	return s
}

func groupThousands(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	digits := ""
	if v == 0 {
		digits = "0"
	}
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	// U+2009 THIN SPACE reads as a separator without the comma/period
	// ambiguity between locales.
	var out []rune
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, ' ')
		}
		out = append(out, r)
	}
	if neg {
		return "−" + string(out)
	}
	return string(out)
}

// niceMax rounds an axis top up to a value a reader can divide by four
// in their head. An axis that tops out at 9 437 helps nobody.
func niceMax(v float64) float64 {
	if v <= 0 {
		return 1
	}
	exp := math.Floor(math.Log10(v))
	pow := math.Pow(10, exp)
	switch f := v / pow; {
	case f <= 1:
		return 1 * pow
	case f <= 2:
		return 2 * pow
	case f <= 2.5:
		return 2.5 * pow
	case f <= 4:
		return 4 * pow
	case f <= 5:
		return 5 * pow
	case f <= 8:
		return 8 * pow
	default:
		return 10 * pow
	}
}

func (s Spec) maxValue() float64 {
	if s.Max > 0 {
		return s.Max
	}
	m := 0.0
	if s.Kind == StackedBar {
		n := 0
		for _, ser := range s.Series {
			if len(ser.Points) > n {
				n = len(ser.Points)
			}
		}
		for i := 0; i < n; i++ {
			sum := 0.0
			for _, ser := range s.Series {
				if i < len(ser.Points) {
					sum += ser.Points[i].Value
				}
			}
			m = math.Max(m, sum)
		}
	} else {
		for _, ser := range s.Series {
			for _, p := range ser.Points {
				m = math.Max(m, p.Value)
			}
		}
	}
	if s.HasTarget {
		m = math.Max(m, s.Target)
	}
	if m <= 0 {
		return 1
	}
	return niceMax(m)
}

func (s Spec) categories() []string {
	var out []string
	for _, ser := range s.Series {
		for i, p := range ser.Points {
			if i >= len(out) {
				out = append(out, p.Label)
			} else if out[i] == "" {
				out[i] = p.Label
			}
		}
	}
	return out
}

// series resolves a colour: the first non-zero override wins, otherwise
// the categorical ramp by index.
func (p Palette) series(i int, overrides ...RGB) RGB {
	for _, o := range overrides {
		if o != (RGB{}) {
			return o
		}
	}
	if len(p.Series) == 0 {
		return p.Ink
	}
	return p.Series[i%len(p.Series)]
}

func rgbPtr(c RGB) *RGB { return &c }
