// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/qmp"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Hot-plug refusals. Each one is a state the operator can act on, so it is
// reported by code rather than as a failure, and McHarbor turns the code into
// a sentence that says what to do.
const (
	// HotplugRestartRequired means the guest was booted before it had its
	// root ports. One restart gives it them.
	HotplugRestartRequired = "hotplug_restart_required"
	// HotplugNoFreePort means the slot is beyond the ports a guest gets.
	HotplugNoFreePort = "hotplug_no_free_port"
	// HotplugBusUnsupported means the bus cannot change live: SATA and IDE
	// have no hot-plug, and a disc drive is never added to a running guest.
	HotplugBusUnsupported = "hotplug_bus_unsupported"
	// HotplugNoController means the first SCSI disk of a guest, whose
	// controller only exists from the next boot.
	HotplugNoController = "hotplug_no_controller"
	// HotplugFixedDevice means the device sits on the root bus, which cannot
	// unplug anything; a NIC pinned there keeps the guest's interface name.
	HotplugFixedDevice = "hotplug_fixed_device"
	// HotplugGuestRefused means the guest did not release the device in time.
	// device_del is a request, and an operating system may decline it.
	HotplugGuestRefused = "hotplug_guest_refused"
	// HotplugBootDisk means the boot disk, which never changes under a guest.
	HotplugBootDisk = "hotplug_boot_disk"
	// HotplugNoDrive means the disc drive does not exist on the running guest.
	HotplugNoDrive = "hotplug_no_drive"
)

// HotplugError is a refusal with one of the codes above.
type HotplugError struct {
	Code   string
	Detail string
}

func (e *HotplugError) Error() string { return "vm: " + e.Detail }

func refuse(code, format string, args ...any) error {
	return &HotplugError{Code: code, Detail: fmt.Sprintf(format, args...)}
}

// QMPRunner is the monitor surface hot-plug needs. *qmp.Monitor satisfies it.
type QMPRunner interface {
	Run(ctx context.Context, execute string, args interface{}) (json.RawMessage, error)
}

// defaultReleaseWait bounds how long an unplug waits for the guest: a Linux
// guest releases in about a second over ACPI and in five over native PCIe, and
// a busy Windows PnP manager can take most of a minute.
const defaultReleaseWait = 90 * time.Second

// releaseCleanup is the part of the caller's budget an unplug leaves for the
// work after it and the reply, so a slow guest is refused by code rather than
// cut off by the transport with nothing the operator can act on.
const releaseCleanup = 10 * time.Second

// minReleaseWait is the least an unplug waits whatever the budget says.
const minReleaseWait = time.Second

// defaultPoll is how often an unplug re-reads the device list, the backstop for
// a DEVICE_DELETED event that was dropped or arrived before anyone listened.
const defaultPoll = 500 * time.Millisecond

// Hotplug changes a running guest's devices over its monitor.
//
// Every operation is idempotent against the device it names: attaching what is
// already attached, or detaching what is already gone, succeeds without a
// command. That is what lets McHarbor retry after a lost reply without first
// asking which half happened.
type Hotplug struct {
	Mon QMPRunner
	// MachineType decides whether the root ports exist at all.
	MachineType string
	// IOThread binds a new virtio-blk disk to the guest's I/O thread, which
	// exists only when the guest was started with one.
	IOThread bool
	// Deleted returns a channel closed when QEMU reports DEVICE_DELETED for id,
	// and a release for the watch. Optional; polling covers its absence.
	Deleted     func(id string) (<-chan struct{}, func())
	ReleaseWait time.Duration
	Poll        time.Duration
}

// topology is what the guest has right now.
type topology struct {
	devices map[string]bool
	// parent maps a PCI device's id to the port it sits behind, "" for pcie.0.
	parent   map[string]string
	occupied map[string]bool
}

type qomEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type pciDevice struct {
	QdevID    string `json:"qdev_id"`
	PCIBridge *struct {
		Devices []pciDevice `json:"devices"`
	} `json:"pci_bridge,omitempty"`
	Regions []pciRegion `json:"regions,omitempty"`
}

// pciRegion is one BAR. QEMU reports an address the guest never assigned as the
// all-ones pcibus_t, which arrives as -1 or, from some builds, as its unsigned form.
type pciRegion struct {
	Type    string      `json:"type"`
	Address json.Number `json:"address"`
}

