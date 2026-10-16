// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package computesim

import (
	"context"
	"encoding/binary"
)

// pixelFormat is the RFB PIXEL_FORMAT a client asked for; every rectangle is
// encoded in it, because noVNC re-negotiates to its own layout after init.
type pixelFormat struct {
	bpp        uint8
	depth      uint8
	bigEndian  bool
	trueColour bool
	rMax       uint16
	gMax       uint16
	bMax       uint16
	rShift     uint8
	gShift     uint8
	bShift     uint8
}

var serverPixelFormat = pixelFormat{bpp: 32, depth: 24, trueColour: true, rMax: 255, gMax: 255, bMax: 255, rShift: 16, gShift: 8, bShift: 0}

func (p pixelFormat) bytes() []byte {
	out := make([]byte, 16)
	out[0], out[1] = p.bpp, p.depth
	if p.bigEndian {
		out[2] = 1
	}
	if p.trueColour {
		out[3] = 1
	}
	binary.BigEndian.PutUint16(out[4:], p.rMax)
	binary.BigEndian.PutUint16(out[6:], p.gMax)
	binary.BigEndian.PutUint16(out[8:], p.bMax)
	out[10], out[11], out[12] = p.rShift, p.gShift, p.bShift
	return out
}

func parsePixelFormat(b []byte) pixelFormat {
	return pixelFormat{
		bpp: b[0], depth: b[1], bigEndian: b[2] != 0, trueColour: b[3] != 0,
		rMax: binary.BigEndian.Uint16(b[4:]), gMax: binary.BigEndian.Uint16(b[6:]), bMax: binary.BigEndian.Uint16(b[8:]),
		rShift: b[10], gShift: b[11], bShift: b[12],
	}
}

func (p pixelFormat) valid() bool {
	return p.trueColour && (p.bpp == 8 || p.bpp == 16 || p.bpp == 32)
}

// encode appends one 0xRRGGBB pixel in this format.
func (p pixelFormat) encode(dst []byte, rgb uint32) []byte {
	r, g, b := (rgb>>16)&0xff, (rgb>>8)&0xff, rgb&0xff
	v := (r*uint32(p.rMax)/255)<<p.rShift | (g*uint32(p.gMax)/255)<<p.gShift | (b*uint32(p.bMax)/255)<<p.bShift
	switch p.bpp {
	case 8:
		return append(dst, byte(v))
	case 16:
		if p.bigEndian {
			return binary.BigEndian.AppendUint16(dst, uint16(v))
		}
		return binary.LittleEndian.AppendUint16(dst, uint16(v))
	default:
		if p.bigEndian {
			return binary.BigEndian.AppendUint32(dst, v)
		}
		return binary.LittleEndian.AppendUint32(dst, v)
	}
}

// rfbServer is a minimal RFB 3.8 server over the console byte stream: no
// authentication, one Raw rectangle per update, input parsed and mostly
// ignored. It is enough for noVNC to paint the simulated boot screen.
type rfbServer struct {
	screen  *bootScreen
	format  pixelFormat
	buf     []byte
	stage   int
	pending bool
	dirty   [4]int
	hasDirt bool
	write   func([]byte) bool
}

const (
	rfbStageVersion = iota
	rfbStageSecurity
	rfbStageInit
	rfbStageNormal
)

func runRFB(ctx context.Context, view guestView, in <-chan []byte, write func([]byte) bool, finish func(string)) {
	srv := &rfbServer{screen: newBootScreen(view), format: serverPixelFormat, write: write}
	if !write([]byte("RFB 003.008\n")) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-in:
			if reason, ok := srv.feed(data); !ok {
				finish(reason)
				return
			}
		}
	}
}

// feed consumes client bytes; it returns false with a reason when the stream
// can no longer be parsed or the connection is gone.
func (r *rfbServer) feed(data []byte) (string, bool) {
	r.buf = append(r.buf, data...)
	for {
		n, reason, ok := r.step()
		if !ok {
			return reason, false
		}
		if n == 0 {
			return "", true
		}
		r.buf = r.buf[n:]
	}
}

