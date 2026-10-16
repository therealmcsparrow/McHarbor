// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package widgetsig defines the manifest a downloaded dashboard widget is signed over,
// shared by the widgets module that verifies it and pluginsigner that produces it.
package widgetsig

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// Prefix is the manifest's first line; a new manifest layout takes a new prefix so
// a signature can never be read under a layout it was not made for.
const Prefix = "mcharbor-widget-v1"

// Size is a grid size, encoded as the catalog entry encodes it.
type Size struct {
	W int `json:"w"`
	H int `json:"h"`
}

// Metadata is everything about a widget the operator sees before its code runs. It
// unmarshals straight from a catalog entry, so the signer and the verifier read the
// same fields the same way.
type Metadata struct {
	Label        string                    `json:"label"`
	Description  string                    `json:"description"`
	Icon         string                    `json:"icon"`
	DefaultSize  Size                      `json:"defaultSize"`
	MinSize      Size                      `json:"minSize"`
	Translations map[string]map[string]any `json:"translations"`
}

// MetadataDigest returns the lowercase hex sha256 of m's canonical JSON: the fields
// in declaration order, map keys sorted, no HTML escaping, no trailing newline, and
// an absent translation set encoded as {}.
func MetadataDigest(m Metadata) (string, error) {
	if m.Translations == nil {
		m.Translations = map[string]map[string]any{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return "", fmt.Errorf("encoding widget metadata: %w", err)
	}
	sum := sha256.Sum256(bytes.TrimRight(buf.Bytes(), "\n"))
	return hex.EncodeToString(sum[:]), nil
}

// BundleDigest returns the lowercase hex sha256 of the bundle bytes.
func BundleDigest(bundle []byte) string {
	sum := sha256.Sum256(bundle)
	return hex.EncodeToString(sum[:])
}

// Manifest returns the bytes a widget signature covers. Binding the key and version
// stops a signed bundle being installed under another key or rolled back, and the
// metadata digest stops unsigned label, icon or translations riding along with it.
func Manifest(key, version, bundleDigest, metadataDigest string) []byte {
	return []byte(strings.Join([]string{Prefix, key, version, bundleDigest, metadataDigest}, "\n"))
}

// EntryManifest builds the manifest for a widget from its parts in one call.
func EntryManifest(key, version string, bundle []byte, m Metadata) ([]byte, error) {
	meta, err := MetadataDigest(m)
	if err != nil {
		return nil, err
	}
	return Manifest(key, version, BundleDigest(bundle), meta), nil
}
