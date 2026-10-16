# OneDrive for Business

Stores backup archives in OneDrive for Business through the tenant-scoped Microsoft Graph API (`/users/{upn}/drive`). Large archives go through Graph's chunked upload session. SharePoint document libraries are a separate plugin; install [SharePoint Online](../sharepoint-online/README.md) for those.

- **Slug** `storage-onedrive-business`, **version** 2026.09.01, **destination type** `onedrive_business`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- Consent and token refresh run against `login.microsoftonline.com/{tenant_id}`; all file operations go to `graph.microsoft.com`.
- Drive resolution order is `drive_id` first, then `/users/{upn}/drive`, then `/me/drive`. Filling in neither is valid and resolves to the consenting account's drive.
- `base_path` has no default. Blank means the drive root.

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
| [Developer](../../../development/plugins/storage/onedrive-business.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
