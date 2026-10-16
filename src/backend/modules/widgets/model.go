// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package widgets

// WidgetDefinition describes a dashboard widget type.
type WidgetDefinition struct {
	Key         string     `json:"key"`
	Label       string     `json:"label"`
	Category    string     `json:"category"`
	Description string     `json:"description"`
	Icon        string     `json:"icon"`
	Source      string     `json:"source"` // "builtin", "plugin" or "downloaded"
	Component   string     `json:"component"`
	Enabled     bool       `json:"enabled"`
	DefaultSize WidgetSize `json:"defaultSize"`
	MinSize     WidgetSize `json:"minSize"`
	Version     string     `json:"version,omitempty"`
	// Verified is set on a downloaded widget only when its stored bundle still
	// verifies against a trust key; the frontend refuses to load one without it.
	Verified bool `json:"verified,omitempty"`
}

// CatalogEntry is one widget in a catalog's widgets/ directory. Its signature covers
// the widgetsig manifest (key, version, bundle hash, metadata hash), not the bundle alone.
type CatalogEntry struct {
	Key          string                    `json:"key"`
	Version      string                    `json:"version"`
	Label        string                    `json:"label"`
	Description  string                    `json:"description"`
	Icon         string                    `json:"icon"`
	DefaultSize  WidgetSize                `json:"defaultSize"`
	MinSize      WidgetSize                `json:"minSize"`
	Bundle       string                    `json:"bundle"`
	SHA256       string                    `json:"sha256,omitempty"`
	Signature    string                    `json:"signature,omitempty"`
	Translations map[string]map[string]any `json:"translations,omitempty"`
}

// CatalogWidget is a catalog entry as the Store lists it: what it is, whether a
// trust key verifies its bundle, and whether it is installed already.
type CatalogWidget struct {
	Key              string     `json:"key"`
	Version          string     `json:"version"`
	Label            string     `json:"label"`
	Description      string     `json:"description"`
	Icon             string     `json:"icon"`
	DefaultSize      WidgetSize `json:"defaultSize"`
	Signed           bool       `json:"signed"`
	Installed        bool       `json:"installed"`
	InstalledVersion string     `json:"installedVersion,omitempty"`
}

// WidgetSize defines width/height in grid units.
type WidgetSize struct {
	W int `json:"w"`
	H int `json:"h"`
}

// WidgetDefinitionWithI18n bundles a definition with all translations.
type WidgetDefinitionWithI18n struct {
	WidgetDefinition
	Translations map[string]map[string]any `json:"translations"` // lang → {label, description, ...}
}

// UpdateWidgetStateInput is the request body for toggling widget availability.
type UpdateWidgetStateInput struct {
	Enabled *bool `json:"enabled"`
}