func (r pciRegion) unassigned() bool {
	return r.Address == "-1" || r.Address == "18446744073709551615"
}

type pciBus struct {
	Bus     int         `json:"bus"`
	Devices []pciDevice `json:"devices"`
}

func (h *Hotplug) run(ctx context.Context, execute string, args interface{}, out interface{}) error {
	raw, err := h.Mon.Run(ctx, execute, args)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("vm: decoding %s: %w", execute, err)
	}
	return nil
}

func (h *Hotplug) topology(ctx context.Context) (topology, error) {
	t := topology{devices: map[string]bool{}, parent: map[string]string{}, occupied: map[string]bool{}}
	var entries []qomEntry
	if err := h.run(ctx, "qom-list", map[string]interface{}{"path": "/machine/peripheral"}, &entries); err != nil {
		return t, fmt.Errorf("vm: listing the guest's devices: %w", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Type, "child<") {
			t.devices[e.Name] = true
		}
	}
	var buses []pciBus
	if err := h.run(ctx, "query-pci", nil, &buses); err != nil {
		return t, fmt.Errorf("vm: reading the guest's PCI layout: %w", err)
	}
	var walk func(parent string, devices []pciDevice)
	walk = func(parent string, devices []pciDevice) {
		for _, d := range devices {
			if d.QdevID != "" {
				t.parent[d.QdevID] = parent
				if parent != "" {
					t.occupied[parent] = true
				}
			}
			if d.PCIBridge != nil {
				walk(d.QdevID, d.PCIBridge.Devices)
			}
		}
	}
	for _, bus := range buses {
		walk("", bus.Devices)
	}
	return t, nil
}

// claimPoll is how often a claim check rereads the guest's PCI layout.
const claimPoll = 500 * time.Millisecond

// DiskClaimed reports whether the guest has picked up the disk in slot, waiting up
// to wait for it. See claimed.
func (h *Hotplug) DiskClaimed(ctx context.Context, slot int, wait time.Duration) (bool, error) {
	return h.claimed(ctx, diskID(slot), wait)
}

// NICClaimed is DiskClaimed for the NIC in slot.
func (h *Hotplug) NICClaimed(ctx context.Context, slot int, wait time.Duration) (bool, error) {
	return h.claimed(ctx, nicID(slot), wait)
}

// claimed waits for the guest to assign an address to any of the device's memory BARs.
// Enumeration assigns them even with no driver for the device, so a device that keeps
// none was never seen at all: the guest kernel has no PCI hot-plug support. A device
// with no memory BAR, or one the layout does not list, is not second-guessed.
func (h *Hotplug) claimed(ctx context.Context, qdevID string, wait time.Duration) (bool, error) {
	deadline := time.Now().Add(wait)
	for {
		var buses []pciBus
		if err := h.run(ctx, "query-pci", nil, &buses); err != nil {
			return true, fmt.Errorf("vm: reading the guest's PCI layout: %w", err)
		}
		device, found := findPCIDevice(buses, qdevID)
		if !found {
			return true, nil
		}
		memory, assigned := 0, 0
		for _, region := range device.Regions {
			if region.Type != "memory" {
				continue
			}
			memory++
			if !region.unassigned() {
				assigned++
			}
		}
		if memory == 0 || assigned > 0 {
			return true, nil
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		case <-time.After(claimPoll):
		}
	}
}

// findPCIDevice walks every bus and bridge for qdevID.
func findPCIDevice(buses []pciBus, qdevID string) (pciDevice, bool) {
	var walk func(devices []pciDevice) (pciDevice, bool)
	walk = func(devices []pciDevice) (pciDevice, bool) {
		for _, d := range devices {
			if d.QdevID == qdevID {
				return d, true
			}
			if d.PCIBridge != nil {
				if found, ok := walk(d.PCIBridge.Devices); ok {
					return found, true
				}
			}
		}
		return pciDevice{}, false
	}
	for _, bus := range buses {
		if found, ok := walk(bus.Devices); ok {
			return found, true
		}
	}
	return pciDevice{}, false
}

// isNotFound reports a QMP refusal that means the thing is already gone.
func isNotFound(err error) bool {
	var qerr *qmp.Error
	if errors.As(err, &qerr) {
		return qerr.Class == "DeviceNotFound" ||
			strings.Contains(strings.ToLower(qerr.Desc), "not found") ||
			strings.Contains(strings.ToLower(qerr.Desc), "cannot find")
	}
	return false
}

