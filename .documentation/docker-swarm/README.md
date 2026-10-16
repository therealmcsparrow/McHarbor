# Docker Swarm

Manages Docker Swarm from McHarbor: services, tasks, nodes, secrets, configs, stacks and overlay networks, with cluster init, join, leave, token rotation, unlock-key handling, scaling, rolling restarts and rollback.

- **Slug** `addon-swarm`, **version** 2026.09.01
- **Host range** `>=2026.09.01`
- **Where it appears** its own section in the sidebar, and its own pages under `/plugins/<slug>`
- **Gated by** the permission keys it declares (see below)

## How it works

- It stores **nothing**. Swarm state lives on the Docker daemon an environment is already connected to, and the plugin discovers it with a capability probe, which is why it ships no migrations and needs no credentials of its own.
- `GET /api/swarm/capability` is the one route with no permission key. It returns a boolean and the local node's role, leaks nothing about workloads, and the UI needs it before it can decide between a cluster view and the "initialise" empty state. It still requires a session.
- Reading a join token or the unlock key takes `swarm.manage`, not `swarm.view`, even though both are GETs: they are credential reads.
- A service's environment is masked for readers. `GET /api/swarm/services/{id}` (the detail page, `mcharbor swarm service get` and the runbook diagnostics) returns `••••••••` for a credential-looking value (a name containing `key`, `secret`, `token`, `passw`, `dsn` …, or a URL with a password). **Reveal values** on the detail page and `mcharbor swarm service env` read them through `GET /api/swarm/services/{id}/env`, which takes `swarm_services.manage` and is audited as `plugin.swarm.service.env_revealed`. The reveal writes its own audit row at warning severity naming the service id and name, the environment, and any value that stayed withheld. On a service that runs McHarbor (recognised by core McHarbor's detector, including a stack-deployed `<stack>_mcharbor` or `mcharbor-node-N` service), `MCHARBOR_SECRET`, `ENCRYPTION_KEY`, `MCHARBOR_DB_DSN` and `MCHARBOR_BACKUP_KEY` stay masked even then, and on any service a value carrying the keys McHarbor itself runs with stays masked. Updating a service from a form that still shows `••••••••` keeps the current value; keeping a value nobody may reveal while changing the image, command, arguments or labels is refused.
- Scaling has its own key, `swarm_services.scale`, so an on-call role can resize a service without being able to update or delete it.
- `swarm.manage` is global scope while the rest are environment-scoped: cluster lifecycle (init, join, leave, unlock) is not a per-environment decision.

## What it contributes

| | |
|---|---|
| Routes | 46 |
| Sidebar entries | 4 |
| Dashboard widgets | `swarm-overview`, `swarm-services`, `swarm-nodes` |
| Workflow nodes | `swarm-service-scale`, `swarm-service-action`, `swarm-stack-deploy`, `swarm-stack-remove`, `swarm-node-availability`, `swarm-overview`, `swarm-event`. The service and node fields are pick lists of the environment services and nodes, and a typed name or id is still accepted |
| Migrations | none |
| Backend | 24 Go files, ~8,879 lines |

## Permissions it adds

| Key | Scope | What it allows |
|---|---|---|
| `swarm.view` | env | View Swarm |
| `swarm.manage` | global | Manage the Swarm cluster |
| `swarm_services.view` | env | View Swarm services |
| `swarm_services.manage` | env | Manage Swarm services |
| `swarm_services.scale` | env | Scale Swarm services |
| `swarm_services.delete` | env | Delete Swarm services |
| `swarm_tasks.view` | env | View Swarm tasks |
| `swarm_nodes.view` | env | View Swarm nodes |
| `swarm_nodes.manage` | env | Manage Swarm nodes |
| `swarm_nodes.delete` | env | Remove Swarm nodes |
| `swarm_secrets.view` | env | View Swarm secrets |
| `swarm_secrets.manage` | env | Manage Swarm secrets |
| `swarm_secrets.delete` | env | Delete Swarm secrets |
| `swarm_configs.view` | env | View Swarm configs |
| `swarm_configs.manage` | env | Manage Swarm configs |
| `swarm_configs.delete` | env | Delete Swarm configs |
| `swarm_stacks.view` | env | View Swarm stacks |
| `swarm_stacks.manage` | stack | Deploy Swarm stacks |
| `swarm_stacks.delete` | env | Remove Swarm stacks |
| `swarm_networks.view` | env | View Swarm overlay networks |

## In this section

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../development/add-ons/docker-swarm.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Add-ons](../plugins/addons.md): what the six have in common, and what the plugin runtime does with them
- [Plugin catalog](../plugins/catalog.md): every shipped plugin
