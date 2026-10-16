# Synology DSM: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A DSM account allowed to manage snapshots (an account-scoped app password is the tighter option), and the shared folder to snapshot.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `dsm_url` | string | Yes | — | Base URL of the Synology DSM web UI (e.g. https://nas.local:5001). |
| `username` | string | Yes | — | DSM user account with permission to manage snapshots. Use an account-scoped app password for tighter security. |
| `password` | secret | Yes | — | DSM password or app-specific password. Stored encrypted at rest by the host's encryption service. |
| `share_name` | string | Yes | — | Shared folder name to snapshot (e.g. /volume1/backups). |
| `ignore_certificate_errors` | boolean | No | — | When true, accept self-signed certificates on the DSM endpoint. Required for default Synology certificates. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved: a response names the stored ones in `secretsSet`
instead. Editing an existing entry shows the field as "Stored - leave empty to keep", and
leaving it empty keeps the stored secret.

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Configure the repository with the appliance URL and credentials.
3. Select it as the consistency mode on the environment or the backup plan.
4. Run a backup and confirm the snapshot is created and cleaned up afterwards.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `snapshot.adapter.register` | Register a snapshot driver with the host snapshot registry. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [Synology DSM](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
