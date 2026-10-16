# System container backups

A snapshot is a rollback point on the same storage pool the container lives on. A backup leaves
the host. When a pool fails it takes every snapshot on it with it, and the backups are what is
left, which is why this exists and why the protection report now names, per container, whether
it has one.

Backups appear on the **Backups** tab of a container's detail page, and every run also shows on
the host's own **Backups** page beside the Docker and virtual machine ones.

- **Permission keys** `ct.backup` and `ct.restore`
- **Destinations** the storage locations under **Settings → Storage**, the same ones Docker
  container backups and reports write to
- **Agent floor** `2026.09.01` build `202609270250` or newer

## What an artifact contains

Two objects per run, at every destination the schedule names.

| Object | What it is |
|---|---|
| `<run>-rootfs-<kind>.tar.gz` | A gzipped tar of the container's root filesystem, taken with `--numeric-owner --xattrs --acls --selinux --sparse` |
| `<run>-manifest-<kind>.json` | The container's definition: name, host, distribution, init system, privileged and nesting flags, pool backend and rootfs path, limits, interfaces and mounts, and (`idMap`) the uid/gid range the rootfs was archived in. Manifests written before addon-lxc `2026.09.12` have no `idMap` and are still read |

They land under `<environment>/ct/<container>/` at the destination, named so an operator
browsing it can tell what an object is without McHarbor in front of them.

**The extended attributes matter more than they look.** File capabilities live in xattrs, so a
tar that drops them produces a restored container that boots and then fails at `ping`, at
`dumpcap`, and at anything else that relies on one, in ways nobody traces back to the backup.
This is also why the host needs **GNU tar**: `bsdtar` and `busybox tar` accept `--create` and
understand neither `--listed-incremental` nor `--xattrs`, so a host without GNU tar is refused
up front rather than producing a lesser archive.

A pool-native send stream (`zfs send`, `btrfs send`) is faster and pins the restore to the
same pool mechanism it came from. The common case is restoring onto a different host whose pool
is not the same backend, so the tar is the first and only mode here.

## Full and incremental

Each container has one **chain**. A full copy starts it; each incremental after that carries
only what changed, computed against a GNU tar listing file the agent keeps on the host under
`<compute runtime root>/.backup-listings/`.

That listing is the exact analogue of a virtual machine chain's dirty bitmap, and it is lost the
same ways: a host rebuilt, a pool moved, a rootfs reverted or restored. When the agent cannot
find it, the run is taken as a **full copy** and says so: the chain restarts, the row records
`full`, and the reason appears behind the warning triangle in the Kind column. McHarbor never
records an incremental that was not taken, because a restore would then apply a delta on top of
a base that does not contain what it assumes.

**Take a full copy every N runs** is a floor rather than a tuning knob. A chain that never
restarts is one lost listing away from a very long restore.

### Why a dir pool's live copy is never an incremental

GNU tar's listing matches each directory to its last run **by inode number**. The rootfs itself
and every btrfs, ZFS or LVM snapshot of it share one numbering (a btrfs subvolume snapshot and a
ZFS snapshot keep their files' inode numbers, and an LVM snapshot is a block copy of the same
filesystem), so a listing taken from any of them anchors an incremental read from any other, and
the archive is taken with `--no-check-device` so the snapshot's different device number does not
matter.

A dir pool's live copy is a fresh `cp -a` every run. It gets new inode numbers, and a number the
previous copy freed comes back on a different directory. tar then records that directory as
**renamed** from wherever the number used to be, and cannot replay that on restore:
`Cannot rename './etc/periodic/monthly/if-post-up.d' to './etc/network'`. The backup reports
success; the restore fails.

So the agent writes an origin beside each listing (`tree` for the rootfs or a pool snapshot,
`copy` for a live copy) and takes a **full copy** instead of an incremental when:

| Reason shown behind the triangle | When |
|---|---|
| "this run read a fresh copy of the rootfs (a dir pool's live copy) …" | The container is running on a dir pool, so this run reads a copy |
| "the previous backup in this chain read a copy of the rootfs …" | The run before it read a copy, so its listing is from another numbering |
| "this chain's listing predates McHarbor recording which tree it was read from …" | The listing was written by an agent at build `202609271856` or older. Happens once per chain after the agent update, on every pool backend |
| "this container's incremental listing is not on the host …" | The listing is gone |

A full taken this way heads a **new chain**: the run moves onto it, and the next incremental
builds on it. On a dir pool, stop the container before a run if you want incrementals;
an at-rest run reads the rootfs itself. Freezing it is not enough: a frozen container on a dir pool
is still read through a copy.

