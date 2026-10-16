# Samba

Stores backup archives on an SMB/CIFS share, over `go-smb2` rather than a kernel mount. Anonymous, basic user-and-password and NTLMv2 domain authentication are all supported, and nothing has to be mounted inside the container.

- **Slug** `storage-samba`, **version** 2026.09.01, **destination type** `samba`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- Share names are case-sensitive on most servers.
- `auth_method` picks which credential fields are read: `guest` uses `guest_user`/`guest_password`, `basic` and `domain` use `username`/`password`, and `domain` additionally requires `domain`.
- Backslashes in **Path** are passed through as the server expects them.

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
| [Developer](../../../development/plugins/storage/samba.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
