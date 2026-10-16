# Backups: Actions

Everything you can do to a backup run or a plan, and the permission each one needs.
Restore is the part worth reading before you need it: an archive comes back in four
layers, you choose which, and an archive encrypted under an old key asks for that key.

| Action | Gated by | What it does |
|---|---|---|
| **Run now** | `backups.manage` | Runs a backup immediately |
| **Run plan** | `backups.manage` | Runs a saved plan on demand |
| **Cancel run** | `backups.manage` | Stops a running backup |
| **Restore** | `restore.run` | Writes archive contents back into the container ("can overwrite existing files") |
| **Restore from file** | `restore.run` | Uploads an archive that is not in this McHarbor at all, reads it, then offers the same choices as **Restore** |
| **Download** | `backups.download` | Downloads the archive |
| **Delete run** | `backups.delete` | Removes one run and its archive |
| **Create / edit plan** | `backups.manage` | Container, contents, destinations, schedule, retention, consistency |
| **Delete plan** | `backups.delete` | Removes the schedule, not the archives it produced |

**Virtual machine rows** (runs and plans the Virtual Machines plugin contributes) carry
that plugin's own actions and keys instead: Restore, Delete and Open on a run; Edit, Delete
and Open on a plan, gated by `vm.restore.run`, `vm.backup.manage`, `vm.backup.delete` and
`vm.view`. Restore and Edit open the machine's Backups tab with the dialog already open.
Download and Cancel are not offered for them. See
[Virtual machine backups → Actions](../../virtual-machines/backups.md#actions).

## Restore is selective

An archive is presented as four layers, and you choose what comes back:

| Layer | What it is |
|---|---|
| **Config** | The container’s configuration |
| **Filesystem** | The container’s own filesystem |
| **Mount** | Named volume contents |
| **Bind** | Bind-mounted host paths |

Within the filesystem and mount layers you can browse a file tree and pick individual
folders and files, with a selected count as you go. A very large tree is truncated, and the
page says so instead of pretending it showed you everything.

Ticking a folder restores everything beneath it. The files and folders under a ticked folder
show as ticked and cannot be unticked one by one; untick the folder and pick its contents
instead. After the restore, **Restored items** on the run marks every item a restored folder,
mount, filesystem layer or configuration covered as restored, not only the rows you ticked.
A restore run writes no archive of its own, so its size reads "—".

## Restoring configuration

Configuration is applied by recreating the container: McHarbor stops and removes it, then
creates and starts it again with the restored settings. The dialog says so before you press
**Restore**, under the title "This recreates the container". Data in volumes and bind mounts
is kept; the container's own filesystem layer is not, unless that layer is part of the same
restore.

When it is, the restore runs in two passes: first everything except the filesystem layer,
with configuration applied last, and then the filesystem layer (whole, or the files and
folders you picked in it) into the recreated container. A full restore of a full backup
therefore brings back configuration and files together.

## Restore from file

"Restore from file" takes an archive downloaded from this or another McHarbor and restores it
into the container whose Backups tab you are on. It works in two steps:

1. Choose the file and, if it was encrypted with a different key than this McHarbor's, enter
   that key under "Backup security key". Press **Read backup**.
2. The archive is stored as a backup run of this container and opened with that key. The
   restore dialog then shows the same layers and file trees as any other run, with the key
   already filled in, and nothing is written until you press **Restore**.

A wrong key or a file that is not a McHarbor backup is refused at **Read backup**, and nothing
is kept. The uploaded run stays in the run list afterwards, so it can be restored again or
deleted like any other.

Restoring into another server's container is done this way: **Download** on the original
run, then **Restore from file** on the new container. The new container needs the same mount
paths inside the container for the **Mount** and **Bind** layers to land where the app
expects them. Restoring **Config** recreates the container; restore the **Filesystem** layer in
the same restore and it is written into the recreated container (see
[Restoring configuration](#restoring-configuration)).

## When the key has changed

If an archive was encrypted with a different key than the current backup key, restore asks
for the original secret: "This backup was encrypted with a different key than the current
McHarbor backup key. Enter the original secret key to restore it."

## Bulk actions

Select rows and the bulk bar reports "N of M selected" with the actions that apply.
Progress is reported as it runs, as in "12 of 20 complete (11 succeeded, 1 failed, 0 running)",
so a partial failure stays visible instead of being rolled into one toast.

| Action | What it does |
|---|---|
| **Download** | Downloads the selected runs |
| **Delete** | Deletes the selected runs after a confirmation naming the count |

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence, such as "this cannot be undone" or "all its data", is quoting the
product, not padding.

## Related

- [Backups](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
