# Kubernetes

`deploy/helm/mcharbor` installs McHarbor into a Kubernetes cluster in one of two modes:

| `mode` | Pods | Database | Traffic |
|---|---|---|---|
| `single` (default) | 1 | SQLite on the pod's PersistentVolume, or an external database when `externalDatabase.existingSecret` is set | The one pod |
| `active-passive` | `replicaCount`, at least 2 | PostgreSQL, MySQL or MariaDB, shared by every pod | The active pod only |

Requires Kubernetes 1.27 or later (`unhealthyPodEvictionPolicy` on the PodDisruptionBudget,
`persistentVolumeClaimRetentionPolicy` on the StatefulSet) and Helm 3.

McHarbor in a cluster manages Docker **elsewhere**, through the remote agent. See
[Docker hosts](#docker-hosts) before planning anything else.

**Docker Compose remains the supported default.** It is what McHarbor is developed and
tested against on every release, and it is the path the [Installation](../installation/README.md)
and [Compose](./compose.md) pages describe. Kubernetes is supported and exercised, but a
problem reproduced on Compose is the one that gets diagnosed fastest. [Compatibility
matrix](./compatibility.md) lists the versions each path is tested against.

## Install

### Single node

```bash
kubectl create namespace mcharbor

kubectl -n mcharbor create secret generic mcharbor-secrets \
  --from-literal=mcharbor-secret="$(openssl rand -base64 48)" \
  --from-literal=encryption-key="$(openssl rand -base64 32)" \
  --from-literal=backup-key="$(openssl rand -base64 32)"

helm install mcharbor deploy/helm/mcharbor -n mcharbor \
  --set secrets.existingSecret=mcharbor-secrets \
  --set config.allowedOrigins=https://mcharbor.example.com
```

The Secret is optional in single mode. Without it McHarbor generates the session secret and
the encryption key into its data volume on first start, and container backups stay
unavailable until a backup key is mounted. Creating it up front is what lets you restore the
data volume onto a new pod, or move to active/passive later, without re-entering every
credential.

### Active/passive

Create the shared Secret as above, then a Secret holding the PostgreSQL DSN:

```bash
kubectl -n mcharbor create secret generic mcharbor-database \
  --from-literal=dsn='postgres://mcharbor:<url-encoded password>@postgres.db.svc:5432/mcharbor?sslmode=require'

helm install mcharbor deploy/helm/mcharbor -n mcharbor \
  -f deploy/kubernetes/values-active-passive.yaml
```

The database is not part of the chart. Use a managed PostgreSQL service or an operator such as
CloudNativePG; a PostgreSQL that is itself a single pod gives you a highly available McHarbor
on a database that is not.

## Why a StatefulSet

The chart runs McHarbor as a StatefulSet in both modes.

- **Stable node identity.** `MCHARBOR_NODE_ID` is the pod name (`mcharbor-0`, `mcharbor-1`, …),
  which a StatefulSet keeps across restarts and rescheduling. The HA topology, the audit log
  and the failover controls name nodes by it, and a Deployment's random pod suffix would make
  each restart look like a new node.
- **Stable addresses.** Each pod is reachable at
  `<pod>.<release>-nodes.<namespace>.svc.<clusterDomain>` through the headless Service, which is
  what `MCHARBOR_ADVERTISE_URL` is set to.
- **One volume per pod.** `volumeClaimTemplates` gives every pod its own `data` claim. In single
  mode that claim holds the SQLite database; in active/passive it holds the pod's certificate,
  its copy of installed plugin packages and other local state, while the database is shared.
- **No overlap in single mode.** With one replica a StatefulSet rolling update deletes the old
  pod before it creates the new one, which is what a Deployment's `Recreate` strategy does. A
  ReadWriteOnce volume is never wanted by two pods, and SQLite never has two writers.

Claims are kept when the StatefulSet is deleted or scaled down
(`persistentVolumeClaimRetentionPolicy: Retain`). `helm uninstall` leaves them, and the
chart-created Secret, in place; delete them yourself when the data is no longer wanted.

## How active/passive behaves in Kubernetes

Exactly one pod holds the PostgreSQL active lock and is **Ready**. Every other pod is a
standby: running, alive, and **NotReady** on purpose, because `/api/readyz` answers `503` on a
standby. `kubectl get pods` therefore shows `0/1` for standbys; that is the intended state.

| Object | Effect |
|---|---|
| Service `<release>` | `publishNotReadyAddresses: false`, so its endpoints are the active pod only. The Ingress and anything else in the cluster reach McHarbor through it |
| Service `<release>-nodes` | Headless, `publishNotReadyAddresses: true`, so standbys have DNS names too. Used for node addressing, never for traffic |
| PodDisruptionBudget | `maxUnavailable: 1` with `unhealthyPodEvictionPolicy: AlwaysAllow`. Standbys can always be evicted. The active pod is the only Ready one, so the budget allows no voluntary disruption of it: `kubectl drain` on its node waits until you move the active role |
| Anti-affinity | `ha.antiAffinity: soft` prefers one pod per node; `hard` requires it |
| `podManagementPolicy: Parallel` | All pods start together. With the default ordered policy the second pod would wait for the first to be Ready, and a third would wait for a standby that never becomes Ready |

When the active pod stops cleanly it releases the lock on the way down and a standby takes
over on its next five-second tick. When it dies without stopping, the database ends its
session and the lock goes with it.

### Draining a node

Move the active role first, then drain:

1. **Enterprise → High availability**, **Make active** on a standby, or **Drain** on the
   active node, which hands over first.
2. Wait until the old active pod is NotReady and another pod is Ready.
3. `kubectl drain` the Kubernetes node.

### Upgrading

`ha.updateStrategy` defaults to `OnDelete`. A StatefulSet `RollingUpdate` waits for each
replaced pod to become Ready before it moves on, and a standby never does, so an automatic
rollout would stop after the first pod. With `OnDelete`, `helm upgrade` changes the template
and you replace pods in the order that keeps one active:

```bash
helm upgrade mcharbor deploy/helm/mcharbor -n mcharbor -f values.yaml
kubectl -n mcharbor delete pod mcharbor-1        # a standby; wait until it is Running
# Enterprise -> High availability: Make active on mcharbor-1
kubectl -n mcharbor delete pod mcharbor-0        # the old active, now a standby
```

Every pod runs pending database migrations when it starts, standby or not, so the first
upgraded standby migrates the database the older active pod is still using. Check
[Upgrading](../upgrading/README.md) and the release notes for a release whose migrations an
older node cannot run against; for those, stop every pod and start the upgraded ones instead.

Single mode uses `RollingUpdate`; `helm upgrade` replaces the pod by itself.

## Values

| Value | Default | What it does |
|---|---|---|
| `mode` | `single` | `single` or `active-passive`. Anything else fails the render |
| `replicaCount` | `2` | Pods in active/passive. Ignored in single mode, which is always 1 |
| `image.repository` | `ghcr.io/therealmcsparrow/mcharbor` | Image |
| `image.tag` | chart `appVersion` | Pin it; never deploy `latest` |
| `image.pullPolicy` | `IfNotPresent` | |
| `imagePullSecrets` | `[]` | |
| `clusterDomain` | `cluster.local` | Used to build `MCHARBOR_ADVERTISE_URL` and the certificate names |
| `serviceAccount.create` | `true` | |
| `serviceAccount.automountToken` | `false` | The chart creates no RBAC. McHarbor reaches clusters through the connections configured in its interface, not through its own service account |
| `config.logLevel` | `info` | `LOG_LEVEL` |
| `config.logJSON` | `false` | `LOG_JSON` |
| `config.tz` | `UTC` | `TZ` |
| `config.allowedOrigins` | empty | `ALLOWED_ORIGINS`. Empty with the Ingress enabled becomes `https://<host>` for every Ingress host; empty without it leaves only the localhost defaults, and sign-in from anywhere else fails |
| `config.trustedProxyCIDRs` | empty | `MCHARBOR_TRUSTED_PROXY_CIDRS`. Set it to the pod CIDR of your ingress controller; see [Reverse proxy](./reverse-proxy.md#forwarding-headers-and-trusted-proxies) |
| `config.selfSignedSANExtra` | empty | Appended to the generated certificate's names, after the Service and pod names the chart adds itself |
| `config.forceAgentTLS` | `true` | `FORCE_AGENT_TLS` |
| `config.hstsMaxAge` | `0` | `HSTS_MAX_AGE` |
| `tls.enabled` | `true` | `TLS_ENABLED`, and the scheme of every probe, the Service port name and the advertise URL |
| `tls.minVersion` | `1.3` | `TLS_MIN_VERSION` |
| `secrets.existingSecret` | empty | A Secret holding the three keys below. Required in active/passive unless all three values are given |
| `secrets.mcharborSecret` / `secrets.encryptionKey` / `secrets.backupKey` | empty | Values for a Secret the chart creates when `existingSecret` is empty. `encryptionKey` must be base64 of 32 bytes, or the render fails. They end up in your values file and the Helm release; prefer `existingSecret` |
| `secrets.keys.mcharborSecret` | `mcharbor-secret` | Key name in the Secret for `MCHARBOR_SECRET` |
| `secrets.keys.encryptionKey` | `encryption-key` | Key name for `ENCRYPTION_KEY` |
| `secrets.keys.backupKey` | `backup-key` | Key name for the backup key, mounted as `/run/secrets/mcharbor_backup_key` |
| `externalDatabase.driver` | `postgres` | `MCHARBOR_DB_DRIVER`: `postgres`, `mysql` (MySQL and MariaDB) or `mssql` |
| `externalDatabase.existingSecret` | empty | Secret holding the DSN. Required in active/passive; in single mode, setting it replaces SQLite |
| `externalDatabase.dsnKey` | `dsn` | Key name of the DSN in that Secret, exposed as `MCHARBOR_DB_DSN` |
| `ha.pluginStore` | `database` | `MCHARBOR_PLUGIN_STORE`: `database`, `shared-fs:/path` or `object:<storage location id>`. For `shared-fs`, mount the shared volume with `extraVolumes` / `extraVolumeMounts` |
| `ha.antiAffinity` | `soft` | `soft`, `hard`, or anything else for none. Ignored when `affinity` is set |
| `ha.updateStrategy` | `OnDelete` | See [Upgrading](#upgrading) |
| `persistence.enabled` | `true` | Per-pod `data` claim. `false` uses an `emptyDir`, allowed only with an external database: the pod then regenerates its certificate and re-fetches plugin packages on every start |
| `persistence.storageClass` | cluster default | |
| `persistence.accessModes` | `[ReadWriteOnce]` | |
| `persistence.size` | `10Gi` | |
| `backupVolume.enabled` | `false` | Mounts a volume for backups at `backupVolume.mountPath` |
| `backupVolume.existingClaim` | empty | Use this claim instead of creating one. In active/passive it has to be `ReadWriteMany`, so whichever pod is active sees the same backups |
| `backupVolume.accessModes` / `size` / `storageClass` | `[ReadWriteOnce]` / `50Gi` / default | For the claim the chart creates. It is annotated `helm.sh/resource-policy: keep` |
| `backupVolume.mountPath` | `/mnt/backup` | |
| `extraCACerts.existingConfigMap` | empty | ConfigMap of PEM certificates McHarbor trusts when it calls out: a private registry, an internal licence server, PostgreSQL with a private CA. Mounted at `/etc/mcharbor/ca` and added through `SSL_CERT_DIR` |
| `docker.socket.enabled` | `false` | See [Docker hosts](#docker-hosts) |
| `docker.socket.hostPath` | `/var/run/docker.sock` | |
| `docker.socket.groupId` | empty | Group that owns the socket on the node; required when the socket is enabled |
| `service.type` | `ClusterIP` | |
| `service.port` | `8705` | |
| `service.annotations` | `{}` | |
| `ingress.enabled` | `false` | |
| `ingress.className` | empty | |
| `ingress.annotations` | `{}` | See [Ingress](#ingress) |
| `ingress.hosts` | `mcharbor.example.com`, `/` | |
| `ingress.tls` | `[]` | Standard Ingress TLS entries |
| `pdb.enabled` | `true` | Rendered in active/passive only |
| `pdb.maxUnavailable` | `1` | |
| `pdb.unhealthyPodEvictionPolicy` | `AlwaysAllow` | |
| `podSecurityContext` | UID/GID 1000, `fsGroup: 1000`, `runAsNonRoot`, `RuntimeDefault` seccomp | See [Security context](#security-context) |
| `securityContext` | no privilege escalation, all capabilities dropped, read-only root filesystem | |
| `resources` | requests 250m / 512Mi, limit 2Gi memory | |
| `probes.startup` / `liveness` / `readiness` | see `values.yaml` | The startup probe allows ten minutes, because first start runs every migration |
| `terminationGracePeriodSeconds` | `30` | Time for the active pod to release its lock and close connections |
| `podAnnotations` / `podLabels` / `nodeSelector` / `tolerations` / `affinity` / `topologySpreadConstraints` / `priorityClassName` | empty | Standard pod scheduling fields |
| `extraEnv` / `extraVolumes` / `extraVolumeMounts` | `[]` | Anything else from [Configuration](../configuration/README.md) |

Environment variables the chart sets itself: `PORT`, `DATA_DIR=/app/data`, `PUID`/`PGID`
(matching `runAsUser`/`runAsGroup`), `BACKUP_ENCRYPTION_KEY_FILE`, `MCHARBOR_NODE_ID` (the pod
name), `MCHARBOR_ADVERTISE_URL` (the pod's headless-Service address), `SELF_SIGNED_SAN_EXTRA`
(the Service and pod names), `MCHARBOR_DB_DRIVER`, `DATABASE_PATH` for SQLite, and
`MCHARBOR_HA_MODE=active-passive` plus `MCHARBOR_PLUGIN_STORE` in active/passive.

### What the chart refuses to render

| Condition | Why |
|---|---|
| `mode: active-passive` without `externalDatabase.existingSecret` | SQLite cannot be shared; a node would refuse to start anyway |
| `mode: active-passive` with a driver other than `postgres`, `mysql` or `mssql` | The active lock and failover are validated on PostgreSQL 16, MySQL 8.4, MariaDB 11 and SQL Server 2022 (MariaDB uses `mysql`) |
| `mode: active-passive` with `replicaCount` below 2 | |
| `mode: active-passive` without the three shared secrets | A node with its own `ENCRYPTION_KEY` refuses to start on a shared database; sessions and backups would not survive a failover |
| `mode: active-passive` with `docker.socket.enabled` | Each pod would manage a different node's daemon |
| `mode: single` with `persistence.enabled: false` and no external database | The SQLite database would be lost on every restart |
| `secrets.encryptionKey` that is not base64 of 32 bytes | McHarbor would refuse to start |
| `docker.socket.enabled` without `nodeSelector` or `docker.socket.groupId` | See below |

## Probes

| Probe | Route | Scheme |
|---|---|---|
| Startup | `/api/livez` | HTTPS when `tls.enabled`, otherwise HTTP |
| Liveness | `/api/livez` | same |
| Readiness | `/api/readyz` | same |

The kubelet does not verify certificates on HTTPS probes, so the self-signed certificate is
not a problem there.

`TLS_ENABLED` is only the default. Once HTTPS is switched off or on under **Settings →
HTTPS**, the value stored in the database wins at the next start, and the probes' scheme no
longer matches the listener. Keep `tls.enabled` in step with that setting.

## Security context

The image has no `USER` and starts as root by default: its entrypoint `chown`s `DATA_DIR`
to `PUID:PGID` and can refresh the system CA bundle, both of which need root. Neither is
needed in Kubernetes, so the chart runs the container as UID and GID 1000 instead:

- `fsGroup: 1000` makes the data volume writable, which replaces the entrypoint's `chown` (that
  `chown` still runs and is a no-op);
- all capabilities are dropped and privilege escalation is off; port 8705 needs no
  capability;
- the root filesystem is read-only, with an `emptyDir` at `/tmp`; the core server writes to
  `DATA_DIR` and `/tmp` only;
- the seccomp profile is `RuntimeDefault`.

This combination was checked by starting the image as UID 1000, with every capability dropped
and a read-only root filesystem: it starts, migrates, generates its certificate and answers
both probes. Two things change compared with running as root:

- The entrypoint cannot run `update-ca-certificates`. Use `extraCACerts.existingConfigMap`
  instead, which McHarbor's own outbound calls read through `SSL_CERT_DIR`.
- A plugin that writes outside `DATA_DIR` or `/tmp` fails. If one does, set
  `securityContext.readOnlyRootFilesystem: false` rather than running as root.

## Docker hosts

McHarbor manages Docker through a socket it can reach, or through the remote agent.

**The recommended pattern is the agent.** Install it on each Docker host; it dials **out** to
McHarbor over a WebSocket on `/api/agent/ws`, so the Docker hosts need no inbound port, the
socket never leaves the host it belongs to, and it works identically for one McHarbor pod or
several. Point the agent's `MCHARBOR_URL` at the external URL (the Ingress). In
active/passive, a standby refuses agent connections with `503`, and the Service only routes
to the active pod anyway, so every agent attaches to the active node. See
[Agents](../../agents/README.md).

**Mounting a node's Docker socket** (`docker.socket.enabled: true`) is supported in single mode,
with three conditions the chart enforces or you have to accept:

- **It is root on that node.** Anyone who can start a container through the socket can mount
  the node's filesystem. Every McHarbor administrator becomes root on the Kubernetes node, and
  the pod's own non-root, no-capability context no longer protects anything.
- **It pins the pod.** The socket belongs to one node, so `nodeSelector` is required; without
  it a reschedule would silently point McHarbor at a different node's daemon.
- **It needs the socket's group.** Set `docker.socket.groupId` to the output of
  `stat -c %g /var/run/docker.sock` on that node; it becomes a supplemental group of the pod.
- **It cannot be combined with active/passive**, and most Kubernetes nodes run containerd, not
  Docker, so there is no socket to mount.

## Ingress

The Ingress is off by default. When enabled it points at the `<release>` Service, which in
active/passive routes to the active pod only. TLS for users belongs on the Ingress, with a
certificate your clients already trust.

The backend speaks HTTPS with a self-signed certificate unless `tls.enabled: false`. For
ingress-nginx:

```yaml
ingress:
  enabled: true
  className: nginx
  annotations:
    nginx.ingress.kubernetes.io/backend-protocol: HTTPS
    nginx.ingress.kubernetes.io/proxy-body-size: "0"
    nginx.ingress.kubernetes.io/proxy-request-buffering: "off"
    nginx.ingress.kubernetes.io/proxy-buffering: "off"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
  hosts:
    - host: mcharbor.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: mcharbor-tls
      hosts:
        - mcharbor.example.com
config:
  trustedProxyCIDRs: 10.244.0.0/16
```

ingress-nginx handles WebSocket upgrades without an annotation. Other controllers need the
same four things (HTTPS or HTTP to the backend, no body limit, no buffering, hour-long read
timeouts) in their own syntax. [Reverse proxy](./reverse-proxy.md) explains why each one is
needed.

Alternatively set `tls.enabled: false` and let the Ingress talk plain HTTP to the pods. Do
that only where pod-to-pod traffic is already protected: a network policy that admits only
the ingress controller, or a service mesh with mTLS.

### The long-lived paths the annotations exist for

Nothing in the Ingress needs a per-path rule: everything is one `Prefix` match on `/`, on one
port. The four annotations exist because of the routes below, which are the ones that break
first under a default ingress configuration. A buffering proxy holds a stream until it has a
full response, and a 60-second read timeout kills an idle one.

**WebSocket: needs the upgrade to pass through, and no read timeout.**

| Route | What it carries |
|---|---|
| `GET /api/agent/ws` | The remote agent's outbound connection. This is how every Docker host that is not in the cluster is managed, and it is idle between commands |
| `GET /api/terminal/ws` | A shell inside a container |
| `GET /api/system/host-terminal/ws` | A shell on the Docker host itself |
| `GET /api/compute/console/{ticket}` | A virtual machine's serial or VNC console |

**Server-sent events: need `proxy-buffering: off`, or the client receives nothing until the
stream ends.**

| Route | What it carries |
|---|---|
| `GET /api/events/stream` | The live Docker event feed for an environment |
| `GET /api/logs/{id}` | A container's log tail |
| `GET /api/metrics/containers/{id}/stream` | Live container stats |
| `GET /api/workflows/{id}/live` | Workflow run progress |
| `POST /api/workflows/{id}/execute` | A workflow run's own output |
| `GET /api/system/os-updates/stream` | Host package upgrade progress |
| `POST /api/app-store/install/stream`, `POST /api/app-store/installations/{id}/uninstall/stream` | App install and uninstall progress |
| `POST /api/appstore/plugins/install/stream`, `POST /api/appstore/plugins/{slug}/update/stream` | Plugin install and update progress |
| `POST /api/containers/{id}/move/stream` | Moving a container between hosts |
| `POST /api/catalogs/{id}/test/stream` | A catalog connectivity test |

`proxy-request-buffering: off` and `proxy-body-size: "0"` are for the other direction: an
image or virtual-machine disk upload is streamed, and a body limit would cap it.

In active/passive, every one of these reaches the active pod, because the `<release>` Service
has `publishNotReadyAddresses: false` and a standby is never Ready. An agent that reconnects
during a failover is therefore routed to the new active pod without any change on the agent's
side.

## Plain manifests

`deploy/kubernetes/single-node.yaml` and `deploy/kubernetes/active-passive.yaml` are the chart
rendered with release name `mcharbor` from `values-single-node.yaml` and
`values-active-passive.yaml` beside them. They carry no namespace; apply them into one:

```bash
kubectl create namespace mcharbor
kubectl -n mcharbor create secret generic mcharbor-secrets ...   # as above
kubectl -n mcharbor apply -f deploy/kubernetes/single-node.yaml
```

Both expect the Secret `mcharbor-secrets`, and the active/passive file also expects
`mcharbor-database` with a `dsn` key. Both set `ALLOWED_ORIGINS` to
`https://mcharbor.example.com`; change it in the ConfigMap before applying.

To change them, edit the values file and run `deploy/kubernetes/generate.sh`. It uses a local
`helm` when there is one and the `alpine/helm` image otherwise, lints the chart, and rewrites
both files. Editing the rendered YAML directly works, but the next regeneration discards it.

## What is not supported

- **Active/active.** Several Ready pods behind one Service. See
  [High availability](../../settings/system/high-availability/README.md#what-is-not-supported).
- **MySQL or MariaDB in active/passive.** The chart refuses it.
- **Horizontal Pod Autoscaling.** More replicas add standbys, not capacity.
- **The Docker socket in active/passive**, for the reasons above.
- **Changing `mode` on an existing release.** Moving from SQLite to PostgreSQL is a data
  migration (see [Database](../../settings/system/database/README.md)), not a values change.

## Related

- [Deployment](./README.md): topologies and files
- [Reverse proxy](./reverse-proxy.md)
- [High availability](../../settings/system/high-availability/README.md)
- [Configuration](../configuration/README.md)
