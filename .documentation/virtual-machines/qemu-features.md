# QEMU features

What McHarbor can ask QEMU to give a guest beyond processors, memory and a disk:
resizable memory, processor hotplug, hugepages, NUMA placement, shared
directories, a direct host channel, per-disk I/O limits and image diagnostics.

Every one of these depends on what the **host's own QEMU** was built with, so
none of them is offered unconditionally. McHarbor asks the host what it can do
and disables what it cannot, naming the missing piece. The alternative is a
control that looks available and fails at use time with QEMU's own wording.

## The capability gate

At capability-probe time the agent asks the host's `qemu-system-*` three
questions it cannot answer from a version number alone:

| Question | How | What it decides |
|---|---|---|
| Which devices can it emit? | `-device help` | virtio-mem, virtiofs, vhost-vsock, virtio-gpu, the U2F token |
| Which CPU models does it know? | `-cpu help` | What the processor-model picker offers |
| Which QMP commands does it implement? | `query-qmp-schema` on a throwaway `-machine none` QEMU | Live resize, I/O throttling, dirty bitmaps |

The host's hugepage pool (`/proc/meminfo`) and NUMA layout
(`/sys/devices/system/node`) are read at the same time.

Nothing is started to answer this. The throwaway QEMU allocates no memory, runs
no guest and exits immediately. The schema is a property of the binary, and
probing through a running guest's monitor would make the answer depend on
whether one happened to be running.

The answer is cached per host for ten minutes. A host whose QEMU was upgraded
picks the new capabilities up on the next probe rather than needing a restart.

`GET /api/vms/capability?env=<id>` returns it; reading it needs `vm.view`.

### Machine types

The probe also records the machine types the host offers. McHarbor pins a
concrete versioned type per machine (`pc-q35-8.2`, never `q35`) at creation,
and QEMU removes old types on a schedule, so a machine pinned to one that a
package upgrade removed will not start. The machine's Overview tab marks the
pinned type when the host no longer offers it, **before** the start rather than
after it.

## Resizable memory

`virtio-mem` lets a running machine's memory change in both directions. It is
the modern replacement for DIMM hotplug, where unplug essentially never works.

- Enabled per machine under **Settings → QEMU features → Resizable memory**,
  and applies at the machine's next start.
- Needs a **memory ceiling above the boot size**. The ceiling is what reserves
  address space for the growth, and it is fixed when the machine starts.
- Replaces the balloon. QEMU documents the two as incompatible, so exactly one
  is given to any machine.
- Resize from **Hardware → Live size changes**. Needs `vm.manage`.

The guest decides how fast it complies, and on a shrink it may decline
entirely; freeing the memory is the guest's job. McHarbor reports **what the
machine ended up with**, not what was asked for, and says so when the two
differ. The host also rounds requests to its block size (2 MiB normally, larger
with hugepages), so the figure shown after a resize is the rounded one.

## Processor hotplug

Processors are added and removed on a running machine through the same panel.

- The **processor ceiling** under **Settings → QEMU features** is what reserves
  the slots. It is fixed at start, so raising the ceiling needs a restart while
  changing the count within it does not. The two are separate controls for
  exactly that reason.
- Removal is a request. Windows guests and Linux guests without the right udev
  rules routinely decline, and McHarbor reports the count the machine actually
  has rather than the one requested.
- Needs `vm.manage`.

## Hugepages

Backs a machine's memory with hugepages, typically 5 to 15% faster on
memory-heavy workloads and required by some passthrough configurations.

McHarbor **never reserves hugepages on your behalf**. That takes memory away
from everything else on the host, and it is a host configuration decision. The
toggle is available only when the host already has a pool, and says so when it
does not.

Two consequences worth knowing before enabling it:

- Hugepage memory **cannot be swapped**. An over-committed host running
  hugepage-backed machines runs out of memory rather than degrading.
- Memory is preallocated at start, so a large machine takes proportionally
  longer to start, which reads as a hang on a machine with 64 GB.

## NUMA placement

On a host with more than one memory domain, a large machine's memory is
otherwise spread across all of them and its performance becomes unpredictable.

Three policies rather than a topology editor, because hand-building a NUMA
layout is not a reasonable thing to ask:

| Policy | What it does | When |
|---|---|---|
| **No placement** | Nothing. The default | Single-domain hosts, small machines |
| **Keep on one domain** | Memory and processors bound to one host domain | The machine fits inside one domain |
| **Mirror the host's layout** | One guest domain per host domain | The machine is too large to fit in one |

Pinning shows the free memory and processor count per domain at the moment you
choose, because a machine pinned to a domain with nothing left fails to start
with an error naming neither the domain nor the reason.

Available only when the host reports more than one domain.

## Shared directories

`virtiofs` shares a directory from the host into the machine with near-native
performance and proper POSIX semantics, which 9p never managed.

**This is a privilege boundary, not a convenience.** The daemon runs as root on
the host, and a badly chosen path hands the host to the guest. So:

- The whole feature is gated on **`vm.privileged`**, alongside device
  passthrough, never `vm.manage`.
- An administrator sets the **allowed roots** per host first. With none
  configured, every share is refused. There is no default of `/`, and `/`
  cannot be entered as a root.
- Each share names a directory inside an allowed root, a **mount tag** the
  guest mounts by, and read-only or read-write. Read-only is the default.
- Each share runs its own `virtiofsd`, started before QEMU and stopped with it,
  sandboxed in its own mount namespace so a symlink inside the shared directory
  cannot reach the rest of the host.

The machine's **Hardware → Shared directories** lists each share with the exact
line to run inside the guest:

```
mount -t virtiofs <tag> /mnt/<tag>
```

Shares attach when the machine starts. Adding or removing one takes effect at
its next start. Detaching a mounted filesystem from a running machine hangs
every process inside it holding a file open.

Sharing also changes how the machine's memory is allocated: `virtiofsd` has to
map the guest's RAM, so a machine with shares gets a shareable memory backend.

## Direct host channel

`vhost-vsock` gives host and guest a socket channel with no network involved:
no address, no interface, no firewall rule. Useful for reaching a machine that
has no working network.

The context id is **allocated by McHarbor** and unique per host, enforced by a
database constraint. It is shown read-only: a collision is silent and lands the
channel on the wrong machine, so it is not something to type in.

## Per-disk I/O limits

Caps how hard one machine can hit the host's storage. Without them, a machine
running a restore starves every other machine on the host.

- Separate read and write limits, in operations per second and in MB/s. Read
  and write are separate because a restore saturates writes while reading
  almost nothing, and one number for both throttles the wrong half.
- Set from **Hardware → the disk's I/O limits**. Needs `vm.manage`.
- Applied to a **running** machine immediately, and stored so a restart keeps
  them.
- A bandwidth limit under 1 MB/s is refused: it produces a machine that looks
  hung rather than slow.
- Zero means no limit, which is the default.

A related setting, **dedicated I/O thread** under Settings → QEMU features,
moves a machine's disk I/O onto its own thread so heavy disk use does not stall
the rest of the machine. One thread per machine.

## Shared disk access

A disk can be opened by two machines at once. This is required by a cluster
filesystem and **catastrophic without one**: two machines writing an ordinary
filesystem through two independent page caches corrupt it.

Off by default, per disk, changed only while the machine is stopped, and never
offered on a disc. The switch carries the warning.

## Disk diagnostics

Read-only `qemu-img` checks on a machine's disk, run by the host, from
**Hardware → inspect the disk**:

| Check | Answers |
|---|---|
| Image details and backing chain | What the image really is, and the whole chain rather than just the top |
| Consistency check | Whether the image is intact, with leaks and corruption counted |
| Which parts come from where | Which ranges come from which backing file |
| What this image really takes | Actual and fully-allocated size on this storage |

**Repair** is offered only while the machine is stopped, and the host refuses
it regardless if the machine is running. Repairing an image a live machine has
open corrupts it, and only the host can tell whether it is open.

The destructive `qemu-img` subcommands are not reachable. `dd`, `bench` and
`amend` are deliberately outside the allow-list: `bench` would run against a
live machine's disk and `amend --force` can erase an encryption key.

## Virtual security key

An emulated U2F token, for guests testing WebAuthn sign-in. Nothing is passed
through from the host, so no physical key has to be plugged into it.

Attached from **Hardware → attach a device**, alongside PCI, USB and block
passthrough, and gated on `vm.privileged` with them.

