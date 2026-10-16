# Containers

41 endpoints. The largest surface in the API, because a container is the thing McHarbor
exists to manage: lifecycle, configuration, files, processes, health, exec, and moving one
host to another.

All of them take [`?env=`](../conventions/parameters.md) to say which environment, and all of
them are gated by one of three keys:

| Key | Covers |
|---|---|
| `containers.view` | Reads: list, inspect, logs, stats, files, health, diff, exec history |
| `containers.manage` | Lifecycle, configuration, file writes, exec, move |
| `containers.delete` | Removal, in either of its two forms |
| `containers.debug_shell` | Installing and removing the temporary busybox debug shell in a shell-less container, and attaching the terminal to it |

## Things worth knowing first

**McHarbor refuses to act on its own container.** Stop, restart, pause, kill and remove all
check for it and return `err.container.self_stop` and friends rather than taking the product
down mid-request.

**There are two ways to remove a container.** `DELETE /api/containers/{id}` is Docker's
removal with `?force=` and `?v=`. `POST /api/containers/{id}/remove` is the extended form,
whose body carries four separate opt-ins: take the volumes, the image, the stack and the
network with it. Every one defaults to false, and an anonymous volume holding a database is
still a volume.

**Updating configuration is not the same as recreating.** `POST /{id}/update` changes runtime
resources in place: memory, CPU, restart policy. `POST /{id}/recreate` stops, renames,
recreates, starts and then removes the old container, which loses in-memory state.

