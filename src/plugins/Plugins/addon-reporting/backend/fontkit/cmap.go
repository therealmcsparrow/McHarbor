// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fontkit

import (
	"encoding/binary"
	"fmt"
)

func (f *Face) readCmap() error {
	cmap := f.tables["cmap"]
	if len(cmap) < 4 {
		return fmt.Errorf("%w: missing cmap", ErrBadFont)
	}
	n := int(binary.BigEndian.Uint16(cmap[2:4]))
	if len(cmap) < 4+8*n {
		return fmt.Errorf("%w: cmap directory truncated", ErrBadFont)
	}

	// Preference order: Windows UCS-4, Windows BMP, Unicode, anything.
	bestRank, bestOff := 1<<30, -1
	for i := 0; i < n; i++ {
		rec := cmap[4+8*i : 4+8*i+8]
		plat := binary.BigEndian.Uint16(rec[0:2])
		enc := binary.BigEndian.Uint16(rec[2:4])
		off := int(binary.BigEndian.Uint32(rec[4:8]))
		if off >= len(cmap) {
			continue
		}
		rank := 40
		switch {
		case plat == 3 && enc == 10:
			rank = 0
		case plat == 3 && enc == 1:
			rank = 10
		case plat == 0:
			rank = 20
		}
		if rank < bestRank {
			bestRank, bestOff = rank, off
		}
	}
	if bestOff < 0 {
		return fmt.Errorf("%w: no usable cmap subtable", ErrBadFont)
	}

	t := cmap[bestOff:]
	if len(t) < 4 {
		return fmt.Errorf("%w: cmap subtable truncated", ErrBadFont)
	}
	switch binary.BigEndian.Uint16(t[0:2]) {
	case 4:
		return f.cmap4(t)
	case 6:
		return f.cmap6(t)
	case 12:
		return f.cmap12(t)
	default:
		return fmt.Errorf("%w: unsupported cmap format", ErrBadFont)
	}
}

func (f *Face) cmap4(t []byte) error {
	if len(t) < 14 {
		return ErrBadFont
	}
	segX2 := int(binary.BigEndian.Uint16(t[6:8]))
	seg := segX2 / 2
	if seg == 0 || len(t) < 16+segX2*4 {
		return ErrBadFont
	}
	endAt, startAt := 14, 16+segX2
	deltaAt, rangeAt := startAt+segX2, startAt+segX2*2

	for i := 0; i < seg; i++ {
		end := binary.BigEndian.Uint16(t[endAt+2*i:])
		start := binary.BigEndian.Uint16(t[startAt+2*i:])
		delta := binary.BigEndian.Uint16(t[deltaAt+2*i:])
		ro := binary.BigEndian.Uint16(t[rangeAt+2*i:])
		if start > end {
			continue
		}
		for c := uint32(start); c <= uint32(end) && c <= 0xFFFF; c++ {
			var gid uint16
			if ro == 0 {
				gid = uint16(c) + delta
			} else {
				idx := rangeAt + 2*i + int(ro) + 2*int(c-uint32(start))
				if idx+2 > len(t) {
					continue
				}
				gid = binary.BigEndian.Uint16(t[idx:])
				if gid != 0 {
					gid += delta
				}
			}
			if gid != 0 && gid < f.m.NumGlyphs {
				f.cmap[rune(c)] = gid
			}
		}
	}
	return nil
}

func (f *Face) cmap6(t []byte) error {
	if len(t) < 10 {
		return ErrBadFont
	}
	first := uint32(binary.BigEndian.Uint16(t[6:8]))
	count := int(binary.BigEndian.Uint16(t[8:10]))
	for i := 0; i < count && 10+2*i+2 <= len(t); i++ {
		if gid := binary.BigEndian.Uint16(t[10+2*i:]); gid != 0 && gid < f.m.NumGlyphs {
			f.cmap[rune(first+uint32(i))] = gid
		}
	}
	return nil
}

func (f *Face) cmap12(t []byte) error {
	if len(t) < 16 {
		return ErrBadFont
	}
	groups := int(binary.BigEndian.Uint32(t[12:16]))
	for i := 0; i < groups; i++ {
		off := 16 + 12*i
		if off+12 > len(t) {
			break
		}
		start := binary.BigEndian.Uint32(t[off:])
		end := binary.BigEndian.Uint32(t[off+4:])
		gid := binary.BigEndian.Uint32(t[off+8:])
		if end < start || end-start > 0x10FFFF {
			continue
		}
		for c := start; c <= end; c++ {
			if g := gid + (c - start); g != 0 && g < uint32(f.m.NumGlyphs) {
				f.cmap[rune(c)] = uint16(g)
			}
		}
	}
	return nil
}
