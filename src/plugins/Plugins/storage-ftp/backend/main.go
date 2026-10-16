// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// FTP/FTPS plugin: stores backups on an FTP server (RFC 959)
// or FTPS (RFC 4217). The plugin owns one LocationType ("ftp")
// and supports five authentication modes:
//
//   - anonymous:    USER anonymous, PASS guest. Common for
//                    public mirrors.
//   - password:     Plain FTP on TCP/21. Username + password
//                    travel in cleartext. Use only on trusted
//                    networks.
//   - ftps_explicit:Plain control channel that upgrades to TLS
//                    via AUTH TLS (RFC 4217). Standard "FTP over
//                    TLS" — server keeps port 21, TLS is a
//                    session upgrade. Username + password travel
//                    inside the TLS tunnel.
//   - ftps_implicit:Direct TLS connection on TCP/990. The
//                    entire session is TLS-wrapped from the
//                    first byte. The legacy "FTPS" mode still
//                    requested by some enterprise servers.
//   - client_cert:  TLS dial with a client certificate
//                    (tls.Config.Certificates). The server
//                    identifies the client by the cert subject
//                    during the TLS handshake; no password is
//                    sent. Optional username used for servers
//                    that still require USER after the TLS
//                    handshake.
//
// We use the jlaffaye/ftp client (BSD-3-Clause). All five
// modes share the same dial options pattern; the differences
// are in the DialOption list and the Login parameters.

package main

import (
	"context"
	"strconv"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// ftpLocationType is the LocationType this plugin owns. Single
// LocationType — no list.
const ftpLocationType = "ftp"

// Auth methods. Match the values rendered by the dropdown in
// the host's storage location form.
const (
	ftpAuthAnonymous    = "anonymous"
	ftpAuthPassword     = "password"
	ftpAuthFTPSExplicit = "ftps_explicit"
	ftpAuthFTPSImplicit = "ftps_implicit"
	ftpAuthClientCert   = "client_cert"
)

// ftpDefaultConnectTimeout is the per-connection TLS/TCP handshake
// cap. Operators can override per-destination. Default ports
// (21 / 990) live in ftp_client.go with the rest of the dial
// configuration; this file declares only config-shape defaults.
const ftpDefaultConnectTimeout = "30s"

// FTPPlugin is the plugin.Plugin implementation the loader
// constructs at boot. It owns one LocationType's worth of
// StorageAdapterSpecs and routes host calls into ftpClient.
type FTPPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the static manifest the loader read from
// plugin.yaml. The host treats this as immutable.
func (p *FTPPlugin) Manifest() plugin.Manifest { return p.manifest }

// Modules the plugin doesn't contribute to are no-ops. Each
// call site runs once at install time during the loader's
// lifecycle walk (loader.go: Manifest → RegisterRoutes →
// RegisterMenu → RegisterWidgets → RegisterNodes →
// RegisterPermissions → RegisterAdapters → Init).
func (p *FTPPlugin) RegisterRoutes(_ plugin.Registry) error       { return nil }
func (p *FTPPlugin) RegisterMenu(_ plugin.MenuRegistry) error     { return nil }
func (p *FTPPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }
func (p *FTPPlugin) RegisterNodes(_ plugin.NodeRegistry) error     { return nil }

// RegisterPermissions is empty for storage plugins — the host's
// settings.manage is sufficient (see mcharbor-storage-plugins-no-rbac).
func (p *FTPPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the single StorageAdapterSpec this
// plugin owns. The host dispatches destinations with location_type
// == "ftp" to the StorageAdapterClient we hand back here.
func (p *FTPPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			LocationType: ftpLocationType,
			DisplayName:  "FTP/FTPS",
			Slug:         "storage-ftp",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				// FTP variants have their own auth handshake
				// (USER/PASS, AUTH TLS, mTLS). Not OAuth.
				OAuth:           false,
				SupportsListing: true,
				SupportsDelete:  true,
			},
			ConfigSchema: ftpConfigSchema(),
		},
		NewFTPClient(),
	)
}

// Init runs after all registries are wired up. We stash the host
// context so Shutdown can emit an audit event.
func (p *FTPPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-ftp.init", map[string]any{
			"slug": p.manifest.Slug,
		})
	}
	return nil
}

