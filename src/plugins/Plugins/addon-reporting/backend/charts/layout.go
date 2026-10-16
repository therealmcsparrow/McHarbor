// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package charts

import "math"

// Build lays a chart out inside a w×h box.
func Build(s Spec, w, h float64, m Measure, pal Palette) Layout {
	l := &Layout{W: w, H: h}
	if w <= 0 || h <= 0 || len(s.Series) == 0 {
		return *l
	}
	switch s.Kind {
	case HBar:
		buildHBar(l, s, m, pal)
	case Donut:
		buildDonut(l, s, m, pal)
	case Line, Area:
		buildLine(l, s, m, pal, s.Kind == Area)
	case Gauge:
		buildGauge(l, s, m, pal)
	default:
		buildBar(l, s, m, pal, s.Kind == StackedBar)
	}
	return *l
}

func (l *Layout) add(sh Shape)  { l.Shapes = append(l.Shapes, sh) }
func (l *Layout) text(lb Label) { l.Labels = append(l.Labels, lb) }

// gridlines draws the horizontal reference lines and their value labels,
// and returns the plot rectangle left over.
func (l *Layout) gridlines(s Spec, m Measure, pal Palette, left, top, right, bottom float64, maxV float64) (x0, y0, x1, y1 float64) {
	const steps = 4
	hasTarget := s.HasTarget && maxV > 0 && s.Target <= maxV && s.Target >= 0

	// Reserve exactly the width the widest label needs. The target's
	// label lives in the same gutter as the axis values — inside the
	// plot it would sit under whichever bar happened to be tallest, and
	// there is no side of the plot that is reliably free.
	gutter := 0.0
	for i := 0; i <= steps; i++ {
		gutter = math.Max(gutter, m(s.format(maxV*float64(i)/steps), axisSize, false))
	}
	if hasTarget && s.TargetLabel != "" {
		gutter = math.Max(gutter, m(s.TargetLabel, axisSize, true))
	}
	x0 = left + gutter + 6
	x1 = l.W - right
	y0 = top
	y1 = l.H - bottom

	targetY := math.Inf(1)
	if hasTarget {
		targetY = y1 - (y1-y0)*(s.Target/maxV)
	}

	for i := 0; i <= steps; i++ {
		v := maxV * float64(i) / steps
		y := y1 - (y1-y0)*float64(i)/steps
		// The zero line is the baseline and reads darker; the rest are
		// scaffolding and should recede.
		col := pal.Hairline
		alpha := 0.7
		if i == 0 {
			col = pal.Muted
			alpha = 0.45
		}
		l.add(Shape{Kind: ShapeLine, Pts: []Pt{{x0, y}, {x1, y}}, Stroke: rgbPtr(col), LineWidth: 0.5, Alpha: alpha})
		// An SLO of 99.5 against an axis topping out at 100 puts two
		// labels in the same few points of gutter. The target is the one
		// worth reading, so the axis value yields.
		if math.Abs(y-targetY) < axisSize+2 {
			continue
		}
		l.text(Label{Text: s.format(v), X: x0 - 6, Y: y + axisSize*0.35, Size: axisSize, Color: pal.Muted, Align: AlignRight})
	}

	if hasTarget {
		l.add(Shape{Kind: ShapeLine, Pts: []Pt{{x0, targetY}, {x1, targetY}},
			Stroke: rgbPtr(pal.Ink), LineWidth: 0.8, Alpha: 0.55, Dashed: true})
		if s.TargetLabel != "" {
			l.text(Label{Text: s.TargetLabel, X: x0 - 6, Y: targetY + axisSize*0.35,
				Size: axisSize, Color: pal.Ink, Align: AlignRight, Bold: true})
		}
	}
	return x0, y0, x1, y1
}

// legend renders a single row of swatch+name pairs across the bottom and
// returns the height it consumed.
func (l *Layout) legend(s Spec, m Measure, pal Palette, yTop float64) float64 {
	if !s.ShowLegend || len(s.Series) == 0 {
		return 0
	}
	const sw = 7.0
	total := 0.0
	for _, ser := range s.Series {
		total += sw + 4 + m(ser.Name, legendSize, false) + 14
	}
	total -= 14
	x := math.Max(0, (l.W-total)/2)
	for i, ser := range s.Series {
		c := pal.series(i, ser.Color)
		l.add(Shape{Kind: ShapeRoundRect, X: x, Y: yTop, W: sw, H: sw, R: 1.6, Fill: rgbPtr(c), Alpha: 1})
		l.text(Label{Text: ser.Name, X: x + sw + 4, Y: yTop + sw - 0.6, Size: legendSize, Color: pal.Muted})
		x += sw + 4 + m(ser.Name, legendSize, false) + 14
	}
	return sw + 8
}

