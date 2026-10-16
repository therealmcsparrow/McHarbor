# Host Requirements for Guests

What a host needs before it can run KVM virtual machines or LXC system containers, why each
item is needed, and how to fix the ones that fail.

Every check on this page is performed by the agent itself and reported on
**Compute → Hosts → \<host\>**. Nothing here has to be verified by hand. The panel names
what is missing and prints the command that fixes it, resolved for that host's own package
manager. This page exists so the answer is knowable *before* you build the host.
[Supported hosts and guests](./supported-platforms.md) is the inventory beside this checklist:
which platforms are supported at all, which host to recommend, and what a guest may be.

## Decide this first: container or native agent

**A containerised agent cannot host guests.** Not "runs them with reduced features"; it cannot.

| | Container agent | Native agent (binary + systemd) |
|---|---|---|
| Docker and Compose management | Yes | Yes |
| Host stats, logs, terminal | Yes | Yes |
| Backups, snapshots, transfers | Yes | Yes |
| Storage pools | Yes | Yes |
| Guest networks | Yes, with `--network host --cap-add NET_ADMIN` | Yes |
| **Virtual machines** | **No** | Yes |
| **System containers** | **No** | Yes |

Three separate reasons, and none is a packaging oversight:

- **systemd.** McHarbor supervises every guest as a systemd transient unit, which is what
  lets a guest survive the agent restarting or self-updating. The agent image runs `tini` as
  PID 1, not systemd.
- **QEMU.** The agent image ships no `qemu-system-*` and no `qemu-img`. Adding them would
  clear two checks and leave the other two.
- **`/dev/kvm`.** A container reaches the hypervisor only if the device is passed in *and*
  the kernel underneath actually exposes it.

So the supported arrangement for guest hosting is the **binary install**: the agent on the
host, under systemd. This is also the one case where McHarbor and an agent legitimately run
on the same machine; the licence server collapses them into one seat by machine fingerprint.

See [Installing an Agent → As a binary with systemd](../agents/installation/README.md).

**Preferred distribution:** Ubuntu Server 24.04.5 LTS or 26.04.1 LTS. A stock install of
either clears `systemd`, `cgroup2`, `userns` and `idmapped` with no boot parameter changed and
no kernel to replace, which leaves only the QEMU packages and the subuid range to arrange. Any
systemd distribution that passes the probe below is supported; these two are the ones McHarbor
is tested on.

## One command that installs most of it

`scripts/compute/install-host-dependencies.sh` in the McHarbor repository installs the
packages, loads the KVM module, grants a subuid range and then reports whatever is left:

```bash
sudo ./install-host-dependencies.sh            # both guest types
sudo ./install-host-dependencies.sh --vm       # virtual machines only
./install-host-dependencies.sh --check         # report only, changes nothing, no root
```

Debian/Ubuntu, RHEL/Fedora, SUSE and Arch package names are mapped. It exits non-zero
while anything required is missing, so it can gate a provisioning step.

Two things it cannot do: install the agent, and turn on virtualization in your firmware. It
names both rather than failing obscurely. The agent's own probe remains the authority; the
create dialogs gate on that, not on this script.

Before any of that, **Check VM readiness** in the environment create dialog answers whether
the host needs the script at all: it probes the CPU flag, `/dev/kvm`, `qemu-system-x86_64`,
`qemu-img`, OVMF and `swtpm` over SSH and installs nothing.
[Environments](../screens/environments/README.md) describes both checks.

## Installing from the capability panel

A missing QEMU, `qemu-img`, OVMF or `swtpm` row in the host's capability panel carries an
**Install** button when the host's agent can install it itself: it runs as root, the host is
Debian, Ubuntu, RHEL, Fedora, Rocky or AlmaLinux, and `apt-get` or `dnf` (`yum`) is there.
The button needs `compute.manage`, asks first and shows the command the probe suggested,
and the agent then runs its package manager (`apt-get update` first on Debian-family hosts).
On success the panel shows the host's fresh capability; on failure the dialog stays open
with the host's message and the tail of the package manager's output.

