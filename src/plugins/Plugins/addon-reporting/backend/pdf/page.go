// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pdf

import (
	"bytes"
	"fmt"
	"math"
)

// Align controls horizontal placement of drawn text.
type Align int

const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

// Page is one page's content stream.
//
// All coordinates are top-left origin with y growing downward. The
// conversion to PDF's bottom-left space happens here and nowhere else,
// so the layout code above never has to think about it.
type Page struct {
	doc     *Doc
	index   int
	content bytes.Buffer
}

// Index is the zero-based page position, used for bookmarks.
func (p *Page) Index() int { return p.index }

func (p *Page) y(v float64) float64 { return p.doc.H - v }

func (p *Page) op(format string, args ...any) {
	fmt.Fprintf(&p.content, format, args...)
	p.content.WriteByte('\n')
}

// Save/Restore bracket a graphics state change.
func (p *Page) Save()    { p.op("q") }
func (p *Page) Restore() { p.op("Q") }

// SetAlpha sets constant alpha for subsequent fills. Implemented by
// blending against white at draw time rather than with an ExtGState:
// report backgrounds are always the page, so the result is identical
// and the file stays simpler.
func blendOnWhite(c Color, alpha float64) Color {
	if alpha >= 1 {
		return c
	}
	if alpha <= 0 {
		return Color{1, 1, 1}
	}
	return Mix(Color{1, 1, 1}, c, alpha)
}

// Tint returns `c` blended toward white — the plugin's stand-in for the
// low-opacity surface fills the UI gets from OKLCH alpha tokens.
func Tint(c Color, alpha float64) Color { return blendOnWhite(c, alpha) }

func (p *Page) setFill(c Color)   { p.op("%s %s %s rg", num(c.R), num(c.G), num(c.B)) }
func (p *Page) setStroke(c Color) { p.op("%s %s %s RG", num(c.R), num(c.G), num(c.B)) }

// Rect fills and/or strokes an axis-aligned rectangle.
func (p *Page) Rect(x, yTop, w, h float64, fill *Color, stroke *Color, lineWidth float64) {
	if w <= 0 || h <= 0 {
		return
	}
	p.Save()
	if fill != nil {
		p.setFill(*fill)
	}
	if stroke != nil {
		p.setStroke(*stroke)
		p.op("%s w", num(lineWidth))
	}
	p.op("%s %s %s %s re", num(x), num(p.y(yTop+h)), num(w), num(h))
	p.paint(fill, stroke)
	p.Restore()
}

// RoundRect is Rect with corner radius `r`, drawn with four Bézier arcs.
func (p *Page) RoundRect(x, yTop, w, h, r float64, fill *Color, stroke *Color, lineWidth float64) {
	if w <= 0 || h <= 0 {
		return
	}
	if r <= 0 {
		p.Rect(x, yTop, w, h, fill, stroke, lineWidth)
		return
	}
	r = math.Min(r, math.Min(w, h)/2)
	// k is the circular-arc Bézier constant.
	const k = 0.5522847498
	c := k * r
	y0 := p.y(yTop + h) // bottom
	y1 := p.y(yTop)     // top
	x0, x1 := x, x+w

	p.Save()
	if fill != nil {
		p.setFill(*fill)
	}
	if stroke != nil {
		p.setStroke(*stroke)
		p.op("%s w", num(lineWidth))
	}
	p.op("%s %s m", num(x0+r), num(y0))
	p.op("%s %s l", num(x1-r), num(y0))
	p.op("%s %s %s %s %s %s c", num(x1-r+c), num(y0), num(x1), num(y0+r-c), num(x1), num(y0+r))
	p.op("%s %s l", num(x1), num(y1-r))
	p.op("%s %s %s %s %s %s c", num(x1), num(y1-r+c), num(x1-r+c), num(y1), num(x1-r), num(y1))
	p.op("%s %s l", num(x0+r), num(y1))
	p.op("%s %s %s %s %s %s c", num(x0+r-c), num(y1), num(x0), num(y1-r+c), num(x0), num(y1-r))
	p.op("%s %s l", num(x0), num(y0+r))
	p.op("%s %s %s %s %s %s c", num(x0), num(y0+r-c), num(x0+r-c), num(y0), num(x0+r), num(y0))
	p.op("h")
	p.paint(fill, stroke)
	p.Restore()
}

