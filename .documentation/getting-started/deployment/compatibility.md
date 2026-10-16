# Compatibility matrix

What this release of McHarbor runs with, and what it refuses. Read it before an upgrade: a
row that says "not supported" is enforced in code, not a recommendation.

Current release: **2026.09.01**. Versions in this page are the ones this release was built
and tested against; a version this page does not name is untested rather than forbidden,
unless a row says otherwise.

## Databases

| Backend | Minimum | Tested against | Single node | Active/passive | Active/active |
|---|---|---|---|---|---|
| SQLite (bundled, `modernc.org/sqlite`, no CGO) | ships with McHarbor | the bundled build | Yes, the default | **No** | **No** |
| PostgreSQL (`github.com/jackc/pgx/v5`) | 12 | 16 | Yes | Yes | Readiness-gated; see [active/active](../../settings/system/high-availability/active-active.md) |
| MySQL (`github.com/go-sql-driver/mysql`) | 8.0 | 8.4 | Yes | Yes, validated on 8.4 | Readiness-gated |
| MariaDB (same driver) | 10.6 | 11 | Yes | Yes, validated on 11 | Readiness-gated |
| Microsoft SQL Server (`github.com/microsoft/go-mssqldb`) | 2022 | 2022 | Yes | Yes, validated on 2022 | Readiness-gated |

