# OpenCloud: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- An OpenCloud account with write access, and either an OAuth 2.0 client or an app password.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `host` | string | Yes | — | Base URL of the OpenCloud instance, e.g. https://cloud.example.com. No trailing slash. The plugin derives the WebDAV endpoint at call time. |
| `username` | string | Yes | — | OpenCloud login (username or email). |
| `clientId` | string | No | — | OAuth 2.0 client_id from OpenCloud admin → Settings → Security → OAuth 2.0 clients. Leave blank if you use an app password instead. |
| `clientSecret` | secret | Yes | — | Either the OAuth 2.0 client_secret, or an app password from OpenCloud Personal settings → Security → App passwords. Both work over HTTP Basic auth. |
| `basePath` | string | No | — | Optional subfolder under the user's OpenCloud Files root. |
| `apiPath` | string | No | — | Override the WebDAV path. Defaults to /remote.php/dav/files/{username}/. |

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

- [OpenCloud](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
