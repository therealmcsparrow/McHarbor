// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// nfsLocationType is the LocationType this plugin owns. Single
// LocationType — no list.
const nfsLocationType = "nfs"

// NFS security flavors. AUTH_SYS is the legacy UID-based
// "trust the client" flavor; the krb5* flavors use Kerberos 5
// (RPCSEC_GSS) and differ in whether payloads are integrity-
// checked (krb5i) or encrypted (krb5p).
//
//   - sys:   AUTH_SYS / AUTH_UNIX. Available on NFSv3 and
//     NFSv4. The server trusts the client's UID/GID.
//   - krb5:  Kerberos 5 authentication. The server knows the
//     client's principal; payloads are NOT integrity-protected.
//   - krb5i: Kerberos 5 + per-message integrity. Detects
//     tampering but doesn't hide content.
//   - krb5p: Kerberos 5 + per-message privacy (encryption).
//     Strongest; requires an nfs/<host>@REALM principal and
//     the matching keytab on the client.
const (
	nfsSecSys   = "sys"
	nfsSecKrb5  = "krb5"
	nfsSecKrb5i = "krb5i"
	nfsSecKrb5p = "krb5p"
)

// NFS version strings passed through to the kernel mount helper's
// `vers=` option. `auto` lets the kernel negotiate (defaults to
// NFSv4.2 on modern kernels, NFSv3 on older ones).
const (
	nfsVersAuto = "auto"
	nfsVers3    = "3"
	nfsVers4    = "4"
	nfsVers41   = "4.1"
	nfsVers42   = "4.2"
)

// nfsDefaultMountBase is where the plugin creates per-destination
// mount points. McHarbor stores its own state under
// /var/lib/mcharbor/ on the host; we co-locate the NFS mounts there
// for discoverability.
const nfsDefaultMountBase = "/var/lib/mcharbor/mounts/nfs"

// NFSPlugin is the plugin.Plugin implementation the loader
// constructs at boot. It owns one LocationType's worth of
// StorageAdapterSpecs and routes host calls into nfsClient.
type NFSPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the static manifest the loader read from
// plugin.yaml. The host treats this as immutable.
func (p *NFSPlugin) Manifest() plugin.Manifest { return p.manifest }

// Modules the plugin doesn't contribute to are no-ops. Each
// call site runs once at install time during the loader's
// lifecycle walk (loader.go: Manifest → RegisterRoutes →
// RegisterMenu → RegisterWidgets → RegisterNodes →
// RegisterPermissions → RegisterAdapters → Init).
func (p *NFSPlugin) RegisterRoutes(_ plugin.Registry) error    { return nil }
func (p *NFSPlugin) RegisterMenu(_ plugin.MenuRegistry) error  { return nil }
func (p *NFSPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }
func (p *NFSPlugin) RegisterNodes(_ plugin.NodeRegistry) error    { return nil }

// RegisterPermissions is empty for storage plugins — the host's
// settings.manage is sufficient (see mcharbor-storage-plugins-no-rbac).
func (p *NFSPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the single StorageAdapterSpec this
// plugin owns. The host dispatches destinations with location_type
// == "nfs" to the StorageAdapterClient we hand back here.
func (p *NFSPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			LocationType: nfsLocationType,
			DisplayName:  "NFS",
			Slug:         "storage-nfs",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				// NFS has its own auth handshake (RPCSEC_GSS +
				// AUTH_SYS), not OAuth. The host's OAuth consent
				// UI does not apply; the operator fills in
				// principal + keytab directly.
				OAuth:           false,
				SupportsListing: true,
				SupportsDelete:  true,
			},
			ConfigSchema: nfsConfigSchema(),
		},
		NewNFSClient(),
	)
}

// Init runs after all registries are wired up. We stash the host
// context so Shutdown can emit an audit event and so the client
// (if it ever needed plugin config) could read it.
func (p *NFSPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-nfs.init", map[string]any{
			"slug": p.manifest.Slug,
		})
	}
	return nil
}

