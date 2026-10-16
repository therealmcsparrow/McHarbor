# Nextcloud: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A Nextcloud account with write access, and either an OAuth 2.0 client or an app password.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `host` | string | Yes | — | Base URL of the Nextcloud instance, e.g. https://cloud.example.com. No trailing slash. The plugin derives the WebDAV endpoint ({base}/remote.php/dav/files/{username}/) at call time. |
| `username` | string | Yes | — | Nextcloud login (username or email). Used to scope the WebDAV path under /remote.php/dav/files/{username}/ and identifies the OAuth user during refresh. |
| `clientId` | string | No | — | OAuth 2.0 client_id from the Nextcloud admin → Settings → Security → OAuth 2.0 clients page. Leave blank if you use an app password instead (see client_secret below). |
| `clientSecret` | secret | Yes | — | Two valid options: (1) the OAuth 2.0 client_secret from the Nextcloud OAuth client config, or (2) an app password generated in Nextcloud Personal settings → Security → App passwords. The plugin uses HTTP Basic auth on every WebDAV request, so an app password works directly without OAuth setup. Stored encrypted at rest. |
| `basePath` | string | No | — | Optional subfolder under the user's Nextcloud Files root, e.g. Backups/2026. Leave blank for the user root. Forward slashes are treated as folder separators. |
| `apiPath` | string | No | — | Override the WebDAV path. Defaults to /remote.php/dav/files/{username}/. Set this only if your Nextcloud sits behind a path prefix or a reverse proxy that rewrites the WebDAV path. |

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

- [Nextcloud](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
