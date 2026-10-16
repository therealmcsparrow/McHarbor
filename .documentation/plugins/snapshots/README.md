# Snapshot Drivers

A snapshot-driver plugin lets McHarbor take a filesystem-level snapshot before it reads a
container's data, so a backup is consistent without stopping or pausing the container. The
host decides when a snapshot is needed; the plugin probes capability, creates the
snapshot, and removes it afterwards.

| Plugin | Driver type | Target | What it snapshots |
|---|---|---|---|
| [Synology DSM](./synology/README.md) | `synology` | Remote appliance | Synology DSM snapshot repository via the DSM 7.x REST API. |
| [TrueNAS](./truenas/README.md) | `truenas` | Remote appliance | TrueNAS SCALE / CORE snapshot repository via the middleware API. |

Each plugin has a folder of its own: a README, the configuration reference, a
troubleshooting table, and a developer page covering the adapter it registers and how to
build, sign and fork it.

## What they have in common

- They register through `AdapterRegistry.Snapshot()` and are selected as the consistency
  mode on an environment or a backup plan.
- Both are **remote-target** drivers: the snapshot is taken on the appliance that owns the
  storage, over its API, and neither needs the McHarbor agent.
- Neither supports mounting a snapshot. It exists to hold a consistent point in time
  while the archive is read, not to be browsed.
- Credentials are encrypted at rest, and both appliances commonly ship self-signed
  certificates, so both expose a verification switch. Turning it off is a decision about
  the network the call crosses.

## Local drivers

ZFS, Btrfs, LVM and VSS snapshots on a Docker host itself are handled by the host's own
built-in drivers, not by a plugin. These two plugins are for the case where the data lives
on a NAS.

## Related

- [Backup](../../backup/README.md): where Snapshot is chosen as the consistency mode
- [Settings → Integrations → Snapshots](../../settings/integrations/snapshots/README.md): where a driver is configured per environment
- [Plugin catalog](../catalog.md): every shipped plugin in one table
- [Plugins](../README.md): the section index
