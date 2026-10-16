// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the storage-samba plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns the single `samba` LocationType.
// Samba / SMB backups use the hirochachacha/go-smb2 client to
// talk to a remote SMB share over TCP (default port 445).
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

// LocationType this plugin registers. Single value — the SMB
// share surface (\\server\share\path).
const sambaLocationType = "samba"

// sambaAuthGuest, sambaAuthBasic, sambaAuthDomain are the three
// authentication modes the plugin supports. Operators pick one
// via the `auth_method` config field.
//
//   - guest:  authenticate as the SMB guest user (default
//     `nobody`, empty password). go-smb2 v1.x rejects an
//     empty User on the wire — instead the operator supplies
//     a real guest account that the Samba server has been
//     configured with (`guest account = nobody` in smb.conf).
//     The plugin sends those credentials; the server then
//     applies the share's `guest ok = yes` policy and binds
//     the connection to the guest identity. Useful for
//     public shares that don't require login.
//   - basic:  NTLMv2 user/password against the share's local
//     SAM database (workgroup auth).
//   - domain: NTLMv2 user/password with an explicit Windows
//     domain. The Domain field on NTLMInitiator drives the
//     auth realm the server validates against.
const (
	sambaAuthGuest  = "guest"
	sambaAuthBasic  = "basic"
	sambaAuthDomain = "domain"
)

// sambaDefaultGuestUser is the SMB guest user we send on the
// wire when auth_method=guest. Most Samba installations use
// `nobody` (the system account) — operators with a custom
// guest account can override via cfg["guest_user"].
const sambaDefaultGuestUser = "nobody"

// StorageSambaPlugin is the singleton the host dispatches to.
type StorageSambaPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *StorageSambaPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — storage-samba does not register HTTP
// routes. The host's /api/storage-locations CRUD is the single
// admin surface and routes are owned by the host module.
func (p *StorageSambaPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Samba destinations are
// managed from the host's existing /settings/storage-locations
// page — the type picker on that page already lists every
// registered LocationType (built-in + plugin), so a dedicated
// sidemenu entry would just duplicate the surface.
func (p *StorageSambaPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *StorageSambaPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *StorageSambaPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions is intentionally a no-op. The plugin's
// plugin.yaml declares no `adds_permissions`, so the manifest
// slice is empty and there is nothing to register with the
// host's AccessRegistry. Keeping the method implemented (rather
// than relying on a default) makes the bypass RBAC contract
// explicit at the call site.
func (p *StorageSambaPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the Samba client for its single
// LocationType. The cfg["location_type"] key the host always
// passes will be "samba" — the client reads it for
// compatibility with the host's interface contract but does
// not branch on it.
//
// Slug is set to the plugin's own slug so the frontend can
// resolve the Icon field via
// /api/plugins/loader/icon?slug=storage-samba&name=plugin.svg.
func (p *StorageSambaPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			Slug:         p.manifest.Slug,
			LocationType: sambaLocationType,
			DisplayName:  "Samba / SMB",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				// SMB has its own auth handshake (NTLM/NTLMv2),
				// not OAuth. The host's OAuth consent UI does
				// not apply; the operator fills in credentials
				// directly in the form.
				OAuth:                false,
				SupportsListing:      true,
				SupportsDelete:       true,
			},
			ConfigSchema: sambaConfigSchema(),
		},
		NewSambaClient(),
	)
}

// sambaConfigSchema returns the typed config fields the form
// renders. Mirrors the FIELDS list in frontend/src/ConfigForm.tsx
// so the host can render the form *without* the bundle (the
// bundle is purely for advanced UI; the host owns the default
// renderer).
//
// The auth_method field controls which conditional fields are
// shown (the host renders or hides them based on this value):
//
//   - guest:  guest_user (defaults to "nobody"), guest_password
//     (defaults to empty). The server's smb.conf must allow
//     guest on the target share.
//   - basic:  username + password (NTLMv2 workgroup).
//   - domain: username + password + domain (NTLMv2 AD/domain).
func sambaConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:      "server",
			Type:     "string",
			Label:    "Server",
			Required: true,
			Help:     "SMB server hostname or IP. Default port 445; append :PORT to override.",
		},
		{
			Key:      "share",
			Type:     "string",
			Label:    "Share",
			Required: true,
			Help:     "SMB share name (case-sensitive on most servers).",
		},
		{
			Key:         "path",
			Type:        "string",
			Label:       "Path",
			Required:    false,
			Help:        "Optional subdirectory inside the share. Backslashes are server-native; the client preserves them as-is.",
			Placeholder: "Backups",
		},
		{
			Key:      "auth_method",
			Type:     "string",
			Label:    "Authentication",
			Required: true,
			// Default value used by the host's default renderer
			// when the operator doesn't pick one.
			Default: sambaAuthBasic,
			Help:     "One of: \"guest\" (send a guest account, requires `guest ok = yes` on the share), \"basic\" (NTLMv2 workgroup user + password), or \"domain\" (NTLMv2 user + password + explicit Windows domain).",
		},
		{
			Key:      "username",
			Type:     "string",
			Label:    "Username",
			Required: false,
			Help:     "Used when auth_method is basic or domain.",
		},
		{
			Key:      "password",
			Type:     "secret",
			Label:    "Password",
			Required: false,
			Help:     "Used when auth_method is basic or domain. Stored encrypted at rest.",
		},
		{
			Key:      "domain",
			Type:     "string",
			Label:    "Domain",
			Required: false,
			Help:     "Required when auth_method is domain. Optional for basic (workgroup auth).",
		},
		{
			Key:      "guest_user",
			Type:     "string",
			Label:    "Guest user",
			Required: false,
			Help:     "Username sent on the wire when auth_method=guest. Defaults to " + sambaDefaultGuestUser + ". The Samba server's smb.conf must include `guest account = <this user>` and the share must set `guest ok = yes`.",
			Default:  sambaDefaultGuestUser,
		},
		{
			Key:      "guest_password",
			Type:     "secret",
			Label:    "Guest password",
			Required: false,
			Help:     "Password sent on the wire when auth_method=guest. Most guest accounts have an empty password; stored encrypted at rest just in case.",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host
// context so the Samba client can decrypt credentials at call
// time.
func (p *StorageSambaPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-samba.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *StorageSambaPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-samba.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*StorageSambaPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must
// be a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &StorageSambaPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "storage-samba",
		Name:    "Samba",
		Version: "2026.10.01",
	},
}
