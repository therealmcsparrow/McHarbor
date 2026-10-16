// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/netconf"
	"github.com/therealmcsparrow/mcharbor-agent/compute/storage"
	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"golang.org/x/sys/unix"
)

// fdSender is the monitor's getfd, asserted rather than added to vm.QMPRunner
// so the hot-plug engine stays testable without a socket.
type fdSender interface {
	SendFD(ctx context.Context, name string, fd int) error
}

// handleHotplug answers vm_disk_hotplug, vm_media_change and vm_nic_hotplug.
//
// A refusal the operator can act on comes back as an ErrorCode on a successful
// reply, so McHarbor can say "restart once" rather than "the host failed".
func (rt *computeRuntime) handleHotplug(ctx context.Context, msgType string, req *VMPayload, reply *VMPayload) error {
	if req.Hotplug == nil || req.Spec == nil {
		return fmt.Errorf("compute: %s carried no change", msgType)
	}
	spec := toVMSpec(req.VMID, req.Spec)
	change := req.Hotplug

	err := rt.vms.WithHotplug(ctx, spec, func(h *vm.Hotplug) error {
		switch msgType {
		case MsgVMDiskHotplug:
			return rt.hotplugDisk(ctx, h, change, reply)
		case MsgVMMediaChange:
			return hotplugMedia(ctx, h, change)
		default:
			return rt.hotplugNIC(ctx, h, req.VMID, change, reply)
		}
	})
	var refusal *vm.HotplugError
	if errors.As(err, &refusal) {
		rt.logger.Info("compute: hot-plug refused", "vm", req.VMID, "message", msgType,
			"action", change.Action, "code", refusal.Code, "reason", refusal.Detail)
		reply.ErrorCode = refusal.Code
		return nil
	}
	return err
}

// hotplugDiskFrom converts a protocol disk into the engine's own.
func hotplugDiskFrom(in *VMDiskSpec) vm.Disk {
	disk := vm.Disk{
		ID: in.DiskID, Slot: in.Slot, Bus: in.Bus, Path: in.VolumePath, Format: in.Format,
		ReadOnly: in.ReadOnly, IsBoot: in.IsBoot, IsCDROM: in.IsCDROM, Discard: in.Discard,
		CacheMode: in.CacheMode, SizeBytes: in.SizeBytes, BootIndex: in.BootIndex, ShareRW: in.ShareRW,
		ReadIOPS: in.ReadIOPS, WriteIOPS: in.WriteIOPS, ReadBPS: in.ReadBPS, WriteBPS: in.WriteBPS,
	}
	if info, err := os.Stat(in.VolumePath); err == nil && info.Mode()&os.ModeDevice != 0 {
		disk.IsBlock = true
	}
	return disk
}

func (rt *computeRuntime) hotplugDisk(ctx context.Context, h *vm.Hotplug, change *VMHotplugSpec, reply *VMPayload) error {
	if change.Disk == nil {
		return errors.New("compute: a disk hot-plug carried no disk")
	}
	disk := hotplugDiskFrom(change.Disk)
	switch change.Action {
	case "attach":
		if err := h.AttachDisk(ctx, disk); err != nil {
			return err
		}
		rt.noteUnclaimed(reply, func() (bool, error) { return h.DiskClaimed(ctx, disk.Slot, hotplugClaimWait) })
		return nil
	case "detach":
		return h.DetachDisk(ctx, disk)
	case "resize":
		if disk.IsBoot {
			// Refused before the pool driver grows anything, not after.
			return &vm.HotplugError{Code: vm.HotplugBootDisk, Detail: "the boot disk cannot be resized while the guest runs"}
		}
		size := change.Disk.SizeBytes
		if disk.IsBlock {
			// A zvol or an LV is grown by its pool first; QEMU is then told
			// the device is larger. A file is grown by QEMU itself.
			driver, err := rt.pools.Get(change.Disk.PoolBackend)
			if err != nil {
				return err
			}
			pool := storage.Pool{ID: change.Disk.PoolID, Backend: change.Disk.PoolBackend, Target: change.Disk.PoolTarget}
			grown, err := driver.ResizeVolume(ctx, pool, change.Disk.VolumePath, size)
			if err != nil {
				return err
			}
			size = grown
		}
		if err := h.ResizeDisk(ctx, disk, size); err != nil {
			return err
		}
		reply.Disk = &VMDiskSpec{DiskID: change.Disk.DiskID, Slot: change.Disk.Slot, SizeBytes: size}
		return nil
	}
	return fmt.Errorf("compute: %q is not a disk hot-plug action", change.Action)
}

