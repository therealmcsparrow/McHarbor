# Getting Started with Guests

From nothing to a virtual machine or system container you can log into. Seven steps, in the
order the dependencies actually run; each one is useless until the one before it is done.

If you only read one thing first, read
[Host requirements](./host-requirements.md). Step 1 is the decision that most often gets made
wrong, and it is the expensive one to undo.

## 1. Pick the machine, and install the agent on it *natively*

Guests run on the machine the agent runs on, never inside McHarbor, and **never inside a
containerised agent**. The agent image has no systemd, no QEMU and no `/dev/kvm`, and no
amount of configuration adds them.

So on the machine that will host guests, use the **binary install** and let systemd run it:

```bash
curl -fsSL https://mcharbor.example.com/api/agent/install/<token> | sh
```

The script detects that this host has no Docker-only intent, installs
`/usr/local/bin/mcharbor-agent`, writes a systemd unit and starts it. Get the token from
**Environments → Add environment → Agent**.

That machine may be the same one McHarbor runs on. A server and an agent on one host are
collapsed into a single licence seat by machine fingerprint, so this costs nothing extra.

**Check before continuing:** the environment shows as connected, and its agent version is
**3.0.0 or newer**. Managed and shared guest networks are refused below that, with a message
saying so.

### Install the host's guest dependencies

Before moving on, run this on that host. It installs QEMU, OVMF, `swtpm`, the KVM module and
a subuid range, then tells you what is still missing:

```bash
sudo ./scripts/compute/install-host-dependencies.sh
```

`--check` reports without changing anything and needs no root. Full detail in
[scripts/compute/README.md](../../scripts/compute/README.md).

## 2. Install the plugins

**App Store → Plugins**:

| Plugin | Slug | What it gives you |
|---|---|---|
| Compute Foundation | `addon-compute` | Hosts, storage pools, guest networks, the image library. Required by both below |
| Virtual Machines | `addon-kvm` | KVM guests, consoles, snapshots, clones, templates |
| System Containers | `addon-lxc` | LXC guests: lighter, no hypervisor needed |

Installing either guest plugin installs Compute Foundation with it. Install only the one you
need; they are independent.

Then grant the keys to the roles that should see them, or the sidebar sections stay hidden:
`compute.view` and `compute.manage`, plus `vm.*` or `ct.*`. Console access (`vm.console`,
`ct.console`) and device passthrough (`vm.privileged`, `ct.privileged`) are separate keys on
purpose: a console is a full interactive session on the guest.

## 3. Run the setup wizard

**Compute → Set up compute** walks the four things that must exist, in dependency order.

### Choose a host

Only agent environments appear. If the machine you want is missing, step 1 is not finished.

### Check the host

**This step is not decorative, and it is where you find out whether step 1 was right.**
Every item was checked on the host itself. Items marked required must be fixed before a guest
will run, and each one prints the command that fixes it, resolved for that host's own package
manager. Paste it, then re-probe.

A host reporting four required failures is almost always a containerised agent. See
[Host requirements](./host-requirements.md).

### Storage

Where guest disks live.

| Backend | Use it when | Why it matters |
|---|---|---|
| `zfs` / `btrfs` | The host already has a dataset or subvolume | Snapshots and clones complete instantly instead of copying the whole disk |
| `lvm` | The host uses LVM | Thin snapshots |
| `dir` | Anything else | A plain directory. Always available; snapshots copy data |

`dir` works everywhere and is the right starting choice if you are unsure. It can be added to
later; a host can hold several pools.

### Network

How guests reach the network. Four modes, and the first one is both the common case and the
safe one:

| Mode | What McHarbor does | Guests get addresses from |
|---|---|---|
| **Bridged** | Attaches the guest's own interface to a bridge **you already have**. Touches nothing else | Your own DHCP server |
| **Managed** | Creates its own bridge, hands out addresses on it, and masquerades traffic out through an interface you pick | McHarbor |
| **External** | Attaches one of the host's adapters to a McHarbor bridge and moves the host's address onto it | Your own DHCP server |
| **macvlan** | Stacks each guest directly on an adapter with its own MAC address. The adapter is not reconfigured | Your own DHCP server |

