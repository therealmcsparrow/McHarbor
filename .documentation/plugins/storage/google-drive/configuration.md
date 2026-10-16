# Google Drive: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A Google Cloud project with the **Drive API** enabled.
- An OAuth client of type **Web application** whose redirect URI points at this McHarbor host.
- The `https://www.googleapis.com/auth/drive.file` scope. It only grants access to files this app created, which is what you want for a backup destination.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `client_id` | string | Yes | — | Application (client) ID from the Google Cloud console → APIs & Services → Credentials. The OAuth client must be of type \ |
| `client_secret` | secret | Yes | — | Client secret from the same OAuth client. Stored encrypted at rest by the host's AES-256-GCM service. |
| `folder_id` | string | No | — | Optional. ID of the Drive folder where backups should land. Leave blank to use the Drive root. From drive.google.com → open the folder → copy the ID from the URL after /folders/. |

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

- [Google Drive](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
