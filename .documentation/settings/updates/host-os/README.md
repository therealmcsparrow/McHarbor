# Host OS

**Settings → Updates → Host OS** · `?tab=updates-host`

Operating-system package updates for the machine McHarbor itself is installed on.

This section **always acts on that machine**. It does not follow the environment selector:
picking another environment in the top bar changes nothing here. To update the host behind
an environment, use that environment's **Updates** tab instead
([Environments](../../../screens/environments/README.md)).

## Permissions

| Key | What it unlocks |
|---|---|
| `host_self.view` | See the section, read the host's OS, check for updates, follow a running upgrade |
| `host_self.manage` | Start an upgrade, save the schedule and its defaults |
| `host_self.reboot` | Schedule or cancel a reboot of the McHarbor host |

These keys are separate from the `host_updates.*` keys an environment's **Updates** tab uses, and
they are checked at **global** scope: the McHarbor host is not an environment, so a grant of
`host_updates.*`, on one environment or on all of them, does not reach it. A role that used to
manage this section through `host_updates.*` needs `host_self.*` added. The Administrator role
holds every key and needs no change. The **Services** section under **Settings → System** is
gated by the same `host_self.view` key.

## What it shows

The same panel as an environment's **Updates** tab:

- **Host operating system**: distribution, kernel, package manager, virtualisation,
  uptime, and **Reboot required**.
- **Host OS updates**: the cached check, **Check now**, the pending package list, and
  **Update host**, whose confirmation carries **Install new packages when required**,
  **Skip unavailable packages** and **Remove unused packages** for that run.
- **Update progress**: the upgrade's output line by line; a reloaded page reattaches to an
  upgrade that is still running.
- **Automatic check**: the schedule, and the saved defaults for all three options. These are
  stored for the McHarbor host alone, separately from every environment.

What the two options do per package manager, and the one automatic retry after a failed
download, are described under
[Environments → When a package cannot be downloaded](../../../screens/environments/README.md#when-a-package-cannot-be-downloaded).

## When the host cannot be reached

McHarbor reaches its own host the way it reaches a local environment's: a short-lived
privileged utility container with the host's root mounted at `/host`. That needs McHarbor to
find **its own container** on the Docker daemon it is connected to. A McHarbor installed
directly on the machine runs the package manager itself, which needs root.

When neither holds, the section says why and every control is disabled, rather than acting
on some other host:

| Message | Cause | Fix |
|---|---|---|
| "…cannot find itself on the Docker daemon it is connected to…" | `DOCKER_HOST` points at another daemon, or a socket proxy hides McHarbor's own container | Mount the host's Docker socket into the McHarbor container |
| "…has no Docker connection of its own…" | No Docker client could be created from the environment | Set `DOCKER_HOST` or mount `/var/run/docker.sock` |
| "…without root privileges…" | A native install running as an ordinary user | Run McHarbor as root, or use the container image |
| "…a machine that is not Linux…" | A native install on macOS or Windows | Nothing to update through a Linux package manager |

## Related

- [Updates](../README.md): the other sections
- [Environments](../../../screens/environments/README.md): the per-environment **Updates** tab
