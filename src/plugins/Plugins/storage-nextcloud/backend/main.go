// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the storage-nextcloud plugin.
// It exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin handles ONLY the `nextcloud` location type.
// The Nextcloud client speaks plain WebDAV (RFC 4918) against the
// server's `/remote.php/dav/files/{username}/` endpoint. OAuth 2.0
// (refresh_token grant) is supported for the consent flow; direct
// app-password authentication is the simpler fallback for
// self-hosted instances.
//
// RBAC: storage plugins bypass the per-plugin permission registry
// (see mcharbor-storage-plugins-no-rbac), so RegisterPermissions is a
// no-op.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// nextcloudLocationType is the single LocationType this plugin
// registers. Matches the host's filter in storage_locations/handler.go
// (validLocationTypes) and the form outlet's key match.
const nextcloudLocationType = "nextcloud"

// StorageNextcloudPlugin is the singleton the host dispatches to.
type StorageNextcloudPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *StorageNextcloudPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — storage-nextcloud does not register HTTP
// routes. The host's /api/storage-locations CRUD is the single
// admin surface and routes are owned by the host module.
func (p *StorageNextcloudPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Nextcloud destinations
// are managed from the host's existing /settings/storage-locations
// page — the type picker on that page already lists every
// registered LocationType (built-in + plugin), so a dedicated
// sidemenu entry would just duplicate the surface. The plugin
// still owns the config_form + setup_guide form slots keyed to
// "nextcloud", which the host loads when the operator picks the
// Nextcloud tile.
func (p *StorageNextcloudPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *StorageNextcloudPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *StorageNextcloudPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions is intentionally a no-op. The plugin's
// plugin.yaml declares no `adds_permissions`, so the manifest slice
// is empty and there is nothing to register with the host's
// AccessRegistry. Keeping the method implemented (rather than
// relying on a default) makes the bypass RBAC contract explicit at
// the call site.
func (p *StorageNextcloudPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the Nextcloud client for its single
// LocationType. The slug is set to the plugin's own slug so the
// frontend can resolve the Icon field via
// /api/plugins/loader/icon?slug=storage-nextcloud&name=plugin.svg.
func (p *StorageNextcloudPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			Slug:         p.manifest.Slug,
			LocationType: nextcloudLocationType,
			DisplayName:  "Nextcloud",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				OAuth:                true,
				SupportsListing:      true,
				SupportsDelete:       true,
			},
			ConfigSchema: nextcloudConfigSchema(),
		},
		NewNextcloudClient(),
	)
}

// nextcloudConfigSchema returns the form fields the dialog shows
// when the operator picks Nextcloud. The plugin's ConfigForm.tsx
// renders the same fields directly so the UMD bundle stays the
// source of truth for the operator UX. The host's typed save path
// persists the standard columns (host, port, username, password,
// clientId, clientSecret, refreshToken, basePath); `apiPath` is
// plugin-specific and lands in the per-row Config JSON map.
func nextcloudConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "host",
			Type:        "string",
			Label:       "Nextcloud base URL",
			Required:    true,
			Help:        "Base URL of the Nextcloud instance, e.g. https://cloud.example.com. No trailing slash. The plugin derives the WebDAV endpoint ({base}/remote.php/dav/files/{username}/) at call time.",
			Placeholder: "https://cloud.example.com",
		},
		{
			Key:         "username",
			Type:        "string",
			Label:       "Username",
			Required:    true,
			Help:        "Nextcloud login (username or email). Used to scope the WebDAV path under /remote.php/dav/files/{username}/ and identifies the OAuth user during refresh.",
			Placeholder: "backups",
		},
		{
			Key:      "clientId",
			Type:     "string",
			Label:    "OAuth client ID",
			Required: false,
			Help:     "OAuth 2.0 client_id from the Nextcloud admin → Settings → Security → OAuth 2.0 clients page. Leave blank if you use an app password instead (see client_secret below).",
		},
		{
			Key:      "clientSecret",
			Type:     "secret",
			Label:    "OAuth client secret / app password",
			Required: true,
			Help:     "Two valid options: (1) the OAuth 2.0 client_secret from the Nextcloud OAuth client config, or (2) an app password generated in Nextcloud Personal settings → Security → App passwords. The plugin uses HTTP Basic auth on every WebDAV request, so an app password works directly without OAuth setup. Stored encrypted at rest.",
		},
		{
			Key:         "basePath",
			Type:        "string",
			Label:       "Folder under the user root",
			Required:    false,
			Help:        "Optional subfolder under the user's Nextcloud Files root, e.g. Backups/2026. Leave blank for the user root. Forward slashes are treated as folder separators.",
			Placeholder: "Backups",
		},
		{
			Key:      "apiPath",
			Type:     "string",
			Label:    "WebDAV path prefix",
			Help:     "Override the WebDAV path. Defaults to /remote.php/dav/files/{username}/. Set this only if your Nextcloud sits behind a path prefix or a reverse proxy that rewrites the WebDAV path.",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host context
// so the Nextcloud client can emit audit events and decrypt
// credentials at call time.
func (p *StorageNextcloudPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-nextcloud.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *StorageNextcloudPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-nextcloud.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*StorageNextcloudPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be
// a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &StorageNextcloudPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "storage-nextcloud",
		Name:    "Nextcloud",
		Version: "2026.10.01",
	},
}