func diskID(slot int) string   { return fmt.Sprintf("disk%d", slot) }
func diskNode(slot int) string { return fmt.Sprintf("blk%d", slot) }
func nicID(slot int) string    { return fmt.Sprintf("nic%d", slot) }
func netdevID(slot int) string { return fmt.Sprintf("net%d", slot) }
func diskBus(disk Disk) string {
	if disk.Bus == "" {
		return "virtio-blk"
	}
	return disk.Bus
}

// dropNodes closes a disk's two block nodes, ignoring ones that are already
// gone. Format first: the protocol node is held open by the node above it.
func (h *Hotplug) dropNodes(ctx context.Context, slot int) {
	node := diskNode(slot)
	for _, name := range []string{node, node + "-file"} {
		h.bestEffort(ctx, "blockdev-del", map[string]interface{}{"node-name": name}) // best effort: a missing node is the goal state
	}
}

// bestEffort runs a cleanup command whose failure leaves nothing to undo. A
// "not found" is the state it was after, so only anything else is logged.
func (h *Hotplug) bestEffort(ctx context.Context, execute string, args interface{}) {
	if _, err := h.Mon.Run(ctx, execute, args); err != nil && !isNotFound(err) {
		errlog.Log(err)
	}
}

// AttachDisk plugs a data disk into a running guest: blockdev-add for the file
// and the format, then device_add on the disk's own port. A failed device_add
// closes the nodes again, so a retry starts clean.
func (h *Hotplug) AttachDisk(ctx context.Context, disk Disk) error {
	if disk.IsBoot {
		return refuse(HotplugBootDisk, "the boot disk is never plugged into a running guest")
	}
	if disk.IsCDROM {
		return refuse(HotplugBusUnsupported, "a disc drive cannot be added to a running guest")
	}
	if disk.Path == "" {
		return fmt.Errorf("vm: disk in slot %d has no path", disk.Slot)
	}
	topo, err := h.topology(ctx)
	if err != nil {
		return err
	}
	id := diskID(disk.Slot)
	if topo.devices[id] {
		return nil
	}

	device := map[string]interface{}{"id": id, "drive": diskNode(disk.Slot)}
	switch diskBus(disk) {
	case "virtio-blk":
		device["driver"] = "virtio-blk-pci"
		if isQ35(h.MachineType) {
			if disk.Slot >= DiskPorts {
				return refuse(HotplugNoFreePort, "disk slot %d is beyond the %d hot-plug ports a guest has", disk.Slot, DiskPorts)
			}
			port := DiskPortID(disk.Slot)
			if !topo.devices[port] {
				return refuse(HotplugRestartRequired, "this guest was started before it had hot-plug ports")
			}
			if topo.occupied[port] {
				return refuse(HotplugNoFreePort, "the port for disk slot %d is in use", disk.Slot)
			}
			device["bus"] = port
		}
		if h.IOThread {
			device["iothread"] = ioThreadID
		}
	case "virtio-scsi", "scsi":
		if !topo.devices["scsi0"] {
			return refuse(HotplugNoController, "this guest has no SCSI controller until it restarts with a SCSI disk")
		}
		device["driver"] = "scsi-hd"
		device["bus"] = "scsi0.0"
	default:
		return refuse(HotplugBusUnsupported, "a %s disk cannot be plugged into a running guest", diskBus(disk))
	}
	if disk.ShareRW {
		device["share-rw"] = true
	}
	for name, value := range map[string]int64{
		"throttling.iops-read": disk.ReadIOPS, "throttling.iops-write": disk.WriteIOPS,
		"throttling.bps-read": disk.ReadBPS, "throttling.bps-write": disk.WriteBPS,
	} {
		if value > 0 {
			device[name] = value
		}
	}
	if disk.BootIndex > 0 {
		device["bootindex"] = disk.BootIndex
	}

	// A node left by an unplug whose cleanup failed would make blockdev-add
	// refuse the name; nothing uses it, because the device is gone.
	h.dropNodes(ctx, disk.Slot)
	if err := h.addNodes(ctx, disk); err != nil {
		return err
	}
	if _, err := h.Mon.Run(ctx, "device_add", device); err != nil {
		h.dropNodes(ctx, disk.Slot)
		return fmt.Errorf("vm: plugging disk %d into the guest: %w", disk.Slot, err)
	}
	return nil
}

