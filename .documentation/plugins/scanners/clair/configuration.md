# Clair: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A reachable Clair v4 server, and a bearer token if it has authentication enabled.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `server_url` | string | Yes | — | Base URL of the Clair v4 server. The indexer and matcher paths are appended automatically. |
| `auth_token` | secret | No | — | Sent as an Authorization: Bearer header. Leave empty for a Clair server with authentication disabled. Stored encrypted; leave blank when editing to keep the stored value. |
| `timeout_seconds` | int | No | — | Per-request timeout for the index and report calls. Indexing a large image can take a while on a cold Clair; the host's own scan timeout still bounds the whole scan. |
| `insecure_skip_verify` | bool | No | — | Accept any certificate from the Clair server. Only for a self-signed certificate on a trusted network. It disables the check that the server is who it claims to be. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Open **Security → Scanners** and configure it.
3. Press **Test**, then scan an image from the Images page.
4. Findings appear on the image, and on every container running it.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `scanner.adapter.register` | Register an image scanner with the host scanner registry. |
| `encryption.handle.read` | Read the host encryption handle so it can decrypt the credentials it stored. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [Clair](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
