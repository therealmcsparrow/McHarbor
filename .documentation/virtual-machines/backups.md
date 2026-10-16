# Backups

Copies of a machine's disks, written to storage away from its host, on a
schedule, with retention, the same shape container backups already have.
Incremental after the first copy, so a nightly backup moves only what changed
rather than the whole disk every night.

## What a backup is here

One run copies every disk a machine has, except discs and read-only disks: an
installer ISO is not the machine's state and copying it would double the
transfer for nothing.

The machine **keeps running**. Its filesystems are frozen through the guest
agent for the length of the copy and thawed immediately afterwards, not for
the upload, which would freeze a busy machine for however long the network
takes.

Where the guest agent is not installed, the copy is **crash-consistent**: it
looks like the machine lost power. That is recoverable for a journalling
filesystem and not for a database mid-write, and each run records which of the
two it got rather than claiming the better one.

## Where backups go

The same destinations container backups use: the storage locations configured
under **Settings**, provided by the `storage-*` plugins: SFTP, NFS, Samba,
Nextcloud, OneDrive, SharePoint, Box, Dropbox and the rest, plus `local`.

A plan writes to **every** destination it names, and it must name at least
one; a plan with no destination is refused at creation, because a backup that
goes nowhere is not a backup. More than one is a good idea for the same reason.

### How the bytes travel

Worth understanding, because it explains both the disk-space requirement and
why no credential ever reaches a remote host:

1. The agent produces the artifact on the **host** and stages it there.
2. It streams that file to McHarbor over a one-shot, token-authenticated
   upload it cannot use twice.
3. McHarbor **encrypts the stream in flight** and pipes it straight out to
   every destination at once.

**McHarbor stages nothing.** The artifact is never written to McHarbor's own
disk, so backing up a 200 GB machine does not require 200 GB free on the
McHarbor host.

**The host does need scratch space.** A full copy needs roughly the machine's
used disk size free in the compute runtime directory; an incremental needs only
the delta. The staged file is removed whether the run succeeded or failed.

### Encryption

Every backup is encrypted, and it cannot be turned off. The master key stays on
McHarbor: the plugin never holds it and neither does the agent, which is a
stricter arrangement than container backups, where the key is handed to the
agent so it can seal the archive itself.

A host with no backup encryption key installed refuses to back up rather than
writing in the clear. Install one under **Settings → Security**.

## Full and incremental

The first run of a chain copies the whole disk and starts a **change log**:
a persistent QEMU dirty bitmap stored inside the image, which survives the
machine restarting. Later runs copy only the clusters that changed.

### When a full copy is taken anyway

- The first backup of a disk.
- **Every N runs**, where N is the plan's *Take a full copy every* setting
  (7 by default, 0 to never force one). Restarting the chain regularly keeps a
  restore short.
- The previous run failed. Building a delta on a base that may never have
  reached its destination is not safe.
- The chain was **invalidated** (below).
- The operator asked for one.

### When a chain is invalidated

A change log describes a disk. Anything that changes the disk underneath it
makes the log describe something that is no longer there, and an incremental
taken afterwards restores to a disk that **looks intact and is not**. So the
chain is broken deliberately, and the next run is a full copy:

| Event | Why |
|---|---|
| The disk is resized | The new space was never recorded as changed, so an incremental would omit everything written into it |
| The machine is reverted to a snapshot | Every disk moves back in time while its change log still describes the future it came from |
| A disk is restored from a backup | Its contents no longer match what the log has been tracking |
| A disk is removed | The log lived inside the image that is gone |

### The check that actually protects you

Before every incremental, the **host** (not McHarbor) reads the live bitmap
and confirms three things: that it exists, that it is still recording, and
that it is not marked inconsistent after an unclean shutdown. If any of those
fails, the host takes a **full copy instead** and says why.

McHarbor then records a full run. **The history never shows an incremental
that was not actually taken**, because a restore would apply a delta on top of
a base that does not contain what it assumes.

A run that was downgraded shows a warning icon in the history with the host's
own reason, which is also the explanation for why tonight's "incremental"
moved the whole disk.

## Scheduling

