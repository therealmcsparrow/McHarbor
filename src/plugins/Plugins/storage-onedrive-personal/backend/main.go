// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the storage-onedrive-personal
// plugin. It exports a single symbol — `Plugin` — that the host
// loads via `plugin.Open` and dispatches to.
//
// Scope (see mcharbor-storage-microsoft-split): this plugin
// handles ONLY the OneDrive Personal location type. The consumer
// endpoint at graph.microsoft.com/v1.0/me/drive is distinct from
// the work / school tenant endpoints covered by OneDrive Business
// and SharePoint Online, so we keep them separate rather than
// reinvent an account-type picker inside a single plugin.
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

// StorageOneDrivePersonalPlugin is the singleton the host
// dispatches to.
type StorageOneDrivePersonalPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *StorageOneDrivePersonalPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — storage-onedrive-personal does not register
// HTTP routes. The host's /api/storage-locations CRUD is the
// single admin surface and routes are owned by the host module.
func (p *StorageOneDrivePersonalPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Personal OneDrive
// destinations are managed from the host's existing Storage
// Locations settings page.
func (p *StorageOneDrivePersonalPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *StorageOneDrivePersonalPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *StorageOneDrivePersonalPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions is intentionally a no-op. The plugin's
// plugin.yaml declares no `adds_permissions`, so the manifest
// slice is empty and there is nothing to register with the host's
// AccessRegistry. Keeping the method implemented (rather than
// relying on a default) makes the bypass RBAC contract explicit
// at the call site.
func (p *StorageOneDrivePersonalPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the Personal client for the
// onedrive_personal location type only. The host's storage
// dispatcher consults the AdapterRegistry before falling back to
// any built-in implementation.
//
// The client is constructed without the host context here; Init
// runs after RegisterAdapters and stashes the host context for
// later credential decryption.
//
// ConfigSchema mirrors Home Assistant's OneDrive flow: an Azure AD
// app registered as "Personal Microsoft accounts only" with the
// "Mobile and desktop applications" public-client platform has no
// client_secret. We therefore expose client_secret as optional and
// let the FormLayout field guide the operator through the right
// app-registration path. Confidential ("Web") registrations still
// work and just supply a non-empty client_secret.
func (p *StorageOneDrivePersonalPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			Slug:         p.manifest.Slug,
			LocationType: "onedrive_personal",
			DisplayName:  "OneDrive (Personal)",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				OAuth:                true,
				SupportsListing:      true,
				SupportsDelete:       true,
			},
			ConfigSchema: []plugin.ConfigFieldDecl{
				{
					Key:     "drive_id",
					Type:    "string",
					Label:   "Drive ID",
					Help:    "Optional. Leave blank to use the default drive returned by Microsoft for the signed-in account.",
					Default: "",
				},
				{
					Key:     "base_path",
					Type:    "string",
					Label:   "Base path",
					Help:    "Optional. Folder inside the drive, e.g. /Backups/2026. Leave blank to use the drive root.",
					Default: "",
				},
				{
					Key:         "client_id",
					Type:        "string",
					Label:       "Application (client) ID",
					Help:        "Optional. Leave blank to use the plugin's embedded McSparrow app registration. Fill in your own client ID if you registered a custom Azure AD app for OneDrive Personal.",
					Required:    false,
					Placeholder: "00000000-0000-0000-0000-000000000000",
				},
				{
					Key:      "client_secret",
					Type:     "secret",
					Label:    "Client secret",
					Help:     "Optional. Only needed for confidential (Web) app registrations. Leave blank for public-client (Mobile and desktop applications) registrations.",
					Required: false,
				},
			},
		},
		NewOneDrivePersonalClient(),
	)
}

// Init is called after RegisterAdapters. We stash the host
// context so the Personal client can decrypt credentials at
// call time. The host context is not directly available to the
// adapter (it lives behind the AdapterRegistry), so the client
// holds its own reference; encryption / decryption for the
// Personal plugin is delegated to the host via host.Encryption
// (see onedrive_personal_client.go for the call sites).
func (p *StorageOneDrivePersonalPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-onedrive-personal.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *StorageOneDrivePersonalPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-onedrive-personal.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*StorageOneDrivePersonalPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be
// a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &StorageOneDrivePersonalPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "storage-onedrive-personal",
		Name:    "OneDrive (Personal)",
		Version: "2026.10.01",
	},
}