**Protected containers.** A container marked protected refuses rename and resource updates
until the request sets `unlockProtected: true`. The flag is the operator confirming the unlock,
not a permission: on a McHarbor container every one of those requests also needs
`host_self.manage`; see [McHarbor's own containers](#mcharbors-own-containers).

**Moving a container** is a three-step flow: `POST /{id}/move/plan` previews what the target
environment is missing, `POST /{id}/move` performs it, and `POST /{id}/move/stream` performs
it while streaming progress. The public
`GET /api/containers/internal/move-transfers/{transferId}` is how a connected agent pulls the
prepared archive; it authenticates on its own token, not on a session.

**File operations are path-validated.** Paths must be absolute; traversal is rejected
(`err.path_traversal`), and reads above a size ceiling return `err.container.file_too_large`.

## McHarbor's own containers

On the environment McHarbor runs on, `containers.manage` is effectively administration of
McHarbor itself: a shell in its container reads its keys and database, a recreate rebuilds it
onto any image, and a new container that mounts the Docker socket reaches the whole host. So
these operations need the global `host_self.manage` **as well as** the route's own key:

| Target | Operations that also need `host_self.manage` |
|---|---|
| A McHarbor container: this node, an HA peer, a McHarbor helper (self-update helper and watchdog, host shells, bind-path helpers) or a container labelled `com.mcharbor.protected` | `POST /{id}/exec`, the terminal (`GET /api/terminal/ws`), `POST /{id}/debug-shell`, `start`, `stop`, `restart`, `pause`, `unpause`, `kill`, `update`, `rename`, `recreate`, `DELETE /{id}`, `POST /{id}/remove`, `move`, `network/connect`, `network/disconnect`, and every file write (`PUT`/`DELETE /files/content`, `files/upload`, `files/directory`, `files/rename`, `files/chmod`) |
| Any container on the environment McHarbor runs on that is privileged, runs with `pid: host`, adds `SYS_ADMIN` (or `ALL`), or mounts `/`, `/proc`, `/sys`, `/dev`, `/etc`, `/root`, `/home`, the system directories, `/var/lib/docker` or other runtime data, `/run`, the Docker socket (or a directory holding it), a path or volume McHarbor itself mounts, or `volumes_from` a McHarbor container | exec, the terminal, the debug shell and file writes into it; and **creating** such a container: `POST /api/containers`, a recreate that keeps or adds those settings, and a move onto that environment |
| Creating or recreating **any** container on that environment with `devices`, a `security_opt` that removes a confinement, `userns_mode: host`, `ipc: host`, `uts: host`, `cgroup: host`, `network_mode: host`, any `cap_add`, a `container:` or `service:` namespace join, or a bind that is a volume in disguise (a `local` volume with `device` and `o=bind` options, including through `POST /api/volumes`) | The same create-time refusal, `err.host_self.host_access_required`. A shell in a container that merely has a capability added is not gated |
| A stack that runs a McHarbor container | `PUT`, `DELETE`, `up`, `stop`, `down`, `restart`, `update`, `reinstall`, `prune` and `PUT env-vars` on `/api/stacks/{name}`, and `POST /api/stacks/adopt` for it |
| A compose file brought up or saved on the environment McHarbor runs on | Any service with the host access above, or with a `secrets` or `configs` `file:`, `build.context`, `env_file`, `label_file` or `device_cgroup_rules` that would reach it; a top-level `name:` equal to McHarbor's own compose project (which would adopt and recreate McHarbor), or an `include:` / `extends: file:` the check cannot read. Applies to stack create with auto-start, save, `up`, `update`, `reinstall` and App Store installs |
| A McHarbor container, reached through another module | `POST /api/networks/{id}/connect` and `…/disconnect` (`networks.manage`); `POST /api/reconciler/{id}/reconcile` when it would start, stop or remove it; a container backup restore into it; and a runbook step `container.start`, `container.stop` or `container.restart` |

The same list, organised by what a container may reach, is on [Self-protection](../../screens/containers/self-protection/README.md).

A refusal is `403` with `code: err.host_self.container_required` (or
`err.host_self.host_access_required`, `err.host_self.stack_required`) and a body naming the
key:

```json
{ "success": false, "code": "err.host_self.container_required",
  "error": "This container belongs to McHarbor itself. …",
  "data": { "permission": "host_self.manage", "target": "app", "reasons": [] } }
```

Every refusal writes a `host_self.denied` audit entry at warning severity, naming the
operation, the target kind and the reasons.

**Detection.** A McHarbor container is recognised by the image label
`com.mcharbor.role=app` (baked into the image, so it survives a retag or a mirror), the
image's own `MCHARBOR_PLUGIN_TRUST_KEYS` variable, this process's own container id (so a
renamed container or image is still caught), a compose service named `mcharbor` or
`mcharbor-node-N`, a container name `mcharbor` or `…mcharbor-node-N…`, or an image whose last
path segment is `mcharbor`. Helpers carry `com.mcharbor.helper`. The environment McHarbor runs
on is the one whose daemon holds McHarbor's own container or a peer McHarbor node's; outside a
container it is the local-socket environment. A container labelled `com.mcharbor.protected` (the
HA compose file labels its PostgreSQL this way) is protected like McHarbor's own, and the paths and
volumes it mounts are added to the ones McHarbor protects on that daemon.

**What still holds for a `host_self.manage` holder.** The older lock that keeps the app
running is unchanged: stop, restart, pause, kill, remove and file writes on McHarbor's own
container still answer `err.protected_target`, and rename, update and recreate still need
`unlockProtected: true`. `host_self.manage` passes the permission check; it does not remove
that lock. McHarbor's own self-update and restart (`POST /api/versions/self-update`,
`POST /api/system/restart`, under `system.manage`) do not go through these routes and are not
affected.


## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/containers` | `containers.view` | Returns all containers | `?all=` |
| `POST` | `/api/containers` | `containers.manage` | Creates a new container | — |
| `POST` | `/api/containers/check-updates` | `containers.view` | Checks for available image updates for containers | — |
| `GET` | `/api/containers/health/summary` | `containers.view` | Returns fleet-wide availability over a window, stitched from the raw health/uptime series and the daily rollup behind it | `?days=`, `?from=`, `?limit=`, `?to=` |
| `GET` | `/api/containers/internal/move-transfers/{transferId}` | — | Streams one prepared move archive to a connected agent | `{transferId}` |
| `POST` | `/api/containers/prune` | `containers.manage` | Removes stopped containers | — |
| `GET` | `/api/containers/stats/summary` | `containers.view` | Returns a snapshot of resource stats for all running containers | — |
| `DELETE` | `/api/containers/{id}` | `containers.delete` | Removes a container | `{id}`; `?force=`, `?v=` |
| `GET` | `/api/containers/{id}` | `containers.view` | Returns detailed container information | `{id}` |
| `GET` | `/api/containers/{id}/diff` | `containers.view` | Returns the filesystem changes recorded against the container's running image versus its declared image layers | `{id}` |
| `POST` | `/api/containers/{id}/exec` | `containers.manage` | Runs a one-shot command inside a running container, capturing stdout/stderr/exit code and persisting the invocation to container_exec_history | `{id}` |
| `GET` | `/api/containers/{id}/exec/history` | `containers.view` | Returns the most recent exec audit entries for the given container | `{id}`; `?limit=` |
| `POST` | `/api/containers/{id}/debug-shell` | `containers.debug_shell` | Uploads a static busybox to `/.mcharbor-debug/busybox` in a running, shell-less, writable container and returns the session (`sessionId`, `path`, `arch`, `busyboxDigest`, `sizeBytes`, `installed`, `expiresAt`); 201. Refuses a container with a shell, a stopped one or a read-only root filesystem with 409; see [Debug shell](../../screens/containers/debug-shell/README.md) | `{id}` |
| `GET` | `/api/containers/{id}/debug-shell` | `containers.view` | The container's open debug sessions, and `leftBehind` when one is past expiry or failed its cleanup | `{id}` |
| `DELETE` | `/api/containers/{id}/debug-shell` | `containers.debug_shell` | Removes the busybox now; `{ removed }` | `{id}`; `?session=` |
| `GET` | `/api/terminal/ws?debugSession=` | `containers.debug_shell` | With `debugSession`, the terminal runs the uploaded busybox shell instead of detecting one, and removes the install when the socket closes | `?container=`, `?debugSession=` |
| `GET` | `/api/containers/{id}/files` | `containers.view` | Lists files and directories at a path inside a container | `{id}`; `?path=` |
| `POST` | `/api/containers/{id}/files/chmod` | `containers.manage` | Changes file permissions inside a container | `{id}` |
| `DELETE` | `/api/containers/{id}/files/content` | `containers.manage` | Deletes a file or directory inside a container | `{id}`; `?path=`, `?recursive=` |
| `GET` | `/api/containers/{id}/files/content` | `containers.view` | Returns the content of a file inside a container | `{id}`; `?download=`, `?path=` |
| `PUT` | `/api/containers/{id}/files/content` | `containers.manage` | Saves content to a file inside a container | `{id}`; `?path=` |
| `POST` | `/api/containers/{id}/files/directory` | `containers.manage` | Creates a directory inside a container | `{id}`; `?path=` |
| `POST` | `/api/containers/{id}/files/rename` | `containers.manage` | Renames a file or directory inside a container | `{id}` |
| `POST` | `/api/containers/{id}/files/upload` | `containers.manage` | Uploads a file to a container directory via multipart form | `{id}`; `?path=` |
| `GET` | `/api/containers/{id}/health/history` | `containers.view` | Returns the persisted healthcheck probe series and the run-state samples behind the Overview tab's Health and Uptime cards | `{id}`; `?limit=` |
| `POST` | `/api/containers/{id}/kill` | `containers.manage` | Sends a signal to a container | `{id}`; `?signal=` |
| `GET` | `/api/containers/{id}/logs` | `containers.view` | Returns container logs | `{id}`; `?since=`, `?stderr=`, `?stdout=`, `?tail=` |
| `POST` | `/api/containers/{id}/move` | `containers.manage` | Moves a container to another Docker environment | `{id}` |
| `POST` | `/api/containers/{id}/move/plan` | `containers.view` | Previews the resources and settings needed to move a container | `{id}` |
| `POST` | `/api/containers/{id}/move/stream` | `containers.manage` | Moves a container and streams progress via Server-Sent Events | `{id}` |
| `POST` | `/api/containers/{id}/network/connect` | `containers.manage` | Connects a container to a network | `{id}` |
| `POST` | `/api/containers/{id}/network/disconnect` | `containers.manage` | Disconnects a container from a network | `{id}` |
| `POST` | `/api/containers/{id}/pause` | `containers.manage` | Pauses a container | `{id}` |
| `POST` | `/api/containers/{id}/recreate` | `containers.manage` | Stops, renames, recreates, starts, and removes the old container | `{id}` |
| `POST` | `/api/containers/{id}/remove` | `containers.delete` | Removes a container with optional image/stack cleanup | `{id}` |
| `POST` | `/api/containers/{id}/rename` | `containers.manage` | Changes the Docker container name | `{id}` |
| `POST` | `/api/containers/{id}/restart` | `containers.manage` | Restarts a container | `{id}` |
| `GET` | `/api/containers/{id}/services` | `containers.view` | Detects OS-level services running inside a container | `{id}` |
| `POST` | `/api/containers/{id}/shells` | `containers.view` | Probes for available shells in a container | `{id}` |
| `POST` | `/api/containers/{id}/start` | `containers.manage` | Starts a container | `{id}` |
| `GET` | `/api/containers/{id}/stats` | `containers.view` | Returns container resource usage statistics | `{id}`; `?stream=` |
| `POST` | `/api/containers/{id}/stop` | `containers.manage` | Stops a container | `{id}`; `?t=` |
| `GET` | `/api/containers/{id}/top` | `containers.view` | Returns running processes in a container | `{id}` |
| `POST` | `/api/containers/{id}/unpause` | `containers.manage` | Unpauses a container | `{id}` |
| `POST` | `/api/containers/{id}/update` | `containers.manage` | Updates container runtime resources | `{id}` |