McHarbor sends only the row's name. The agent alone decides which packages that means,
from its own table, so neither the button nor the route can install anything else:

| Row | Debian / Ubuntu | Fedora | RHEL, Rocky, AlmaLinux |
|---|---|---|---|
| QEMU | `qemu-system-x86` (`qemu-system-arm` on arm64) | `qemu-system-x86` (`qemu-system-aarch64`) | `qemu-kvm` |
| `qemu-img` | `qemu-utils` | `qemu-img` | `qemu-img` |
| OVMF, OVMF Secure Boot | `ovmf` (`qemu-efi-aarch64`) | `edk2-ovmf` (`edk2-aarch64`) | `edk2-ovmf` (`edk2-aarch64`) |
| `swtpm` | `swtpm`, `swtpm-tools` | `swtpm`, `swtpm-tools` | `swtpm`, `swtpm-tools` |

Every install is audited at warning as `plugin.compute.capability.installed`, with the
packages and whether it succeeded. The route is
`POST /api/compute/environments/{envId}/capability/install` with `{"item": "qemu_img"}`, and
the CLI is `mcharbor compute environment install <envId> --item qemu_img`. It needs an agent
at `2026.10.01` build `202610050946` or newer; an older agent shows no button. Kernel
features, firmware settings and the subuid range have no button: they are not packages.

## Installing the VM libraries by hand

The script above is the short path. This is the same work spelled out, for a host that is
provisioned by configuration management, or where the script cannot run.

Five things matter, and only the first two are non-negotiable:

| What | Why | Blocks a VM |
|---|---|---|
| `qemu-system-x86_64` | The emulator McHarbor drives over QMP. There is no libvirt in the path | Yes |
| `qemu-img` | Creates, resizes and converts guest disks | Yes |
| OVMF / edk2 | UEFI firmware. BIOS guests boot without it; Windows 11 and Secure Boot do not | No |
| `swtpm` | The software TPM 2.0 Windows 11 requires | No |
| QEMU built with SPICE | The SPICE console and the `.vv` file for remote-viewer. Debian and Ubuntu ship it as a separate module, `qemu-system-modules-spice`; SUSE, Arch and Alpine packages have it built in; RHEL 9 and later `qemu-kvm` does not. The agent reports it as `spice` only when its QEMU actually starts with a SPICE server, and the UI disables both SPICE controls with the reason when it does not | No |

### Debian, Ubuntu, and derivatives

```bash
sudo apt-get update
sudo apt-get install -y qemu-system-x86 qemu-utils ovmf swtpm swtpm-tools
```

`qemu-system-x86` is the package that provides the `qemu-system-x86_64` binary. There is no
package by that name on Debian, which is the single most common reason a hand-written
playbook fails here.

### RHEL, Fedora, Rocky, AlmaLinux

```bash
sudo dnf install -y qemu-kvm qemu-img edk2-ovmf swtpm swtpm-tools
```

On RHEL derivatives `qemu-kvm` installs `/usr/libexec/qemu-kvm` and, on most versions, the
`qemu-system-x86_64` symlink the probe looks for. Where it does not, install
`qemu-system-x86-core` as well.

### openSUSE and SLES

```bash
sudo zypper install -y qemu-x86 qemu-tools qemu-ovmf-x86_64 swtpm
```

### Arch

```bash
sudo pacman -S --noconfirm qemu-full edk2-ovmf swtpm
```

**Alpine is not supported**, as a guest host or any other kind. It runs OpenRC with
systemd absent, and the probe refuses a host without systemd, so there is no package list
for it here.

### Then the hypervisor itself

Packages do not load the kernel module, and nothing installs VT-x for you:

