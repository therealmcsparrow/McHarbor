// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package computesim

import (
	"strings"
	"time"
)

// glyphs is a 5x7 bitmap font, upper case only, which is all a boot screen
// needs; lower case is drawn with the upper-case glyph.
var glyphs = map[rune][7]string{
	'A':  {".###.", "#...#", "#...#", "#####", "#...#", "#...#", "#...#"},
	'B':  {"####.", "#...#", "#...#", "####.", "#...#", "#...#", "####."},
	'C':  {".###.", "#...#", "#....", "#....", "#....", "#...#", ".###."},
	'D':  {"####.", "#...#", "#...#", "#...#", "#...#", "#...#", "####."},
	'E':  {"#####", "#....", "#....", "####.", "#....", "#....", "#####"},
	'F':  {"#####", "#....", "#....", "####.", "#....", "#....", "#...."},
	'G':  {".###.", "#...#", "#....", "#.###", "#...#", "#...#", ".####"},
	'H':  {"#...#", "#...#", "#...#", "#####", "#...#", "#...#", "#...#"},
	'I':  {".###.", "..#..", "..#..", "..#..", "..#..", "..#..", ".###."},
	'J':  {"..###", "...#.", "...#.", "...#.", "...#.", "#..#.", ".##.."},
	'K':  {"#...#", "#..#.", "#.#..", "##...", "#.#..", "#..#.", "#...#"},
	'L':  {"#....", "#....", "#....", "#....", "#....", "#....", "#####"},
	'M':  {"#...#", "##.##", "#.#.#", "#.#.#", "#...#", "#...#", "#...#"},
	'N':  {"#...#", "#...#", "##..#", "#.#.#", "#..##", "#...#", "#...#"},
	'O':  {".###.", "#...#", "#...#", "#...#", "#...#", "#...#", ".###."},
	'P':  {"####.", "#...#", "#...#", "####.", "#....", "#....", "#...."},
	'Q':  {".###.", "#...#", "#...#", "#...#", "#.#.#", "#..#.", ".##.#"},
	'R':  {"####.", "#...#", "#...#", "####.", "#.#..", "#..#.", "#...#"},
	'S':  {".####", "#....", "#....", ".###.", "....#", "....#", "####."},
	'T':  {"#####", "..#..", "..#..", "..#..", "..#..", "..#..", "..#.."},
	'U':  {"#...#", "#...#", "#...#", "#...#", "#...#", "#...#", ".###."},
	'V':  {"#...#", "#...#", "#...#", "#...#", "#...#", ".#.#.", "..#.."},
	'W':  {"#...#", "#...#", "#...#", "#.#.#", "#.#.#", "#.#.#", ".#.#."},
	'X':  {"#...#", "#...#", ".#.#.", "..#..", ".#.#.", "#...#", "#...#"},
	'Y':  {"#...#", "#...#", ".#.#.", "..#..", "..#..", "..#..", "..#.."},
	'Z':  {"#####", "....#", "...#.", "..#..", ".#...", "#....", "#####"},
	'0':  {".###.", "#...#", "#..##", "#.#.#", "##..#", "#...#", ".###."},
	'1':  {"..#..", ".##..", "..#..", "..#..", "..#..", "..#..", ".###."},
	'2':  {".###.", "#...#", "....#", "...#.", "..#..", ".#...", "#####"},
	'3':  {"#####", "...#.", "..#..", "...#.", "....#", "#...#", ".###."},
	'4':  {"...#.", "..##.", ".#.#.", "#..#.", "#####", "...#.", "...#."},
	'5':  {"#####", "#....", "####.", "....#", "....#", "#...#", ".###."},
	'6':  {"..##.", ".#...", "#....", "####.", "#...#", "#...#", ".###."},
	'7':  {"#####", "....#", "...#.", "..#..", ".#...", ".#...", ".#..."},
	'8':  {".###.", "#...#", "#...#", ".###.", "#...#", "#...#", ".###."},
	'9':  {".###.", "#...#", "#...#", ".####", "....#", "...#.", ".##.."},
	'.':  {".....", ".....", ".....", ".....", ".....", ".##..", ".##.."},
	',':  {".....", ".....", ".....", ".....", ".##..", ".##..", ".#..."},
	':':  {".....", ".##..", ".##..", ".....", ".##..", ".##..", "....."},
	';':  {".....", ".##..", ".##..", ".....", ".##..", ".##..", ".#..."},
	'-':  {".....", ".....", ".....", "#####", ".....", ".....", "....."},
	'_':  {".....", ".....", ".....", ".....", ".....", ".....", "#####"},
	'/':  {"....#", "....#", "...#.", "..#..", ".#...", "#....", "#...."},
	'(':  {"...#.", "..#..", ".#...", ".#...", ".#...", "..#..", "...#."},
	')':  {".#...", "..#..", "...#.", "...#.", "...#.", "..#..", ".#..."},
	'[':  {".###.", ".#...", ".#...", ".#...", ".#...", ".#...", ".###."},
	']':  {".###.", "...#.", "...#.", "...#.", "...#.", "...#.", ".###."},
	'!':  {"..#..", "..#..", "..#..", "..#..", "..#..", ".....", "..#.."},
	'?':  {".###.", "#...#", "....#", "...#.", "..#..", ".....", "..#.."},
	'\'': {"..#..", "..#..", ".#...", ".....", ".....", ".....", "....."},
	'"':  {".#.#.", ".#.#.", ".....", ".....", ".....", ".....", "....."},
	'@':  {".###.", "#...#", "#.###", "#.#.#", "#.###", "#....", ".####"},
	'#':  {".#.#.", ".#.#.", "#####", ".#.#.", "#####", ".#.#.", ".#.#."},
	'+':  {".....", "..#..", "..#..", "#####", "..#..", "..#..", "....."},
	'=':  {".....", ".....", "#####", ".....", "#####", ".....", "....."},
	'<':  {"...#.", "..#..", ".#...", "#....", ".#...", "..#..", "...#."},
	'>':  {".#...", "..#..", "...#.", "....#", "...#.", "..#..", ".#..."},
	'|':  {"..#..", "..#..", "..#..", "..#..", "..#..", "..#..", "..#.."},
	'%':  {"##...", "##..#", "...#.", "..#..", ".#...", "#..##", "...##"},
	'*':  {".....", "#.#.#", ".###.", "#####", ".###.", "#.#.#", "....."},
	'~':  {".....", ".....", ".#...", "#.#.#", "...#.", ".....", "....."},
	'$':  {"..#..", ".####", "#.#..", ".###.", "..#.#", "####.", "..#.."},
	'\\': {"#....", "#....", ".#...", "..#..", "...#.", "....#", "....#"},
}

