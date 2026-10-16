# Box

Stores backup archives in Box. The plugin talks to `api.box.com` for metadata and `upload.box.com` for content, and uploads larger archives through a Box chunked upload session so archive size is not bounded by a single request.

- **Slug** `storage-box`, **version** 2026.09.01, **destination type** `box`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- Delegated auth uses the OAuth 2.0 refresh-token flow. Consent runs through `https://account.box.com/api/oauth2/authorize`; the refresh token the host stores is what keeps the destination working after the access token expires.
- App-only auth uses the client-credentials grant, and that is the only mode where `box_subject_type` and `box_subject_id` are read.
- Leave `folder_id` blank to upload into the account root (Box folder id `0`).

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
| [Developer](../../../development/plugins/storage/box.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
