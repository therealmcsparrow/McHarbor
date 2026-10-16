# NFS

Stores backup archives on an NFS export (RFC 7530). The plugin mounts the export with the kernel NFS client, which means it needs the mount capability inside the McHarbor container. It is not a userspace client.

- **Slug** `storage-nfs`, **version** 2026.09.01, **destination type** `nfs`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- `sec=sys` trusts the client UID and GID. The container's UID has to match what the export grants, or writes land as `nobody` and fail.
- `krb5`, `krb5i` and `krb5p` add authentication, integrity and privacy respectively, and all three need `principal`, `realm` and `keytab_path` filled in.
- Mount points are created under `mount_base`, default `/var/lib/mcharbor/mounts/nfs`, one per destination.
- Leave **NFS Version** on auto unless the server is pinned to v3; modern kernels negotiate 4.2.

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
| [Developer](../../../development/plugins/storage/nfs.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
