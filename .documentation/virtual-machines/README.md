# Virtual Machines

Runs KVM virtual machines on your own hosts: create from a cloud image and reach a login prompt in the browser in about two minutes, or install from an ISO. Disks, network interfaces, snapshots, clones and templates, with a VNC console.

- **Slug** `addon-kvm`, **version** 2026.09.01
- **Host range** `>=2026.09.01`
- **Where it appears** its own section in the sidebar, and its own pages under `/plugins/<slug>`
- **Gated by** the permission keys it declares (see below)

## Before you install: what the host needs

A virtual machine needs a host that can actually run one, and **a containerised agent cannot**:
it has no systemd, no QEMU and no `/dev/kvm`. Guest hosting needs the agent installed as a
**binary under systemd** on the target host.

Four checks must pass, and the plugin refuses to create a VM unless all four do:

| Check | Fix if it fails |
|---|---|
| `systemd` as PID 1 | A systemd host. There is no degraded mode |
| `/dev/kvm` | Enable VT-x / AMD-V in firmware, load `kvm_intel` / `kvm_amd`, put the agent's user in the `kvm` group |
| `qemu-system-*` | Install the QEMU system emulator |
| `qemu-img` | Install `qemu-img` (`qemu-utils` on Debian/Ubuntu) |

UEFI guests also want `ovmf`, and Windows 11 additionally wants `swtpm`; neither blocks a VM
from running. **Compute → Hosts → \<host\>** checks every item on a live host and prints the
command that fixes each one.

Full detail, including nested virtualization and the exact conclusion formula:
[Host requirements for guests](../compute-foundation/host-requirements.md). To go from nothing
to a guest you can log into, follow
[Getting started with guests](../compute-foundation/getting-started.md).

## How it works

- McHarbor drives QEMU directly over QMP and supervises it with systemd. There is no libvirt, no Proxmox and no third-party hypervisor UI in the path, which is the reason the host needs systemd and the agent.
- It requires [Compute Foundation](../compute-foundation/README.md), which is installed with it, and consumes `compute.view` from it.
- `vm.console` and `vm.privileged` are separate keys. Console access is a full interactive session on the guest, and privileged operations change what the guest may do to the host. Neither belongs inside `vm.manage`.
- Four widgets, five workflow nodes, and the `vm` translation namespace.
- What the host's QEMU can actually do is probed rather than assumed, and every
  feature that depends on it renders disabled with the missing piece named. See
  [QEMU features](./qemu-features.md); the same probe is what warns when a
  machine is pinned to a machine type its host no longer offers.
- Hardware (disks, installer discs, network interfaces and passed-through host devices) is
  changed from the machine's **Hardware** tab. Passed-through host devices need the machine
  stopped. Data disks, installer discs and network interfaces also change while it runs, when
  the host reports `hotplug`; see [Hot-plug](./hot-plug/README.md) for what a running machine
  refuses.
- Cloud-init user-data comes back from `GET /api/vms/{id}` and `mcharbor kvm vm get` with
  credential-looking values replaced by `••••••••`: everything under a key such as `passwd`,
  `chpasswd` or `*_token`, `NAME=value` assignments with a credential-looking name, and any URL
  carrying a password. `GET /api/vms/{id}/cloud-init` and `mcharbor kvm vm cloud-init` return
  it as stored; they need `vm.manage` and are audited as `plugin.kvm.vm.cloud_init_revealed` at warning severity.
  A create whose user-data still contains `••••••••` is refused. The stored password is only
  ever reported as set or not set.
- A cloud image is configured from a **cloud-init seed**: `seed.iso` in the guest's runtime
  directory on the host (`/var/lib/mcharbor/compute/vm/<guestId>/`), written when the machine is
  defined and attached as a read-only CD-ROM with no place in the boot order. It is attached
  until the machine has booted with it once (the agent then writes `seed.delivered` beside it),
  because cloud-init only reads it on the first boot and a CD-ROM left attached would block live
  migration. Changing the machine's cloud-init writes a new seed, which is attached again on the
  next start. The wizard's user gets a sudo rule and joins `adm` only, so the account is created
  on Debian- and RHEL-family images alike.
- The wizard's password and SSH keys can come **from the secret vault**. **From the vault**
  beside the password swaps the field for a secret picker, and **SSH keys from the vault** adds
  keys from `ssh_key` secrets (one key per line) to any typed ones. Both list the secrets visible
  in the machine's environment and need `secrets.view` there, besides `vm.manage`; without it
  they are disabled and name the key. The value is read on the server, as the creating user, and
  **copied once**: cloud-init applies it on the first boot only, so rotating the secret later
  does not change the machine. The secret's usage list shows the machine as **Copied once**, and
  deleting the machine removes it from that list. A typed password and a password secret
  together are refused. From the CLI: `mcharbor kvm vm create --password-secret <NAME>
  --ssh-key-secret <NAME>` (repeatable).
