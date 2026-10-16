# Snapshots

**Settings → Integrations → Snapshots** · `?tab=snapshots`

Filesystem snapshot drivers detected per environment, and the configurations that use them.

## Permissions

| Key | What it unlocks |
|---|---|
| `settings.manage` | Configure snapshot drivers; this section has no dedicated key of its own |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- Which drivers McHarbor found on the selected environment, and whether each is usable.
- Named snapshot configurations that backup plans select as their consistency mode.
- Recent snapshots taken on that environment.

## Detected drivers

Detection runs against the host, not against a list of what you installed. Each driver
shows **Available** or **Unavailable**, whether it is **No pause** or **Pauses briefly**,
and how many targets it can snapshot.

Drivers are grouped by where they came from (**Local host**, **Remote agent** or
**Plugin**), under a heading for each, so a driver probed on the host is not mistaken for
one a plugin contributed.

An empty result is explained rather than left blank: "The host offers no snapshot-capable
filesystem or tool McHarbor recognises." ZFS, Btrfs and LVM are what a Linux host typically
offers; an ext4 volume offers nothing, which is a property of the filesystem.

## Why this matters

A backup plan’s **Consistency** setting is where snapshots are used. Without a
configuration here, Snapshot mode is unavailable and the plan must choose between pausing
the container and risking a torn archive.

If a snapshot cannot be taken when a backup runs, McHarbor pauses the container instead and
records why on the run, so it degrades to correct rather than to broken.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Backups](../../../backup/screen/README.md): consistency modes
- [Snapshot driver plugins](../../../plugins/snapshots/README.md): the two appliance drivers