const (
	screenWidth  = 1024
	screenHeight = 768
	glyphCols    = 5
	glyphRows    = 7
)

// framebuffer is 0xRRGGBB pixels, encoded into the client's pixel format only
// when a rectangle is sent.
type framebuffer struct {
	w, h int
	pix  []uint32
}

func newFramebuffer(w, h int) *framebuffer {
	return &framebuffer{w: w, h: h, pix: make([]uint32, w*h)}
}

func (f *framebuffer) fill(x, y, w, h int, c uint32) {
	for yy := max(y, 0); yy < min(y+h, f.h); yy++ {
		row := f.pix[yy*f.w : (yy+1)*f.w]
		for xx := max(x, 0); xx < min(x+w, f.w); xx++ {
			row[xx] = c
		}
	}
}

// text draws s at (x, y) with every font pixel scaled to a scale-sized square,
// and returns the x just past the last glyph.
func (f *framebuffer) text(x, y, scale int, s string, c uint32) int {
	advance := (glyphCols + 1) * scale
	for _, r := range strings.ToUpper(s) {
		g, ok := glyphs[r]
		if !ok && r != ' ' {
			g = glyphs['?']
		}
		if r != ' ' {
			for row, bits := range g {
				for col := 0; col < len(bits) && col < glyphCols; col++ {
					if bits[col] == '#' {
						f.fill(x+col*scale, y+row*scale, scale, scale, c)
					}
				}
			}
		}
		x += advance
	}
	return x
}

func textWidth(s string, scale int) int { return len([]rune(s)) * (glyphCols + 1) * scale }

const (
	colBackground = 0x0b1220
	colHeader     = 0x1e293b
	colAccent     = 0x7dd3fc
	colText       = 0xe2e8f0
	colDim        = 0x94a3b8
	colOK         = 0x4ade80
	colWarn       = 0xfbbf24
	colWinBlue    = 0x0a3069
)

// bootScreen is what a VNC viewer shows: a console login for Linux and BSD
// guests, a lock screen for Windows, both marked as a simulation.
type bootScreen struct {
	view    guestView
	typed   string
	message string
	fb      *framebuffer
}

