# First Run

What happens the first time the container starts, and the five things to do before McHarbor
is genuinely in service.

## What the container does before you see anything

1. The entrypoint creates `$DATA_DIR` and `$DATA_DIR/tls`, and `chown`s the data directory
   to `PUID:PGID`.
2. With `MCHARBOR_SECRET` unset, it generates one into `$DATA_DIR/.secret`. Every later
   start reuses that file, so sessions survive restarts.
3. It reports whether the Docker socket is present, and the socket's group id.
4. It refreshes any bundled plugin payloads whose install directory already exists,
   which matters on upgrade and is a no-op on a fresh install.
5. McHarbor opens its database and applies migrations, generates a local root certificate
   authority and the server certificate it signs when TLS is on and none exists, and starts
   the listeners. Both fingerprints go into the log.

At that point `/api/health` answers and the login page renders.

While no account exists, McHarbor also generates a **setup token** and prints it once, at
warning level, in its log. Read it now; the installer asks for it:

```bash
# Container
docker logs mcharbor 2>&1 | grep 'setup token'

# Native install (deb, rpm or tarball)
journalctl -u mcharbor | grep 'setup token' | tail -n 1
```

A native install does not need the second command the first time: `install.sh`, and the
`install-debian.sh` / `install-fedora.sh` wrappers, read the token from the journal once the
service has started and print it at the end of their output (`Setup token: …`). When no
token is printed, the instance already has an account and none is needed.

The log line is `first-run setup token: enter it in the installer to set up this instance`, with
the token beside it. No token is generated once an account exists.

## The setup flow

Opening `/` with no account yet sends you into setup. There are two possible first steps:

Either way the page asks for the [setup token](#the-setup-token).

| Situation | Where you land | Why |
|---|---|---|
| No database configured, and no `MCHARBOR_DB_DRIVER` in the environment | `/setup/database` | The database question comes **before** the first admin account, because that account is the first row McHarbor writes. Asking afterwards would mean discarding it or copying it |
| The environment pins the driver, or a database is already configured | `/setup` | There is nothing to choose |

### The setup token

Before the first account exists, the three routes the installer uses (the database connection
test, the database apply and the admin account) answer nobody, because there is no login to
protect them. The setup token is what stands in for one. Both installer pages have a **Setup
token** field; paste the token from the log (or from the native installer's output) into it, and the installer sends it as the
`X-McHarbor-Setup-Token` header on those calls.

- **It changes on every restart.** A new token is generated each time McHarbor starts while no
  account exists. Applying a database other than the one already running (PostgreSQL, MySQL or SQL
  Server on a fresh SQLite install) restarts McHarbor, so after **Apply** copy the token again
  from the new log output before creating the admin account. Choosing SQLite does not restart
  it, and the token stays valid.
- **A missing or stale token is refused** with a message that says to copy the current one from
  the McHarbor log. Nothing is created and nothing is dialled.
- **It is not stored.** The page keeps it for the browser session only, and it is not a
  credential once the admin account exists.
- **Setup is serialised.** The database apply and the admin account cannot run at the same time,
  so two people opening the installer cannot both win.

### Choosing a database

SQLite is the default and needs no input: one file inside the data directory, with no server
to run and no credentials to keep. It is **single node only**. A SQLite path typed into the form
must resolve inside the data directory (`DATA_DIR`); any other path is refused, because a
connection test creates the file it names.

PostgreSQL 12+ or MySQL 8 / MariaDB 10.6+ are the alternatives, and the reason to pick one
is active-active multi-node. The form takes host, port, database, user, password and an SSL
mode, or a raw connection string for a managed database whose options the form does not
model. Credentials are encrypted with the McHarbor secret key and stored in the data
directory, never in an environment variable, and never sent back to the page.

**Test connection** runs a short sequence of named checks (details complete, host
reachable, credentials accepted) and tells you which step failed rather than returning one
opaque error.

### Creating the admin account

`/setup` asks for the setup token, a username, an optional email address, and a password of at
least eight characters. This account is created with full access.

Give it an email address even though it is optional: password reset needs somewhere to send
the link.

## After the first login

### 1. Create the first environment

**McHarbor creates no environment for you.** A fresh install has an empty Environments page,
and that is deliberate: the container may see a Docker socket it is not meant to manage, and
inventing an environment for it decides on your behalf which host McHarbor governs.

Open [Environments](../../screens/environments/README.md) → **Add environment** and create
the one you want. For the daemon on this host, pick **Socket** and give it the path the
socket is mounted at, usually `/var/run/docker.sock`. It should report a Docker version
straight away; if it does not, the socket is either not mounted or not readable by
`PUID:PGID`.

Until an environment exists, container, stack, image, volume and network pages have nothing
to show, and no metrics are collected.

### 2. Add the hosts that are not this one

| Kind | Use when | Needs |
|---|---|---|
| Local socket | The daemon on this host | The socket mount |
| Remote TCP / TLS | A daemon that exposes a reachable port | The CA, client certificate and key for TLS |
| Agent | Anything behind NAT, or a host whose socket you will not expose | The agent on that host, dialling out |

Creating an agent environment produces a token. It is shown **once** and redacted from every
later response. Copy it then, or regenerate it later.

### 3. Set the time zone

[Settings → General](../../settings/general/general/README.md) sets the time zone used by
cron schedules, run timestamps and every other server-side time calculation. Schedules will
not behave the way you expect until this matches the one you think in.

### 4. Deal with the certificate

The certificate McHarbor generated is signed by a root that only this install knows about, so
browsers complain until they are told about that root. Fix it once rather than clicking
through the warning every time:

**Settings → Security → HTTPS → Download root CA**, then install `mcharbor-root-ca.pem` on
each machine and device that will open McHarbor. The click paths for macOS, Windows, Linux,
Firefox, Android and iOS are in
[Installing the root CA](../../screens/security/install-root-ca.md). Trust survives every
later rotation, because the root is what you installed and the root is what McHarbor keeps.

The alternatives are unchanged: upload a publicly-trusted certificate under
[Security → HTTPS](../../screens/security/https.md), terminate TLS at a proxy in front, or
accept the warning knowingly on a private network.

### 5. Decide what backups mean here

Nothing is backed up until you say so. The order that works:

1. Generate or install the backup encryption key:
   [Settings → Integrations → Storage](../../settings/integrations/storage/README.md).
2. Add a storage destination. Local paths work with no plugin; anything remote needs a
   [storage plugin](../../plugins/storage/README.md).
3. Create a backup plan against the containers that matter, and run it once by hand to see
   it work end to end.

## Worth doing early, if the install is more than personal

- **Create roles before creating users.** A user with no role is easier to place than a user
  whose role has to be taken away.
- **Wire one notification channel.** An alert with nowhere to go is not an alert; see
  [Settings → Integrations → Communications](../../settings/integrations/communications/README.md).
- **Turn on an audit forwarder** if audit data has to survive the install itself.
- **Check retention**, under
  [Settings → Monitoring → Data retention](../../settings/monitoring/data-retention/README.md).
  The defaults are chosen to keep the database small, not to satisfy an auditor.

## In this folder

| Page | What it covers |
|---|---|
| [Troubleshooting](./troubleshooting.md) | When the first login will not work |

## Related

- [Installation](../installation/README.md): Compose, ports, volumes, TLS
- [Configuration](../configuration/README.md): every environment variable
- [Screens](../../screens/README.md): what each part of the interface does
- [Settings](../../settings/README.md): every setting, section by section
