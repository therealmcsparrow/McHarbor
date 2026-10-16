# McHarbor Kubernetes

Cluster management as an installable add-on: workloads, networking, storage,
config, namespaces, nodes and events, with pod logs, deployment scaling and
rolling restarts.

## Why it is a plugin

This shipped as part of the host until 2.0.9. Being core cost every
installation — including the ones that have never touched Kubernetes —
`k8s.io/client-go` in the binary, five encrypted columns on the `environments`
table, ten permission keys in the catalog and a six-entry sidebar section.
None of that is free, and none of it is useful to a Docker-only install.

Moving it out means the host knows nothing about Kubernetes: no dependency, no
schema, no permissions, no navigation. The plugin brings its own.

## What it owns

| Surface | Where |
|---|---|
| REST API (`/api/kubernetes/*`) | `backend/handler.go`, declared in `plugin.yaml` |
| client-go reads and writes | `backend/service.go` |
| Cluster connections + credentials | `backend/clusters.go`, `migrations/0001_init.sql` |
| Client pool | `backend/client.go`, `backend/connection.go` |
| UI (7 sidebar entries, one bundle) | `frontend/src/` |
| Permissions | `adds_permissions` in `plugin.yaml` |
| Strings (12 locales) | `i18n/<lang>/kubernetes.json` |

## Clusters

A cluster is a row in `plugin_kubernetes_clusters`, not a McHarbor
environment. It is reached either by a kubeconfig blob or by a server URL with
a bearer token — the two shapes `connection.go` can turn into a `rest.Config`.

`kubeconfig`, `bearer_token` and `ca_cert` are encrypted at rest through the
host's AES-256-GCM service and are **write-only** on the API: the create and
update bodies accept them, and no response ever returns them. The listing
carries `hasKubeconfig` / `hasToken` booleans instead, so the edit form can say
"a credential is stored" without handing one back.

### Upgrading from a build where Kubernetes was core

Host migration `147_kubernetes_to_plugin.sql` copies existing Kubernetes
environments into `plugin_kubernetes_clusters`, keeping the same ids, and then
drops the `environments` columns. It carries the *ciphertext*: the plugin's
encryption handle is the host's own service, so nothing is decrypted during the
move.

The hand-off has to happen in the host rather than here — host migrations run
at boot and plugin migrations run at install, so by the time
`migrations/0001_init.sql` runs the source columns are already gone. That file
is written `CREATE TABLE IF NOT EXISTS` so it is a no-op on an upgraded install
and the creating statement on a fresh one.

## Permissions

| Key | Gates |
|---|---|
| `kubernetes.view` | Every read: the summary behind all tabs, resource detail, pod logs, and the cluster picker's list |
| `kubernetes.manage` | Scaling a deployment, rolling restart |
| `kubernetes.delete` | Deleting pods and deployments |
| `kubernetes.clusters.manage` | Adding, editing and removing cluster connections |

These replace the host's `pods.*`, `deployments.*`, `k8s_services.*` and
`namespaces.view`. Read access is one key rather than three because all of
those resources arrive in a single `/summary` response — a role with
`pods.view` but not `k8s_services.view` was still handed the services, so the
old split described a boundary the code did not enforce.

Migration 147 removes the old keys and the grants that referenced them.
Administrators re-assign Kubernetes access after upgrading.

## Workflow nodes

The plugin contributes five nodes to the workflow editor, listed in the
**Action** group of the palette once the plugin is loaded. They call the
plugin's own service, so a workflow can do nothing a role could not do
from the pages. Every node has an `output` and an `error` port; an
operational failure (cluster unreachable, object missing, RBAC refusal)
answers on `error` with a short reason in `msg.payload.error` rather
than aborting the run.

| Node | Palette gate | Config | Result in `msg.payload` |
|---|---|---|---|
| Kubernetes: Scale Deployment | `kubernetes.manage` | cluster, namespace, deployment, replicas | `{ cluster, namespace, name, replicas }` |
| Kubernetes: Deployment Action | `kubernetes.manage` | cluster, namespace, deployment, action (`restart` / `delete`) | `{ cluster, namespace, name, action }` |
| Kubernetes: Delete Pod | `kubernetes.delete` | cluster, namespace, pod | `{ cluster, namespace, name, deleted }` |
| Kubernetes: Pod Logs | `kubernetes.logs` | cluster, namespace, pod, container, tail lines, previous | `{ logs, lines, truncated, ... }` — capped at 1 MiB |
| Kubernetes: Cluster Summary | `kubernetes.view` | cluster, namespace (empty = all), include full object lists | counts and version, or the full summary when detail is on |

**Trigger.** *Kubernetes: Event* (palette gate `kubernetes.view`) starts a
workflow when a cluster records an event, Warnings by default. Filter by
cluster, namespace, event type, reason (`BackOff`, `Unhealthy`,
`FailedScheduling`, ...) and object kind; `msg.payload` carries
`{ cluster, namespace, type, reason, kind, name, object, message, count,
source, time }`. The plugin polls every active cluster every 30 seconds
while at least one active workflow places the trigger, fires each event it
has not seen before, and never replays history from before it started.

`cluster` is the id of a saved cluster; empty means the default cluster,
exactly as the pages resolve it. Each mutating node writes one audit
event (`plugin.kubernetes.node.*`) carrying the workflow id and, for
manual runs, the account that pressed Run.

## Building

```bash
# Backend (.so) — built inside the image; locally, vet is the useful check:
cd src/plugins/Plugins/kubernetes && go vet ./backend

# Frontend (UMD bundle)
cd frontend && npm install --legacy-peer-deps && npm run build
```

The `.so` is compiled by `docker/Dockerfile` inside the `go.work` workspace, so
the plugin and the host resolve one module graph and their shared package ABI
hashes match — without that, `plugin.Open` rejects the result.

## Notes

- The frontend bundle carries its own React, Radix and react-router copies. It
  therefore has **no Router context**: `<Link>` and `useNavigate()` throw and
  blank the plugin root. Navigation goes through `nav-context.tsx`, which wraps
  the `PluginNavContext` the host passes on mount and on every in-plugin
  navigation.
- The cluster selection lives in `cluster-store.ts` (`useSyncExternalStore` +
  `localStorage`), not in the host's environment store — the topbar switcher is
  Docker-only and always was.
