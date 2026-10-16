// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the storage-opencloud plugin.
// It exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: storage-opencloud handles ONLY the `opencloud` location
// type. OpenCloud is a Nextcloud-compatible server (same WebDAV
// protocol surface), so the plugin uses the same WebDAV paths and
// Basic auth flow as the storage-nextcloud plugin. Two auth modes
// are supported: OAuth 2.0 refresh-token or direct app-password
// authentication.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// opencloudLocationType is the single LocationType this plugin
// registers. Matches the form outlet's key match.
const opencloudLocationType = "opencloud"

// StorageOpencloudPlugin is the singleton the host dispatches to.
type StorageOpencloudPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *StorageOpencloudPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — storage-opencloud does not register HTTP
// routes. The host's /api/storage-locations CRUD is the single
// admin surface and routes are owned by the host module.
func (p *StorageOpencloudPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. OpenCloud destinations
// are managed from the host's existing /settings/storage-locations
// page; the host's type picker already lists every registered
// LocationType (built-in + plugin).
func (p *StorageOpencloudPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *StorageOpencloudPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *StorageOpencloudPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions is intentionally a no-op (storage plugins
// bypass the per-plugin permission registry).
func (p *StorageOpencloudPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the OpenCloud client for its single
// LocationType. The slug is set to the plugin's own slug so the
// frontend can resolve the Icon via /api/plugins/loader/icon.
func (p *StorageOpencloudPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			Slug:         p.manifest.Slug,
			LocationType: opencloudLocationType,
			DisplayName:  "OpenCloud",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				OAuth:                true,
				SupportsListing:      true,
				SupportsDelete:       true,
			},
			ConfigSchema: opencloudConfigSchema(),
		},
		NewOpencloudClient(),
	)
}

// opencloudConfigSchema returns the form fields the dialog shows
// when the operator picks OpenCloud. The plugin's ConfigForm.tsx
// renders the same fields directly.
func opencloudConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "host",
			Type:        "string",
			Label:       "OpenCloud base URL",
			Required:    true,
			Help:        "Base URL of the OpenCloud instance, e.g. https://cloud.example.com. No trailing slash. The plugin derives the WebDAV endpoint at call time.",
			Placeholder: "https://cloud.example.com",
		},
		{
			Key:         "username",
			Type:        "string",
			Label:       "Username",
			Required:    true,
			Help:        "OpenCloud login (username or email).",
			Placeholder: "backups",
		},
		{
			Key:      "clientId",
			Type:     "string",
			Label:    "OAuth client ID",
			Required: false,
			Help:     "OAuth 2.0 client_id from OpenCloud admin → Settings → Security → OAuth 2.0 clients. Leave blank if you use an app password instead.",
		},
		{
			Key:      "clientSecret",
			Type:     "secret",
			Label:    "OAuth client secret / app password",
			Required: true,
			Help:     "Either the OAuth 2.0 client_secret, or an app password from OpenCloud Personal settings → Security → App passwords. Both work over HTTP Basic auth.",
		},
		{
			Key:         "basePath",
			Type:        "string",
			Label:       "Folder under the user root",
			Required:    false,
			Help:        "Optional subfolder under the user's OpenCloud Files root.",
			Placeholder: "Backups",
		},
		{
			Key:      "apiPath",
			Type:     "string",
			Label:    "WebDAV path prefix",
			Help:     "Override the WebDAV path. Defaults to /remote.php/dav/files/{username}/.",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host context
// so the OpenCloud client can emit audit events and decrypt
// credentials at call time.
func (p *StorageOpencloudPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-opencloud.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *StorageOpencloudPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-opencloud.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*StorageOpencloudPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up.
var Plugin plugin.Plugin = &StorageOpencloudPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "storage-opencloud",
		Name:    "OpenCloud",
		Version: "2026.10.01",
	},
}
