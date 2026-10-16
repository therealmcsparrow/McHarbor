# Supported Hosts and Guests

What McHarbor runs on, what the agent runs on, which of those hosts can actually run guests,
and what those guests can be.

This page is the *inventory*. [Host requirements](./host-requirements.md) is the *checklist*:
the per-item probe, what each check blocks, and the command that fixes it.

## McHarbor itself

McHarbor ships as a container. There is nothing to install on the host beyond Docker.

| | Supported |
|---|---|
| Operating system | Any host running Docker Engine 20.10 or newer with the Compose v2 plugin |
| Architecture | `x86_64` and `arm64`; the runtime image is published for both |
| Docker Desktop | macOS and Windows, against the Desktop socket |
| Storage | One writable volume. About 200 MB to start; metrics history, plugins and vulnerability databases grow it |
| Ports | `8705` (HTTPS, self-signed on first boot), in the shipped mapping |
| Browser | A current Chrome, Edge, Firefox or Safari, with WebSocket upgrades allowed through any proxy in front |

**Recommended:** **Ubuntu Server 24.04.5 LTS or 26.04.1 LTS** on `x86_64`, running Docker
Engine (not Docker Desktop) with a named volume for `/app/data` and a certificate from a CA
your agents already trust. Any distribution with a working Docker Engine runs McHarbor; those
two are what each release is verified on. Docker Desktop works and is the normal way to try
McHarbor; it is not the arrangement to leave a fleet on, because its socket lives inside a VM
whose lifecycle you do not control.

## The remote agent

The agent dials **out** to McHarbor over one WebSocket. Nothing dials in, so a host behind NAT
needs no inbound rule. The one exception is direct agent-to-agent transfer, which wants TCP
`8788` reachable on the receiving host and falls back to relaying through McHarbor when it is
not.

| Platform | Build | Docker management | Guest hosting |
|---|---|---|---|
| `linux/amd64` | CGO, static, seccomp | Yes | Yes, as a native binary under systemd |
| `linux/arm64` | CGO, static, seccomp | Yes | Yes, as a native binary under systemd |
| `darwin/amd64` | Pure Go | Yes | No |
| `darwin/arm64` | Pure Go | Yes | No |
| `windows/amd64` | Pure Go | Yes | No |

A Linux agent cannot be built with CGO disabled: the container runtime's `nsenter` is a C
constructor with no pure-Go fallback, which is why the Linux binaries come out of the image
build and the other platforms are cross-compiled.

**macOS and Windows agents manage Docker only.** They serve host stats, host logs and the host
terminal, and they take part in backups and transfers. They cannot host virtual machines or
system containers. Neither can a Linux agent running *in a container*.

### The line that matters: container or native

| | Container agent | Native agent (binary + systemd) |
|---|---|---|
| Docker and Compose | Yes | Yes |
| Host stats, logs, terminal | Yes | Yes |
| Backups, snapshots, transfers | Yes | Yes |
| Guest networks | Yes, with `--network host --cap-add NET_ADMIN` | Yes |
| **Virtual machines** | **No** | Yes |
| **System containers** | **No** | Yes |

A containerised agent has no systemd to supervise a guest with, no `qemu-system-*`, and no
`/dev/kvm`. This is not a packaging gap that some flag closes. See
[Host requirements → Decide this first](./host-requirements.md).

**Recommended guest host:** a physical `x86_64` machine (not a VM, unless you have checked
nested virtualization) running **Ubuntu Server 24.04.5 LTS or 26.04.1 LTS**, which ships
systemd with cgroup v2 as the unified hierarchy, with VT-x or AMD-V enabled in firmware,
`qemu-system-x86_64`, `qemu-img`, `ovmf` and `swtpm` installed, a subuid range granted to the
agent's user, and the **agent installed as a binary under systemd**. `scripts/compute/install-host-dependencies.sh` maps the package names
for Debian and Ubuntu, RHEL and Fedora, SUSE and Arch, and reports whatever it cannot
fix itself. Alpine is not a supported host: it runs OpenRC rather than systemd, and the
probe refuses a host without it.

This is also the one case where McHarbor and an agent legitimately run on the same machine;
the licence server collapses them into one seat by machine fingerprint.

## Supported guests

Two guest types, run by two plugins on top of this one.