func buildBar(l *Layout, s Spec, m Measure, pal Palette, stacked bool) {
	cats := s.categories()
	if len(cats) == 0 {
		return
	}
	maxV := s.maxValue()

	legendH := 0.0
	if s.ShowLegend && len(s.Series) > 1 {
		legendH = legendSize + 10
	}
	bottom := catSize + 8 + legendH
	x0, y0, x1, y1 := l.gridlines(s, m, pal, 0, valueSize+4, 4, bottom, maxV)

	plotW := x1 - x0
	slot := plotW / float64(len(cats))
	// A group occupies 68 % of its slot; the rest is the gutter that
	// lets the eye separate categories without a divider line.
	groupW := slot * 0.68
	nSeries := len(s.Series)
	barW := groupW
	if !stacked && nSeries > 1 {
		barW = groupW / float64(nSeries)
	}

	for ci, cat := range cats {
		cx := x0 + slot*float64(ci) + slot/2
		stackTop := y1
		for si, ser := range s.Series {
			if ci >= len(ser.Points) {
				continue
			}
			v := ser.Points[ci].Value
			if v < 0 {
				v = 0
			}
			hpx := 0.0
			if maxV > 0 {
				hpx = (y1 - y0) * (v / maxV)
			}
			c := pal.series(si, ser.Color)

			var bx, by float64
			if stacked {
				bx = cx - groupW/2
				by = stackTop - hpx
				stackTop = by
				barW = groupW
			} else {
				bx = cx - groupW/2 + barW*float64(si)
				by = y1 - hpx
			}
			if hpx < 0.6 && v > 0 {
				// Keep a non-zero value visible rather than letting it
				// round away to nothing.
				hpx = 0.6
				by = y1 - hpx
			}
			radius := math.Min(2, barW/3)
			if stacked {
				radius = 0
			}
			l.add(Shape{Kind: ShapeRoundRect, X: bx, Y: by, W: math.Max(barW-1, 1), H: hpx, R: radius, Fill: rgbPtr(c), Alpha: 1})

			if s.ShowValues && !stacked && v > 0 {
				l.text(Label{Text: s.format(v), X: bx + (barW-1)/2, Y: by - 2.5,
					Size: valueSize, Color: pal.Muted, Align: AlignCenter})
			}
		}
		if stacked && s.ShowValues {
			sum := 0.0
			for _, ser := range s.Series {
				if ci < len(ser.Points) {
					sum += ser.Points[ci].Value
				}
			}
			if sum > 0 {
				l.text(Label{Text: s.format(sum), X: cx, Y: stackTop - 2.5,
					Size: valueSize, Color: pal.Muted, Align: AlignCenter})
			}
		}
		l.text(Label{Text: fit(cat, m, catSize, slot-2), X: cx, Y: y1 + catSize + 4,
			Size: catSize, Color: pal.Muted, Align: AlignCenter})
	}

	if legendH > 0 {
		l.legend(s, m, pal, l.H-legendH+2)
	}
}

func buildHBar(l *Layout, s Spec, m Measure, pal Palette) {
	if len(s.Series) == 0 || len(s.Series[0].Points) == 0 {
		return
	}
	pts := s.Series[0].Points
	maxV := s.maxValue()

	// Reserve the label gutter from the actual labels, capped so one
	// runaway name cannot squeeze every bar to nothing.
	labelW := 0.0
	for _, p := range pts {
		labelW = math.Max(labelW, m(p.Label, catSize, false))
	}
	labelW = math.Min(labelW, l.W*0.42)

	valueW := 0.0
	if s.ShowValues {
		for _, p := range pts {
			valueW = math.Max(valueW, m(s.format(p.Value), valueSize, true))
		}
		valueW += 6
	}

	x0 := labelW + 8
	x1 := l.W - valueW
	rowH := l.H / float64(len(pts))
	barH := math.Min(rowH*0.6, 14)

	for i, p := range pts {
		cy := rowH*float64(i) + rowH/2
		w := 0.0
		if maxV > 0 {
			w = (x1 - x0) * (p.Value / maxV)
		}
		if w < 0 {
			w = 0
		}
		// One colour for the whole series unless a point carries its own
		// status. Cycling the ramp down a "top offenders" list implies a
		// categorical difference between rows that does not exist.
		c := pal.series(0, p.Color, s.Series[0].Color)
		// Track first, so short bars still read as "out of this much".
		l.add(Shape{Kind: ShapeRoundRect, X: x0, Y: cy - barH/2, W: x1 - x0, H: barH, R: barH / 2,
			Fill: rgbPtr(pal.Track), Alpha: 1})
		if w > 0 {
			l.add(Shape{Kind: ShapeRoundRect, X: x0, Y: cy - barH/2, W: math.Max(w, barH*0.6), H: barH, R: barH / 2,
				Fill: rgbPtr(c), Alpha: 1})
		}
		l.text(Label{Text: fit(p.Label, m, catSize, labelW), X: labelW, Y: cy + catSize*0.35,
			Size: catSize, Color: pal.Ink, Align: AlignRight})
		if s.ShowValues {
			l.text(Label{Text: s.format(p.Value), X: x1 + 4, Y: cy + valueSize*0.35,
				Size: valueSize, Color: pal.Muted, Bold: true})
		}
	}
}

