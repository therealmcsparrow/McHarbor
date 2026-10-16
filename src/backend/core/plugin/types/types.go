// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package types holds the leaf data shapes that appear in plugin
// manifests, spec structs, and host � plugin interfaces. Both
// core/plugin/contract and core/plugin/specs/* depend on this
// package — and nothing in this package depends on them. That
// keeps the dependency graph acyclic, which is required for the
// per-package reach-set hash to line up between host and plugin
// builds (see plan at .references/backup/2026-08-14-1900-shim-core-plugin/).
package types

import "context"

// PermissionDecl declares a permission the plugin owns. The host inserts
// a row in `rbac_permissions` keyed by `Key` with `origin = <slug>`.
// Two plugins cannot share a key; conflict surfaces as ErrPermissionExists.
type PermissionDecl struct {
	Key         string `json:"key" yaml:"key"`
	Description string `json:"description" yaml:"description"`
	Scope       string `json:"scope,omitempty" yaml:"scope,omitempty"` // "global" | "env" | "stack"
	Category    string `json:"category,omitempty" yaml:"category,omitempty"`
	// Presets names the built-in role presets that receive this key
	// when the plugin is installed: any of "viewer", "editor",
	// "operator". Admin holds a wildcard and is never listed. The
	// host adds the key to `role_<preset>` at install time and strips
	// every plugin key from every role at uninstall, so a plugin that
	// omits this is reachable only by admins until an operator edits
	// a role by hand.
	Presets []string `json:"presets,omitempty" yaml:"presets,omitempty"`
}

// ABACRuleDecl declares an ABAC rule the plugin ships with. The host
// inserts a row in `rbac_policies` with `origin = <slug>` so the
// uninstall gate chain can drop rules by origin.
type ABACRuleDecl struct {
	ID     string `json:"id"`
	When   string `json:"when"`
	Then   string `json:"then"` // "allow" | "deny"
	Reason string `json:"reason"`
}

// ConfigFieldDecl describes one editable plugin setting rendered in
// the host's plugin-settings UI. Secret fields are stored encrypted.
type ConfigFieldDecl struct {
	Key         string `json:"key"`
	Type        string `json:"type"` // "string" | "int" | "bool" | "secret"
	Label       string `json:"label"`
	Default     any    `json:"default,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Min         *int   `json:"min,omitempty"`
	Max         *int   `json:"max,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Help        string `json:"help,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	// LabelKey and HelpKey name Label and Help in the plugin's own i18n
	// namespace; the English fields stay as the fallback for the CLI and API.
	LabelKey string `json:"label_key,omitempty" yaml:"label_key,omitempty"`
	HelpKey  string `json:"help_key,omitempty" yaml:"help_key,omitempty"`
}

// OAuthDefaults declares the non-secret half of a provider's OAuth
// device-flow configuration that a plugin ships so the operator signs
// in with no client-ID or endpoint entry — the legitimate "zero-config"
// sign-in. The client ID and the endpoint URLs are NOT secrets: they
// travel in the user's own browser during any normal OAuth login, so a
// plugin may ship them and the host may return them to the client.
//
// A confidential client SECRET is never declared here. If a provider
// needs one, it is a `secret` ConfigFieldDecl stored encrypted in
// plugin_config — it must not be baked into a plugin bundle. Most
// device-grant providers are public clients and need no secret at all.
//
// The host merges these defaults with any per-connection values the
// operator entered by hand (the hand-entered value wins), so declaring
// defaults is purely additive: it removes typing, it never overrides
// an operator's explicit configuration.
type OAuthDefaults struct {
	ClientID  string `json:"clientId,omitempty" yaml:"clientId,omitempty"`
	DeviceURL string `json:"deviceUrl,omitempty" yaml:"deviceUrl,omitempty"`
	TokenURL  string `json:"tokenUrl,omitempty" yaml:"tokenUrl,omitempty"`
	Scopes    string `json:"scopes,omitempty" yaml:"scopes,omitempty"`
}

// SCIMSchema declares one SCIM 2.0 resource type the plugin owns.
// The host inserts the schema into the SCIM ServiceProviderConfig
// after install and removes it on uninstall.
type SCIMSchema struct {
	URI         string              `json:"uri"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Attributes  []SCIMAttributeDecl `json:"attributes"`
}

// SCIMAttributeDecl describes one attribute of a SCIM resource type a plugin exposes.
type SCIMAttributeDecl struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
}

// WidgetDescriptor / NodeDescriptor are passed by value to the host
// registries at Install() time. Concrete shapes are filled in by the
// plugin author; the host stores them verbatim.
type WidgetDescriptor struct {
	Title    string
	Path     string // "/widgets/<key>/<Widget>.tsx" relative to plugin root
	Width    int    // grid columns
	Height   int    // grid rows
	Settings map[string]any
}

// NodeDescriptor is the palette metadata for a workflow node a plugin contributes.
type NodeDescriptor struct {
	Category    string
	DisplayName string
	Description string
	Icon        string
	Inputs      []string
	Outputs     []string
	// Config is a legacy free-form bag. The workflow canvas decodes
	// configSchema as an *ordered list* of fields, so a map cannot
	// express it; the loader only honours a "fields" member holding
	// such a list. New plugins use Fields directly.
	Config map[string]any
	// Fields is the per-node configSchema, in display order. Each
	// entry is one ConfigField as the workflow canvas understands it
	// (key, label, type, required, options, default, showWhen, ...).
	// The host renders it in NodeConfigPanel exactly as it renders a
	// built-in node's schema.
	Fields []map[string]any
	// Requires is the permission the host checks before exposing the
	// node in the palette. Empty string = no extra check (the host's
	// built-in RBAC still applies through the plugin's own
	// permissions: block).
	Requires string
	// Execute runs the node when a workflow reaches it. The engine
	// hands over the node's saved config and the incoming msg, and
	// expects the output port name plus the outgoing msg. A nil
	// Execute registers a palette-only node that passes its msg
	// through unchanged.
	Execute NodeExecFunc
}

// NodeExecInput is what the workflow engine hands a plugin node at run
// time. Msg follows the Node-RED shape the built-in nodes use
// (_msgid, topic, payload, ...); the plugin must not mutate it in
// place — clone, then change the clone.
type NodeExecInput struct {
	// Key is the plugin-local node key the descriptor was registered
	// under, so one executor can serve several nodes.
	Key string
	// Config is the node's saved configuration from the canvas.
	Config map[string]any
	// Msg is the incoming message.
	Msg map[string]any
	// EnvironmentID is the node's own "environment" config value or,
	// when that is empty, the workflow run's fallback environment.
	EnvironmentID string
	// UserID is the account that started the run, when a person did
	// (manual runs). Empty for scheduled, webhook and event runs.
	UserID string
	// WorkflowID identifies the running workflow, for audit lines.
	WorkflowID string
}

// NodeExecOutput is a plugin node's result. Port names one of the
// descriptor's Outputs ("output" and "error" by convention); Msg is
// the outgoing message.
type NodeExecOutput struct {
	Port string
	Msg  map[string]any
}

// NodeExecFunc is the executor signature a plugin attaches to a
// NodeDescriptor. Returning an error fails the node; returning
// Port "error" with a payload routes the failure down the error
// port instead, which is what most nodes want.
type NodeExecFunc func(ctx context.Context, in NodeExecInput) (NodeExecOutput, error)