func hotplugMedia(ctx context.Context, h *vm.Hotplug, change *VMHotplugSpec) error {
	if change.Disk == nil {
		return errors.New("compute: a media change carried no drive")
	}
	disk := vm.Disk{Slot: change.Disk.Slot, Path: change.Disk.VolumePath, IsCDROM: true}
	switch change.Action {
	case "insert":
		return h.InsertMedia(ctx, disk)
	case "eject":
		return h.EjectMedia(ctx, disk)
	}
	return fmt.Errorf("compute: %q is not a media action", change.Action)
}

func (rt *computeRuntime) hotplugNIC(ctx context.Context, h *vm.Hotplug, vmID string, change *VMHotplugSpec, reply *VMPayload) error {
	if change.NIC == nil {
		return errors.New("compute: a NIC hot-plug carried no NIC")
	}
	nic := vm.NIC{
		ID: change.NIC.NICID, Slot: change.NIC.Slot, Model: change.NIC.Model, MAC: change.NIC.MAC,
		TapName: change.NIC.TapName, PCIAddr: change.NIC.PCIAddr, LinkDown: change.NIC.LinkDown,
	}
	switch change.Action {
	case "attach":
		return rt.attachNIC(ctx, h, vmID, nic, change.NIC, reply)
	case "detach":
		if err := h.DetachNIC(ctx, nic); err != nil {
			return err
		}
		releaseNIC(vmID, nic)
		return nil
	case "link":
		return h.SetLink(ctx, nic, change.LinkUp)
	}
	return fmt.Errorf("compute: %q is not a NIC action", change.Action)
}

// attachNIC builds the host side, hands it to QEMU and plugs the NIC in, and
// takes the host side down again when the plug fails.
func (rt *computeRuntime) attachNIC(ctx context.Context, h *vm.Hotplug, vmID string, nic vm.NIC, wanted *VMNICSpec, reply *VMPayload) error {
	if nic.MAC == "" {
		return fmt.Errorf("compute: NIC in slot %d has no MAC address", nic.Slot)
	}
	attached, err := h.NICAttached(ctx, nic.Slot)
	if err != nil {
		return err
	}
	if attached {
		return nil
	}
	if err := prepareGuestNIC(vmID, &nic, wanted); err != nil {
		return err
	}

	netdev := map[string]interface{}{"type": "tap", "vhost": vm.VhostEnabled()}
	if nic.DevicePath != "" {
		name := fmt.Sprintf("fd-net%d", nic.Slot)
		if err := sendNICDescriptor(ctx, h, name, nic.DevicePath); err != nil {
			releaseNIC(vmID, nic)
			return err
		}
		netdev["fd"] = name
	} else {
		netdev["ifname"] = nic.TapName
		netdev["script"] = "no"
		netdev["downscript"] = "no"
	}
	if err := h.AttachNIC(ctx, nic, netdev); err != nil {
		releaseNIC(vmID, nic)
		return err
	}
	rt.noteUnclaimed(reply, func() (bool, error) { return h.NICClaimed(ctx, nic.Slot, hotplugClaimWait) })
	return nil
}

// hotplugClaimWait bounds how long an attach waits for the guest to pick the device
// up. A guest with hot-plug support assigns its BARs within a second or two; one
// without never does, and the operator should hear so rather than wait longer.
const hotplugClaimWait = 8 * time.Second

// noteUnclaimed marks a successful attach the guest never picked up. A failed check
// is logged and leaves the reply alone: the attach itself worked.
func (rt *computeRuntime) noteUnclaimed(reply *VMPayload, check func() (bool, error)) {
	claimed, err := check()
	if err != nil {
		rt.logger.Debug("compute: could not confirm the guest picked up a hot-added device", "error", err)
		return
	}
	if !claimed {
		reply.Notice = hotplugUnclaimedNotice
	}
}

// hotplugUnclaimedNotice is VMPayload.Notice for a device the guest never claimed.
const hotplugUnclaimedNotice = "hotplug_unclaimed"

// sendNICDescriptor opens a macvtap and passes it to QEMU. The agent's copy is
// closed once QEMU holds its own.
func sendNICDescriptor(ctx context.Context, h *vm.Hotplug, name, device string) error {
	sender, ok := h.Mon.(fdSender)
	if !ok {
		return errors.New("compute: this monitor cannot pass a descriptor")
	}
	fd, err := unix.Open(device, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("compute: opening %s: %w", device, err)
	}
	defer func() { errlog.Log(unix.Close(fd)) }()
	return sender.SendFD(ctx, name, fd)
}

// releaseNIC removes a NIC's host side. DeleteLink refuses anything outside the
// managed namespace, so a crafted name cannot take out the host's uplink.
func releaseNIC(vmID string, nic vm.NIC) {
	tap := nic.TapName
	if tap == "" {
		tap = netconf.TapName(vmID, nic.Slot)
	}
	// A link that is already gone is not an error, so only a real failure logs.
	errlog.Log(netconf.DeleteLink(tap))
	errlog.Log(netconf.DeleteLink(netconf.DirectName(vmID, nic.Slot)))
}