func (p *Page) paint(fill *Color, stroke *Color) {
	switch {
	case fill != nil && stroke != nil:
		p.op("B")
	case fill != nil:
		p.op("f")
	case stroke != nil:
		p.op("S")
	default:
		p.op("n")
	}
}

// Line strokes a straight segment.
func (p *Page) Line(x1, y1, x2, y2 float64, c Color, lineWidth float64) {
	p.Save()
	p.setStroke(c)
	p.op("%s w", num(lineWidth))
	p.op("1 J") // round caps: hairlines meeting at a corner look joined
	p.op("%s %s m %s %s l S", num(x1), num(p.y(y1)), num(x2), num(p.y(y2)))
	p.Restore()
}

// Pt is a point in the page's top-left coordinate system.
type Pt struct{ X, Y float64 }

// Polygon fills and/or strokes a closed path.
func (p *Page) Polygon(pts []Pt, fill *Color, stroke *Color, lineWidth float64) {
	if len(pts) < 2 {
		return
	}
	p.Save()
	if fill != nil {
		p.setFill(*fill)
	}
	if stroke != nil {
		p.setStroke(*stroke)
		p.op("%s w", num(lineWidth))
		p.op("1 j") // round joins
	}
	p.op("%s %s m", num(pts[0].X), num(p.y(pts[0].Y)))
	for _, pt := range pts[1:] {
		p.op("%s %s l", num(pt.X), num(p.y(pt.Y)))
	}
	p.op("h")
	p.paint(fill, stroke)
	p.Restore()
}

// Polyline strokes an open path — used for trend lines.
func (p *Page) Polyline(pts []Pt, c Color, lineWidth float64) {
	if len(pts) < 2 {
		return
	}
	p.Save()
	p.setStroke(c)
	p.op("%s w", num(lineWidth))
	p.op("1 J")
	p.op("1 j")
	p.op("%s %s m", num(pts[0].X), num(p.y(pts[0].Y)))
	for _, pt := range pts[1:] {
		p.op("%s %s l", num(pt.X), num(p.y(pt.Y)))
	}
	p.op("S")
	p.Restore()
}

// Circle fills and/or strokes a circle of radius r centred at (cx, cy).
func (p *Page) Circle(cx, cy, r float64, fill *Color, stroke *Color, lineWidth float64) {
	const k = 0.5522847498
	c := k * r
	y := p.y(cy)
	p.Save()
	if fill != nil {
		p.setFill(*fill)
	}
	if stroke != nil {
		p.setStroke(*stroke)
		p.op("%s w", num(lineWidth))
	}
	p.op("%s %s m", num(cx+r), num(y))
	p.op("%s %s %s %s %s %s c", num(cx+r), num(y+c), num(cx+c), num(y+r), num(cx), num(y+r))
	p.op("%s %s %s %s %s %s c", num(cx-c), num(y+r), num(cx-r), num(y+c), num(cx-r), num(y))
	p.op("%s %s %s %s %s %s c", num(cx-r), num(y-c), num(cx-c), num(y-r), num(cx), num(y-r))
	p.op("%s %s %s %s %s %s c", num(cx+c), num(y-r), num(cx+r), num(y-c), num(cx+r), num(y))
	p.op("h")
	p.paint(fill, stroke)
	p.Restore()
}

