// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fontkit

import "encoding/binary"

// Point is a coordinate in whatever space the caller asked for.
type Point struct{ X, Y float64 }

// Contour is a closed polygon: quadratic curves are already flattened.
type Contour []Point

// Simple-glyph flag bits.
const (
	flagOnCurve = 1 << 0
	flagXShort  = 1 << 1
	flagYShort  = 1 << 2
	flagRepeat  = 1 << 3
	flagXSame   = 1 << 4 // also "x is positive" when XShort
	flagYSame   = 1 << 5 // also "y is positive" when YShort
)

// Composite-glyph flag bits.
const (
	cArgsAreWords   = 1 << 0
	cArgsAreXY      = 1 << 1
	cHaveScale      = 1 << 3
	cMoreComponents = 1 << 5
	cHaveXYScale    = 1 << 6
	cHave2x2        = 1 << 7
)

// maxCompositeDepth stops a malformed font recursing forever.
const maxCompositeDepth = 5

// Outline returns the glyph's contours scaled by `scale` (font units are
// multiplied by it) with y already flipped so it grows downward, which
// is what a raster target wants. The origin is the glyph's baseline
// origin.
func (f *Face) Outline(gid uint16, scale float64) []Contour {
	raw := f.outlineUnits(gid, 0)
	if len(raw) == 0 {
		return nil
	}
	out := make([]Contour, 0, len(raw))
	for _, c := range raw {
		if len(c) < 3 {
			continue
		}
		s := make(Contour, len(c))
		for i, p := range c {
			s[i] = Point{X: p.X * scale, Y: -p.Y * scale}
		}
		out = append(out, s)
	}
	return out
}

func (f *Face) outlineUnits(gid uint16, depth int) []Contour {
	if depth > maxCompositeDepth || f.glyf == nil || int(gid)+1 >= len(f.loca) {
		return nil
	}
	start, end := f.loca[gid], f.loca[gid+1]
	if end <= start || int(end) > len(f.glyf) {
		return nil // empty glyph, e.g. space
	}
	g := f.glyf[start:end]
	if len(g) < 10 {
		return nil
	}
	numContours := int(i16(g[0:]))
	if numContours < 0 {
		return f.compositeOutline(g[10:], depth)
	}
	return simpleOutline(g[10:], numContours)
}

func simpleOutline(g []byte, numContours int) []Contour {
	if numContours == 0 || len(g) < 2*numContours+2 {
		return nil
	}
	ends := make([]int, numContours)
	for i := 0; i < numContours; i++ {
		ends[i] = int(binary.BigEndian.Uint16(g[2*i:]))
	}
	numPoints := ends[numContours-1] + 1
	if numPoints <= 0 || numPoints > 10000 {
		return nil
	}

	p := 2 * numContours
	if p+2 > len(g) {
		return nil
	}
	instrLen := int(binary.BigEndian.Uint16(g[p:]))
	p += 2 + instrLen
	if p > len(g) {
		return nil
	}

	flags := make([]byte, 0, numPoints)
	for len(flags) < numPoints && p < len(g) {
		fl := g[p]
		p++
		flags = append(flags, fl)
		if fl&flagRepeat != 0 && p < len(g) {
			n := int(g[p])
			p++
			for i := 0; i < n && len(flags) < numPoints; i++ {
				flags = append(flags, fl)
			}
		}
	}
	if len(flags) < numPoints {
		return nil
	}

	xs := make([]float64, numPoints)
	x := 0
	for i := 0; i < numPoints; i++ {
		fl := flags[i]
		switch {
		case fl&flagXShort != 0:
			if p >= len(g) {
				return nil
			}
			d := int(g[p])
			p++
			if fl&flagXSame == 0 {
				d = -d
			}
			x += d
		case fl&flagXSame == 0:
			if p+2 > len(g) {
				return nil
			}
			x += int(i16(g[p:]))
			p += 2
		}
		xs[i] = float64(x)
	}

	ys := make([]float64, numPoints)
	y := 0
	for i := 0; i < numPoints; i++ {
		fl := flags[i]
		switch {
		case fl&flagYShort != 0:
			if p >= len(g) {
				return nil
			}
			d := int(g[p])
			p++
			if fl&flagYSame == 0 {
				d = -d
			}
			y += d
		case fl&flagYSame == 0:
			if p+2 > len(g) {
				return nil
			}
			y += int(i16(g[p:]))
			p += 2
		}
		ys[i] = float64(y)
	}

	var out []Contour
	first := 0
	for _, last := range ends {
		if last < first || last >= numPoints {
			break
		}
		out = append(out, flattenContour(xs[first:last+1], ys[first:last+1], flags[first:last+1]))
		first = last + 1
	}
	return out
}

