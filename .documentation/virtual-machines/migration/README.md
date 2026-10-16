# Virtual machines: migration

**Move to another host** relocates a machine, its disks and its definition, to another compute
host McHarbor manages, and removes the original once the move succeeds. There are two kinds.

| Kind | What happens | Downtime |
|---|---|---|
| **Cold** (the default) | The machine is shut down (clean shutdown, 120 seconds before the host escalates), its disks are copied through McHarbor, and it is started on the target if it was running. | The whole copy. |
| **Live** (**Move while running**) | The disks and memory are mirrored host to host while the machine runs, and it pauses briefly at the end. | A short pause at the switchover. |

Live migration is written and unit-tested but has not yet been run between two real hosts.
Treat a first live move as a trial, and try a cold move first.

## What a machine must not have

Each of these is refused up front, before anything moves, rather than being dropped on the way: a
guest that arrives without its snapshots or its vTPM is not the guest that left.

| Refusal | Code | What to do |
|---|---|---|
| Snapshots | `err.vm.migrate_snapshots` | Delete or merge them first |
| A virtual TPM | `err.vm.migrate_tpm` | Not supported yet |
| Passed-through host devices | `err.vm.migrate_devices` | Detach them |
| Shared host directories | `err.vm.migrate_shares` | Remove the shares |
| Installer media or any CD-ROM disc | `err.vm.migrate_media` | Eject it |
| A template that has linked copies | template in use | Move or delete the copies first |
| The target is the machine's own host | `err.vm.migrate_same_host` | Pick another host |
| A move already running for the machine | `err.vm.migrate_in_flight` | Wait or cancel |
| A host whose agent cannot do it | `err.vm.migrate_unsupported` | Update the agent, or use a cold move |
| A live move of a stopped machine | not running | Start it, or use a cold move |

The capabilities are reported by each agent as `migrate_cold` and `migrate_live`, and both hosts
must report the one you asked for. The live capability needs the QEMU on each host to implement
`migrate`, `migrate-incoming`, `nbd-server-start`, `blockdev-mirror` and `object-add`. A machine that
runs with SPICE cannot be moved live to a host whose QEMU has none.

## Doing it

1. Open the machine and choose **Move to another host**. It is offered when the machine's host
   reports `migrate_cold`, and needs `vm.migrate`.
2. Pick the **Target host**. A target needs compute turned on and its agent connected, or the list
   reads "No other host can take this machine."
3. Pick a **Storage pool** on the target.
4. Choose **Move while running** for a live move. It is disabled with "Only a running machine can
   be moved live." when the machine is stopped, and with the reason when the hosts lack
   `migrate_live`.
5. Optionally set **Networks on the target**. An interface left on its current bridge needs a
   bridge of the same name on the target.
6. Live only, optionally: **Target address** (empty uses the first address the target reports),
   **Bandwidth limit (MB/s)** and **Longest pause (ms)**.
7. **Review**, read the confirmation, and choose **Move machine**.

The move continues on the hosts if you close the dialog. Progress is polled, with **n% done**
and the current step, and **Stop the move** cancels it. A finished move reads "The machine now
runs on <target>"; a failed or stopped one reads that the machine is still on its original host.

## Permissions

`vm.migrate` is scoped to an environment and is checked on **both** hosts: the one the machine
leaves and the one it moves to. Holding it on only one is refused with `err.vm.migrate_forbidden`
("You need permission to move machines on both hosts."). It is in the `operator` preset.

| Route | Key | Audit |
|---|---|---|
| `POST /api/vms/{id}/migrate` | `vm.migrate` | `plugin.kvm.vm.migration_started` |
| `GET /api/vms/{id}/migration` | `vm.view` | none (read) |
| `POST /api/vms/{id}/migration/cancel` | `vm.migrate` | `plugin.kvm.vm.migration_cancelled` |

Completion and failure are audited as `plugin.kvm.vm.migration_completed` and
`plugin.kvm.vm.migration_failed`. The status carries no token, certificate or key.

Command line: `mcharbor kvm vm migrate`, `mcharbor kvm vm migration` and
`mcharbor kvm vm migration-cancel`.

## What has to be reachable

- **Cold.** Each agent connects **outbound to McHarbor**, so both hosts must be able to reach
  McHarbor's address as they already do for their connection. The disks stream through McHarbor
  over two one-time, per-migration relay URLs (`/api/vms/migrations/internal/{grantId}`), and
  nothing is stored on McHarbor. The UEFI variable store moves with the disks.
- **Live.** The source host connects **directly to the target host** on two TCP ports, one for
  the disk mirror (NBD) and one for the memory transfer. The target picks both at random when the
  move starts, so there is no fixed range to open: the hosts must be able to reach each other
  freely on TCP, or the move fails. The address the source dials is the **Target address** you set,
  otherwise the first one the target reports. The channel is mutual TLS, with a throwaway
  certificate authority and certificates issued by McHarbor for this migration alone, valid for
  seven hours and deleted afterwards.

A migration is bounded at seven hours end to end.

## Failure and cancel

Until the target runs the machine and McHarbor has recorded that, the source's copy is untouched.
Every failure before that point removes what the target received and, for a cold move, starts the
source again if it was running. Cancelling runs the same rollback.

After the move, McHarbor removes the machine and its disks from the old host. If that removal
fails the move still counts as complete, and the status carries a warning that the old disks could
not be removed from the previous host.

## Related

- [Virtual machines](../README.md)
- [Clone](../clone/README.md)
- [Host requirements for guests](../../compute-foundation/host-requirements.md)
