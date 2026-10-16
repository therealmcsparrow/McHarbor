# Synology DSM

Takes filesystem snapshots on a Synology NAS through the DSM 7.x REST API, covering shared-folder snapshots and Hyper Backup vaults, without pausing containers.

- **Slug** `snapshot-synology`, **version** 2026.09.01, **driver type** `synology`
- **Host range** `>=2026.09.01`
- **Where it appears** the consistency mode on an environment, and on a backup plan
- **Gated by** `settings.manage` on the environment configuration that selects a snapshot repository

## How it works

- `dsm_url` is the DSM web UI origin, typically on port 5001 for HTTPS.
- `ignore_certificate_errors` is usually required: Synology ships a self-signed certificate by default.
- Snapshot support depends on the volume filesystem: Btrfs volumes support shared-folder snapshots, ext4 volumes do not.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `SupportsMount` | No | A snapshot can be mounted and read directly. |
| `SupportsRemoteTarget` | Yes | Snapshots are taken on a remote appliance rather than on the Docker host. |
| `RequiresAgent` | No | The driver needs the McHarbor agent on the host to do its work. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/snapshots/synology.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Snapshot drivers](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
