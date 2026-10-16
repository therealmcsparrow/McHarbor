// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package services

import (
	"strings"
	"unicode"
)

// MaxSlugLen bounds one path segment.
//
// OneDrive and SharePoint reject a segment over 255 characters, and S3 caps
// the whole key at 1024 bytes. 96 leaves room for the rest of the path while
// staying long enough that real names survive intact.
const MaxSlugLen = 96

// windowsReservedNames cannot be a path segment on an SMB share, whatever the
// extension. A backup folder called CON is not a folder that can be created.
var windowsReservedNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// Slugify turns an operator-chosen name into one path segment every
// destination accepts.
//
// McHarbor writes the same key to a local filesystem, an S3 bucket, an SMB
// share and OneDrive, and the strictest of those decides what is legal:
// lowercase ASCII, no leading or trailing separators, no reserved device
// name, bounded length. A name that slugs to nothing returns "", and the
// caller substitutes an id rather than writing an empty segment.
func Slugify(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	lastDash := false
	for _, r := range strings.TrimSpace(value) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '-' || r == '_' || r == '.' || unicode.IsSpace(r):
			// Runs collapse to a single dash, so "Home  Assistant" and
			// "Home-Assistant" land in the same folder rather than two.
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			// Anything else — accents, CJK, emoji, path separators — is not
			// reliably representable across these destinations. Dropping it
			// rather than transliterating keeps the rule one sentence long.
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	slug := strings.Trim(b.String(), "-")
	if len(slug) > MaxSlugLen {
		slug = strings.Trim(slug[:MaxSlugLen], "-")
	}
	if windowsReservedNames[slug] {
		// Suffixed rather than rejected: the operator named it, and "con-"
		// still reads as theirs.
		slug += "-x"
	}
	return slug
}

// SlugifyOr falls back when a name slugs to nothing — an id, or a machine
// named entirely in a script this cannot carry across destinations.
func SlugifyOr(value, fallback string) string {
	if slug := Slugify(value); slug != "" {
		return slug
	}
	if slug := Slugify(fallback); slug != "" {
		return slug
	}
	return "unnamed"
}
