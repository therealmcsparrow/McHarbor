# Compute Foundation configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A role holding at least this plugin's `view` key, or its sidebar section stays hidden.

## Where it appears

| Path | Label key | Permission |
|---|---|---|
| `/plugins/addon-compute` | `nav.overview` | `compute.view` |
| `/plugins/addon-compute/images` | `nav.images` | `compute.view` |
| `/plugins/addon-compute/pools` | `nav.pools` | `compute.view` |
| `/plugins/addon-compute/networks` | `nav.networks` | `compute.view` |

Sidebar entries are **hidden** when the permission is missing, unlike buttons, which
render disabled with a tooltip. A dead-end page is not informative.

## Pools without a pools page

`POST /api/compute/pools/default` returns the host's pool and, only when there
is none, creates a directory pool at the location the caller names, defaulting
to `/var/lib/mcharbor/compute`. It is what lets Virtual Machines and System
Containers ask for a data location instead of a storage pool: the pool row still
exists, because every guest disk is addressed through one, but nobody has to
create it first.

The directory backend is always offered. Creating a `dir` pool is a `mkdir`;
`qemu-img` is needed to allocate a disk in it, not to create it, so a host
missing the package gets a pool it can create and a stated reason disks will
fail until `qemu-utils` is installed.

## The downloadable image list

The catalog of images McHarbor can fetch is a directory of JSON files, one per
entry, bind-mounted at `/mnt/compute-catalog`. It is editable at runtime and is
the only way to change the built-in list, because a plugin's migrations run once
at install. See [Image catalog](./image-catalog.md).

A host can also be marked as **sharing the image library**, meaning its agent
mounts the library directory on its own cache root, so a file downloaded once is
already present there. It is a per-host switch on the Compute overview, and it is
your statement about your storage rather than something McHarbor probes: it
cannot see the far side of that mount. Turning it on re-reads that host's
inventory immediately, and **Download to all hosts** downloads once for the group
that shares.

## The shared image library

A catalog entry names a mirror and each host downloads from it. That shape has
nothing to offer an image nobody publishes (a golden qcow2, a vendor appliance,
an installer ISO), so uploads go into a directory instead.

McHarbor writes uploaded images to the directory named by
`MCHARBOR_COMPUTE_IMAGE_LIBRARY` (default `<data>/compute/images`). Every compute
host bind-mounts that same directory onto its agent's image cache root,
`/var/lib/mcharbor/compute/images`, a compiled-in constant, not a setting. An
image uploaded once is then already present on each host that shares the mount,
and nothing streams over the agent transport.

`docker-compose.yml` wires both halves through
`MCHARBOR_COMPUTE_IMAGE_LIBRARY_PATH` (default `./mnt/compute-images`). For a
host on another machine, point the bind at storage both machines can see: NFS,
SMB, or a replicated dataset.

McHarbor hashes an upload with SHA-256 as it receives it, and the agent re-hashes
the file before adopting it. That second pass is what puts the mount itself in
the trust chain: shared storage is writable by things that are not McHarbor.

**Make available** on an uploaded image's card runs that verification on the
chosen host. It fails with *the image is not in the shared library* when the bind
is missing on that host, which is the only way this feature goes wrong.

Uploading needs `compute.images.upload`, which ships in no role preset. It is
separate from `compute.manage` because it is a different authority, not a bigger
one: a catalog download fetches a named, checksummed image from a distribution,
while an upload puts bytes of the operator's choosing on the host's disk.

## Routes

Every route is mounted under the plugin router and carries the permission declared in
the manifest. The host mounts only what the manifest declares.

| Method | Path | Permission | Audit action |
|---|---|---|---|
| `GET` | `/api/compute/environments` | `compute.view` | — |
| `GET` | `/api/compute/environments/{envId}/capability` | `compute.view` | — |
| `POST` | `/api/compute/environments/{envId}/probe` | `compute.view` | — |
| `PUT` | `/api/compute/environments/{envId}` | `compute.manage` | `plugin.compute.environment.updated` |
| `POST` | `/api/compute/images/catalog/reload` | `compute.manage` | `plugin.compute.catalog.reloaded` |
| `POST` | `/api/compute/images/{id}/fetch-all` | `compute.manage` | `plugin.compute.image.fetched_all` |
| `GET` | `/api/compute/pools` | `compute.view` | — |
| `GET` | `/api/compute/pools/detect` | `compute.view` | — |
| `POST` | `/api/compute/pools` | `compute.manage` | `plugin.compute.pool.created` |
| `POST` | `/api/compute/pools/default` | `compute.manage` | `plugin.compute.pool.created` |
| `PUT` | `/api/compute/pools/{id}` | `compute.manage` | `plugin.compute.pool.updated` |
| `DELETE` | `/api/compute/pools/{id}` | `compute.delete` | `plugin.compute.pool.deleted` |
| `GET` | `/api/compute/networks` | `compute.view` | — |
| `GET` | `/api/compute/networks/detect` | `compute.view` | — |
| `POST` | `/api/compute/networks` | `compute.manage` | `plugin.compute.network.created` |
| `DELETE` | `/api/compute/networks/{id}` | `compute.delete` | `plugin.compute.network.deleted` |
| `GET` | `/api/compute/networks/{id}/leases` | `compute.view` | — |
| `GET` | `/api/compute/images` | `compute.view` | — |
| `GET` | `/api/compute/images/library` | `compute.view` | — |
| `POST` | `/api/compute/images/upload` | `compute.images.upload` | `plugin.compute.image.uploaded` |
| `POST` | `/api/compute/images` | `compute.manage` | `plugin.compute.image.added` |
| `POST` | `/api/compute/images/{id}/fetch` | `compute.manage` | `plugin.compute.image.fetched` |
| `DELETE` | `/api/compute/images/{id}/cache` | `compute.delete` | `plugin.compute.image.evicted` |
| `DELETE` | `/api/compute/images/{id}` | `compute.delete` | `plugin.compute.image.deleted` |
| `GET` | `/api/compute/overview` | `compute.view` | — |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Setting it up

- Install it from **App Store → Plugins**.
- Grant its permission keys to the roles that should see it.
- Open its section in the sidebar.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `menu.sidebar.add` | Add an entry to the sidebar. |
| `i18n.namespace.add:compute` | Merge the `compute` translation namespace into the host bundles. |
| `rbac.permission.register` | Reserve the permission keys it declares in `adds_permissions`. |
| `db.migrations.run` | Create and migrate its own tables at install time. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |
| `agent.rpc` | Send declared messages to a host's agent through the agent RPC bridge. Only takes effect with an `agent-rpc` capability. |
| `reporting.adapter.register` | Contribute report sections to the Reporting plugin. |
| `alerting.adapter.register` | Contribute alert conditions the host evaluates on a schedule. |
| `ai.invoke` | Call the configured AI provider through the host, without holding a key of its own. |

## Related

- [Compute Foundation](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