`MCHARBOR_DB_DRIVER` accepts `sqlite`, `postgres`, `mysql` and `mssql` (spellings in
[Configuration](../configuration/README.md#database)); any other value stops McHarbor at startup
instead of starting it on the SQLite file. MariaDB uses the `mysql` driver.

**SQLite cannot be shared between nodes.** A second node pointed at the same file is not a
cluster, and `MCHARBOR_HA_MODE=active-passive` on SQLite refuses to start rather than
corrupt the database.

**Active/passive is validated on PostgreSQL 16, MySQL 8.4, MariaDB 11 and SQL Server 2022.** The
active lock is a PostgreSQL advisory lock, a MySQL/MariaDB `GET_LOCK` named lock or a SQL Server
`sp_getapplock` application lock, all held on a session, so a node whose database session dies
loses the role with it. Each backend has been through a real two-node run (one node holds the
lock, it is killed, the standby takes over, the first node restarts as standby), with the standby
active about five seconds after the kill (one HA tick) on all four. `scripts/ha/failover-check.sh`
is that run, and CI repeats it on every push. The Helm chart renders `mode: active-passive` with
`externalDatabase.driver` set to `postgres`, `mysql` or `mssql`.

**Microsoft SQL Server** applies every migration on SQL Server 2022, and CI repeats that on every
push. Foreign keys SQL Server refuses for having more than one cascade path are carried by delete
triggers instead, so deletes still cascade. The database needs `READ_COMMITTED_SNAPSHOT` on, and
the login must not set `XACT_ABORT`; see [Database](../../settings/system/database/README.md).
Builds before this one ignored `MCHARBOR_DB_DRIVER=mssql` and started on SQLite; SQL Server could
only be reached through **Settings → System → Database**.

**Active/active is readiness-gated.** Every node serves every request, and a request for an
agent held by another node is relayed to it, terminals, consoles and host logs included. The
mode is selectable once the HA readiness gates pass; see
[active/active](../../settings/system/high-availability/active-active.md).

## Remote agent

The agent and McHarbor share one version, and both are at **2026.09.01** in this release.
Beyond that, individual features carry their own floor: an older agent does not refuse a
message it does not know, it **drops** it, so the server would otherwise wait out the full
timeout for a reply that is never coming. Each floor below turns that silent hang into a
named answer.

The four numeric floors are historical and deliberate. They are not rewritten to the
calendar scheme: a `2026.09.01` agent clears all of them because 2026 is greater than 3,
while a genuinely old `3.5.3` agent still fails the `3.8.0` gate exactly as intended.

| Feature | Minimum agent | Minimum build within that version |
|---|---|---|
| Container, image, volume, network and stack management | any connected agent | — |
| Terminal exec through the agent | 1.1.0 | — |
| KVM virtual machines and LXC system containers | **3.0.0** | — |
| Adopting an image from the shared image library | **3.2.0** | — |
| Managed guest networks, and sharing a host adapter | **3.4.0** | — |
| Listing the volumes in a pool, and tailing a guest journal unit | **3.8.0** | — |
| Host OS details and host package upgrades | 2026.09.01 | `202609182336` |
| The libvirt, agent and per-guest qemu log sources | 2026.09.01 | `202609231806` |
| Pushed container stats instead of one pull per container | 2026.09.01 | `202609251653` |
| Bundled agent version this release serves and updates toward | 2026.09.01 | the release's `BUILD` |

Every value in that table is a constant in `src/backend/core/agent/version.go`. Where a
build stamp is listed, the version alone cannot separate the agents: calendar versioning
gives every agent built in one month the same version, so the build is part of the gate
rather than decoration.

An agent that reports build `dev` was compiled without the `-X main.agentBuild=...` flag. It
fails every build-stamped gate above, and it is indistinguishable in the interface from an
agent that is simply old.

**A native agent updates only from a published GitHub release.** Raising the version in this
repository does not deliver anything; the agent fetches the release for the target version
and, when there is none, fails on a 404 and backs off for six hours.

## High-availability modes

| Mode | `MCHARBOR_HA_MODE` | Nodes | Database | Status |
|---|---|---|---|---|
| Single node | unset | 1 | SQLite or external | Supported, the default |
| Active/passive | `active-passive` | 2 or more | PostgreSQL, MySQL or MariaDB, shared | Supported. Exactly one node is Ready; every other node is alive and deliberately NotReady |
| Active/active | `active-active` | — | — | **Refused.** The request is downgraded to `active-passive` and the reason is logged |

Active/active is not missing, it is gated. Two readiness gates stand in the way and both
name their remediation in the interface: a remote agent holds one connection to one node and
a request served by another node cannot reach it (cross-node agent routing is not
implemented), and console tickets and rate-limit counters are kept in memory per node. While
either gate fails, the mode cannot enable itself. See
[High availability](../../settings/system/high-availability/README.md).

`MCHARBOR_HA_MODE` is an environment variable, not a stored setting, because it is read
before the database is open and every node has to agree on it from the moment it starts.

## Plugin package store

In any deployment with more than one node, or any node whose data directory does not
survive a restart, the plugin package store has to be somewhere every node can read.

| `MCHARBOR_PLUGIN_STORE` | Where packages live | Multi-node |
|---|---|---|
| unset, or `local` | the node's own `DATA_DIR` | No |
| `database` | the shared database, as chunks | Yes, the default in the Helm chart's active/passive mode |
| `shared-fs:<absolute path>` | a volume mounted on every node | Yes, if the volume is `ReadWriteMany` |
| `object:<storage location id>` | a configured object-storage location | Yes |

## Hosts McHarbor manages

| What | Minimum | Tested against |
|---|---|---|
| Docker Engine on a managed host | 20.10 | Docker Engine 28.x, through Docker SDK 28.5.2 with API version negotiation |
| Docker Compose, for stacks and encrypted backups | v2 plugin | v2 |
| Kubernetes cluster McHarbor manages (the Kubernetes add-on) | 1.27 | client-go 0.37, so the 1.37 API surface |

The Docker client negotiates the API version with each daemon, so a newer daemon than the
SDK is not a problem; a daemon older than 20.10 lacks endpoints McHarbor calls.

## Hosts McHarbor runs on

| Platform | Minimum | Notes |
|---|---|---|
| Docker Compose on Linux | Docker Engine 20.10 with the Compose v2 plugin | **The supported default.** Ubuntu Server 24.04 LTS and 26.04 LTS are what McHarbor is developed against |
| Kubernetes | **1.27** | The chart uses `unhealthyPodEvictionPolicy` on the PodDisruptionBudget and `persistentVolumeClaimRetentionPolicy` on the StatefulSet, both of which need 1.27 |
| Helm | 3 | Optional; `deploy/kubernetes/` carries the same manifests already rendered |
| Debian, RHEL, Fedora, SUSE, Arch | — | Any host that runs Docker Engine. **Alpine is not supported** |

## Compute (KVM and LXC) hosts

Compute needs a **native** agent on the host: `qemu`, `libvirt` and `systemd` have to be
real, and the containerised agent has none of them. The floors are `3.0.0` for compute at
all, and the three later numeric floors above for the individual capabilities.

## Upgrade direction

Migrations only move forward. There is no down-migration for any of them, on any backend, so
a rollback is a restore:

- SQLite: take an instance backup or a copy of the data volume first.
- External database: take a dump first, and on a rollback stop every node, restore, and
  start the previous image tag on all of them together.

Keep `ENCRYPTION_KEY`, `MCHARBOR_SECRET` and the backup key unchanged across an upgrade and
any rollback. A node whose encryption key does not match the key-check row in the shared
database refuses to start.

The **Pre-upgrade check** (Enterprise → Support bundle) reads the installed plugins'
declared McHarbor ranges against the target version and links back to this page.

## Related

- [Deployment](./README.md): topologies and the files that implement them
- [Kubernetes](./kubernetes.md)
- [Compose](./compose.md)
- [Upgrading](../upgrading/README.md)
- [Database](../../settings/system/database/README.md)
- [High availability](../../settings/system/high-availability/README.md)
- [Pre-upgrade check](../../settings/system/support/pre-upgrade-check.md)