```bash
# 1. Does the CPU advertise virtualization at all?
grep -Eom1 '(vmx|svm)' /proc/cpuinfo || echo "enable VT-x / AMD-V in BIOS/UEFI"

# 2. Load the module, and keep it loaded across reboots
sudo modprobe kvm_intel || sudo modprobe kvm_amd
echo kvm_intel | sudo tee /etc/modules-load.d/kvm.conf   # kvm_amd on AMD

# 3. Let the account the agent runs as open the device
sudo usermod -aG kvm "$USER"

# 4. Confirm
ls -l /dev/kvm
qemu-system-x86_64 -accel kvm -version
```

Step 3 matters even when the agent runs as root today: an agent installed under a service
account fails `kvm_device` with the device plainly present, and that is the failure that
reads as a paradox on the panel. **Check VM readiness** in the environment create dialog
separates the two cases: it tests `/dev/kvm` for read *and* write as the SSH user, not
merely for existence.

### Guest networking and storage, if you need them

A guest with no network still boots. When you want one on the LAN, the host needs a bridge
and the agent needs to be able to see it: `bridge-utils` or the distribution's own bridge
tooling, plus `NET_ADMIN`. [Guest networking](./guest-networking.md) has the whole story.
Storage pools need nothing extra for a directory pool; ZFS, Btrfs and LVM pools need their
own userspace tools installed already.

### System containers, for comparison

LXC system containers need no hypervisor. Their extra package is the subordinate-id tooling
(`uidmap` on Debian, `shadow-utils` on RHEL, `shadow` on SUSE and Arch), plus a
non-overlapping block of 65536 subordinate ids:

```bash
sudo usermod --add-subuids 100000-165535 --add-subgids 100000-165535 "$USER"
```

## What the probe checks

`Critical` means the capability is refused outright when the item fails. Non-critical items
narrow what guests can do rather than whether they run at all.

| Check | Blocks | Critical | Why it is needed | Typical fix |
|---|---|---|---|---|
| `systemd` | VMs **and** containers | Yes | Guests are supervised as systemd transient units, so they outlive an agent restart | A systemd host. There is no degraded mode |
| `kvm_device` | VMs | Yes | `/dev/kvm` is the hypervisor. The probe separates "missing" from "present but the agent cannot open it" | Enable VT-x / AMD-V in firmware, `modprobe kvm_intel` or `kvm_amd`, and put the agent's user in the `kvm` group |
| `qemu` | VMs | Yes | The emulator McHarbor drives over QMP. No libvirt in the path | Install `qemu-system-x86_64` (`qemu-system-x86` on Debian/Ubuntu) |
| `qemu_img` | VMs | Yes | Creates and resizes guest disks | Install `qemu-img` (`qemu-utils` on Debian/Ubuntu) |
| `cgroup2` | Containers | Yes | The unified hierarchy. A cgroup v1 host is refused | Boot with `systemd.unified_cgroup_hierarchy=1`, or use a newer distribution |
| `userns` | Containers | Yes | Unprivileged containers are built on user namespaces | A kernel with `CONFIG_USER_NS=y`; on some distros also `sysctl user.max_user_namespaces=15000` |
| `subuid` | Containers | Yes | Each unprivileged container needs its own non-overlapping block of 65536 subordinate ids | `sudo usermod --add-subuids 100000-165535 --add-subgids 100000-165535 <user>` |
| `cpu_virt` | VMs | No | Reports whether the CPU advertises VT-x / SVM, which distinguishes "disabled in firmware" from "unsupported silicon" | Enable virtualization in BIOS/UEFI |
| `ovmf` | — | No | UEFI firmware. BIOS guests still boot without it; Windows 11 and Secure Boot do not | Install `ovmf` / `edk2-ovmf` |
| `swtpm` | — | No | The vTPM Windows 11 requires | Install `swtpm` |
| `idmapped_mounts` | — | No | Lets unprivileged container rootfs trees be shared and cloned without a chown pass | Kernel 5.12+, 5.19+ recommended |
| `dbus` | — | No | Without it the agent falls back to `systemd-run`, which polls for guest state instead of receiving it | Ensure dbus is running and `/run/dbus` is visible to the agent |
| `seccomp` | — | No | Whether this agent build carries a syscall filter for containers. User-namespace isolation applies either way | Use an agent build compiled with the seccomp tag |
| `iommu` | — | No | PCI and GPU passthrough | Enable VT-d / AMD-Vi and add `intel_iommu=on` or `amd_iommu=on` to the kernel command line |
| `host_network` | — | No | Whether the agent can see and change the host's own interfaces, which guest networking needs | Run the agent in the host's network namespace with `NET_ADMIN` |

