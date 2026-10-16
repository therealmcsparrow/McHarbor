// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the storage-google-drive plugin.
// It exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin handles ONLY the `google_drive` location
// type. It speaks Google's OAuth2 (`/o/oauth2/v2/auth` and
// `/oauth2.googleapis.com/token`) for consent and the Drive API
// v3 for storage. No client SDK is imported — raw `net/http` keeps
// the dependency surface aligned with the other storage plugins
// (Dropbox, Box, SharePoint Online, OneDrive *).
//
// RBAC: storage plugins bypass the per-plugin permission registry
// (see mcharbor-storage-plugins-no-rbac). RegisterPermissions is a
// no-op so the host's plugin loader does not complain about missing
// wiring.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// googleDriveLocationType is the single LocationType this plugin
// registers. It matches the pre-existing host scaffolding's
// `validLocationTypes["google_drive"]` entry — that entry is
// removed from the host in the same change set.
const googleDriveLocationType = "google_drive"

// StorageGoogleDrivePlugin is the singleton the host dispatches to.
type StorageGoogleDrivePlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *StorageGoogleDrivePlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — storage-google-drive does not register HTTP
// routes. The host's /api/storage-locations CRUD is the single
// admin surface and routes are owned by the host module.
func (p *StorageGoogleDrivePlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Google Drive destinations
// are managed from the host's existing /settings/storage-locations
// page — the type picker on that page already lists every
// registered LocationType (built-in + plugin), so a dedicated
// sidemenu entry would just duplicate the surface. The plugin
// still owns the config_form + setup_guide form slots keyed to
// "google_drive", which the host loads when the operator picks
// the Google Drive tile.
func (p *StorageGoogleDrivePlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *StorageGoogleDrivePlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *StorageGoogleDrivePlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions is intentionally a no-op. The plugin's
// plugin.yaml declares no `adds_permissions`, so the manifest
// slice is empty and there is nothing to register with the host's
// AccessRegistry. Keeping the method implemented (rather than
// relying on a default) makes the bypass RBAC contract explicit
// at the call site.
func (p *StorageGoogleDrivePlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the Google Drive client for its
// single LocationType. The cfg["location_type"] key the host
// always passes will be "google_drive" — the client reads it for
// compatibility with the host's interface contract but does not
// branch on it. Slug is set to the plugin's own slug so the
// frontend can resolve the Icon field via
// /api/plugins/loader/icon?slug=storage-google-drive&name=plugin.svg.
func (p *StorageGoogleDrivePlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			Slug:         p.manifest.Slug,
			LocationType: googleDriveLocationType,
			DisplayName:  "Google Drive",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				OAuth:                true,
				SupportsListing:      true,
				SupportsDelete:       true,
			},
			ConfigSchema: googleDriveConfigSchema(),
		},
		NewGoogleDriveClient(),
	)
}

// googleDriveConfigSchema returns the form fields the admin UI
// renders when an operator picks this destination type.
//
// client_id and client_secret are required — they come from the
// Google Cloud console OAuth client registration. The host's
// OAuth consent flow writes the refresh_token into the cfg map
// silently so the form never shows it.
//
// folder_id is optional and defaults to the Drive root. Operators
// either leave it blank (uploads land in My Drive root) or paste
// a specific folder ID from drive.google.com (the tail end of the
// folder URL after /folders/). base_path from the host's first-
// class column is treated as an alias when folder_id is blank so
// installations using the host's hardcoded form fields continue
// to work.
func googleDriveConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "client_id",
			Type:        "string",
			Label:       "OAuth client ID",
			Required:    true,
			Help:        "Application (client) ID from the Google Cloud console → APIs & Services → Credentials. The OAuth client must be of type \"Web application\" with the redirect URI set to your McHarbor host's /api/storage-locations/oauth/callback.",
			Placeholder: "000000000000-xxxxxxxxxxxxxxxxxxxxxxxx.apps.googleusercontent.com",
		},
		{
			Key:      "client_secret",
			Type:     "secret",
			Label:    "OAuth client secret",
			Required: true,
			Help:     "Client secret from the same OAuth client. Stored encrypted at rest by the host's AES-256-GCM service.",
		},
		{
			Key:         "folder_id",
			Type:        "string",
			Label:       "Drive folder ID",
			Required:    false,
			Help:        "Optional. ID of the Drive folder where backups should land. Leave blank to use the Drive root. From drive.google.com → open the folder → copy the ID from the URL after /folders/.",
			Placeholder: "1AbC2dEf3GhI4jKl5mNo6pQr7StU8vWxY",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host
// context so the Google Drive client can emit audit events and
// decrypt credentials at call time.
func (p *StorageGoogleDrivePlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-google-drive.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *StorageGoogleDrivePlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-google-drive.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*StorageGoogleDrivePlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must
// be a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &StorageGoogleDrivePlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "storage-google-drive",
		Name:    "Google Drive",
		Version: "2026.10.01",
	},
}
