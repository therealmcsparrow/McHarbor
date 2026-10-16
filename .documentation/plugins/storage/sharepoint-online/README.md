# SharePoint Online

Stores backup archives in a SharePoint Online document library through the tenant-scoped Microsoft Graph API (`/sites/{site_id}/drive`). OneDrive for Business drives are a separate plugin; install [OneDrive for Business](../onedrive-business/README.md) for those.

- **Slug** `storage-sharepoint-online`, **version** 2026.09.01, **destination type** `sharepoint`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- Consent and refresh run against `login.microsoftonline.com/{tenant_id}`; file operations go to `graph.microsoft.com`.
- Give either `drive_id` or `site_id`. With only `site_id`, the site's default document library is used.
- Find a site ID with `GET /sites/{hostname}:/sites/{path}` in Graph Explorer.

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
| [Developer](../../../development/plugins/storage/sharepoint-online.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
