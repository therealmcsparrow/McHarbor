# Virtual machines: clone

**Clone** makes a second machine from a stopped one, on the same host and in the same storage
pool. It is also what **From a template** in the create dialog uses.

## Doing it

1. Stop the machine. **Clone** in the machine's header is offered only while the machine is
   stopped, and a running guest is refused by the route (`409`) and by the agent ("stop it before
   cloning"), because a disk that is being written is a disk from no single moment.
2. Choose **Clone** (it needs `vm.manage`).
3. Give the copy a **Name for the copy** and pick a **Kind of copy**.

| Kind of copy | What it does |
|---|---|
| **Full copy** | A complete, independent copy of every disk. It takes longer and uses the full space, and nothing depends on the original. |
| **Linked copy** (**Share data with the original**) | Instant, and stores only what changes afterwards. The original cannot be deleted while a linked copy exists. |

The dialog shows progress as **Copying disks… n%** from the machine's `cloneProgress`, and closes
when the copy exists.

## What the copy gets

Everything that makes a guest itself is regenerated, because a clone that kept them would answer
to the original's name with the original's host keys:

- a new id, machine UUID and **new hardware (MAC) addresses**, so the two can share a network
  segment
- a new cloud-init instance id and a hostname derived from the new name, so the first-boot setup
  runs again
- autostart turned off
- each network interface on the same network or bridge as the original, and each disk as a
  volume in the source's pool

Installer media and other CD-ROM discs stay the same file on the host; only real disks are copied
or backed by the source.

A failed or cancelled clone removes the volumes it had made, so no half-guest is left behind.

## Linked copies

A linked copy of a disk is backed by the original's disk. The consequences:

- The original **cannot be deleted** while a linked copy exists. This holds for templates and for
  any other machine that has a linked copy: the error names the copies.
- The original's disks must stay where they are. A linked copy shares the pool, which is why it
  is made in the source's own pool.
- A template with linked copies cannot be migrated to another host. See
  [Migration](../migration/README.md).

## Permissions and audit

| Step | Key |
|---|---|
| `POST /api/vms/{id}/clone` (body `name`, `linked`) | `vm.manage`; audited as `plugin.kvm.vm.cloned` |

Command line: `mcharbor kvm vm clone <id> --name <name> [--linked]`.

## Related

- [Virtual machines](../README.md)
- [Migration](../migration/README.md)
