# Installation

McHarbor ships as one container. There is no separate database, cache or queue to stand up.

## Docker Compose

```yaml
services:
  mcharbor:
    image: ghcr.io/therealmcsparrow/mcharbor:latest
    container_name: mcharbor
    restart: unless-stopped
    ports:
      - "8705:8705"   # HTTPS — the main listener
    environment:
      - TZ=Europe/Amsterdam
      - TLS_ENABLED=true
      - ALLOWED_ORIGINS=https://mcharbor.example.com:8705,http://10.0.0.10:8705
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - mcharbor-data:/app/data

volumes:
  mcharbor-data:
```

```bash
docker compose up -d
docker compose logs -f mcharbor
```

Then open `https://<host>:8705` and follow [First run](../first-run/README.md). The installer asks
for a one-time **setup token** that McHarbor prints in its log when it starts, so keep the
`docker compose logs` output at hand or read it back with `docker logs mcharbor`. The token
changes on every restart.

Set `ALLOWED_ORIGINS` to the URL you will actually type in the browser before the first
login. The built-in default only covers `localhost`, and a mismatch fails the CORS preflight,
which looks like a login form that does nothing.

## What the image contains

Built in three stages (Go for the backend, Node for the frontend,
`debian:bookworm-slim` for the runtime), the final image carries:

| In the image | Why |
|---|---|
| The `mcharbor` binary | The whole application: API, scheduler, metrics collector, reconciler, static file server |
| `docker-cli` and `docker-cli-compose` | Compose stack operations shell out to the Compose CLI |
| `sqlite`, `curl`, `bash`, `ca-certificates`, `tzdata` | Maintenance, health checks, and time-zone data for the scheduler |
| `trivy` and `grype` | The two scanner plugins run these binaries in-process rather than calling a service |
| Every bundled plugin `.so` under `/tmp`, and its tarball under `/app/ai-plugin-catalog` | Installed on demand from the App Store, and refreshed at each start |
| The built frontend under `/app/static` | Served by the same process |

`EXPOSE 8705`, and the health check polls `/api/health` on that port every 30 seconds,
trying HTTPS first and falling back to HTTP.

## Ports

| Container port | Set by | What it serves |
|---|---|---|
| `8705` | `PORT` | The single main listener. HTTPS when `TLS_ENABLED=true` (the default), plain HTTP otherwise |
| _(unset)_ | `HTTP_REDIRECT_ADDR` | A redirect-only listener that answers every request with a 308 to HTTPS. Empty disables it |

`HTTP_REDIRECT_ADDR` is worth setting when you run HTTPS only: it gives you somewhere to
land if a certificate upload goes wrong, instead of a dead port.

## Volumes

| Mount | Purpose | Losing it means |
|---|---|---|
| `/app/data` | The database, the session secret, TLS material, installed plugins, agent state | Losing the install |
| `/var/run/docker.sock` | The local Docker daemon | No socket environment can reach it |
| `/mnt/backup` | Default local backup destination | Losing local backup archives |
| `/mnt/app-catalog`, `/mnt/plugin-catalog`, `/mnt/themes-catalog`, `/mnt/plugins` | Read-only catalog mounts the App Store reads from | Those catalog entries disappear from the store |

Everything under `/app/data` is created at first boot by the entrypoint, which also:

- creates `$DATA_DIR` and `$DATA_DIR/tls`,
- generates a session secret into `$DATA_DIR/.secret` when `MCHARBOR_SECRET` is unset, and
  reuses it on every later start,
- `chown`s the data directory to `PUID:PGID` (default `1000:1000`),
- reports the Docker socket's group id if the socket is present,
- refreshes bundled plugin payloads.

A named volume is the default. A bind mount is easier to back up, and the only thing that
matters is that the same directory comes back after a recreate.

## TLS

TLS is on by default, and McHarbor runs its **own certificate authority** to make that work
without configuration. On first boot, with nothing at `/app/data/tls/cert.pem`, it generates
two things:

| File | What it is | Lives for |
|---|---|---|
| `ca.pem` + `ca-key.pem` | A local root. The `.pem` half is public and is the file you distribute | 10 years |
| `cert.pem` + `key.pem` | The served certificate, signed by that root, with the root appended as the chain | 90 days, reissued automatically inside the last 30 |

The served certificate covers `localhost`, the container hostname and `host.docker.internal`
as names, and `127.0.0.1`, `::1` and every address bound to a live non-loopback adapter as IP
addresses, so it is valid when McHarbor is opened from another machine on the network, not
only from the box it runs on. `SELF_SIGNED_SAN_EXTRA` adds more: entries that parse as an IP
address become IP SANs, the rest become names.

The SAN list is fixed at issue time. A host that changed address afterwards keeps the old list
until the certificate is reissued under Security → HTTPS → **Regenerate**, which mints a new
server certificate and **keeps the root**, so machines that already trust it need no action.

Browsers warn until they know the root. Three ways out:

1. **Install the root**, which is the intended one. Security → HTTPS → **Download root CA**,
   then install `mcharbor-root-ca.pem` once per machine or device;
   [Installing the root CA](../../screens/security/install-root-ca.md) has the procedure for
   macOS, Windows, Linux, Firefox, Chrome's NSS store, Android and iOS. It survives every
   later rotation.
