# TrueNAS

Takes ZFS snapshots on TrueNAS SCALE or CORE through the middleware API, so a container's data can be captured consistently without `docker pause`.

- **Slug** `snapshot-truenas`, **version** 2026.09.01, **driver type** `truenas`
- **Host range** `>=2026.09.01`
- **Where it appears** the consistency mode on an environment, and on a backup plan
- **Gated by** `settings.manage` on the environment configuration that selects a snapshot repository

## How it works

- Snapshots are taken recursively under `dataset_path`, so child datasets are included.
- The default API URL is `https://nas.local/api/v2.0`. Give the middleware API root, not the UI root.
- `verify_tls` off is equivalent to `--insecure` and is common on default TrueNAS installs, which ship a self-signed certificate. Install a trusted certificate if this crosses an untrusted network.
- This is the snapshot driver. For writing backup **archives** to TrueNAS over WebDAV, use the [TrueNAS storage destination](../../storage/truenas/README.md) instead; they are separate plugins.

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
| [Developer](../../../development/plugins/snapshots/truenas.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Snapshot drivers](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
