# OpenCloud

Stores backup archives on an OpenCloud instance over WebDAV. OpenCloud presents the Nextcloud-compatible WebDAV surface, so this plugin uses the same `/remote.php/dav/files/{user}/` paths and the same Basic-auth flow.

- **Slug** `storage-opencloud`, **version** 2026.09.01, **destination type** `opencloud`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- Put an app password in `clientSecret` and leave `clientId` blank to skip OAuth entirely.
- `apiPath` overrides the derived WebDAV prefix; set it when OpenCloud sits behind a path-rewriting proxy.
- Uploads use `Content-Range` PUT, handled server-side.

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
| [Developer](../../../development/plugins/storage/opencloud.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