// addNodes opens a disk the way buildDiskArgs does on the command line.
func (h *Hotplug) addNodes(ctx context.Context, disk Disk) error {
	node := diskNode(disk.Slot)
	driver := "file"
	if disk.IsBlock {
		driver = "host_device"
	}
	file := map[string]interface{}{"driver": driver, "node-name": node + "-file", "filename": disk.Path}
	if disk.CacheMode == "" || disk.CacheMode == "none" {
		file["cache"] = map[string]interface{}{"direct": true, "no-flush": false}
	}
	if disk.Discard {
		file["discard"] = "unmap"
	}
	if disk.ReadOnly {
		file["read-only"] = true
	}
	if disk.ShareRW {
		file["locking"] = "off"
	}
	if _, err := h.Mon.Run(ctx, "blockdev-add", file); err != nil {
		return fmt.Errorf("vm: opening %s: %w", disk.Path, err)
	}

	format := disk.Format
	if format == "" {
		format = "qcow2"
	}
	top := map[string]interface{}{"driver": format, "node-name": node, "file": node + "-file"}
	if disk.ReadOnly {
		top["read-only"] = true
	}
	if disk.Discard && format == "qcow2" {
		top["discard"] = "unmap"
		top["detect-zeroes"] = "unmap"
	}
	if _, err := h.Mon.Run(ctx, "blockdev-add", top); err != nil {
		h.bestEffort(ctx, "blockdev-del", map[string]interface{}{"node-name": node + "-file"}) // rollback of the half that did open
		return fmt.Errorf("vm: opening %s as %s: %w", disk.Path, format, err)
	}
	return nil
}

// DetachDisk asks the guest to release a data disk, waits until it has, then
// closes the disk's nodes so the volume can be removed.
func (h *Hotplug) DetachDisk(ctx context.Context, disk Disk) error {
	if disk.IsBoot {
		return refuse(HotplugBootDisk, "the boot disk cannot be removed while the guest runs")
	}
	if disk.IsCDROM {
		return refuse(HotplugBusUnsupported, "a disc drive cannot be removed from a running guest")
	}
	topo, err := h.topology(ctx)
	if err != nil {
		return err
	}
	id := diskID(disk.Slot)
	if !topo.devices[id] {
		h.dropNodes(ctx, disk.Slot)
		return nil
	}
	switch diskBus(disk) {
	case "virtio-blk":
		if isQ35(h.MachineType) && topo.parent[id] == "" {
			if diskHasPort(disk) {
				return refuse(HotplugRestartRequired, "this disk was attached before the guest had hot-plug ports")
			}
			return refuse(HotplugFixedDevice, "disk slot %d sits on the root bus, which cannot unplug it", disk.Slot)
		}
	case "virtio-scsi", "scsi":
	default:
		return refuse(HotplugBusUnsupported, "a %s disk cannot be removed from a running guest", diskBus(disk))
	}
	if err := h.unplug(ctx, topo, id); err != nil {
		return err
	}
	h.dropNodes(ctx, disk.Slot)
	return nil
}

// ResizeDisk tells QEMU a data disk is now sizeBytes. For a file it grows the
// image itself; a block device is grown by its pool driver first.
func (h *Hotplug) ResizeDisk(ctx context.Context, disk Disk, sizeBytes int64) error {
	if disk.IsBoot {
		return refuse(HotplugBootDisk, "the boot disk cannot be resized while the guest runs")
	}
	if disk.IsCDROM {
		return refuse(HotplugBusUnsupported, "a disc cannot be resized")
	}
	if sizeBytes <= 0 {
		return fmt.Errorf("vm: refusing to resize disk %d to %d bytes", disk.Slot, sizeBytes)
	}
	topo, err := h.topology(ctx)
	if err != nil {
		return err
	}
	if !topo.devices[diskID(disk.Slot)] {
		return fmt.Errorf("vm: disk %d is not attached to the running guest", disk.Slot)
	}
	if _, err := h.Mon.Run(ctx, "block_resize", map[string]interface{}{
		"node-name": diskNode(disk.Slot), "size": sizeBytes,
	}); err != nil {
		return fmt.Errorf("vm: growing disk %d: %w", disk.Slot, err)
	}
	return nil
}

