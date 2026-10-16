# Agent Prerequisites

What the remote host needs before the agent will connect, and what changes depending on what
you intend to do with it.

## The host

| Requirement | Notes |
|---|---|
| Ubuntu Server 24.04.5 LTS or 26.04.1 LTS | The preferred distribution for an agent host, and the one compute hosting is tested on |
| Linux, macOS or Windows | The install script detects the OS and architecture. McHarbor serves `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64` and `windows/amd64` |
| Docker, for Docker management | The agent answers Docker API calls against the host's socket. Without Docker there is nothing to manage, though the agent still serves host stats, host logs and the host terminal |
| Access to `/var/run/docker.sock` | The container install mounts it; the binary install reaches it directly |
| Outbound HTTPS/WSS to McHarbor | The only network requirement. See below |
| A writable state directory | `/var/lib/mcharbor-agent` by default; the offline buffer and the instance id live there |

The distribution is a preference, not a gate: the agent manages Docker on any of the five
platforms above. Ubuntu Server 24.04.5 LTS and 26.04.1 LTS are named because a host that
intends to run **guests** has to clear the compute probe, and a stock install of either
clears cgroup v2, user namespaces and idmapped mounts without a single boot parameter
changed, which is not true of every current distribution.

## The network, in one direction

**The agent dials out. McHarbor never dials in.**

| From | To | Why |
|---|---|---|
| Agent host | McHarbor, on its HTTPS or HTTP port | The WebSocket the whole agent runs over |

That is the complete requirement for managing Docker. No inbound rule on the remote host, no
port forward, no VPN.

### The one exception: direct transfers

Moving a container or image between two agent hosts is fastest when the source agent pushes
straight to the target agent rather than relaying everything through McHarbor. That path
needs the target to be reachable:

| From | To | Why |
|---|---|---|
| Source agent host | Target agent host, TCP **8788** | Direct agent-to-agent transfer |

If it is not reachable, nothing breaks: transfers fall back to relaying through McHarbor,
which is slower. [Settings → System → Agent](../../settings/system/agent/README.md) has a
test that tells you which path you are on.

## TLS

If McHarbor is served over HTTPS, the agent dials `wss://` and **the remote host has to trust
McHarbor's certificate**. The install script the environment wizard produces sets
`MCHARBOR_FORCE_TLS=1` when the host is reachable over HTTPS.

Three ways to satisfy that:

1. A certificate from a CA the host already trusts. This is the clean answer.
2. Install McHarbor's certificate into the host's trust store.
3. `MCHARBOR_INSECURE=true`, which skips verification. Acceptable on a trusted private
   network with a privately-issued certificate; not acceptable across anything else. Trusting
   McHarbor's root on the host is the better answer; see
   [Installing the root CA](../../screens/security/install-root-ca.md).

File transfers are **pushed to** `https://<agent-host>:8788`, so an agent that receives them
needs to be trusted in the same direction.

## What it can do once it is there

Everything below travels over the one connection:

- Docker API calls: containers, images, volumes, networks, stacks
- Compose operations, staged under `MCHARBOR_COMPOSE_DIR`
- Container logs, stats, and interactive exec
- Host stats, host logs, and an interactive **host** shell
- `docker prune` run locally
- Filesystem snapshot detection and creation: ZFS, Btrfs, LVM on Linux, VSS on Windows
- Container backup and restore performed on the host, uploading the archive
- Direct agent-to-agent transfers
- Guest hosting for virtual machines and system containers, when the host qualifies

## Which hosts can do what

Docker management works on every platform above. **Guest hosting does not**: it needs Linux,
systemd, and the agent installed as a **binary** rather than in a container. macOS and Windows
agents manage Docker and nothing else.

[Supported hosts and guests](../../compute-foundation/supported-platforms.md)
has the whole matrix: which platform hosts what, which host to recommend, and what a guest
may be. [Host requirements](../../compute-foundation/host-requirements.md) has
the per-item probe and the fix for each item.

## Extra requirements for compute hosting

Running KVM virtual machines or LXC system containers through the agent asks more of the
host, and the capability probe checks each item and returns a remediation string for the
ones that fail:

| Requirement | Needed for | Why |
|---|---|---|
| **systemd as PID 1** | Both | Guests are supervised as systemd transient units, so they outlive an agent restart. A host without systemd is refused rather than given a degraded mode |
| **`/dev/kvm`, and VT-x or AMD-V enabled** | VMs | The probe distinguishes "virtualization disabled in firmware" from "the agent's user is not in the `kvm` group" |
| **QEMU (`qemu-system-*`)** | VMs | McHarbor drives it directly over QMP; there is no libvirt in the path |
| **`qemu-img`** | VMs | Creates and resizes guest disks |
| **cgroup v2 unified hierarchy** | Containers | A cgroup v1 host is refused. The remediation is `systemd.unified_cgroup_hierarchy=1` or a newer distribution |
| **User namespaces** | Containers | Unprivileged containers are built on them |
| **Subordinate uids/gids** | Containers | Each unprivileged container needs its own non-overlapping block of 65536. Missing ones do not block the capability; they silently leave every container running as root |
| **OVMF/edk2**, and **swtpm** | Optional | UEFI boot, and the vTPM Windows 11 requires |
| **Idmapped mounts** | Optional | Sharing and cloning container rootfs trees without a chown pass; an old kernel without `mount_setattr` is reported as such |
| **IOMMU** | Optional | PCI and GPU passthrough |

You do not have to take any of this on trust before committing to a host. The environment
create dialog's **Check Docker** and **Check VM readiness** buttons run the whole list over
SSH before the environment is created (read-only, installing nothing) and print the fix for
each item that fails. See [Environments](../../screens/environments/README.md).

The probe returns the package name for the host's own package manager, so the remediation is
a command you can paste. The full table, the exact "can run" formulas and the nested-
virtualization cases are in
[Host requirements for guests](../../compute-foundation/host-requirements.md).

The consequence worth planning for: **compute hosting wants the agent installed as a binary
on the host, not as a container.** A container cannot be the execution host for QEMU or LXC.
That is also the one configuration where McHarbor and an agent legitimately run on the same
machine.

## Licence seats

An agent does not claim a seat. The seat is its **environment**, counted by the McHarbor that
manages it, exactly as a direct environment is, so enabling compute on the McHarbor host costs
the one seat of that environment.

An agent reports nothing to the licence server. `MCHARBOR_LICENSE_KEY` is ignored with a
warning in the agent log, and no path in the agent binary is gated on a licence.

## Checking an SSH host before you deploy

When you deploy over SSH, the environment wizard has a **Prerequisites** panel under the SSH
fields ("Check what this host already has before deploying."). It is the same probe McHarbor runs
at deploy time, made visible so you can fix a host before pressing Create.

1. Fill in the SSH details and choose **Check host**. It needs `environments.manage` and renders
   disabled without it.
2. The panel reports **Detected &lt;os&gt; on &lt;arch&gt; (&lt;distro&gt;)** and a list of
   prerequisites. Each is present or missing, marked **— required for this method** when the deploy
   method cannot work without it, and each missing one carries the install command for the host's
   own distribution. Where none is known it says "No install command is known for this
   distribution. Install it by hand."
3. The probe can be scoped: `docker` (Docker and the Compose plugin) or `vm` (CPU virtualisation,
   `/dev/kvm`, QEMU, `qemu-img`, OVMF and `swtpm`). Unscoped, it reports everything.
4. Tick the missing items to install and choose **Install selected**. Only what you tick is
   installed, only on this host, and the install output is shown under **Install output**.

**How privileged commands run.** The panel shows **Privileged commands run:** with the mode McHarbor
detected, in this order:

| Mode | Shown as |
|---|---|
| Already root | directly (already root) |
| Passwordless `sudo` | through passwordless sudo |
| `sudo` that wants a password | through sudo, with the SSH password |
| Only `su` | through su, with the root password |
| None of these | not at all: no root, no sudo, no root password |

When `su` is the only route and no root password has been supplied, a **Root password** field
appears ("This host has no sudo, so installing uses su. The password is used for this run only and
is never stored."). Installing uses the detected mode, not a fixed `sudo -n` as it once did, so a
host with no sudo, or where you sign in as root, now installs correctly.

**Create is gated.** While a required prerequisite is missing, or the ticked packages cannot be
installed because the host offers no way to run privileged commands, the panel says so and the
Create button stays disabled. With nothing required missing it reads "Ready to continue. Nothing
required is missing." When the deploy runs at create time, the root password you entered is sent
with it.

Routes: `POST /api/agents/probe` and `POST /api/agents/install-prereqs`, both `environments.manage`.
There is no command-line equivalent yet.

## Before you install, decide two things

**Where the state directory lives.** `MCHARBOR_DATA_DIR` holds the offline buffer and the
instance id. Mount it as a volume for a container install, or the buffer survives restarts
but not recreates, and a fresh instance id claims a fresh licence seat.

**Container or binary.** Container is simpler and updates cleanly. Binary is required for
compute hosting and is the better fit on a host that already runs everything under systemd.

## Related

- [Installation](../installation/README.md): the three install paths
- [Configuration](../configuration/README.md): every environment variable
- [Use cases](../use-cases/README.md): what people actually use agents for