2. **Upload a publicly-trusted certificate** under
   [Security → HTTPS](../../screens/security/https.md), or place `cert.pem` and `key.pem` in
   `/app/data/tls/`. A watcher picks up a changed certificate within about 30 seconds, with no
   restart, and an uploaded certificate is never touched by renewal.
3. **Terminate TLS in front**, at a reverse proxy pointing at the plain-HTTP listener. Allow
   WebSocket upgrades, or the terminal, live logs and events will not work. An ACME companion
   from the App Store against a real domain is the same answer with the certificate automated.

`HSTS_MAX_AGE` is `0` (off) on purpose. Turning HSTS on before you have a working
certificate can lock you out of the recovery path, so it is opt-in.

### TLS versions

**TLS 1.3 is preferred, and is what McHarbor ships.** `TLS_MIN_VERSION` defaults to `772`,
so a fresh install already refuses anything older. There is nothing to turn on.

**TLS 1.2 is the minimum still supported.** Drop to it only when something that has to reach
McHarbor cannot speak 1.3: OpenSSL 1.0.2 (CentOS 7, Ubuntu 16.04), Java 10 and earlier, or
a TLS-inspecting middlebox on the path. TLS 1.1 and below are refused at startup and cannot
be re-enabled.

| | Value | Accepted spellings |
|---|---|---|
| Preferred (default) | TLS 1.3 | `1.3`, `13`, `0x0304`, `772` |
| Minimum supported | TLS 1.2 | `1.2`, `12`, `0x0303`, `771` |

To stay on the preferred floor, change nothing. To allow older clients, set the variable in
your compose file; the shipped `docker-compose.yml` already carries the line:

```yaml
    environment:
      - TLS_MIN_VERSION=1.2
```

Then `docker compose up -d` to recreate the container. A native install sets the same
variable in `/etc/mcharbor/mcharbor.env` and restarts the service.

To confirm what the listener actually accepts, ask it directly. The first command should
succeed and the second should fail on a default install:

```bash
openssl s_client -connect <host>:8705 -tls1_3 </dev/null
openssl s_client -connect <host>:8705 -tls1_2 </dev/null
```

The startup log also states the floor in use: `server listening with TLS … min_version=772`.

Note that where TLS is terminated at a reverse proxy, the proxy's own minimum version is
what clients negotiate against; `TLS_MIN_VERSION` then governs only the hop between the
proxy and McHarbor.

## Behind a reverse proxy

- Allow WebSocket upgrades: terminal, container logs and live events all use them.
- Add the external URL to `ALLOWED_ORIGINS`.
- Set `MCHARBOR_TRUSTED_PROXY_CIDRS` to the proxy's address or network. Empty is the secure
  default and means forwarding headers are ignored entirely. Never `0.0.0.0/0`; that lets
  any caller claim any origin. A malformed value stops the container from starting.
- Consider `FORCE_SECURE_COOKIES=true` when the proxy terminates TLS.

Worked Caddy, Traefik and NGINX configurations (WebSockets, SSE, large uploads, health checks
on `/api/readyz`) are in [Deployment → Reverse proxy](../deployment/reverse-proxy.md).

## Adding the agent

The agent manages a Docker host that is not this one, including hosts behind NAT: it dials
**outbound** to McHarbor over WebSocket.

The shipped Compose file carries an `agent` service under the `agent` profile:

```bash
docker compose --profile agent up -d
```

It needs `MCHARBOR_URL` and `MCHARBOR_AGENT_TOKEN`, both of which come from the environment
you create in the interface; McHarbor generates the token and shows it **once**. In
practice you will more often run the agent on the other host, with the install command the
environment wizard produces.

## Verifying the install

```bash
docker compose exec mcharbor curl -fk https://localhost:8705/api/health
docker compose logs mcharbor | head -50
```

A container that is `healthy` and a login page that renders means the backend, the database
and the frontend bundle are all fine. What it does not prove is that your browser can reach
it; that is `ALLOWED_ORIGINS` and the proxy.

## Container or native

The container above is the recommended default for every host: no host packages, one
image to pull, and the upgrade path this whole page describes. McHarbor also installs
**natively** on Linux: a `.deb`, `.rpm` or tarball running under systemd as an
unprivileged `mcharbor` account, with no container runtime involved at all.

Native suits a narrower set of hosts: one whose only job is being a Docker control plane,
one under a policy against privileged containers or bind-mounting the Docker socket, or a
control-plane-only install that manages remote agents and never runs a local Docker daemon.
It also comes with a real constraint the container does not have: a glibc floor that rules
out several common distributions outright.

See [Native Linux install](./native-linux.md) for supported distributions, the exact
failure on an unsupported one, the filesystem layout, and the `docker`-group security
statement that applies there.

## In this folder

| Page | What it covers |
|---|---|
| [Installing Docker Engine](./docker-engine.md) | Docker Engine on every supported distribution, the post-install steps, and the socket path |
| [Native Linux install](./native-linux.md) | Installing without a container: supported distributions, packages, upgrading, uninstalling, and the security statement |
| [Troubleshooting](./troubleshooting.md) | What goes wrong during or just after an install |

## Related

- [Prerequisites](../prerequisites/README.md): what has to be true first
- [First run](../first-run/README.md): the setup wizard and the first environment
- [Configuration](../configuration/README.md): every environment variable
- [Upgrading](../upgrading/README.md): moving to a new version
