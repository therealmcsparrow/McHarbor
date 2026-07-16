// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

# McHarbor Cluster Setup and Shared Archives

A McHarbor cluster runs two or more application nodes behind a load
balancer with a shared database and a shared object store (or SMB
share) for container-backup archives. The single-node SQLite path is
unchanged and is still the recommended default for personal and
small-team use — cluster mode is for environments that need
failover or higher availability.

## When to use a cluster

- You want zero-downtime upgrades of McHarbor itself.
- One McHarbor instance is the bottleneck for scheduled backups,
  metrics, or UI.
- You have a homelab, NAS, or VM that you can re-deploy to quickly
  and want to take the manual-recovery step out of a node failure.
- You already run Postgres or MySQL / MariaDB and an S3-compatible
  store (or a NAS you can expose over SMB / NFS) for other workloads
  and prefer to centralize.

## When not to use a cluster

- A single Raspberry Pi or NAS — cluster mode requires a separate
  database service and a shared archive target, which is more
  operational overhead than a single-node SQLite install.
- You are running McHarbor for one user or family. The single-node
  install is backed up by SQLite, has lower latency, and uses one
  named volume.

## Architecture

```
                                  ┌────────────────────────┐
        Internet  / LAN  ────────▶│   Load Balancer        │
                                  │  (cookie affinity or   │
                                  │   round-robin)         │
                                  └─┬───────────────┬──────┘
                                    │               │
                              ┌─────▼─────┐   ┌─────▼─────┐
                              │ McHarbor  │   │ McHarbor  │
                              │ node 1    │   │ node 2    │
                              │ (active)  │   │ (active)  │
                              └─────┬─────┘   └─────┬─────┘
                                    │               │
              ┌─────────────────────┴───────────────┴──────────────────┐
              │                                                     │
   ┌──────────▼─────────┐                              ┌─────────────▼─────────┐
   │  Shared database   │                              │  Shared archive store │
   │  (Postgres, MySQL, │                              │  S3, NFS, Samba share,│
   │  or MariaDB)       │                              │  Azure Files, etc.     │
   └────────────────────┘                              └───────────────────────┘
```

What changes from single-node:

| Concern | Single-node | Cluster |
| --- | --- | --- |
| Database | SQLite file under `DATA_DIR` | Postgres 14+ or MySQL 8+ / MariaDB 10.6+ |
| State | Local volume | Shared across all nodes |
| Container backup archives | Local filesystem (`/app/data/backups/...`) | S3-compatible object store **or** Samba / SMB share (NFS also works) |
| Lock-protected singletons | Always run | Run only on the coordinator-leader node |
| Health endpoint | `/api/health` | `/api/cluster/status` adds nodeId, leadership roles, DB reachability |

The single-node install path stays supported. You do not have to
migrate to a cluster.

## Choosing a database

McHarbor supports three database backends in cluster mode.

| Backend | When to pick it | When not to pick it |
| --- | --- | --- |
| SQLite (default) | Single-node installs only. | Not safe for active-active — single writer. |
| PostgreSQL 14+ | Production multi-node with strict consistency. Mature advisory-lock primitive. | More operational overhead (Postgres has a steeper ops learning curve than MySQL). |
| MySQL 8+ / MariaDB 10.6+ | Production multi-node with MySQL ops experience. Named locks via `GET_LOCK()` work on every supported version. | Slightly noisier than Postgres advisory locks during failover (the lock auto-releases on connection close, but a slow failover can cause a brief "no leader" window). |

The coordinator uses driver-specific primitives for singleton
locks:

- **Postgres**: `pg_try_advisory_lock(int)` on a long-lived
  connection. The lock auto-releases on connection close.
- **MySQL / MariaDB**: `GET_LOCK(name, 0)` (zero timeout =
  try-once) on a long-lived connection. The lock auto-releases
  on connection close.
- **SQLite**: no leader election. The single node always runs
  every job.

The choice is transparent to application code — the same Go
code runs on all three backends via `database/sql`.

## Choosing a shared archive target

Three patterns work for the shared-archive path.

### Pattern A — S3-compatible object store (recommended)

