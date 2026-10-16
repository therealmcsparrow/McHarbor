// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the storage-sftp plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns a single LocationType ("sftp"). The
// four authentication methods (password / publickey /
// keyboard_interactive / agent) are encoded in the
// per-destination cfg["auth_method"] field — the backend reads
// it, the frontend renders the conditional form layout. There
// is no plugin-side branch on LocationType.
//
// RBAC: storage plugins bypass the per-plugin permission
// registry (see mcharbor-storage-plugins-no-rbac).
// RegisterPermissions is a no-op so the host's plugin loader
// does not complain about missing wiring.
package main

import (
	"context"
	"strconv"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// sftpLocationType is the LocationType this plugin owns.
// Single LocationType — no list.
const sftpLocationType = "sftp"

// Auth methods. Match the values rendered by the dropdown in
// the host's storage location form (see frontend ConfigForm).
const (
	sftpAuthPassword           = "password"
	sftpAuthPublicKey          = "publickey"
	sftpAuthKeyboardInteractive = "keyboard_interactive"
	sftpAuthAgent              = "agent"
)

// sftpDefaultPort is the standard SSH port. Configurable per
// destination via cfg["port"].
const sftpDefaultPort = 22

// sftpDefaultConnectTimeout is the per-connection SSH
// handshake cap. Operators can override per destination via
// cfg["connect_timeout"]. 5s minimum is enforced in the
// client.
const sftpDefaultConnectTimeout = "30s"

// SFTPPlugin is the singleton the host dispatches to. It
// owns one LocationType's worth of StorageAdapterSpecs and
// routes host calls into the sftpClient.
type SFTPPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the static manifest the loader read from
// plugin.yaml. The host treats this as immutable.
func (p *SFTPPlugin) Manifest() plugin.Manifest { return p.manifest }

// Modules the plugin doesn't contribute to are no-ops. Each
// call site runs once at install time during the loader's
// lifecycle walk (loader.go: Manifest → RegisterRoutes →
// RegisterMenu → RegisterWidgets → RegisterNodes →
// RegisterPermissions → RegisterAdapters → Init).
func (p *SFTPPlugin) RegisterRoutes(_ plugin.Registry) error         { return nil }
func (p *SFTPPlugin) RegisterMenu(_ plugin.MenuRegistry) error       { return nil }
func (p *SFTPPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }
func (p *SFTPPlugin) RegisterNodes(_ plugin.NodeRegistry) error     { return nil }

// RegisterPermissions is intentionally a no-op for storage
// plugins — the host's settings.manage is sufficient (see
// mcharbor-storage-plugins-no-rbac).
func (p *SFTPPlugin) RegisterPermissions(_ plugin.AccessRegistry) error {
	return nil
}

// RegisterAdapters registers the single StorageAdapterSpec
// this plugin owns. The host dispatches destinations with
// location_type == "sftp" to the StorageAdapterClient we
// hand back here.
//
// OAuth is intentionally false — SFTP authentication is
// credential-based (USER/PASS, publickey, keyboard-interactive,
// or agent), not a delegated authorization-code flow.
func (p *SFTPPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Storage().Register(
		plugin.StorageAdapterSpec{
			LocationType: sftpLocationType,
			DisplayName:  "SFTP",
			Slug:         "storage-sftp",
			Icon:         "plugin.svg",
			Capabilities: plugin.StorageAdapterCapabilities{
				EncryptedCredentials: true,
				// SFTP variants have their own auth handshake
				// (password / publickey / keyboard-interactive /
				// agent). Not OAuth.
				OAuth:           false,
				SupportsListing: true,
				SupportsDelete:  true,
			},
			ConfigSchema: sftpConfigSchema(),
		},
		NewSFTPClient(),
	)
}

// Init runs after all registries are wired up. We stash the
// host context so Shutdown can emit an audit event.
func (p *SFTPPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.storage-sftp.init", map[string]any{
			"slug": p.manifest.Slug,
		})
	}
	return nil
}

// Shutdown runs on graceful stop or uninstall.
func (p *SFTPPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.storage-sftp.shutdown", nil)
	}
	return nil
}

