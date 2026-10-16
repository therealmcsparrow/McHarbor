// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"time"
)

// Page sizes in points (1 pt = 1/72 inch).
const (
	A4Width  = 595.276
	A4Height = 841.890
)

// Color is RGB in the 0..1 range.
type Color struct{ R, G, B float64 }

// RGB builds a Color from 8-bit channels, which is how the brand palette
// is written down everywhere else in this plugin.
func RGB(r, g, b uint8) Color {
	return Color{float64(r) / 255, float64(g) / 255, float64(b) / 255}
}

// Mix blends two colours; t=0 returns a, t=1 returns b.
func Mix(a, b Color, t float64) Color {
	return Color{a.R + (b.R-a.R)*t, a.G + (b.G-a.G)*t, a.B + (b.B-a.B)*t}
}

// Doc is a PDF document under construction.
//
// The public coordinate system is top-left origin with y growing
// downward, because that is how the layout code thinks. Conversion to
// PDF's bottom-left space happens in one place, at emit time.
type Doc struct {
	W, H float64

	Title, Author, Subject, Creator string
	Keywords                        string

	pages  []*Page
	fonts  []*Font
	images []*Image

	outline []bookmark
}

type bookmark struct {
	title string
	level int // 0 = top level, 1 = nested
	page  int // index into pages
	y     float64
}

// New starts a document with the given page size in points.
func New(w, h float64) *Doc {
	return &Doc{W: w, H: h, Creator: "McHarbor Reporting"}
}

// AddFont registers a TrueType face. `name` is the PostScript base name
// written into the PDF; it is what a viewer shows in Document Properties.
func (d *Doc) AddFont(name string, ttfData []byte) (*Font, error) {
	f, err := newFont(name, ttfData)
	if err != nil {
		return nil, err
	}
	f.refID = len(d.fonts) + 1
	d.fonts = append(d.fonts, f)
	return f, nil
}

// NewPage appends a blank page and returns it.
func (d *Doc) NewPage() *Page {
	p := &Page{doc: d, index: len(d.pages)}
	d.pages = append(d.pages, p)
	return p
}

// PageCount is how many pages exist so far — the running footer needs it,
// which is why the caller renders in two passes.
func (d *Doc) PageCount() int { return len(d.pages) }

// Page returns the page at index, or nil.
func (d *Doc) Page(i int) *Page {
	if i < 0 || i >= len(d.pages) {
		return nil
	}
	return d.pages[i]
}

// AddBookmark records a PDF outline entry pointing at a position on a
// page. A board pack that cannot be navigated from the sidebar is a
// worse document than one that can.
func (d *Doc) AddBookmark(title string, level, pageIndex int, yTop float64) {
	if pageIndex < 0 || pageIndex >= len(d.pages) || strings.TrimSpace(title) == "" {
		return
	}
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}
	d.outline = append(d.outline, bookmark{title: title, level: level, page: pageIndex, y: yTop})
}

// --- object plumbing ------------------------------------------------

type objects struct {
	buf     bytes.Buffer
	offsets []int // 1-based object number -> byte offset
}

func (o *objects) alloc() int {
	o.offsets = append(o.offsets, 0)
	return len(o.offsets)
}

func (o *objects) put(num int, body []byte) {
	o.offsets[num-1] = o.buf.Len()
	fmt.Fprintf(&o.buf, "%d 0 obj\n", num)
	o.buf.Write(body)
	o.buf.WriteString("\nendobj\n")
}

func (o *objects) putStream(num int, dict string, payload []byte, compress bool) {
	data := payload
	filter := ""
	if compress {
		var z bytes.Buffer
		w := zlib.NewWriter(&z)
		_, _ = w.Write(payload)
		_ = w.Close()
		// Only keep the compressed form when it actually helps; tiny
		// streams grow under zlib's header.
		if z.Len() < len(payload) {
			data = z.Bytes()
			filter = " /Filter /FlateDecode"
		}
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "<< %s /Length %d%s >>\nstream\n", dict, len(data), filter)
	b.Write(data)
	b.WriteString("\nendstream")
	o.put(num, b.Bytes())
}