func buildDonut(l *Layout, s Spec, m Measure, pal Palette) {
	if len(s.Series) == 0 || len(s.Series[0].Points) == 0 {
		return
	}
	pts := s.Series[0].Points
	total := 0.0
	for _, p := range pts {
		if p.Value > 0 {
			total += p.Value
		}
	}

	// The legend is the label: a donut without one is a decoration.
	// The label column is measured directly and the value/percentage
	// columns added to it — deriving the label width back out of the
	// total lost a point or two to rounding and clipped short names.
	labelW := 0.0
	for _, p := range pts {
		labelW = math.Max(labelW, m(p.Label, legendSize, false))
	}
	const legendValueW = 66.0
	labelW = math.Min(labelW, l.W*0.34)
	legendW := math.Min(labelW+legendValueW, l.W*0.58)

	dia := math.Min(l.H, l.W-legendW-10)
	if dia < 20 {
		dia = math.Min(l.H, l.W)
		legendW = 0
	}
	cx := dia/2 + 2
	cy := l.H / 2
	rOuter := dia / 2
	rInner := rOuter * 0.62

	if total <= 0 {
		l.add(Shape{Kind: ShapeArc, X: cx, Y: cy, R: rOuter, RInner: rInner, Start: 0, Sweep: 359.9,
			Fill: rgbPtr(pal.Track), Alpha: 1})
	} else {
		angle := 0.0
		for i, p := range pts {
			if p.Value <= 0 {
				continue
			}
			sweep := 360 * p.Value / total
			l.add(Shape{Kind: ShapeArc, X: cx, Y: cy, R: rOuter, RInner: rInner,
				Start: angle, Sweep: sweep, Fill: rgbPtr(pal.series(i, p.Color)), Alpha: 1})
			angle += sweep
		}
	}

	// Total in the hole — the one number a reader wants before the split.
	l.text(Label{Text: DefaultFormat(total), X: cx, Y: cy + 1, Size: math.Min(rInner*0.62, 15),
		Color: pal.Ink, Align: AlignCenter, Bold: true})
	if s.Unit != "" {
		l.text(Label{Text: s.Unit, X: cx, Y: cy + 11, Size: axisSize, Color: pal.Muted, Align: AlignCenter})
	}

	if legendW <= 0 {
		return
	}
	lx := dia + 12
	rowH := math.Min(15, l.H/float64(maxInt(len(pts), 1)))
	startY := (l.H - rowH*float64(len(pts))) / 2
	for i, p := range pts {
		y := startY + rowH*float64(i)
		c := pal.series(i, p.Color)
		l.add(Shape{Kind: ShapeRoundRect, X: lx, Y: y + rowH/2 - 3.5, W: 7, H: 7, R: 1.6, Fill: rgbPtr(c), Alpha: 1})
		l.text(Label{Text: fit(p.Label, m, legendSize, labelW), X: lx + 11, Y: y + rowH/2 + 2.4,
			Size: legendSize, Color: pal.Ink})
		pct := 0.0
		if total > 0 {
			pct = 100 * p.Value / total
		}
		l.text(Label{Text: DefaultFormat(p.Value), X: l.W - 30, Y: y + rowH/2 + 2.4,
			Size: legendSize, Color: pal.Ink, Align: AlignRight, Bold: true})
		l.text(Label{Text: DefaultFormat(pct) + "%", X: l.W, Y: y + rowH/2 + 2.4,
			Size: axisSize, Color: pal.Muted, Align: AlignRight})
	}
}