// blockInfo is one query-block entry, as much of it as a drive needs.
type blockInfo struct {
	Qdev     string `json:"qdev"`
	Inserted *struct {
		File string `json:"file"`
	} `json:"inserted,omitempty"`
}

func (h *Hotplug) driveMedium(ctx context.Context, id string) (string, bool, error) {
	var blocks []blockInfo
	if err := h.run(ctx, "query-block", nil, &blocks); err != nil {
		return "", false, fmt.Errorf("vm: reading the guest's drives: %w", err)
	}
	for _, b := range blocks {
		if b.Qdev != id && !strings.HasSuffix(b.Qdev, "/"+id) && !strings.Contains(b.Qdev, "/"+id+"/") {
			continue
		}
		if b.Inserted == nil {
			return "", true, nil
		}
		return b.Inserted.File, true, nil
	}
	return "", false, nil
}

// InsertMedia puts a disc into an existing drive. SATA cannot grow a drive on a
// running guest, so the drive has to be there; one emptied by a live eject is.
func (h *Hotplug) InsertMedia(ctx context.Context, disk Disk) error {
	if disk.Path == "" {
		return fmt.Errorf("vm: no disc to insert into drive %d", disk.Slot)
	}
	topo, err := h.topology(ctx)
	if err != nil {
		return err
	}
	id := diskID(disk.Slot)
	if !topo.devices[id] {
		return refuse(HotplugNoDrive, "the running guest has no drive in slot %d", disk.Slot)
	}
	current, _, err := h.driveMedium(ctx, id)
	if err != nil {
		return err
	}
	if current == disk.Path {
		return nil
	}
	if _, err := h.Mon.Run(ctx, "blockdev-change-medium", map[string]interface{}{
		"id": id, "filename": disk.Path, "format": "raw", "read-only-mode": "read-only",
	}); err != nil {
		return fmt.Errorf("vm: inserting %s: %w", disk.Path, err)
	}
	// The nodes the command line opened for the previous disc are no longer
	// attached, and would hold that ISO open until the guest stops.
	h.dropNodes(ctx, disk.Slot)
	return nil
}

// EjectMedia empties a drive and leaves the drive itself, which the guest keeps
// until it stops.
func (h *Hotplug) EjectMedia(ctx context.Context, disk Disk) error {
	topo, err := h.topology(ctx)
	if err != nil {
		return err
	}
	id := diskID(disk.Slot)
	if !topo.devices[id] {
		return nil
	}
	current, _, err := h.driveMedium(ctx, id)
	if err != nil {
		return err
	}
	if current != "" {
		if _, err := h.Mon.Run(ctx, "eject", map[string]interface{}{"id": id, "force": true}); err != nil {
			return fmt.Errorf("vm: ejecting the disc in drive %d: %w", disk.Slot, err)
		}
	}
	h.dropNodes(ctx, disk.Slot)
	return nil
}

// AttachNIC plugs a NIC whose host side the caller has already created. netdev
// is the netdev_add argument set without its id. A failed device_add removes
// the backend again.
func (h *Hotplug) AttachNIC(ctx context.Context, nic NIC, netdev map[string]interface{}) error {
	topo, err := h.topology(ctx)
	if err != nil {
		return err
	}
	id := nicID(nic.Slot)
	if topo.devices[id] {
		return nil
	}
	device := map[string]interface{}{"id": id, "netdev": netdevID(nic.Slot), "mac": nic.MAC}
	model := nic.Model
	if model == "" || model == "virtio-net" {
		model = "virtio-net-pci"
	}
	device["driver"] = model
	if isQ35(h.MachineType) {
		if nic.Slot < 0 || nic.Slot >= NICPorts {
			return refuse(HotplugNoFreePort, "NIC slot %d is beyond the %d hot-plug ports a guest has", nic.Slot, NICPorts)
		}
		port := NICPortID(nic.Slot)
		if !topo.devices[port] {
			return refuse(HotplugRestartRequired, "this guest was started before it had hot-plug ports")
		}
		device["bus"] = port
	}

	args := map[string]interface{}{"id": netdevID(nic.Slot)}
	for key, value := range netdev {
		args[key] = value
	}
	h.bestEffort(ctx, "netdev_del", map[string]interface{}{"id": netdevID(nic.Slot)}) // a stale backend from a failed attempt; absent is the usual answer
	if _, err := h.Mon.Run(ctx, "netdev_add", args); err != nil {
		return fmt.Errorf("vm: opening the network backend for NIC %d: %w", nic.Slot, err)
	}
	if _, err := h.Mon.Run(ctx, "device_add", device); err != nil {
		h.bestEffort(ctx, "netdev_del", map[string]interface{}{"id": netdevID(nic.Slot)}) // rollback of the backend added above
		return fmt.Errorf("vm: plugging NIC %d into the guest: %w", nic.Slot, err)
	}
	if nic.LinkDown {
		if _, err := h.Mon.Run(ctx, "set_link", map[string]interface{}{"name": id, "up": false}); err != nil {
			return fmt.Errorf("vm: cutting NIC %d's link: %w", nic.Slot, err)
		}
	}
	return nil
}

