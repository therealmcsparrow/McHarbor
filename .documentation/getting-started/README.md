# Getting Started

McHarbor is a self-hosted container management platform. It runs as a single Docker
container, manages Docker across local, remote and NAT-ed hosts, and ships with stacks,
backups, workflows, RBAC, auditing and a plugin system.

This section takes an empty host to a working install. Each step is a folder.

| Step | What it covers |
|---|---|
| [Prerequisites](./prerequisites/README.md) | What has to be true first: Docker, the socket, ports, browser, and the two decisions to make before you start |
| [Installation](./installation/README.md) | The Compose file, what is in the image, ports, volumes, TLS, reverse proxies, and adding the agent |
| [First run](./first-run/README.md) | The setup flow, the admin account, the first environment, and the five things to do next |
| [Configuration](./configuration/README.md) | Every environment variable, with its default and what it changes |
| [Upgrading](./upgrading/README.md) | Moving to a new version, what happens to plugins, agents, rollback, and factory reset |
| [Deployment](./deployment/README.md) | Production layouts: hardened Compose, external database, Helm chart and plain Kubernetes manifests, Caddy/Traefik/NGINX in front, and the security posture of an internet-facing host |
| [Verifying releases](./verifying-releases/README.md) | Checking image and package signatures with cosign, and extracting the SBOM |

Beside the steps, one reference page:

| Page | What it covers |
|---|---|
| [Changelog](./changelog/README.md) | What changed in each **release**, the versioning scheme, and the breaking changes to read before you upgrade. Read it before [Upgrading](./upgrading/README.md), not after |

That page is the product's changelog. [CHANGELOG.md](../CHANGELOG.md) at the root of this set
is a different thing: what changed in the **documentation**.

## The short version

```bash
mkdir mcharbor && cd mcharbor
# write the Compose file from the Installation page
docker compose up -d
```

Open `https://<host>:8705`, paste the setup token printed in `docker logs mcharbor` (on a native install, the installer prints it, or read `journalctl -u mcharbor`), choose a database (SQLite unless you need multi-node), create the
admin account, and then create your first environment. McHarbor no longer makes one for the
mounted socket, so Environments is empty until you add it.

Then set the time zone, deal with the certificate, and decide what backups mean here.
[First run](./first-run/README.md) covers all three.

## Two things that catch people out

**`ALLOWED_ORIGINS`.** The built-in default covers `localhost` only. Open McHarbor at a LAN
address that is not listed and the browser's preflight fails, which looks like a login form
that does nothing at all. Set it to the URL you will actually type.

**The Docker socket is root.** Anything that can talk to `/var/run/docker.sock` can start a
privileged container that mounts the host filesystem. Mounting it `:ro` limits nothing about
the API; it only stops the socket file being replaced. RBAC inside McHarbor decides who may
ask McHarbor to act; it does not reduce what the McHarbor process itself can do.

## Where to go next

| You want to | Read |
|---|---|
| Understand a screen | [Screens](../screens/README.md) |
| Change how McHarbor behaves | [Settings](../settings/README.md) |
| Extend it | [Plugins](../plugins/README.md) |
| Know what changed since your version | [Changelog](./changelog/README.md) |
