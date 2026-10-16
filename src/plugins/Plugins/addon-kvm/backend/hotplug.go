// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Live hardware changes. The agent plugs and unplugs over QMP; this side
// decides whether a change may happen live at all, and keeps the rows equal to
// what the running guest has: a row is written only after the agent confirmed,
// and a step that fails after the guest changed is undone on the guest too.
const (
	msgVMDiskHotplug = "vm_disk_hotplug"
	msgVMMediaChange = "vm_media_change"
	msgVMNICHotplug  = "vm_nic_hotplug"
	// agentHotplugUnclaimed is the reply Notice for a device the guest never picked up.
	agentHotplugUnclaimed = "hotplug_unclaimed"
)

// agentHotplug mirrors the agent's VMHotplugSpec.
type agentHotplug struct {
	Action string     `json:"action"`
	Disk   *agentDisk `json:"disk,omitempty"`
	NIC    *agentNIC  `json:"nic,omitempty"`
	LinkUp bool       `json:"linkUp,omitempty"`
}

// Errors a live change maps onto its own codes.
var (
	ErrBootDiskRunning    = errors.New("the boot disk cannot change while the guest runs")
	ErrHotplugUnavailable = errors.New("this host cannot change hardware on a running guest")
	ErrHotplugRestart     = errors.New("the guest was started before it had hot-plug ports")
	ErrHotplugBus         = errors.New("that bus cannot change while the guest runs")
	ErrHotplugController  = errors.New("the guest has no SCSI controller until it restarts")
	ErrHotplugNoPort      = errors.New("the slot is beyond the guest's hot-plug ports")
	ErrHotplugFixed       = errors.New("the device sits on a bus that cannot unplug it")
	ErrHotplugRefused     = errors.New("the guest did not release the device")
	ErrMediaNoDrive       = errors.New("the guest has no empty disc drive")
)

// hotplugRefusals maps the agent's refusal codes onto those errors.
var hotplugRefusals = map[string]error{
	"hotplug_restart_required": ErrHotplugRestart,
	"hotplug_no_free_port":     ErrHotplugNoPort,
	"hotplug_bus_unsupported":  ErrHotplugBus,
	"hotplug_no_controller":    ErrHotplugController,
	"hotplug_fixed_device":     ErrHotplugFixed,
	"hotplug_guest_refused":    ErrHotplugRefused,
	"hotplug_boot_disk":        ErrBootDiskRunning,
	"hotplug_no_drive":         ErrMediaNoDrive,
}

// nicModels are the interface types the agent can put on the command line.
var nicModels = map[string]bool{"virtio-net": true, "e1000e": true, "e1000": true}

// effectiveBootIndex is where a disk sits in the boot order, as the agent
// derives it: an explicit index wins, the boot flag means first, and a disk
// with neither is not in the order at all.
func effectiveBootIndex(disk Disk) (int, bool) {
	if disk.BootIndex > 0 {
		return disk.BootIndex, true
	}
	if disk.IsBoot {
		return 0, true
	}
	return 0, false
}

// isBootDisk reports whether disk is the one the guest boots from: the disk
// flagged as boot, or the non-disc disk first in the boot order. Either may be
// what the running operating system lives on, so neither changes under it.
func isBootDisk(disks []Disk, disk Disk) bool {
	if disk.IsCDROM {
		return false
	}
	if disk.IsBoot {
		return true
	}
	best, found, bestID := 0, false, ""
	for _, d := range disks {
		if d.IsCDROM {
			continue
		}
		index, ok := effectiveBootIndex(d)
		if ok && (!found || index < best) {
			best, found, bestID = index, true, d.ID
		}
	}
	return found && bestID == disk.ID
}

// toAgentDisk is one disk row as the agent reads it.
func toAgentDisk(disk Disk) *agentDisk {
	return &agentDisk{
		DiskID: disk.ID, PoolID: disk.PoolID, PoolBackend: disk.PoolBackend,
		PoolTarget: disk.PoolTarget, Slot: disk.Slot, Bus: disk.Bus,
		VolumePath: disk.VolumePath, Format: disk.Format, SizeBytes: disk.SizeBytes,
		BackingPath: disk.BackingPath, CacheMode: disk.CacheMode,
		Discard: disk.Discard, ReadOnly: disk.ReadOnly,
		IsBoot: disk.IsBoot, IsCDROM: disk.IsCDROM,
		BootIndex: disk.BootIndex, ShareRW: disk.ShareRW,
		ReadIOPS: disk.ReadIOPS, WriteIOPS: disk.WriteIOPS,
		ReadBPS: disk.ReadBPS, WriteBPS: disk.WriteBPS,
	}
}

// toAgentNIC is one interface row as the agent reads it.
func toAgentNIC(nic NIC) *agentNIC {
	return &agentNIC{
		NICID: nic.ID, NetworkID: nic.NetworkID, Bridge: nic.Bridge,
		VLANTag: nic.VLANTag, AttachKind: nic.AttachKind,
		ParentLink: nic.ParentLink, LinkMode: nic.LinkMode,
		Slot: nic.Slot, Model: nic.Model,
		MAC: nic.MAC, TapName: nic.TapName, PCIAddr: nic.PCIAddr, LinkDown: nic.LinkDown,
	}
}