// DetachNIC asks the guest to release a NIC, then removes its backend. The
// caller removes the host side afterwards.
func (h *Hotplug) DetachNIC(ctx context.Context, nic NIC) error {
	topo, err := h.topology(ctx)
	if err != nil {
		return err
	}
	id := nicID(nic.Slot)
	if topo.devices[id] {
		if isQ35(h.MachineType) && topo.parent[id] == "" {
			if nic.PCIAddr == "" && nic.Slot < NICPorts {
				return refuse(HotplugRestartRequired, "this NIC was attached before the guest had hot-plug ports")
			}
			return refuse(HotplugFixedDevice, "NIC %d sits on the root bus, which cannot unplug it", nic.Slot)
		}
		if err := h.unplug(ctx, topo, id); err != nil {
			return err
		}
	}
	h.bestEffort(ctx, "netdev_del", map[string]interface{}{"id": netdevID(nic.Slot)}) // already gone is the goal state
	return nil
}

// NICAttached reports whether the running guest already has NIC slot, so a
// retried attach does not rebuild the host side under a live interface.
func (h *Hotplug) NICAttached(ctx context.Context, slot int) (bool, error) {
	topo, err := h.topology(ctx)
	if err != nil {
		return false, err
	}
	return topo.devices[nicID(slot)], nil
}

// SetLink connects or disconnects a NIC's cable without unplugging it.
func (h *Hotplug) SetLink(ctx context.Context, nic NIC, up bool) error {
	topo, err := h.topology(ctx)
	if err != nil {
		return err
	}
	id := nicID(nic.Slot)
	if !topo.devices[id] {
		return fmt.Errorf("vm: NIC %d is not attached to the running guest", nic.Slot)
	}
	if _, err := h.Mon.Run(ctx, "set_link", map[string]interface{}{"name": id, "up": up}); err != nil {
		return fmt.Errorf("vm: setting NIC %d's link: %w", nic.Slot, err)
	}
	return nil
}

// unplug sends device_del and waits for the device to be gone: DEVICE_DELETED
// when the event arrives, the device list when it does not.
func (h *Hotplug) unplug(ctx context.Context, _ topology, id string) error {
	var gone <-chan struct{}
	if h.Deleted != nil {
		ch, release := h.Deleted(id)
		defer release()
		gone = ch
	}
	if _, err := h.Mon.Run(ctx, "device_del", map[string]interface{}{"id": id}); err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("vm: asking the guest to release %s: %w", id, err)
	}

	wait := releaseWait(ctx, h.ReleaseWait)
	poll := h.Poll
	if poll <= 0 {
		poll = defaultPoll
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-gone:
			return nil
		case <-ticker.C:
			topo, err := h.topology(ctx)
			if err == nil && !topo.devices[id] {
				return nil
			}
		case <-deadline.C:
			return refuse(HotplugGuestRefused, "the guest did not release %s within %s; its kernel may have no "+
				"PCI hot-plug driver (acpiphp or pciehp), and the device goes when the guest stops", id, wait)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// releaseWait is how long one unplug may wait: the configured wait, cut to what
// the caller's deadline leaves after the cleanup.
func releaseWait(ctx context.Context, configured time.Duration) time.Duration {
	wait := configured
	if wait <= 0 {
		wait = defaultReleaseWait
	}
	if deadline, ok := ctx.Deadline(); ok {
		if left := time.Until(deadline) - releaseCleanup; left < wait {
			wait = left
		}
	}
	if wait < minReleaseWait {
		wait = minReleaseWait
	}
	return wait
}
