# TrueNAS: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A configured WebDAV share on the TrueNAS system and a local user with read and write access to it.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `host` | string | Yes | — | Base URL of the TrueNAS server, e.g. https://nas.local. No trailing slash. |
| `username` | string | Yes | — | Local TrueNAS user. The user must have read/write access to the configured WebDAV share. |
| `password` | secret | Yes | — | Password for the local TrueNAS user. Stored encrypted at rest by the host's AES-256-GCM service. |
| `basePath` | string | No | — | Optional subfolder under the share's WebDAV root. Leave blank for the share root. |
| `apiPath` | string | No | — | Override the WebDAV path. Defaults to /webdav. Operators with multiple WebDAV shares can set this to /webdav/{share-name}. |

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

- [TrueNAS](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
