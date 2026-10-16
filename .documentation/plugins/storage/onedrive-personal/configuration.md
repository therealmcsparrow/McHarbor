# OneDrive (Personal): Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A Microsoft account. Registering your own Entra app is optional. Leave `client_id` blank to use the app registration embedded in the plugin.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `drive_id` | string | No | — | Optional. Leave blank to use the default drive returned by Microsoft for the signed-in account. |
| `base_path` | string | No | — | Optional. Folder inside the drive, e.g. /Backups/2026. Leave blank to use the drive root. |
| `client_id` | string | No | — | Optional. Leave blank to use the plugin's embedded McSparrow app registration. Fill in your own client ID if you registered a custom Azure AD app for OneDrive Personal. |
| `client_secret` | secret | No | — | Optional. Only needed for confidential (Web) app registrations. Leave blank for public-client (Mobile and desktop applications) registrations. |

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

- [OneDrive (Personal)](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