## How the copy is taken

A backup of a filesystem that is being written to is not a backup: it can hold half of a package
upgrade or a database mid-checkpoint, and it cannot be told from a good one. So one of three
things happens, and the run records which.

| Recorded as | When | What it costs |
|---|---|---|
| **From a snapshot** | The container is running and its pool handed back a readable point-in-time copy of the rootfs tree | Nothing, the container never stops. btrfs, ZFS and LVM thin pools; also a dir pool when the operator had already frozen the container |
| **Frozen** | The container is running and the pool's snapshot could not be taken or read | Its processes are frozen for the length of the archive, and thawed on every path out, including a failure |
| **At rest** | The container is not running | Nothing |
| **Live copy** | The container is running on a dir pool: its rootfs is copied with no snapshot and no freeze | Nothing while it runs, and the copy is **not point-in-time**. Files are read at different moments, so a database or a package upgrade in progress can be captured half-written |

A running container that can be neither snapshotted, copied nor frozen is **refused**. The dir
pool's live copy is the one deliberate exception, and it is recorded as **Live copy** so it never
reads as a snapshot.

### Per pool backend

The run record says **From a snapshot** for every backend below; the progress detail of the run
names the mechanism it actually used.

| Pool | What the archive reads | Falls back to the freezer when |
|---|---|---|
| **btrfs** | A read-only subvolume snapshot beside the rootfs, `<rootfs>.snap.mcharbor-ctbk-<run>` | The rootfs is not a subvolume |
| **zfs** | `<mountpoint>/.zfs/snapshot/mcharbor-ctbk-<run>/…` of the dataset that holds the rootfs. Nothing is mounted by McHarbor; ZFS mounts the snapshot on first access | The dataset's `mountpoint` is `legacy` or `none`, the dataset is not mounted, or `snapdir=disabled`. The default `snapdir=hidden` only hides `.zfs` from directory listings; the path still resolves, so it works |
| **lvm** | A thin snapshot volume `<vg>/mcharbor-ctbk-<run>`, activated and mounted read-only with `nodev,nosuid,noexec` under `/run/mcharbor-agent/ct-snap/mcharbor-ctbk-<run>/`. xfs is mounted with `nouuid`, because the snapshot carries the origin's filesystem UUID | The volume holding the rootfs is not a thin volume (a thick snapshot needs a size McHarbor cannot choose), or its filesystem is not ext2/3/4 or xfs |
| **dir** | A full copy of the rootfs, `<rootfs>.snap.mcharbor-ctbk-<run>`, taken while the container keeps running, recorded as **Live copy** | The copy fails |

