# Services

`/environments/:id?tab=services`

The **Services** tab manages the systemd services of the host behind an environment: it
lists every `.service` unit, starts, stops, restarts, enables and disables them, and edits
their configuration the way `systemctl edit` does. Everything goes through the McHarbor
agent on that host.

The tab exists only on an environment whose agent is installed **natively**: a binary
under systemd, reported as install kind `native` (Overview → Connection details). A
containerised agent sees its own container, not the host's systemd, so the tab is hidden
there.

## Permissions

| Key | Scope | What it unlocks |
|---|---|---|
| `host_services.view` | `environment` | See the tab's list, and open a unit's configuration read-only |
| `host_services.manage` | `environment` | Start, stop, restart, enable, disable; save and revert an override; reload systemd |

No role preset carries either key. Stopping or reconfiguring a unit can take a host off the
network, so the keys are granted deliberately, per role, in **Security → Roles**.

A control you lack the key for renders **disabled with a tooltip naming the key**.

### McHarbor's own host

**Settings → System → Services** (`?tab=host-services`) shows the same tab for the machine
McHarbor is installed on. It is gated by `host_self.view` (see the list and open a unit) and
`host_self.manage` (every action), both at **global** scope, and not by `host_services.*`: a
grant of the per-environment keys, even on every environment, does not reach McHarbor's own
host. The routes live under `/api/host-services/self/...` and never read `?env=`.

## What it shows

One row per `.service` unit: every unit systemd has loaded, merged with every installed
unit file, so a service that is installed but has never started still appears.

