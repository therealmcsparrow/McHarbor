// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package fontkit parses TrueType fonts far enough to do two things the
// Reporting plugin needs: embed a face in a PDF, and rasterise glyph
// outlines so chart labels can be drawn into the images the Word export
// carries.
//
// It is deliberately small. It reads the tables that describe metrics
// and outlines and nothing else — no hinting, no shaping, no kerning.
// Report copy is Latin, Greek or Cyrillic set at small sizes, where the
// difference is invisible; anything more would be a typesetting engine.
package fontkit

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrBadFont is returned for anything this package cannot make sense of.
var ErrBadFont = errors.New("fontkit: malformed TrueType font")

// Metrics are the design-space values a PDF font descriptor needs.
type Metrics struct {
	UnitsPerEm             uint16
	NumGlyphs              uint16
	XMin, YMin, XMax, YMax int16
	Ascender, Descender    int16
	CapHeight, XHeight     int16
	ItalicAngle            float64
	WeightClass            uint16
	IsSerif, IsItalic      bool
}

// Face is a parsed font.
type Face struct {
	raw    []byte
	m      Metrics
	tables map[string][]byte

	advances []uint16
	cmap     map[rune]uint16

	locaLong bool
	loca     []uint32
	glyf     []byte
}

// Parse reads a TrueType (glyf-outline) font.
func Parse(data []byte) (*Face, error) {
	if len(data) < 12 {
		return nil, ErrBadFont
	}
	tag := binary.BigEndian.Uint32(data[0:4])
	if tag != 0x00010000 && tag != 0x74727565 {
		return nil, fmt.Errorf("%w: unsupported sfnt version %#x", ErrBadFont, tag)
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 12+16*numTables {
		return nil, ErrBadFont
	}

	f := &Face{raw: data, tables: make(map[string][]byte, numTables), cmap: make(map[rune]uint16, 512)}
	for i := 0; i < numTables; i++ {
		rec := data[12+16*i : 12+16*i+16]
		name := string(rec[0:4])
		off := int(binary.BigEndian.Uint32(rec[8:12]))
		length := int(binary.BigEndian.Uint32(rec[12:16]))
		if off < 0 || off >= len(data) {
			continue
		}
		if length < 0 || off+length > len(data) {
			length = len(data) - off
		}
		f.tables[name] = data[off : off+length]
	}

	if err := f.readHead(); err != nil {
		return nil, err
	}
	if err := f.readMaxpHhea(); err != nil {
		return nil, err
	}
	f.readOS2Post()
	if err := f.readCmap(); err != nil {
		return nil, err
	}
	f.readLoca()
	return f, nil
}

func (f *Face) readHead() error {
	head := f.tables["head"]
	if len(head) < 54 {
		return fmt.Errorf("%w: head table too short", ErrBadFont)
	}
	f.m.UnitsPerEm = binary.BigEndian.Uint16(head[18:20])
	if f.m.UnitsPerEm == 0 {
		f.m.UnitsPerEm = 1000
	}
	f.m.XMin = i16(head[36:])
	f.m.YMin = i16(head[38:])
	f.m.XMax = i16(head[40:])
	f.m.YMax = i16(head[42:])
	f.m.IsItalic = binary.BigEndian.Uint16(head[44:46])&0x0002 != 0
	f.locaLong = binary.BigEndian.Uint16(head[50:52]) == 1
	return nil
}

func (f *Face) readMaxpHhea() error {
	maxp := f.tables["maxp"]
	if len(maxp) < 6 {
		return fmt.Errorf("%w: maxp table too short", ErrBadFont)
	}
	f.m.NumGlyphs = binary.BigEndian.Uint16(maxp[4:6])

	hhea := f.tables["hhea"]
	if len(hhea) < 36 {
		return fmt.Errorf("%w: hhea table too short", ErrBadFont)
	}
	f.m.Ascender = i16(hhea[4:])
	f.m.Descender = i16(hhea[6:])
	numHMetrics := int(binary.BigEndian.Uint16(hhea[34:36]))
	if numHMetrics <= 0 {
		return fmt.Errorf("%w: hhea declares no horizontal metrics", ErrBadFont)
	}

	hmtx := f.tables["hmtx"]
	f.advances = make([]uint16, f.m.NumGlyphs)
	last := uint16(0)
	for i := 0; i < int(f.m.NumGlyphs); i++ {
		if i < numHMetrics && 4*i+2 <= len(hmtx) {
			last = binary.BigEndian.Uint16(hmtx[4*i:])
		}
		// Glyphs past numHMetrics all share the last advance; only their
		// side bearings differ, and nothing here needs those.
		f.advances[i] = last
	}
	return nil
}

func (f *Face) readOS2Post() {
	if os2 := f.tables["OS/2"]; len(os2) >= 90 {
		f.m.WeightClass = binary.BigEndian.Uint16(os2[4:6])
		f.m.XHeight = i16(os2[86:])
		f.m.CapHeight = i16(os2[88:])
		f.m.IsItalic = f.m.IsItalic || binary.BigEndian.Uint16(os2[62:64])&0x0001 != 0
		// panose[0]==2 is Latin Text; serif style lives in panose[1].
		f.m.IsSerif = os2[32] == 2 && os2[33] >= 2 && os2[33] <= 10
	}
	if f.m.CapHeight == 0 {
		f.m.CapHeight = int16(float64(f.m.Ascender) * 0.72)
	}
	if post := f.tables["post"]; len(post) >= 8 {
		f.m.ItalicAngle = float64(int32(binary.BigEndian.Uint32(post[4:8]))) / 65536.0
	}
}

func (f *Face) readLoca() {
	loca := f.tables["loca"]
	f.glyf = f.tables["glyf"]
	if loca == nil || f.glyf == nil {
		return
	}
	n := int(f.m.NumGlyphs) + 1
	f.loca = make([]uint32, 0, n)
	if f.locaLong {
		for i := 0; i < n && 4*i+4 <= len(loca); i++ {
			f.loca = append(f.loca, binary.BigEndian.Uint32(loca[4*i:]))
		}
		return
	}
	for i := 0; i < n && 2*i+2 <= len(loca); i++ {
		f.loca = append(f.loca, uint32(binary.BigEndian.Uint16(loca[2*i:]))*2)
	}
}

// Raw is the original font file, for PDF FontFile2 embedding.
func (f *Face) Raw() []byte { return f.raw }

// Metrics returns the design-space metrics.
func (f *Face) Metrics() Metrics { return f.m }

// UnitsPerEm is the design grid size.
func (f *Face) UnitsPerEm() int { return int(f.m.UnitsPerEm) }

// NumGlyphs is the glyph count.
func (f *Face) NumGlyphs() int { return int(f.m.NumGlyphs) }

// GlyphID maps a rune to a glyph id, 0 when the font has no glyph.
func (f *Face) GlyphID(r rune) uint16 { return f.cmap[r] }

// HasGlyph reports whether the font covers r.
func (f *Face) HasGlyph(r rune) bool {
	gid, ok := f.cmap[r]
	return ok && gid != 0
}

// Advance1000 is the glyph advance in 1000-units-per-em space, which is
// what PDF widths use.
func (f *Face) Advance1000(gid uint16) int {
	if int(gid) >= len(f.advances) {
		return 0
	}
	return int(float64(f.advances[gid]) * 1000 / float64(f.m.UnitsPerEm))
}

// AdvanceUnits is the glyph advance in font units.
func (f *Face) AdvanceUnits(gid uint16) int {
	if int(gid) >= len(f.advances) {
		return 0
	}
	return int(f.advances[gid])
}

// Scale1000 converts a design-space value to 1000-units-per-em space.
func (f *Face) Scale1000(v int16) int {
	return int(float64(v) * 1000 / float64(f.m.UnitsPerEm))
}

func i16(b []byte) int16 { return int16(binary.BigEndian.Uint16(b)) }
