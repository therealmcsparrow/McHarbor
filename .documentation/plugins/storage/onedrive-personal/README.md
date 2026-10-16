# OneDrive (Personal)

Stores backup archives in a consumer OneDrive account. It talks to Microsoft Graph `/me/drive` and takes tokens from the `consumers` endpoint, so it works with a personal Microsoft account rather than a tenant.

- **Slug** `storage-onedrive-personal`, **version** 2026.09.01, **destination type** `onedrive_personal`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- Consent runs at `login.microsoftonline.com/consumers/oauth2/v2.0/authorize`.
- `client_secret` is only needed for a confidential (Web) app registration. Public-client registrations leave it blank.
- Large uploads use Graph's chunked upload session, so archive size is not bounded by a single request.

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
| [Developer](../../../development/plugins/storage/onedrive-personal.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
