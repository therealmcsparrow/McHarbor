# Compose

Two production Compose files under `deploy/compose/`, each with an `.env.example` listing every
variable it reads. The `docker-compose.yml` in the repository root is the development file (it
builds the image from source and mounts the catalog directories a developer works on) and is
not the starting point for a production host.

| File | What it runs |
|---|---|
| `deploy/compose/single-node/docker-compose.yml` | One McHarbor node on SQLite |
| `deploy/compose/external-database/docker-compose.yml` | One McHarbor node on PostgreSQL 17 |
| `docker-compose.ha.yml` + `ha.env.example` (repository root) | Two nodes, active/passive, on PostgreSQL; see [Active/passive](#activepassive) |

## Single node

### Prepare the host

```bash
mkdir -p /opt/mcharbor && cd /opt/mcharbor
cp <repo>/deploy/compose/single-node/docker-compose.yml .
cp <repo>/deploy/compose/single-node/.env.example .env

mkdir -p secrets backup ca
openssl rand -base64 32 > secrets/mcharbor_backup_key
chown 1000:1000 secrets/mcharbor_backup_key backup
chmod 0400 secrets/mcharbor_backup_key

stat -c %g /var/run/docker.sock      # put this in DOCKER_GID
```

Then edit `.env`:

| Variable | Set it to |
|---|---|
| `MCHARBOR_VERSION` | The release you are installing. There is no default, on purpose: an unpinned tag upgrades the day someone pulls |
| `MCHARBOR_SECRET` | `openssl rand -base64 48` |
| `ENCRYPTION_KEY` | `openssl rand -base64 32`; must decode to exactly 32 bytes |
| `DOCKER_GID` | The group id printed above |
| `ALLOWED_ORIGINS` | The URL users open McHarbor from, no trailing slash |
| `PUID` / `PGID` | The user McHarbor runs as. `1000:1000` unless that collides with something on the host; the `chown` above must match |

`docker compose config -q` checks the file and fails with the message of any required variable
that is still empty. Then:

```bash
docker compose up -d
docker compose ps           # mcharbor becomes healthy after its first start
```

### What the file does

| Setting | Effect |
|---|---|
| `image: …:${MCHARBOR_VERSION}` | A pinned tag; upgrades happen when you change it |
| `mcharbor-init` | A one-shot container that `chown`s the data volume to `PUID:PGID` and exits. It runs as root with `CHOWN` and `DAC_READ_SEARCH` only, no network, and `mcharbor` waits for it to finish |
| `user: "${PUID}:${PGID}"` | McHarbor runs as a non-root user. The image defaults to root; this was checked to start, migrate and serve as UID 1000 |
| `volume: { nocopy: true }` on the data volume | Stops Docker from copying the image's root-owned `/app/data` over the volume's ownership when the volume is still empty, which would undo `mcharbor-init` |
| `group_add: ${DOCKER_GID}` | Lets the non-root user open the Docker socket |
| `read_only: true`, `tmpfs: /tmp` | The root filesystem is read-only; McHarbor writes to `/app/data` and `/tmp` |
| `cap_drop: ALL` | No Linux capabilities. Port 8705 needs none |
| `security_opt: no-new-privileges:true` | No process in the container can gain privileges through a setuid binary |
| `deploy.resources.limits` | 2 CPUs, 2 GiB, 2048 processes by default; `MCHARBOR_CPUS` and `MCHARBOR_MEMORY` override the first two |
| `logging` | `json-file`, 10 MB × 5 files per container |
| `healthcheck` | `GET /api/livez`, HTTPS then HTTP. Liveness, not readiness: see [Deployment](./README.md#what-every-topology-has-in-common) |
| `secrets: mcharbor_backup_key` | The backup key file, mounted at `/run/secrets/mcharbor_backup_key` |
| `restart: unless-stopped` | |

### The Docker socket

The socket is mounted `:ro`. That stops the container from replacing or deleting the socket
file; it does **not** make the Docker API read-only. Anything that can open the socket can
start a privileged container with the host's root filesystem mounted, which makes every McHarbor
administrator root on this host. The non-root user, the dropped capabilities and the read-only
root filesystem protect against a compromise of the McHarbor process that does not go through
the socket; they do not change that.

To manage Docker on other hosts, install the agent there instead of exposing their daemons;
see [Agents](../../agents/README.md). A McHarbor that manages only remote hosts can drop the
socket mount and the `group_add` line altogether.

### Secrets on disk

Compose has no way to hand `MCHARBOR_SECRET` or `ENCRYPTION_KEY` to McHarbor except as
environment variables, so they sit in `.env`. Keep that file `0600`, owned by the account that
runs `docker compose`, and out of version control. The backup key is a file-based Compose
secret: a bind mount of the host file, so the host file's own owner and mode apply, which is
why it is `chown`ed to `PUID` above.

`docker inspect` shows environment variables to anyone who can reach the Docker API, which on
this host is already everyone who is root.

### Extra CA certificates

With a read-only root filesystem the image's entrypoint cannot add certificates to the system
bundle. The file mounts `./ca` at `/etc/mcharbor/ca` and sets
`SSL_CERT_DIR=/etc/ssl/certs:/etc/mcharbor/ca`, so McHarbor's own outbound TLS (a private
registry, a licence server, PostgreSQL behind a private CA) trusts any PEM certificate placed in
`./ca`. `curl` inside the container does not read it that way. Restart after adding one.

### TLS

The file keeps `TLS_ENABLED=true`: McHarbor serves HTTPS on 8705 with its own root CA, and
[Installing the root CA](../../screens/security/install-root-ca.md) makes browsers trust it.
Behind a reverse proxy, set `MCHARBOR_BIND_ADDRESS=127.0.0.1` so the port is reachable through
the proxy only, and see [Reverse proxy](./reverse-proxy.md).

### If something needs root

A plugin that writes outside `/app/data` and `/tmp` fails on a read-only root filesystem; remove
`read_only: true` first. If something still needs root, remove `user:` and `group_add:`, and
remove `mcharbor-init` together with the `depends_on` entry that waits for it. The image then
runs as it does in the root `docker-compose.yml`.

## External database

`deploy/compose/external-database/docker-compose.yml` is the single-node file with
`MCHARBOR_DB_DRIVER=postgres` and a `postgres:17-alpine` service beside it. Prepare it as above,
then add the two secret files the database needs: the PostgreSQL password, and McHarbor's
connection string, which carries that same password.

```bash
openssl rand -hex 24 | tr -d '\n' > secrets/postgres_password
printf 'postgres://mcharbor:%s@postgres:5432/mcharbor?sslmode=disable' "$(cat secrets/postgres_password)" > secrets/mcharbor_db_dsn
chown 1000:1000 secrets/mcharbor_db_dsn
chmod 0400 secrets/postgres_password secrets/mcharbor_db_dsn
```

The PostgreSQL service reads `POSTGRES_PASSWORD_FILE=/run/secrets/postgres_password` and
McHarbor reads `MCHARBOR_DB_DSN_FILE=/run/secrets/mcharbor_db_dsn`, so neither password is in
any container's `docker inspect` output. **The two files must agree**: the password inside
`secrets/mcharbor_db_dsn` is the one in `secrets/postgres_password`. PostgreSQL applies its
password only when it first initialises an empty data volume; McHarbor reads the connection
string on every start. Editing `secrets/postgres_password` later therefore does not change the
database's password; it only makes the two disagree.

`sslmode=disable` is acceptable only because the database is on the Compose project's private
network and publishes no port. The password is inserted into a URL: if it contains `@`, `:`,
`/`, `?`, `#` or `%`, URL-encode it in the connection string, or generate one without them
(`openssl rand -hex 24`).

**Moving an existing deployment to this file.** The data volume already holds a database
initialised with some password. Write that **current** password into
`secrets/postgres_password` and build `secrets/mcharbor_db_dsn` from it, as above, before you
run `docker compose up -d`. A different password breaks nothing inside the volume (PostgreSQL
ignores the file once the database exists), but McHarbor then connects with a password the
database does not have and stops at start-up. Remove `POSTGRES_PASSWORD` and `MCHARBOR_DB_DSN`
from `.env`: the file no longer reads them.

To use a PostgreSQL server that is **not** in this file, write its full libpq URL (with
`sslmode=verify-full` and the server's CA in `./ca` where it presents a certificate) into
`secrets/mcharbor_db_dsn`, then delete the `postgres` service, its `depends_on` entry, its
volume and the `postgres_password` secret.

The PostgreSQL container runs with `no-new-privileges` and every capability dropped except the
five its entrypoint needs to initialise the data directory and switch to the `postgres` user:
`CHOWN`, `DAC_OVERRIDE`, `FOWNER`, `SETGID`, `SETUID`.

Setting `MCHARBOR_DB_DRIVER` pins the database: **Settings → System → Database** shows the
connection as fixed by the environment and does not offer to change it. To move an existing
SQLite install onto this database, see [Database](../../settings/system/database/README.md).

`MCHARBOR_PLUGIN_STORE` is empty here, so plugin packages stay in the node's data volume. Set it
to `database` before you add a second node; see
[High availability](../../settings/system/high-availability/README.md#plugin-packages).

## Active/passive

`docker-compose.ha.yml` and `ha.env.example` in the repository root run two McHarbor nodes and a
PostgreSQL on one Docker host, with every shared value the nodes need:

```bash
cp ha.env.example ha.env           # replace every placeholder
docker compose --env-file ha.env -f docker-compose.ha.yml up -d
```

The secret, the encryption key, the database connection string and the backup key reach the
nodes as Docker secrets read from `./secrets/`, and the bundled PostgreSQL reads its password
from one too (`POSTGRES_PASSWORD_FILE`). Create `secrets/mcharbor_secret`,
`secrets/mcharbor_encryption_key`, `secrets/postgres_password` and `secrets/mcharbor_db_dsn`
before the first `up`, as [Shared keys as Docker secrets](../../settings/system/high-availability/README.md#shared-keys-as-docker-secrets)
shows. The password in `mcharbor_db_dsn` must be the one in `postgres_password`.

One host is a template for validating the setup, not high availability. In production run each
node on its own host with the same secret, encryption key and backup key files,
`MCHARBOR_HA_MODE` and `MCHARBOR_PLUGIN_STORE`, a different `MCHARBOR_NODE_ID`, and the same
external PostgreSQL. Put a load balancer that checks `/api/readyz` in front of them.
[Reverse proxy](./reverse-proxy.md) has one for each of Caddy, Traefik and NGINX.

The hardening in the single-node file (non-root user, `nocopy`, read-only root filesystem,
dropped capabilities, resource and log limits) applies to each HA node the same way; copy those
keys onto `mcharbor-node-1` and `mcharbor-node-2`.

How the nodes pick a role, what a standby refuses, and the failover controls are on
[High availability](../../settings/system/high-availability/README.md).

## Related

- [Deployment](./README.md)
- [Reverse proxy](./reverse-proxy.md)
- [Configuration](../configuration/README.md)
- [Upgrading](../upgrading/README.md)
