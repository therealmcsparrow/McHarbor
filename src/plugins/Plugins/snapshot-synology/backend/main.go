// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the snapshot-synology plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope (see core/plugin/adapters.go §SnapshotAdapter*): this
// plugin registers a single SnapshotAdapter under driver="synology"
// that calls Synology DSM's REST API directly from the host
// process. RequiresAgent=false because HTTP calls go from the
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

// synologyDriver is the Driver key the host uses to dispatch
// snapshot configs to this adapter. Must match
// SnapshotAdapterSpec.Driver.
const synologyDriver = "synology"

// SnapshotSynologyPlugin is the singleton the host dispatches to.
type SnapshotSynologyPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *SnapshotSynologyPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — snapshot-synology does not register HTTP
// routes. The host's /api/container-backups/snapshot-configs CRUD
// is the admin surface, and per-config Test Connection is dispatched
// through the snapshot adapter (Capability) — no plugin-owned
// routes needed.
func (p *SnapshotSynologyPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Snapshot configurations
// are managed from the host's existing /settings → Snapshots tab.
func (p *SnapshotSynologyPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *SnapshotSynologyPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *SnapshotSynologyPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions is intentionally a no-op. The plugin's
// plugin.yaml declares no `adds_permissions`, so the manifest
// slice is empty and there is nothing to register with the host's
// AccessRegistry. Keeping the method implemented (rather than
// relying on a default) makes the bypass RBAC contract explicit
// at the call site.
func (p *SnapshotSynologyPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the Synology DSM snapshot adapter
// under driver "synology". The adapter calls the DSM REST API
// directly from the host process.
//
// Capabilities match the runtime contract:
//   - SupportsMount: false. Synology's snapshot repositories don't
//     expose a stable local mount path the host could read from
//     while the snapshot is held. The host falls back to the
//     legacy pause path for Synology targets until a streaming
//     SnapshotRead RPC exists.
//   - SupportsRemoteTarget: true. Synology is a remote NAS target.
//   - RequiresAgent: false. The plugin makes HTTP calls from the
//     host process.
func (p *SnapshotSynologyPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Snapshot().Register(
		plugin.SnapshotAdapterSpec{
			Driver:      synologyDriver,
			DisplayName: "Synology DSM",
			Slug:        p.manifest.Slug,
			Icon:        "plugin.svg",
			Description: "Synology DSM snapshot repository via the DSM 7.x REST API. Filesystem-level snapshots without `docker pause`.",
			Capabilities: plugin.SnapshotAdapterCapabilities{
				SupportsMount:        false,
				SupportsRemoteTarget: true,
				RequiresAgent:        false,
			},
			ConfigSchema: synologyConfigSchema(),
		},
		NewSynologyClient(),
	)
}

// synologyConfigSchema returns the form fields the admin UI
// renders when an operator picks the Synology driver. Credentials
// are stored encrypted at rest by the host's AES-256-GCM service.
func synologyConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "dsm_url",
			Type:        "string",
			Label:       "DSM URL",
			Required:    true,
			LabelKey:    "form.dsmUrl.label",
			HelpKey:     "form.dsmUrl.help",
			Help:        "Base URL of the Synology DSM web UI (e.g. https://nas.local:5001).",
			Placeholder: "https://nas.local:5001",
		},
		{
			Key:      "username",
			Type:     "string",
			Label:    "Username",
			Required: true,
			LabelKey: "form.username.label",
			HelpKey:  "form.username.help",
			Help:     "DSM user account with permission to manage snapshots. Use an account-scoped app password for tighter security.",
		},
		{
			Key:      "password",
			Type:     "secret",
			Label:    "Password",
			Required: true,
			LabelKey: "form.password.label",
			HelpKey:  "form.password.help",
			Help:     "DSM password or app-specific password. Stored encrypted at rest by the host's encryption service.",
		},
		{
			Key:         "share_name",
			Type:        "string",
			Label:       "Share",
			Required:    true,
			LabelKey:    "form.shareName.label",
			HelpKey:     "form.shareName.help",
			Help:        "Shared folder name to snapshot (e.g. /volume1/backups).",
			Placeholder: "backups",
		},
		{
			Key:      "ignore_certificate_errors",
			Type:     "boolean",
			Label:    "Ignore certificate errors",
			Required: false,
			LabelKey: "form.ignoreCert.label",
			HelpKey:  "form.ignoreCert.help",
			Help:     "When true, accept self-signed certificates on the DSM endpoint. Required for default Synology certificates.",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host context
// so the Synology client can reach the encryption service at call
// time.
func (p *SnapshotSynologyPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.snapshot-synology.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *SnapshotSynologyPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.snapshot-synology.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*SnapshotSynologyPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be
// a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &SnapshotSynologyPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "snapshot-synology",
		Name:    "Synology DSM",
		Version: "2026.10.01",
	},
}