The bundled HA overlay includes MinIO as a test target. In
production point at AWS S3, Cloudflare R2, SeaweedFS, Garage,
or any S3-compatible store. The archive rows live in a
single bucket, prefixed by environment so multiple McHarbor
clusters can share a bucket.

Pros: highly available, scales without limit, no FUSE
dependency. Cons: pay-per-GB for cloud options.

### Pattern B — Samba / SMB share (homelab-friendly)

A NAS or any Windows / Linux host with an SMB share. Each
McHarbor node talks to the share over the SMB2 protocol using
the same `hirochachacha/go-smb2` client. Archives land at
`\\<server>\<share>\containers\...` from every node.

Pros: works with a NAS you already own, no cloud bill, low
operational overhead. Cons: single point of failure on the
share; throughput is bounded by the share's link speed.

This is the path most homelab users take. Each McHarbor node
must be able to reach the SMB server. The share credentials
are stored on every McHarbor node; the leader uploads, the
followers can read the same files for restore.

### Pattern C — NFS or other network filesystem

Mount an NFS share (or any FUSE-mountable network filesystem)
on every McHarbor node at a known path. Use the `local`
storage type pointing at the mounted path. No application
code changes.

Pros: no extra storage-credential surface in the app.
Cons: every node must mount the share before McHarbor boots;
file-locking semantics vary by NFS version.

This is the simplest pattern when you control the network and
the share is highly available (e.g. a TrueNAS volume).

## Prerequisites

You will deploy:

- **A shared database**. Postgres 14+ (16 recommended),
  MySQL 8+, or MariaDB 10.6+. The bundled HA overlay ships
  with Postgres 16. Smaller versions are supported but not
  exercised.
- **A shared archive target** (one of the three patterns
  above). For tests use MinIO from the bundled overlay.
- **2-3 application nodes**. 2 nodes is the minimum for
  failover. 3 is recommended for safe leader election during
  maintenance.
- **A load balancer** in front of the application nodes.
  Any TCP-level L4 load balancer works. Cookie affinity is
  not required because McHarbor is stateless; the load
  balancer only needs to spread requests and skip unhealthy
  nodes.
- **Shared secret**: a stable `MCHARBOR_SECRET` so sessions
  survive a failover to another node.
- **Backup encryption key**: generated once from Settings,
  mounted as a Docker secret on every node so encrypted
  archives can be decrypted from any node.

## Step 1 — Stand up the shared services

### Database

#### Postgres

The bundled overlay creates a Postgres 16 service. In
production, use your managed Postgres. Whichever you pick:

```sql
CREATE DATABASE mcharbor WITH ENCODING 'UTF8';
CREATE USER mcharbor WITH ENCRYPTED PASSWORD 'choose-a-strong-one';
GRANT ALL PRIVILEGES ON DATABASE mcharbor TO mcharbor;
```

McHarbor's first boot applies all migrations automatically.

#### MySQL or MariaDB

```sql
CREATE DATABASE mcharbor CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
CREATE USER mcharbor IDENTIFIED BY 'choose-a-strong-one';
GRANT ALL PRIVILEGES ON mcharbor.* TO 'mcharbor'@'%';
FLUSH PRIVILEGES;
```

Use the server's local timezone, but McHarbor sends
tz-aware RFC3339 timestamps so the application's date math
is correct regardless of the DB host's clock. The startup
code sets the session timezone to UTC for consistency.

### Archive target

For **S3** (or MinIO), create a bucket and a credential with
read+write. For **Samba**, create a share with read+write for
the McHarbor service account, then test connectivity from
every node:

```bash
smbclient -L //<server>/<share> -U <user>%<password>
```

For **NFS**, export the directory and mount it on every node
at the same path (e.g. `/mnt/backup`).

## Step 2 — Configure environment variables

The cluster uses three env vars on each node:

| Variable | Purpose | Example |
| --- | --- | --- |
| `MCHARBOR_DB_DRIVER` | `sqlite` (default), `postgres`, or `mysql`. | `postgres` |
| `MCHARBOR_DB_DSN` | Connection string for the external driver. | `postgres://mcharbor:secret@pg-host:5432/mcharbor?sslmode=require` |
| `MCHARBOR_NODE_ID` | Stable identifier for this node across restarts. Auto-generated if unset; reuse on restarts for log correlation. | `node-1` |