// Bytes serialises the document.
func (d *Doc) Bytes() ([]byte, error) {
	if len(d.pages) == 0 {
		d.NewPage()
	}
	o := &objects{}
	// Object 1 is conventionally the catalog; allocate the fixed spine
	// first so the file reads sensibly in a text editor.
	catalogNum := o.alloc()
	pagesNum := o.alloc()
	infoNum := o.alloc()

	fontNums := make(map[*Font]int, len(d.fonts))
	for _, f := range d.fonts {
		if len(f.used) == 0 {
			continue // never drawn with — do not embed 300 KB for nothing
		}
		f.objNum = o.alloc()
		fontNums[f] = f.objNum
	}
	for _, img := range d.images {
		img.objNum = o.alloc()
		if img.hasAlpha {
			img.smaskNum = o.alloc()
		}
	}

	pageNums := make([]int, len(d.pages))
	contentNums := make([]int, len(d.pages))
	for i := range d.pages {
		pageNums[i] = o.alloc()
		contentNums[i] = o.alloc()
	}

	outlineNums := d.allocOutline(o)

	// --- write the fonts
	for _, f := range d.fonts {
		if _, ok := fontNums[f]; !ok {
			continue
		}
		d.writeFont(o, f)
	}
	for _, img := range d.images {
		writeImage(o, img)
	}

	// --- write the pages
	for i, p := range d.pages {
		var res bytes.Buffer
		res.WriteString("<< /ProcSet [/PDF /Text /ImageC] ")
		if len(fontNums) > 0 {
			res.WriteString("/Font << ")
			for _, f := range d.fonts {
				if n, ok := fontNums[f]; ok {
					fmt.Fprintf(&res, "/F%d %d 0 R ", f.refID, n)
				}
			}
			res.WriteString(">> ")
		}
		if len(d.images) > 0 {
			res.WriteString("/XObject << ")
			for _, img := range d.images {
				fmt.Fprintf(&res, "/X%d %d 0 R ", img.refID, img.objNum)
			}
			res.WriteString(">> ")
		}
		res.WriteString(">>")

		body := fmt.Sprintf(
			"<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources %s /Contents %d 0 R >>",
			pagesNum, num(d.W), num(d.H), res.String(), contentNums[i],
		)
		o.put(pageNums[i], []byte(body))
		o.putStream(contentNums[i], "", p.content.Bytes(), true)
	}

	// --- page tree
	var kids bytes.Buffer
	kids.WriteString("[")
	for i, n := range pageNums {
		if i > 0 {
			kids.WriteString(" ")
		}
		fmt.Fprintf(&kids, "%d 0 R", n)
	}
	kids.WriteString("]")
	o.put(pagesNum, []byte(fmt.Sprintf(
		"<< /Type /Pages /Count %d /Kids %s >>", len(pageNums), kids.String())))

	// --- catalog
	cat := fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R /PageLayout /SinglePage /PageMode /%s",
		pagesNum, pageMode(len(outlineNums) > 0))
	if len(outlineNums) > 0 {
		cat += fmt.Sprintf(" /Outlines %d 0 R", outlineNums[0])
	}
	cat += " >>"
	o.put(catalogNum, []byte(cat))

	// --- document information
	now := time.Now()
	info := fmt.Sprintf("<< /Title %s /Author %s /Subject %s /Creator %s /Producer %s /Keywords %s /CreationDate %s /ModDate %s >>",
		pdfString(d.Title), pdfString(d.Author), pdfString(d.Subject),
		pdfString(d.Creator), pdfString("McHarbor Reporting"), pdfString(d.Keywords),
		pdfDate(now), pdfDate(now))
	o.put(infoNum, []byte(info))

	d.writeOutline(o, outlineNums, pageNums)

	// --- assemble
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	// A binary comment marks the file as binary for transfer agents.
	out.Write([]byte{'%', 0xE2, 0xE3, 0xCF, 0xD3, '\n'})
	base := out.Len()
	out.Write(o.buf.Bytes())

	xrefAt := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", len(o.offsets)+1)
	out.WriteString("0000000000 65535 f \n")
	for _, off := range o.offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", off+base)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(o.offsets)+1, catalogNum, infoNum, xrefAt)
	return out.Bytes(), nil
}

func pageMode(hasOutline bool) string {
	if hasOutline {
		return "UseOutlines"
	}
	return "UseNone"
}

func (d *Doc) writeFont(o *objects, f *Font) {
	descNum := o.alloc()
	fileNum := o.alloc()
	cidNum := o.alloc()
	toUniNum := o.alloc()

	// Type0 wrapper.
	o.put(f.objNum, []byte(fmt.Sprintf(
		"<< /Type /Font /Subtype /Type0 /BaseFont /%s /Encoding /Identity-H /DescendantFonts [%d 0 R] /ToUnicode %d 0 R >>",
		f.Name, cidNum, toUniNum)))

	// CIDFontType2 descendant + widths.
	o.put(cidNum, []byte(fmt.Sprintf(
		"<< /Type /Font /Subtype /CIDFontType2 /BaseFont /%s /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor %d 0 R /DW 1000 /W %s /CIDToGIDMap /Identity >>",
		f.Name, descNum, widthsArray(f))))

	o.put(descNum, []byte(fmt.Sprintf(
		"<< /Type /FontDescriptor /FontName /%s /Flags %d /FontBBox [%d %d %d %d] /ItalicAngle %s /Ascent %d /Descent %d /CapHeight %d /StemV %d /FontFile2 %d 0 R >>",
		f.Name, f.flags(),
		f.face.Scale1000(f.m.XMin), f.face.Scale1000(f.m.YMin),
		f.face.Scale1000(f.m.XMax), f.face.Scale1000(f.m.YMax),
		num(f.m.ItalicAngle),
		f.face.Scale1000(f.m.Ascender), f.face.Scale1000(f.m.Descender),
		f.face.Scale1000(f.m.CapHeight),
		f.stemV(), fileNum)))

	raw := f.face.Raw()
	o.putStream(fileNum, fmt.Sprintf("/Length1 %d", len(raw)), raw, true)
	o.putStream(toUniNum, "", toUnicodeCMap(f), true)
}

