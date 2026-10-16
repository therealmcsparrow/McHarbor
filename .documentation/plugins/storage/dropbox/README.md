# Dropbox

Stores backup archives in Dropbox through API v2. Authentication is a long-lived access token generated on a Dropbox app, so there is no consent round-trip to complete in the browser.

- **Slug** `storage-dropbox`, **version** 2026.09.01, **destination type** `dropbox`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- Metadata calls go to `api.dropboxapi.com`, content to `content.dropboxapi.com`.
- A scoped app sees only its own app folder; `base_path` is then relative to that folder rather than to the account root.
- Dropbox access tokens generated in the app console are long-lived but revocable. Revoking one silently breaks every destination using it, and the failure only shows up on the next backup or on **Test**.

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
| [Developer](../../../development/plugins/storage/dropbox.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
