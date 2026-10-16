# McHarbor Docker Swarm

Docker Swarm — services, tasks, nodes, secrets, configs, swarm stacks and
overlay networks — as an installable McHarbor add-on.

Install it from **App Store → Plugins → Docker Swarm**. A **Swarm** section
appears in the sidebar with four groups (Overview, Workloads, Cluster, Config)
covering ten pages.

## What it does

| Area | What you get |
|---|---|
| Cluster | Initialise, join and leave a Swarm; read and rotate the worker and manager join tokens; read, rotate and use the autolock unlock key |
| Services | List, inspect, create, update, scale, restart, roll back, remove; read aggregated service logs |
| Tasks | List and inspect tasks; read per-task logs |
| Nodes | List, inspect, label, drain/pause/activate, promote/demote, remove |
| Secrets | List and inspect metadata, create, relabel, remove |
| Configs | List, read contents, create, relabel, remove |
| Stacks | List, inspect, deploy a Compose file to the Swarm, remove |
| Networks | List the cluster's overlay networks |

## What it stores

Nothing.

Swarm is not an environment type and not a set of credentials. A Docker
environment is Swarm-capable when its daemon reports an active local node state,
and that can flip at runtime when an operator runs init, join or leave from
inside McHarbor. The plugin asks the daemon the host is already connected to —
`backend/capability.go` — caches the answer briefly per environment, and
invalidates it on every lifecycle mutation.

That is why this add-on ships no migrations and no config schema, and why
uninstalling it takes nothing with it.

## Permissions

Twenty keys, all declared in `plugin.yaml` under `adds_permissions`:

```
swarm.view              swarm.manage
swarm_services.view     swarm_services.manage    swarm_services.scale    swarm_services.delete
swarm_tasks.view
swarm_nodes.view        swarm_nodes.manage       swarm_nodes.delete
swarm_secrets.view      swarm_secrets.manage     swarm_secrets.delete
swarm_configs.view      swarm_configs.manage     swarm_configs.delete
swarm_stacks.view       swarm_stacks.manage      swarm_stacks.delete
swarm_networks.view
```

Two of them are worth knowing about:

- **`swarm_services.scale` is separate from `.manage`.** Scaling is the routine
  operation an on-call engineer performs, and it cannot change the image, the
  mounts, or the secrets a service reads. Folding it into `manage` would mean
  handing out image replacement in order to grant a replica bump.
- **`swarm.manage` covers the join tokens and the unlock key.** Those are
  credentials that let a stranger's host into the cluster, so they sit with
  init/join/leave rather than with `view`, even though reading one is a `GET`.

### Upgrading from a build where Swarm was in the host

These are the same twenty key strings the host used, so **existing grants keep
working**. Host migration `094_swarm_to_plugin.sql` deletes the rows from
`permission_catalog` — a plugin cannot shadow a host key, so they have to leave
before these can land — and deliberately leaves `roles.permissions`,
`role_presets.permissions` and `user_permissions` untouched.

The consequence on an install that never adds this plugin: those grants name
keys no catalog defines. That is inert, and installing the add-on later restores
every one of them exactly as it was.

## Workflow nodes

The plugin contributes six nodes to the workflow editor. They appear in
the **Action** group of the palette once the plugin is loaded, and run
through the plugin's own service — the same code the pages call — so a
workflow can do nothing a role could not do by hand. Every node has an
`output` and an `error` port; an operational failure (service missing,
daemon unreachable, not a manager) goes down `error` with a short reason
in `msg.payload.error` rather than aborting the run.

| Node | Palette gate | Config | Result in `msg.payload` |
|---|---|---|---|
| Swarm: Scale Service | `swarm_services.scale` | environment, service (name or id), replicas | `{ service, replicas }` |
| Swarm: Service Action | `swarm_services.manage` | environment, service, action (`restart` / `rollback` / `remove`) | `{ service, action }` |
| Swarm: Deploy Stack | `swarm_stacks.manage` | environment, name, compose source (inline YAML or `msg.payload`), prune | the deploy result (`created`, `updated`, `removed`, `networks`, `rejected`); a deploy with rejected services answers on `error` with the same payload |
| Swarm: Remove Stack | `swarm_stacks.delete` | environment, name | `{ stack, removed }` |
| Swarm: Node Availability | `swarm_nodes.manage` | environment, node id, availability (`active` / `pause` / `drain`) | `{ node, availability }` |
| Swarm: Cluster Overview | `swarm.view` | environment | the cluster overview (node, service and task counts) |

**Trigger.** *Swarm: Event* (palette gate `swarm.view`) starts a workflow
when a service, node, secret or config changes on any Swarm manager the host
knows. Filter by environment, object type, action (`create`, `update`,
`remove`, ...) and object name; `msg.payload` carries `{ environmentId,
type, action, id, name, attributes, time }`. The plugin subscribes to the
Docker event stream of every active Docker environment that is a Swarm
manager and re-discovers environments every 30 seconds.

`environment` is a Docker environment select; empty means the host's
default environment. Each mutating node writes one audit event
(`plugin.swarm.node.*`) carrying the workflow id and, for manual runs,
the account that pressed Run.

## Building

```bash
# backend (from the repo root, so the go workspace is in scope)
go build -trimpath -buildmode=plugin -o addon-swarm.so ./src/plugins/Plugins/addon-swarm/backend

# frontend UMD bundle
cd src/plugins/Plugins/addon-swarm/frontend
npm install --legacy-peer-deps
npm run build          # emits ../frontend/addon-swarm.umd.cjs
```

`--legacy-peer-deps` is required: `@vitejs/plugin-react` has not shipped a peer
range covering the Vite version this repo pins, and without the flag npm fails
to resolve rather than warning.

The dependency ranges in `frontend/package.json` are **the host's own**, and
must stay that way. The bundle resolves its own copy of React, React Query and
React Table (see the aliases in `vite-plugin.config.ts`) but compiles host
source reached through `@resources` against them — the shared `TablePage` is
written to React Table v9, and an `^8` pin here fails the build with a wall of
`MISSING_EXPORT` rather than a version complaint.

## Notes for maintainers

Three things in this plugin exist only because a UMD bundle carries its own copy
of every module it imports, and each replaced something that worked fine when
Swarm was part of the host:

- **`src/nav-context.tsx`** replaces react-router. A second react-router copy has
  no Router context, so `<Link>` and `useNavigate()` throw and blank the plugin
  root. The host hands the plugin its location and a `navigate` bound to the
  host's own router; everything here is a shell over that.
- **`src/host-env.ts`** replaces `@resources/stores/environment`. Every request
  this plugin makes is scoped by `?env=`, and a second copy of that Zustand
  store would hydrate from the same `localStorage` key — correct on first paint,
  then silently wrong from the moment the user switches environments. The host
  publishes the real store on `window.McharborHost.environment`.
- **`src/components/PluginBreadcrumb.tsx`** replaces `HeaderBreadcrumb`, for
  both of the above reasons at once: the shared `Breadcrumb` builds its crumbs
  with react-router's `Link`, and its portal target is claimed through another
  module-level store.

The four `menu:` entries in `plugin.yaml` and the four groups in `src/nav.ts`
are the one pair in this plugin that has to be kept in step by hand — the
sidebar is built from the manifest before any of the bundle has loaded.