DSN format by driver:

| Driver | DSN format |
| --- | --- |
| Postgres | `postgres://user:password@host:5432/db?sslmode=require` |
| MySQL / MariaDB | `user:password@tcp(host:3306)/db?parseTime=true&loc=UTC&tls=true` |

The full HA example (`.env.ha.example`) also documents these
plus the existing `DATA_DIR` and `MCHARBOR_BACKUP_KEY_FILE`
variables so a clean install has one canonical place to
start.

Use the same `MCHARBOR_SECRET` on every node. Sessions are
signed with this secret so a user who logged in on node 1
keeps their session on node 2 after a failover.

## Step 3 — Define the compose stack

The bundled HA overlay (`docker-compose.ha.yml`) is the
starting point. It declares Postgres + MinIO + two McHarbor
instances on ports 8705/8706.

For production, replace the bundled Postgres and MinIO with
your managed services. The McHarbor service stays the same:

```yaml
services:
  mcharbor:
    build:
      context: .
      dockerfile: docker/Dockerfile
    image: ghcr.io/therealmcsparrow/mcharbor:latest
    restart: unless-stopped
    environment:
      - TZ=Europe/Amsterdam
      - LOG_LEVEL=info
      - MCHARBOR_DB_DRIVER=postgres            # or: mysql
      - MCHARBOR_DB_DSN=postgres://mcharbor:secret@pg-host:5432/mcharbor?sslmode=require
      - MCHARBOR_NODE_ID=node-1               # change per host
      - MCHARBOR_SECRET=shared-secret-for-all-nodes
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - mcharbor-data:/app/data
      # Optional: if you use the NFS pattern, mount the
      # network share at the same path on every node.
      - /mnt/backup:/mnt/backup
```

For MySQL / MariaDB, swap the DSN:

```yaml
      - MCHARBOR_DB_DRIVER=mysql
      - MCHARBOR_DB_DSN=mcharbor:secret@tcp(mysql-host:3306)/mcharbor?parseTime=true&loc=UTC&tls=true
```

Repeat for `mcharbor-2`, `mcharbor-3`, ... with a different
`MCHARBOR_NODE_ID` and a different host port mapping.

If you use the SMB pattern for archives, configure a storage
location in the UI after first boot (Settings → Storage →
New). Fill in:

- Type: **Samba**
- Name: anything (e.g. `nas-backups`)
- Host: the SMB server hostname or IP
- Port: `445` (or leave empty for the default)
- Share name: the SMB share name (e.g. `Backups`)
- Base path: directory inside the share where archives land
- Username + Password: a service account with read+write on
  the share

The location row stores the credentials encrypted at rest
with the same AES-256-GCM key that protects other secrets.

If you use Docker secrets for the backup encryption key, layer
in `docker-compose.secrets.yml` (or a similar file) so every
node mounts the same secret file.

## Step 4 — Set up the load balancer

McHarbor is stateless from the load balancer's point of view.
The same session cookie works on any node because they all
share `MCHARBOR_SECRET`.

Use any TCP L4 load balancer. Examples:

- **HAProxy**

  ```haproxy
  frontend mcharbor
      bind *:443 ssl crt /etc/ssl/mcharbor.pem
      default_backend mcharbor_nodes

  backend mcharbor_nodes
      balance roundrobin
      option httpchk GET /api/health
      http-check expect status 200
      server node1 10.0.0.11:5474 check
      server node2 10.0.0.12:5474 check
      server node3 10.0.0.13:5474 check
  ```

- **Nginx (stream module)**

  ```nginx
  stream {
      upstream mcharbor {
          server 10.0.0.11:5474;
          server 10.0.0.12:5474;
          server 10.0.0.13:5474;
      }
      server {
          listen 443 ssl;
          ssl_certificate     /etc/ssl/mcharbor.pem;
          ssl_certificate_key /etc/ssl/mcharbor.key;
          proxy_pass mcharbor;
          proxy_timeout 4h;  # longer than the longest backup upload
      }
  }
  ```

Two things to keep in mind when sizing timeouts:

- The longest single PUT to `/api/.../agent-archives/{id}` is the
  agent upload of a container backup. Default budget is 2 hours;
  do not set the LB below that.
- The connection-upgrade WebSocket for the agent is also
  long-lived. Sticky-by-cookie is not required but stable
  routing per agent is helpful for log correlation.

## Step 5 — First boot and verification

Bring up the cluster:

```bash
docker compose up -d
```

Watch the logs of each node until you see `mcharbor started`
plus `migration applied` lines for every migration:

```bash
docker compose logs -f mcharbor-1
```

Check the cluster status:

```bash
curl https://mcharbor.example.com/api/cluster/status | jq
```

Expected output (Postgres shown — `databaseDriver` will be
`mysql` if that's what you configured):

```json
{
  "nodeId": "node-1",
  "databaseDriver": "postgres",
  "roles": {
    "container_backup_scheduler": true
  },
  "database": { "reachable": true },
  "serverTime": "2026-07-08T10:00:00Z"
}
```

`container_backup_scheduler: true` on the node that currently
holds the scheduler leadership. The leader can change over
time — McHarbor uses the database's named-lock primitive
(Postgres `pg_try_advisory_lock`, MySQL `GET_LOCK`) to elect
a new leader when the previous one disappears.

Create a backup plan in the UI and trigger a manual run.
Verify:

- The plan executes on the leader node (`[INFO] container backup run started`).
- The plan uploads succeed against the shared archive target
  (S3, SMB, or NFS-mounted local path).
- Other nodes do not also run the plan (the lock prevents
  this).

## Day-2 operations

### Adding a node

1. Build the same image on the new host.
2. Use the same `MCHARBOR_SECRET` and the same `MCHARBOR_DB_DSN`.
3. Pick a unique `MCHARBOR_NODE_ID` (`node-4`, etc.).
4. Add the new host to the load balancer pool.
5. Start the container. The new node picks up the existing
   schema and starts serving traffic immediately. No
   database migration step is required.

### Removing a node

1. Drain it from the load balancer pool.
2. Stop the container.
3. The remaining nodes re-elect leadership within one
   scheduler tick (default 60 s). No manual coordination
   required.
4. If the node will be removed permanently, also clean up
   any `node-N` rows in the `cluster_status` table. The
   cluster module handles this automatically on the next
   status write.

### Updating McHarbor

You can update one node at a time. The recommended order is:

1. Drain the first node from the load balancer.
2. Stop the container and pull the new image.
3. Start the container. The new image runs the migration
   runner at boot; the database's named-lock primitive
   ensures only one node applies a given migration at a
   time, so the rest of the cluster can keep running the
   old image.
4. Wait until `/api/health` returns 200 and the node is
   back in the load balancer.
5. Repeat for the other nodes.

## Migrating from single-node to cluster without losing data

The goal of a cluster migration is to keep every artifact the
single-node install produced: environments, container
backup plans, container backup runs (and their on-disk
archives), notification channels, RBAC, audit log,
workflows, custom nodes, dashboards. The split between what
the cluster can and cannot carry over is a property of the
storage layout, not the database.

### What transfers automatically

The cluster keeps the **schema**. Every SQLite table has a
direct counterpart in Postgres / MySQL, and all 58+
migrations are now dialect-portable:

- `INSERT ... ON CONFLICT (cols) DO NOTHING` (Postgres) is
  translated to `INSERT IGNORE` for MySQL.
- `datetime('now')` (SQLite scalar) is translated to
  `CURRENT_TIMESTAMP` for MySQL.
- All `INTEGER DEFAULT 0/1` boolean columns stay as-is on
  every backend; both Postgres and MySQL accept integers
  with auto-cast.
- `PRAGMA foreign_keys = OFF` is stripped (no longer needed
  on the network drivers — they use the backend's native
  FK semantics).

So the database schema migrates byte-for-byte. The rows
also migrate — see the procedure below.

### What needs operator action

- **Container backup archives on the local filesystem**.
  The single-node install stored them at
  `{DATA_DIR}/backups/{env}/containers/...`. The cluster
  has no shared filesystem by default. To preserve access
  to existing archives from the cluster, you have three
  options:
  1. **NFS / mount the old disk on every node.** Mount the
     single-node DATA_DIR under the same path on every
     cluster node (e.g. `/mnt/old-backup-data`). The
     existing destination rows in the new database still
     point at the same logical paths, so the cluster can
     read them as long as the mount is in place. Use the
     `local` storage type pointing at the mount.
  2. **Copy the archives to the shared archive target**
     (S3, SMB, or NFS) and create new destination rows in
     the cluster DB that point at the new locations. Old
     destination rows remain in the DB for history but
     won't resolve to a file at the new path.
  3. **Accept that old runs are read-only history.** New
     runs land on the shared target. Old runs still show
     up in the UI with their original `remote_path`, but
     the file is no longer at that path. Restores of old
     runs fail with "file not found".

- **Docker socket local to the node.** Each cluster node
  has its own Docker daemon. Environments that pointed at
  `unix:///var/run/docker.sock` on the single-node host
  continue to work on whichever cluster node happens to
  handle that request — but for environments on the
  *original* host specifically, the cluster node must
  mount that host's Docker socket. Plan the bind mount
  for each node.

- **Agent tokens.** The agent uses a per-environment
  token. Tokens are stored in the database and migrate
  with the rest of the schema, so existing agents keep
  working as long as the network path to the cluster is
  reachable.

- **In-process coordination state**. The single-node
  install used local-only locks (SQLite reserved). The
  cluster uses database-backed locks. The transition is
  automatic; the first node to win the lock after
  migration becomes the leader.

### The recommended migration path

This procedure keeps the single-node install reachable
throughout the cutover, so you can roll back if anything
goes wrong.

#### 1. Pick the database

Decide between Postgres and MySQL / MariaDB. Stand up the
service if you don't already have one (see [Step 1](#step-1--stand-up-the-shared-services)).

#### 2. Pick the shared archive target

If you have a NAS or a network share, the NFS-mount path
needs zero new app config. If you don't, the S3-compatible
store is the cleanest option. If you want to stay on a
local filesystem, use an SMB / NFS share that every node
mounts at the same path.

#### 3. Capture a SQLite snapshot

Stop the single-node install cleanly. The simplest way:

```bash
docker compose down       # or: docker stop mcharbor
cp /path/to/data/mcharbor.db /backup/mcharbor-pre-cluster.db
```

The snapshot is your rollback target. Keep it until the
cluster has been running cleanly for at least a week.

#### 4. Export the schema and data with sqlite3

We use the official SQLite `sqlite3` CLI to produce a
portable SQL dump. Don't try to copy the .db file directly
to Postgres or MySQL — the on-disk format is different and
the SQLite-only constructs (PRAGMA, AUTOINCREMENT, the
datetime format) will not transfer.

```bash
sqlite3 /backup/mcharbor-pre-cluster.db .dump > /tmp/mcharbor-export.sql
wc -l /tmp/mcharbor-export.sql
```

The dump is one SQL statement per row plus the table
definitions. It includes the SQLite-only `BEGIN` / `COMMIT`
transaction markers around each row. The next step
post-processes the dump so the cluster database accepts it.

#### 5. Translate the dump to the cluster database dialect

The migration runner already translates the bundled
migration files at runtime (`ON CONFLICT (col) DO NOTHING`
→ `ON DUPLICATE KEY UPDATE col = col` for MySQL, plus
`datetime('now')` → `CURRENT_TIMESTAMP`). For the data
dump we do the same translation offline so we can apply it
to the cluster database.

The cleanest approach is to use the database's own
import utility:

```bash
# Postgres
sed -E 's/PRAGMA [^;]+;//g' /tmp/mcharbor-export.sql | \
  psql "postgres://mcharbor:secret@pg-host:5432/mcharbor?sslmode=require"

# MySQL / MariaDB
sed -E 's/PRAGMA [^;]+;//g' /tmp/mcharbor-export.sql | \
  mysql --protocol=tcp -h mysql-host -u mcharbor -p mcharbor
```

The `sed` strips the SQLite-only `PRAGMA` statements.
Postgres and MySQL are both tolerant of the leftover
`BEGIN; ... COMMIT;` pairs around row inserts — they treat
them as no-op transactions. The `INSERT OR REPLACE`,
`INSERT OR IGNORE`, and other SQLite-specific keywords are
not used in the McHarbor schema (the migrations were
already ported to `ON CONFLICT`), so the dump feeds
cleanly into both backends.

If your output contains literal `\r` characters from
Windows-style line endings, normalize them first:

```bash
dos2unix /tmp/mcharbor-export.sql   # optional
```

If you want a finer-grained translation, the same regex
set the migration runner uses works on the dump:

```bash
# Strip the `datetime('now')` SQLite scalar in INSERT
# statements and replace with CURRENT_TIMESTAMP. The
# migration runner's translator handles the same pattern
# at runtime; doing it here keeps the data import simple.
sed -E "s/datetime\\('now'\\)/CURRENT_TIMESTAMP/g" /tmp/mcharbor-export.sql \
  | sed -E "s/ON CONFLICT \\(([^)]+)\\) DO NOTHING/ON DUPLICATE KEY UPDATE \\1 = \\1/g" \
  | mysql --protocol=tcp -h mysql-host -u mcharbor -p mcharbor
```

#### 6. Record the data-only migrations as already applied

The cluster database starts empty. After the dump import
the schema_migrations table will be empty, but the schema
itself is already in place from the data import. The
migration runner will try to re-apply every migration,
which can fail on `CREATE TABLE` for tables that already
exist (with a "table already exists" error on Postgres or
MySQL).

Two ways to handle this:

1. **Strip the schema from the dump and only import the
   data.** Use the SQLite CLI's `.schema` separately to
   produce a schema-only file, translate it for the target
   database, and let the migration runner apply the
   schema. Then import the data. This is cleaner.

   ```bash
   # SQLite: write schema and data to separate files
   sqlite3 /backup/mcharbor-pre-cluster.db .schema > /tmp/mcharbor-schema.sql
   sqlite3 /backup/mcharbor-pre-cluster.db .dump --no-data > /tmp/mcharbor-schema-with-data.sql
   # Strip CREATE statements from the data dump — only keep
   # the row INSERTs.
   grep -v -E '^(CREATE|PRAGMA|BEGIN TRANSACTION)' /tmp/mcharbor-schema-with-data.sql > /tmp/mcharbor-data.sql
   # Translate the data dump and import.
   sed -E 's/PRAGMA [^;]+;//g; s/datetime\(.now.\)/CURRENT_TIMESTAMP/g; s/ON CONFLICT \(([^)]+)\) DO NOTHING/ON DUPLICATE KEY UPDATE \1 = \1/g' \
     /tmp/mcharbor-data.sql | mysql -h mysql-host -u mcharbor -p mcharbor
   ```

2. **Pre-seed the `schema_migrations` table with every
   migration filename.** Faster if you don't care about
   the schema-reapplication warning in the logs. The
   trick is to record every applied migration in the new
   database, then let the migration runner skip them all.

   ```bash
   # Generate a list of all migration filenames
   ls src/backend/core/db/migrations/*.sql | \
     xargs -I{} basename {} | \
     sed "s/'/''/g" > /tmp/migration-names.txt

   # Postgres
   psql "$DSN" -c "$(cat <<EOF
   INSERT INTO schema_migrations (name) VALUES
   $(awk '{printf "('%s'),\n", $1}' /tmp/migration-names.txt | sed '$ s/,$//')
   ON CONFLICT (name) DO NOTHING;
   EOF
   )"

   # MySQL / MariaDB
   mysql -h host -u mcharbor -p mcharbor -e "
   INSERT IGNORE INTO schema_migrations (name) VALUES
   $(awk '{printf \"('%s'),\n\", $1}' /tmp/migration-names.txt | sed '$ s/,$//');
   "
   ```

   With every migration marked applied, the cluster's
   migration runner skips the schema and goes straight to
   startup.

#### 7. Bring up the cluster with the imported data

Start one cluster node against the new database. Tail the
log; you should see every migration marked as already
applied, followed by a normal startup:

```bash
docker compose up -d mcharbor-1
docker compose logs -f mcharbor-1
```

Once node 1 is healthy and serving, start the other
nodes against the same database.

#### 8. Move archives into the shared target

For each destination in the single-node install that held
a successful run:

1. **NFS / shared mount**: keep the mounts. The cluster
   nodes can read the archives at the same paths the
   destination rows point at.
2. **SMB / S3**: copy the archives from the single-node
   DATA_DIR to the new shared target. Use a script that
   mirrors the directory layout:

   ```bash
   rsync -a /path/to/old/data/backups/ \
     /mnt/nas/Backups/  # or: aws s3 sync ...
   ```

   Then create new storage locations in the cluster UI
   that point at the shared target. New runs will land on
   the shared target; old runs keep their existing
   destination rows.

#### 9. Verify and decommission the single-node install

In the cluster UI, click through every environment and
every container backup plan and confirm:

- The plans list the correct destinations.
- Recent runs show `success` for every destination that
  should succeed.
- The "Open" link for a run on the shared target downloads
  the file (verifies the credentials work from the
  cluster node that handled the request).

Once verified, stop and remove the single-node container.
Keep the SQLite snapshot from step 3 around for at least
a week. After that, archive it or delete it.

#### 10. Roll back (if anything goes wrong)

If the cluster can't be brought up cleanly, the
single-node install can be restarted against its
original SQLite file with no loss of data:

```bash
docker stop mcharbor-cluster-node-1 mcharbor-cluster-node-2
docker compose up -d   # the original single-node compose
```

The SQLite snapshot from step 3 is unchanged. The cluster
nodes can be removed once the single-node install is back
online. The cluster database can be kept or dropped —
nothing in the single-node install depends on it.

### Partial migrations

You don't have to do the full migration in one sitting.
Useful intermediate states:

- **Step 1-7 only**: the cluster runs against the imported
  database with the existing archive directories still on
  the single-node host. This is the "test the cluster"
  state — operators can verify the cluster is healthy
  before any archive relocation happens.
- **Step 1-9 in order**: the cluster takes over completely.
  The single-node install is decommissioned.

The migration is incremental; do as much or as little as
your maintenance window allows.

## Troubleshooting

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| `/api/cluster/status` returns `database.reachable: false` on one node | Node lost network to the database | Check the node's connection to the database; the node will recover automatically once the connection comes back. |
| Logs show `pg_try_advisory_lock` errors (Postgres) | Postgres permissions or DSN wrong | Verify the DSN and the user's permissions on the database. |
| Logs show `GET_LOCK` errors (MySQL / MariaDB) | The `mcharbor` user lacks the global `GET_LOCK` privilege, or the DSN is wrong | Verify the DSN; `GET_LOCK` is a global MySQL function, available to all users by default. |
| Both nodes run the same backup plan | Database named lock is not held by either node | Check `/api/cluster/status`; both should report `container_backup_scheduler` for only one of them. Restart both nodes if leadership is stuck. |
| New node fails to apply migrations | Database connection is fine but user lacks CREATE / ALTER | Grant the user DDL rights. On Postgres: `GRANT ALL ON DATABASE mcharbor TO mcharbor`. On MySQL: `GRANT ALL ON mcharbor.* TO 'mcharbor'@'%'`. |
| Logs show "database is locked" errors everywhere | A long-running write transaction is starving the writer lock | SQLite-only — Postgres and MySQL handle row-level locks without this issue. |
| Container backup upload to SMB share fails with "permission denied" | The SMB user doesn't have write access to the share | `chmod -R u+rwX` on the share from a host with access; the test endpoint validates credentials on the share before upload. |
| Container backup upload to SMB share fails with "host unreachable" | A firewall between McHarbor and the SMB server blocks port 445 | Open 445/TCP from the McHarbor node to the SMB server. |
| Data imported into the cluster database is missing rows | The data dump was filtered by `sed` and lost some `INSERT` statements | Re-export from the SQLite snapshot and re-import. The original SQLite file is unchanged. |

## See also

- `application/deployment-and-runtime.md` — single-node install, TLS, runtime
- `application/remote-agent.md` — the remote agent protocol
- `application/agent-setup.md` — agent installation, including agent-to-agent
- `application/configuration-and-data.md` — environment variables, encryption
