# Prerequisites

What has to be true before McHarbor will start, and what each requirement is actually for.

## The host

| Requirement | Notes |
|---|---|
| Ubuntu Server 24.04.5 LTS or 26.04.1 LTS | The preferred distribution. Any host that runs Docker Engine works; these two are what McHarbor is developed and tested against |
| Docker Engine 20.10 or newer | With the Compose v2 plugin, which stacks and encrypted backups both use end to end. [Installing Docker Engine](../installation/docker-engine.md) has the procedure for every supported distribution |
| Access to a Docker socket | `/var/run/docker.sock` on Linux, the Docker Desktop socket on macOS and Windows |
| x86-64 or arm64 | The runtime image is Debian-based and published for both |
| One writable volume | About 200 MB is plenty to start. Metrics history, installed plugins and the vulnerability databases grow it; backup archives normally go somewhere else entirely |
| One free TCP port | 8705 for HTTPS in the shipped mapping |

McHarbor itself runs as a container. There is nothing to install on the host beyond Docker.

"Preferred" is not a gate. McHarbor is a container and will run on any host with a working
Docker Engine: Debian, RHEL, Fedora, SUSE or Arch. Alpine is not supported.
Ubuntu Server 24.04.5 LTS and 26.04.1 LTS are named because they are the combination every
release is verified on, they ship cgroup v2 as the unified hierarchy, and their package names
are the ones the documentation's commands assume.

Running **virtual machines or system containers** on a host is a separate question with a
stricter answer; see
[Supported hosts and guests](../../compute-foundation/supported-platforms.md).

## Access to the Docker socket

McHarbor manages the local Docker daemon through `/var/run/docker.sock`, mounted into the
container. The shipped Compose file mounts it with `:ro`:

```yaml
- /var/run/docker.sock:/var/run/docker.sock:ro
```

Do not read more into `:ro` than it gives you. The flag applies to the bind mount, not to
the Docker API: it stops the socket file itself being replaced, and stops nothing else. A
process that can connect to the socket can still start, stop, delete and create containers,
read-only mount or not.

So there is only one thing to understand before mounting it: **access to the Docker socket
is equivalent to root on the host.** Anything that can talk to it can start a privileged
container that mounts the host filesystem. RBAC inside McHarbor governs who may ask McHarbor
to do something; it does not reduce what the McHarbor process itself is able to do.

If you do not want to give McHarbor the local socket at all, you do not have to; see
**Remote hosts** below.

### The native install has the same equivalence, through a group instead of a mount

McHarbor also installs directly on Linux, outside a container; see
[Native Linux install](../installation/native-linux.md). There the service runs as root by
default, and with `--no-run-as-root` as the `mcharbor` service account, which is added to the
host's **`docker` group** so it can reach `/var/run/docker.sock` directly. The same fact
above applies either way: **on any host running a Docker daemon, access to the socket is
root-equivalent.** The sandbox `--no-run-as-root` adds (`ProtectSystem=strict`,
`ProtectHome`, a `ReadWritePaths` limited to `/var/lib/mcharbor`) narrows what a bug
**inside** McHarbor can reach elsewhere on the filesystem; it is not a boundary against the
McHarbor process itself, which holds that root-equivalent credential by design, same as the
container's socket mount does.

## Ports

The shipped Compose file publishes one:

| Host port | Container port | What it is |
|---|---|---|
| `8705` | `8705` | The HTTPS listener. TLS is on by default; a local root CA and the certificate it signs are generated on first boot |

A third port, `8788`, belongs to the **agent** container, not to this one. It is the inbound
file-transfer listener the host pushes to.

## Browser

A current version of Chrome, Edge, Firefox or Safari. The interface is a single-page React
application and uses WebSockets for the terminal, container logs and live events, so a proxy
in front of McHarbor has to allow WebSocket upgrades.

## Optional, depending on what you turn on

| If you want | You also need |
|---|---|
| To manage a host that is not this one | Either a reachable Docker TCP endpoint with TLS, or the McHarbor agent on that host |
| Containers behind NAT or a firewall | The agent. It dials **outbound** over WebSocket, so the remote host needs no inbound port and no exposed socket |
| Multi-node McHarbor (active/passive) | PostgreSQL 12+ or MySQL 8 / MariaDB 10.6+. The default SQLite database is single-node only |
| Email, chat or push notifications | A communication-channel plugin and credentials for it |
| Backups to anywhere but a local path | A storage-destination plugin and credentials for it |
| Image vulnerability scanning | Nothing extra for Trivy or Grype; both binaries ship in the image. Clair needs a Clair v4 server you run |
| Virtual machines or system containers | A Linux host with systemd and the agent installed, plus KVM support for VMs |

## What McHarbor does not require

- **No external database.** SQLite is the default and is a single file in the data volume.
  Postgres and MySQL exist for multi-node, not as a recommendation.
- **No message broker, no Redis, no sidecar.** The scheduler, the metrics collector and the
  reconciler all run inside the one process.
- **No internet access at runtime**, unless you use something that needs it: image pulls,
  the App Store catalog, update checks, vulnerability database refreshes, hosted AI
  providers, and licence validation. An air-gapped install works, with those features off or
  pointed at internal mirrors.
- **No agent on the local host.** The agent is for *other* hosts.

## Before you start, decide two things

**Where the data volume lives.** Everything McHarbor owns (the database, the session
secret, TLS material, installed plugins) is under `/app/data` in the container. Losing that
volume means losing the install. It is a named volume by default; a bind mount is easier to
back up.

**Which URL people will use.** `ALLOWED_ORIGINS` has to contain it. The default list only
covers `localhost` variants, so a browser opening McHarbor at a LAN address gets a failed
preflight and a login that never reaches the server. This is the single most common
first-install problem.

## Related

- [Installation](../installation/README.md): the Compose file, the volumes, the ports
- [Native Linux install](../installation/native-linux.md): installing without a
  container, and the `docker`-group statement in full
- [First run](../first-run/README.md): the setup wizard and the first environment
- [Configuration](../configuration/README.md): every environment variable
