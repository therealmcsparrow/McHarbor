# Deployment

Production layouts for McHarbor, and the files in `deploy/` that implement them. The
[Installation](../installation/README.md) page gets one container running; this section is
for deciding what a production install looks like and wiring the pieces around it: a
database, a reverse proxy, a second node, a Kubernetes cluster.

Every file referenced here is an example to copy and adapt. Hostnames such as
`mcharbor.example.com` and `mcharbor-node-1` are placeholders.

**Where the files live.** `deploy/` holds examples an operator copies and adapts: Compose,
Helm, Kubernetes manifests, reverse proxies. `packaging/` holds the inputs to the build's native
artifacts: the `.deb`, `.rpm`, tarball and the systemd units they install. Neither folder
duplicates the other.

Before choosing a topology, check the [Compatibility matrix](./compatibility.md): it lists the
database backends and minimum versions, the remote-agent floors per feature, which HA modes
are supported, and the Docker and Kubernetes versions this release was tested against.

Before exposing any topology to the internet, read [Security posture](./security-posture/README.md):
which defaults McHarbor warns about at boot, and what to set instead.

## Topologies

| Topology | Database | Nodes | Use it when | Start from |
|---|---|---|---|---|
| Single node, Compose | SQLite in `DATA_DIR` | 1 | One Docker host, the common case | `deploy/compose/single-node/` |
| Single node, external database | PostgreSQL | 1 | You want the database on managed or separately backed-up storage, or you are preparing to add a second node | `deploy/compose/external-database/` |
| Active/passive, Compose | PostgreSQL, MySQL/MariaDB or SQL Server, shared | 2 or more | Failover in seconds when a node stops | `docker-compose.ha.yml` + `ha.env.example` in the repository root |
| Single node, Kubernetes | SQLite on a PersistentVolume | 1 | McHarbor runs in a cluster; the Docker hosts it manages are elsewhere | `deploy/helm/mcharbor` with `mode: single` |
| Active/passive, Kubernetes | PostgreSQL, MySQL/MariaDB or SQL Server, shared | 2 or more | Failover inside a cluster | `deploy/helm/mcharbor` with `mode: active-passive` |

Active/active (several nodes serving at once) is **readiness-gated**. A remote agent holds one
connection to one node, and a request that lands on another node is relayed to it, streams
included. See [active/active](../../settings/system/high-availability/active-active.md) for the
gates and what a proxy between nodes must allow.

## Files

| Path | What it is | Page |
|---|---|---|
| `deploy/compose/single-node/docker-compose.yml` | One hardened node: non-root, read-only root filesystem, no capabilities, pinned image, resource and log limits | [Compose](./compose.md) |
| `deploy/compose/single-node/.env.example` | Every variable that file reads | [Compose](./compose.md) |
| `deploy/compose/external-database/docker-compose.yml` | The same node plus PostgreSQL 17 | [Compose](./compose.md#external-database) |
| `deploy/compose/external-database/.env.example` | Every variable that file reads | [Compose](./compose.md#external-database) |
| `deploy/helm/mcharbor/` | Helm chart, both modes | [Kubernetes](./kubernetes.md) |
| `deploy/kubernetes/single-node.yaml` | Plain manifests rendered from the chart, single mode | [Kubernetes](./kubernetes.md#plain-manifests) |
| `deploy/kubernetes/active-passive.yaml` | Plain manifests rendered from the chart, active/passive | [Kubernetes](./kubernetes.md#plain-manifests) |
| `deploy/kubernetes/values-single-node.yaml` | The values those single-mode manifests were rendered from | [Kubernetes](./kubernetes.md#plain-manifests) |
| `deploy/kubernetes/values-active-passive.yaml` | The values those active/passive manifests were rendered from | [Kubernetes](./kubernetes.md#plain-manifests) |
| `deploy/kubernetes/generate.sh` | Re-renders both manifest files from the chart | [Kubernetes](./kubernetes.md#plain-manifests) |
| `deploy/proxy/caddy/Caddyfile` | Caddy in front of one or two nodes | [Reverse proxy](./reverse-proxy.md#caddy) |
| `deploy/proxy/traefik/docker-compose.labels.yml` | Traefik Docker-provider labels for the `mcharbor` service | [Reverse proxy](./reverse-proxy.md#traefik) |
| `deploy/proxy/traefik/dynamic.yml` | Traefik file-provider router, service and upstream transport | [Reverse proxy](./reverse-proxy.md#traefik) |
| `deploy/proxy/nginx/mcharbor.conf` | NGINX server block and upstream | [Reverse proxy](./reverse-proxy.md#nginx) |

## What every topology has in common

**Three secrets that must be kept.** `MCHARBOR_SECRET` signs sessions, `ENCRYPTION_KEY`
encrypts credentials at rest, and the backup key encrypts container and instance backups.
Left unset, McHarbor generates the first two into `DATA_DIR` on first start. That is fine for a
single node whose data volume you back up, fatal for a second node, which would generate
different ones. Generate them once and store them with your other production secrets:

```bash
openssl rand -base64 48   # MCHARBOR_SECRET
openssl rand -base64 32   # ENCRYPTION_KEY (must decode to exactly 32 bytes)
openssl rand -base64 32   # backup key file content
```

The backup key is read from the file named by `BACKUP_ENCRYPTION_KEY_FILE`
(default `/run/secrets/mcharbor_backup_key`). It accepts 32 raw bytes, 64 hex characters,
or base64 of 32 bytes.

**Two probes.**

| Route | Answers | Use it for |
|---|---|---|
| `GET /api/livez` | `200` while the process runs | Container health check, Kubernetes liveness and startup probes |
| `GET /api/readyz` | `200` when the node can take traffic, `503` on a standby, a draining node, or a node without its database | Load-balancer health check, Kubernetes readiness probe |

A standby is alive and not ready. Pointing a container health check at `/api/readyz` would
restart every standby in a loop; pointing a load balancer at `/api/livez` would send traffic
to standbys, which refuse every change.

**One port.** Everything (the UI, the API, WebSockets, server-sent events and the agent
connection) is served on `PORT` (`8705`). With `TLS_ENABLED=true`, the default, it is HTTPS
with a certificate issued by a root CA McHarbor generates into `DATA_DIR/tls`. Each node
generates its own, so in front of more than one node terminate TLS at the proxy with a real
certificate.

**The agent, not the socket, for remote Docker hosts.** McHarbor manages the Docker daemon
behind the socket it is given, and every other host through the remote agent, which dials
**out** to McHarbor over a WebSocket on `/api/agent/ws`. That is the only way to manage Docker
from a Kubernetes-hosted McHarbor or from more than one node. See [Agents](../../agents/README.md).

## Related

- [Compatibility matrix](./compatibility.md): supported databases, agent floors, HA modes, tested Docker and Kubernetes versions
- [Security posture](./security-posture/README.md): the boot-time posture warnings, the settings an internet-facing host needs, and plugin signing
- [Content-Security-Policy](./content-security-policy/README.md): the default policy, its two relaxations, report-only rollout and the empty-value escape hatch
- [Configuration](../configuration/README.md): every environment variable
- [High availability](../../settings/system/high-availability/README.md): how nodes pick a role, the controls, the probes
- [Installation](../installation/README.md): the development-style Compose file in the repository root
- [Upgrading](../upgrading/README.md)
