# OneDrive for Business: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A Microsoft Entra app registration with the **Files.ReadWrite** delegated permission and a **Web** redirect URI pointing at this McHarbor host.
- A client secret from **Certificates & secrets**.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `tenant_id` | string | Yes | — | Microsoft Entra tenant ID (GUID). From Entra admin center → Overview. The OAuth consent flow redirects to login.microsoftonline.com/{tenant_id}/… |
| `client_id` | string | Yes | — | Application (client) ID of the Entra app registration with Files.ReadWrite delegated permission and a Web redirect URI pointing at this McHarbor host. |
| `client_secret` | secret | Yes | — | Client secret value from Entra admin center → App registrations → Certificates & secrets. Stored encrypted at rest. |
| `drive_id` | string | No | — | Optional. Fully-qualified Graph drive identifier. Leave blank to use the service account's drive (resolved via UPN or /me/drive). |
| `base_path` | string | No | — | Optional. Graph path inside the drive, e.g. /Backups/2026. Leave blank to use the drive root. No default: operators either leave blank or enter an explicit path. |
| `upn` | string | No | — | Optional. UPN of the service account, e.g. backups@contoso.onmicrosoft.com. When drive_id is also blank, uploads target /users/{upn}/drive; if both upn and drive_id are blank, /me/drive (the refresh token's user) is used as a fallback. |

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

- [OneDrive for Business](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