func newBootScreen(view guestView) *bootScreen {
	b := &bootScreen{view: view, fb: newFramebuffer(screenWidth, screenHeight)}
	b.render()
	return b
}

const (
	bodyScale  = 2
	lineHeight = 22
	bodyLeft   = 40
)

// inputBand is the rectangle the login line and its message occupy, which is
// all a keystroke repaints.
func (b *bootScreen) inputBand() (x, y, w, h int) {
	if b.view.family == "windows" {
		return 0, 500, screenWidth, 3 * lineHeight
	}
	return 0, b.loginY(), screenWidth, 3 * lineHeight
}

func (b *bootScreen) loginY() int { return 90 + 11*lineHeight }

func (b *bootScreen) render() {
	f, v := b.fb, b.view
	if v.family == "windows" {
		f.fill(0, 0, f.w, f.h, colWinBlue)
		now := time.Now()
		clock := now.Format("15:04")
		f.text((f.w-textWidth(clock, 12))/2, 180, 12, clock, colText)
		date := now.Format("Monday, January 2")
		f.text((f.w-textWidth(date, 3))/2, 290, 3, date, colText)
		name := v.osName
		f.text((f.w-textWidth(name, 2))/2, 380, 2, name, colDim)
		prompt := "Press Ctrl+Alt+Delete to sign in."
		_, y, _, _ := b.inputBand()
		f.text((f.w-textWidth(prompt, 2))/2, y, 2, prompt, colText)
		if b.message != "" {
			f.text((f.w-textWidth(b.message, 2))/2, y+lineHeight+8, 2, b.message, colWarn)
		}
		b.footer()
		return
	}
	f.fill(0, 0, f.w, f.h, colBackground)
	f.fill(0, 0, f.w, 48, colHeader)
	f.text(20, 14, 3, "McHarbor demo console - "+v.name, colAccent)
	ip := "no address"
	if len(v.ips) > 0 {
		ip = v.ips[0]
	}
	lines := []struct {
		text  string
		color uint32
	}{
		{"[  OK  ] Started Journal Service.", colOK},
		{"[  OK  ] Reached target Local File Systems.", colOK},
		{"[  OK  ] Started Network Configuration.", colOK},
		{"[  OK  ] Started QEMU Guest Agent.", colOK},
		{"[  OK  ] Started OpenSSH server daemon.", colOK},
		{"[  OK  ] Reached target Multi-User System.", colOK},
		{"", colText},
		{v.osName, colText},
		{"Kernel " + v.kernel + " on an x86_64 (tty1)", colDim},
		{"eth0: " + ip, colDim},
	}
	for i, l := range lines {
		f.text(bodyLeft, 90+i*lineHeight, bodyScale, l.text, l.color)
	}
	b.drawLogin()
	b.footer()
}

func (b *bootScreen) drawLogin() {
	f := b.fb
	x, y, w, h := b.inputBand()
	bg := uint32(colBackground)
	if b.view.family == "windows" {
		bg = colWinBlue
	}
	f.fill(x, y, w, h, bg)
	if b.view.family == "windows" {
		b.render()
		return
	}
	end := f.text(bodyLeft, y, bodyScale, b.view.name+" login: "+b.typed, colText)
	f.fill(end, y, glyphCols*bodyScale, glyphRows*bodyScale, colText)
	if b.message != "" {
		f.text(bodyLeft, y+lineHeight+4, bodyScale, b.message, colWarn)
	}
}

func (b *bootScreen) footer() {
	f := b.fb
	f.fill(0, f.h-40, f.w, 40, colHeader)
	msg := "DEMO MODE - simulated display. Nothing here is real."
	f.text((f.w-textWidth(msg, 2))/2, f.h-27, 2, msg, colWarn)
}

// key applies one key press and reports whether the screen changed.
func (b *bootScreen) key(keysym uint32) bool {
	switch {
	case keysym == 0xff0d || keysym == 0xff8d:
		if b.view.family == "windows" {
			b.message = "Sign-in is disabled in demo mode."
		} else {
			b.message = "Login incorrect (demo mode - there is no real guest)."
		}
		b.typed = ""
	case keysym == 0xff08:
		if b.typed == "" {
			return false
		}
		b.typed = b.typed[:len(b.typed)-1]
	case keysym >= 0x20 && keysym < 0x7f:
		if b.view.family == "windows" || len(b.typed) >= 40 {
			return false
		}
		b.typed += string(rune(keysym))
		b.message = ""
	default:
		return false
	}
	b.drawLogin()
	return true
}
