# TrueNAS: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- An API key from **Credentials → Local Users → <user> → API Keys**, and the dataset the container's volumes live on.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `api_url` | string | Yes | — | Base URL of the TrueNAS web UI middleware API (default https://nas.local/api/v2.0). |
| `api_key` | secret | Yes | — | API key from Credentials → Local Users → <user> → API Keys. Stored encrypted at rest by the host's encryption service. |
| `dataset_path` | string | Yes | — | Path to the ZFS dataset to snapshot (e.g. tank/backups). Snapshots are taken recursively under this dataset. |
| `verify_tls` | boolean | No | — | Disable to accept self-signed certificates (common on default TrueNAS installs). Off is equivalent to --insecure. |

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

- [TrueNAS](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