A plan carries its schedule as a readable interval ("every 1 day starting at
02:00"), with the raw cron expression underneath for anything the builder
cannot express. The history shows when a plan last ran and when it is next due.

Plans are checked once a minute. A plan whose previous run is still going is
skipped rather than started twice: two backups of one machine at once would
fight over the same change log.

## Retention

Two rules, either of which may be off, and a backup that survives either rule
survives:

- **Keep this many backups**: the last N successful runs.
- **Keep backups for this many days.**

Zero means forever for both.

**One rule outranks them.** A backup that a later incremental still builds on
is never removed, however old it is or however the numbers are set. Deleting it
would leave every backup after it unrestorable, and nothing would report that
until somebody tried to restore.

Retention removes the objects from their destinations, not only the record of
them. Where a destination's plugin cannot delete, McHarbor says so in the log
and leaves the object alone rather than quietly losing track of it.

## Restore

**Restore replaces the machine's current disks. It cannot be undone.**

Three things are checked before a single byte moves:

1. The machine is **stopped**. QEMU holds its disks open while it runs, so this
   is a refusal rather than a warning.
2. The operator confirmed, explicitly, in the dialog.
3. The **whole chain is present**: the full copy the backup builds on and
   every incremental between them. A gap is refused by name rather than
   restoring part of a chain and producing a plausible-looking disk.

Deleting a full copy that later incrementals depend on is refused for the same
reason, and says how many backups depend on it.

A restore is recorded as a run of its own, alongside the backups, so the
history shows both in one place.

### Free space on the host

A restore assembles each disk **beside** its target and only moves it into
place once it is complete, so a failure at any point before that last step
leaves the machine exactly as it was. The price of that safety is room for a
second copy: the host needs roughly **twice** the machine's disk size free
while a restore runs: the rebuilt disks plus the chain that produces them.

The agent checks free space **before** downloading anything and refuses by
name if there is not enough, rather than failing an hour into a transfer with
the machine already down. The restore dialog states the figure before you
confirm.

## Actions

The machine's **Backups** tab and the host's **Backups** page offer the same actions on a
machine's backups, as icon buttons in a titled **Actions** column:

| On | Action | Gated by |
|---|---|---|
| A schedule | **Back up now**: runs that schedule immediately | `vm.backup.manage` |
| A schedule | **Edit** (pencil) | `vm.backup.manage` |
| A schedule | **Delete**: removes the schedule, not the backups it wrote | `vm.backup.delete` |
| A backup | **Restore** | `vm.restore.run` |
| A backup | **Delete**: removes the backup and the copies it wrote | `vm.backup.delete` |
| Either, on the Backups page | **Open**: goes to the machine's Backups tab | `vm.view` |

Both deletes ask for confirmation. On the host's Backups page, **Restore** and **Edit** open
the machine's own Backups tab with that restore or that schedule's editor already open,
because those dialogs belong to the Virtual Machines plugin.

A machine's backup has no **Download** and no **Cancel**, unlike a container backup:
there is no route for either yet.

## Permissions

| Key | Covers |
|---|---|
| `vm.backup.view` | Seeing plans, history and where each backup was written |
| `vm.backup.manage` | Creating and editing schedules, starting a backup |
| `vm.backup.delete` | Removing a backup and the copies it wrote |
| `vm.restore.run` | Rebuilding a machine's disks from a backup |

`vm.restore.run` is separate from `vm.backup.manage` on purpose, the same
separation container backups make between `backups.manage` and `restore.run`.
Taking a backup is additive; restoring one destroys what is there now.

## What goes wrong

| Symptom | Cause |
|---|---|
| "This backup has nowhere to go" | The plan names no destination, or the one it named has been deleted |
| Backups refuse to start at all | No backup encryption key is installed on this McHarbor |
| Tonight's incremental moved the whole disk | The change log could not be used. The run says which of the reasons above applied |
| A run failed with no space | The **host** ran out of scratch space. A full copy needs roughly the machine's used size free |
| Restore is refused | The machine is running, or the chain is missing a link, or restore is not available on this build yet |
| A backup will not delete | A later incremental builds on it. Delete those first, or let retention restart the chain |

## See also

- [Virtual Machines](README.md)
- [QEMU features](qemu-features.md): the change-log mechanism backups use
- [Troubleshooting](troubleshooting.md)
