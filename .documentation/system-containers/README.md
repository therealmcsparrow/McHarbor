# System Containers

Runs full-system LXC containers on your own hosts: a whole distribution with its own init, users and services, in a fraction of a VM's memory and starting in about a second. Unprivileged by default, with CPU, memory and pid limits that apply live, plus a browser console, snapshots, clones and templates.

- **Slug** `addon-lxc`, **version** 2026.09.01
- **Host range** `>=2026.09.01`
- **Where it appears** its own section in the sidebar, and its own pages under `/plugins/<slug>`
- **Gated by** the permission keys it declares (see below)

## Before you install: what the host needs

A system container needs no hypervisor, so the list is shorter than for virtual machines,
but **a containerised agent still cannot host one**, because guests are supervised as systemd
transient units. Guest hosting needs the agent installed as a **binary under systemd**.

| Check | Fix if it fails |
|---|---|
| `systemd` as PID 1 | A systemd host. There is no degraded mode |
| cgroup v2 unified hierarchy | Boot with `systemd.unified_cgroup_hierarchy=1`, or use a newer distribution |
| User namespaces | A kernel with `CONFIG_USER_NS=y`; some distros also need `sysctl user.max_user_namespaces=15000` |
| Subordinate uids/gids | `sudo usermod --add-subuids 100000-165535 --add-subgids 100000-165535 <user>` |

**The last one is the trap.** `subuid` is not part of the "can run system containers"
calculation, so a host can report that it can and then run every container as root, because
there are no subordinate ids to map an unprivileged one into. Read the checklist, not only the
summary line.

Idmapped mounts (kernel 5.12+) are not required, but without them rootfs trees cannot be
shared or cloned without a chown pass.

Full detail: [Host requirements for guests](../compute-foundation/host-requirements.md). To go
from nothing to a container you can log into, follow
[Getting started with guests](../compute-foundation/getting-started.md).

## How it works

- McHarbor drives libcontainer directly and supervises it with systemd. There is no LXD, no Incus and no `lxc-*` toolchain involved.
- It requires [Compute Foundation](../compute-foundation/README.md) and consumes `compute.view` from it.
- Limits apply without a restart, which is the practical difference from a VM: resizing a guest is a live operation.
- `ct.console` and `ct.privileged` are separate keys. A privileged container can affect the host, so granting it is a deliberate decision rather than part of `ct.manage`.
- Two widgets, five workflow nodes, and the `ct` translation namespace.
- Backups leave the host through the existing storage locations: a compressed tar of the rootfs plus a manifest of the container's definition, with incremental chains. See [Backups](./backups/README.md).

## Creating and deleting a container

A container starts from an **image** or a **template**, never both.

- **From an image**: the image id names a rootfs tarball in the host's image cache. Download
  it to that host from the image library first; a create naming an image the host has not
  cached fails with "image … is not in this host's image cache". The agent unpacks it into
  `<pool target>/containers/<id>/rootfs`, detecting gzip, xz, zstd, bzip2 or a plain tar from
  the file itself.
- **From a template**: the template container's rootfs is copied, and every file is re-owned
  from the template's id range into the new container's.
