# High availability

**Enterprise → High availability** · `/enterprise/ha`

Two or more McHarbor nodes on one shared database. This page describes **active/passive**:
exactly one node serves at a time, and a standby takes over within seconds when the active
node stops, is drained, or is handed over to.

The other mode, [active/active](active-active.md), has every node serve every request and
forwards an agent call to the node holding the agent. Active/passive stays the simpler
answer: it needs no network path between the nodes and has no capability that works on one
node and not another.

## Permissions

| Key | What it unlocks |
|---|---|
| `cluster.view` | The High availability page, `GET /api/cluster/status`, `GET /api/cluster/readiness`, `GET /api/cluster/startup-health` and the HA migration plan |
| `cluster.manage` | **Make active**, **Drain** and **Resume** on any node |

Both are global. Neither is in a role preset.

## What you need

| Requirement | Why |
|---|---|
| PostgreSQL, MySQL 8.4, MariaDB 11 or SQL Server 2022 shared by every node | SQLite cannot be shared between nodes. See *MySQL and MariaDB* and *SQL Server* below for what differs |
| The same `ENCRYPTION_KEY` on every node | Without it each node generates its own key in `DATA_DIR` and cannot read secrets another node wrote. A node whose key does not match the database **refuses to start** |
| The same `MCHARBOR_SECRET` on every node | Sessions survive a failover only if every node signs them the same way |
| The same backup key on every node | Encrypted archives written by one node must be readable by the next active one |
| A stable `MCHARBOR_NODE_ID` per node | It is how the topology, the audit log and the failover controls name the node |
| A plugin package store | See *Plugin packages* below |
| A load balancer that health-checks `/api/readyz` | It is what sends traffic to the active node only |

`docker-compose.ha.yml` and `ha.env.example` in the repository are a two-node template
with all of the above.

### Shared keys as Docker secrets

The template hands the secret, the encryption key, the database connection string and the
backup key to every node as Docker secrets, not variables:
`MCHARBOR_SECRET_FILE=/run/secrets/mcharbor_secret`,
`ENCRYPTION_KEY_FILE=/run/secrets/mcharbor_encryption_key`,
`MCHARBOR_DB_DSN_FILE=/run/secrets/mcharbor_db_dsn` and
`BACKUP_ENCRYPTION_KEY_FILE=/run/secrets/mcharbor_backup_key`. The bundled PostgreSQL gets its
password the same way, through the official image's
`POSTGRES_PASSWORD_FILE=/run/secrets/postgres_password`. A variable is part of the
container's configuration, which `docker inspect` and anything with socket access can read; a
secret is a file mounted into the container, and is in no inspect output.

Create the files once and copy them to every node:

```bash
mkdir -p secrets
openssl rand -base64 48 | tr -d '\n' > secrets/mcharbor_secret
openssl rand -base64 32 | tr -d '\n' > secrets/mcharbor_encryption_key
openssl rand -hex 24 | tr -d '\n' > secrets/postgres_password
printf 'postgres://mcharbor:%s@postgres:5432/mcharbor?sslmode=disable' "$(cat secrets/postgres_password)" > secrets/mcharbor_db_dsn
chmod 0400 secrets/mcharbor_secret secrets/mcharbor_encryption_key secrets/postgres_password secrets/mcharbor_db_dsn
```

**`secrets/postgres_password` and the password inside `secrets/mcharbor_db_dsn` must be the
same.** PostgreSQL applies its password only when it first initialises an empty data volume;
the nodes read the connection string on every start. Changing `postgres_password` afterwards
does not change the database's password, it only makes the two files disagree and the nodes
fail to connect. Rotate the database password with `ALTER ROLE` and then update both files.
If the password contains `@`, `:`, `/`, `?`, `#` or `%`, URL-encode it in the connection
string (`openssl rand -hex 24` avoids the problem).

`MCHARBOR_SECRET_HOST_FILE`, `MCHARBOR_ENCRYPTION_KEY_HOST_FILE`, `MCHARBOR_DB_DSN_HOST_FILE`
and `POSTGRES_PASSWORD_HOST_FILE` in `ha.env` move the files elsewhere on the host. A node
refuses to start when a `_FILE` variable and its plain variable are both set, or when the file
is missing, unreadable or empty.

**Moving an existing cluster to the new template.** Write the values you already run with into
the files, never new ones, or every node loses its sessions and can no longer read the
database:

```bash
printf '%s' "$MCHARBOR_SECRET" > secrets/mcharbor_secret
printf '%s' "$ENCRYPTION_KEY" > secrets/mcharbor_encryption_key
printf '%s' "$POSTGRES_PASSWORD" > secrets/postgres_password
printf 'postgres://mcharbor:%s@postgres:5432/mcharbor?sslmode=disable' "$POSTGRES_PASSWORD" > secrets/mcharbor_db_dsn
```

