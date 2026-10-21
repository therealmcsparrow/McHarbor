# Samba: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A share reachable on TCP/445 (append `:port` to **Server** for a non-standard port) and an account with write access.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `server` | string | Yes | — | SMB server hostname or IP. Default port 445; append :PORT to override. |
| `share` | string | Yes | — | SMB share name (case-sensitive on most servers). |
| `path` | string | No | — | Optional subdirectory inside the share. Backslashes are server-native; the client preserves them as-is. |
| `auth_method` | string | Yes | — | One of: \ |
| `username` | string | No | — | Used when auth_method is basic or domain. |
| `password` | secret | No | — | Used when auth_method is basic or domain. Stored encrypted at rest. |
| `domain` | string | No | — | Required when auth_method is domain. Optional for basic (workgroup auth). |
| `guest_user` | string | No | — | Username sent on the wire when auth_method=guest. Defaults to |
| `guest_password` | secret | No | — | Password sent on the wire when auth_method=guest. Most guest accounts have an empty password; stored encrypted at rest just in case. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Destination rules

The plugin checks the address it resolved, right before it connects, and refuses an address that is never a real server: `0.0.0.0/8`, link-local (`169.254.0.0/16`, `fe80::/10`), the cloud metadata endpoints (`169.254.169.254`, `100.100.100.200`, `fd00:ec2::/32`), multicast, broadcast and `::`. IPv6 forms that carry one of these (NAT64, 6to4, Teredo, ISATAP, IPv4-compatible) are refused as well. A name that resolves to a refused address is refused like the address itself. Loopback, private ranges (RFC 1918) and ULA stay allowed, so a server on your LAN or on the McHarbor host works. A Unix socket is never a valid destination. A refusal reads "storage-samba: the SMB server address is not an allowed destination".

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

- [Samba](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
