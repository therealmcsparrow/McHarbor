# Host, metrics and logs

39 endpoints covering the machine rather than the workloads on it: daemon info, resource
metrics, Docker events, log streams, terminals, OS updates, and McHarbor's own restart and
reset.

| Key | Covers |
|---|---|
| `host.view` | Host info, hardware inventory, container and host metric series |
| `host.manage` | Docker prune |
| `host_self.view` | Read McHarbor's own host: its OS, OS update check and schedule, reboot state, and systemd services |
| `host_self.manage` | Change McHarbor's own host: apply OS updates, set their schedule, run service actions |
| `host_self.reboot` | Schedule or cancel a reboot of McHarbor's own host |
| `events.view` | The Docker event stream |
| `logs.view` | Container log streams, OS logs |
| `terminal.access` | Container and host shells |
| `activity.view` | The container event feed |
| `system.manage` | Restart and factory-reset McHarbor |
| `settings.manage` | Purges |
| `host_updates.view` | Host OS description, the cached OS update check, its schedule and the apply stream |
| `host_updates.manage` | Apply OS package updates, set the automatic check schedule |
| `host_updates.reboot` | Schedule or cancel a reboot of an environment's host |
| `metrics.scrape` | McHarbor's own Prometheus exposition |

## Things worth knowing first

**Metrics are tiered.** Raw 30-second samples are kept for `retention_metrics_raw_hours`
(default 24); once a 10-minute bucket lies entirely past that line the collector folds it
into an average and deletes the raw rows, and those averages survive for
`retention_metrics_rollup_days` (default 30). Neither accepts "keep forever": an unbounded
30-second series would grow without limit. See [Settings](../settings/reference.md).

**Hardware inventory always answers 200.** An unreadable `/sys` reports `available: false`
rather than failing, because a 500 there would put an error state on a page whose other
twelve sections rendered correctly, for a machine that is working fine.

**`POST /api/metrics/host/prune` needs `confirm: true`**, and `volumes: true` on top of that
before it will delete named volumes.

**The purge endpoints share a shape.** `DELETE /api/activity` and `DELETE /api/lifecycle`
here, and the audit, scan and backup-log equivalents elsewhere, all delete everything past
the configured retention cutoff, record the purge itself as an audit entry, and accept
`?vacuum=true` to hand the freed pages back to the operating system.

**Exec events are off by default.** On a measured install, `exec_create` / `exec_start` /
`exec_die`, overwhelmingly healthcheck probes, were 95.4% of all stored events, about
5.2 GB. Turning `retention_record_exec_events` on is supported and genuinely high-volume.
Nothing is lost while it is off: healthcheck results are still recorded, with exit codes.

**Restart and reset both go through a detached helper container**, so the HTTP response is
delivered before the process stops. Reset additionally requires a double-confirmation string
in the body, and both refuse on a non-local environment.

**`/api/system/os-terminal/ws` and `/api/system/host-terminal/ws` are the same handler**
mounted at two paths.