## Request bodies

**`POST` `/api/containers`**: `name`, `image`, `cmd`?, `env`?, `labels`?, `exposedPorts`?, `hostConfig`?, `networkingConfig`?, `volumes`?, `workingDir`?, `entrypoint`?, `user`?, `hostname`?, `domainname`?, `tty`?, `openStdin`?

**`POST` `/api/containers/{id}/exec`**: `command`, `workDir`?, `user`?, `shell`?

**`POST` `/api/containers/{id}/move`**: `targetEnvId`, `targetName`?, `networkMode`?, `transferImage`, `createMissingNetworks`, `createMissingVolumes`, `copyNamedVolumes`, `startTarget`, `stopSource`, `removeSource`, `networks`?, `volumes`?

**`POST` `/api/containers/{id}/move/plan`**: `targetEnvId`, `targetName`?, `networkMode`?, `networks`?, `volumes`?

**`POST` `/api/containers/{id}/move/stream`**: `targetEnvId`, `targetName`?, `networkMode`?, `transferImage`, `createMissingNetworks`, `createMissingVolumes`, `copyNamedVolumes`, `startTarget`, `stopSource`, `removeSource`, `networks`?, `volumes`?

**`POST` `/api/containers/{id}/network/connect`**: `network`, `aliases`?, `ipv4Address`?, `ipv6Address`?, `macAddress`?, `reconnect`?

**`POST` `/api/containers/{id}/network/disconnect`**: `network`, `force`?

**`POST` `/api/containers/{id}/remove`**: `force`, `removeVolumes`, `removeImage`, `removeStack`, `removeNetwork`

**`POST` `/api/containers/{id}/rename`**: `name`, `unlockProtected`?

**`POST` `/api/containers/{id}/update`**: `memory`?, `memorySwap`?, `memoryReservation`?, `nanoCPUs`?, `cpuShares`?, `cpuQuota`?, `cpuPeriod`?, `cpusetCpus`?, `cpusetMems`?, `blkioWeight`?, `restartPolicy`?, `unlockProtected`?

## Related

- [Stacks](./stacks.md): containers that belong to a Compose project
- [Backups](./backups.md): the per-container backup routes
- [Host](./host.md): metrics and log streams for the same containers
- [Containers screen](../../screens/containers/README.md)