**The dir copy is not free, and not point-in-time.** The container is never frozen, so the local
`cp -a` reads files at different moments while it keeps writing; for a consistent dir-pool backup,
stop or freeze the container before the run. The host needs room for a second copy of the rootfs
while the run lasts. A live copy is always taken as a **full copy**, even when the schedule
asks for an incremental. See [Why a dir pool's live copy is never an
incremental](#why-a-dir-pools-live-copy-is-never-an-incremental). On a btrfs, ZFS or LVM pool the
snapshot keeps the files' inode numbers, so incrementals stay small whichever tree a run read.

### Cleanup

The snapshot, the copy or the mount is removed on every path out of a run: success, failure and
cancellation. On LVM that is unmount, remove the mount directory, then `lvremove`; an unmount that
cannot complete falls back to a lazy one.

An agent killed mid-run cannot clean up after itself, so everything a run creates is named
`mcharbor-ctbk-<run>` and swept later:

- **when the agent's compute runtime starts**: every stale mount under
  `/run/mcharbor-agent/ct-snap/`, every snapshot volume with that name and an origin, and every
  ZFS snapshot with that name, host-wide;
- **before every backup**: the same mounts and volumes, the ZFS snapshots of that container's
  dataset, and that container's leftover dir or btrfs copies beside its rootfs.

A run still in flight is never swept. An operator's own snapshots, volumes and mounts never carry
the prefix and are never touched.

Snapshot quiesce on ZFS, LVM and dir pools needs an agent built from `2026.09.01` after build
`202609270250`; an older agent freezes as before.

## Retention

Per schedule, two rules, either of which may be off: keep the last *N* successful runs, and keep
runs for *D* days. A run that survives either rule survives. Zero means forever.

Two rules outrank both:

- **A run a later incremental still builds on is never deleted.** Removing it would leave a chain
  whose base is gone, and nothing would report that until somebody tried to restore.
- **The single newest backup of a container that still exists is never deleted.** A retention
  policy that can leave a live container with no backup at all has stopped being one.

Deleting a run removes the objects at every destination first and the row second. A host that
cannot remove objects from a destination keeps both and says so in the log, rather than dropping
the record and leaving the bytes.

## Restoring

Restoring rebuilds the container's root filesystem from the backup and everything the chain
needs. It replaces everything on that filesystem now and cannot be undone, so:

- the container has to be **stopped**: replacing the rootfs under a running init leaves its
  processes holding inodes that no longer exist, and it does not fail cleanly;
- the operator types the **container's name** rather than ticking a box;
- the whole chain is checked **before a byte moves**; a missing link is refused with the reason;
- a chain whose incrementals were read from a dir pool's live copy is **refused** before a byte
  moves, naming the full backup at its head to restore instead (see below);
- the tree is rebuilt **beside** the live one and moved into place in one step at the end, so the
  host needs room for a second copy while it runs and every earlier failure leaves the container
  exactly as it was. A recreate whose restore fails also removes the `containers/<id>/`
  directory it created, so no empty directory is left on the pool.

### Dir-pool chains taken before the fix

Agents at build `202609271856` and older took incrementals from a dir pool's live copy. Those
incrementals cannot be restored. A restore or recreate of one, or of any run in a chain that
contains one, is refused with "This backup cannot be restored" (`err.ct.backup_chain_unrestorable`,
409), and the detail names the full backup the chain starts with and when it was taken. That full
restores normally; only what changed after it is lost. Recognised by the runs' **Live copy**
consistency, and by **From a snapshot** on a container still known to be on a dir pool (a copy
taken while the operator had frozen it). Take a new full backup of each such container after
updating the agent.

### Restoring into another id range

The archive holds **host** ids (`--numeric-owner`): an unprivileged container's root is stored as
the first id of its range, for example `1000000`. Every unprivileged container has its own range,
so a restore into a different container (a recreate while the original still exists, or a
restore with another target) lands in a different range. The host therefore re-owns the rebuilt
tree into the target's range **after the whole chain is applied and before it replaces the
rootfs**: each host id `h` in the source range becomes `h - source start + target start`.

- The owner is changed with `lchown`, so a symlink is re-owned itself and never followed.
- `chown` clears setuid, setgid and file capabilities, so the mode is re-applied and the
  capability re-written afterwards. POSIX ACL entries naming a user or group are remapped, and so
  is the root id of a v3 file capability (the kind a namespaced root sets); v1 and v2 capabilities
  carry no id and are kept as they are.
- **Privileged to unprivileged** and back both work: a privileged container's ids are its own, so
  they shift up into an unprivileged range, or down from one.
- An owner **outside the source range** (for example a file a host administrator created as host
  root inside the rootfs) is **refused**, naming the first entries and the count. Inside the source
  container such a file had no owner at all (it showed as `65534`, the overflow id), so giving it
  to a real user of the target would change who can modify it. The whole tree is checked before
  anything changes and the target rootfs is left untouched; re-own those files on the source and
  back up again, or restore into a container with the source's range. The usual source of such a
  file was the pool's file browser (the pool detail page's **Files** tab), which created files and folders
  as host root until the agent was rebuilt with addon-lxc `2026.09.13`. From that agent on, a file
  written or a folder created anywhere inside `containers/<id>/rootfs/` belongs to that
  container's root (the first id of its range, or `0` for a privileged container); overwriting a
  file keeps its owner when the container can see it, and gives an old host-root file to the
  container's root. The owner is read off the rootfs root itself, set on the new entry without
  following a symlink, and nothing outside a container rootfs changes.
- A restore into the container the backup came from, or between identical ranges, moves nothing.

Where the source range comes from, most reliable first:

1. what the backup runs recorded when they were taken (addon-lxc `2026.09.12` and newer). A chain
   whose runs recorded **different** ranges is refused before anything moves with "the backups in
   its chain recorded different id ranges" (`err.ct.backup_idmap_conflict`, 409);
2. for an older backup, the source container, while McHarbor still has it: a container keeps its
   range for life;
3. the manifest's `idMap`, or its `privileged` flag for a manifest written before `idMap`;
4. otherwise the host reads it off the restored tree: the rootfs root must own the lowest id and
   every owner must fall inside the one 65536-id block above it, `0` meaning privileged. A tree
   that does not show one range unambiguously is refused with "cannot tell which id range this
   backup was taken in", and the target is left untouched.

