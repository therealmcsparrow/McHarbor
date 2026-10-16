// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pdf

import (
	"sort"
	"strings"
	"unicode"

	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-reporting/backend/fontkit"
)

// Font is an embedded TrueType face.
//
// Text is written with Identity-H encoding: a show-text operand is a hex
// string of 16-bit glyph ids rather than character codes. That sidesteps
// the 8-bit encoding tables entirely, so Latin-Extended, Greek and
// Cyrillic all work from one font with no per-page encoding juggling,
// and a ToUnicode CMap keeps the result selectable and searchable in a
// viewer.
type Font struct {
	Name string // PostScript-ish base name, e.g. "InterTight-SemiBold"

	face *fontkit.Face
	m    fontkit.Metrics

	// used records every glyph actually drawn so the ToUnicode CMap and
	// the /W widths array only carry what the document needs.
	used map[uint16]rune

	// refID names the font inside page content streams (`/F1`, `/F2`).
	// Content is generated long before object numbers are handed out, so
	// the resource name has to be stable from registration time; the
	// page's /Font dictionary maps it to the object at write time.
	refID  int
	objNum int // assigned at write time
}

// Missing glyphs fall back to the replacement mark rather than silently
// disappearing: a blank in a board document is worse than a mark that
// says the font has no glyph for that character.
const fallbackRune = '�'

func newFont(name string, data []byte) (*Font, error) {
	face, err := fontkit.Parse(data)
	if err != nil {
		return nil, err
	}
	return &Font{Name: name, face: face, m: face.Metrics(), used: make(map[uint16]rune, 256)}, nil
}

// Face exposes the parsed font, so the chart rasteriser can draw with
// exactly the face the PDF embeds.
func (fo *Font) Face() *fontkit.Face { return fo.face }

// UnitsPerEm exposes the design grid.
func (fo *Font) UnitsPerEm() int { return fo.face.UnitsPerEm() }

// Ascent, Descent and CapHeight are in points for the given size.
func (fo *Font) Ascent(size float64) float64 {
	return float64(fo.face.Scale1000(fo.m.Ascender)) * size / 1000
}

func (fo *Font) Descent(size float64) float64 {
	return float64(fo.face.Scale1000(fo.m.Descender)) * size / 1000
}

func (fo *Font) CapHeight(size float64) float64 {
	return float64(fo.face.Scale1000(fo.m.CapHeight)) * size / 1000
}

// LineHeight is the font's natural leading at `size`, before any
// document-level line-spacing multiplier.
func (fo *Font) LineHeight(size float64) float64 {
	return fo.Ascent(size) - fo.Descent(size)
}

func (fo *Font) glyph(r rune) uint16 {
	if gid := fo.face.GlyphID(r); gid != 0 {
		return gid
	}
	// Normalise a few characters reports routinely carry that a text
	// face may not cover: the non-breaking and narrow spaces, and the
	// typographic minus.
	switch r {
	case ' ', ' ', ' ', ' ':
		if gid := fo.face.GlyphID(' '); gid != 0 {
			return gid
		}
	case '−':
		if gid := fo.face.GlyphID('-'); gid != 0 {
			return gid
		}
	}
	if gid := fo.face.GlyphID(fallbackRune); gid != 0 {
		return gid
	}
	return fo.face.GlyphID('?')
}

// Width measures a string at `size` points.
func (fo *Font) Width(s string, size float64) float64 {
	total := 0
	for _, r := range s {
		total += fo.face.Advance1000(fo.glyph(r))
	}
	return float64(total) * size / 1000
}

// encode turns a string into the hex glyph-id operand for Tj, recording
// each glyph for the ToUnicode CMap.
func (fo *Font) encode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(s) * 4)
	for _, r := range s {
		gid := fo.glyph(r)
		if _, seen := fo.used[gid]; !seen {
			fo.used[gid] = r
		}
		b.WriteByte(hex[(gid>>12)&0xF])
		b.WriteByte(hex[(gid>>8)&0xF])
		b.WriteByte(hex[(gid>>4)&0xF])
		b.WriteByte(hex[gid&0xF])
	}
	return b.String()
}

// Wrap breaks text into lines that fit `maxW`, honouring existing
// newlines. Words longer than the line (a container id, a digest) are
// hard-split rather than allowed to bleed into the margin.
func (fo *Font) Wrap(s string, size, maxW float64) []string {
	if maxW <= 0 {
		return []string{s}
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := ""
		for _, w := range words {
			cand := w
			if line != "" {
				cand = line + " " + w
			}
			if fo.Width(cand, size) <= maxW {
				line = cand
				continue
			}
			if line != "" {
				out = append(out, line)
				line = ""
			}
			if fo.Width(w, size) <= maxW {
				line = w
				continue
			}
			for _, chunk := range fo.hardSplit(w, size, maxW) {
				if fo.Width(chunk, size) <= maxW {
					line = chunk
				} else {
					out = append(out, chunk)
				}
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

func (fo *Font) hardSplit(w string, size, maxW float64) []string {
	var parts []string
	cur := ""
	for _, r := range w {
		next := cur + string(r)
		if fo.Width(next, size) > maxW && cur != "" {
			parts = append(parts, cur)
			cur = string(r)
			continue
		}
		cur = next
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	return parts
}

// Truncate shortens `s` to fit `maxW`, appending an ellipsis. Table
// cells use this: wrapping every long value would make a dense table
// unreadable, and a report table is scanned, not read.
func (fo *Font) Truncate(s string, size, maxW float64) string {
	if fo.Width(s, size) <= maxW {
		return s
	}
	const ell = "…"
	ellW := fo.Width(ell, size)
	if ellW > maxW {
		return ""
	}
	runes := []rune(s)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		for len(runes) > 0 && unicode.IsSpace(runes[len(runes)-1]) {
			runes = runes[:len(runes)-1]
		}
		if fo.Width(string(runes), size)+ellW <= maxW {
			return string(runes) + ell
		}
	}
	return ell
}

// sortedGlyphs returns the used glyph ids in ascending order, which both
// the /W array and the ToUnicode CMap need.
func (fo *Font) sortedGlyphs() []uint16 {
	ids := make([]uint16, 0, len(fo.used))
	for gid := range fo.used {
		ids = append(ids, gid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// flags builds the PDF font descriptor flag word.
func (fo *Font) flags() int {
	const (
		serif     = 1 << 1
		nonSymbol = 1 << 5
		italic    = 1 << 6
	)
	// Identity-H fonts are conventionally marked symbolic, but marking a
	// text face non-symbolic keeps viewers using standard Latin
	// substitution if the file is ever opened where the font is stripped.
	v := nonSymbol
	if fo.m.IsSerif {
		v |= serif
	}
	if fo.m.IsItalic {
		v |= italic
	}
	return v
}

// stemV is estimated from weight class. PDF requires the key and no
// viewer reads it precisely enough to justify measuring the outlines.
func (fo *Font) stemV() int {
	w := int(fo.m.WeightClass)
	if w == 0 {
		w = 400
	}
	return 50 + (w*w)/6250
}