// Shutdown runs on Disable / Uninstall with a 30s deadline.
func (p *FTPPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-ftp.shutdown", nil)
	}
	return nil
}

// ftpConfigSchema returns the typed config fields the form
// renders. Mirrors the FIELDS list in
// frontend/src/ConfigForm.tsx.
//
// Conditional fields per auth_method:
//   - anonymous:    none beyond server/port (no user/pass)
//   - password:     username + password
//   - ftps_explicit:username + password (TLS is part of the
//                   dial, not the credentials)
//   - ftps_implicit:username + password
//   - client_cert:  client_cert_path + client_key_path; username
//                   optional (server-dependent)
//
// tls_skip_verify is advanced — defaults to false. Operators
// enable it only when their server uses a self-signed cert
// they trust.
//
// Note on Type values: the SDK's ConfigFieldDecl supports
// "string" | "int" | "bool" | "secret" | "select". The
// plugin's own UMD bundle (ConfigForm.tsx) provides the
// full conditional rendering that the host's stock form
// cannot express.
func ftpConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "server",
			Type:        "string",
			Label:       "Server",
			Required:    true,
			Placeholder: "ftp.example.com or 10.0.0.5",
			Help:        "Hostname or IP of the FTP server. The host process must have network access to it (TCP/21 or TCP/990).",
		},
		{
			Key:         "port",
			Type:        "string",
			Label:       "Port",
			Required:    false,
			Default:     strconv.Itoa(ftpDefaultPortPlain),
			Placeholder: "21",
			Help:        "TCP port. Plain / explicit TLS use 21. Implicit FTPS uses 990.",
		},
		{
			Key:      "auth_method",
			Type:     "select",
			Label:    "Authentication Method",
			Required: true,
			Default:  ftpAuthPassword,
			Help:     "How the client proves its identity to the server. Anonymous uses built-in USER/PASS; client_cert uses a TLS certificate.",
		},
		{
			Key:         "username",
			Type:        "string",
			Label:       "Username",
			Required:    false,
			Placeholder: "ftpuser",
			Help:        "FTP username. Required for password / FTPS variants. For client_cert auth, optional — server-dependent.",
		},
		{
			Key:         "password",
			Type:        "secret",
			Label:       "Password",
			Required:    false,
			Placeholder: "••••••••",
			Help:        "FTP password. Required for password / FTPS variants. Stored encrypted at rest.",
		},
		{
			Key:      "tls_skip_verify",
			Type:     "bool",
			Label:    "Skip TLS Verification",
			Required: false,
			Default:  false,
			Help:     "Disable server-cert verification. Only enable for self-signed certs you trust.",
		},
		{
			Key:         "client_cert_path",
			Type:        "string",
			Label:       "Client Certificate Path",
			Required:    false,
			Placeholder: "/etc/ftp/client.crt",
			Help:        "PEM-encoded client certificate. The host process must be able to read it.",
		},
		{
			Key:         "client_key_path",
			Type:        "string",
			Label:       "Client Key Path",
			Required:    false,
			Placeholder: "/etc/ftp/client.key",
			Help:        "PEM-encoded private key matching the client certificate. The host process must be able to read it.",
		},
		{
			Key:         "path",
			Type:        "string",
			Label:       "Subdirectory",
			Required:    false,
			Placeholder: "mcharbor",
			Help:        "Optional subdirectory on the FTP server. Leave empty to write to the user's home directory.",
		},
		{
			Key:         "connect_timeout",
			Type:        "string",
			Label:       "Connect Timeout",
			Required:    false,
			Default:     ftpDefaultConnectTimeout,
			Placeholder: "30s",
			Help:        "Maximum time to wait for the TCP/TLS handshake. Minimum 5s. Use a longer value for high-latency networks.",
		},
	}
}

// Plugin is the symbol the loader looks for. plugin.Open
// dlopens the .so and resolves this var; missing/wrong type
// produces a clear error at boot.
var Plugin plugin.Plugin = &FTPPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "storage-ftp",
		Name:    "FTP/FTPS",
		Version: "2026.10.01",
		Perms:   []string{"storage.adapter.register", "audit.emit"},
	},
}

// main is required for -buildmode=plugin. The loader does not
// invoke it; the side effect of binding `Plugin` is the whole
// reason this file exists.
func main() { _ = Plugin }