The restore run's log says which of these it used, and the run itself records what the host did
with the owners (addon-lxc `2026.09.13` and newer). It is returned as `idShift` on the run
(`GET /api/system-containers/{id}/backups`) and shown next to **Restore** in the backup history:

| `idShift` | Shown as | Meaning |
|---|---|---|
| `none` | **Owners unchanged** | The backup and the target share an id range, so no owner moved |
| `shifted` | **Owners moved** | Ownership moved to this container's id range |
| `skipped` | **Owners not moved** | The host restored the files as archived; the run is failed when owners could have moved |

A restore run from before `2026.09.13`, or one the agent never answered, shows nothing. An agent without this change ignores the
ranges; McHarbor notices that its reply says nothing about ownership and marks any restore that
could move owners as **failed** ("...too old to move their owners into this container's id
range..."). The files were restored, but still belong to the source range: update the agent and
restore again.

A restore can target a **different container** than the backup came from. The typed name is
checked against the container being written over, not the one the backup came from. To bring
back a container that no longer exists, use **Recreate** below instead of building one by hand.

## Recreating a deleted container

Deleting a container never deletes its backups: runs are keyed by the container's id with no
link to its row, so they outlive it on purpose. They are found again under **Backups of deleted
containers** on the System Containers list, grouped by the container they came from, and each
successful run offers **Recreate as a new container**. The same action is on every successful
backup in a live container's **Backups** tab, for a copy beside the original.

Recreate reads the run's `<run>-manifest-<kind>.json` back from the first destination that
still holds it and opens a form pre-filled with the definition it carries:

| From the manifest | Notes |
|---|---|
| Name, host | Change the name when it is taken; a clash is refused as "A container on this host already has that name" |
| Storage pool | The manifest names a backend and a path, not a pool. The pool on the chosen host with the same path and backend is picked; when there is none the form says so and an operator picks another |
| Init system, privileged, nesting | An init McHarbor does not run falls back to systemd, with a warning |
| Memory, CPU and process limits | Other limits in the manifest are carried over unchanged. A negative value is reset to no limit, with a warning |
| Interfaces | Each keeps its network, address mode and static address. An interface whose network no longer exists is flagged; pick another or remove it. Interfaces get **new MAC addresses** |
| Mounts | Listed and removable. A volume mount needs the volume to exist on the new pool |

Nothing is built until **Recreate container** is pressed. Then, in order: the backup chain is
checked (a missing link is refused before anything is written), the container is defined
(stopped, with no rootfs yet), the chain is restored into its rootfs path, and only then does the
host build the container around the restored tree. No image is unpacked just to be replaced.
Progress is the restore run in the new container's **Backups** tab and on the host's
**Backups** page. **Start it once the restore finishes** starts it after the last step.

If the restore or the final build fails, the container stays in the list as **Crashed** with the
reason attached, and the restore run says the same. Delete it and recreate again.

- **Permission keys** `ct.restore` and `ct.manage`, both on the host the container is created
  on; `ct.privileged` as well when the definition is privileged, nested, or binds a host path
  or device. Reading the manifest and listing deleted containers' backups takes `ct.backup`.
- **Routes** `GET /api/system-containers/orphaned-backups` (`?env=`, `?limit=`),
  `GET /api/system-containers/backups/{runId}/manifest`, `POST /api/system-containers/backups/{runId}/recreate`.
- **CLI** `mcharbor lxc backup orphaned`, `mcharbor lxc backup manifest <runId>`,
  `mcharbor lxc backup recreate <runId> --pool-id <pool> [--name …] [--environment-id …] [--start]`.
  Without `--data` the server reads the manifest itself and the flags override it; the manifest
  names no pool, so `--pool-id` is needed unless `--data` supplies a full `create` object (the
  `create` field of `lxc backup manifest -o json`).

Every restore **invalidates the chain**: the filesystem no longer matches what the listing has
been tracking, so the next backup is a full copy.

## What goes wrong

| Symptom | Cause | Fix |
|---|---|---|
| "This container's host runs an agent too old to back up a container" | The agent predates the four `ct_backup_*` messages | Rebuild and reinstall the agent; a native agent self-updates only from a published release |
| "System container backups need GNU tar on the host" | `bsdtar` or `busybox tar` | Install GNU tar on the host |
| "This container cannot be quiesced, so a backup of it would not be restorable" | Running, no pool snapshot, and the cgroup freezer refused | Stop the container for the backup, or move it to a pool that can snapshot |
| A running ZFS or LVM container's backup says **Frozen** | The snapshot was not readable: a `legacy`/`none` mountpoint, `snapdir=disabled`, a thick LV, or a filesystem other than ext2/3/4 or xfs. The agent log names the reason | Nothing is wrong with the backup. For a snapshot instead, give the dataset its own mountpoint, or put the rootfs on a thin volume |
| A `mcharbor-ctbk-…` mount, volume, snapshot or `rootfs.snap.mcharbor-ctbk-…` directory is still on the host | The agent was killed or the host lost power mid-backup | Nothing to do: the next agent start or the next backup of that container removes it |
| A dir-pool backup fails for lack of space | The copy needs room for a second rootfs | Free space on the pool, or move the container to a copy-on-write pool |
| "This backup has nowhere to go" | The schedule names no destination, or one it names was deleted | Pick a destination under **Settings → Storage** and edit the schedule |
| A failed run's error reads "<destination>: storage stream: no backup encryption key is installed on this host" | McHarbor has no backup encryption key, so every destination refuses the encrypted upload | Install one under **Settings → Storage → Backup encryption key**, then run the backup again. The error dialog on the run names this page |
| A schedule shows no next run | It is switched off, or has no schedule expression | A plan cannot be switched on without one |
| Tonight's incremental moved the whole filesystem | The listing was lost: a rebuilt host, a moved pool, a reverted rootfs | Nothing to fix. The chain restarted itself; the reason is behind the triangle in the Kind column |
| "This backup's chain is incomplete" on a restore | A link was pruned or never reached a destination | Restore an older full copy, or the newest one whose chain is whole |
| "This backup cannot be restored: its incremental backups were read from a live copy of a dir-pool rootfs" | An incremental taken on a dir pool by an agent at build `202609271856` or older | Restore the full backup the detail names. Nothing was changed on the host |
| Every backup of a running dir-pool container says "Taken as a full copy" | A live copy is never taken as an incremental | Expected. Stop the container before the run for incrementals |
| The first incremental after an agent update was a full copy on every pool | The chain's listing had no recorded origin | Expected once per chain |
| A run says "Interrupted: McHarbor restarted while this was running" | A reload or a recreate during the run | Start it again. The chain was invalidated, so the next run is a full copy |
| "This backup's container definition is no longer at any of its destinations" on Recreate | The manifest object was removed from every destination, or none can be reached | Create a container by hand with the same pool and restore into it |
| "This backup's container definition could not be read" | The manifest object is not JSON, is larger than any manifest McHarbor writes, or has a version this plugin does not read | The detail names which. Create the container by hand and restore into it |
| A recreated container shows **Crashed** | The restore or the host's final build failed; the reason is on the container and on its restore run | Delete it and recreate again once the cause is fixed |
| A container restored or recreated from another container's backup shows every file as `65534` inside, and root gets "Permission denied" in `/root` and `/etc` | Restored by addon-lxc `2026.09.11` or older, which never moved the owners into the new container's id range | Stop it and restore the same backup into it again. If it holds data worth keeping since then, back it up first |
| "the backup's owners do not fit this restore's id ranges: N entries cannot be moved ..." | Files owned by a host id outside the source container's range, named in the message | Re-own them into the source range on the source host and back up again, or restore into a container with the source's range. Nothing was changed |
| "cannot tell which id range this backup was taken in" | A backup from before the range was recorded, whose container is gone, whose manifest has none, and whose files span more than one range | Restore into a container with the original's range, if one exists. Nothing was changed |
| "...too old to move their owners into this container's id range..." on a restore run | The host's agent predates re-owning; the files arrived owned by the source range | Update the agent, then restore again |
| A file created or edited through the pool's file browser inside a container's rootfs shows as `nobody` (`65534`) in the container, and a later restore of its backup into another container is refused for it | The host's agent predates addon-lxc `2026.09.13` and wrote the file as host root | Update the agent. Saving the file again through the file browser gives it to the container's root; a folder needs `chown <range start>:<range start>` on the host |
| "This backup cannot be restored: the backups in its chain recorded different id ranges" | The runs of one chain disagree on their range, which no real chain does | Restore the full backup at the head of the chain, or another chain |
| A template's backup button is disabled | A template's rootfs is the backing store for every clone of it | Back up the clones, not the template |

## Related

- [System Containers](../README.md): the plugin
- [Snapshots and what they are not](../configuration.md): the rollback points on the same pool
- [Storage locations](../../settings/README.md): where the destinations are configured
- [Report catalog](../../reporting/reports/README.md): the protection report, which names each
  container's newest backup
