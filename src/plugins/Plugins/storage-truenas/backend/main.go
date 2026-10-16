// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the storage-truenas plugin.
// It exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: storage-truenas handles ONLY the `truenas` location type.
// TrueNAS exposes WebDAV via the system WebDAV share, configurable
// per-share on the TrueNAS side. The default WebDAV path is
// `/webdav` at the server root. Operators authenticate with a local
// TrueNAS user + password over HTTP Basic auth.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// truenasLocationType is the single LocationType this plugin
// registers. Matches the form outlet's key match.
const truenasLocationType = "truenas"

// StorageTruenasPlugin is the singleton the host dispatches to.
type StorageTruenasPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

func (p *StorageTruenasPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *StorageTruenasPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

func (p *StorageTruenasPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

func (p *StorageTruenasPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

func (p *StorageTruenasPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

func (p *StorageTruenasPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the TrueNAS client for its single
// LocationType. The slug is set to the plugin's own slug so the
// frontend can resolve the Icon via /api/plugins/loader/icon.
func (p *StorageTruenasPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			Slug:         p.manifest.Slug,
			LocationType: truenasLocationType,
			DisplayName:  "TrueNAS",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				OAuth:                false,
				SupportsListing:      true,
				SupportsDelete:       true,
			},
			ConfigSchema: truenasConfigSchema(),
		},
		NewTruenasClient(),
	)
}

// truenasConfigSchema returns the form fields the dialog shows
// when the operator picks TrueNAS. The plugin's ConfigForm.tsx
// renders the same fields directly.
func truenasConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "host",
			Type:        "string",
			Label:       "TrueNAS base URL",
			Required:    true,
			Help:        "Base URL of the TrueNAS server, e.g. https://nas.local. No trailing slash.",
			Placeholder: "https://nas.local",
		},
		{
			Key:      "username",
			Type:     "string",
			Label:    "Username",
			Required: true,
			Help:     "Local TrueNAS user. The user must have read/write access to the configured WebDAV share.",
		},
		{
			Key:      "password",
			Type:     "secret",
			Label:    "Password",
			Required: true,
			Help:     "Password for the local TrueNAS user. Stored encrypted at rest by the host's AES-256-GCM service.",
		},
		{
			Key:      "basePath",
			Type:     "string",
			Label:    "Subfolder on the WebDAV share",
			Help:     "Optional subfolder under the share's WebDAV root. Leave blank for the share root.",
		},
		{
			Key:      "apiPath",
			Type:     "string",
			Label:    "WebDAV path override",
			Help:     "Override the WebDAV path. Defaults to /webdav. Operators with multiple WebDAV shares can set this to /webdav/{share-name}.",
		},
	}
}

func (p *StorageTruenasPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-truenas.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

func (p *StorageTruenasPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-truenas.shutdown", nil)
	}
	return nil
}

var _ plugin.Plugin = (*StorageTruenasPlugin)(nil)

var Plugin plugin.Plugin = &StorageTruenasPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "storage-truenas",
		Name:    "TrueNAS",
		Version: "2026.10.01",
	},
}
