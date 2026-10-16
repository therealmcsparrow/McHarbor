# Nextcloud

Stores backup archives on a Nextcloud instance over WebDAV (RFC 4918). Two authentication modes are supported: an OAuth 2.0 client created in **Settings → Administration → Security**, or an app password generated under **Personal → Security**. Both end up on the same WebDAV path.

- **Slug** `storage-nextcloud`, **version** 2026.09.01, **destination type** `nextcloud`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- The WebDAV endpoint defaults to `{host}/remote.php/dav/files/{username}/`. `apiPath` overrides it, which is what you need behind a reverse proxy that adds a path prefix.
- Both auth modes send HTTP Basic credentials. For the app-password mode, put the app password in `clientSecret` and leave `clientId` blank.
- Uploads use `Content-Length` plus `Content-Range` PUT, which Nextcloud's WebDAV server handles server-side.
- Do not set `host` with a trailing slash; the plugin appends paths to it verbatim.

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
| [Developer](../../../development/plugins/storage/nextcloud.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