// liveGuest reports whether a change to vm has to happen live, and refuses one
// that cannot: a paused guest cannot answer an unplug, and a host without the
// feature cannot plug anything.
func (s *Service) liveGuest(ctx context.Context, vm VM) (bool, error) {
	switch vm.State {
	case StateRunning:
		if !s.SupportsFeature(ctx, vm.EnvironmentID, FeatureHotplug) {
			return true, ErrHotplugUnavailable
		}
		return true, nil
	case StatePaused:
		return true, ErrRunning
	}
	return false, nil
}

// hotplug sends one live change. A refusal the agent reports by code comes
// back as the matching error, so the operator reads what to do next.
func (s *Service) hotplug(ctx context.Context, vm VM, msgType string, change agentHotplug) (*agentPayload, error) {
	spec := s.buildSpec(ctx, vm)
	// The seed is for a first boot; a hot-plug has no use for the password hash.
	spec.CloudInit = nil
	reply, err := s.call(ctx, vm.EnvironmentID, msgType, agentPayload{VMID: vm.ID, Spec: spec, Hotplug: &change})
	if err != nil {
		return nil, err
	}
	if reply.ErrorCode != "" {
		if refusal, ok := hotplugRefusals[reply.ErrorCode]; ok {
			return reply, refusal
		}
		return reply, fmt.Errorf("the host refused the change: %s", reply.ErrorCode)
	}
	return reply, nil
}

// addDiskLive allocates the volume, plugs it in, and only then records it.
// Each step that fails undoes the ones before it, on the host as well.
func (s *Service) addDiskLive(ctx context.Context, vm VM, disk Disk) (Disk, error) {
	reply, err := s.hotplug(ctx, vm, msgVMDiskHotplug, agentHotplug{Action: "attach", Disk: toAgentDisk(disk)})
	if err != nil {
		s.dropVolume(vm, disk)
		return Disk{}, err
	}
	if err := s.insertDisk(ctx, disk); err != nil {
		if _, undoErr := s.hotplug(context.WithoutCancel(ctx), vm, msgVMDiskHotplug,
			agentHotplug{Action: "detach", Disk: toAgentDisk(disk)}); undoErr != nil {
			s.logger.Warn("kvm: unplugging a disk whose row could not be written",
				"vm", vm.ID, "disk", disk.ID, "error", undoErr)
			return Disk{}, err
		}
		s.dropVolume(vm, disk)
		return Disk{}, err
	}
	disk.Notice = reply.Notice
	return disk, nil
}

// dropVolume removes a volume nothing records any more. Logged rather than
// returned: the caller is already reporting the failure that made it orphaned.
func (s *Service) dropVolume(vm VM, disk Disk) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := s.call(ctx, vm.EnvironmentID, msgVMDiskDelete, agentPayload{
		VMID: vm.ID,
		Disk: &agentDisk{DiskID: disk.ID, PoolID: disk.PoolID, PoolBackend: disk.PoolBackend,
			PoolTarget: disk.PoolTarget, VolumePath: disk.VolumePath},
	}); err != nil {
		s.logger.Warn("kvm: removing the volume of a disk that was never attached",
			"vm", vm.ID, "path", disk.VolumePath, "error", err)
	}
}

// deleteDiskLive unplugs a data disk, destroys its volume and drops its row.
// A volume that will not go away is plugged back in, so the guest keeps a disk
// its row still names rather than losing it with nothing to show for it.
func (s *Service) deleteDiskLive(ctx context.Context, vm VM, disk Disk) error {
	change := agentHotplug{Action: "detach", Disk: toAgentDisk(disk)}
	if _, err := s.hotplug(ctx, vm, msgVMDiskHotplug, change); err != nil {
		return err
	}
	if _, err := s.call(ctx, vm.EnvironmentID, msgVMDiskDelete, agentPayload{
		VMID: vm.ID,
		Disk: &agentDisk{DiskID: disk.ID, PoolID: disk.PoolID, PoolBackend: disk.PoolBackend,
			PoolTarget: disk.PoolTarget, VolumePath: disk.VolumePath},
	}); err != nil {
		change.Action = "attach"
		if _, undoErr := s.hotplug(context.WithoutCancel(ctx), vm, msgVMDiskHotplug, change); undoErr != nil {
			s.logger.Warn("kvm: re-plugging a disk whose volume could not be removed",
				"vm", vm.ID, "disk", disk.ID, "error", undoErr)
		}
		return err
	}
	return s.forgetDisk(ctx, disk.ID)
}

// forgetDisk drops a removed disk's row and its backup chain, whose bitmap
// lived inside the image that is now gone.
func (s *Service) forgetDisk(ctx context.Context, diskID string) error {
	if err := s.InvalidateChain(ctx, diskID,
		"this disk was removed, so its change log is gone with it"); err != nil {
		s.logger.Warn("kvm: invalidating a chain after a disk delete", "disk", diskID, "error", err)
	}
	_, err := s.db.Exec(ctx, `DELETE FROM plugin_addon_kvm_disks WHERE id = ?`, diskID)
	return err
}