// sftpConfigSchema returns the typed config fields the form
// renders. Mirrors the FIELDS list in frontend/src/ConfigForm.tsx.
//
// Conditional fields per auth_method:
//
//   - password             username + password
//   - publickey            username + private_key_path (+
//                          optional private_key_passphrase)
//   - keyboard_interactive username (+ optional password —
//                          server-driven challenges)
//   - agent                username + agent_socket
//
// Note on Type values: the SDK's ConfigFieldDecl supports
// "string" | "int" | "bool" | "secret". The plugin's own UMD
// bundle (ConfigForm.tsx) renders auth_method as a select via
// the frontend's native <select>; the host's stock form would
// render it as a text input (still valid — the values are
// constrained client-side).
func sftpConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "server",
			Type:        "string",
			Label:       "Server",
			Required:    true,
			Placeholder: "sftp.example.com or 10.0.0.5",
			Help:        "Hostname or IP of the SFTP server. The host process must have network access to it on the configured port.",
		},
		{
			Key:         "port",
			Type:        "string",
			Label:       "Port",
			Required:    false,
			Default:     strconv.Itoa(sftpDefaultPort),
			Placeholder: "22",
			Help:        "TCP port. SFTP runs over SSH; default 22. Change when the SSH daemon listens on a non-standard port.",
		},
		{
			Key:      "auth_method",
			Type:     "string",
			Label:    "Authentication Method",
			Required: true,
			Default:  sftpAuthPassword,
			Help:     "How the SSH client proves its identity to the server. password = username + password. publickey = SSH private key (with optional passphrase). keyboard_interactive = server-driven challenge (Duo / Okta / SecurID). agent = local SSH agent socket.",
		},
		{
			Key:         "username",
			Type:        "string",
			Label:       "Username",
			Required:    true,
			Placeholder: "sftpuser",
			Help:        "SSH username. Required for every auth method.",
		},
		{
			Key:         "password",
			Type:        "secret",
			Label:       "Password",
			Required:    false,
			Placeholder: "••••••••",
			Help:        "SSH password. Required for password auth. For keyboard_interactive, the same password is echoed back to password-style prompts; multi-factor prompts that need operator interaction are returned empty. Stored encrypted at rest via AES-256-GCM.",
		},
		{
			Key:         "private_key_path",
			Type:        "string",
			Label:       "Private Key Path",
			Required:    false,
			Placeholder: "/etc/sftp/id_ed25519",
			Help:        "PEM-encoded SSH private key. Required for publickey auth. The host process must be able to read it. Supports OpenSSH and RSA formats; PKCS#8 / PuTTY .ppk are not supported — convert with ssh-keygen -i.",
		},
		{
			Key:         "private_key_passphrase",
			Type:        "secret",
			Label:       "Private Key Passphrase",
			Required:    false,
			Placeholder: "••••••••",
			Help:        "Optional passphrase for the private key. Leave blank for unencrypted keys.",
		},
		{
			Key:         "agent_socket",
			Type:        "string",
			Label:       "SSH Agent Socket",
			Required:    false,
			Placeholder: "/run/ssh-agent.sock",
			Help:        "Path to the SSH agent's UNIX socket. Required for agent auth. Windows named pipes are not supported in v1 — operators on Windows can run ssh-agent inside WSL or use ssh-agent -D plus a TCP-forwarded socket.",
		},
		{
			Key:         "host_key_fingerprint",
			Type:        "string",
			Label:       "Host Key Fingerprint (SHA-256)",
			Required:    false,
			Placeholder: "SHA256:abcd…",
			Help:        "Optional. SHA-256 fingerprint of the server's host key, e.g. SHA256:abcd… When set, the client verifies the server's key against this fingerprint and rejects mismatches (TOFU protection). Get the fingerprint with: ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub -E sha256",
		},
		{
			Key:         "path",
			Type:        "string",
			Label:       "Subdirectory",
			Required:    false,
			Placeholder: "backups",
			Help:        "Optional subdirectory on the SFTP server. Leave empty to write to the user's home directory. The plugin creates intermediate directories as needed (mkdir -p semantics).",
		},
		{
			Key:         "connect_timeout",
			Type:        "string",
			Label:       "Connect Timeout",
			Required:    false,
			Default:     sftpDefaultConnectTimeout,
			Placeholder: "30s",
			Help:        "Maximum time to wait for the SSH handshake. Minimum 5s. Use a longer value for high-latency networks.",
		},
	}
}

// Plugin is the symbol the loader looks for. plugin.Open
// dlopens the .so and resolves this var; missing/wrong type
// produces a clear error at boot.
var (
	// Compile-time interface check. If SFTPPlugin drifts
	// away from plugin.Plugin (e.g. a new interface method
	// lands), the build fails here before the host even
	// tries to load the .so.
	_ plugin.Plugin = (*SFTPPlugin)(nil)

	// Symbol the host dlopens.
	Plugin plugin.Plugin = &SFTPPlugin{
		manifest: plugin.Manifest{
			Schema:  "mcharbor.plugin/v1",
			Slug:    "storage-sftp",
			Name:    "SFTP",
			Version: "2026.10.01",
			Perms:   []string{"storage.adapter.register", "audit.emit"},
		},
	}
)

// main is required for -buildmode=plugin. The loader does not
// invoke it; the side effect of binding `Plugin` is the whole
// reason this file exists.
func main() { _ = Plugin }
