// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the storage-onedrive-business
// plugin. It exports a single symbol — `Plugin` — that the host
// loads via `plugin.Open` and dispatches to.
//
// Scope (see mcharbor-storage-microsoft-split): this plugin
// handles OneDrive for Business ONLY. The consumer endpoint lives
// in storage-onedrive-personal; SharePoint document libraries
// live in storage-sharepoint-online. This plugin is therefore a
// single-LocationType adapter — there is no cfg["location_type"]
// branch in the client.
//
// RBAC: storage plugins bypass the per-plugin permission registry
// (see mcharbor-storage-plugins-no-rbac). RegisterPermissions is a
// no-op so the host's plugin loader does not complain about
// missing wiring.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// LocationType this plugin registers. Single value — the onedrive
// business drive surface (/users/{upn}/drive or /drives/{drive_id}).
const oneDriveBusinessLocationType = "onedrive_business"

// StorageOneDriveBusinessPlugin is the singleton the host dispatches to.
type StorageOneDriveBusinessPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *StorageOneDriveBusinessPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — storage-onedrive-business does not register
// HTTP routes. The host's /api/storage-locations CRUD is the
// single admin surface and routes are owned by the host module.
func (p *StorageOneDriveBusinessPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. OneDrive for Business
// destinations are managed from the host's existing
// /settings/storage-locations page — the type picker on that page
// already lists every registered LocationType (built-in + plugin),
// so a dedicated sidemenu entry would just duplicate the surface.
// The plugin still owns the config_form + setup_guide form slots
// keyed to "onedrive_business", which the host loads when the
// operator picks the OneDrive for Business tile.
func (p *StorageOneDriveBusinessPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *StorageOneDriveBusinessPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *StorageOneDriveBusinessPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions is intentionally a no-op. The plugin's
// plugin.yaml declares no `adds_permissions`, so the manifest
// slice is empty and there is nothing to register with the host's
// AccessRegistry. Keeping the method implemented (rather than
// relying on a default) makes the bypass RBAC contract explicit
// at the call site.
func (p *StorageOneDriveBusinessPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the OneDrive for Business client for
// its single LocationType. The cfg["location_type"] key the host
// always passes will be "onedrive_business" — the client reads it
// for compatibility with the host's interface contract but does
// not branch on it. Slug is set to the plugin's own slug so the
// frontend can resolve the Icon field via
// /api/plugins/loader/icon?slug=storage-onedrive-business&name=plugin.svg.
func (p *StorageOneDriveBusinessPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			Slug:         p.manifest.Slug,
			LocationType: oneDriveBusinessLocationType,
			DisplayName:  "OneDrive for Business",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				OAuth:                true,
				SupportsListing:      true,
				SupportsDelete:       true,
			},
			ConfigSchema: oneDriveBusinessConfigSchema(),
		},
		NewOneDriveBusinessClient(),
	)
}

// oneDriveBusinessConfigSchema returns the form fields the admin UI
// renders when an operator picks this destination type.
//
// drive_id is optional and falls back to /users/{upn}/drive when
// blank. /me/drive is the final fallback when both drive_id and
// upn are blank, but only under delegated (refresh_token) auth.
//
// base_path is intentionally not prefilled — operators either
// leave it blank (drive root) or enter an explicit Graph path
// like /Backups/2026. There is no default because the wrong
// default here would silently route uploads to a folder the
// operator didn't choose.
func oneDriveBusinessConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "tenant_id",
			Type:        "string",
			Label:       "Azure AD tenant ID",
			Required:    true,
			Help:        "Microsoft Entra tenant ID (GUID). From Entra admin center → Overview. The OAuth consent flow redirects to login.microsoftonline.com/{tenant_id}/…",
			Placeholder: "00000000-0000-0000-0000-000000000000",
		},
		{
			Key:         "client_id",
			Type:        "string",
			Label:       "Azure AD application (client) ID",
			Required:    true,
			Help:        "Application (client) ID of the Entra app registration with Files.ReadWrite delegated permission and a Web redirect URI pointing at this McHarbor host.",
			Placeholder: "00000000-0000-0000-0000-000000000000",
		},
		{
			Key:      "client_secret",
			Type:     "secret",
			Label:    "Azure AD client secret",
			Required: true,
			Help:     "Client secret value from Entra admin center → App registrations → Certificates & secrets. Stored encrypted at rest.",
		},
		{
			Key:         "drive_id",
			Type:        "string",
			Label:       "Drive ID",
			Required:    false,
			Help:        "Optional. Fully-qualified Graph drive identifier. Leave blank to use the service account's drive (resolved via UPN or /me/drive).",
			Placeholder: "b!abc123…",
		},
		{
			Key:      "base_path",
			Type:     "string",
			Label:    "Base path inside the drive",
			Required: false,
			Help:     "Optional. Graph path inside the drive, e.g. /Backups/2026. Leave blank to use the drive root. No default — operators either leave blank or enter an explicit path.",
		},
		{
			Key:         "upn",
			Type:        "string",
			Label:       "User Principal Name (UPN)",
			Required:    false,
			Help:        "Optional. UPN of the service account, e.g. backups@contoso.onmicrosoft.com. When drive_id is also blank, uploads target /users/{upn}/drive; if both upn and drive_id are blank, /me/drive (the refresh token's user) is used as a fallback.",
			Placeholder: "backups@contoso.onmicrosoft.com",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host
// context so the OneDrive for Business client can decrypt
// credentials at call time.
func (p *StorageOneDriveBusinessPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-onedrive-business.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *StorageOneDriveBusinessPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-onedrive-business.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*StorageOneDriveBusinessPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be
// a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &StorageOneDriveBusinessPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "storage-onedrive-business",
		Name:    "OneDrive for Business",
		Version: "2026.10.01",
	},
}
