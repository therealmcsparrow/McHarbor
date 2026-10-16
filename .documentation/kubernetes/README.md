# Kubernetes

Manages Kubernetes clusters from McHarbor: workloads, services and networking, storage, config, namespaces, nodes and events, with pod logs, deployment scaling and rolling restarts. Cluster connections belong to the plugin, and their kubeconfig and bearer-token credentials are encrypted at rest with the host's own key.

- **Slug** `addon-kubernetes`, **version** 2026.09.01
- **Host range** `>=2026.09.01`
- **Where it appears** its own section in the sidebar, and its own pages under `/plugins/<slug>`
- **Gated by** the permission keys it declares (see below)

## How it works

- Clusters are the plugin's own records, in `plugin_kubernetes_*` tables created by its migration. They are not McHarbor environments; a Docker environment and a Kubernetes cluster are separate connections.
- Credentials are write-only on the API. The cluster list returns names and ids and never a kubeconfig, a bearer token or a CA certificate.
- `kubernetes.logs` is separate from `kubernetes.view` on purpose: a pod's stdout routinely carries connection strings and tokens, so reading logs is a privileged read rather than part of the general view.
- No view returns container environment values, Secret data or ConfigMap data. Secrets and config maps are listed by name, namespace, type and age; pod and deployment detail carry status, images and conditions. There is no YAML, describe or apply view, so there is nothing to mask and no reveal route. A regression test fails the build if a read surface starts returning one of those values.
- The seven sidebar entries render as one **Kubernetes** section, grouped by the job a resource does for an application rather than by API group, the same split the Kubernetes Dashboard, Lens and Rancher settle on.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `SupportsFor` | Yes |  |

## What it contributes

| | |
|---|---|
| Routes | 13 |
| Sidebar entries | 7 |
| Dashboard widgets | `cluster-health`, `workloads`, `events` |
| Workflow nodes | `kubernetes-scale-deployment`, `kubernetes-deployment-action`, `kubernetes-delete-pod`, `kubernetes-pod-logs`, `kubernetes-summary`, `kubernetes-event`. The deployment and pod fields are pick lists, and a typed name or `namespace/name` is still accepted |
| Migrations | `0001_init.sql` |
| Backend | 14 Go files, ~5,745 lines |

## Permissions it adds

| Key | Scope | What it allows |
|---|---|---|
| `kubernetes.view` | env | View Kubernetes |
| `kubernetes.logs` | env | Read Kubernetes pod logs |
| `kubernetes.manage` | env | Manage Kubernetes workloads |
| `kubernetes.delete` | env | Delete Kubernetes workloads |
| `kubernetes.clusters.manage` | global | Manage Kubernetes clusters |

## In this section

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../development/add-ons/kubernetes.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Add-ons](../plugins/addons.md): what the six have in common, and what the plugin runtime does with them
- [Plugin catalog](../plugins/catalog.md): every shipped plugin
