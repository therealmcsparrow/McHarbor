# Kubernetes: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A role holding at least this plugin's `view` key, or its sidebar section stays hidden.

## Where it appears

| Path | Label key | Permission |
|---|---|---|
| `/plugins/addon-kubernetes` | `nav.overview` | `kubernetes.view` |
| `/plugins/addon-kubernetes/pods` | `nav.workloads` | `kubernetes.view` |
| `/plugins/addon-kubernetes/services` | `nav.network` | `kubernetes.view` |
| `/plugins/addon-kubernetes/storage` | `nav.storage` | `kubernetes.view` |
| `/plugins/addon-kubernetes/config` | `nav.config` | `kubernetes.view` |
| `/plugins/addon-kubernetes/nodes` | `nav.cluster` | `kubernetes.view` |
| `/plugins/addon-kubernetes/clusters` | `nav.clusters` | `kubernetes.clusters.manage` |

Sidebar entries are **hidden** when the permission is missing, unlike buttons, which
render disabled with a tooltip. A dead-end page is not informative.

## Routes

Every route is mounted under the plugin router and carries the permission declared in
the manifest. The host mounts only what the manifest declares.

| Method | Path | Permission | Audit action |
|---|---|---|---|
| `GET` | `/api/kubernetes/clusters` | `kubernetes.view` | — |
| `POST` | `/api/kubernetes/clusters` | `kubernetes.clusters.manage` | `plugin.kubernetes.cluster.created` |
| `GET` | `/api/kubernetes/clusters/{id}` | `kubernetes.clusters.manage` | — |
| `PUT` | `/api/kubernetes/clusters/{id}` | `kubernetes.clusters.manage` | `plugin.kubernetes.cluster.updated` |
| `DELETE` | `/api/kubernetes/clusters/{id}` | `kubernetes.clusters.manage` | `plugin.kubernetes.cluster.deleted` |
| `GET` | `/api/kubernetes/summary` | `kubernetes.view` | — |
| `GET` | `/api/kubernetes/pods/{namespace}/{name}` | `kubernetes.view` | — |
| `GET` | `/api/kubernetes/pods/{namespace}/{name}/logs` | `kubernetes.logs` | — |
| `DELETE` | `/api/kubernetes/pods/{namespace}/{name}` | `kubernetes.delete` | `plugin.kubernetes.pod.deleted` |
| `GET` | `/api/kubernetes/deployments/{namespace}/{name}` | `kubernetes.view` | — |
| `POST` | `/api/kubernetes/deployments/{namespace}/{name}/scale` | `kubernetes.manage` | `plugin.kubernetes.deployment.scaled` |
| `POST` | `/api/kubernetes/deployments/{namespace}/{name}/restart` | `kubernetes.manage` | `plugin.kubernetes.deployment.restarted` |
| `DELETE` | `/api/kubernetes/deployments/{namespace}/{name}` | `kubernetes.delete` | `plugin.kubernetes.deployment.deleted` |

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
| `i18n.namespace.add:kubernetes` | Merge the `kubernetes` translation namespace into the host bundles. |
| `rbac.permission.register` | Reserve the permission keys it declares in `adds_permissions`. |
| `db.migrations.run` | Create and migrate its own tables at install time. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |
| `reporting.adapter.register` | Contribute report sections to the Reporting plugin. |
| `alerting.adapter.register` | Contribute alert conditions the host evaluates on a schedule. |

## Related

- [Kubernetes](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
