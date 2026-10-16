# Agent Use Cases

What people actually install agents for, and what each case asks of you.

## A host behind NAT or a firewall

**The case.** A VPS, a box at home, a machine on a customer network. You can reach the
internet from it; nothing can reach it from outside.

**Why the agent.** It dials out. No port forward, no VPN, no dynamic DNS, no exposed Docker
socket.

**What to set.** Nothing beyond the URL and the token. If direct transfers between hosts are
not possible on that network, either leave them to fall back to the relay or disable the
receiver outright with an empty `MCHARBOR_TRANSFER_LISTEN`.

**Watch out for.** Idle connections through consumer NAT get dropped silently. The ping
interval in [Settings → System → Agent](../../settings/system/agent/README.md) is what keeps
them alive.

## A homelab with several machines

**The case.** A NAS, a mini PC, an old laptop, each running a few containers, all managed
from one McHarbor.

**Why the agent.** One environment per machine, every container in one list, one set of
backup plans and alert rules across all of them.

**What to set.** Install the agent on each host. Give environments names you will recognise
at 2am. Use the environment switcher, and **All Environments** where an aggregate makes
sense.

**Watch out for.** Keep the agents current. A current agent pushes its container stats once
per interval, whatever the container count; an older one is pulled at about 2s per container,
which is nothing on a handful of containers and leaves gaps in history on a few hundred. Each
environment's Overview says which one it is; see [Container stats](../stats/README.md).

## Not exposing the Docker socket

**The case.** You could reach the host with a TCP Docker endpoint, and you would rather not.

**Why the agent.** An unauthenticated Docker daemon on the network is root on that host for
anyone who can reach the port. TLS-protected Docker is better, and still means distributing
client certificates and opening a port. The agent authenticates with a token you can
regenerate in one click, and needs nothing inbound.

**What to set.** Nothing special. Regenerate the token if it is ever exposed; it invalidates
the old one immediately.

## Running virtual machines and system containers

**The case.** KVM guests or LXC system containers on your own hardware, managed from
McHarbor.

**Why the agent.** The compute add-ons drive QEMU and libcontainer **through** the agent,
because a plugin cannot extend the agent protocol and a container cannot be the execution
host for guests.

**What to set.** Install the agent **as a binary** on the host, with systemd. The capability
probe then checks systemd as PID 1, cgroup v2, `/dev/kvm` and virtualization extensions,
QEMU and OVMF, and idmapped mounts, and returns a remediation string for anything missing
rather than a generic failure.

**Watch out for.** This is the one case where McHarbor and an agent legitimately run on the
same machine. Two instance ids are minted; the licence server collapses them into one seat
using the machine fingerprint.

## Moving containers between hosts

**The case.** A container has outgrown a box, or you are retiring one.

**Why the agent.** With agents on both ends, the source pushes the image and archive straight
to the target on port 8788 instead of relaying every byte through McHarbor.

**What to set.** Allow TCP 8788 from source to target. Check the path with the
agent-to-agent transfer test before you rely on it. It reports both agent versions, the
advertised URL, the URL actually probed, the HTTP status, the duration and a responder marker
confirming the answer came from the expected agent.

**Watch out for.** Where the direct path fails, transfers still work through the relay. The
failure is a speed problem, not a correctness one, but it is worth knowing which path a
900 MB move is taking.

## Sites that lose connectivity

**The case.** A boat, a building site, a rural link, a lab that gets unplugged.

**Why the agent.** Containers keep running whatever McHarbor knows. The agent buffers Docker
events, container stats and its own log locally in a bounded SQLite ring, reconnects with
exponential backoff up to a minute, and replays the buffer in acknowledged batches.

**What to set.** `MCHARBOR_OFFLINE_BUFFER=true` is the default; make sure
`MCHARBOR_DATA_DIR` is a real volume so the buffer survives a container recreate.
`MCHARBOR_OFFLINE_METRICS_INTERVAL` controls how much metric history you accumulate while
away.

**Watch out for.** The buffer is bounded (20,000 log rows, 50,000 events, 200,000 metric
samples) and drops the oldest first. Weeks offline means the most recent history is kept,
not all of it.

## Backups on the far side of a slow link

**The case.** A remote host with a lot of data and a thin uplink.

**Why the agent.** Backup and restore run **on the host**: the agent creates the archive
locally and uploads it, rather than streaming the container's filesystem through McHarbor
twice. Snapshot detection and creation also happen there, because only the agent can see the
host's PATH and its ZFS, Btrfs, LVM or VSS tooling.

**What to set.** Configure a snapshot driver on that environment if the filesystem supports
one, then use Snapshot consistency on the backup plan.

**Watch out for.** If the snapshot cannot be taken when the backup runs, the container is
paused instead and the reason is recorded on the run.

## Diagnosing a host that is misbehaving

**The case.** Docker will not start, or the disk is full, and you are not next to the
machine.

**Why the agent.** It serves host stats, a bounded host log snapshot, and an interactive
**host** shell. That shell is the most privileged surface in the product, and it exists
because you cannot diagnose a broken Docker from inside a container.

**What to set.** Nothing. It is the environment detail page's Console group.

**Watch out for.** Host terminal access is audited as an event, not as a transcript. Treat
who holds `environments.manage` accordingly.

## Related

- [Prerequisites](../prerequisites/README.md): what each case needs from the host
- [Protocol](../protocol/README.md): how each of these travels over one connection
- [Environments](../../screens/environments/README.md): creating and operating agent environments