// flattenContour turns one TrueType contour into a polygon.
//
// TrueType stores quadratic B-splines where consecutive off-curve points
// imply an on-curve point at their midpoint. Rebuilding those implied
// points first makes the rest a plain "on, control, on" walk.
func flattenContour(xs, ys []float64, flags []byte) Contour {
	n := len(xs)
	if n == 0 {
		return nil
	}
	type pt struct {
		p  Point
		on bool
	}
	pts := make([]pt, 0, n*2)
	for i := 0; i < n; i++ {
		cur := pt{Point{xs[i], ys[i]}, flags[i]&flagOnCurve != 0}
		if len(pts) > 0 && !pts[len(pts)-1].on && !cur.on {
			prev := pts[len(pts)-1].p
			pts = append(pts, pt{Point{(prev.X + cur.p.X) / 2, (prev.Y + cur.p.Y) / 2}, true})
		}
		pts = append(pts, cur)
	}
	if len(pts) == 0 {
		return nil
	}
	// Rotate so the contour starts on-curve. A contour that is entirely
	// off-curve (a pure oval) gets a synthetic start at the midpoint.
	startIdx := -1
	for i, p := range pts {
		if p.on {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		mid := Point{(pts[0].p.X + pts[len(pts)-1].p.X) / 2, (pts[0].p.Y + pts[len(pts)-1].p.Y) / 2}
		pts = append([]pt{{mid, true}}, pts...)
		startIdx = 0
	}
	pts = append(pts[startIdx:], pts[:startIdx]...)

	out := Contour{pts[0].p}
	i := 1
	for i <= len(pts) {
		var cur pt
		if i == len(pts) {
			cur = pts[0]
		} else {
			cur = pts[i]
		}
		if cur.on {
			out = append(out, cur.p)
			i++
			continue
		}
		// Off-curve control; the next point (wrapping) is the end point.
		var next pt
		if i+1 >= len(pts) {
			next = pts[0]
		} else {
			next = pts[i+1]
		}
		out = appendQuad(out, out[len(out)-1], cur.p, next.p)
		i += 2
	}
	return out
}

// appendQuad flattens a quadratic Bézier. The step count is derived from
// the control polygon's size in font units; at 2048 upem, 1/8 of an em
// per segment is well below a pixel at any size a report uses.
func appendQuad(dst Contour, p0, c, p1 Point) Contour {
	d := abs(c.X-p0.X) + abs(c.Y-p0.Y) + abs(p1.X-c.X) + abs(p1.Y-c.Y)
	steps := int(d / 40)
	if steps < 3 {
		steps = 3
	}
	if steps > 24 {
		steps = 24
	}
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		mt := 1 - t
		dst = append(dst, Point{
			X: mt*mt*p0.X + 2*mt*t*c.X + t*t*p1.X,
			Y: mt*mt*p0.Y + 2*mt*t*c.Y + t*t*p1.Y,
		})
	}
	return dst
}

// compositeOutline assembles a glyph built from other glyphs — how
// accented Latin, and much of Cyrillic, are actually stored.
func (f *Face) compositeOutline(g []byte, depth int) []Contour {
	var out []Contour
	p := 0
	for {
		if p+4 > len(g) {
			break
		}
		flags := binary.BigEndian.Uint16(g[p:])
		glyphIndex := binary.BigEndian.Uint16(g[p+2:])
		p += 4

		var dx, dy float64
		if flags&cArgsAreWords != 0 {
			if p+4 > len(g) {
				break
			}
			a1, a2 := i16(g[p:]), i16(g[p+2:])
			p += 4
			if flags&cArgsAreXY != 0 {
				dx, dy = float64(a1), float64(a2)
			}
		} else {
			if p+2 > len(g) {
				break
			}
			a1, a2 := int8(g[p]), int8(g[p+1])
			p += 2
			if flags&cArgsAreXY != 0 {
				dx, dy = float64(a1), float64(a2)
			}
		}

		a, b, c, d := 1.0, 0.0, 0.0, 1.0
		switch {
		case flags&cHaveScale != 0:
			if p+2 > len(g) {
				return out
			}
			a = f2dot14(g[p:])
			d = a
			p += 2
		case flags&cHaveXYScale != 0:
			if p+4 > len(g) {
				return out
			}
			a, d = f2dot14(g[p:]), f2dot14(g[p+2:])
			p += 4
		case flags&cHave2x2 != 0:
			if p+8 > len(g) {
				return out
			}
			a, b, c, d = f2dot14(g[p:]), f2dot14(g[p+2:]), f2dot14(g[p+4:]), f2dot14(g[p+6:])
			p += 8
		}

		for _, cont := range f.outlineUnits(glyphIndex, depth+1) {
			moved := make(Contour, len(cont))
			for i, pt := range cont {
				moved[i] = Point{
					X: a*pt.X + c*pt.Y + dx,
					Y: b*pt.X + d*pt.Y + dy,
				}
			}
			out = append(out, moved)
		}
		if flags&cMoreComponents == 0 {
			break
		}
	}
	return out
}

func f2dot14(b []byte) float64 { return float64(i16(b)) / 16384.0 }

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
