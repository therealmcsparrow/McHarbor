# Docker Swarm: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A role holding at least this plugin's `view` key, or its sidebar section stays hidden.

## Where it appears

| Path | Label key | Permission |
|---|---|---|
| `/plugins/addon-swarm` | `nav.swarm.overview` | `swarm.view` |
| `/plugins/addon-swarm/services` | `nav.swarm.workloads` | `swarm_services.view` |
| `/plugins/addon-swarm/nodes` | `nav.swarm.cluster` | `swarm_nodes.view` |
| `/plugins/addon-swarm/secrets` | `nav.swarm.config` | `swarm_secrets.view` |

Sidebar entries are **hidden** when the permission is missing, unlike buttons, which
render disabled with a tooltip. A dead-end page is not informative.

## Routes

Every route is mounted under the plugin router and carries the permission declared in
the manifest. The host mounts only what the manifest declares.

| Method | Path | Permission | Audit action |
|---|---|---|---|
| `GET` | `/api/swarm/capability` | _session only_ | — |
| `GET` | `/api/swarm` | `swarm.view` | — |
| `GET` | `/api/swarm/overview` | `swarm.view` | — |
| `PUT` | `/api/swarm` | `swarm.manage` | `plugin.swarm.cluster.updated` |
| `POST` | `/api/swarm/init` | `swarm.manage` | `plugin.swarm.cluster.initialised` |
| `POST` | `/api/swarm/join` | `swarm.manage` | `plugin.swarm.cluster.joined` |
| `POST` | `/api/swarm/leave` | `swarm.manage` | `plugin.swarm.cluster.left` |
| `GET` | `/api/swarm/join-tokens` | `swarm.manage` | — |
| `POST` | `/api/swarm/join-tokens/{kind}/rotate` | `swarm.manage` | `plugin.swarm.join_token.rotated` |
| `GET` | `/api/swarm/unlock-key` | `swarm.manage` | — |
| `POST` | `/api/swarm/unlock-key/rotate` | `swarm.manage` | `plugin.swarm.unlock_key.rotated` |
| `POST` | `/api/swarm/unlock` | `swarm.manage` | `plugin.swarm.cluster.unlocked` |
| `GET` | `/api/swarm/services` | `swarm_services.view` | — |
| `POST` | `/api/swarm/services` | `swarm_services.manage` | `plugin.swarm.service.created` |
| `GET` | `/api/swarm/services/{id}` | `swarm_services.view` | — |
| `GET` | `/api/swarm/services/{id}/logs` | `swarm_services.view` | — |
| `PUT` | `/api/swarm/services/{id}` | `swarm_services.manage` | `plugin.swarm.service.updated` |
| `POST` | `/api/swarm/services/{id}/restart` | `swarm_services.manage` | `plugin.swarm.service.restarted` |
| `POST` | `/api/swarm/services/{id}/rollback` | `swarm_services.manage` | `plugin.swarm.service.rolled_back` |
| `POST` | `/api/swarm/services/{id}/scale` | `swarm_services.scale` | `plugin.swarm.service.scaled` |
| `DELETE` | `/api/swarm/services/{id}` | `swarm_services.delete` | `plugin.swarm.service.removed` |
| `GET` | `/api/swarm/tasks` | `swarm_tasks.view` | — |
| `GET` | `/api/swarm/tasks/{id}` | `swarm_tasks.view` | — |
| `GET` | `/api/swarm/tasks/{id}/logs` | `swarm_tasks.view` | — |
| `GET` | `/api/swarm/nodes` | `swarm_nodes.view` | — |
| `GET` | `/api/swarm/nodes/{id}` | `swarm_nodes.view` | — |
| `PUT` | `/api/swarm/nodes/{id}` | `swarm_nodes.manage` | `plugin.swarm.node.updated` |
| `POST` | `/api/swarm/nodes/{id}/availability` | `swarm_nodes.manage` | `plugin.swarm.node.availability_changed` |
| `POST` | `/api/swarm/nodes/{id}/role` | `swarm_nodes.manage` | `plugin.swarm.node.role_changed` |
| `DELETE` | `/api/swarm/nodes/{id}` | `swarm_nodes.delete` | `plugin.swarm.node.removed` |
| `GET` | `/api/swarm/secrets` | `swarm_secrets.view` | — |
| `POST` | `/api/swarm/secrets` | `swarm_secrets.manage` | `plugin.swarm.secret.created` |
| `GET` | `/api/swarm/secrets/{id}` | `swarm_secrets.view` | — |
| `PUT` | `/api/swarm/secrets/{id}` | `swarm_secrets.manage` | `plugin.swarm.secret.updated` |
| `DELETE` | `/api/swarm/secrets/{id}` | `swarm_secrets.delete` | `plugin.swarm.secret.removed` |
| `GET` | `/api/swarm/configs` | `swarm_configs.view` | — |
| `POST` | `/api/swarm/configs` | `swarm_configs.manage` | `plugin.swarm.config.created` |
| `GET` | `/api/swarm/configs/{id}` | `swarm_configs.view` | — |
| `PUT` | `/api/swarm/configs/{id}` | `swarm_configs.manage` | `plugin.swarm.config.updated` |
| `DELETE` | `/api/swarm/configs/{id}` | `swarm_configs.delete` | `plugin.swarm.config.removed` |
| `GET` | `/api/swarm/stacks` | `swarm_stacks.view` | — |
| `POST` | `/api/swarm/stacks` | `swarm_stacks.manage` | `plugin.swarm.stack.deployed` |
| `GET` | `/api/swarm/stacks/{name}` | `swarm_stacks.view` | — |
| `DELETE` | `/api/swarm/stacks/{name}` | `swarm_stacks.delete` | `plugin.swarm.stack.removed` |
| `GET` | `/api/swarm/networks` | `swarm_networks.view` | — |

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
| `docker.socket.read` | Read from the Docker daemon an environment is already connected to. |
| `menu.sidebar.add` | Add an entry to the sidebar. |
| `i18n.namespace.add:swarm` | Merge the `swarm` translation namespace into the host bundles. |
| `rbac.permission.register` | Reserve the permission keys it declares in `adds_permissions`. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |
| `reporting.adapter.register` | Contribute report sections to the Reporting plugin. |

## Related

- [Docker Swarm](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