// step handles at most one complete client message and returns how many bytes
// it consumed; zero means more bytes are needed.
func (r *rfbServer) step() (int, string, bool) {
	b := r.buf
	switch r.stage {
	case rfbStageVersion:
		if len(b) < 12 {
			return 0, "", true
		}
		r.stage = rfbStageSecurity
		return 12, "", r.write([]byte{1, 1})
	case rfbStageSecurity:
		if len(b) < 1 {
			return 0, "", true
		}
		if b[0] != 1 {
			return 0, "the viewer asked for an unsupported security type", false
		}
		r.stage = rfbStageInit
		return 1, "", r.write([]byte{0, 0, 0, 0})
	case rfbStageInit:
		if len(b) < 1 {
			return 0, "", true
		}
		r.stage = rfbStageNormal
		name := []byte(r.screen.view.name + " (McHarbor demo)")
		out := make([]byte, 0, 24+len(name))
		out = binary.BigEndian.AppendUint16(out, screenWidth)
		out = binary.BigEndian.AppendUint16(out, screenHeight)
		out = append(out, serverPixelFormat.bytes()...)
		out = binary.BigEndian.AppendUint32(out, uint32(len(name)))
		out = append(out, name...)
		return 1, "", r.write(out)
	}
	if len(b) < 1 {
		return 0, "", true
	}
	switch b[0] {
	case 0:
		if len(b) < 20 {
			return 0, "", true
		}
		pf := parsePixelFormat(b[4:20])
		if !pf.valid() {
			return 0, "the viewer asked for a colour-map pixel format", false
		}
		r.format = pf
		return 20, "", true
	case 2:
		if len(b) < 4 {
			return 0, "", true
		}
		n := 4 + 4*int(binary.BigEndian.Uint16(b[2:4]))
		if len(b) < n {
			return 0, "", true
		}
		return n, "", true
	case 3:
		if len(b) < 10 {
			return 0, "", true
		}
		if b[1] == 0 {
			return 10, "", r.sendRect(0, 0, screenWidth, screenHeight)
		}
		r.pending = true
		return 10, "", r.flush()
	case 4:
		if len(b) < 8 {
			return 0, "", true
		}
		if b[1] != 0 && r.screen.key(binary.BigEndian.Uint32(b[4:8])) {
			x, y, w, h := r.screen.inputBand()
			r.markDirty(x, y, w, h)
		}
		return 8, "", r.flush()
	case 5:
		if len(b) < 6 {
			return 0, "", true
		}
		return 6, "", true
	case 6:
		if len(b) < 8 {
			return 0, "", true
		}
		n := 8 + int(binary.BigEndian.Uint32(b[4:8]))
		if n > 1<<20 {
			return 0, "clipboard text too large", false
		}
		if len(b) < n {
			return 0, "", true
		}
		return n, "", true
	}
	return 0, "the viewer sent a message this demo console does not understand", false
}

func (r *rfbServer) markDirty(x, y, w, h int) {
	if !r.hasDirt {
		r.dirty, r.hasDirt = [4]int{x, y, x + w, y + h}, true
		return
	}
	r.dirty[0], r.dirty[1] = min(r.dirty[0], x), min(r.dirty[1], y)
	r.dirty[2], r.dirty[3] = max(r.dirty[2], x+w), max(r.dirty[3], y+h)
}

// flush answers an outstanding incremental request once something changed; an
// unchanged screen leaves the request open, which RFB allows.
func (r *rfbServer) flush() bool {
	if !r.pending || !r.hasDirt {
		return true
	}
	d := r.dirty
	r.hasDirt = false
	return r.sendRect(d[0], d[1], d[2]-d[0], d[3]-d[1])
}

// sendRect writes one FramebufferUpdate carrying a single Raw rectangle.
func (r *rfbServer) sendRect(x, y, w, h int) bool {
	r.pending = false
	fb := r.screen.fb
	x, y = max(0, x), max(0, y)
	w, h = min(w, fb.w-x), min(h, fb.h-y)
	if w <= 0 || h <= 0 {
		return true
	}
	out := make([]byte, 0, 16+w*h*int(r.format.bpp/8))
	out = append(out, 0, 0)
	out = binary.BigEndian.AppendUint16(out, 1)
	out = binary.BigEndian.AppendUint16(out, uint16(x))
	out = binary.BigEndian.AppendUint16(out, uint16(y))
	out = binary.BigEndian.AppendUint16(out, uint16(w))
	out = binary.BigEndian.AppendUint16(out, uint16(h))
	out = binary.BigEndian.AppendUint32(out, 0)
	for yy := y; yy < y+h; yy++ {
		row := fb.pix[yy*fb.w+x : yy*fb.w+x+w]
		for _, px := range row {
			out = r.format.encode(out, px)
		}
	}
	return r.write(out)
}
