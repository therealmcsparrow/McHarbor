// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fontkit

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// Canvas is a small anti-aliased polygon rasteriser over an RGBA image.
//
// It exists so the Word export can carry the same charts the PDF draws
// as vectors. Coverage is computed by sampling `subSamples` sub-scanlines
// per pixel row and accumulating exact horizontal span coverage on each,
// which gives clean edges on both glyph curves and chart geometry
// without pulling in a rendering dependency.
type Canvas struct {
	img  *image.RGBA
	W, H int

	cov []float32 // one scanline of accumulated coverage
	xs  []crossing
}

type crossing struct {
	x   float64
	dir int
}

// subSamples is the vertical sample count per pixel row. Four is the
// point where more stops being visible in printed output.
const subSamples = 4

// NewCanvas allocates an opaque white canvas.
func NewCanvas(w, h int, bg color.RGBA) *Canvas {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = bg.R, bg.G, bg.B, bg.A
	}
	return &Canvas{img: img, W: w, H: h, cov: make([]float32, w)}
}

// Image exposes the underlying buffer.
func (c *Canvas) Image() *image.RGBA { return c.img }

// Fill rasterises a set of contours with the non-zero winding rule,
// which is what TrueType outlines and every chart shape here assume.
func (c *Canvas) Fill(contours []Contour, col color.RGBA, alpha float64) {
	if len(contours) == 0 || alpha <= 0 {
		return
	}
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, cont := range contours {
		for _, p := range cont {
			minY = math.Min(minY, p.Y)
			maxY = math.Max(maxY, p.Y)
		}
	}
	y0 := int(math.Floor(minY))
	y1 := int(math.Ceil(maxY))
	if y0 < 0 {
		y0 = 0
	}
	if y1 > c.H {
		y1 = c.H
	}

	for py := y0; py < y1; py++ {
		for i := range c.cov {
			c.cov[i] = 0
		}
		touched := false
		for s := 0; s < subSamples; s++ {
			sy := float64(py) + (float64(s)+0.5)/subSamples
			c.xs = c.xs[:0]
			for _, cont := range contours {
				n := len(cont)
				for i := 0; i < n; i++ {
					a, b := cont[i], cont[(i+1)%n]
					if a.Y == b.Y {
						continue
					}
					dir := 1
					lo, hi := a, b
					if a.Y > b.Y {
						lo, hi = b, a
						dir = -1
					}
					// Half-open interval avoids double-counting a vertex
					// that two edges share.
					if sy < lo.Y || sy >= hi.Y {
						continue
					}
					t := (sy - lo.Y) / (hi.Y - lo.Y)
					c.xs = append(c.xs, crossing{x: lo.X + t*(hi.X-lo.X), dir: dir})
				}
			}
			if len(c.xs) < 2 {
				continue
			}
			sort.Slice(c.xs, func(i, j int) bool { return c.xs[i].x < c.xs[j].x })
			wind := 0
			for i := 0; i < len(c.xs)-1; i++ {
				wind += c.xs[i].dir
				if wind == 0 {
					continue
				}
				if c.addSpan(c.xs[i].x, c.xs[i+1].x) {
					touched = true
				}
			}
		}
		if touched {
			c.blendRow(py, col, alpha)
		}
	}
}

// addSpan accumulates fractional horizontal coverage for one sub-scanline.
func (c *Canvas) addSpan(xa, xb float64) bool {
	if xb <= xa {
		return false
	}
	if xa < 0 {
		xa = 0
	}
	if xb > float64(c.W) {
		xb = float64(c.W)
	}
	if xb <= xa {
		return false
	}
	const w = 1.0 / subSamples
	ia, ib := int(xa), int(xb)
	if ia == ib {
		c.cov[ia] += float32((xb - xa) * w)
		return true
	}
	c.cov[ia] += float32((float64(ia+1) - xa) * w)
	for i := ia + 1; i < ib && i < c.W; i++ {
		c.cov[i] += float32(w)
	}
	if ib < c.W {
		c.cov[ib] += float32((xb - float64(ib)) * w)
	}
	return true
}

func (c *Canvas) blendRow(py int, col color.RGBA, alpha float64) {
	base := py * c.img.Stride
	for x := 0; x < c.W; x++ {
		a := float64(c.cov[x]) * alpha
		if a <= 0.0005 {
			continue
		}
		if a > 1 {
			a = 1
		}
		o := base + x*4
		c.img.Pix[o] = mixByte(c.img.Pix[o], col.R, a)
		c.img.Pix[o+1] = mixByte(c.img.Pix[o+1], col.G, a)
		c.img.Pix[o+2] = mixByte(c.img.Pix[o+2], col.B, a)
		if c.img.Pix[o+3] < 255 {
			c.img.Pix[o+3] = mixByte(c.img.Pix[o+3], 255, a)
		}
	}
}

func mixByte(dst, src uint8, a float64) uint8 {
	return uint8(float64(dst)*(1-a) + float64(src)*a + 0.5)
}

// FillRect is the axis-aligned fast path — most chart geometry is
// rectangles, and going through the general rasteriser for them would be
// both slower and slightly soft on edges that should be crisp.
func (c *Canvas) FillRect(x, y, w, h float64, col color.RGBA, alpha float64) {
	if w <= 0 || h <= 0 || alpha <= 0 {
		return
	}
	c.Fill([]Contour{{
		{X: x, Y: y}, {X: x + w, Y: y}, {X: x + w, Y: y + h}, {X: x, Y: y + h},
	}}, col, alpha)
}