// Arc fills an annular sector: the donut-chart primitive. Angles are in
// degrees, clockwise from 12 o'clock, which is how a reader describes a
// pie slice.
func (p *Page) Arc(cx, cy, rOuter, rInner, startDeg, sweepDeg float64, fill Color) {
	if sweepDeg <= 0 || rOuter <= 0 {
		return
	}
	if sweepDeg >= 360 {
		sweepDeg = 359.999
	}
	toRad := func(d float64) float64 { return (90 - d) * math.Pi / 180 }
	// Flatten to segments: 2° gives a curve that is smooth well past the
	// zoom level anyone will use, without emitting Bézier control points.
	steps := int(math.Ceil(sweepDeg / 2))
	if steps < 2 {
		steps = 2
	}
	pts := make([]Pt, 0, steps*2+2)
	for i := 0; i <= steps; i++ {
		a := toRad(startDeg + sweepDeg*float64(i)/float64(steps))
		pts = append(pts, Pt{cx + rOuter*math.Cos(a), cy - rOuter*math.Sin(a)})
	}
	if rInner > 0 {
		for i := steps; i >= 0; i-- {
			a := toRad(startDeg + sweepDeg*float64(i)/float64(steps))
			pts = append(pts, Pt{cx + rInner*math.Cos(a), cy - rInner*math.Sin(a)})
		}
	} else {
		pts = append(pts, Pt{cx, cy})
	}
	p.Polygon(pts, &fill, nil, 0)
}

// Clip intersects the clipping path with a rectangle until the matching
// Restore. Used to keep long table cells inside their column.
func (p *Page) Clip(x, yTop, w, h float64) {
	p.op("%s %s %s %s re W n", num(x), num(p.y(yTop+h)), num(w), num(h))
}

// Text draws a single line. `yBaseline` is the baseline measured from
// the top of the page.
func (p *Page) Text(f *Font, size float64, c Color, x, yBaseline float64, s string) {
	if s == "" || f == nil {
		return
	}
	p.Save()
	p.setFill(c)
	p.op("BT /F%d %s Tf %s %s Td <%s> Tj ET",
		fontRef(f), num(size), num(x), num(p.y(yBaseline)), f.encode(s))
	p.Restore()
}

// TextAligned draws a line inside [x, x+w] with the given alignment.
func (p *Page) TextAligned(f *Font, size float64, c Color, x, w, yBaseline float64, s string, a Align) {
	if s == "" || f == nil {
		return
	}
	tx := x
	switch a {
	case AlignCenter:
		tx = x + (w-f.Width(s, size))/2
	case AlignRight:
		tx = x + w - f.Width(s, size)
	}
	p.Text(f, size, c, tx, yBaseline, s)
}

// TextTracked draws a line with extra letter spacing, for the small-caps
// style eyebrow labels the report uses above each section.
func (p *Page) TextTracked(f *Font, size float64, c Color, x, yBaseline, tracking float64, s string) {
	if s == "" || f == nil {
		return
	}
	p.Save()
	p.setFill(c)
	p.op("BT /F%d %s Tf %s Tc %s %s Td <%s> Tj ET",
		fontRef(f), num(size), num(tracking), num(x), num(p.y(yBaseline)), f.encode(s))
	p.Restore()
}

// TrackedWidth measures what TextTracked will occupy.
func TrackedWidth(f *Font, s string, size, tracking float64) float64 {
	return f.Width(s, size) + tracking*float64(len([]rune(s)))
}

// Paragraph draws wrapped text and returns the y just below the last
// line. `leading` is the baseline-to-baseline distance.
func (p *Page) Paragraph(f *Font, size float64, c Color, x, yTop, w, leading float64, s string) float64 {
	lines := f.Wrap(s, size, w)
	y := yTop + f.Ascent(size)
	for _, ln := range lines {
		p.Text(f, size, c, x, y, ln)
		y += leading
	}
	return y - leading + (-f.Descent(size))
}

// DrawImage places an image, preserving the caller's box exactly.
func (p *Page) DrawImage(img *Image, x, yTop, w, h float64) {
	if img == nil || w <= 0 || h <= 0 {
		return
	}
	p.Save()
	p.op("%s 0 0 %s %s %s cm", num(w), num(h), num(x), num(p.y(yTop+h)))
	p.op("/X%d Do", img.refID)
	p.Restore()
	img.used = true
}

// fontRef is the resource name suffix. Object numbers are assigned at
// write time, so the content stream is generated first and the resource
// dictionary is keyed to match — see Doc.Bytes.
func fontRef(f *Font) int { return f.refID }
