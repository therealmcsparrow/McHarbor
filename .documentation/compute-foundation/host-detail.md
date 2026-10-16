# The host detail page

One compute host, on one page: what it is, how it is configured, whether it is ready to
carry a guest, and what it has been saying. It exists so the answer to "why will this host
not run anything" does not require an SSH session.

Reach it from **Compute → Overview**: the host's name in the table, the info action in the
row, or **Host details** on the card. The URL is
`/plugins/addon-compute/hosts/<environment id>/<tab>`, so a tab is linkable.

- **Gated by** `compute.view` for the page, `compute.manage` and `compute.delete` for the
  changes in Settings, and the host's own `logs.view` for the operating-system log sources.

## Overview

Read-only. The operating system, architecture, kernel and agent version the last capability
probe reported, when it ran, whether the shared image library is mounted, the host's pools
with their capacity and target paths, its networks, and how many images it has cached. A
capability the host does not support carries the host's own explanation behind the error
icon rather than a generic sentence.

## Settings

Four sections, every control disabled with a tooltip naming the missing permission rather
than hidden.

| Section | What it changes | Needs |
|---|---|---|
| Host | Compute enabled, shared image library, default storage pool, default network | `compute.manage` |
| Storage locations | Where the host keeps VM state, container rootfs trees and cached images | `compute.manage`, and `compute.delete` to clear an override |
| Storage pools | Create, refresh, move and delete this host's pools | `compute.manage`, `compute.delete` to delete |
| Networks | Create, edit and delete this host's networks | `compute.manage`, `compute.delete` to delete |

**Shared image library** is a statement, not a probe. McHarbor cannot see whether the host
really mounts the library; switching it on tells McHarbor that the host mounts
`/var/lib/mcharbor/compute/images`, and an image downloaded once is then already there.

**Storage locations** resolve per field, in this order: what is set for this host, then the
McHarbor-wide default, then the agent's own. Each field's badge says which of the three it
came from. Paths must be absolute. What is shown after a save is **what the agent took**,
not what it was asked for. A host declines a move that would strand a running guest, and
the section then says *Not applied on the host* and offers **Retry on host**. Clearing an
override returns the field to what it inherits; it moves no data by itself.

## Tests

The readiness checklist answers "can this host carry a guest yet", one line per condition:
agent reachable, host checked, host supported, can run virtual machines, can run system
containers, storage pool present, network present, shared image library mounted. Each is
*Ready*, *Worth checking* or *Blocking*, and a blocking one carries the host's own message.

Below it is the same capability panel the setup wizard shows, and it re-runs the probe. A missing package row the host's agent can install has an **Install** button
([Host requirements → Installing from the capability panel](./host-requirements.md#installing-from-the-capability-panel)).
This is the page to open before blaming a guest that will not start.

## Logs

One viewer over eight sources, so a failing host can be read without a terminal. Pick a
source, a line count (1 to 1000) and an optional text filter.

| Source | What it reads | Needs |
|---|---|---|
| Compute activity | McHarbor's own record for this host: pools created and removed, and anything else compute put on the timeline | a session |
| System | `/var/log/syslog`, `/var/log/messages`, else `journalctl` | `logs.view` |
| Kernel | `/var/log/kern.log`, `/var/log/dmesg`, else `journalctl -k` | `logs.view` |
| Authentication | `/var/log/auth.log`, `/var/log/secure`, else the ssh journal | `logs.view` |
| Docker | `/var/log/docker.log`, else the docker journal | `logs.view` |
| libvirt | `/var/log/libvirt/libvirtd.log`, else the libvirtd journal | `logs.view` |
| McHarbor agent | the `mcharbor-agent` journal | `logs.view` |
| Guest (QEMU) | one guest's log under `/var/log/libvirt/qemu/` | `logs.view` |

Choosing **Guest (QEMU)** with no guest selected asks the host which guests have a log and
fills the guest picker from the answer; pick one to read it. The guest name is checked
against a strict character set and resolved inside the qemu log directory only.

The last three sources need an agent of version **2026.09.01, build 202609231806 or
newer**. They arrived after the four operating-system sources, and a calendar version
alone cannot tell two agents built in the same month apart. An older agent is named as
such rather than returning an empty log. Rebuilding the agent is
`scripts/development/build-agent.sh`; a native agent must then be installed by hand,
because it self-updates only on a version increase.

A source that read nothing says why (no such log on this host, no `journalctl`,
permission denied) rather than showing an empty pane.

## What goes wrong

| Symptom | Cause | Fix |
|---|---|---|
| The page says the host is not here | Compute was never switched on for it, or the environment was removed | Compute → Overview, then **Set up compute** |
| Every Settings control is disabled | The account lacks `compute.manage` | The tooltip names the key; grant it on the role |
| The Logs tab shows a permission notice | The account lacks the host's `logs.view` | Compute activity still works without it |
| libvirt, agent or guest logs report an outdated agent | That host's agent predates build 202609231806 | Rebuild the agent and install it on the host |
| A storage location says *Not applied on the host* | The host was offline, or a guest is still running on the old location | Stop the guest, then **Retry on host** |
| The readiness checklist blocks on *Can run virtual machines* | The host has no KVM, or the agent runs in a container | See [Host requirements](./host-requirements.md) |

## Related

- [Host requirements](./host-requirements.md): what a host needs before any of this passes
- [Configuration](./configuration.md): the same settings at the section level
- [Troubleshooting](./troubleshooting.md): symptoms across the whole feature