| Column | Source | Notes |
|---|---|---|
| **Unit** | unit name | A shield marks a [protected](#protected-units) unit |
| **Description** | `Description=` | |
| **State** | `ActiveState · SubState` | `active · running`, `inactive · dead`, `failed · failed`… A failed unit carries an error icon; the reason is behind it |
| **Enabled** | unit file state | `enabled`, `disabled`, `static`, `masked`, `indirect`, `generated`… |
| **Main PID** | `MainPID` | Empty when the unit has no running main process |
| **Load** | `LoadState` | Hidden by default; turn it on from the columns menu |

Facets: **All**, **Running**, **Failed**, **Inactive**, **Enabled**, **Disabled**. The
toolbar carries search, the columns menu, density, CSV/JSON export, refresh, and
**Reload systemd**.

A failed unit's reason is assembled from systemd's own properties (`Result`, the main
process's exit status or signal, and `StatusText`) and is shown in full in a dialog behind
the error icon, never truncated into the cell.

Template units (`name@.service`) are listed but their lifecycle buttons are off: a template
cannot be started without an instance name.

## Actions

| Action | Confirms | Notes |
|---|---|---|
| **Start** | no | `systemctl start -- <unit>` |
| **Stop** | yes | Refused on protected units |
| **Restart** | yes | Allowed on protected units. On the agent's own unit the dialog warns that the connection drops and reconnects |
| **Enable** | no | Off for `static` and `masked` units |
| **Disable** | yes | Refused on protected units |
| **Edit configuration** | — | Opens the override editor below |
| **Revert** (in the editor) | yes | Deletes the override |
| **Reload systemd** (toolbar) | yes | `systemctl daemon-reload` |

Every mutation is written to the audit log against the signed-in user, with the action, the
unit, the environment and the outcome, including a refusal.

## The drop-in model

**Edit configuration** never touches the vendor unit file. It works exactly like
`systemctl edit <unit>`:

- The dialog shows the **effective configuration** read-only: the output of
  `systemctl cat <unit>`, the vendor file followed by every drop-in that applies.
- The editor holds `/etc/systemd/system/<unit>.d/override.conf`. It is empty until the first
  save, which creates the directory and the file.
- **Save** writes the file atomically: a temporary file in the same directory, synced,
  mode `0644`, owned by root, then renamed over `override.conf`. systemd never reads a
  half-written drop-in.
- **Reload systemd after saving** (on by default) runs `systemctl daemon-reload` so the change
  takes effect for the next start or restart. It does not restart the service.
- **Check with systemd-analyze verify** (on by default) runs `systemd-analyze verify` on the
  unit and shows what it reports as a warning. It never blocks the save: `verify` flags
  harmless things on many stock units.
- **Revert** deletes `override.conf`, removes `<unit>.d` only when nothing else is left in it
  (another tool's drop-in in the same directory is kept), and reloads systemd. The override's
  content cannot be recovered afterwards.

The audit row for a save carries the SHA-256 of the override and its size, not the text.

An override is at most 64 KiB of UTF-8 text.

## Protected units

Stop and disable are **refused** on these units, by the McHarbor server and again by the
agent, so neither a modified server nor a hand-crafted request alone can cut the host off:

| Units | Why |
|---|---|
| `mcharbor-agent*` | The agent itself; stopping it ends McHarbor's access to the host |
| `ssh.service`, `sshd.service` | Remote shell access; stopping it can lock you out |
| `systemd-*` | Core systemd components |
| `dbus*` | The system message bus systemd and most services depend on |
| `docker.service`, `containerd.service` | The container runtime the agent drives |

Restart stays allowed on all of them, because it brings the unit straight back. Restarting
`mcharbor-agent*` is answered first and performed afterwards: the agent replies, then asks
systemd to restart it without waiting, so the connection drops and the agent reconnects a
few seconds later.

A refused request answers `409` with `err.host_services.protected`. In the tab the Stop and
Disable buttons of a protected row are disabled, with a tooltip saying why.

## Unit names

A unit name must match `^[A-Za-z0-9@._:-]+\.service$`, be at most 256 characters, contain no
`/`, `\` or `..`, and not start with `-` or `.`. The server, the agent and the CLI all check
it; anything else is refused with `err.host_services.invalid_unit` before a command runs.

## Routes

All take `?env=<environment id>`.

| Method and path | Key |
|---|---|
| `GET /api/host-services` | `host_services.view` |
| `GET /api/host-services/{unit}/override` | `host_services.view` |
| `PUT /api/host-services/{unit}/override` | `host_services.manage` |
| `DELETE /api/host-services/{unit}/override` | `host_services.manage` |
| `POST /api/host-services/{unit}/{action}` with `start`, `stop`, `restart`, `enable`, `disable` | `host_services.manage` |
| `POST /api/host-services/daemon-reload` | `host_services.manage` |

The McHarbor host takes the same routes under `/api/host-services/self/` (`GET /`,
`GET|PUT|DELETE /{unit}/override`, `POST /{unit}/{action}` and `POST /daemon-reload`), with
`host_self.view` for the reads and `host_self.manage` for the rest, both required globally.

`GET /api/host-services` on an environment that cannot be managed answers `200` with
`availability.supported: false` and `availability.reason` (`not_agent`, `not_native`,
`offline`, `agent_outdated` or `no_systemd`) and no units, so the tab explains itself.

## Agent requirements

The agent must be **2026.09.01 build 202609271429** or newer. An older agent does not know
the `host_service_request` message and would drop it, so McHarbor refuses up front and the
tab says **Update the agent to use Services**, naming the build it has and the build it
needs. A native agent updates only from a published release; see
[Settings → Updates → Agents](../../../settings/updates/agents/README.md).

The agent reports `host_services` in its capabilities at connect when it runs natively, PID
1 is systemd and `systemctl` is on `PATH`. Each `systemctl` call is bounded (30 s for
reads, 100 s for start/stop/restart), and its output is capped at 4 MiB. The listing uses
`systemctl list-units --output=json` where the host's systemd supports it and falls back to
the plain table otherwise.

## Command line

`mcharbor host service list|start|stop|restart|enable|disable|cat|edit|revert|daemon-reload
--env <id>`. See [Command-line client](../../../cli/README.md).

## In this folder

| Page | What it covers |
|---|---|
| [Troubleshooting](./troubleshooting.md) | What goes wrong, and what to do about it |