func widthsArray(f *Font) string {
	ids := f.sortedGlyphs()
	var b strings.Builder
	b.WriteString("[")
	i := 0
	for i < len(ids) {
		// Emit consecutive runs as `first [w w w]`, which is both the
		// compact form and the one every viewer parses fastest.
		j := i
		for j+1 < len(ids) && ids[j+1] == ids[j]+1 {
			j++
		}
		fmt.Fprintf(&b, " %d [", ids[i])
		for k := i; k <= j; k++ {
			if k > i {
				b.WriteString(" ")
			}
			fmt.Fprintf(&b, "%d", f.face.Advance1000(ids[k]))
		}
		b.WriteString("]")
		i = j + 1
	}
	b.WriteString(" ]")
	return b.String()
}

func toUnicodeCMap(f *Font) []byte {
	ids := f.sortedGlyphs()
	var b bytes.Buffer
	b.WriteString(`/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName /Adobe-Identity-UCS def
/CMapType 2 def
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
`)
	// bfchar sections are capped at 100 entries by the spec.
	for start := 0; start < len(ids); start += 100 {
		end := start + 100
		if end > len(ids) {
			end = len(ids)
		}
		fmt.Fprintf(&b, "%d beginbfchar\n", end-start)
		for _, gid := range ids[start:end] {
			fmt.Fprintf(&b, "<%04X> <%s>\n", gid, utf16BE(f.used[gid]))
		}
		b.WriteString("endbfchar\n")
	}
	b.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return b.Bytes()
}

func utf16BE(r rune) string {
	if r < 0x10000 {
		return fmt.Sprintf("%04X", r)
	}
	r -= 0x10000
	return fmt.Sprintf("%04X%04X", 0xD800+(r>>10), 0xDC00+(r&0x3FF))
}

// --- outline --------------------------------------------------------

func (d *Doc) allocOutline(o *objects) []int {
	if len(d.outline) == 0 {
		return nil
	}
	nums := make([]int, len(d.outline)+1)
	for i := range nums {
		nums[i] = o.alloc()
	}
	return nums
}

func (d *Doc) writeOutline(o *objects, nums, pageNums []int) {
	if len(nums) == 0 {
		return
	}
	root := nums[0]
	items := nums[1:]

	// Level-1 entries attach to the nearest preceding level-0 entry;
	// anything before the first level-0 entry is promoted rather than
	// orphaned.
	parentOf := make([]int, len(d.outline))
	lastTop := -1
	for i, bm := range d.outline {
		if bm.level == 0 || lastTop < 0 {
			parentOf[i] = -1
			lastTop = i
		} else {
			parentOf[i] = lastTop
		}
	}

	children := func(parent int) []int {
		var out []int
		for i := range d.outline {
			if parentOf[i] == parent {
				out = append(out, i)
			}
		}
		return out
	}

	top := children(-1)
	if len(top) == 0 {
		return
	}
	o.put(root, []byte(fmt.Sprintf("<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>",
		items[top[0]], items[top[len(top)-1]], len(d.outline))))

	emit := func(list []int, parentObj int) {
		for k, idx := range list {
			bm := d.outline[idx]
			var b strings.Builder
			fmt.Fprintf(&b, "<< /Title %s /Parent %d 0 R /Dest [%d 0 R /XYZ 0 %s 0]",
				pdfString(bm.title), parentObj, pageNums[bm.page], num(d.H-bm.y+18))
			if k > 0 {
				fmt.Fprintf(&b, " /Prev %d 0 R", items[list[k-1]])
			}
			if k < len(list)-1 {
				fmt.Fprintf(&b, " /Next %d 0 R", items[list[k+1]])
			}
			kids := children(idx)
			if len(kids) > 0 {
				fmt.Fprintf(&b, " /First %d 0 R /Last %d 0 R /Count %d",
					items[kids[0]], items[kids[len(kids)-1]], len(kids))
			}
			b.WriteString(" >>")
			o.put(items[idx], []byte(b.String()))
		}
	}
	emit(top, root)
	for _, idx := range top {
		emit(children(idx), items[idx])
	}
}

// --- literal helpers -------------------------------------------------

// num formats a float the way PDF wants it: no exponent, no trailing
// zero noise, and "0" rather than "-0".
func num(v float64) string {
	if v == 0 || (v > -0.0005 && v < 0.0005) {
		return "0"
	}
	s := fmt.Sprintf("%.3f", v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "-0" {
		return "0"
	}
	return s
}

func pdfString(s string) string {
	// Text strings go out as UTF-16BE with a BOM so accented and
	// non-Latin metadata survives; the escape set is the same either way.
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, r := range s {
		b.WriteString(utf16BE(r))
	}
	b.WriteString(">")
	return b.String()
}

func pdfDate(t time.Time) string {
	_, off := t.Zone()
	sign := "+"
	if off < 0 {
		sign = "-"
		off = -off
	}
	return fmt.Sprintf("(D:%s%s%02d'%02d')",
		t.Format("20060102150405"), sign, off/3600, (off%3600)/60)
}
