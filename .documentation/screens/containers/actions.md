# Containers: Actions

Everything you can do to a container, and the permission each one needs. Two are worth
knowing before you click them: remove offers to take volumes, image, stack and network
with it, and editing recreates the container rather than amending it.

| Action | Gated by | What it does |
|---|---|---|
| **Start** / **Stop** / **Restart** | `containers.manage` | Lifecycle. Stop confirms first |
| **Pause** / **Resume** | `containers.manage` | Freezes and thaws the container’s processes |
| **Kill** | `containers.manage` | "This will forcefully kill the container", with no graceful stop |
| **Recreate** | `containers.manage` | Replaces the container with a new one built from its current image and configuration. Offered per container in the table, the card and the detail hero; "Recreate All" does the same across every container |
| **Remove** | `containers.delete` | Offers to take its volumes, image, stack and network with it. "This action cannot be undone" |
| **Create** | `containers.manage` | Three ways in (a form, a Compose service, or a `docker run` command), all filling the same fields |
| **Edit** | `containers.manage` | Opens the configuration editor. Editing is locked until you confirm, because saving recreates the container. Environment values shown as `••••••••` are kept as they are unless you type a new one |
| **Reveal values** | `containers.manage` | On the `environment` tab, when a value is withheld. Shows the credential-looking values and writes a `reveal_env` audit entry; McHarbor's own keys stay withheld. See [Environment values](./fields.md#environment-values) |
| **Move** | `containers.manage` | Transfers the container to another environment, naming the required changes first |
| **Rename** | `containers.manage` | Renames it; a protected container needs unlocking first, and `host_self.manage` on a McHarbor container |
| **Take over** | `containers.manage` | Adopts a container McHarbor did not create |
| **Relink stack** | `containers.manage` | Re-attaches a container whose stack link was lost |
| **Terminal** / **Logs** / **Files** / **Backups** | the tab's own key | Shortcuts straight to that tab |
| **Debug shell** | `containers.debug_shell` | On the Terminal tab. For an image with no shell: confirms, uploads a static busybox to `/.mcharbor-debug/busybox`, connects the terminal to it, and removes it when you disconnect (at the latest after 15 minutes). Refused for a container that has a shell, is stopped, or has a read-only root filesystem. See [Debug shell](./debug-shell/README.md) |
| **Open Website** | `containers.view` | Offered when the container publishes an HTTP port |
| **Scan image** | `scans.manage` | Runs the configured scanner against the container's image |
| **Test healthcheck** | `containers.manage` | On the `environment` tab. Runs the healthcheck command once and shows the verdict and output; recorded in exec history like any exec. Disabled while the container is stopped. See [Detail-tab fields](./fields.md#detail-tab-fields) |
| **Auto-heal** | `autoheal.manage` | Restarts the container when its health check fails, tracking restarts and last heal |

## McHarbor's own containers

On a container that belongs to McHarbor (this node, an HA peer, a McHarbor helper, or one
labelled `com.mcharbor.protected`), Start, Stop, Restart, Pause, Resume, Kill, Recreate,
Edit, Rename, Move and Remove also need `host_self.manage`. The buttons render disabled for a caller
without it, and the tooltip reads "Requires the host_self.manage permission". The same goes
for **Terminal**, **Exec**, **Debug shell** and the **Files** write buttons on any container of
the environment McHarbor runs on that mounts the Docker socket, `/`, `/proc`, `/etc` or
McHarbor's data, or runs privileged: a shell there is a shell on McHarbor's host.

Creating or recreating such a container on that environment needs `host_self.manage` too;
the create is refused with a message naming the key and what the container would reach.
Holding `host_self.manage` does not unlock McHarbor's own container for stop, remove or file
writes; that lock keeps the application running and stands for everyone. See
[McHarbor's own containers](../../api/endpoints/containers.md#mcharbors-own-containers) for
the full list and the detection rules, and [Self-protection](./self-protection/README.md) for every path that is gated.


## Removing a container

The remove dialog carries four separate checkboxes, all off by default: take its
**volumes**, its **image**, its **stack** and its **network** with it. This is where
accidental data loss happens: an anonymous volume holding a database is still a volume.

## Editing recreates

Saving the editor stops the container, recreates it with the new configuration, and starts
it again: "any in-memory state will be lost". The confirmation lists exactly which fields
changed. Some fields apply live without a recreate and the editor says which.

A container that belongs to a Compose stack carries its own warning: the stack file is the
source of truth, and an edit here is overwritten on the next deploy.

### Mounts

The editor's **Mounts** section adds, changes and removes mounts: **Host path**, **Volume** or
**Memory (tmpfs)**, with source, destination and read-only. It opens with a warning, "Changing
mounts recreates the container": data is not moved, a mount you remove or change is detached
from the new container, and a new host path or volume starts empty. Binding a host path on
the environment McHarbor runs on needs `host_self.manage`, as anywhere else.

A mount is refused when its destination is not an absolute path, when two mounts share a
destination, when a host path is not absolute, or when a volume name is not valid. The
secret-files mount at `/run/secrets` is never offered for editing: it belongs to secret file
mode, and an edit that names it is refused. It is also left out of the Mounts tab and the
inspect view unless you reveal the container's values (`containers.manage`), so who
receives secret files, and from which volume, is part of the same audited reveal.

### Exposed ports

While editing, the **Exposed Ports** card lists each exposed port with a remove button, or a
lock saying why it stays: **Declared by the image** (an `EXPOSE` the daemon adds back on
every create) or **Used by a port mapping**. Removing a port mapping also releases its
container port, unless the image declares it or another mapping still uses it.

## Bulk actions

Select rows and the bulk bar reports "N of M selected" with the actions that apply.
Progress is reported as it runs ("12 of 20 complete (11 succeeded, 1 failed, 0 running)"),
so a partial failure stays visible instead of being rolled into one toast.

| Action | What it does |
|---|---|
| **Start**, **Stop**, **Restart**, **Remove** | Applied across the selection, with per-item progress |

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Containers](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
