// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the snapshot-truenas plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope (see core/plugin/adapters.go §SnapshotAdapter*): this
// plugin registers a single SnapshotAdapter under driver="truenas"
// that calls TrueNAS' middleware REST API directly from the host
// process. RequiresAgent=false because HTTPS calls go from the
// host, not the agent transport.
//
// RBAC: snapshot plugins bypass the per-plugin permission registry
// (mirrors mcharbor-storage-plugins-no-rbac). RegisterPermissions
// is a no-op.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// truenasDriver is the Driver key the host uses to dispatch
// snapshot configs to this adapter. Must match
// SnapshotAdapterSpec.Driver.
const truenasDriver = "truenas"

// SnapshotTruenasPlugin is the singleton the host dispatches to.
type SnapshotTruenasPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *SnapshotTruenasPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — snapshot-truenas does not register HTTP
// routes. The host's /api/container-backups/snapshot-configs CRUD
// is the admin surface, and per-config Test Connection is dispatched
// through the snapshot adapter (Capability) — no plugin-owned
// routes needed.
func (p *SnapshotTruenasPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Snapshot configurations
// are managed from the host's existing /settings → Snapshots tab.
func (p *SnapshotTruenasPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *SnapshotTruenasPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *SnapshotTruenasPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions is intentionally a no-op. The plugin's
// plugin.yaml declares no `adds_permissions`, so the manifest
// slice is empty and there is nothing to register with the host's
// AccessRegistry. Keeping the method implemented (rather than
// relying on a default) makes the bypass RBAC contract explicit
// at the call site.
func (p *SnapshotTruenasPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the TrueNAS middleware snapshot
// adapter under driver "truenas". The adapter calls the TrueNAS
// REST API directly from the host process.
//
// Capabilities match the runtime contract:
//   - SupportsMount: false. TrueNAS snapshot access requires a
//     read-only mount via `zfs clone`, which the host doesn't
//     have a generic flow for. Until a streaming SnapshotRead RPC
//     exists, TrueNAS targets fall back to the legacy pause path.
//   - SupportsRemoteTarget: true. TrueNAS is a remote NAS target.
//   - RequiresAgent: false. The plugin makes HTTPS calls from the
//     host process.
func (p *SnapshotTruenasPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Snapshot().Register(
		plugin.SnapshotAdapterSpec{
			Driver:      truenasDriver,
			DisplayName: "TrueNAS",
			Slug:        p.manifest.Slug,
			Icon:        "plugin.svg",
			Description: "TrueNAS SCALE / CORE snapshot repository via the middleware REST API. Filesystem-level ZFS snapshots without `docker pause`.",
			Capabilities: plugin.SnapshotAdapterCapabilities{
				SupportsMount:        false,
				SupportsRemoteTarget: true,
				RequiresAgent:        false,
			},
			ConfigSchema: truenasConfigSchema(),
		},
		NewTruenasClient(),
	)
}

// truenasConfigSchema returns the form fields the admin UI
// renders when an operator picks the TrueNAS driver. Credentials
// are stored encrypted at rest by the host's AES-256-GCM service.
func truenasConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "api_url",
			Type:        "string",
			Label:       "API URL",
			Required:    true,
			LabelKey:    "form.apiUrl.label",
			HelpKey:     "form.apiUrl.help",
			Help:        "Base URL of the TrueNAS web UI middleware API (default https://nas.local/api/v2.0).",
			Placeholder: "https://nas.local/api/v2.0",
		},
		{
			Key:      "api_key",
			Type:     "secret",
			Label:    "API key",
			Required: true,
			LabelKey: "form.apiKey.label",
			HelpKey:  "form.apiKey.help",
			Help:     "API key from Credentials → Local Users → <user> → API Keys. Stored encrypted at rest by the host's encryption service.",
		},
		{
			Key:         "dataset_path",
			Type:        "string",
			Label:       "Dataset path",
			Required:    true,
			LabelKey:    "form.datasetPath.label",
			HelpKey:     "form.datasetPath.help",
			Help:        "Path to the ZFS dataset to snapshot (e.g. tank/backups). Snapshots are taken recursively under this dataset.",
			Placeholder: "tank/backups",
		},
		{
			Key:      "verify_tls",
			Type:     "boolean",
			Label:    "Verify TLS",
			Required: false,
			LabelKey: "form.verifyTls.label",
			HelpKey:  "form.verifyTls.help",
			Help:     "Disable to accept self-signed certificates (common on default TrueNAS installs). Off is equivalent to --insecure.",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host context
// so the TrueNAS client can reach the encryption service at call
// time.
func (p *SnapshotTruenasPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.snapshot-truenas.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *SnapshotTruenasPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.snapshot-truenas.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*SnapshotTruenasPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be
// a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &SnapshotTruenasPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "snapshot-truenas",
		Name:    "TrueNAS",
		Version: "2026.10.01",
	},
}
