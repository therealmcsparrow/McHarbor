# Google Drive

Stores backup archives in Google Drive using Drive API v3. Uploads go through a resumable session, so archive size is not bounded by a single request, and listing and deletion use the same API surface, so selective restore and retention both work against this destination.

- **Slug** `storage-google-drive`, **version** 2026.09.01, **destination type** `google_drive`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- Consent runs at `accounts.google.com/o/oauth2/v2/auth`; tokens are refreshed against `oauth2.googleapis.com/token`.
- Metadata calls go to `www.googleapis.com`, uploads to `upload.googleapis.com`.
- With the `drive.file` scope, the plugin cannot see files it did not create. That includes a `folder_id` for a folder created by hand in the Drive UI, unless that folder is shared with the consenting account in a way that grants write access.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | Credentials are handed to the plugin decrypted and stored re-encrypted by the host. |
| `SupportsListing` | Yes | The host can list what is already at the destination, which is what selective restore browses. |
| `SupportsDelete` | Yes | Retention can delete old archives at the destination instead of only locally. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/storage/google-drive.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
