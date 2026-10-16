# Configuration

Every environment variable McHarbor reads, with its default and what it actually changes.
Values are read **once, at start-up**; changing one means recreating the container.

Most of what an operator configures day to day is not here. It lives in the database and is
edited in the interface, under [Settings](../../settings/README.md). Environment variables
are for the things that have to be true before the database is open: listeners, TLS, the
database itself, and the secrets that encrypt everything else.

## Application

| Variable | Default | What it does |
|---|---|---|
| `PORT` | `8705` | Port for the main listener |
| `HOST` | `0.0.0.0` | Bind address for the main listener. Also feeds the generated certificate's SANs when it names one interface rather than all of them |
| `DATA_DIR` | `./data` | Root of everything McHarbor owns. `/app/data` in the container |
| `TZ` | `UTC` | IANA time zone for the container. The Debian base image honours it through `/etc/localtime` |
| `PUID` / `PGID` | `1000` / `1000` | Read by the entrypoint, which `chown`s `DATA_DIR` to them |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_JSON` | `false` | Structured JSON log lines instead of text |
| `MCHARBOR_NODE_ID` | random 6-char suffix | Names this node in the HA topology, the audit log and leader-election logs. Keep it stable across restarts |
| `MCHARBOR_HA_MODE` | empty | `active-passive` makes exactly one node active on a shared PostgreSQL database; see [High availability](../../settings/system/high-availability/README.md) |
| `MCHARBOR_PLUGIN_STORE` | empty (local) | `database`, `shared-fs:/path` or `object:<storage location id>`: where plugin packages are kept for the other nodes |
| `MCHARBOR_ADVERTISE_URL` | empty | How other nodes and operators reach this node; shown in the HA topology. In every multi-node mode (peer, active/passive, active/active) the other nodes refuse a plain `http://` address unless `MCHARBOR_NODE_LINK_ALLOW_HTTP=1` |
| `MCHARBOR_NODE_LINK_ALLOW_HTTP` | empty | `1` lets this node send node-to-node calls to a peer that does not advertise `https://` and accept node-to-node calls that arrive without TLS, in any multi-node mode, for a trusted private network; see [Active/active](../../settings/system/high-availability/active-active.md#how-the-hops-are-protected) |
| `MCHARBOR_NODE_LINK_CA_FILE` | empty | Path to the PEM certificate of the private CA that signs the nodes' certificates; both node-to-node hops verify peers against it alone. Recommended over `MCHARBOR_NODE_LINK_SKIP_VERIFY`, which it overrides |
| `MCHARBOR_NODE_LINK_SKIP_VERIFY` | empty | `1` accepts any peer certificate on the node-to-node hops. Whoever can intercept the path can then forge streamed bodies, agent replies and relayed sessions; the HA readiness page warns |
| `MCHARBOR_NODE_LINK_RATE_LIMIT` | `3000` | Requests per minute, per verified peer node, each internal agent hop accepts. Requests that fail verification have a separate fixed budget of 120 a minute per source address |
| `MCHARBOR_RESET_KEY_CHECK` | empty | `1` accepts a deliberate `ENCRYPTION_KEY` change once; a node whose key does not match the database otherwise refuses to start on a shared database |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | Base URL of an OTLP/HTTP collector. Setting it turns trace export on; empty means no spans are created. See [Tracing](../../observability/tracing.md) |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | empty | Traces-only OTLP/HTTP URL; wins over the base URL and also turns tracing on |
| `MCHARBOR_RECONCILE_INTERVAL` | `60s` | How often the background drift loop checks every desired state. Go duration syntax (`30s`, `5m`). `0` turns the loop off and leaves drift checks to `GET /api/reconciler/{id}/drift` |
| `MCHARBOR_PUSH_CONTACT` | `https://www.mcharbor.eu` | Contact the Web Push services are given for this install (the VAPID subject): an e-mail address, `mailto:` added for you, or an `https://` URL. See [Push notifications](../../oncall/push.md) |

## Secrets

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_SECRET` | generated | Signs sessions and node-to-node calls. When unset, McHarbor generates one into `$DATA_DIR/.secret` and reuses it. Changing it invalidates every session. McHarbor removes it (with `ENCRYPTION_KEY`, `MCHARBOR_DB_DSN` and `MCHARBOR_BACKUP_KEY`) from its own Go process environment once the configuration has loaded, so compose and other child processes never inherit it. A value set as a variable stays in the container's `Config.Env` (every `docker inspect` shows it) and in `/proc/1/environ`. Use `MCHARBOR_SECRET_FILE` to keep it out of both. A value of only whitespace counts as unset |
| `MCHARBOR_SECRET_FILE` | — | Path to a file holding the secret, normally a Docker secret such as `/run/secrets/mcharbor_secret`. Surrounding whitespace is trimmed. McHarbor refuses to start when `MCHARBOR_SECRET` is also set, or when the file is missing, unreadable or empty |
| `ENCRYPTION_KEY` | — | Key for the AES-256-GCM encryption of credentials at rest: registry passwords, agent tokens, plugin configuration, identity secrets. Credentials a previous version stored in plaintext are encrypted in place at the first start that has a key (see [Upgrading](../upgrading/README.md#credentials-are-encrypted-on-the-first-start)). Set as a variable, it is visible in the container's `Config.Env` like `MCHARBOR_SECRET` |
| `ENCRYPTION_KEY_FILE` | — | Path to a file holding the key, normally `/run/secrets/mcharbor_encryption_key`. Same rules as `MCHARBOR_SECRET_FILE`: never together with `ENCRYPTION_KEY`, and a missing, unreadable or empty file stops the start |
| `BACKUP_ENCRYPTION_KEY_FILE` | `/run/secrets/mcharbor_backup_key` | File holding the backup encryption key, as a Docker secret |
| `MCHARBOR_BACKUP_KEY` | — | The same key inline, for deployments without Docker secrets |

Keep `MCHARBOR_SECRET`, `ENCRYPTION_KEY` and the backup key wherever you keep your other
production secrets. A restored data volume without them is a database of values nothing can
read.

Compose runs (stack deploys and the workflow deploy node) get an allow-list of McHarbor's
environment rather than all of it: `PATH`, `HOME`, `USER`, `TMPDIR`, locale and time zone, the
proxy variables, `SSL_CERT_*`, `SSH_AUTH_SOCK`, `XDG_*`, the `DOCKER_*`, `COMPOSE_*` and
`BUILDKIT_*` families, and the Windows system variables, plus the stack's own variables. A
compose file cannot interpolate McHarbor's keys or other settings, and compose output is
scrubbed of the keys McHarbor runs with.

Prefer the `_FILE` forms whenever you pin these values. Anyone who can read a container's
configuration can read a variable: McHarbor withholds `MCHARBOR_SECRET`, `ENCRYPTION_KEY`,
`MCHARBOR_DB_DSN` and `MCHARBOR_BACKUP_KEY` from its own container's inspect view for every
user (see [Containers → Environment values](../../screens/containers/fields.md#environment-values)),
but the Docker daemon, `docker inspect` on the host and any tool with socket access still see
them. A Docker secret is a file mounted into the container, so it appears in none of those.

## Database

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_DB_DRIVER` | `sqlite` | `sqlite`, `postgres`, `mysql` (MySQL 8+ and MariaDB 10.6+) or `mssql` (SQL Server 2022+). Any other value stops the start; see below |
| `DATABASE_PATH` | `./data/mcharbor.db` | SQLite file path. The entrypoint defaults it to `$DATA_DIR/mcharbor.db` |
| `MCHARBOR_DB_DSN` | — | Connection string for the external drivers. Postgres takes a libpq URL; MySQL takes a go-sql-driver DSN; SQL Server takes a `sqlserver://` URL naming the database. Set as a variable, it (password included) is part of the container's `Config.Env` |
| `MCHARBOR_DB_DSN_FILE` | — | Path to a file holding the connection string, normally a Docker secret such as `/run/secrets/mcharbor_db_dsn`. Surrounding whitespace is trimmed. McHarbor refuses to start when `MCHARBOR_DB_DSN` is also set, or when the file is missing, unreadable or empty |

Setting `MCHARBOR_DB_DRIVER` **pins** the database: the Database screen will say so and
refuse to manage the connection, because anything chosen there would be ignored at the next
start.

The value is read case-insensitively and accepts a few spellings: `sqlite` or `sqlite3`;
`postgres`, `postgresql` or `pgsql`; `mysql` or `mariadb`; `mssql`, `sqlserver` or `azuresql`.
Anything else stops McHarbor at startup with
`failed to resolve database configuration … MCHARBOR_DB_DRIVER="<value>" is not a supported database driver`,
rather than starting it on the SQLite file. An empty value means SQLite.

## Docker

| Variable | Default | What it does |
|---|---|---|
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | The local daemon. A `tcp://` endpoint works for a remote one |
| `DOCKER_TLS_VERIFY` | — | Verify the daemon's TLS certificate on a TCP endpoint |
| `DOCKER_CERT_PATH` | — | Directory holding `ca.pem`, `cert.pem` and `key.pem` |

## TLS and HTTP listeners

| Variable | Default | What it does |
|---|---|---|
| `TLS_ENABLED` | `true` | Serve the main listener over TLS. A local root CA and the server certificate it signs are generated on first boot when none exists |
| `TLS_MIN_VERSION` | `772` (TLS 1.3, preferred) | The floor the listener accepts. TLS 1.3 is what McHarbor ships and what you should stay on; **TLS 1.2 is the minimum still supported**, for clients that predate 2019. Accepts `1.3`/`13`/`0x0304`/`772` and `1.2`/`12`/`0x0303`/`771`. TLS 1.1 and below are refused at startup. See [TLS versions](../installation/README.md#tls-versions) |
| `TLS_FORCE_HTTPS` | `true` | Redirect HTTP to HTTPS based on `X-Forwarded-Proto` |
| `HSTS_MAX_AGE` | `0` | HSTS max-age in seconds. Off by default, because turning it on before the certificate works can lock you out of the recovery listener |
| `CONTENT_SECURITY_POLICY` | _(built-in policy)_ | The Content-Security-Policy sent on everything McHarbor serves. Unset uses the default; **set but empty sends no CSP at all**. See [Content-Security-Policy](../deployment/content-security-policy/README.md) |
| `CONTENT_SECURITY_POLICY_REPORT_ONLY` | `false` | Send the policy as `Content-Security-Policy-Report-Only`, which reports violations in the browser console without blocking them. Use it to trial a policy across your plugins |
| `HTTP_REDIRECT_ADDR` | _(empty)_ | Address for a redirect-only listener that 308s everything to HTTPS |
| `HTTP_LISTEN_ADDR` | _(empty)_ | Address for an optional second main listener serving the full app over plain HTTP, e.g. `:8705` |
| `SELF_SIGNED_SAN_EXTRA` | _(empty)_ | Comma-separated extra names for the generated certificate: a DNS name you invented for the host, an address it moved to. Entries that parse as an IP address are added as IP SANs, the rest as DNS names. Press **Regenerate** afterwards; the list is fixed at issue time |
| `FORCE_SECURE_COOKIES` | `false` | Mark cookies `Secure` even when McHarbor itself was reached over HTTP |

The root McHarbor generates here is the one **clients** install to trust this host; see
[Installing the root CA](../../screens/security/install-root-ca.md). It is unrelated to
`MCHARBOR_LOCAL_CA_PATH` below, which is about roots McHarbor itself trusts when calling out.

## Network trust

| Variable | Default | What it does |
|---|---|---|
| `ALLOWED_ORIGINS` | _(empty)_ | Comma-separated origins the API accepts, no trailing slash. The built-in fallback covers `localhost` variants only |
| `MCHARBOR_TRUSTED_PROXY_CIDRS` | _(empty)_ | CIDRs or bare addresses whose forwarding headers may be believed when resolving a client address: login throttling, the request log, audit rows, password-reset records, API-key usage and a policy's `request.ip` |

`ALLOWED_ORIGINS` is the first thing to check when a login form does nothing.

`MCHARBOR_TRUSTED_PROXY_CIDRS` empty means "trust nothing": every forwarding header is
ignored and the socket peer is recorded as the client. **Never `0.0.0.0/0`**: that trusts
every caller's claim about its own identity. A malformed value fails start-up rather than
silently degrading.

### Trusting a private CA on the way out

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_LOCAL_CA_PATH` | `./mnt/certs` | Host directory of extra `*.crt` roots, mounted read-only at `/usr/local/share/ca-certificates` |

The two settings above govern what McHarbor believes about callers coming *in*. This one
governs what it trusts calling *out*: a licence server on an internal host, a registry
behind a corporate CA, a homelab service with a self-signed certificate.

Drop the root in as a `.crt` file and restart. The entrypoint runs
`update-ca-certificates`, and from then on Go's `crypto/x509`, `curl`, and everything else
in the container trust the same bundle. An empty directory is the normal case: the
entrypoint finds no `*.crt` and skips the refresh, so nothing changes for an install that
does not opt in.

Without this, an untrusted chain fails outright:

```
tls: failed to verify certificate: x509: certificate signed by unknown authority
```

Go has no click-through for that the way a browser does. It either chains to a trusted
root or the request fails. Adding the root is the fix; there is deliberately no option to
skip verification, which would leave an unauthenticated handshake on the wire permanently
to work around one certificate.

Two details that bite: the extension must be `.crt` (`.pem` is ignored), and the contents
must be PEM; convert DER with `openssl x509 -inform der -in cert.der -out cert.crt`. Never
put a private key in the directory; the trust store needs the certificate alone. See
`mnt/certs/README.md` for capturing a self-signed certificate from the host that serves it.

## Agent transport

| Variable | Default | What it does |
|---|---|---|
| `TRANSFER_PORT` | `:8788` | Port used for file transfer with agents |
| `FORCE_AGENT_TLS` | `true` | The install script emits `MCHARBOR_FORCE_TLS=1`, so the agent dials `wss://` |

## App Store and plugins

| Variable | Default | What it does |
|---|---|---|
| `APPSTORE_CATALOG_URL` | _(empty)_ | Remote catalog URL |
| `APPSTORE_SYNC_CRON` | _(empty)_ | Cron expression for automatic catalog refresh |
| `MCHARBOR_PLUGIN_CATALOG_DIRS` | _(empty)_ | Comma-separated catalog directories, e.g. `/mnt/plugin-catalog`. Drives the plugin-icon fallback when a plugin's own icon cannot be resolved |
| `MCHARBOR_PLUGIN_TRUST_KEYS` | _(empty)_ | PEM file of ed25519 public keys the plugin installer trusts. With none configured the installer warns and proceeds |
| `MCHARBOR_PLUGIN_RELAXED_SCHEMA` | unset | Loosens manifest schema validation. For plugin development |
| `MCHARBOR_AUTO_APPLY_PLUGIN_MIGRATIONS` | unset | Applies a plugin's pending migrations automatically |
| `MCHARBOR_PLUGIN_CATALOG_ROOT` | native: `/opt/mcharbor` | Native install only. Root the binary re-syncs bundled plugin payloads from on upgrade. It is **distinct** from `MCHARBOR_PLUGIN_CATALOG_DIRS` above, which is about the icon fallback, not the payload root |
| `MCHARBOR_THEME_CATALOG_ROOT` | native: `/opt/mcharbor` | Native install only. Root the binary reads bundled theme payloads from |
| `MCHARBOR_DEBUG_BIN_DIR` | `debug/` beside the binary (`/app/debug` in the image, `/opt/mcharbor/debug` native) | Directory holding `busybox-amd64` and `busybox-arm64` for the container [debug shell](../../screens/containers/debug-shell/README.md). Set it only for a development build run with `go run`, which has no `debug/` directory |

## Native install

These apply only outside the container: a `.deb`, `.rpm` or tarball install
running under systemd. See [Native Linux install](../installation/native-linux.md) for the
full picture: supported distributions, the filesystem layout, upgrading and uninstalling.

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_INSTALL_KIND` | native: `deb`, `rpm` or `tarball` | Set automatically by the package's postinstall script; **do not edit by hand**. Tells the running binary which install format it is, for diagnostics and update logic. The tarball's shipped sample env file shows `tarball` only because `install.sh` has not run yet to overwrite it; an installed system always reports the format it was actually installed from |

The three variables above are read from `/etc/mcharbor/mcharbor.env` on a native install,
the conffile equivalent of the container's environment block. Edits there survive a
package upgrade the same way `DATA_DIR` does on the container.

## Authentication

| Variable | Default | What it does |
|---|---|---|
| `AUTH_DISABLE` | `false` | Disables authentication entirely |

`AUTH_DISABLE=true` means anyone who can reach the port is an administrator of every
container McHarbor can see, and by extension of the Docker host. It exists for development.

## Version reporting

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_VERSION` | from `VERSION` | Overrides the reported version |
| `MCHARBOR_VERSION_FILE` | `./VERSION` | Path to the version file |

## One variable that does not exist

The shipped `docker-compose.yml` carries a commented-out `AUDIT_LOG_RETENTION_DAYS`,
described there as a recovery path for a misconfigured retention policy. **Nothing in the
code reads it.** Audit retention is set only in
[Settings → Monitoring → Data retention](../../settings/monitoring/data-retention/README.md).

## The agent's own variables

The agent is a separate binary with its own environment.

| Variable | Default | What it does |
|---|---|---|
| `MCHARBOR_URL` | **required** | The McHarbor instance to dial |
| `MCHARBOR_AGENT_TOKEN` | **required** | The environment's agent token. Shown once, at creation |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | The daemon the agent manages |
| `LOG_LEVEL` | `info` | Agent log level |
| `MCHARBOR_INSECURE` | `false` | Skip TLS verification when dialling McHarbor |
| `MCHARBOR_FORCE_TLS` | `false` | Promote `ws://` to `wss://` and `http://` to `https://` |
| `MCHARBOR_DATA_DIR` | `/var/lib/mcharbor-agent` | Agent state, including the offline buffer. Mount it or the buffer survives restarts only |
| `MCHARBOR_OFFLINE_BUFFER` | `true` | Buffer events, stats and the agent log while McHarbor is unreachable, and replay on reconnect |
| `MCHARBOR_OFFLINE_METRICS_INTERVAL` | `30` | Stats sampling cadence in seconds while offline |
| `MCHARBOR_TRANSFER_LISTEN` | `0.0.0.0:8788` | Where the direct agent-to-agent transfer receiver binds. Empty forces every transfer through the relay |
| `MCHARBOR_TRANSFER_ADVERTISE_URL` | _(empty)_ | Overrides address discovery behind port forwarding or a proxy |
| `MCHARBOR_COMPOSE_DIR` | `/var/lib/mcharbor-agent/compose` | Where the agent keeps Compose project files |
| `MCHARBOR_LICENSE_KEY` | _(empty)_ | Ignored on an agent, with a warning in its log: an agent does not claim a seat, its environment is counted by McHarbor |
| `MCHARBOR_RETIRE_CONTAINER_ID` | _(empty)_ | Used by the agent self-update flow to retire the previous container |

## Related

- [Installation](../installation/README.md): the Compose file these variables go in
- [Settings](../../settings/README.md): everything configured in the interface instead
- [First run](../first-run/README.md): what happens the first time the container starts
