# Backup

Encrypted container backups, end to end: what is backed up, how consistent the archive is,
where it goes, how long it is kept, and how the data comes back.

Backup is spread across four places in the product (the Backups screen, a container's own
backup settings, two settings sections, and the plugins that talk to a remote system), so
this section is the index that ties them together.

## The chain

| Step | Where it is decided | What matters |
|---|---|---|
| **What** | A plan's *What to back up* group | Config, the container filesystem, named volumes, bind mounts, and a tail of the container log |
| **How consistent** | The plan's consistency mode | **Pause**, **Snapshot** or **Live**: the choice that decides whether the archive is usable |
| **When** | The plan's schedule | A cron expression read in the configured time zone and rendered back in words |
| **Where** | The plan's destinations | **Local storage** and any configured destination, more than one at a time |
| **Encrypted with** | The backup encryption key | Held as a Docker secret, not by McHarbor |
| **Kept for** | Retention, by count and by days | Prunes locally *and* at the destination; both values at zero keeps every successful run |
| **Back again** | Restore, in four selectable layers | Config, filesystem, mount, bind. An archive under an old key asks for that key |

## Consistency is the setting that matters most

| Mode | What it does | Cost |
|---|---|---|
| **Pause** | Stops the container while the archive is written, then starts it again | Correct everywhere, at the cost of downtime |
| **Snapshot** | Freezes the filesystem and reads the frozen copy, so the container keeps serving traffic | Needs a snapshot configuration on that environment |
| **Live** | Reads the running container's files as they are | Fastest, and the only mode that can produce a torn archive |

If a snapshot cannot be taken when the backup actually runs, **the container is paused
instead** and the reason is recorded on the run, so a failed snapshot degrades to a correct
backup rather than to a broken one.

Live is the right choice for a container whose data is not a database. It is the wrong
choice for one that is.

## Permissions

| Key | What it unlocks |
|---|---|
| `backups.view` | See runs and plans |
| `backups.manage` | Create plans, run backups, cancel, retry an upload |
| `backups.download` | Download an archive |
| `backups.delete` | Delete runs and plans |
| `restore.view` | Browse inside an archive |
| `restore.run` | Restore into a container |
| `backup_log.view` | Read the backup log |
| `settings.view` or `storage_locations.view` | See storage destinations |
| `settings.manage` or `storage_locations.manage` | Add, change or test a destination |
| `storage_locations.delete` | Remove a destination |

`backups.download` is separate from `backups.view` on purpose: an archive is the
container's data, off the host, in one file.

## In this section

| Page | What it covers |
|---|---|
| [The Backups screen](./screen/README.md) | `/backups`: what it shows, and the four sections it is built from |
| [Actions](./screen/actions.md) | Run, cancel, restore, restore from file, download, delete, and what each one really does |
| [Fields](./screen/fields.md) | Every column, filter and plan field, including retention and the consistency selector |
| [Troubleshooting](./screen/troubleshooting.md) | Symptom, cause and fix, including a Live-mode restore that comes back corrupt |

## Where archives go

The host owns the destination rows, the encryption, the scheduling and the retention
sweep. A plugin supplies only the code that talks to the remote system: test, upload,
download, delete, list, and where the protocol needs it, an OAuth consent round-trip.

- [Storage destination plugins](../plugins/storage/README.md): all 14, with their fields,
  their capabilities and their developer notes
- [Settings → Integrations → Storage](../settings/integrations/storage/README.md): where a
  destination is created, tested and given capacity reporting, and where the backup
  encryption key is generated, installed and rotated

**Local storage** is always available and is the default. A remote destination needs one of
the storage plugins installed.

## What makes Snapshot mode available

- [Snapshot driver plugins](../plugins/snapshots/README.md): the two appliance drivers,
  Synology DSM and TrueNAS, for data that lives on a NAS
- [Settings → Integrations → Snapshots](../settings/integrations/snapshots/README.md):
  snapshot configurations per environment

ZFS, Btrfs, LVM and VSS snapshots on a Docker host itself are handled by the host's own
built-in drivers, not by a plugin.

## Driving backup from elsewhere

| Surface | What it gives you |
|---|---|
| [Containers](../screens/containers/README.md) | Per-container backup settings, schedules and runs, without leaving the container |
| [Workflow action nodes](../workflows/nodes/actions/README.md) | `container-backup-run`, `container-backup-plan-run`, `container-backup-manage`, `container-backup-download`, `container-backup-restore` and `stack-backup` |
| [Backups API](../api/endpoints/backups.md) | 44 endpoints: plans, runs, the restore browser, the log feed and the destinations |
| [Settings → Monitoring → Data retention](../settings/monitoring/data-retention/README.md) | How long run history is kept, separately from the archives themselves |

## Related

- [Volumes](../screens/volumes/README.md): what actually protects the data in a volume
- [Settings → Automation](../settings/automation/README.md): where a backup plan's schedule sits among the other schedules
- [Plugins](../plugins/README.md): the runtime that loads the destination and snapshot adapters
