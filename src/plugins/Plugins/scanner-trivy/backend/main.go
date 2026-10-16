// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the scanner-trivy plugin. It exports
// a single symbol — `Plugin` — that the host loads via `plugin.Open`
// and dispatches to.
//
// Scope: this plugin owns a single ScannerType ("trivy"). The
// trivyClient implements plugin.ImageScannerAdapterClient (Available +
// Test + Scan) by shelling out to the Trivy CLI that ships in the
// McHarbor container image, or that the plugin downloads into its own folder.
//
// The ScannerType is deliberately the same string the built-in scanner
// used, so every `scans.scanner = 'trivy'` row already in an
// operator's history keeps resolving to a real, installed engine after
// the upgrade.
package main

import (
	"context"
	"path/filepath"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// trivyScannerType is the ScannerType this plugin owns. It matches the
// `scanner_type` value migration 080 imports the old
// scanner_trivy_enabled setting under.
const trivyScannerType = "trivy"

// ScannerTrivyPlugin is the singleton the host dispatches to.
type ScannerTrivyPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
	client   *trivyClient
}

// Manifest returns the parsed plugin.yaml.
func (p *ScannerTrivyPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — no HTTP routes; the host's /api/scans subtree is the
// single admin surface for every scanner.
func (p *ScannerTrivyPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is a no-op: configured from Security > Scanners.
func (p *ScannerTrivyPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets — no dashboard widgets. Scan results are surfaced by
// the host's own vulnerability widgets, which read the host's scan
// tables and work the same whichever engine produced the rows.
func (p *ScannerTrivyPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes — no workflow nodes. The host's image-scan node already
// covers this, and shipping a per-engine node would fork every existing
// workflow the moment an operator switched scanners.
func (p *ScannerTrivyPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions — declares no permission keys. The host's
// scans.view / scans.manage / settings.manage already gate the tab that
// configures this plugin and the scans it runs, so there is no discrete
// capability left to grant. Same reasoning as the storage-adapter
// family.
func (p *ScannerTrivyPlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

// RegisterAdapters registers the Trivy client for its single
// ScannerType.
//
// RequiresLocalBinary is true: this adapter is a wrapper around a CLI,
// and if `trivy` is not on PATH there is nothing to wrap. Declaring it
// lets the Scanners tab say "the Trivy binary is missing" instead of a
// bare "unavailable" that sends an operator looking at their network.
func (p *ScannerTrivyPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Scanner().Register(
		plugin.ImageScannerAdapterSpec{
			ScannerType: trivyScannerType,
			Slug:        p.manifest.Slug,
			DisplayName: "Trivy",
			Icon:        "plugin.svg",
			Description: "Aqua Security's Trivy: OS packages and language dependencies, scanned locally.",
			Capabilities: plugin.ImageScannerAdapterCapabilities{
				EncryptedCredentials: false,
				RequiresLocalBinary:  true,
				SupportsTest:         true,
			},
			ConfigSchema: trivyConfigSchema(),
		},
		p.client,
	)
}

// trivyConfigSchema returns the form fields the Scanners tab renders.
//
// Deliberately small. Trivy has hundreds of flags; exposing them here
// would turn a security setting into a CLI reference and make every
// field a support question. These four are the ones that change what
// the *host* ends up storing.
func trivyConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:     "severity",
			Type:    "string",
			Label:   "Minimum severity",
			Default: "LOW",
			Help:    "Comma-separated Trivy severities to report: UNKNOWN, LOW, MEDIUM, HIGH, CRITICAL. Reporting everything and filtering in McHarbor keeps the history complete; raising the floor here permanently discards the findings below it.",
		},
		{
			Key:     "ignore_unfixed",
			Type:    "bool",
			Label:   "Ignore unfixed vulnerabilities",
			Default: false,
			Help:    "Report only vulnerabilities that have a fix available. Useful for a patch-driven workflow, misleading for a risk review — an unfixed critical is still a critical.",
		},
		{
			Key:     "scanners",
			Type:    "string",
			Label:   "Scanners",
			Default: "vuln",
			Help:    "Trivy's own scanner list: vuln, secret, misconfig, license. McHarbor stores vulnerability findings, so anything beyond vuln costs scan time without adding rows.",
		},
		{
			Key:         "insecure_registry",
			Type:        "bool",
			Label:       "Allow insecure registry",
			Default:     false,
			Help:        "Skip TLS verification when pulling image metadata. Only for a registry with a self-signed certificate on a trusted network.",
			Placeholder: "",
		},
	}
}

// Init stashes the host context and, when no trivy is on PATH, starts downloading the pinned
// release into the plugin's own install directory, which the host deletes on uninstall.
func (p *ScannerTrivyPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	p.client.attach(host)
	dir := ""
	if host.WorkDir != "" {
		dir = filepath.Join(host.WorkDir, "plugins", p.manifest.Slug+"@"+p.manifest.Version, "bin")
	}
	p.client.bin = newBinaryInstaller(dir)
	var warn func(string, ...any)
	if host.Logger != nil {
		warn = host.Logger.Warn
	}
	p.client.bin.start(warn)
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.scanner-trivy.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *ScannerTrivyPlugin) Shutdown(_ context.Context) error {
	if p.client.bin != nil {
		p.client.bin.stop()
	}
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.scanner-trivy.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*ScannerTrivyPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be a
// variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &ScannerTrivyPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "scanner-trivy",
		Name:    "Trivy",
		Version: "2026.10.01",
	},
	client: newTrivyClient(),
}
