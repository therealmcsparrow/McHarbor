# TrueNAS

Stores backup archives on a TrueNAS WebDAV share. Authentication is a local TrueNAS user over HTTP Basic; this is the file-transfer destination, not the ZFS snapshot driver; that one is [TrueNAS snapshots](../../snapshots/truenas/README.md).

- **Slug** `storage-truenas`, **version** 2026.09.01, **destination type** `truenas`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- The WebDAV path defaults to `/webdav`. With more than one WebDAV share, set `apiPath` to `/webdav/{share-name}`.
- Uploads use `Content-Range` PUT.
- Give `host` as a bare origin with no trailing slash.

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
| [Developer](../../../development/plugins/storage/truenas.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
