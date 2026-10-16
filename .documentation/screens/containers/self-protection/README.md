# Containers: Self-protection

On the environment McHarbor runs on, `containers.manage` is administration of McHarbor itself. A
shell in its container reads its keys and database, a recreate rebuilds it onto any image, and a
new container that mounts the Docker socket, or a sensitive host directory, reaches the whole host.
McHarbor therefore asks for the global **`host_self.manage`** permission **in addition to** the
route's own key for every operation that could reach it.

This page is the complete list. The route-level summary, with the `403` body, is in
[McHarbor's own containers](../../../api/endpoints/containers.md#mcharbors-own-containers).

This is a behaviour change for operators. A role that holds `containers.manage` or
`stacks.manage` for the environment McHarbor runs on, without `host_self.manage`, is refused the
operations below. Other environments are unaffected, and so is McHarbor's own self-update.

## What counts as McHarbor's own

| Term | Meaning |
|---|---|
| **A McHarbor container** | This node, an HA peer node, a McHarbor helper (self-update helper, watchdog, host shell, bind-path helper), or any container labelled `com.mcharbor.protected` |
| **The self environment** | The environment whose Docker daemon holds a McHarbor container. A daemon that holds a **peer** McHarbor node's container counts too, so a second node is protected from the first. Outside a container it is the local-socket environment |
| **On a native install** | McHarbor has no container of its own, so none of the containers on its host is "McHarbor's own" unless it is a helper, an HA peer, or labelled `com.mcharbor.protected`. The host itself is still protected through the self environment |
| **A protected container's mounts** | The paths and volumes a protected container mounts join McHarbor's own protected paths and volumes on that daemon |

The HA compose file `docker-compose.ha.yml` labels its `postgres` service
`com.mcharbor.protected=true`, so the shared database is protected like the application.

## What a container may not reach without the key

On the self environment, creating or recreating a container that would reach McHarbor's host needs
`host_self.manage`, and so does a shell or file write in an existing one. The refusal is
`err.host_self.host_access_required`, and its `reasons` name what the container would reach.

**Create-time rules.** Any one of these needs the key:

| Setting | Reason key |
|---|---|
| `privileged` | `privileged` |
| `pid: host` | `pid_host` |
| `cap_add` of `SYS_ADMIN` or `ALL` | `cap_sys_admin` |
| Any other `cap_add` | `cap_add` |
| `devices` | `devices` |
| `security_opt` that removes a confinement: any seccomp profile but the built-in one, any AppArmor profile but `docker-default`, `systempaths=unconfined`, `label=disable` or `label=type:…` | `security_opt` |
| `userns_mode: host` | `userns_host` |
| `ipc: host` | `ipc_host` |
| `network_mode: host` | `network_host` |
| `uts: host` | `uts_host` |
| `cgroup: host` | `cgroup_host` |

These rules judge what a container is **created** with. A shell in a container that merely has
`NET_ADMIN` is not newly gated, because exec and file-write classification keeps the smaller set
(privileged, `pid: host`, `SYS_ADMIN`, and the host paths below).

**Namespace joins.** A `container:<id>` (or compose `service:<name>`) `pid`, `ipc`, network, `uts`
or `userns` mode joins another container's namespaces, so it is refused as `joins_namespace`.
`volumes_from` a McHarbor container is `volumes_from`.

**Host paths.** A bind whose source is, is under, or is a parent of any of these paths needs the
key. Paths are cleaned first, so `..`, `//` and a trailing slash change nothing.

| Path | Reason key |
|---|---|
| `/` | `host_root` |
| `/proc` | `host_proc` |
| `/sys` | `host_sys` |
| `/dev` | `host_dev` |
| `/etc` | `host_etc` |
| `/root`, `/home` | `host_home` |
| `/boot`, `/usr`, `/bin`, `/sbin`, `/lib`, `/lib32`, `/lib64`, `/libx32` | `host_system` |
| `/run`, `/var/run`, and the well-known Docker socket paths | `docker_socket` |
| `/var/lib/docker` | `docker_data` |
| `/opt/containerd`, `/var/lib/containerd`, `/var/lib/kubelet` | `runtime_data` |
| `/var/spool/cron` | `host_cron` |
| `/var/lib/mcharbor`, and every path or volume McHarbor itself mounts | `mcharbor_data`, `mcharbor_volume` |

Ordinary data directories such as `/srv`, `/mnt`, `/media`, `/opt/appdata` and `/data` stay free.
A bind whose source cannot be resolved at create time is `unresolved_source`.

**Symlinks.** The denylist reads the path string, and the Docker daemon follows symlinks when it
mounts. So on the self environment a host bind source that **is, or passes through, a symlink**
needs the key, as `host_bind_symlink`, wherever the link points. It covers container creates,
compose services (bind volumes, `file:` secrets and configs), and volumes that are really binds
(a new `local` bind volume, or an existing one a new container mounts).

The check runs on the Docker daemon itself, in one short-lived helper container per create (one per
compose up), and only when nothing else in the spec already needs the key:

- labelled `com.mcharbor.helper=bind-symlink-check`
- started from McHarbor's own image, which the daemon already holds, or else a local
  `alpine:3.20`, `alpine:latest`, `busybox:latest` or `debian:bookworm-slim`; it is never pulled
- `--network none`, a read-only root filesystem, `no-new-privileges`, every capability dropped
  except `DAC_READ_SEARCH`, 16 processes, 64 MiB, and a 10-second limit end to end
- the host's `/` bound read-only, and every path component of every source checked with
  `test -L`. `DAC_READ_SEARCH` lets it read into a directory another user locked to `0700`, so such
  a directory cannot hide a link. It grants no write

A source that does not exist yet is fine, because the daemon creates it as a directory under a
parent the walk has already cleared. If the helper fails, times out, finds no local image, or meets
a parent it cannot enter, the create is refused as `unresolved_source`. When McHarbor runs natively
on Linux against the daemon's own socket, the same walk runs on McHarbor's own filesystem and no
helper is started. In demo mode nothing runs, and no source counts as a symlink. A compose save
does not run the check, only an up does.

**What it cannot close.** A link planted *between* the check and the create still wins. The window
is the time from the helper's exit to the daemon's mount, usually well under a second, and planting
into it needs a container already writing to that directory at that moment. The Engine API has no
bind option that refuses to follow links, so no create-time check can close it completely. The
remaining mitigation is to keep the directories that writable containers bind away from the paths
operators bind into new containers, or to require `host_self.manage` for any host bind on the self
environment.

**Volumes that are really binds.** A named volume can be a bind in disguise, so these are
classified by the same denylist, on create and on use:

- a container create whose `Mounts` carry a `local` volume with `device` and `o=bind` options
- `POST /api/volumes` with the `local` driver and a sensitive `device` (the volume is refused, and
  audited as a denied attempt)
- a compose top-level `volumes:` entry with `driver_opts`
- an existing named volume that a new container mounts, inspected on the self environment
- moving a container, which checks the source's volume options against the target

**Compose fields.** A compose file brought up on the self environment is read for the same create
rules, and also for: `secrets` and `configs` with a `file:` source, `build.context`, `env_file`,
`label_file`, `device_cgroup_rules`, a top-level `name:` equal to McHarbor's own compose project,
and an `include:` or `extends: file:` the check cannot read. Every reason above applies per service.

## Every path that acts on a McHarbor container

| Path | What is refused without `host_self.manage` |
|---|---|
| Container routes | The operations in the [route table](../../../api/endpoints/containers.md#mcharbors-own-containers): exec, terminal, debug shell, start, stop, restart, pause, unpause, kill, update, rename, recreate, remove, move, file writes, and **network connect and disconnect** |
| Stack routes | Every lifecycle route on a stack that runs McHarbor, and the compose create-time rules above on `up`, `update`, `reinstall` and App Store installs. **Saving** a compose file runs the check too, so a bad file is refused when saved and not only when started |
| Stack adopt | `POST /api/stacks/adopt` when the target is McHarbor's own stack, matching the frontend's disabled **Take over** on a discovered McHarbor stack |
| Volumes | `POST /api/volumes` for a bind-option volume on the self environment |
| Reconciler | `POST /api/reconciler/…/reconcile` when reaching the desired state would start, stop or remove a McHarbor container. The refusal is audited |
| Backups | Starting a restore, restoring from a run, and restoring an uploaded archive into a McHarbor container (or one that reaches the host). The handler answers an audited `403` |
| Runbooks | A runbook step that starts, stops or restarts a McHarbor container (`container.start`, `container.stop`, `container.restart`) |
| GitOps | A promotion that deploys or removes a stack owned by GitOps is checked before any write, and records the refusal's code and translated text as the promotion's failure |
| Workflows | **Saving** a workflow with a node whose target is McHarbor's own (`err.host_self.workflow_required`). At run time, a guarded node in a workflow last saved without the key (`err.host_self.workflow_unauthorized`). See [Workflows → Self-protection](../../../workflows/self-protection/README.md) |

### Nobody behind it

A path that can run with no user, such as a reconciler enforcement pass, a GitOps promotion or an
unattended runbook, is **refused**, not let through. The check needs an operator's decision attached
to the request, and a background job has none. The refusal is logged. It is audited as
`host_self.denied` only when there is a request to attach it to, so the reconciler route and a
runbook step run by a person leave an audit entry, and an enforcement pass leaves a log line. The
bootstrap reconciler's unattended Recreate and StackUp on McHarbor are refused outright.

A **workflow** is the exception. Its run has nobody behind it either, so the decision is made when
the workflow is saved: saving it needs the key when a node's target is McHarbor's own, and the
save records whether the saver held the key. A run then acts on McHarbor's containers only when
that record is set. See [Workflows → Self-protection](../../../workflows/self-protection/README.md).

McHarbor's own self-update and restart (`POST /api/versions/self-update`,
`POST /api/system/restart`, under `system.manage`) are a different path and are unchanged.

## The refusal

`403` with `code` one of `err.host_self.container_required`, `err.host_self.host_access_required`
or `err.host_self.stack_required`, and `data: { permission: "host_self.manage", target, reasons }`.
Every refusal writes a `host_self.denied` audit entry at warning severity that names the operation,
the target kind and the reasons. The interface renders the affected controls disabled with the
tooltip "Requires the host_self.manage permission".

Holding `host_self.manage` does not remove the older application lock: stop, restart, pause, kill,
remove and file writes on McHarbor's own container still answer `err.protected_target`.

## Compliance

The **Container access to McHarbor's host** section of the compliance posture (`selfEnvAccess`) now
also counts **users** who hold `containers.manage` on the self environment through a **direct
permission grant**, which a review of roles never showed. It lists those users, separates the ones
without a global `host_self.manage`, and reports the reason `containersManageOverrideIsAdmin` when
any exist. An expired grant, a deny, a grant on another environment or one scoped to a stack or a
resource does not count.

## What goes wrong

| Symptom | Cause | Fix |
|---|---|---|
| Creating a container on the McHarbor environment answers `err.host_self.host_access_required` | The container would reach the host, and the `reasons` say how | Remove the setting, or hold `host_self.manage` |
| A compose save is refused | The file contains one of the rules above | Same |
| A reconcile or a restore is refused | The target is a McHarbor container | Run it as a user with `host_self.manage` |
| On a native install, ordinary containers are locked as "McHarbor's own" | A McHarbor before this fix read container ids from the host's mount table, which on a host lists every running container | Upgrade; a native install has no container of its own |
| An unattended runbook step stops | Nobody behind it | Make the step attended |
| A workflow node on McHarbor's container fails with `err.host_self.workflow_unauthorized` | The workflow was last saved by someone without the key | Someone with `host_self.manage` saves it again |
| A bind under `/srv` or `/mnt` is fine, one under `/home` is refused | `/home` is on the denylist | Use a directory outside the list |
| A bind of an ordinary path is refused with `host_bind_symlink` | The path, or one of its parent directories, is a symlink on the host | Bind the real directory the link points at, or hold `host_self.manage` |
| Every host bind on the McHarbor environment is refused with `unresolved_source` | The bind-check helper cannot run: no local image, a daemon error or the 10-second limit. McHarbor's log says which | Make sure the daemon holds McHarbor's image or `alpine:3.20`, or hold `host_self.manage` |

## Related

- [Containers](../README.md)
- [McHarbor's own containers (API)](../../../api/endpoints/containers.md#mcharbors-own-containers)
- [Workflows → Self-protection](../../../workflows/self-protection/README.md)
- [Compliance](../../enterprise/compliance.md)
- [Permission conventions](../../../api/conventions/permissions.md)