// emptyDrive returns the guest's first disc drive with nothing in it.
func emptyDrive(disks []Disk) (Disk, bool) {
	for _, disk := range disks {
		if disk.IsCDROM && disk.VolumePath == "" {
			return disk, true
		}
	}
	return Disk{}, false
}

// placeMedia puts a disc into the guest. An empty drive is filled first, live
// or not; a running guest with none is refused, because SATA cannot grow a
// drive under it, and a stopped one gets a new drive as it always has.
func (s *Service) placeMedia(ctx context.Context, vm VM, path string) (Disk, error) {
	live, err := s.liveGuest(ctx, vm)
	if err != nil {
		return Disk{}, err
	}
	drive, found := emptyDrive(vm.Disks)
	if !found {
		if live {
			return Disk{}, ErrMediaNoDrive
		}
		slot, err := s.nextDiskSlot(ctx, vm.ID)
		if err != nil {
			return Disk{}, err
		}
		return s.recordMedia(ctx, vm.ID, slot, path)
	}

	drive.VolumePath = path
	if live {
		if _, err := s.hotplug(ctx, vm, msgVMMediaChange, agentHotplug{Action: "insert", Disk: toAgentDisk(drive)}); err != nil {
			return Disk{}, err
		}
	}
	if err := s.setDrivePath(ctx, drive.ID, path); err != nil {
		if live {
			if _, undoErr := s.hotplug(context.WithoutCancel(ctx), vm, msgVMMediaChange,
				agentHotplug{Action: "eject", Disk: toAgentDisk(drive)}); undoErr != nil {
				s.logger.Warn("kvm: ejecting a disc whose row could not be written",
					"vm", vm.ID, "disk", drive.ID, "error", undoErr)
			}
		}
		return Disk{}, err
	}
	drive.UpdatedAt = time.Now().UTC()
	return drive, nil
}

// setDrivePath records what a drive holds; "" is an empty drive.
func (s *Service) setDrivePath(ctx context.Context, diskID, path string) error {
	if _, err := s.db.Exec(ctx,
		`UPDATE plugin_addon_kvm_disks SET volume_path = ?, updated_at = ? WHERE id = ? AND is_cdrom = 1`,
		path, time.Now().UTC().Format(time.RFC3339), diskID); err != nil {
		return fmt.Errorf("recording the disc: %w", err)
	}
	return nil
}

// UpdateNIC changes an interface's link, live or not, and its type while the
// guest is stopped.
func (s *Service) UpdateNIC(ctx context.Context, vmID, nicID string, req NICUpdateRequest) (NIC, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return NIC{}, err
	}
	var target *NIC
	for i := range vm.NICs {
		if vm.NICs[i].ID == nicID {
			target = &vm.NICs[i]
		}
	}
	if target == nil {
		return NIC{}, ErrNotFound
	}

	if req.Model != "" && req.Model != target.Model {
		if !nicModels[req.Model] {
			return NIC{}, invalid(codeBadRequest, fmt.Sprintf("%q is not an interface type", req.Model))
		}
		if vm.State != StateStopped {
			// The device type is fixed once QEMU has created it.
			return NIC{}, ErrRunning
		}
		target.Model = req.Model
	}
	if req.LinkUp != nil {
		live, err := s.liveGuest(ctx, vm)
		if err != nil {
			return NIC{}, err
		}
		target.LinkDown = !*req.LinkUp
		if live {
			if _, err := s.hotplug(ctx, vm, msgVMNICHotplug,
				agentHotplug{Action: "link", NIC: toAgentNIC(*target), LinkUp: *req.LinkUp}); err != nil {
				return NIC{}, err
			}
		}
	}

	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_nics SET model = ?, link_down = ?, updated_at = ?
		 WHERE id = ? AND vm_id = ?`,
		target.Model, boolToInt(target.LinkDown), time.Now().UTC().Format(time.RFC3339), nicID, vmID); err != nil {
		if req.LinkUp != nil && vm.State == StateRunning {
			// The guest's cable moved but the row did not; put it back so
			// the next restart and the screen agree with the guest.
			if _, undoErr := s.hotplug(context.WithoutCancel(ctx), vm, msgVMNICHotplug,
				agentHotplug{Action: "link", NIC: toAgentNIC(*target), LinkUp: !*req.LinkUp}); undoErr != nil {
				s.logger.Warn("kvm: restoring a NIC link whose row could not be written",
					"vm", vmID, "nic", nicID, "error", undoErr)
			}
		}
		return NIC{}, fmt.Errorf("updating the interface: %w", err)
	}
	return *target, nil
}

// runningNow reports whether a guest is running, for the audit row of a
// hardware change: the same request is a row edit on a stopped guest and a
// change to a live machine on a running one, and the trail should say which.
func (s *Service) runningNow(ctx context.Context, vmID string) bool {
	vm, err := s.Get(ctx, vmID)
	return err == nil && vm.Running()
}
