# Dropbox: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A Dropbox app at [dropbox.com/developers/apps](https://www.dropbox.com/developers/apps) with `files.content.write` and `files.content.read`, and a generated access token.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `access_token` | secret | Yes | — | Long-lived access token from a Dropbox app. Generate one at https://www.dropbox.com/developers/apps. Stored encrypted at rest. |
| `base_path` | string | No | — | Optional folder under the Dropbox app's root to upload into. Leave blank for the app root. |

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

- [Dropbox](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
