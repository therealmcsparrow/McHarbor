# Containers

`/containers` · `/containers/:id` · `/containers/:id/edit`

The containers in the selected environment, a detail page with 21 tabs for each one,
and a full configuration editor.

## Permissions

| Key | What it unlocks |
|---|---|
| `containers.view` | See the list and the detail page |
| `containers.manage` | Start, stop, restart, pause, kill, create, edit, move, rename |
| `containers.delete` | Remove a container |
| `logs.view` | The Logs tab |
| `terminal.access` | The Terminal and Exec tabs |
| `containers.debug_shell` | The Terminal tab's **Debug shell**: a temporary busybox for a shell-less image. See [Debug shell](./debug-shell/README.md) |
| `scans.view` | The Scans tab |
| `backups.view` | The backup tabs |
| `autoheal.manage` | Turn auto-heal on for a container |
| `host.view` | The list of the host's devices behind the **Devices** Host Path dropdown. See [Devices](./devices/README.md) |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- One row per container: name, image, state, ports and environment.
- State is the Docker state plus health where a health check exists: running, stopped, paused, restarting, created, exited, dead, removing, and healthy or unhealthy alongside it.
- The detail page groups its 21 tabs into six: Overview, Configuration, Runtime, Delivery, Security and Data.

## The detail tabs

| Group | Tabs | What is there |
|---|---|---|
| **Overview** | `overview` | Identity, state, and the health, uptime and insights cards |
| **Configuration** | `environment`, `labels`, `network`, `mounts`, `resources` | The running configuration, read-only |
| **Runtime** | `activity`, `logs`, `terminal`, `exec`, `files`, `processes`, `services`, `inspect` | What it is doing right now |
| **Delivery** | `updates`, `history`, `changes`, `diff` | Where the image came from and where it is going |
| **Security** | `security`, `scans` | What it is allowed to do, and what is known to be wrong with its image |
| **Data** | `backup-settings`, `backup-schedules`, `backups` | The long-term safety net |

The Data group's tabs read **Backup**, **Schedules** and **Backup list**, the last one
matching what the standalone Backups page calls its run list. The ids are unchanged, so the
tab id is still the unit of navigation and `?tab=logs` keeps resolving however the groups are
rearranged.

Tabs are no longer draggable. The order is the one the registry declares; pinning a tab to
the page head is a separate thing and is unaffected.

## Insights

The Overview tab derives findings rather than showing raw numbers, each with a severity of
Info, Warning or Critical:

| Insight | Raised when |
|---|---|
| Health checks are failing | Consecutive failed probes |
| Killed by the OOM killer | The kernel killed it for exceeding its memory limit |
| Critical image vulnerabilities | The latest scan reports criticals |
| Sustained high CPU or memory | Recent samples average high against the limit |
| Restarted repeatedly | Docker reports a high restart count |
| Runs in privileged mode | Privileged expands host access and should be reviewed |
| Backup coverage stale or missing | No successful backup in the last 7 days |
| A newer image is available | An update was found |
| No health check configured | Nothing can detect failure automatically |

With nothing to report it says so: "No actionable issues detected from current container
data."

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |
| [Debug shell](./debug-shell/README.md) | A temporary static busybox for distroless and scratch images: the flow, the four cleanup triggers, the audit rows, and where the binary comes from |
| [Devices](./devices/README.md) | Mapping host devices into a container: the grouped Host Path dropdown, why it stores the `by-id` path, the cgroup permissions, and where the list comes from |
| [Self-protection](./self-protection/README.md) | What needs `host_self.manage` on the environment McHarbor runs on: create-time rules, the host-path denylist, namespace joins, compose fields, and every module that can act on a McHarbor container |

## Related

- [Stacks](../stacks/README.md): containers deployed from a Compose file
- [Backups](../../backup/screen/README.md): plans, runs and restore
- [Logs](../logs/README.md) and [Terminal](../terminal/README.md): the same tools outside a detail page
- [Settings → Updates → Containers](../../settings/updates/containers/README.md): automatic image updates
