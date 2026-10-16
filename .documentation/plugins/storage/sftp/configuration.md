# SFTP: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- An SSH account on the target with write access to the destination directory.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `server` | string | Yes | — | Hostname or IP of the SFTP server. The host process must have network access to it on the configured port. |
| `port` | string | No | — | TCP port. SFTP runs over SSH; default 22. Change when the SSH daemon listens on a non-standard port. |
| `auth_method` | string | Yes | — | How the SSH client proves its identity to the server. password = username + password. publickey = SSH private key (with optional passphrase). keyboard_interactive = server-driven challenge (Duo / Okta / SecurID). agent = local SSH agent socket. |
| `username` | string | Yes | — | SSH username. Required for every auth method. |
| `password` | secret | No | — | SSH password. Required for password auth. For keyboard_interactive, the same password is echoed back to password-style prompts; multi-factor prompts that need operator interaction are returned empty. Stored encrypted at rest via AES-256-GCM. |
| `private_key_path` | string | No | — | PEM-encoded SSH private key. Required for publickey auth. The host process must be able to read it. Supports OpenSSH and RSA formats; PKCS#8 / PuTTY .ppk are not supported; convert with ssh-keygen -i. |
| `private_key_passphrase` | secret | No | — | Optional passphrase for the private key. Leave blank for unencrypted keys. |
| `agent_socket` | string | No | — | Path to the SSH agent's UNIX socket. Required for agent auth. Windows named pipes are not supported in v1. Operators on Windows can run ssh-agent inside WSL or use ssh-agent -D plus a TCP-forwarded socket. |
| `host_key_fingerprint` | string | No | — | Optional. SHA-256 fingerprint of the server's host key, e.g. SHA256:abcd… When set, the client verifies the server's key against this fingerprint and rejects mismatches (TOFU protection). Get the fingerprint with: ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub -E sha256 |
| `path` | string | No | — | Optional subdirectory on the SFTP server. Leave empty to write to the user's home directory. The plugin creates intermediate directories as needed (mkdir -p semantics). |
| `connect_timeout` | string | No | — | Maximum time to wait for the SSH handshake. Minimum 5s. Use a longer value for high-latency networks. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Open **Settings → Integrations → Storage** and add a destination of this type.
3. Fill in the fields above, and complete OAuth consent where the plugin uses it.
4. Press **Test**, then select the destination on a backup plan.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `storage.adapter.register` | Register a backup-destination type with the host storage registry. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [SFTP](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
