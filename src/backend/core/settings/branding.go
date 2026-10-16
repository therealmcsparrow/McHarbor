// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package settings reads operator settings and branding straight from the database for code that has no service handle.
package settings

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
)

// BrandingSettings is the operator-supplied application mark. Two images,
// because the shipped brand already needs two: the wide wordmark the sidebar
// and the login card render, and the square icon the 56px collapsed rail
// renders, where a 3.4:1 lockup cannot fit.
//
// Both are empty by default, which means "use the bundled McHarbor mark". An
// install that only uploads the wordmark keeps the bundled icon, and the
// other way round — the two are independent on purpose, since a customer with
// only a horizontal logo is the common case.
//
// The values are `data:` URIs rather than paths. The settings table is the
// only store an install is guaranteed to have, it is what gets backed up, and
// a data URI needs no second request and no static-file route to serve. The
// cost is the size cap below.
// Each mark also takes an optional dark-theme counterpart. The asymmetry is
// deliberate: the plain field is "the logo" and the `*Dark` field is an
// override, so an operator who uploads one image gets it in both variants and
// only has to fill the dark slot when that one image does not survive the
// flip. Resolution is `<mark>Dark` → `<mark>` → bundled in dark, and
// `<mark>` → bundled in light.
type BrandingSettings struct {
	// LogoWide is the wordmark: sidebar (expanded) and the login card.
	LogoWide string `json:"logoWide"`
	// LogoWideDark replaces LogoWide while the dark variant is active.
	LogoWideDark string `json:"logoWideDark"`
	// LogoIcon is the square mark: the collapsed sidebar rail.
	LogoIcon string `json:"logoIcon"`
	// LogoIconDark replaces LogoIcon while the dark variant is active.
	LogoIconDark string `json:"logoIconDark"`

	// LogoWidePNG and LogoIconPNG are PNG renditions of the two *light*
	// marks, base64 with no `data:` prefix.
	//
	// They exist for consumers that write documents rather than pages — the
	// reporting plugin's PDF and DOCX writers embed raster images only, and
	// a logo is usually uploaded as SVG. The browser rasterises at upload
	// time, where a rasteriser already exists, so neither the host nor a
	// plugin has to grow one.
	//
	// Derived, never authored: only the branding card writes them, only
	// from the light slots (a report prints on white), and re-uploading a
	// mark regenerates its rendition so the two cannot drift.
	LogoWidePNG string `json:"logoWidePng"`
	LogoIconPNG string `json:"logoIconPng"`
}

// BrandingImageTypes is the set of image MIME types an uploaded mark may
// carry. SVG is first because it is what a logo should be, and it renders
// through <img src>, which does not execute script in the document — an
// uploaded SVG cannot reach the app's DOM.
var BrandingImageTypes = []string{
	"image/svg+xml", "image/png", "image/jpeg", "image/webp", "image/gif",
}

// BrandingMaxBytes caps the decoded image. Generous for a logo — a
// hand-optimised SVG wordmark is a couple of kilobytes — and small enough
// that the value stays comfortable inside a settings row and inside the
// public branding response every login hits.
const BrandingMaxBytes = 512 * 1024

// DefaultBrandingSettings returns "no custom mark", which is what every
// install ships with and what clearing an upload restores.
func DefaultBrandingSettings() BrandingSettings {
	return BrandingSettings{}
}

// ValidateBrandingImage checks one uploaded mark. An empty string is valid
// and means "no custom mark" — that is how the UI clears an upload.
//
// The check is deliberately strict about shape: base64 only, one of
// BrandingImageTypes, and decodable. A value that survives this is a value
// the browser can render from an `<img src>` without the frontend having to
// re-validate it.
func ValidateBrandingImage(field, value string) error {
	if value == "" {
		return nil
	}
	if !strings.HasPrefix(value, "data:") {
		return fmt.Errorf("%s must be a data URI", field)
	}
	meta, payload, found := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
	if !found {
		return fmt.Errorf("%s is not a well-formed data URI", field)
	}
	mediaType, encoding, _ := strings.Cut(meta, ";")
	if encoding != "base64" {
		return fmt.Errorf("%s must be base64-encoded", field)
	}
	if !validBrandingImageType(strings.TrimSpace(strings.ToLower(mediaType))) {
		return fmt.Errorf("%s type %q is not one of %v", field, mediaType, BrandingImageTypes)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return fmt.Errorf("%s is not valid base64: %w", field, err)
	}
	if len(decoded) == 0 {
		return fmt.Errorf("%s decodes to an empty image", field)
	}
	if len(decoded) > BrandingMaxBytes {
		return fmt.Errorf("%s is %d bytes, over the %d byte limit", field, len(decoded), BrandingMaxBytes)
	}
	return nil
}

// brandingRowValid picks the right validator for a stored row: the two
// `_png` rows carry bare base64, everything else carries a data URI.
func brandingRowValid(key, value string) error {
	if strings.HasSuffix(key, "_png") {
		return ValidateBrandingRaster(key, value)
	}
	return ValidateBrandingImage(key, value)
}

// ValidateBrandingRaster checks a PNG rendition: bare base64, decodable, a
// real PNG, and inside the same size cap as an uploaded mark. Empty is valid
// and means "no rendition" — which is what a mark that could not be
// rasterised, or was never set, leaves behind.
func ValidateBrandingRaster(field, value string) error {
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "data:") {
		return fmt.Errorf("%s must be bare base64, not a data URI", field)
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("%s is not valid base64: %w", field, err)
	}
	if len(decoded) > BrandingMaxBytes {
		return fmt.Errorf("%s is %d bytes, over the %d byte limit", field, len(decoded), BrandingMaxBytes)
	}
	// The eight-byte PNG signature. The exporters that read this field
	// assume PNG outright, so a JPEG smuggled in here would fail far away
	// from the upload that caused it.
	if !bytes.HasPrefix(decoded, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}) {
		return fmt.Errorf("%s is not a PNG", field)
	}
	return nil
}

// validBrandingImageType reports whether t is one of BrandingImageTypes.
func validBrandingImageType(t string) bool {
	for _, valid := range BrandingImageTypes {
		if t == valid {
			return true
		}
	}
	return false
}

// ReadBrandingSettings reads the stored marks, falling back to the bundled
// brand for any row that is missing or no longer passes validation. A row
// that fails is dropped rather than returned: the frontend renders whatever
// comes back straight into an <img>, so "invalid" has to mean "bundled mark",
// never "broken image".
func ReadBrandingSettings(db *sql.DB) BrandingSettings {
	return ReadBrandingSettingsContext(context.Background(), db)
}

// ReadBrandingSettingsContext is ReadBrandingSettings bound to the caller's context, so a cancelled request stops the branding read.
func ReadBrandingSettingsContext(ctx context.Context, db *sql.DB) BrandingSettings {
	s := DefaultBrandingSettings()
	if db == nil {
		return s
	}

	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings WHERE category = 'branding' LIMIT 1000")
	if err != nil {
		return s
	}
	defer closeRows(rows)

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		if brandingRowValid(key, value) != nil {
			continue
		}
		switch key {
		case "branding_logo_wide":
			s.LogoWide = value
		case "branding_logo_wide_dark":
			s.LogoWideDark = value
		case "branding_logo_icon":
			s.LogoIcon = value
		case "branding_logo_icon_dark":
			s.LogoIconDark = value
		case "branding_logo_wide_png":
			s.LogoWidePNG = value
		case "branding_logo_icon_png":
			s.LogoIconPNG = value
		}
	}
	logRowsErr(rows)

	return s
}
