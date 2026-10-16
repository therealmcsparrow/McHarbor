// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the storage-owncloud plugin.
// It exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: storage-owncloud handles ONLY the `owncloud` location
// type. ownCloud (Classic) uses the same WebDAV surface as Nextcloud
// (the upstream project that Nextcloud forked from in 2016) so the
// plugin uses the same WebDAV paths and Basic auth flow as
// storage-nextcloud / storage-opencloud. Two auth modes are
// supported: OAuth 2.0 refresh-token or direct app-password.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// owncloudLocationType is the single LocationType this plugin
// registers. Matches the form outlet's key match.
const owncloudLocationType = "owncloud"

// StorageOwncloudPlugin is the singleton the host dispatches to.
type StorageOwncloudPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *StorageOwncloudPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *StorageOwncloudPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

func (p *StorageOwncloudPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

func (p *StorageOwncloudPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

func (p *StorageOwncloudPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

func (p *StorageOwncloudPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the ownCloud client for its single
// LocationType. The slug is set to the plugin's own slug so the
// frontend can resolve the Icon via /api/plugins/loader/icon.
func (p *StorageOwncloudPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			Slug:         p.manifest.Slug,
			LocationType: owncloudLocationType,
			DisplayName:  "ownCloud",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				OAuth:                true,
				SupportsListing:      true,
				SupportsDelete:       true,
			},
			ConfigSchema: owncloudConfigSchema(),
		},
		NewOwncloudClient(),
	)
}

// owncloudConfigSchema returns the form fields the dialog shows
// when the operator picks ownCloud. The plugin's ConfigForm.tsx
// renders the same fields directly.
func owncloudConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "host",
			Type:        "string",
			Label:       "ownCloud base URL",
			Required:    true,
			Help:        "Base URL of the ownCloud instance, e.g. https://cloud.example.com. No trailing slash. The plugin derives the WebDAV endpoint at call time.",
			Placeholder: "https://cloud.example.com",
		},
		{
			Key:         "username",
			Type:        "string",
			Label:       "Username",
			Required:    true,
			Help:        "ownCloud login (username or email).",
			Placeholder: "backups",
		},
		{
			Key:      "clientId",
			Type:     "string",
			Label:    "OAuth client ID",
			Required: false,
			Help:     "OAuth 2.0 client_id from ownCloud admin → Security → OAuth 2.0. Leave blank if you use an app password instead.",
		},
		{
			Key:      "clientSecret",
			Type:     "secret",
			Label:    "OAuth client secret / app password",
			Required: true,
			Help:     "Either the OAuth 2.0 client_secret, or an app password from ownCloud Personal settings → Security → App passwords. Both work over HTTP Basic auth.",
		},
		{
			Key:         "basePath",
			Type:        "string",
			Label:       "Folder under the user root",
			Required:    false,
			Help:        "Optional subfolder under the user's ownCloud Files root.",
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

func (p *StorageOwncloudPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-owncloud.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

func (p *StorageOwncloudPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-owncloud.shutdown", nil)
	}
	return nil
}

var _ plugin.Plugin = (*StorageOwncloudPlugin)(nil)

var Plugin plugin.Plugin = &StorageOwncloudPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "storage-owncloud",
		Name:    "ownCloud",
		Version: "2026.10.01",
	},
}