- An unprivileged container's files are owned by its own subordinate id range on the host
  (container uid 0 becomes the range's first id). A privileged container keeps the ids the
  image was built with.
- Unpacking is refused outright (nothing is left on disk) for an archive member with an
  absolute path or a `..` component, a relative symlink that climbs above the rootfs, anything
  written through a symlink, a hard link to something outside the tree, or an owner outside
  the container's 65,536-id range. Device nodes in an image are skipped; the runtime provides
  `/dev`. Any failure removes the partial rootfs and the container directory it created.
- **Deleting** a container stops it, removes its host-side network interfaces by name, and
  removes its rootfs and the now-empty `containers/<id>` directory. The agent refuses to remove
  anything that is not strictly inside the container's pool, never the pool root and never
  `/`. **Keep the filesystem** on the delete dialog leaves the tree on disk.
- Building a rootfs from an image or template, and removing one on delete, need agent
  `2026.09.01` build `202609271429` or newer. An older agent is refused before anything is
  changed: the create leaves no container behind, the delete leaves the container in place,
  and both name the build to reach. Recreating from a backup does not need it.

## The console and the init system

A container has one terminal, `/dev/console`, and that is what the browser console and
`mcharbor lxc container console` attach to. It has no virtual terminals: `/dev/tty1` to `tty6`
do not exist.

The agent reads what `/sbin/init` actually is from the rootfs rather than trusting the
declared init system: **systemd** when it leads to systemd, **openrc** when busybox init runs
an installed `/sbin/openrc` (or `/sbin/init` is `openrc-init`), **busybox** when it is busybox
with no openrc (the alpine minirootfs, which the image catalog lists as openrc).

For a busybox-init rootfs, a create **from an image** prepares the console once:

- An image with no `/etc/inittab` gets a small one: a root shell on `/dev/console` and an
  unmount at shutdown. It has no `ctrlaltdel` line, so a Ctrl-C reaching PID 1 cannot reboot
  the container.
- An image whose inittab already runs something on the console (`console::…`, or an empty id
  with `respawn`/`askfirst`) is left exactly as shipped.
- Otherwise the getty lines on `tty1` to `tty6` are commented out with the prefix
  `#mcharbor-no-tty#`, and `console::respawn:-/bin/sh` is appended under a `# McHarbor:` note.
  Every other line is kept byte for byte, and the image's own file is saved beside it as
  `/etc/inittab.mcharbor-orig`.

A create from a template, a clone and a restore never touch the file; they carry whatever
the operator made of it. The console shell starts with "can't access tty; job control turned
off": PID 1 owns the console, so the shell cannot take it as its controlling terminal. Commands
run normally; background jobs and Ctrl-Z do not. For a full terminal, use **exec**, which gets
a terminal of its own, sized 80×24 until the browser sends its size. The minirootfs also prints
"can't run '/sbin/openrc'" three times at boot, because its inittab names openrc it does not
ship; that is harmless. Needs an agent built after `202609271856`.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `SupportsFor` | Yes |  |

## What it contributes

| | |
|---|---|
| Routes | 27 |
| Sidebar entries | 2 |
| Dashboard widgets | `ct-summary`, `ct-density` |
| Workflow nodes | `ct-lifecycle`, `ct-limits`, `ct-snapshot`, `ct-clone`, `ct-state`. The container field is a pick list of the environment system containers, and a typed id is still accepted |
| Migrations | `0001_init.sql`, `0003_ct_backups.sql` |
| Backend | 9 Go files, ~6,107 lines |
| Requires | `addon-compute` |

## Permissions it adds

| Key | Scope | What it allows |
|---|---|---|
| `ct.view` | env | View system containers |
| `ct.manage` | env | Manage system containers |
| `ct.delete` | env | Delete system containers |
| `ct.console` | env | Open a system container console |
| `ct.backup` | env | Back up system containers: schedules, history, starting and deleting |
| `ct.restore` | env | Restore a system container from a backup |
| `ct.privileged` | env | Run privileged containers and mount host paths |

It also consumes `compute.view` from another plugin, a key it
relies on but does not own.

## In this section

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Backups](./backups/README.md) | What an artifact contains, where it goes, the two permission keys, the quiesce rules, retention and restoring |
| [Snapshots and clones](./snapshots-and-clones/README.md) | Rootfs-copy snapshots, revert, and why a clone is a full copy |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../development/add-ons/system-containers.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Report catalog](../reporting/reports/README.md): the system container report this plugin contributes, and [the facets it answers](../reporting/facets/README.md) for the shared reports
- [Add-ons](../plugins/addons.md): what the six have in common, and what the plugin runtime does with them
- [Plugin catalog](../plugins/catalog.md): every shipped plugin
