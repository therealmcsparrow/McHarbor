# Storage Destinations

A storage-destination plugin gives McHarbor somewhere to put backup archives. The host
owns the destination rows, the encryption, the scheduling and the retention sweep; the
plugin supplies only the code that talks to the remote system: test, upload, download,
delete, list, and where the protocol needs it, an OAuth consent round-trip.

| Plugin | Destination type | What it stores to |
|---|---|---|
| [Box](./box/README.md) | `box` | Backup destination backed by Box (box.com). |
| [Dropbox](./dropbox/README.md) | `dropbox` | Backup destination backed by Dropbox (Dropbox API v2). |
| [FTP/FTPS](./ftp/README.md) | `ftp` | Backup destination backed by an FTP (RFC 959) or FTPS (RFC 4217) server. |
| [Google Drive](./google-drive/README.md) | `google_drive` | Google Drive backup destination. |
| [Nextcloud](./nextcloud/README.md) | `nextcloud` | Back up containers to a Nextcloud instance via WebDAV (RFC 4918). |
| [NFS](./nfs/README.md) | `nfs` | Backup destination backed by an NFS (RFC 7530) export. |
| [OneDrive (Personal)](./onedrive-personal/README.md) | `onedrive_personal` | OneDrive Personal (consumer endpoint) backup destination. |
| [OneDrive for Business](./onedrive-business/README.md) | `onedrive_business` | OneDrive for Business backup destination. |
| [OpenCloud](./opencloud/README.md) | `opencloud` | Back up containers to an OpenCloud instance via WebDAV (RFC 4918). |
| [ownCloud](./owncloud/README.md) | `owncloud` | Back up containers to an ownCloud (Classic) instance via WebDAV (RFC 4918). |
| [Samba](./samba/README.md) | `samba` | Backup destination backed by an SMB/CIFS share (samba.org). |
| [SFTP](./sftp/README.md) | `sftp` | SFTP (SSH File Transfer Protocol) backup destination. |
| [SharePoint Online](./sharepoint-online/README.md) | `sharepoint` | SharePoint document library backup destination. |
| [TrueNAS](./truenas/README.md) | `truenas` | Back up containers to a TrueNAS instance via WebDAV (RFC 4918). |

Each plugin has a folder of its own: a README, the configuration reference, a
troubleshooting table, and a developer page covering the adapter it registers and how to
build, sign and fork it.

## What they have in common

- They register through `AdapterRegistry.Storage()` and appear in the **location type**
  dropdown under **Settings → Integrations → Storage**.
- Credentials are encrypted at rest with the host's AES-256-GCM service. The plugin
  receives them decrypted at call time and never sees the key.
- **Test** is a write, change and delete round trip against the real destination. It is
  deliberately destructive of its own test object, so a green result also proves
  retention will be able to delete.
- Every one of them advertises `SupportsListing` and `SupportsDelete`, which is what
  lets selective restore browse a destination and retention prune it.
- All of them except FTP report capacity, so the destination card under **Settings →
  Integrations → Storage** shows used of total space. FTP declines explicitly: RFC 959
  defines no command that asks a server how much room is left.
- **This family ships no RBAC declarations**: no `adds_permissions`, no
  `consumes_permissions`. That is deliberate: the host already gates the screen where
  destinations are created, and the backup permissions gate the jobs that write to them.

## Choosing between the WebDAV ones

Nextcloud, OpenCloud, ownCloud and TrueNAS all speak WebDAV, and three of them use the
same `/remote.php/dav/files/{user}/` path. They are separate plugins because their auth
stories differ: app passwords and OAuth clients are created in different places, and
TrueNAS uses a plain local user on a share path. Pick the one named after your server;
`apiPath` is the escape hatch when the path is not where it should be.

## Related

- [Backup](../../backup/README.md): what writes to these destinations, and what reads back from them
- [Settings → Integrations → Storage](../../settings/integrations/storage/README.md): where a destination is created and tested
- [Plugin catalog](../catalog.md): every shipped plugin in one table
- [Plugins](../README.md): the section index
