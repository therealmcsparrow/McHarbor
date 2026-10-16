// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the storage-box plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns the single `box` LocationType. Box
// backups go through Box's REST API at api.box.com and the
// upload host upload.box.com.
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

// LocationType this plugin registers. Single value — the Box
// drive surface (`/2.0/users/me` for delegated flows,
// `/2.0/folders/{folder_id}` for file ops).
const boxLocationType = "box"

// StorageBoxPlugin is the singleton the host dispatches to.
type StorageBoxPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *StorageBoxPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — storage-box does not register HTTP routes.
// The host's /api/storage-locations CRUD is the single admin
// surface and routes are owned by the host module.
func (p *StorageBoxPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Box destinations are
// managed from the host's existing /settings/storage-locations
// page — the type picker on that page already lists every
// registered LocationType (built-in + plugin), so a dedicated
// sidemenu entry would just duplicate the surface.
func (p *StorageBoxPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *StorageBoxPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *StorageBoxPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions is intentionally a no-op. The plugin's
// plugin.yaml declares no `adds_permissions`, so the manifest
// slice is empty and there is nothing to register with the
// host's AccessRegistry. Keeping the method implemented (rather
// than relying on a default) makes the bypass RBAC contract
// explicit at the call site.
func (p *StorageBoxPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the Box client for its single
// LocationType. The cfg["location_type"] key the host always
// passes will be "box" — the client reads it for compatibility
// with the host's interface contract but does not branch on it.
//
// Slug is set to the plugin's own slug so the frontend can
// resolve the Icon field via
// /api/plugins/loader/icon?slug=storage-box&name=box.svg.
func (p *StorageBoxPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			Slug:         p.manifest.Slug,
			LocationType: boxLocationType,
			DisplayName:  "Box",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				// Box supports both delegated (refresh_token) and
				// app-only (client_credentials) auth. The host's
				// OAuth consent UI drives the delegated flow; the
				// config form drives app-only. We mark OAuth=true
				// so the host exposes the "Connect" button.
				OAuth:                true,
				SupportsListing:      true,
				SupportsDelete:       true,
			},
			ConfigSchema: boxConfigSchema(),
		},
		NewBoxClient(),
	)
}

// boxConfigSchema returns the typed config fields the form
// renders. Mirrors the FIELDS list in frontend/src/ConfigForm.tsx
// so the host can render the form *without* the bundle (the
// bundle is purely for advanced UI; the host owns the default
// renderer).
//
// folder_id is optional and falls back to "0" (Box root) when
// blank — see boxFolderID.
//
// box_subject_type + box_subject_id are only consulted when the
// operator chooses app-only auth (no refresh_token is present
// at Test time). For delegated auth they stay empty.
func boxConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:      "client_id",
			Type:     "string",
			Label:    "Box client ID",
			Required: true,
			Help:     "Box app's client_id from the developer console. The app needs Server Authentication OR a redirect-URI registered for OAuth.",
		},
		{
			Key:      "client_secret",
			Type:     "secret",
			Label:    "Box client secret",
			Required: true,
			Help:     "Box app's client_secret. Stored encrypted at rest.",
		},
		{
			Key:         "folder_id",
			Type:        "string",
			Label:       "Folder ID",
			Required:    false,
			Help:        "Optional Box folder ID. Leave blank to upload to the user's root folder (id=0).",
			Placeholder: "0",
		},
		{
			Key:      "box_subject_type",
			Type:     "string",
			Label:    "App auth subject type",
			Required: false,
			Help:     "Only used for app-only (client_credentials) auth. Either \"enterprise\" (a Box enterprise ID) or \"user\" (a managed user ID). Leave blank to skip app-only auth and use delegated OAuth via the Connect button instead.",
			Default:  "enterprise",
		},
		{
			Key:      "box_subject_id",
			Type:     "string",
			Label:    "App auth subject ID",
			Required: false,
			Help:     "Only used for app-only (client_credentials) auth. Box enterprise ID (when box_subject_type=enterprise) or user ID (when box_subject_type=user). Leave blank if the operator grants consent interactively instead.",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host
// context so the Box client can decrypt credentials at call
// time.
func (p *StorageBoxPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-box.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *StorageBoxPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-box.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*StorageBoxPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must
// be a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &StorageBoxPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "storage-box",
		Name:    "Box",
		Version: "2026.10.01",
	},
}