// StrokeLine draws a line of the given width as a quad.
func (c *Canvas) StrokeLine(x1, y1, x2, y2, width float64, col color.RGBA, alpha float64) {
	dx, dy := x2-x1, y2-y1
	l := math.Hypot(dx, dy)
	if l == 0 || width <= 0 {
		return
	}
	nx, ny := -dy/l*width/2, dx/l*width/2
	c.Fill([]Contour{{
		{X: x1 + nx, Y: y1 + ny}, {X: x2 + nx, Y: y2 + ny},
		{X: x2 - nx, Y: y2 - ny}, {X: x1 - nx, Y: y1 - ny},
	}}, col, alpha)
}

// StrokePath draws a polyline with round-ish joins (overlapping quads
// plus a dot at each interior vertex).
func (c *Canvas) StrokePath(pts []Point, width float64, col color.RGBA, alpha float64) {
	if len(pts) < 2 {
		return
	}
	for i := 0; i < len(pts)-1; i++ {
		c.StrokeLine(pts[i].X, pts[i].Y, pts[i+1].X, pts[i+1].Y, width, col, alpha)
	}
	if width > 1.5 {
		for i := 1; i < len(pts)-1; i++ {
			c.FillCircle(pts[i].X, pts[i].Y, width/2, col, alpha)
		}
	}
}

// FillCircle rasterises a disc.
func (c *Canvas) FillCircle(cx, cy, r float64, col color.RGBA, alpha float64) {
	if r <= 0 {
		return
	}
	steps := int(math.Max(12, math.Min(72, r*3)))
	cont := make(Contour, 0, steps)
	for i := 0; i < steps; i++ {
		a := 2 * math.Pi * float64(i) / float64(steps)
		cont = append(cont, Point{X: cx + r*math.Cos(a), Y: cy + r*math.Sin(a)})
	}
	c.Fill([]Contour{cont}, col, alpha)
}

// FillArc rasterises an annular sector. Angles are degrees clockwise
// from twelve o'clock, matching the PDF side so a donut looks the same
// in both exports.
func (c *Canvas) FillArc(cx, cy, rOuter, rInner, startDeg, sweepDeg float64, col color.RGBA, alpha float64) {
	if sweepDeg <= 0 || rOuter <= 0 {
		return
	}
	if sweepDeg > 359.999 {
		sweepDeg = 359.999
	}
	steps := int(math.Max(6, sweepDeg/2))
	rad := func(d float64) float64 { return (90 - d) * math.Pi / 180 }
	cont := make(Contour, 0, steps*2+2)
	for i := 0; i <= steps; i++ {
		a := rad(startDeg + sweepDeg*float64(i)/float64(steps))
		cont = append(cont, Point{X: cx + rOuter*math.Cos(a), Y: cy - rOuter*math.Sin(a)})
	}
	if rInner > 0 {
		for i := steps; i >= 0; i-- {
			a := rad(startDeg + sweepDeg*float64(i)/float64(steps))
			cont = append(cont, Point{X: cx + rInner*math.Cos(a), Y: cy - rInner*math.Sin(a)})
		}
	} else {
		cont = append(cont, Point{X: cx, Y: cy})
	}
	c.Fill([]Contour{cont}, col, alpha)
}

// FillRoundRect rasterises a rounded rectangle.
func (c *Canvas) FillRoundRect(x, y, w, h, r float64, col color.RGBA, alpha float64) {
	if w <= 0 || h <= 0 {
		return
	}
	r = math.Min(r, math.Min(w, h)/2)
	if r <= 0 {
		c.FillRect(x, y, w, h, col, alpha)
		return
	}
	const seg = 8
	cont := make(Contour, 0, seg*4+4)
	corner := func(cx, cy, from float64) {
		for i := 0; i <= seg; i++ {
			a := from + math.Pi/2*float64(i)/seg
			cont = append(cont, Point{X: cx + r*math.Cos(a), Y: cy + r*math.Sin(a)})
		}
	}
	corner(x+w-r, y+h-r, 0)         // bottom-right
	corner(x+r, y+h-r, math.Pi/2)   // bottom-left
	corner(x+r, y+r, math.Pi)       // top-left
	corner(x+w-r, y+r, 3*math.Pi/2) // top-right
	c.Fill([]Contour{cont}, col, alpha)
}

// DrawText renders a string with its baseline at (x, y). Returns the
// advance width so callers can chain runs.
func (c *Canvas) DrawText(f *Face, s string, size, x, y float64, col color.RGBA, alpha float64) float64 {
	if f == nil || s == "" {
		return 0
	}
	scale := size / float64(f.UnitsPerEm())
	pen := x
	for _, r := range s {
		gid := f.GlyphID(r)
		if gid == 0 && r != ' ' {
			if alt := f.GlyphID('?'); alt != 0 {
				gid = alt
			}
		}
		if outline := f.Outline(gid, scale); len(outline) > 0 {
			moved := make([]Contour, len(outline))
			for i, cont := range outline {
				mc := make(Contour, len(cont))
				for j, p := range cont {
					mc[j] = Point{X: p.X + pen, Y: p.Y + y}
				}
				moved[i] = mc
			}
			c.Fill(moved, col, alpha)
		}
		pen += float64(f.AdvanceUnits(gid)) * scale
	}
	return pen - x
}

// TextWidth measures a string without drawing it.
func (f *Face) TextWidth(s string, size float64) float64 {
	scale := size / float64(f.UnitsPerEm())
	w := 0.0
	for _, r := range s {
		w += float64(f.AdvanceUnits(f.GlyphID(r))) * scale
	}
	return w
}
