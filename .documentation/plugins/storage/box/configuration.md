# Box: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A Box application in the [Box developer console](https://app.box.com/developers/console) with either **Server Authentication (Client Credentials Grant)** or a **User Authentication** app that has a redirect URI pointing at this McHarbor host.
- For app-only auth, the enterprise or user ID the app acts as.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `client_id` | string | Yes | — | Box app's client_id from the developer console. The app needs Server Authentication OR a redirect-URI registered for OAuth. |
| `client_secret` | secret | Yes | — | Box app's client_secret. Stored encrypted at rest. |
| `folder_id` | string | No | — | Optional Box folder ID. Leave blank to upload to the user's root folder (id=0). |
| `box_subject_type` | string | No | `enterprise` | Only used for app-only (client_credentials) auth. Either \ |
| `box_subject_id` | string | No | — | Only used for app-only (client_credentials) auth. Box enterprise ID (when box_subject_type=enterprise) or user ID (when box_subject_type=user). Leave blank if the operator grants consent interactively instead. |

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

- [Box](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