### The two conclusions, exactly

The panel's **Can run virtual machines** and **Can run system containers** are derived, not
reported separately:

```
CanRunVMs        = kvm_device AND qemu AND qemu_img AND systemd
CanRunContainers = cgroup2   AND userns AND systemd
```

One consequence worth knowing: **`subuid` is critical but is not part of either formula.**
A host can report *Can run system containers* while `subuid` fails. Containers will start,
but only as root, because there are no subordinate ids to map an unprivileged one into. Read
the checklist, not only the summary line.

## Reading a real probe

This is the actual output from a **containerised** agent, which is the case most people meet
first. It is included because the shape of the answer matters more than the list:

```
supported=true  canRunVMs=false  canRunContainers=false
FAIL kvm_device   /dev/kvm does not exist. The kvm kernel module is not loaded, or hardware
                  virtualization is disabled in firmware.
ok   cpu_virt     13th Gen Intel(R) Core(TM) i9-13950HX reports Intel VT-x (vmx).
FAIL qemu         No qemu-system-* binary found on PATH.
FAIL qemu_img     qemu-img is not on PATH.
FAIL systemd      This host is not running systemd.
ok   cgroup2      cgroup v2 unified hierarchy mounted at /sys/fs/cgroup.
ok   userns       User namespaces are available.
FAIL subuid       User "root" has 0 subordinate uids and 0 subordinate gids.
ok   idmapped_mounts
```

The CPU is capable and four checks fail. No amount of configuration fixes that combination,
because the agent is a container. It is the container-versus-native decision above, showing
up as a checklist.

## A note on nested virtualization

Running the agent inside a VM (a cloud instance, a Proxmox guest, WSL2) works only if the
layer below exposes `/dev/kvm` to it:

| Platform | What to enable |
|---|---|
| Proxmox / QEMU guest | CPU type `host`, or the `vmx`/`svm` flag on the guest CPU |
| VMware | "Expose hardware assisted virtualization to the guest OS" |
| Hyper-V / WSL2 | Nested virtualization on the VM; WSL2 does not expose `/dev/kvm` by default |
| AWS / GCP / Azure | Bare-metal or nested-capable instance types only |

`cpu_virt` passing while `kvm_device` fails is the signature of this: the CPU flag is visible
but the device node is not there.

## Quick answers

**Can I run VMs on the machine McHarbor itself runs on?**
Yes. Install the agent natively on that host, alongside the McHarbor container. It costs no
extra licence seat.

**Can I add QEMU to the agent image and skip all this?**
It clears `qemu` and `qemu_img` and leaves `systemd` and `kvm_device`. A guest still cannot
start, so it would only move the failure later.

**System containers look closer to working than VMs. Are they?**
Usually, yes. They need no hypervisor, so `cgroup2`, `userns`, `subuid` and `systemd` are the
whole list, and the first three are ordinary host configuration.

**What if the host runs Windows or macOS?**
Compute is Linux-host-only. The probe returns a populated report saying so, rather than an
error, so the UI can explain why the section is empty.

## Related

- [Getting started with guests](./getting-started.md): the seven steps, once the host qualifies
- [Compute Foundation → Configuration](./configuration.md): pools, networks and the image library
- [Compute Foundation → Troubleshooting](./troubleshooting.md)
- [Virtual Machines](../virtual-machines/README.md) · [System Containers](../system-containers/README.md)
- [Agent prerequisites](../agents/prerequisites/README.md): everything an agent needs, guests aside
- [Installing an Agent](../agents/installation/README.md): the container and binary paths
