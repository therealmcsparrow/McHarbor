# Virtual machines: hot-plug

Disks, installer discs and network interfaces can be changed while a machine runs. The agent
plugs and unplugs the device over QMP; McHarbor writes the row only after the agent confirmed,
and a step that fails after the guest changed is undone on the guest too, so the **Hardware**
tab and the guest never disagree.

## What the host needs

- A host whose agent reports the `hotplug` feature. Without it a change to a **running** machine
  is refused with `err.vm.hotplug_unavailable` ("This host cannot change hardware on a running
  machine. Stop the machine first."). A stopped machine is always editable.
- A machine started **after** the host gained hot-plug. An older guest has no spare ports, and the
  change is refused with `err.vm.hotplug_restart_required`; restart it once.

A **paused** machine cannot answer an unplug, so every live change is refused until it is resumed
or stopped.

## What can change while it runs

| Change | Route | Key |
|---|---|---|
| Add a data disk | `POST /api/vms/{id}/disks` | `vm.manage` |
| Grow a data disk | `PATCH /api/vms/{id}/disks/{diskId}` | `vm.manage` |
| Remove a data disk and its volume | `DELETE /api/vms/{id}/disks/{diskId}` | `vm.delete` |
| Put a disc in an empty drive | `POST /api/vms/{id}/media` | `vm.manage` |
| Eject a disc | `DELETE /api/vms/{id}/media/{diskId}` | `vm.manage` |
| Add a network interface | `POST /api/vms/{id}/nics` | `vm.manage` |
| Remove a network interface | `DELETE /api/vms/{id}/nics/{nicId}` | `vm.manage` |
| Connect or disconnect the link | `PATCH /api/vms/{id}/nics/{nicId}` with `linkUp` | `vm.manage` |

The audit row is the same one a stopped machine writes; it records whether the machine was running.
A disk that cannot be shrunk is refused on a stopped machine too.

## What cannot

| Refusal | Cause | Fix |
|---|---|---|
| `err.vm.hotplug_bus_unsupported` | A SATA or IDE disk | Use virtio-blk or virtio-scsi, or stop the machine |
| `err.vm.hotplug_no_controller` | The first SCSI disk on a machine with no SCSI controller yet | Add it while stopped |
| `err.vm.hotplug_no_free_port` | The slot is beyond the machine's hot-plug ports | Stop the machine to add more |
| `err.vm.hotplug_fixed_device` | The device sits on a bus that cannot unplug it | Stop the machine to remove it |
| `err.vm.hotplug_guest_refused` | The guest's operating system did not release the device. Nothing was removed | Try again, or stop the machine |
| The boot disk | The boot disk, or the first non-disc disk in the boot order, never changes under the system that runs from it | Stop the machine |
| No empty drive | A running machine with no empty disc drive cannot grow one; a stopped machine gets a new drive | Stop the machine, or eject first |
| An interface **type** | The device model is fixed once QEMU has created it | Stop the machine |

Removing a data disk unplugs it first and destroys its volume second; if the volume will not go,
the disk is plugged back in. Removing a disk also invalidates its backup chain, because the change
log lived inside the image.

## Related

- [Virtual machines](../README.md)
- [QEMU features](../qemu-features.md)
- [Configuration](../configuration.md)
