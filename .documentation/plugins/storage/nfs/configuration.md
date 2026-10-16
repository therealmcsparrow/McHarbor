# NFS: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- An export reachable from the host, and an export rule that permits the McHarbor host's address.
- For Kerberos flavours, a keytab containing the `nfs/<host>` principal, readable inside the container.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `server` | string | Yes | — | Hostname or IP of the NFS server (e.g. 10.0.0.5 or nfs.example.com). |
| `export` | string | Yes | — | Server-side export path exported to this host. |
| `path` | string | No | — | Optional subdirectory inside the mount. Empty = the export root. |
| `sec` | select | Yes | — | AUTH_SYS trusts the client UID/GID. krb5/krb5i/krb5p use Kerberos 5 (RPCSEC_GSS) with increasing integrity/encryption. |
| `version` | select | No | — | NFS protocol version. Auto lets the kernel negotiate (usually 4.2 on modern kernels). |
| `principal` | string | No | — | Required for krb5/krb5i/krb5p. Format: nfs/<host>@REALM. |
| `realm` | string | No | — | Required for krb5/krb5i/krb5p. Kerberos realm (uppercase). |
| `keytab_path` | string | No | — | Required for krb5/krb5i/krb5p. Path to the keytab file containing the nfs/<host> principal. The host process must be able to read it. |
| `mount_base` | string | No | — | Where the plugin creates per-destination mount points. Default: /var/lib/mcharbor/mounts/nfs. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Open **Settings → Integrations → Storage** and add a destination of this type.
3. Fill in the fields above, and complete OAuth consent where the plugin uses it.
4. Press **Test**, then select the destination on a backup plan.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `storage.adapter.register` | Register a backup-destination type with the host storage registry. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [NFS](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