then remove `MCHARBOR_SECRET`, `ENCRYPTION_KEY` and `POSTGRES_PASSWORD` from `ha.env` and
recreate the nodes. `postgres_password` takes the **current** PostgreSQL password, the one the
existing data volume was initialised with. PostgreSQL ignores the file once the database
exists, so a wrong value changes nothing inside the volume, but it then disagrees with the
connection string, and the nodes stop connecting. The DSN must be the one the nodes already
use. With the template, the line above builds it from the same password; with your own
database, copy the `MCHARBOR_DB_DSN` value you run with (and omit the `postgres` service and
`postgres_password` secret). A missing secret file makes `docker compose up` fail before any
container starts.

## Settings

| Variable | Values | Effect |
|---|---|---|
| `MCHARBOR_HA_MODE` | empty (peers) · `active-passive` · `active-active` | `active-passive` makes exactly one node active. A node refuses to start in this mode on SQLite. `active-active` makes every node serve; see [active/active](active-active.md) |
| `MCHARBOR_PLUGIN_STORE` | empty (local) · `database` · `shared-fs:/path` · `object:<storage location id>` | Where installed plugin packages are kept for the other nodes |
| `MCHARBOR_ADVERTISE_URL` | a URL | How other nodes and operators reach this node; shown in the topology |
| `MCHARBOR_RESET_KEY_CHECK` | `1` | Accepts a deliberate change of `ENCRYPTION_KEY` once; remove it afterwards |

Every node must carry the same `MCHARBOR_HA_MODE` and `MCHARBOR_PLUGIN_STORE`.

## How a node decides its role

In active/passive mode each node, every five seconds:

1. Tries to hold the **active lock**, a database session lock (a PostgreSQL advisory lock, or
   `GET_LOCK` on MySQL and MariaDB, or `sp_getapplock` on SQL Server). Whoever holds it is active.
   Because it is a session lock it is also the fence: an active node whose database session
   dies loses the lock, and its role, at that moment.
2. Publishes a heartbeat. A node that has not beaten for 30 seconds shows **Not responding**.
3. Applies any drain request and any handover request addressed to it.

A **standby**:

- runs none of the background jobs that must run once (schedules, collectors, pruners, the
  alert engine, the ticket outbox …); they belong to the active node;
- serves reads, and refuses every change with `503` and "This McHarbor node is a standby;
  changes are made on the active node";
- refuses remote agent connections, so every agent stays attached to the active node;
- answers `503` on `/api/readyz`, which keeps the load balancer from sending it traffic.

Every signed-in page on a standby shows **This is a standby node**.

## The controls

| Control | What happens | Refused when |
|---|---|---|
| **Make active** (on a standby) | The active node releases its lock and every job, and the chosen node takes over on its next tick, usually within ten seconds. The request is consumed once it completes, so the old active does not take the role back later | Active/passive is off, the node is not alive or not running, or the cluster is in a split brain |
| **Drain** | The node stops reporting ready. An active node hands over first | The node is stopped, or a split brain |
| **Resume** | The node reports ready again and can become active | — |

Each asks for confirmation and is recorded in the audit log (`cluster.failover`,
`cluster.drain`, `cluster.resume`). The node's own reaction is recorded too:
`cluster.promoted`, `cluster.demoted`, `cluster.active_lost`, `cluster.drained`,
`cluster.resumed`.

A node that stops cleanly releases the active lock on its way down, so the standby takes
over on its next tick rather than after the database notices the session is gone.

## Split brain

If two live nodes both report the active role, the page shows **More than one node claims
to be active** and every control is disabled until only one remains. With a session lock
this can only follow a database partition; check each node's database connection.

## Plugin packages

A plugin is installed once, on the active node, but every node has to be able to load it.
`MCHARBOR_PLUGIN_STORE` chooses where the verified package is kept:

| Store | Where | When to pick it |
|---|---|---|
| `database` | The shared database, in chunks | Nothing extra to run. The default in the template |
| `shared-fs:/path` | Files on a mount every node sees (NFS, SMB, a cluster filesystem) | You already have shared storage for backups |
| `object:<id>` | A configured storage location, through its storage plugin | You want packages off the database; the storage plugin itself must be installed |

Every node places the packages it is missing at start, every minute, and at the moment it
is promoted. Each package is verified again against the node's own plugin signing keys
before it touches the plugins directory; one whose bytes no longer match the signature it
was installed with is refused and logged. The page's **Plugin packages** card says which
store is in use, and warns when active/passive runs with packages kept on each node only.

## Readiness checklist

The **HA readiness** card checks the cluster against a target, **Active/active** or
**Active/passive**, through `GET /api/cluster/readiness?target=active-active|active-passive`.
Each gate is `pass`, `warn` or `fail`; a failing gate blocks the target, a warning does not.

| Gate | Passes when |
|---|---|
| `externalDatabase` | The driver is PostgreSQL, MySQL, MariaDB or SQL Server and answers from this node |
| `encryptionKey` | The key comes from `ENCRYPTION_KEY` and matches the key check row every node verifies at boot |
| `pluginStore` | `MCHARBOR_PLUGIN_STORE` is not local |
| `sameVersion` | Every live node in the registry runs the same version and build (a warning when the registry cannot be read) |
| `agentRouting` | Always in active/passive, since only the active node serves. In active/active it passes once a ping has come back from an agent on another node over both hops: the Docker API hop and the session relay that carries terminals, the console, host logs and agent messages. It warns while either is unproven or the relay failed, and fails when the hop is unconfigured or its last real Docker call failed |
| `sessions` | Always: sessions live in the shared database, so sticky sessions are not required |
| `perNodeState` | Always in active/passive. In active/active it warns: console tickets are in the shared database, but rate-limit counters and the console viewer cap are per node by design. It fails when the ticket table does not answer from this node |
| `coordinator` | The lock session answers (never on SQLite) |

