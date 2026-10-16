# Containers: Devices

`/containers/:id` → **Resources** tab → **Devices** (while editing) · **Create container** → **Devices** · `mcharbor host devices --env <id>`

A device mapping hands a node under the host's `/dev` to the container: a USB serial
adapter, a GPU render node, a sound card, `/dev/net/tun`, `/dev/fuse`, a disk. It is
Docker's `--device <host>:<container>:<permissions>`, stored in `HostConfig.Devices`.

Each row has three fields: **Host Path**, **Container Path** and **Permissions**. Host Path
is a dropdown of the devices McHarbor found on the environment's host; anything it did not
find can still be typed.

## Permissions

| Key | Scope | What it unlocks |
|---|---|---|
| `containers.manage` | environment | Adding, changing and removing device mappings; saving recreates the container, as every configuration edit does |
| `host.view` | environment | The list of the host's devices behind the Host Path dropdown, and `mcharbor host devices` |

Without `host.view` the Host Path field is a plain text box, and its **Choose from list**
button renders disabled with a tooltip naming `host.view`. No request reaches the host. You
can still type a path and save it with `containers.manage` alone.

## Host Path

The dropdown is grouped by kind, in this order:

| Group | What lands there |
|---|---|
| Serial | `/dev/ttyUSB*`, `/dev/ttyACM*`, `/dev/ttyAMA*`, and `/dev/ttyS*` only where the kernel reports a real UART behind it |
| GPU | `/dev/dri/card*`, `/dev/dri/renderD*`, `/dev/nvidia*`, `/dev/nvidia-caps/*` |
| Audio | `/dev/snd/*` |
| Video | `/dev/video*`, `/dev/media*` |
| Input | `/dev/input/event*`, `/dev/hidraw*` |
| Network | `/dev/net/tun` |
| USB | `/dev/bus/usb/<bus>/<device>` |
| System | `/dev/fuse`, `/dev/kvm` |
| Block | `/dev/sd*`, `/dev/nvme*n*`, `/dev/mmcblk*`: disks and their partitions |

Each entry reads `<label> (<node>)`. The label comes from the host's udev database (vendor
and model, or the filesystem label, or the serial string), then from the `by-id` name, then
from the DRM driver (`i915`, `amdgpu`, `nvidia`). A device udev knows nothing about shows its
node path alone. The second line carries the stable path, if there is one, and the node
type with its major:minor numbers.

**The value stored is the stable path when one exists.** A USB serial adapter is saved as
`/dev/serial/by-id/usb-…-port0`, not `/dev/ttyUSB0`, because `ttyUSB0` is whichever adapter
the kernel enumerated first and can change after a replug or a reboot. `by-id` is preferred
over `by-path`, and a model-derived `by-id` over a `wwn-` or `nvme-eui.` one. Under the
field, "Resolves to /dev/ttyUSB0 on the host" shows the node it points at today.

A block device shows a **Block device** badge and a warning: it gives the container raw
access to a disk, and writing to it can destroy the host's data.

**Enter manually…** is the last entry. It swaps the dropdown for a text box, for a device
that is not plugged in yet or that the list does not cover; **Choose from list** swaps back.
A saved path the host no longer lists opens in the text box, so nothing is lost.

## Container Path

Picking a device fills Container Path with the device's node (`/dev/ttyUSB0`), which is the
name software inside the container expects. It is only filled while it is empty or still
holds the previous automatic value; a path you typed is kept. Left empty, it is saved as the
host path.

Two devices cannot share a container path. Kernel names are handed out at plug-in time, so a
second USB serial adapter may also have been `/dev/ttyUSB0` when it was picked. When the node
a pick would fill in is already used by another row, the next free one is filled in instead:
`/dev/ttyUSB1`, then `/dev/ttyUSB2`; a path without a trailing number gets one appended. A
path typed or kept that another row already uses shows "Another device in this container
already uses this path." with a **Use &lt;free path&gt;** button.

## Permissions (cgroup)

| Value | Meaning |
|---|---|
| `rwm` | Read, write and create the node (the default) |
| `rw` | Read and write |
| `r` | Read only |
| `m` | Create the node only (`mknod`) |

A container that already carries another combination keeps it; it appears in the list as a
custom value.

## Where the list comes from

`GET /api/system/host-devices?env=<id>`, gated by `host.view`. McHarbor runs a short
read-only script in a throwaway `alpine:3.20` container with the host's root mounted
read-only at `/host`, the same utility container that reads host OS logs. It stats the
nodes above, reads `/run/udev/data` for labels, `/sys/class/drm` for GPU drivers and the
`by-id` / `by-path` links under `/dev/serial`, `/dev/disk`, `/dev/dri`, `/dev/snd`,
`/dev/input` and `/dev/v4l`. It writes nothing.

This works for local, TCP and SSH environments and for agent environments whose agent
proxies Docker. The list is bounded: 512 devices, 30 seconds, and every path is re-checked
server-side to be a clean path under `/dev`. When the host has more, the dropdown says the
list is truncated.

## What goes wrong

| Symptom | Cause | Fix |
|---|---|---|
| "Could not list this host's devices" under a text box | The utility container could not run: the host cannot pull `alpine:3.20`, or the Docker endpoint is unreachable | Enter the path manually; pre-pull `alpine:3.20` on an offline host |
| A plugged-in adapter is missing | The list is cached for a minute | Reopen the editor after a minute |
| No labels, only node paths | The host runs no udev (many minimal and container-optimised systems) | Nothing to fix; the paths are still correct |
| Docker Desktop lists only `/dev/sd*`, `/dev/fuse`, `/dev/net/tun` | The "host" is Docker Desktop's VM, not your PC | Expected; USB devices are not passed into the VM |
| The container fails to start after mapping a by-id path | The device was unplugged | Plug it back in, or remove the mapping |

## Related

- [Containers](../README.md): the screen and its permissions
- [Fields](../fields.md): every form field on the screen
- [CLI](../../../cli/README.md): `mcharbor host devices`
