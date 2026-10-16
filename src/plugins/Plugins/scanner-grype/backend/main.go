// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the scanner-grype plugin. It exports
// a single symbol — `Plugin` — that the host loads via `plugin.Open`
// and dispatches to.
//
// Scope: this plugin owns a single ScannerType ("grype"). The
// grypeClient implements plugin.ImageScannerAdapterClient (Available +
// Test + Scan) by shelling out to the Grype CLI that ships in the
// McHarbor container image, or that the plugin downloads into its own folder.
//
// The ScannerType is deliberately the same string the built-in scanner
// used, so every `scans.scanner = 'grype'` row already in an
// operator's history keeps resolving to a real, installed engine after
// the upgrade.
package main

import (
	"context"
	"path/filepath"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// grypeScannerType is the ScannerType this plugin owns. It matches the
// `scanner_type` value migration 080 imports the old
// scanner_grype_enabled setting under.
const grypeScannerType = "grype"

// ScannerGrypePlugin is the singleton the host dispatches to.
type ScannerGrypePlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
	client   *grypeClient
}

// Manifest returns the parsed plugin.yaml.
func (p *ScannerGrypePlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — no HTTP routes; the host's /api/scans subtree is the
// single admin surface for every scanner.
func (p *ScannerGrypePlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is a no-op: configured from Security > Scanners.
func (p *ScannerGrypePlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets — no dashboard widgets. The host's vulnerability
// widgets read the host's scan tables and work the same whichever
// engine produced the rows.
func (p *ScannerGrypePlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes — no workflow nodes. The host's image-scan node covers
// this, and a per-engine node would fork every existing workflow the
// moment an operator switched scanners.
func (p *ScannerGrypePlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions — declares no permission keys. The host's
// scans.view / scans.manage / settings.manage already gate the tab that
// configures this plugin and the scans it runs.
func (p *ScannerGrypePlugin) RegisterPermissions(_ plugin.AccessRegistry) error { return nil }

// RegisterAdapters registers the Grype client for its single
// ScannerType.
//
// RequiresLocalBinary is true: this adapter is a wrapper around a CLI,
// and if `grype` is not on PATH there is nothing to wrap. Declaring it
// lets the Scanners tab say "the Grype binary is missing" instead of a
// bare "unavailable" that sends an operator looking at their network.
func (p *ScannerGrypePlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Scanner().Register(
		plugin.ImageScannerAdapterSpec{
			ScannerType: grypeScannerType,
			Slug:        p.manifest.Slug,
			DisplayName: "Grype",
			Icon:        "plugin.svg",
			Description: "Anchore's Grype: OS and language package vulnerabilities, scanned locally.",
			Capabilities: plugin.ImageScannerAdapterCapabilities{
				EncryptedCredentials: false,
				RequiresLocalBinary:  true,
				SupportsTest:         true,
			},
			ConfigSchema: grypeConfigSchema(),
		},
		p.client,
	)
}

// grypeConfigSchema returns the form fields the Scanners tab renders.
//
// Three fields, chosen on the same principle as Trivy's: expose what
// changes the rows the host ends up storing, not the CLI's full flag
// surface.
func grypeConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:     "only_fixed",
			Type:    "bool",
			Label:   "Only report fixed vulnerabilities",
			Default: false,
			Help:    "Report only vulnerabilities with a fix available. Useful for a patch-driven workflow, misleading for a risk review — an unfixed critical is still a critical.",
		},
		{
			Key:     "scope",
			Type:    "string",
			Label:   "Scope",
			Default: "squashed",
			Help:    "One of squashed or all-layers. `squashed` scans the final filesystem, which is what actually runs; `all-layers` also reports packages deleted in a later layer, which inflates counts with things that are not present.",
		},
		{
			Key:     "add_cpes_if_none",
			Type:    "bool",
			Label:   "Infer CPEs when missing",
			Default: false,
			Help:    "Generate CPEs for packages whose ecosystem has no advisory feed. Finds more, and produces more false positives — leave off unless you are chasing a specific gap.",
		},
	}
}

// Init stashes the host context and, when no grype is on PATH, starts downloading the pinned
// release into the plugin's own install directory, which the host deletes on uninstall.
func (p *ScannerGrypePlugin) Init(_ context.Context, host plugin.HostContext) error {
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
		_ = host.Audit.Emit("plugin.scanner-grype.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *ScannerGrypePlugin) Shutdown(_ context.Context) error {
	if p.client.bin != nil {
		p.client.bin.stop()
	}
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.scanner-grype.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*ScannerGrypePlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be a
// variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &ScannerGrypePlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "scanner-grype",
		Name:    "Grype",
		Version: "2026.10.01",
	},
	client: newGrypeClient(),
}
