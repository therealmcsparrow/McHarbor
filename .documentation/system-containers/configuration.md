# System Containers configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- The `addon-compute` plugin, which is installed with this one automatically.
- A role holding at least this plugin's `view` key, or its sidebar section stays hidden.

## Where it appears

| Path | Label key | Permission |
|---|---|---|
| `/plugins/addon-lxc` | `nav.containers` | `ct.view` |
| `/plugins/addon-lxc/templates` | `nav.templates` | `ct.view` |

Sidebar entries are **hidden** when the permission is missing, unlike buttons, which
render disabled with a tooltip. A dead-end page is not informative.

## Routes

Every route is mounted under the plugin router and carries the permission declared in
the manifest. The host mounts only what the manifest declares.

| Method | Path | Permission | Audit action |
|---|---|---|---|
| `GET` | `/api/system-containers` | `ct.view` | — |
| `GET` | `/api/system-containers/{id}` | `ct.view` | — |
| `GET` | `/api/system-containers/{id}/stats` | `ct.view` | — |
| `GET` | `/api/system-containers/{id}/processes` | `ct.view` | — |
| `POST` | `/api/system-containers` | `ct.manage` | `plugin.lxc.ct.created` |
| `PUT` | `/api/system-containers/{id}` | `ct.manage` | `plugin.lxc.ct.updated` |
| `POST` | `/api/system-containers/{id}/start` | `ct.manage` | `plugin.lxc.ct.started` |
| `POST` | `/api/system-containers/{id}/shutdown` | `ct.manage` | `plugin.lxc.ct.shutdown` |
| `POST` | `/api/system-containers/{id}/stop` | `ct.manage` | `plugin.lxc.ct.stopped` |
| `POST` | `/api/system-containers/{id}/restart` | `ct.manage` | `plugin.lxc.ct.restarted` |
| `POST` | `/api/system-containers/{id}/freeze` | `ct.manage` | `plugin.lxc.ct.frozen` |
| `POST` | `/api/system-containers/{id}/unfreeze` | `ct.manage` | `plugin.lxc.ct.unfrozen` |
| `DELETE` | `/api/system-containers/{id}` | `ct.delete` | `plugin.lxc.ct.deleted` |
| `PUT` | `/api/system-containers/{id}/limits` | `ct.manage` | `plugin.lxc.ct.limits_changed` |
| `GET` | `/api/system-containers/{id}/nics` | `ct.view` | — |
| `POST` | `/api/system-containers/{id}/nics` | `ct.manage` | `plugin.lxc.nic.added` |
| `DELETE` | `/api/system-containers/{id}/nics/{nicId}` | `ct.manage` | `plugin.lxc.nic.deleted` |
| `GET` | `/api/system-containers/{id}/mounts` | `ct.view` | — |
| `POST` | `/api/system-containers/{id}/mounts` | `ct.manage` | `plugin.lxc.mount.added` |
| `DELETE` | `/api/system-containers/{id}/mounts/{mountId}` | `ct.manage` | `plugin.lxc.mount.deleted` |
| `GET` | `/api/system-containers/{id}/snapshots` | `ct.view` | — |
| `POST` | `/api/system-containers/{id}/snapshots` | `ct.manage` | `plugin.lxc.snapshot.created` |
| `POST` | `/api/system-containers/{id}/snapshots/{snapId}/revert` | `ct.manage` | `plugin.lxc.snapshot.reverted` |
| `DELETE` | `/api/system-containers/{id}/snapshots/{snapId}` | `ct.delete` | `plugin.lxc.snapshot.deleted` |
| `POST` | `/api/system-containers/{id}/clone` | `ct.manage` | `plugin.lxc.ct.cloned` |
| `POST` | `/api/system-containers/{id}/template` | `ct.manage` | `plugin.lxc.ct.templated` |
| `POST` | `/api/system-containers/{id}/diagnose` | `ct.view` | — |

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
| `i18n.namespace.add:ct` | Merge the `ct` translation namespace into the host bundles. |
| `rbac.permission.register` | Reserve the permission keys it declares in `adds_permissions`. |
| `db.migrations.run` | Create and migrate its own tables at install time. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |
| `agent.rpc` | Send declared messages to a host's agent through the agent RPC bridge. Only takes effect with an `agent-rpc` capability. |
| `reporting.adapter.register` | Contribute report sections to the Reporting plugin. |
| `alerting.adapter.register` | Contribute alert conditions the host evaluates on a schedule. |
| `ai.invoke` | Call the configured AI provider through the host, without holding a key of its own. |

## Related

- [System Containers](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
