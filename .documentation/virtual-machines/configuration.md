# Virtual Machines configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- The `addon-compute` plugin, which is installed with this one automatically.
- A role holding at least this plugin's `view` key, or its sidebar section stays hidden.

## Where it appears

| Path | Label key | Permission |
|---|---|---|
| `/plugins/addon-kvm` | `nav.machines` | `vm.view` |
| `/plugins/addon-kvm/templates` | `nav.templates` | `vm.view` |

Sidebar entries are **hidden** when the permission is missing, unlike buttons, which
render disabled with a tooltip. A dead-end page is not informative.

## Storage

The create wizard asks for a storage pool when the host has one. When it has
none, it asks for a **data location** instead (a directory where guest disks
are kept as qcow2 files) and provisions a directory pool there through
[Compute Foundation](../compute-foundation/configuration.md) on submit. The
default is `/var/lib/mcharbor/compute`.

## Routes

Every route is mounted under the plugin router and carries the permission declared in
the manifest. The host mounts only what the manifest declares.

| Method | Path | Permission | Audit action |
|---|---|---|---|
| `GET` | `/api/vms` | `vm.view` | — |
| `GET` | `/api/vms/{id}` | `vm.view` | — |
| `GET` | `/api/vms/{id}/stats` | `vm.view` | — |
| `POST` | `/api/vms` | `vm.manage` | `plugin.kvm.vm.created` |
| `PUT` | `/api/vms/{id}` | `vm.manage` | `plugin.kvm.vm.updated` |
| `POST` | `/api/vms/{id}/start` | `vm.manage` | `plugin.kvm.vm.started` |
| `POST` | `/api/vms/{id}/shutdown` | `vm.manage` | `plugin.kvm.vm.shutdown` |
| `POST` | `/api/vms/{id}/stop` | `vm.manage` | `plugin.kvm.vm.stopped` |
| `POST` | `/api/vms/{id}/reset` | `vm.manage` | `plugin.kvm.vm.reset` |
| `POST` | `/api/vms/{id}/pause` | `vm.manage` | `plugin.kvm.vm.paused` |
| `POST` | `/api/vms/{id}/resume` | `vm.manage` | `plugin.kvm.vm.resumed` |
| `DELETE` | `/api/vms/{id}` | `vm.delete` | `plugin.kvm.vm.deleted` |
| `GET` | `/api/vms/{id}/disks` | `vm.view` | — |
| `POST` | `/api/vms/{id}/disks` | `vm.manage` | `plugin.kvm.disk.added` |
| `PATCH` | `/api/vms/{id}/disks/{diskId}` | `vm.manage` | `plugin.kvm.disk.resized` |
| `DELETE` | `/api/vms/{id}/disks/{diskId}` | `vm.delete` | `plugin.kvm.disk.deleted` |
| `POST` | `/api/vms/{id}/media` | `vm.manage` | `plugin.kvm.media.attached` |
| `DELETE` | `/api/vms/{id}/media/{diskId}` | `vm.manage` | `plugin.kvm.media.ejected` |
| `GET` | `/api/vms/{id}/nics` | `vm.view` | — |
| `POST` | `/api/vms/{id}/nics` | `vm.manage` | `plugin.kvm.nic.added` |
| `DELETE` | `/api/vms/{id}/nics/{nicId}` | `vm.manage` | `plugin.kvm.nic.deleted` |
| `GET` | `/api/vms/{id}/snapshots` | `vm.view` | — |
| `POST` | `/api/vms/{id}/snapshots` | `vm.manage` | `plugin.kvm.snapshot.created` |
| `POST` | `/api/vms/{id}/snapshots/{snapId}/revert` | `vm.manage` | `plugin.kvm.snapshot.reverted` |
| `DELETE` | `/api/vms/{id}/snapshots/{snapId}` | `vm.delete` | `plugin.kvm.snapshot.deleted` |
| `POST` | `/api/vms/{id}/clone` | `vm.manage` | `plugin.kvm.vm.cloned` |
| `POST` | `/api/vms/{id}/template` | `vm.manage` | `plugin.kvm.vm.templated` |
| `POST` | `/api/vms/{id}/devices` | `vm.privileged` | `plugin.kvm.device.attached` |
| `DELETE` | `/api/vms/{id}/devices/{devId}` | `vm.privileged` | `plugin.kvm.device.detached` |
| `POST` | `/api/vms/{id}/diagnose` | `vm.view` | — |

`GET /api/vms/host-devices?env=<id>` marks each PCI and USB candidate with `attachedTo`:
the machines on that environment that already have it. **Hardware → attach a device** labels
such a device "attached to a VM", warns "Already attached to &lt;names&gt;. Only one running VM
can use a passed-through device at a time.", and disables **Attach** when the machine you are
editing already has it ("This VM already has this device."). Attaching to a second, stopped
machine stays allowed; only one of them can start with it.

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
| `i18n.namespace.add:vm` | Merge the `vm` translation namespace into the host bundles. |
| `rbac.permission.register` | Reserve the permission keys it declares in `adds_permissions`. |
| `db.migrations.run` | Create and migrate its own tables at install time. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |
| `agent.rpc` | Send declared messages to a host's agent through the agent RPC bridge. Only takes effect with an `agent-rpc` capability. |
| `reporting.adapter.register` | Contribute report sections to the Reporting plugin. |
| `alerting.adapter.register` | Contribute alert conditions the host evaluates on a schedule. |
| `ai.invoke` | Call the configured AI provider through the host, without holding a key of its own. |

## Related

- [Virtual Machines](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
