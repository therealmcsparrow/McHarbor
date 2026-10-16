// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docx

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// Font embedding.
//
// Without this, a recipient who does not have Inter Tight installed gets
// Word's substitution — in practice a serif — and the exported Word file
// stops matching the PDF. A board pack that looks different depending on
// who opens it is not a board pack.
//
// OOXML embeds fonts as "obfuscated" TrueType (.odttf): the first 32
// bytes are XORed with a key derived from a per-font GUID, which is
// recorded in fontTable.xml as w:fontKey. The scheme is documented in
// ECMA-376 Part 1 §17.8.1 — it is not encryption and is not claimed to
// be; it exists so a font cannot be trivially lifted out of a document.
//
// Only fonts under a licence that permits embedding may be shipped this
// way. Inter Tight and JetBrains Mono are both SIL Open Font License
// 1.1, which permits it; the licence text ships alongside them in
// assets/fonts/OFL.txt.

type embeddedFont struct {
	family  string
	panose  string
	regular []byte
	bold    []byte
}

type odttfPart struct {
	name string
	data []byte
}

// EmbedFont registers a font family for embedding. `regular` and `bold`
// are raw TTF bytes; either may be nil.
func (d *Doc) EmbedFont(family, panose string, regular, bold []byte) {
	if family == "" || (regular == nil && bold == nil) {
		return
	}
	if panose == "" {
		panose = "020B0604020202020204" // generic swiss/sans
	}
	d.embedded = append(d.embedded, embeddedFont{
		family:  family,
		panose:  panose,
		regular: regular,
		bold:    bold,
	})
}

// buildFontParts obfuscates each registered face and returns the parts
// to write, the fontTable XML, and the relationships part for it.
//
// The r:id values inside fontTable.xml resolve against
// word/_rels/fontTable.xml.rels — not document.xml.rels. Putting them in
// the document's relationships makes Word reject the whole package as
// corrupt, with no indication of which part is at fault.
func (d *Doc) buildFontParts() ([]odttfPart, string, string) {
	if len(d.embedded) == 0 {
		return nil, "", ""
	}
	var parts []odttfPart
	var rels []relationship
	var table strings.Builder
	table.WriteString(xmlDecl)
	table.WriteString(`<w:fonts ` + nsAttrs + `>`)

	// Word needs the substitution metrics for any face it may fall back
	// to, so the three standard families are declared even though they
	// are not embedded.
	for _, f := range []struct{ name, panose, family string }{
		{"Calibri", "020F0502020204030204", "swiss"},
		{"Arial", "020B0604020202020204", "swiss"},
		{"Consolas", "020B0609020204030204", "modern"},
	} {
		fmt.Fprintf(&table,
			`<w:font w:name="%s"><w:panose1 w:val="%s"/><w:charset w:val="00"/><w:family w:val="%s"/><w:pitch w:val="variable"/></w:font>`,
			f.name, f.panose, f.family)
	}

	n := 0
	for _, ef := range d.embedded {
		fmt.Fprintf(&table,
			`<w:font w:name="%s"><w:panose1 w:val="%s"/><w:charset w:val="00"/><w:family w:val="swiss"/><w:pitch w:val="variable"/>`,
			esc(ef.family), ef.panose)
		for _, face := range []struct {
			tag  string
			data []byte
		}{{"embedRegular", ef.regular}, {"embedBold", ef.bold}} {
			if face.data == nil {
				continue
			}
			n++
			key, err := fontKey()
			if err != nil {
				continue
			}
			relID := fmt.Sprintf("rIdFont%d", n)
			name := fmt.Sprintf("fonts/font%d.odttf", n)
			parts = append(parts, odttfPart{name: name, data: obfuscate(face.data, key)})
			rels = append(rels, relationship{
				relID,
				"http://schemas.openxmlformats.org/officeDocument/2006/relationships/font",
				name,
			})
			fmt.Fprintf(&table, `<w:%s r:id="%s" w:fontKey="{%s}" w:subsetted="false"/>`, face.tag, relID, key)
		}
		table.WriteString(`</w:font>`)
	}
	table.WriteString(`</w:fonts>`)

	var relsXML strings.Builder
	relsXML.WriteString(xmlDecl)
	relsXML.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, r := range rels {
		fmt.Fprintf(&relsXML, `<Relationship Id="%s" Type="%s" Target="%s"/>`, r.id, r.typ, r.target)
	}
	relsXML.WriteString(`</Relationships>`)

	return parts, table.String(), relsXML.String()
}

// fontKey returns an uppercase GUID string in the canonical
// 8-4-4-4-12 form, without braces.
func fontKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// Stamp version 4 / variant RFC 4122 so the value is a well-formed
	// GUID, which is what Word writes and what validators expect.
	b[6] = (b[6] & 0x0F) | 0x40
	b[8] = (b[8] & 0x3F) | 0x80
	return strings.ToUpper(fmt.Sprintf("%X-%X-%X-%X-%X",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])), nil
}

// obfuscate applies the ECMA-376 §17.8.1 transform: the 16 key bytes are
// the GUID's bytes in reverse order, XORed over the first 32 bytes of
// the font file (i.e. twice through the key).
func obfuscate(font []byte, key string) []byte {
	hexOnly := strings.NewReplacer("-", "", "{", "", "}", "").Replace(key)
	if len(hexOnly) != 32 {
		return font
	}
	var k [16]byte
	for i := 0; i < 16; i++ {
		var v int
		if _, err := fmt.Sscanf(hexOnly[i*2:i*2+2], "%02X", &v); err != nil {
			return font
		}
		k[15-i] = byte(v)
	}
	out := make([]byte, len(font))
	copy(out, font)
	limit := 32
	if len(out) < limit {
		limit = len(out)
	}
	for i := 0; i < limit; i++ {
		out[i] ^= k[i%16]
	}
	return out
}