// Shutdown runs on Disable / Uninstall with a 30s deadline.
func (p *NFSPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-nfs.shutdown", nil)
	}
	return nil
}

// nfsConfigSchema returns the typed config fields the form
// renders. Mirrors the FIELDS list in frontend/src/ConfigForm.tsx.
//
// Conditional fields per sec flavor:
//   - sys:   none beyond server/export/path
//   - krb5/krb5i/krb5p: principal, realm, keytab_path
//
// The mount_base field is advanced — operators leave it blank
// unless they relocated the plugin's mount directory.
//
// Note on Type values: the SDK's ConfigFieldDecl supports
// "string" | "int" | "bool" | "secret" | "select". Select fields
// are rendered as a free-text input by the host's stock form;
// the plugin's own UMD bundle (which already does so for the
// NFS versions) replaces the stock control when present.
func nfsConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:      "server",
			Type:     "string",
			Label:    "Server",
			Required: true,
			Help:     "Hostname or IP of the NFS server (e.g. 10.0.0.5 or nfs.example.com).",
		},
		{
			Key:      "export",
			Type:     "string",
			Label:    "Export Path",
			Required: true,
			Placeholder: "/srv/backups",
			Help:     "Server-side export path exported to this host.",
		},
		{
			Key:      "path",
			Type:     "string",
			Label:    "Subdirectory",
			Required: false,
			Placeholder: "mcharbor",
			Help:     "Optional subdirectory inside the mount. Empty = the export root.",
		},
		{
			Key:      "sec",
			Type:     "select",
			Label:    "Security Flavor",
			Required: true,
			Default:  nfsSecSys,
			Help:     "AUTH_SYS trusts the client UID/GID. krb5/krb5i/krb5p use Kerberos 5 (RPCSEC_GSS) with increasing integrity/encryption.",
		},
		{
			Key:      "version",
			Type:     "select",
			Label:    "NFS Version",
			Required: false,
			Default:  nfsVersAuto,
			Help:     "NFS protocol version. Auto lets the kernel negotiate (usually 4.2 on modern kernels).",
		},
		{
			Key:      "principal",
			Type:     "string",
			Label:    "Kerberos Principal",
			Required: false,
			Placeholder: "nfs/host.example.com",
			Help:     "Required for krb5/krb5i/krb5p. Format: nfs/<host>@REALM.",
		},
		{
			Key:      "realm",
			Type:     "string",
			Label:    "Kerberos Realm",
			Required: false,
			Placeholder: "EXAMPLE.COM",
			Help:     "Required for krb5/krb5i/krb5p. Kerberos realm (uppercase).",
		},
		{
			Key:      "keytab_path",
			Type:     "string",
			Label:    "Keytab Path",
			Required: false,
			Placeholder: "/etc/krb5.keytab",
			Help:     "Required for krb5/krb5i/krb5p. Path to the keytab file containing the nfs/<host> principal. The host process must be able to read it.",
		},
		{
			Key:      "mount_base",
			Type:     "string",
			Label:    "Mount Base",
			Required: false,
			Default:  nfsDefaultMountBase,
			Help:     "Where the plugin creates per-destination mount points. Default: /var/lib/mcharbor/mounts/nfs.",
		},
	}
}

// Plugin is the symbol the loader looks for. plugin.Open
// dlopens the .so and resolves this var; missing/wrong type
// produces a clear error at boot.
var Plugin plugin.Plugin = &NFSPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "storage-nfs",
		Name:    "NFS",
		Version: "2026.10.01",
		Perms:   []string{"storage.adapter.register", "audit.emit"},
	},
}

// main is required for -buildmode=plugin. The loader does not
// invoke it; the side effect of binding `Plugin` is the whole
// reason this file exists.
func main() { _ = Plugin }
