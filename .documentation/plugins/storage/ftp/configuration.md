# FTP/FTPS: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- Network reachability from the McHarbor host process to the server on TCP/21 or TCP/990, plus whatever passive-mode port range the server advertises.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `server` | string | Yes | — | Hostname or IP of the FTP server. The host process must have network access to it (TCP/21 or TCP/990). |
| `port` | string | No | — | TCP port. Plain / explicit TLS use 21. Implicit FTPS uses 990. |
| `auth_method` | select | Yes | — | How the client proves its identity to the server. Anonymous uses built-in USER/PASS; client_cert uses a TLS certificate. |
| `username` | string | No | — | FTP username. Required for password / FTPS variants. For client_cert auth, optional and server-dependent. |
| `password` | secret | No | — | FTP password. Required for password / FTPS variants. Stored encrypted at rest. |
| `tls_skip_verify` | bool | No | — | Disable server-cert verification. Only enable for self-signed certs you trust. |
| `client_cert_path` | string | No | — | PEM-encoded client certificate. The host process must be able to read it. |
| `client_key_path` | string | No | — | PEM-encoded private key matching the client certificate. The host process must be able to read it. |
| `path` | string | No | — | Optional subdirectory on the FTP server. Leave empty to write to the user's home directory. |
| `connect_timeout` | string | No | — | Maximum time to wait for the TCP/TLS handshake. Minimum 5s. Use a longer value for high-latency networks. |

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

- [FTP/FTPS](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
