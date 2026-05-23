// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package customnodes

// ConfigField describes a single config field for a custom node.
type ConfigField struct {
	Key      string            `json:"key"`
	Label    string            `json:"label"`
	Type     string            `json:"type"`
	Required bool              `json:"required"`
	Options  []ConfigOption    `json:"options,omitempty"`
	Default  any               `json:"default,omitempty"`
	ShowWhen map[string]string `json:"showWhen,omitempty"`
}

// ConfigOption is a value/label pair for select fields.
type ConfigOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// CustomNodeDefinition describes a user-created workflow node stored on disk.
type CustomNodeDefinition struct {
	Key          string        `json:"key"`
	Label        string        `json:"label"`
	Category     string        `json:"category"`
	Description  string        `json:"description"`
	Icon         string        `json:"icon"`
	Source       string        `json:"source"` // always "custom"
	ConfigSchema []ConfigField `json:"configSchema"`
	InputPorts   []string      `json:"inputPorts"`
	OutputPorts  []string      `json:"outputPorts"`
}

// CustomNodeWithCode bundles a definition with its script and translations.
type CustomNodeWithCode struct {
	CustomNodeDefinition
	Code         string                    `json:"code"`
	Translations map[string]map[string]any `json:"translations"`
}

// CreateCustomNodeInput is the request body for creating a custom node.
type CreateCustomNodeInput struct {
	Definition   CustomNodeDefinition      `json:"definition"`
	Code         string                    `json:"code"`
	Translations map[string]map[string]any `json:"translations"`
}

// UpdateCustomNodeInput is the request body for updating a custom node.
type UpdateCustomNodeInput struct {
	Definition   CustomNodeDefinition      `json:"definition"`
	Code         string                    `json:"code"`
	Translations map[string]map[string]any `json:"translations"`
}

// TestCustomNodeInput is the request body for testing a custom node.
type TestCustomNodeInput struct {
	Code   string         `json:"code"`
	Config map[string]any `json:"config"`
	Msg    map[string]any `json:"msg"`
}

// TestCustomNodeOutput is the response from testing a custom node.
type TestCustomNodeOutput struct {
	Port     string         `json:"port"`
	Msg      map[string]any `json:"msg"`
	Logs     []string       `json:"logs"`
	Error    string         `json:"error,omitempty"`
	Duration int64          `json:"duration"`
}
