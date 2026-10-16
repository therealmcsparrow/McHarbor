// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package charts

import (
	"image"
	"image/color"
	"math"

	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-reporting/backend/fontkit"
)

// Fonts are the faces the rasteriser draws labels with.
type Fonts struct {
	Regular *fontkit.Face
	Bold    *fontkit.Face
}

// Measurer builds a Measure closure over these faces, so a layout is
// computed against exactly the metrics it will be drawn with.
func (f Fonts) Measurer() Measure {
	return func(text string, size float64, bold bool) float64 {
		face := f.Regular
		if bold && f.Bold != nil {
			face = f.Bold
		}
		if face == nil {
			// Rough fallback: half an em per character keeps a layout
			// sane if a face failed to load rather than collapsing it.
			return float64(len([]rune(text))) * size * 0.5
		}
		return face.TextWidth(text, size)
	}
}

// Render draws a layout into an image at `scale` device pixels per point.
//
// Word has no vector primitive we can emit without embedding a whole
// spreadsheet part, so the Word export carries charts as pictures. They
// are rendered from the same geometry the PDF draws, at 3–4× so they
// stay sharp when printed.
func Render(l Layout, f Fonts, scale float64, bg color.RGBA) *image.RGBA {
	if scale <= 0 {
		scale = 1
	}
	w := int(math.Ceil(l.W * scale))
	h := int(math.Ceil(l.H * scale))
	c := fontkit.NewCanvas(w, h, bg)

	for _, sh := range l.Shapes {
		drawShape(c, sh, scale)
	}
	for _, lb := range l.Labels {
		face := f.Regular
		if lb.Bold && f.Bold != nil {
			face = f.Bold
		}
		if face == nil {
			continue
		}
		x := lb.X * scale
		switch lb.Align {
		case AlignCenter:
			x -= face.TextWidth(lb.Text, lb.Size*scale) / 2
		case AlignRight:
			x -= face.TextWidth(lb.Text, lb.Size*scale)
		}
		c.DrawText(face, lb.Text, lb.Size*scale, x, lb.Y*scale, rgba(lb.Color), 1)
	}
	return c.Image()
}

func drawShape(c *fontkit.Canvas, sh Shape, s float64) {
	alpha := sh.Alpha
	if alpha == 0 {
		alpha = 1
	}
	switch sh.Kind {
	case ShapeRect:
		if sh.Fill != nil {
			c.FillRect(sh.X*s, sh.Y*s, sh.W*s, sh.H*s, rgba(*sh.Fill), alpha)
		}
	case ShapeRoundRect:
		if sh.Fill != nil {
			c.FillRoundRect(sh.X*s, sh.Y*s, sh.W*s, sh.H*s, sh.R*s, rgba(*sh.Fill), alpha)
		}
	case ShapeCircle:
		if sh.Fill != nil {
			c.FillCircle(sh.X*s, sh.Y*s, sh.R*s, rgba(*sh.Fill), alpha)
		}
	case ShapeArc:
		if sh.Fill != nil {
			c.FillArc(sh.X*s, sh.Y*s, sh.R*s, sh.RInner*s, sh.Start, sh.Sweep, rgba(*sh.Fill), alpha)
		}
	case ShapeLine:
		if sh.Stroke == nil || len(sh.Pts) < 2 {
			return
		}
		lw := math.Max(sh.LineWidth*s, 0.7)
		if sh.Dashed {
			drawDashed(c, sh.Pts[0], sh.Pts[1], s, lw, rgba(*sh.Stroke), alpha)
			return
		}
		c.StrokeLine(sh.Pts[0].X*s, sh.Pts[0].Y*s, sh.Pts[1].X*s, sh.Pts[1].Y*s, lw, rgba(*sh.Stroke), alpha)
	case ShapePath:
		if len(sh.Pts) < 2 {
			return
		}
		pts := make([]fontkit.Point, len(sh.Pts))
		for i, p := range sh.Pts {
			pts[i] = fontkit.Point{X: p.X * s, Y: p.Y * s}
		}
		if sh.Fill != nil {
			c.Fill([]fontkit.Contour{pts}, rgba(*sh.Fill), alpha)
		}
		if sh.Stroke != nil {
			c.StrokePath(pts, math.Max(sh.LineWidth*s, 0.7), rgba(*sh.Stroke), alpha)
		}
	}
}

// drawDashed renders a dashed reference line. The dash rhythm is fixed
// in device pixels so it looks the same whatever the scale factor.
func drawDashed(c *fontkit.Canvas, a, b Pt, s, lw float64, col color.RGBA, alpha float64) {
	dash, gap := 3.2*s, 2.4*s
	x1, y1, x2, y2 := a.X*s, a.Y*s, b.X*s, b.Y*s
	length := math.Hypot(x2-x1, y2-y1)
	if length <= 0 {
		return
	}
	ux, uy := (x2-x1)/length, (y2-y1)/length
	for d := 0.0; d < length; d += dash + gap {
		e := math.Min(d+dash, length)
		c.StrokeLine(x1+ux*d, y1+uy*d, x1+ux*e, y1+uy*e, lw, col, alpha)
	}
}

func rgba(c RGB) color.RGBA { return color.RGBA{R: c.R, G: c.G, B: c.B, A: 255} }