- An installer disc is attached as a read-only CD-ROM that boots after the disks, so a machine
  left with its disc in does not reinstall itself on every restart. Ejecting removes the disc
  from the machine and never deletes the image from the host.
- Importing a machine from another hypervisor runs as a background job, **on the host**. The
  uploaded disk streams through McHarbor to the agent once and stays there, so the inspection
  and the conversion run against a path on the host rather than pulling the file across a
  second time. The dialog does not close on an outside click or on Escape, only on its close
  button, on Cancel, or when the import finishes, and closing it leaves the job running, with
  a chip on the machine list to reopen it. The import list carries progress, a **Cancel**
  button, and a **Conversion** tab listing every staged file that has not become a machine yet,
  with convert and delete on each. The percentage beside **Converting** is the host's own
  qemu-img progress, relayed from the agent; a machine with several disks fills one bar across
  all of them, and the figure never moves backwards within a step. In an active/active cluster
  the conversion percentage appears only when the node running the import is also the node
  holding that host's agent; otherwise it stays at 0 until the conversion finishes. A job whose
  request died (a closed tab, a McHarbor restart)
  is recovered rather than left saying *inspecting* forever. The convert form carries Secure
  Boot and vTPM, and machine names take spaces and punctuation.
- A **Logs** tab on the machine detail page reads the guest's own systemd units. Two sources:
  the QEMU unit, and the software TPM separately. They are kept apart because a vTPM that
  died takes the guest down with it and writes nothing to QEMU's log, which is the shape a
  Windows install that hangs on its final reboot usually has. The agent builds the unit name
  from the guest id and never accepts one, so this cannot be pointed at another unit on a
  root-privileged host. Needs an agent at **3.8.0 or newer**.
- **Power controls sit beside the console** (pause and resume, reset, shut down, stop), so the
  guest you are watching can be stopped without leaving the page. They are gated on `vm.manage`,
  not `vm.console`: reaching a console is permission to look, not to act.
