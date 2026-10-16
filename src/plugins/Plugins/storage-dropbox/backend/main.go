// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the storage-dropbox plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns the single `dropbox` LocationType.
// Dropbox backups go through Dropbox's HTTP API at
// api.dropboxapi.com (RPC) and content.dropboxapi.com (upload
// + download bytes).
//
// RBAC: storage plugins bypass the per-plugin permission registry
// (see mcharbor-storage-plugins-no-rbac). RegisterPermissions is
// a no-op so the host's plugin loader does not complain about
// missing wiring.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// LocationType this plugin registers. Single value — the
// Dropbox drive surface (`/2/files/upload_session` for chunked
// uploads, `/2/files/list_folder` for listing).
const dropboxLocationType = "dropbox"

// StorageDropboxPlugin is the singleton the host dispatches to.
type StorageDropboxPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *StorageDropboxPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — storage-dropbox does not register HTTP
// routes. The host's /api/storage-locations CRUD is the single
// admin surface and routes are owned by the host module.
func (p *StorageDropboxPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Dropbox destinations
// are managed from the host's existing /settings/storage-locations
// page — the type picker on that page already lists every
// registered LocationType (built-in + plugin), so a dedicated
// sidemenu entry would just duplicate the surface.
func (p *StorageDropboxPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *StorageDropboxPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *StorageDropboxPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions is intentionally a no-op. The plugin's
// plugin.yaml declares no `adds_permissions`, so the manifest
// slice is empty and there is nothing to register with the
// host's AccessRegistry. Keeping the method implemented (rather
// than relying on a default) makes the bypass RBAC contract
// explicit at the call site.
func (p *StorageDropboxPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the Dropbox client for its single
// LocationType. The cfg["location_type"] key the host always
// passes will be "dropbox" — the client reads it for
// compatibility with the host's interface contract but does
// not branch on it.
//
// Slug is set to the plugin's own slug so the frontend can
// resolve the Icon field via
// /api/plugins/loader/icon?slug=storage-dropbox&name=plugin.svg.
func (p *StorageDropboxPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			Slug:         p.manifest.Slug,
			LocationType: dropboxLocationType,
			DisplayName:  "Dropbox",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				// OAuth: false — Dropbox destinations use a long-lived
				// access token from a Dropbox app. The operator pastes
				// the token into the config form; no browser round-trip
				// is required. The host's encryption layer encrypts the
				// token at rest.
				OAuth:                false,
				SupportsListing:      true,
				SupportsDelete:       true,
			},
			ConfigSchema: dropboxConfigSchema(),
		},
		NewDropboxClient(),
	)
}

// dropboxConfigSchema returns the typed config fields the form
// renders. Mirrors the FIELDS list in frontend/src/ConfigForm.tsx
// so the host can render the form *without* the bundle (the
// bundle is purely for advanced UI; the host owns the default
// renderer).
func dropboxConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:      "access_token",
			Type:     "secret",
			Label:    "Dropbox access token",
			Required: true,
			Help:     "Long-lived access token from a Dropbox app. Generate one at https://www.dropbox.com/developers/apps. Stored encrypted at rest.",
		},
		{
			Key:         "base_path",
			Type:        "string",
			Label:       "Base path",
			Required:    false,
			Help:        "Optional folder under the Dropbox app's root to upload into. Leave blank for the app root.",
			Placeholder: "/Backups",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host
// context so the Dropbox client can decrypt credentials at
// call time.
func (p *StorageDropboxPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-dropbox.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *StorageDropboxPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-dropbox.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*StorageDropboxPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must
// be a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &StorageDropboxPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "storage-dropbox",
		Name:    "Dropbox",
		Version: "2026.10.01",
	},
}
