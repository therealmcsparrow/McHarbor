# McHarbor System Containers

Full-system LXC containers — a whole distribution with its own init, users and
services — as an installable McHarbor add-on.

Install it from **App Store → Plugins → System Containers**. Installing it also
installs **Compute Foundation** (`addon-compute`), which owns the storage pools,
networks, rootfs images and per-host capability checks this plugin builds on.

## Why this and not the Docker section

A Docker container is one process tree wrapped around one application image. A
system container is a machine: it boots systemd, runs cron and sshd and syslog,
holds users, and is administered the way a server is.

That is the wrong shape for shipping an application and the right shape for a
lightweight VM — which is the gap this fills. Most homelab guests are running a
distribution, not a hypervisor workload, and a system container gives them that
at roughly a tenth of a virtual machine's memory, starting in about a second.

## What it does

| Area | What you get |
|---|---|
| Containers | Create from a rootfs image or a template, start, shut down, stop, restart, freeze and thaw |
| Resources | CPU, memory, swap and process limits that apply to a **running** container — no restart |
| Mounts | Volumes inside the container's own pool; host bind mounts and device passthrough behind `ct.privileged` |
| Processes | The container's live process list, read from its cgroup — nothing inside has to cooperate |
| Snapshots | Per pool mechanism (zfs, btrfs, lvm, or an rsync copy on a directory pool), with the container optionally frozen for consistency |
| Clones | Copy-on-write from a template on a CoW pool; a full copy anywhere else, and the UI says which |
| Console | A root shell on the container's pty, over a single-use ticket |
| Diagnosis | Optional AI reading of a container that will not start |

## Live resource limits

The one thing a system container does that a virtual machine cannot. cgroup v2
applies limits to a running container: changing a VM's memory means a reboot,
changing a container's means one write to `memory.max`.

This plugin takes that seriously rather than treating limits as edit fields:

- Limits have their own route, not a field in the update body.
- The response carries what the **kernel applied**, not what was requested. The
  kernel rounds and clamps, and the response names the fields it changed —
  echoing the request back would leave somebody believing a limit that is not in
  force.
- Usage reads against the container's own limit, never the host. A container at
  95% of a 512 MB limit is minutes from an OOM kill on a host with 128 GB free.
- The `ct-limits` workflow node can route to its error port when the kernel did
  not accept a value, so a scheduled job that raises memory for a batch window
  notices when it silently did not get it.

lxcfs is deliberately not required, so `free` and `/proc/cpuinfo` inside a
container report the host's figures. The numbers here read the cgroup and are
correct; the resources panel says so rather than requiring a second daemon to
make `free` look right.

## What it stores

Five tables under `plugin_addon_lxc_`: `containers`, `nics`, `mounts`,
`snapshots`, `provisioning`.

The host restricts each plugin to its own table prefix, so **this plugin cannot
read Compute Foundation's rows**. That is why a container carries its pool's
backend and target alongside the pool id, and an interface carries its bridge
alongside the network id. The denormalisation is deliberate rather than a
workaround: the agent acts on a target, not an id, so those are the values it
needs anyway, and copying them at create time means a container keeps working if
its pool row is later renamed.

No plaintext password is ever stored. The create request carries one, the
service hashes it (SHA-512 crypt, `crypto/rand` salt) before the row is written,
and only the hash reaches the agent.

## Permissions

Five keys, declared in `plugin.yaml` under `adds_permissions`:

| Key | Preset | Grants |
|---|---|---|
| `ct.view` | viewer, editor, operator | See containers, mounts, snapshots, processes, usage |
| `ct.manage` | editor, operator | Create, edit, start, stop, freeze, snapshot, clone, change limits |
| `ct.delete` | operator | Destroy a container and its rootfs; delete snapshots |
| `ct.console` | operator | Attach a console — a root shell inside the container |
| `ct.privileged` | **none** | Privileged mode, host bind mounts, device passthrough, nesting |

`ct.console` is separate from `ct.manage` because a console is a root shell, and
plenty of operators want to grant start/stop without it — the same reasoning
that separates `containers.manage` from exec.

`ct.privileged` is in no role preset. Root in a privileged container is root on
the host in every way that matters, and a host bind mount gives the container
everything behind that path. It is also the one key checked inside a handler
rather than only by route middleware, because whether a mount needs it depends
on the request body: a volume inside the container's own pool takes `ct.manage`,
a bind of an arbitrary host path takes `ct.privileged`. The service revalidates
the source and the agent validates it again — the backend checks are UX and
defence in depth, the agent's is the security boundary.

## Where the work happens

Not in this plugin. The McHarbor process cannot host containers: a
`docker compose up -d` would kill every running one, and creating a user
namespace and a delegated cgroup subtree from inside a container is not
something to rely on.

So the executor lives in the **remote agent** and is part of core — a plugin
cannot extend the agent protocol or its dispatch, and the agent binary is a
versioned core artifact. This plugin reaches it through `host.AgentRPC`
(`agent.rpc` scope token plus an `agent-rpc` capability), which is nil when no agent transport is wired; the
container list then renders and explains that no host can run containers, rather
than the section refusing to load.

The agent must be built with CGO — libcontainer's `nsenter` is C — so the agent
image builds `CGO_ENABLED=1` against musl with a static-linkage assertion. An
older agent cannot run system containers and says so through the capability
probe.

## What this does not contain

No LXD, no Incus, no `lxc-*` toolchain. McHarbor uses libcontainer directly.
LXD and Incus are daemons with their own database of containers, profiles and
storage pools, which then drifts from McHarbor's; and everything needed is a
namespace, a cgroup and a mount, all of which libcontainer already does.
Removing the daemon removes a translation layer, not a safety layer.

## Contributions

- **Workflow nodes** — `ct-lifecycle`, `ct-limits`, `ct-snapshot`, `ct-clone`,
  `ct-state`.
- **Dashboard widgets** — `ct-summary`, `ct-density`.
- **Reports** — isolation for the Reporting plugin's Report Studio, plus the
  inventory, capacity, protection, health and cost facets of its shared reports.
- **Alert conditions** — crashed, stopped unexpectedly, state unknown, memory
  pressure, process pressure, autostart failed.
- **i18n** — the `ct` namespace, six locales.
