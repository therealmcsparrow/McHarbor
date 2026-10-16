# FTP/FTPS

Stores backup archives on an FTP (RFC 959) or FTPS (RFC 4217) server. Five authentication methods are covered: anonymous, username and password, explicit FTPS (`AUTH TLS`), implicit FTPS on port 990, and client-certificate mTLS.

- **Slug** `storage-ftp`, **version** 2026.09.01, **destination type** `ftp`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- Plain FTP and explicit FTPS both connect on port 21; implicit FTPS expects 990. Setting the wrong port for the method is the most common cause of a hang rather than an error.
- `client_cert_path` and `client_key_path` are paths **inside the McHarbor container**. Mount the PEM files in and point at the mounted path, not at a path on your workstation.
- `tls_skip_verify` disables server certificate verification entirely. It exists for self-signed internal servers; on anything reachable from outside, leaving it on removes the only protection FTPS adds.
- `connect_timeout` has a 5s floor.

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
| [Developer](../../../development/plugins/storage/ftp.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
