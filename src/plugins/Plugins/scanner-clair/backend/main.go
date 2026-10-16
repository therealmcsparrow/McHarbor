// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the scanner-clair plugin. It exports
// a single symbol — `Plugin` — that the host loads via `plugin.Open`
// and dispatches to.
//
// Scope: this plugin owns a single ScannerType ("clair"). The
// clairClient implements plugin.ImageScannerAdapterClient (Available +
// Test + Scan) by calling an external Clair v4 server's indexer and
// matcher APIs over HTTP.
//
// This is the one first-party scanner that is *not* a local CLI, which
// is why it is the only one with a required config field and the only
// one that can be unavailable for a reason other than a missing
// binary. Its capabilities declare RequiresLocalBinary=false so the
// Scanners tab explains an unavailable row correctly.
//
// The ScannerType is deliberately the same string the built-in scanner
// used, so every `scans.scanner = 'clair'` row already in an
// operator's history keeps resolving after the upgrade.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// clairScannerType is the ScannerType this plugin owns. It matches the
// `scanner_type` value migration 080 imports the old
// scanner_clair_enabled / scanner_clair_url settings under.
const clairScannerType = "clair"

// ScannerClairPlugin is the singleton the host dispatches to.
type ScannerClairPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
	client   *clairClient
}

// Manifest returns the parsed plugin.yaml.
func (p *ScannerClairPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — no HTTP routes; the host's /api/scans subtree is the
// single admin surface for every scanner.
func (p *ScannerClairPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is a no-op: configured from Security > Scanners.
func (p *ScannerClairPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets — no dashboard widgets.
func (p *ScannerClairPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes — no workflow nodes.
func (p *ScannerClairPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions — declares no permission keys. The host's
// scans.view / scans.manage / settings.manage already gate the tab that
// configures this plugin and the scans it runs.
func (p *ScannerClairPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

// RegisterAdapters registers the Clair client for its single
// ScannerType.
//
// EncryptedCredentials is true because the optional bearer token is a
// `secret` field the host encrypts at rest; the flag is what tells the
// form to show the "stored encrypted" affordance rather than leaving
// an operator to guess.
func (p *ScannerClairPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Scanner().Register(
		plugin.ImageScannerAdapterSpec{
			ScannerType: clairScannerType,
			Slug:        p.manifest.Slug,
			DisplayName: "Clair",
			Icon:        "plugin.svg",
			Description: "Red Hat's Clair v4: image indexing and CVE matching against an external Clair server.",
			Capabilities: plugin.ImageScannerAdapterCapabilities{
				EncryptedCredentials: true,
				RequiresLocalBinary:  false,
				SupportsTest:         true,
			},
			ConfigSchema: clairConfigSchema(),
		},
		p.client,
	)
}

// clairConfigSchema returns the form fields the Scanners tab renders.
//
// `server_url` keeps the key the migration writes the old global
// `scanner_clair_url` setting into, so an operator who already had
// Clair configured finds their endpoint intact after the upgrade.
func clairConfigSchema() []plugin.ConfigFieldDecl {
	timeoutMin, timeoutMax := 5, 600
	return []plugin.ConfigFieldDecl{
		{
			Key:         "server_url",
			Type:        "string",
			Label:       "Server URL",
			Required:    true,
			Help:        "Base URL of the Clair v4 server. The indexer and matcher paths are appended automatically.",
			Placeholder: "https://clair.example.com",
		},
		{
			Key:      "auth_token",
			Type:     "secret",
			Label:    "Bearer token",
			Required: false,
			Help:     "Sent as an Authorization: Bearer header. Leave empty for a Clair server with authentication disabled. Stored encrypted; leave blank when editing to keep the stored value.",
		},
		{
			Key:     "timeout_seconds",
			Type:    "int",
			Label:   "Request timeout (seconds)",
			Default: 60,
			Min:     &timeoutMin,
			Max:     &timeoutMax,
			Help:    "Per-request timeout for the index and report calls. Indexing a large image can take a while on a cold Clair; the host's own scan timeout still bounds the whole scan.",
		},
		{
			Key:     "insecure_skip_verify",
			Type:    "bool",
			Label:   "Skip TLS verification",
			Default: false,
			Help:    "Accept any certificate from the Clair server. Only for a self-signed certificate on a trusted network — it disables the check that the server is who it claims to be.",
		},
	}
}

// Init stashes the host context so the client can reach the logger.
func (p *ScannerClairPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.client.attach(host)
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.scanner-clair.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *ScannerClairPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.scanner-clair.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*ScannerClairPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be a
// variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &ScannerClairPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "scanner-clair",
		Name:    "Clair",
		Version: "2026.10.01",
	},
	client: newClairClient(),
}