`enabled` in the response is true only when no gate fails. Warnings are shown and do not
block, which is how active/active is allowed while the stream paths it cannot forward are
still named.

## Startup health

At boot every node logs one `startup health` line: database driver and whether it
answers, whether `DATA_DIR` is writable and its free space (a warning below 1 GiB), where
the encryption key came from (`env` or `file`), the TLS mode, the plugin store, the HA mode
and, when the mode is `active-active`, whether this node's readiness gates refuse it. The
same report is on the page's
**Startup health** card and at `GET /api/cluster/startup-health`.

## Node identity

Every log line carries `node_id`, every audit row records the node that served it
(`audit_logs.node_id`, `nodeId` in the audit API) and, when its remote work ran on another
node, the node that executed it (`audit_logs.exec_node_id`, `execNodeId`), and every
Prometheus series from
`/api/metrics` carries one constant `node_id` label. Set `MCHARBOR_NODE_ID` so the value
survives restarts; without it a random `node-…` name is chosen at each start.

## MySQL and MariaDB

The application's statements are written for SQLite and translated at the database driver:
reserved words used as names (`key`, `groups`, `condition`, `trigger`, `system`,
`generated`, `release`, …) are quoted, upserts become `ON DUPLICATE KEY UPDATE`, and an
RFC3339 timestamp bound to a `DATETIME` column is converted to UTC. The node reads which
columns are `DATETIME` from `information_schema` after migrations, and again when a plugin
migration adds one. The DSN is completed with `parseTime=true`, `loc=UTC`, `time_zone`,
`sql_mode` and `charset` on every pooled connection; values already in the DSN win, except
`parseTime`, which must be `true`.

**Failover is validated on MySQL 8.4 and MariaDB 11**, as it is on PostgreSQL 16: two nodes on
one database, the active killed outright, the standby active about five seconds later (one
tick), and the killed node back as a standby when it restarts. A clean stop hands over in the
same time. `scripts/ha/failover-check.sh` runs exactly that against any of the three and is part
of CI, and the Helm chart accepts `externalDatabase.driver: mysql` in active/passive mode.

A few migration statements have no MySQL equivalent and are skipped, with a warning in the
log: the partial unique indexes (one default environment, one default channel, one default
AI provider, unique role assignments) and the triggers that make the audit log append-only
and archive deleted API keys. PostgreSQL enforces all of them, so it remains the most
complete choice.

## SQL Server

The active lock is an `sp_getapplock` application lock, exclusive and owned by the session, on a
connection the node keeps for it; a node whose session dies loses the lock and its role, as on
the other backends. SQL Server has no call that releases every lock at once, so a node handing
over releases its locks one by one.

**Failover is validated on SQL Server 2022** in the same two-node run as the other backends: the
active killed outright, the standby active about five seconds later (one tick), the killed node
back as a standby, and a clean stop handing over in the same time. `scripts/ha/failover-check.sh`
with `HA_DB_DRIVER=mssql` is that run and is part of CI, and the Helm chart accepts
`externalDatabase.driver: mssql` in active/passive mode.

What the database needs:

| Requirement | Why |
|---|---|
| `READ_COMMITTED_SNAPSHOT` on | Without it a long read blocks writers, and two nodes reading and writing the same tables deadlock under load |
| No `XACT_ABORT ON` for the login | McHarbor refuses to start on a session that has it |
| `MCHARBOR_DB_DSN` as a `sqlserver://` URL naming the database | The ODBC keyword form and the system databases are refused |

Builds before this one ignored `MCHARBOR_DB_DRIVER=mssql` and started on the SQLite file, so a
node configured through the environment was never on SQL Server. A value McHarbor does not
recognise now stops the node at startup instead.

## Probes

| Route | Answers | Use it for |
|---|---|---|
| `GET /api/livez` | `200` while the process runs | Container health |
| `GET /api/readyz` | `200` when ready, `503` otherwise, with a pass/fail per check: `database`, `coordinator`, `accepting` (not draining), `serving` (not a standby) | Load balancer health |
| `GET /api/health` | `200`, the node id and its role | The standby banner |

The probes are public and carry verdicts only, never an error text.

## What is not supported

- **A session following its agent.** In [active/active](active-active.md) a terminal,
  console or log stream opened on any node is relayed to the node holding the agent, but
  when the agent reconnects to another node the open session ends, naming why, and has to
  be reopened. Active/passive is unaffected: every agent is on the one node that serves.

## Related

- [Active/active](active-active.md): every node serving, and the agent call forwarded to
  the node holding the agent
- [Database](../database/README.md): moving from SQLite to PostgreSQL
- [Instance backup](../instance-backup/README.md): the key material every node must share