func buildLine(l *Layout, s Spec, m Measure, pal Palette, filled bool) {
	cats := s.categories()
	if len(cats) < 2 {
		return
	}
	maxV := s.maxValue()

	legendH := 0.0
	if s.ShowLegend && len(s.Series) > 1 {
		legendH = legendSize + 10
	}
	x0, y0, x1, y1 := l.gridlines(s, m, pal, 0, 6, 6, catSize+8+legendH, maxV)

	step := (x1 - x0) / float64(len(cats)-1)
	for si, ser := range s.Series {
		c := pal.series(si, ser.Color)
		pts := make([]Pt, 0, len(ser.Points))
		for i, p := range ser.Points {
			v := p.Value
			if v < 0 {
				v = 0
			}
			y := y1
			if maxV > 0 {
				y = y1 - (y1-y0)*(v/maxV)
			}
			pts = append(pts, Pt{X: x0 + step*float64(i), Y: y})
		}
		if len(pts) < 2 {
			continue
		}
		if filled {
			area := append([]Pt{}, pts...)
			area = append(area, Pt{X: pts[len(pts)-1].X, Y: y1}, Pt{X: pts[0].X, Y: y1})
			l.add(Shape{Kind: ShapePath, Pts: area, Fill: rgbPtr(c), Alpha: 0.14})
		}
		l.add(Shape{Kind: ShapePath, Pts: pts, Stroke: rgbPtr(c), LineWidth: 1.5, Alpha: 1})
		// Dots only when they can be told apart; on a dense series they
		// turn the line into a caterpillar.
		if len(pts) <= 14 {
			for _, p := range pts {
				l.add(Shape{Kind: ShapeCircle, X: p.X, Y: p.Y, R: 2.1, Fill: rgbPtr(c), Alpha: 1})
			}
		}
	}

	// Label the ends and the middle — a dated axis under a trend line
	// rarely needs more, and more would collide.
	idx := []int{0, len(cats) / 2, len(cats) - 1}
	align := []Align{AlignLeft, AlignCenter, AlignRight}
	seen := map[int]bool{}
	for k, i := range idx {
		if seen[i] || i < 0 || i >= len(cats) {
			continue
		}
		seen[i] = true
		l.text(Label{Text: cats[i], X: x0 + step*float64(i), Y: y1 + catSize + 4,
			Size: catSize, Color: pal.Muted, Align: align[k]})
	}
	if legendH > 0 {
		l.legend(s, m, pal, l.H-legendH+2)
	}
}

func buildGauge(l *Layout, s Spec, m Measure, pal Palette) {
	if len(s.Series) == 0 || len(s.Series[0].Points) == 0 {
		return
	}
	p := s.Series[0].Points[0]
	maxV := s.Max
	if maxV <= 0 {
		maxV = 100
	}
	frac := math.Max(0, math.Min(1, p.Value/maxV))

	// A 240° sweep reads as a dial, with the gap at the bottom where a
	// dial's gap belongs. Angles run clockwise from twelve o'clock, so
	// starting at 240° and sweeping 240° ends at 120° — leaving 120°
	// open across the bottom.
	const sweepTotal = 240.0
	const start = 240.0

	// The drawn shape spans r above the centre and r·cos(60°) below it,
	// so it needs 1.5·r of height, not 2·r.
	r := math.Min(l.W/2-2, l.H/1.55)
	cx := l.W / 2
	cy := r + 2
	thickness := math.Max(6, r*0.2)

	l.add(Shape{Kind: ShapeArc, X: cx, Y: cy, R: r, RInner: r - thickness,
		Start: start, Sweep: sweepTotal, Fill: rgbPtr(pal.Track), Alpha: 1})
	c := pal.series(0, s.Series[0].Color)
	if frac > 0 {
		l.add(Shape{Kind: ShapeArc, X: cx, Y: cy, R: r, RInner: r - thickness,
			Start: start, Sweep: sweepTotal * frac, Fill: rgbPtr(c), Alpha: 1})
	}

	l.text(Label{Text: DefaultFormat(p.Value), X: cx, Y: cy + r*0.14,
		Size: r * 0.6, Color: pal.Ink, Align: AlignCenter, Bold: true})
	if p.Label != "" {
		l.text(Label{Text: p.Label, X: cx, Y: cy + r*0.52,
			Size: legendSize, Color: pal.Muted, Align: AlignCenter})
	}
}

// fit truncates a label to the space available, with an ellipsis.
func fit(s string, m Measure, size, maxW float64) string {
	if maxW <= 0 || m(s, size, false) <= maxW {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		if m(string(runes)+"…", size, false) <= maxW {
			return string(runes) + "…"
		}
	}
	return "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
