// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pdf

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

// Image is an embedded raster, stored as a DeviceRGB XObject with an
// optional soft mask carrying the alpha channel.
//
// PNG's own compressed bytes are not reused: the PNG filter/interlace
// combinations a source file may use are not all things a PDF /FlateDecode
// image can express, and the assets here are small. Decoding and
// re-deflating is a few milliseconds and always correct.
type Image struct {
	W, H     int
	rgb      []byte
	alpha    []byte
	hasAlpha bool

	// refID names the image inside page content streams (`/X1`, `/X2`).
	// Like fonts, content is written before object numbers are handed
	// out, so the resource name must be stable from registration time.
	refID    int
	objNum   int
	smaskNum int
	used     bool
}

// Aspect is width/height, so callers can size a logo by one dimension.
func (i *Image) Aspect() float64 {
	if i == nil || i.H == 0 {
		return 1
	}
	return float64(i.W) / float64(i.H)
}

// AddImagePNG decodes a PNG and registers it for use on any page.
func (d *Doc) AddImagePNG(data []byte) (*Image, error) {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("reporting: decode png: %w", err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("reporting: png has no pixels")
	}

	img := &Image{W: w, H: h, rgb: make([]byte, 0, w*h*3)}
	alpha := make([]byte, 0, w*h)
	opaque := true

	// image/color returns alpha-premultiplied 16-bit values; PDF wants
	// straight (un-premultiplied) 8-bit colour plus a separate mask, so
	// un-premultiply as we go. Fully transparent pixels have no colour
	// to recover — white keeps them invisible against the page either way.
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r16, g16, b16, a16 := src.At(x, y).RGBA()
			if a16 == 0 {
				img.rgb = append(img.rgb, 0xFF, 0xFF, 0xFF)
				alpha = append(alpha, 0)
				opaque = false
				continue
			}
			r := uint8(r16 * 0xFFFF / a16 >> 8)
			g := uint8(g16 * 0xFFFF / a16 >> 8)
			bl := uint8(b16 * 0xFFFF / a16 >> 8)
			a := uint8(a16 >> 8)
			img.rgb = append(img.rgb, r, g, bl)
			alpha = append(alpha, a)
			if a != 0xFF {
				opaque = false
			}
		}
	}
	if !opaque {
		img.alpha = alpha
		img.hasAlpha = true
	}
	img.refID = len(d.images) + 1
	d.images = append(d.images, img)
	return img, nil
}

// AddImageRGBA registers an in-memory image — the path the chart
// rasteriser uses so a rendered chart never round-trips through PNG.
func (d *Doc) AddImageRGBA(src *image.RGBA) (*Image, error) {
	if src == nil {
		return nil, fmt.Errorf("reporting: nil image")
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("reporting: image has no pixels")
	}
	img := &Image{W: w, H: h, rgb: make([]byte, 0, w*h*3)}
	alpha := make([]byte, 0, w*h)
	opaque := true
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := src.PixOffset(b.Min.X, y)
		for x := 0; x < w; x++ {
			o := row + x*4
			a := src.Pix[o+3]
			if a == 0 {
				img.rgb = append(img.rgb, 0xFF, 0xFF, 0xFF)
				alpha = append(alpha, 0)
				opaque = false
				continue
			}
			r, g, bl := src.Pix[o], src.Pix[o+1], src.Pix[o+2]
			if a != 0xFF {
				r = uint8(int(r) * 255 / int(a))
				g = uint8(int(g) * 255 / int(a))
				bl = uint8(int(bl) * 255 / int(a))
				opaque = false
			}
			img.rgb = append(img.rgb, r, g, bl)
			alpha = append(alpha, a)
		}
	}
	if !opaque {
		img.alpha = alpha
		img.hasAlpha = true
	}
	img.refID = len(d.images) + 1
	d.images = append(d.images, img)
	return img, nil
}

func writeImage(o *objects, img *Image) {
	dict := fmt.Sprintf(
		"/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8",
		img.W, img.H)
	if img.hasAlpha {
		dict += fmt.Sprintf(" /SMask %d 0 R", img.smaskNum)
	}
	o.putStream(img.objNum, dict, img.rgb, true)

	if img.hasAlpha {
		o.putStream(img.smaskNum, fmt.Sprintf(
			"/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8",
			img.W, img.H), img.alpha, true)
	}
}