**Bridged** if the host already has a bridge. Nothing about the host's networking changes.

**Managed** on a host with one NIC and no bridge. Choosing the interface that carries the
host's default route is correct here and is not dangerous: it only names where masquerading
exits. The adapter itself is never touched.

**External** is the Hyper-V "external switch" / VMware "bridged" equivalent: guests sit
directly on your LAN. It does reconfigure the adapter, so it asks for an explicit
acknowledgement, and the agent puts everything back after 45 seconds unless McHarbor confirms
it can still reach the host. Try it on a spare adapter before your only one.

**macvlan** puts guests on your LAN without touching the host at all, also on an adapter
that already carries Docker macvlan containers. The one catch: the host and its guests
cannot reach each other unless you turn on host access and give the host a free address in
the LAN's prefix. If a guest never gets an address, see
[A guest gets no address](./guest-networking.md#a-guest-gets-no-address).

## 4. Get an image

**Compute → Images.**

**From the catalog:** pick a distribution and press Download. The agent fetches it straight
from the mirror to the host and verifies the checksum; the image never travels through
McHarbor.

**Your own image:** upload a qcow2, a rootfs archive or an ISO. Uploads land in a shared
directory McHarbor writes and every host mounts on its own cache root, so the file is simply
already present on the host rather than streamed over the agent transport. That needs one
bind per host:

```
-v /srv/mcharbor/compute-images:/var/lib/mcharbor/compute/images
```

and `MCHARBOR_COMPUTE_IMAGE_LIBRARY_PATH` pointing at the same storage on McHarbor's side.
On separate machines that storage has to genuinely be shared: NFS, SMB, a replicated dataset.
A local directory on each will look right and never produce a usable image. See
[Configuration → The shared image library](./configuration.md#the-shared-image-library).

Cloud images boot to a login prompt fastest. ISOs mean an interactive install over the
console.

## 5. Create the guest

**Virtual Machines → Create**, or **System Containers → Create**.

You are asked for a name, the image, CPU and memory, the pool from step 3 and the network
from step 3. Two things worth getting right the first time:

- **Cloud image + cloud-init** gives you a guest with your SSH key and a set password at
  first boot. An ISO gives you an installer.
- **UEFI** needs `ovmf` on the host, and **Windows 11** additionally needs `swtpm`. Both are
  non-blocking checks in the capability panel. A VM without them still runs; it just cannot
  boot those.

## 6. Open the console

Both guest types have a console in the browser: VNC for a graphical VM, serial for a system
container. This is where a cloud-init guest first shows a login prompt, typically within a
couple of minutes of pressing Start.

Console access is gated by `vm.console` / `ct.console` separately from `manage`.

## 7. Check the addresses

**Compute → Networks → \<network\> → Addresses** shows what a managed network actually handed
out: the guest's MAC, its address, and the name the guest reported for itself.

On a bridged or external network this page stays empty, and that is correct: your own DHCP
server issued the lease, not McHarbor.

## When it does not work

| Symptom | Almost always |
|---|---|
| "This host cannot run virtual machines" | A containerised agent, or a missing host package. Read the capability panel; it names which |
| The create form has no pool or no network | Step 3 was not finished for *this* host |
| Guest networks offer no bridge and no outbound interface | The agent cannot see the host's interfaces: a containerised agent, or one installed with `--isolated` |
| A managed network's guest gets no address | Agent older than 3.0.0. Before it, DHCP did not exist |
| A guest has an address but reaches nothing | IPv4 forwarding off, or the masquerade rules could not be installed |
| "The agent on this host is too old" | Managed and shared networks need agent 3.0.0+ |

More in [Troubleshooting](./troubleshooting.md) and
[Agent troubleshooting](../agents/troubleshooting/README.md).

## Related

- [Host requirements](./host-requirements.md): every check, and why a containerised agent cannot host guests
- [Configuration](./configuration.md): pools, networks and the shared image library in full
- [Virtual Machines](../virtual-machines/README.md) · [System Containers](../system-containers/README.md)
- [Installing an Agent](../agents/installation/README.md): the container and binary paths