**Writing a lifecycle event takes `settings.manage`**, the same key its sibling purge route
carries. It used to carry none at all, despite a comment calling itself admin-only, which left
any signed-in account able to write into the record everyone else reads. The two **reads**
remain open: they back the Docker logging tab and the dashboard panel.
## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `DELETE` | `/api/activity` | `settings.manage` | Deletes every container event older than the configured retention cutoff | `?vacuum=` |
| `GET` | `/api/activity` | `activity.view` | Returns paginated container events with optional filtering | — |
| `POST` | `/api/activity` | `containers.manage` | Records a new container event | — |
| `GET` | `/api/docker/info` | — | Returns extended Docker daemon info | — |
| `GET` | `/api/events/stream` | `events.view` | Streams Docker events via Server-Sent Events | — |
| `DELETE` | `/api/lifecycle` | `settings.manage` | Deletes every lifecycle event older than the configured retention cutoff | `?vacuum=` |
| `GET` | `/api/lifecycle` | — | Returns paginated lifecycle events with optional filtering | `?envId=`, `?from=`, `?page=`, `?perPage=`, `?search=`, `?severity=`, `?subjectType=`, `?until=` |
| `POST` | `/api/lifecycle` | `settings.manage` | Lets other modules (stack deploys, image updates, backup migrations) record an event | — |
| `GET` | `/api/lifecycle/recent` | — | Returns the last N events without filters | `?n=` |
| `GET` | `/api/logs/{id}` | `logs.view` | Streams container logs via Server-Sent Events | `{id}` |
| `GET` | `/api/metrics` | `metrics.scrape` | McHarbor's own Prometheus exposition (HTTP, agent transport, background work), read from memory | — |
| `GET` | `/api/metrics/containers/history` | `host.view` | Returns one series per container with history in the window | `?limit=` |
| `GET` | `/api/metrics/containers/{id}/history` | `host.view` | Returns the stored metric series for one container | `{id}` |
| `GET` | `/api/metrics/containers/{id}/stream` | `host.view` | Streams live container stats via SSE | `{id}` |
| `GET` | `/api/metrics/hardware` | `host.view` | Returns the local host's device inventory | — |
| `GET` | `/api/metrics/host` | `host.view` | Returns host system info and disk usage | — |
| `GET` | `/api/metrics/host/history` | `host.view` | Returns the stored host series: CPU and memory are the host's own reading (container sums only where none is available), network and block I/O are container sums | — |
| `POST` | `/api/metrics/host/prune` | `host.manage` | Runs a Docker prune operation | — |
| `GET` | `/api/metrics/stats-path` | `host.view` | Reports how an environment's container stats reach McHarbor (pushed by its agent, pulled, or not at all) and why | — |
| `GET` | `/api/system/host-terminal/ws` | `terminal.access` | Upgrades to a WebSocket and attaches to a host OS shell | — |
| `GET` | `/api/system/os-info` | `host_updates.view` | Describes the environment's host OS | — |
| `GET` | `/api/system/os-logs` | `logs.view` | Returns a bounded OS log snapshot | `?source=`, `?tail=` |
| `GET` | `/api/system/os-reboot` | `host_updates.view` | Returns the environment host's reboot row | — |
| `POST` | `/api/system/os-reboot` | `host_updates.reboot` | Schedules a reboot of the environment host | — |
| `DELETE` | `/api/system/os-reboot` | `host_updates.reboot` | Cancels a scheduled reboot of the environment host | — |
| `GET` | `/api/system/os-reboot/inventory` | `host_updates.view` | Reports what a reboot of the environment host stops; `guests` counts every guest the loaded reboot-drain plugins reported | — |
| `GET` | `/api/system/os-terminal/ws` | `terminal.access` | Upgrades to a WebSocket and attaches to a host OS shell | — |
| `POST` | `/api/system/os-updates/apply` | `host_updates.manage` | Applies host OS package updates after explicit confirmation | — |
| `GET` | `/api/system/os-updates/check` | `host_updates.view` | Checks host OS package updates | — |
| `PUT` | `/api/system/os-updates/schedule` | `host_updates.manage` | Stores when the automatic OS update check should run | — |
| `GET` | `/api/system/os-updates/state` | `host_updates.view` | Returns the cached check result and the schedule | — |
| `GET` | `/api/system/os-updates/stream` | `host_updates.view` | Streams one apply job's output as SSE; `offset` resumes a reloaded tab | `?job=`, `?offset=` |
| `GET` | `/api/system/self/os-info` | `host_self.view` (global) | Describes the OS of the machine McHarbor is installed on, or why it cannot be reached | — |
| `GET` | `/api/system/self/os-reboot` | `host_self.view` (global) | Returns the McHarbor host's reboot row | — |
| `POST` | `/api/system/self/os-reboot` | `host_self.reboot` (global) | Schedules a reboot of the McHarbor host | — |
| `DELETE` | `/api/system/self/os-reboot` | `host_self.reboot` (global) | Cancels a scheduled reboot of the McHarbor host | — |
| `GET` | `/api/system/self/os-reboot/inventory` | `host_self.view` (global) | Reports what a reboot of the McHarbor host stops; `guests` counts every guest the loaded reboot-drain plugins reported | — |
| `POST` | `/api/system/self/os-updates/apply` | `host_self.manage` (global) | Upgrades the McHarbor host after explicit confirmation | — |
| `GET` | `/api/system/self/os-updates/check` | `host_self.view` (global) | Checks the McHarbor host's packages | — |
| `PUT` | `/api/system/self/os-updates/schedule` | `host_self.manage` (global) | Stores the McHarbor host's check schedule and upgrade defaults | — |
| `GET` | `/api/system/self/os-updates/state` | `host_self.view` (global) | Returns the McHarbor host's cached check and schedule | — |
| `GET` | `/api/system/self/os-updates/stream` | `host_self.view` (global) | Streams a McHarbor host apply job; refuses an environment's job, and the environment route refuses this one | `?job=`, `?offset=` |
| `POST` | `/api/system/reset` | `system.manage` | Schedules a factory reset of McHarbor | — |
| `POST` | `/api/system/restart` | `system.manage` | Schedules a self-restart of the running McHarbor container | — |
| `GET` | `/api/terminal/ws` | — | Upgrades to a WebSocket and attaches to a Docker exec session | `?container=`, `?shell=`, `?user=`, `?workDir=` |


## Request bodies

**`POST` `/api/activity`**: `environmentId`?, `containerId`, `containerName`?, `eventType`, `action`, `metadata`?

**`POST` `/api/lifecycle`**: `id`, `environmentId`?, `subjectType`, `subjectId`, `subjectName`?, `eventType`, `action`, `state`?, `severity`, `metadata`?, `source`, `timestamp`

**`POST` `/api/metrics/host/prune`**: `type`, `volumes`?, `confirm`

**`POST` `/api/system/os-updates/apply`**: `confirm`, `withNewPackages`?, `skipUnavailable`?, `removeUnused`?
(each overrides the environment's saved default for this run only; the job status stream
carries `hint: "skipUnavailable"` when a download failure would have been skipped with it on)

**`PUT` `/api/system/os-updates/schedule`**: `enabled`, `time`, `days`, `withNewPackages`, `skipUnavailable`, `removeUnused`

The `/api/system/self/...` routes take the same bodies. They never read `?env=`: they act on
the machine McHarbor itself is installed on, so they are gated by the `host_self.*` keys, checked
at global scope, and not by `host_updates.*`. Those are per-environment keys, and a grant of
them, even on every environment, no longer reaches the McHarbor host. The same three keys gate
the systemd services routes under `/api/host-services/self/...`
([Services](../../screens/environments/services/README.md)).

**`POST` `/api/system/reset`**: `confirm`

## Related

- [Containers](./containers.md): per-container stats and logs
- [Streaming](../conventions/streaming.md): the SSE and WebSocket routes here
- [Settings reference](../settings/reference.md): retention keys
