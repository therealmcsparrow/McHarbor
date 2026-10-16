# System containers: snapshots and clones

Snapshots and clones of a system container are copies of its root filesystem on the host. Both
worked in the interface and the plugin before the agent implemented them, and the agent refused
every request. Both now run.

## Snapshots

A snapshot is a copy of the container's rootfs tree, kept in the same pool beside the container:

```text
<pool>/containers/<container id>/rootfs                     the container
<pool>/containers/<container id>.snap-<snapshot id>/rootfs  one snapshot
```

It is a copy and not a pool snapshot on purpose. A container's rootfs is a plain directory inside
the pool, not a dataset or subvolume of its own, so a ZFS or Btrfs snapshot would capture every
container on the pool and reverting it would roll all of them back. The copy keeps ownership and
special files, and it costs real space: each snapshot is a full copy, which the **Size** column
shows.

| Action | What happens | Key |
|---|---|---|
| **Create** | A running container is **frozen** around the copy so it is of one moment, then thawed, even when the copy fails. Turning **Freeze** off, or snapshotting a stopped container, gives a copy recorded as crash-consistent. A chain is refused at depth 16. | `ct.manage` |
| **Revert** | The container must be **stopped**. Its current rootfs is moved aside, the snapshot is copied in, and the old tree is removed only once the copy is in place, so a revert that fails half way leaves the container exactly as it was. | `ct.manage` |
| **Delete** | Removes the snapshot's copy from the pool. | `ct.delete` |

A revert **discards the history after the snapshot**: every later snapshot is removed from the pool
and from the list, because each one is its own copy and the revert made them meaningless. Deleting a
container removes all of its snapshots with it.

Routes are `GET|POST /api/system-containers/{id}/snapshots`,
`POST …/snapshots/{snapId}/revert` and `DELETE …/snapshots/{snapId}`, audited as
`plugin.lxc.snapshot.created`, `plugin.lxc.snapshot.reverted` and `plugin.lxc.snapshot.deleted`.
The agent refuses a snapshot request from an older System Containers plugin that does not say
where the rootfs is, with "update the System Containers plugin", because guessing a path on a host
that runs as root is not an option.

## Clones

**Clone** in the container's header (needs `ct.manage`) makes a new container from a stopped one,
on the same pool unless you pick another.

| Field | Meaning |
|---|---|
| **Name of the clone** | Required |
| **Share storage with the source** | Asks the pool for a linked clone. Disabled, with "This container's pool cannot share data, so the clone will be a full copy.", on a pool whose backend is not ZFS, Btrfs or LVM |
| **Start the clone when it is ready** | Starts it after the copy |

The container must be stopped: a running one is refused ("stop it before cloning").

### Clones are full copies

Every unprivileged container has its own id range. A clone is given a fresh one, because reusing
the source's would let the clone's root reach the source's files. Re-owning the tree into that range
changes every inode's owner, so **nothing can be shared with the source**, and the agent reports
the clone as a full copy. The request for a linked clone is therefore an ask, not an instruction:

- The clone is recorded as **linked** only when the host reports that it made a link. Today that
  is never, and the result reads "&lt;name&gt; was created as a full copy. It shares nothing with
  its source."
- A linked clone, when a host does make one, makes the source **undeletable** while the clone
  exists, for a template and for any other container. The refusal names the clones.
- A full copy is independent, so deleting the source stays safe.

Recording a link that is not there would make the source undeletable for no reason, which is why
the record follows the host's answer and not the request.

The route is `POST /api/system-containers/{id}/clone`, audited as `plugin.lxc.ct.cloned`. Command
line: `mcharbor lxc container clone <id> --name <name> [--linked]`.

## What goes wrong

| Symptom | Cause | Fix |
|---|---|---|
| Revert is refused | The container is running | Stop it, then revert |
| Snapshot create is refused at the depth limit | 16 snapshots in the chain | Delete old ones |
| Snapshot fails with "update the System Containers plugin" | The plugin predates the agent contract | Update the plugin |
| **Share storage with the source** is disabled | The pool's backend cannot share | Expected; you get a full copy |
| The source cannot be deleted | A linked clone exists | Delete the clone first |

## Related

- [System containers](../README.md)
- [Backups](../backups/README.md)