| | Virtual machines (`addon-kvm`) | System containers (`addon-lxc`) |
|---|---|---|
| What it is | A full KVM guest with its own kernel and firmware | A full-system LXC guest sharing the host kernel |
| Guest operating system | Anything QEMU can boot | A Linux distribution with a published root filesystem |
| Needs | `/dev/kvm`, `qemu-system-*`, `qemu-img`, systemd | cgroup v2, user namespaces, systemd, a subuid range |
| Firmware | BIOS, or UEFI with `ovmf`; Secure Boot and vTPM with `swtpm` | Not applicable |

**Anything bootable runs.** The catalog below is what McHarbor fetches and verifies *for you*;
it is not the boundary of what a guest may be. Point an entry at your own URL, or upload a
disk image or an ISO, and a guest is a guest.

### The image catalog

49 entries ship with this release.

| Kind | Entries | What it is for |
|---|---|---|
| ISO | 24 | An installer or a live image. You do the install, in the console |
| Cloud image | 22 | A prepared disk that boots in seconds and takes its user, SSH key and network from cloud-init |
| Root filesystem | 3 | A tarball, for a system container |

| Architecture | Entries |
|---|---|
| `x86_64` | 40 |
| `aarch64` | 9 |

Covered today: **AlmaLinux, Alpine, Android-x86, Arch Linux, Arch Linux ARM, BlissOS, CentOS
Stream, Debian, elementary OS, EndeavourOS, Fedora (Workstation, Cloud and IoT), Flatcar, Home
Assistant OS, Linux Mint, Manjaro, MX Linux, NixOS, openSUSE (Leap and Tumbleweed), Oracle
Linux, Pop!\_OS, Puppy Linux, Rocky Linux, Solus, Ubuntu (desktop, server and cloud), Windows
11 Enterprise LTSC, Windows Server 2025, Zorin OS.**

Every source and checksum URL is verified before an entry is catalogued. An entry whose
publisher offers no usable checksum says so and downloads behind an explicit acknowledgement,
rather than carrying a checksum somebody invented.

### Guests worth knowing something about before you start

| Guest | What it needs |
|---|---|
| **Windows 11** | UEFI (`ovmf`), Secure Boot and a vTPM (`swtpm`). The Hardware tab exposes both; a host without `swtpm` cannot install it |
| **Windows, any version** | virtio storage and network drivers during setup. The guest-agent disc carries the virtio payload, so attach it before the installer asks for a disk |
| **Home Assistant OS** | A cloud-image entry, `x86_64`. It ships compressed, so the download spends several minutes decompressing after the transfer has reached 100% |
| **Android-x86 and BlissOS** | Installer ISOs, `x86_64`. Graphical console only |
| **Cloud images generally** | cloud-init. Set the user and the SSH key at create time, or you will have no way in |
| **System containers** | One of the root-filesystem entries, namely Alpine 3.21 (`x86_64`), Ubuntu 24.04 (`x86_64`) and Arch Linux ARM (`aarch64`), or a tarball of your own |

A guest reports its IP address only once the **QEMU guest agent** is installed inside it. The
disc that installs it is one click from the machine header, the list row, the card or the
Hardware tab, and it is built ahead of the click rather than during the request.

## Quick answers

**Can I run virtual machines on the McHarbor host itself?** Only if a native agent is
installed on that host as well. McHarbor's own container is not a guest host.

**Can I run guests on a Raspberry Pi?** The `linux/arm64` agent hosts guests on any `aarch64`
machine that clears the probe. The catalog's `aarch64` coverage is nine entries; anything else
you supply yourself.

**Can I run guests inside a VM?** Only with nested virtualization enabled on the outer
hypervisor, and the probe will tell you whether `/dev/kvm` actually appeared.

**Is macOS or Windows a guest host?** No. Docker management only.

## Related

- [Host requirements](./host-requirements.md): the probe, item by item, with the fix for each
- [Getting started](./getting-started.md): nothing to a running guest, in seven steps
- [Guest networking](./guest-networking.md): bridged, external and managed modes
- [Image catalog](./image-catalog.md): the JSON behind the list, and how to add an entry
- [Installing an Agent](../agents/installation/README.md): container and binary installs
- [Agent prerequisites](../agents/prerequisites/README.md): what the remote host needs