## SPICE, cloning, migration and serial resize

Five more capabilities are probed and reported in the same answer, under these keys:

| Feature key | How it is decided | Gates |
|---|---|---|
| `spice` | The host's QEMU implements `query-spice`, which only a SPICE build does | The SPICE console and the `.vv` file. See [SPICE console](./spice-console/README.md) |
| `clone` | Answered by the agent's own code, so true on every agent that knows the feature | Reported; see [Clone](./clone/README.md) |
| `migrate_cold` | Answered by the agent's own code | **Move to another host**. See [Migration](./migration/README.md) |
| `migrate_live` | QEMU implements `migrate`, `migrate-incoming`, `nbd-server-start`, `blockdev-mirror` and `object-add` | **Move while running** |
| `serial_resize` | Answered by the agent's own code | Resizing the serial terminal, which still needs the guest agent inside the guest |

An agent older than these features does not report the key, which reads as unsupported, so the
control is disabled with the reason instead of failing at use time.

## Run states

QEMU distinguishes more machine states than "running" and "stopped", and two of
them matter enough to surface on their own:

- **Disk error**: QEMU stopped the machine because a write to one of its disks
  failed, usually a full pool. The machine resumes once there is space. This
  previously read as *Running*, which was actively misleading.
- **Kernel panic**: the guest's own operating system crashed. It is still
  loaded but not running; a reset is the only way out.

Long-running block operations (a snapshot still flattening, a mirror) are
readable through `GET /api/vms/{id}/block-jobs` with `vm.view`, so an operation
that takes minutes does not read as a machine that has hung.

## Guest agent

Where the QEMU guest agent is installed, McHarbor asks the machine what it
thinks it is: its own addresses, its real operating system name and version,
and its hostname. All of it is guest-reported, displayed as text, and never
used to build a link or make an access decision.

The agent is also used for two things the machine cannot do for itself:

- **Filesystem-consistent snapshots.** The filesystems are frozen for the
  duration and thawed afterwards, including on every failure path, and on the
  next reconcile if McHarbor itself died holding the freeze. A machine left
  frozen stops answering every write, which is worse than the crash-consistent
  snapshot this improves on.
- **Shutdown when ACPI is ignored.** A machine sitting on an "unsaved work"
  dialog drops the ACPI request; the agent is asked to shut down from inside
  before the machine is stopped hard. The escalation is still reported, so a
  machine that needs this is visible rather than merely slow.

`guest-exec` (running arbitrary commands inside the machine) is deliberately
**not** exposed. It is a remote shell with no audit story.

## Permissions

Apart from `vm.migrate`, no permission keys were added for any of this. Each control uses the key
that already governs its kind of change:

| Key | Covers |
|---|---|
| `vm.view` | Reading the host's capabilities, block jobs, shares and the guest's own report |
| `vm.manage` | Resizing memory and processors, I/O limits, disk checks and repair, every Settings → QEMU features toggle |
| `vm.privileged` | Shared directories, the allowed share roots, the virtual security key |
| `vm.console` | The SPICE console and the `.vv` file |
| `vm.migrate` | Moving a machine to another host, checked on both hosts |

## What goes wrong

| Symptom | Cause |
|---|---|
| A toggle is disabled with "this host's QEMU cannot do this" | The host's QEMU was built without that device or command. Nothing McHarbor can configure around |
| Memory resize is disabled on a running machine | The machine has no resizable memory device. It is a start-time choice; enable it and restart |
| A resize reports fewer than asked | The guest declined, which is its right, particularly on a shrink, and particularly on Windows |
| Adding processors fails with "raising that needs a restart" | The target is above the ceiling the machine started with |
| A machine with hugepages will not start | The host does not have enough free hugepages. They are not swappable and not allocated on demand |
| Sharing a directory is refused | The path is outside the host's allowed roots, or no roots are configured yet |
| "This disk is already open by another guest" | Another machine has it. Stop that machine, or enable shared access if both run a cluster filesystem |

## See also

- [Virtual Machines](README.md): the plugin itself
- [Configuration](configuration.md): the settings that are not QEMU features
- [Troubleshooting](troubleshooting.md)
- [Host requirements for guests](../compute-foundation/host-requirements.md)
