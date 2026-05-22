// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package widgets

// WidgetDefinition describes a dashboard widget type.
type WidgetDefinition struct {
	Key         string    `json:"key"`
	Label       string    `json:"label"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	Source      string    `json:"source"` // "builtin" or "store"
	Component   string    `json:"component"`
	Enabled     bool      `json:"enabled"`
	DefaultSize WidgetSize `json:"defaultSize"`
	MinSize     WidgetSize `json:"minSize"`
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

// InstallWidgetInput is the request body for installing a new widget.
type InstallWidgetInput struct {
	Definition   WidgetDefinition          `json:"definition"`
	Translations map[string]map[string]any `json:"translations"`
}

// UpdateWidgetStateInput is the request body for toggling widget availability.
type UpdateWidgetStateInput struct {
	Enabled *bool `json:"enabled"`
}
