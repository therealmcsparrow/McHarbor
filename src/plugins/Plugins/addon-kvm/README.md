# McHarbor Virtual Machines

KVM virtual machines on your own hosts — as an installable McHarbor add-on.

Install it from **App Store → Plugins → Virtual Machines**. Installing it also
installs **Compute Foundation** (`addon-compute`), which owns the storage pools,
networks, guest images and per-host capability checks this plugin builds on.

## What it does

| Area | What you get |
|---|---|
| Guests | Create from a cloud image or an ISO, start, shut down, stop, reset, pause, resume |
| Disks | Add, resize and remove; qcow2 on a directory pool, native volumes on zfs, btrfs and lvm |
| Network | Virtio interfaces on a bridge, with optional VLAN tags |
| Snapshots | Per pool mechanism, with consistency reported honestly — crash-consistent without a guest agent |
| Clones | Linked (copy-on-write) or full, with a fresh machine UUID and a regenerated cloud-init instance id |
| Templates | Mark a stopped guest as a template and clone from it |
| Console | VNC and serial in the browser, over a single-use ticket |
| Passthrough | PCI, USB and raw host block devices, behind `vm.privileged` |
| Diagnosis | Optional AI reading of a guest that will not start |

A cloud image to a login prompt in the browser takes under two minutes.

## What it stores

Six tables under `plugin_addon_kvm_`: `vms`, `disks`, `nics`, `snapshots`,
`cloud_init`, `devices`.

The host restricts each plugin to its own table prefix, so **this plugin cannot
read Compute Foundation's rows**. That is why a disk carries its pool's backend
and target alongside the pool id, and an interface carries its bridge alongside
the network id. The denormalisation is deliberate: the agent acts on a target,
not an id, so those are the values it needs anyway, and copying them at create
time means a guest keeps working if its pool row is later renamed.

No plaintext password is ever stored or written to a seed image. The create
request carries one, the service hashes it (SHA-512 crypt, `crypto/rand` salt)
before the row is written, and only the hash reaches the cloud-init seed — which
is readable by anything that can read the pool.

## Two things it is careful to tell the truth about

**Guest state.** The agent is the only authority. `unknown` is a first-class
state, entered whenever the owning agent is offline, because guessing that a VM
is still running because it was ten minutes ago is how a management UI lies. A
guest whose host is unreachable is greyed with its actions disabled rather than
shown with stale state and buttons that will fail.

**Snapshot consistency.** Without the QEMU guest agent a snapshot is
crash-consistent — the equivalent of pulling the power cord. That is recorded
per snapshot and shown next to the name, because six months later it is the
difference between a restore that boots and one that runs fsck.

## Permissions

Five keys, declared in `plugin.yaml` under `adds_permissions`:

| Key | Preset | Grants |
|---|---|---|
| `vm.view` | viewer, editor, operator | See guests, hardware, snapshots, usage |
| `vm.manage` | editor, operator | Create, edit, start, stop, resize, snapshot, clone |
| `vm.delete` | operator | Destroy a guest and its disks; delete snapshots |
| `vm.console` | operator | Open the VNC or serial console |
| `vm.privileged` | **none** | PCI and USB passthrough, raw host block devices |

`vm.console` is separate from `vm.manage` because reaching a console is
effectively root inside the guest, and plenty of operators want to grant
start/stop without it — the same reasoning that separates `containers.manage`
from exec.

`vm.privileged` is in no role preset. Each of the things it grants can
compromise the host itself rather than only the guest.

## Where the work happens

Not in this plugin. The McHarbor process cannot host guests: a
`docker compose up -d` would kill every running VM, taps created in its network
namespace would sit behind Docker's NAT rather than on the LAN, and the
privileges required amount to `privileged: true` plus host networking.

So the executor lives in the **remote agent** and is part of core — a plugin
cannot extend the agent protocol or its dispatch, and the agent binary is a
versioned core artifact. This plugin reaches it through `host.AgentRPC`
(`agent.rpc` scope token plus an `agent-rpc` capability), which is nil when no agent transport is wired; the
guest list then renders and explains that no host can run guests, rather than
the section refusing to load.

## What this does not contain

No libvirt, no Proxmox, no third-party hypervisor UI. McHarbor drives
`qemu-system-*` directly over QMP and supervises it with systemd transient
units. libvirt is a daemon rather than a library, it keeps its own domain
definitions and storage pools that then drift from McHarbor's, and everything
needed from a running guest is a QMP command anyway — so talking to QMP directly
removes a translation layer, not a safety layer.

Machine types are pinned at creation as a concrete version (`pc-q35-8.2`, not
`q35`) and never silently changed. An unpinned machine type is how guests break
on a host package update.

## Contributions

- **Workflow nodes** — `vm-lifecycle`, `vm-snapshot`, `vm-clone`, `vm-state`.
- **Dashboard widgets** — `vm-summary`, `vm-resources`.
- **Reports** — backup assurance, guest stability and snapshot schedules for the
  Reporting plugin's Report Studio, plus the inventory, capacity, protection,
  health and cost facets of its shared reports.
- **Alert conditions** — crashed, stopped unexpectedly, state unknown, snapshot
  chain too deep, guest agent gone, autostart failed.
- **i18n** — the `vm` namespace, six locales.

## Build note

The frontend bundle pins `@novnc/novnc` to 1.6.0 and applies a build-time
transform. noVNC ends `lib/util/browser.js` with a top-level `await` for a
WebCodecs probe, and top-level await cannot be expressed in UMD — rollup refuses
the bundle outright. 1.7 is worse: it drops the transpiled `lib/` tree for
untranspiled ESM carrying the same await. The `mcharbor:strip-novnc-tla` plugin
in `frontend/vite-plugin.config.ts` rewrites that one statement into a `.then()`;
the flag has a `false` initialiser and its only reader consults it mid-connection,
long after module evaluation. The pin and the transform are one decision.