- **A machine's address comes from the QEMU guest agent inside it**, not from McHarbor. Without
  that agent installed in the guest there is nothing to report and the address stays blank;
  on Windows it is `qemu-ga` from the virtio-win media, which a plain install does not place.
  The **guest-agent disc** button (on the machine header, the list row, the card and the
  Hardware tab's media section) attaches an installer disc built for that guest's family,
  Windows getting the virtio payload with it. The agent builds the disc into its image cache
  ahead of the click, so the request is a stat rather than a 31 MB download, and the disc is
  recorded as an ordinary CD-ROM you eject like any other.
- **Snapshots are a tree.** Branches expand and collapse, a snapshot's name takes spaces and
  punctuation, notes are a textarea, and memory snapshots work. Reverting to an earlier
  snapshot **keeps the branch below it**, the way VirtualBox and Hyper-V behave, rather than
  discarding it: take a snapshot after a revert and it becomes a new branch. A revert on a
  running guest offers to stop it, revert and start it again, so it is one action rather than
  three. Taking a snapshot closes the dialog and reports its progress on the row (quiescing,
  pausing, writing memory, freezing the disks) instead of holding a modal open for minutes.
  Schedules use the readable/advanced control every other schedule in McHarbor uses.
- **The console has three kinds**: **VNC**, **Serial** and **SPICE**, chosen under **Console type**. It keeps its connection when you switch tab or navigate away; only the **Disconnect** button closes it. SPICE needs a host QEMU built with it, and a **Remote viewer file** button downloads a `.vv` file for `remote-viewer`. See [SPICE console](./spice-console/README.md).
- **The Logs tab uses the container runtime log's own toolchain**: text and regex filtering,
  level and time range, newest or oldest first, live tail and export.
- **The list row and the card carry the detail page's actions**: diagnose, console, start,
  shutdown, resume, pause, reset, hard stop and delete, so the two surfaces no longer
  disagree about what you can do from where.
- **A guest owns a directory.** Its disks, snapshots, overlays, sockets, NVRAM, TPM state and
  manifests live under `<pool target>/<guestId>/`, keyed by the guest's id rather than its
  name, and every qcow2 backing reference is written relatively, which is what makes moving a
  pool's data survivable.
- A machine can be created with **no image** at all. It is defined with its disk, its network
  and nothing to boot from, and starts at the firmware prompt, which is what a disk that
  already holds a system, a network install, and a disc attached later all need. Neither this
  path nor the installer-disc path runs cloud-init, so neither asks for a login.
- The **Storage** step can adopt a disk instead of allocating one. The list is every volume the
  host reports in the chosen pool minus every volume a machine already refers to, so a disk left
  behind by a machine deleted without its volumes can be booted again. Nothing is written to an
  adopted disk. Reading that list needs `vm.manage` and an agent new enough to answer
  `vm_disk_list`; an older agent reports that the disks could not be read, and creating a new
  disk still works.
- **Any stopped machine can become a template.** **Make a template** sits on the list row, the
  card and the machine's page header; on a template the same button reads **Turn back into a
  virtual machine**. Both confirm first, need `vm.manage`, and are disabled while the guest is
  running, paused or stopping. Templates are listed under **Virtual Machines → Templates**.
- **A machine can be created from a template.** **From a template** is the fourth source in
  the create dialog, beside **From a ready-made image**, **Install from a disc** and **No image**. It lists the
  templates on the chosen host and skips straight to the summary, because the template fixes
  size, disks and network. **Share data with the original** makes an instant linked copy that
  stores only what changes afterwards, and the template cannot then be deleted while the copy
  exists; off, the disks are copied in full. Either way the new machine gets its own identity,
  and **Create and start** starts it once the clone is done. It uses the clone route
  (`POST /api/vms/{id}/clone`, `vm.manage`), so it is audited as `plugin.kvm.vm.cloned`.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `SupportsFor` | Yes |  |

Its manifest also declares `runbook-provider` (`vm`), `backup-row-actions`, `estate-source`
(`vms`), `lifecycle-subject` (`vm`), `picker-source` (`vm`, the workflow VM picker) and
`workflow-node-alias` (`vm-trigger`). See
[Capability declarations](../development/contracts/capabilities.md).

## What it contributes

| | |
|---|---|
| Routes | 32 |
| Sidebar entries | 2 |
| Dashboard widgets | `vm-summary`, `vm-resources`, `vm-protection`, `vm-imports` |
| Workflow nodes | `vm-lifecycle`, `vm-snapshot`, `vm-clone`, `vm-state`, `vm-trigger` |
| Migrations | `0001_init.sql` |
| Backend | 9 Go files, ~5,093 lines |
| Requires | `addon-compute` |

### VM trigger

**VM Trigger** (`vm-trigger`) starts a workflow when a guest changes state. It needs `vm.view` to
appear in the palette.

| Field | Key | Notes |
|---|---|---|
| Environment | `environment` | Blank matches every environment |
| Virtual Machine | `vm` | Picked from `GET /api/vms` through the plugin's `picker-source`, or typed as an id or a name. Blank matches every guest |
| States | `states` | `running`, `paused`, `stopping`, `stopped`, `crashed`, `migrating`, `defined`, `deleting`. Blank matches every state |
| Severity | `severity` | `any`, `success`, `info`, `warning`, `error` |

It fires on the same transitions the Logging timeline records: a change of stored state, never
the reconciler re-asserting the same one. `msg.payload` carries `trigger: "vm"`, `vm.id`,
`vm.name`, `environmentId`, `action`, `state`, `severity`, `timestamp`, `autoTriggered`, and
`metadata` and `error` when the guest failed. `msg.topic` is `vm-trigger`.

Run by hand from the editor, it reports the current state of the first guest its filters accept,
with `autoTriggered: false`, or `matched: false` when none does.

Workflows saved while the host shipped this node store it as `vm-trigger` rather than
`plugin:addon-kvm:vm-trigger`. The manifest's `workflow-node-alias` capability resolves them to
this node, so they run unchanged. While the plugin is uninstalled they keep the node on the
canvas, and it does not fire.

## Permissions it adds

| Key | Scope | What it allows |
|---|---|---|
| `vm.view` | env | View virtual machines |
| `vm.manage` | env | Manage virtual machines |
| `vm.delete` | env | Delete virtual machines |
| `vm.console` | env | Open a virtual machine console, including SPICE and its `.vv` file |
| `vm.migrate` | env | Move a virtual machine to another host, cold or live; needed on both hosts |
| `vm.privileged` | env | Attach host devices to virtual machines |

It also consumes `compute.view` from another plugin, a key it
relies on but does not own.

## In this section

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Backups](./backups.md) | Scheduled, retained, off-host copies of a machine's disks, incremental after the first, and what makes a restore trustworthy |
| [SPICE console](./spice-console/README.md) | The SPICE console in the browser, the `.vv` file for remote-viewer, what the host QEMU needs, and serial resize |
| [Hot-plug](./hot-plug/README.md) | Adding and removing disks and interfaces, swapping discs and moving a link while the machine runs, and each refusal |
| [Display profiles](./display-profiles/README.md) | The SPICE options a machine starts with: three built-in presets, custom profiles, and the per-machine choice |
| [Clone](./clone/README.md) | Full and linked copies of a stopped machine, what the copy gets, and the linked-copy rules |
| [Migration](./migration/README.md) | Cold and live moves between hosts, what a machine must not have, what must be reachable, and `vm.migrate` |
| [QEMU features](./qemu-features.md) | Resizable memory, processor hotplug, hugepages, NUMA, shared directories, I/O limits and disk checks, and why each is offered only where the host can do it |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../development/add-ons/virtual-machines.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Report catalog](../reporting/reports/README.md): the three virtual machine reports this plugin contributes, and [the facets it answers](../reporting/facets/README.md) for the shared reports
- [Add-ons](../plugins/addons.md): what the six have in common, and what the plugin runtime does with them
- [Plugin catalog](../plugins/catalog.md): every shipped plugin
