// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package computesim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"strings"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/demo/fakeagent"
)

var (
	errNotRunning = errors.New("the guest is not running")
	errPaused     = errors.New("the guest is paused; resume it instead")
	errNotPaused  = errors.New("the guest is not paused")
)

// vmTransition applies one power operation and returns the QMP event kind a
// real host would emit for it, or "" when nothing changed.
func vmTransition(g *vm, op string, now time.Time) (string, error) {
	running := g.State == stateRunning
	paused := g.State == statePaused
	switch op {
	case coreagent.MsgVMStart:
		switch {
		case running:
			return "", nil
		case paused:
			return "", errPaused
		}
		g.State, g.RunState, g.LastError, g.StartedAt = stateRunning, stateRunning, "", now
		return "", nil
	case coreagent.MsgVMShutdown, coreagent.MsgVMStop:
		if !running && !paused {
			return "", nil
		}
		g.State, g.RunState, g.StartedAt = stateStopped, "", time.Time{}
		if op == coreagent.MsgVMShutdown {
			return "powerdown", nil
		}
		return "shutdown", nil
	case coreagent.MsgVMReset:
		if !running && !paused {
			return "", errNotRunning
		}
		g.State, g.RunState, g.StartedAt = stateRunning, stateRunning, now
		return "reset", nil
	case coreagent.MsgVMPause:
		switch {
		case paused:
			return "", nil
		case !running:
			return "", errNotRunning
		}
		g.State, g.RunState = statePaused, statePaused
		return "stop", nil
	case coreagent.MsgVMResume:
		switch {
		case running:
			return "", nil
		case !paused:
			return "", errNotPaused
		}
		g.State, g.RunState = stateRunning, stateRunning
		return "resume", nil
	}
	return "", fmt.Errorf("unknown power operation %s", op)
}

// vmSnapshotCreate records a snapshot on top of the current one.
func vmSnapshotCreate(g *vm, id, handle string, withMemory bool, consistency string, now time.Time) (*snapshot, error) {
	if handle == "" {
		return nil, errors.New("the snapshot carried no name")
	}
	for _, snap := range g.Snapshots {
		if snap.Handle == handle {
			return nil, fmt.Errorf("a snapshot named %q already exists", handle)
		}
	}
	if id == "" {
		id = stableID(0, now, "snap", g.ID, handle)
	}
	snap := &snapshot{ID: id, ParentID: g.Current, Name: handle, Handle: handle, CreatedAt: now,
		WithMemory: withMemory, Consistency: stateOr(consistency, "crash"), savedState: g.State,
		SizeBytes: int64(len(g.Snapshots)+1) * 3 * gib}
	g.Snapshots = append(g.Snapshots, snap)
	g.Current = snap.ID
	return snap, nil
}

func findSnapshot(list []*snapshot, id, handle string) *snapshot {
	for _, snap := range list {
		if (id != "" && snap.ID == id) || (handle != "" && snap.Handle == handle) {
			return snap
		}
	}
	return nil
}

// vmSnapshotRevert moves the guest back to a snapshot. A memory snapshot brings
// back the power state it was taken in; a disk-only one leaves the guest off.
func vmSnapshotRevert(g *vm, id, handle string, now time.Time) {
	snap := findSnapshot(g.Snapshots, id, handle)
	if snap == nil {
		return
	}
	g.Current = snap.ID
	if snap.WithMemory && snap.savedState == stateRunning {
		g.State, g.RunState, g.StartedAt = stateRunning, stateRunning, now
		return
	}
	if g.State == stateRunning || g.State == statePaused {
		g.State, g.RunState, g.StartedAt = stateStopped, "", time.Time{}
	}
}

// vmSnapshotDelete drops a snapshot, re-parents its children onto its parent,
// and moves the current pointer up when it pointed at the deleted one.
func vmSnapshotDelete(g *vm, id, handle string) {
	snap := findSnapshot(g.Snapshots, id, handle)
	if snap == nil {
		return
	}
	kept := g.Snapshots[:0]
	for _, other := range g.Snapshots {
		if other == snap {
			continue
		}
		if other.ParentID == snap.ID {
			other.ParentID = snap.ParentID
		}
		kept = append(kept, other)
	}
	g.Snapshots = kept
	if g.Current == snap.ID {
		g.Current = snap.ParentID
	}
}

func (s *Sim) handleVM(ctx context.Context, host *estate.Host, msg *coreagent.WSMessage, send fakeagent.Sender) {
	req := msg.VM
	if req == nil {
		req = &coreagent.VMPayload{}
	}
	reply := &coreagent.VMPayload{Operation: msg.Type, EnvID: req.EnvID, VMID: req.VMID}
	out := &coreagent.WSMessage{Type: coreagent.MsgVMResult, ID: msg.ID, VM: reply}

	switch msg.Type {
	case coreagent.MsgVMImportInspect, coreagent.MsgVMImportConvert, coreagent.MsgVMImportStage,
		coreagent.MsgVMImportDiscard, coreagent.MsgVMBackupRun, coreagent.MsgVMBackupRestore:
		reply.Error, reply.ErrorCode = ErrDemoRefused, "demo_mode"
		s.deliver(send, out)
		return
	case coreagent.MsgVMMigrate:
		out.Type = coreagent.MsgVMMigrateResult
		reply.Error, reply.ErrorCode = ErrDemoRefused, "demo_mode"
		s.deliver(send, out)
		return
	case coreagent.MsgVMSpiceTicket:
		// Demo hosts serve no SPICE, so there is nothing a remote-viewer could
		// reach; the capability answer already says so and this is the backstop.
		reply.Error, reply.ErrorCode = ErrDemoRefused, "demo_mode"
		s.deliver(send, out)
		return
	case coreagent.MsgVMCloneRequest:
		s.startVMClone(ctx, host, msg, req, send)
		return
	}

	var event string
	s.mu.Lock()
	hs := s.hostFor(host)
	var err error
	if hs == nil {
		err = errors.New("this demo host runs no guests")
	} else {
		event, err = s.applyVM(hs, msg.Type, req, reply)
	}
	s.mu.Unlock()

	if err != nil {
		reply.Error = err.Error()
	} else {
		reply.Success = true
	}
	s.deliver(send, out)
	if event != "" {
		s.deliver(send, &coreagent.WSMessage{Type: coreagent.MsgVMEvent, VM: &coreagent.VMPayload{
			Operation: "event", VMID: req.VMID,
			Event: &coreagent.VMEvent{VMID: req.VMID, Kind: event, Timestamp: s.now().Format(time.RFC3339)},
		}})
	}
}

// vmFor returns the guest id names on hs, adopting a guest the simulator has
// not seen — one created in the UI after the last restart — from its spec.
func (s *Sim) vmFor(hs *hostSim, id string, spec *coreagent.VMSpec) *vm {
	if g, ok := hs.vms[id]; ok {
		if spec != nil {
			applyVMSpec(g, spec)
		}
		return g
	}
	g := &vm{ID: id, Name: "vm-" + id, State: stateStopped, CreatedAt: s.now(), MachineType: "pc-q35-8.2",
		UUID: stableUUID(id), VCPUs: 2, MemoryMB: 2048, OS: osProfiles["ubuntu2404"], load: 0.12}
	if spec != nil {
		applyVMSpec(g, spec)
	}
	hs.vms[id] = g
	hs.vmOrder = append(hs.vmOrder, id)
	return g
}

func applyVMSpec(g *vm, spec *coreagent.VMSpec) {
	if spec.Name != "" {
		g.Name = spec.Name
	}
	if spec.VCPUs > 0 {
		g.VCPUs = spec.VCPUs
	}
	if spec.MemoryMB > 0 {
		g.MemoryMB = spec.MemoryMB
	}
	if spec.MachineType != "" {
		g.MachineType = spec.MachineType
	}
	if spec.UUID != "" {
		g.UUID = spec.UUID
	}
	if spec.SpiceOptions != nil {
		options := *spec.SpiceOptions
		g.SpiceOptions = &options
	}
	if spec.GuestOSFamily != "" && spec.GuestOSFamily != g.OS.family {
		g.OS = profileForFamily(spec.GuestOSFamily)
	}
	if len(spec.Disks) > 0 {
		disks := make([]*disk, 0, len(spec.Disks))
		for _, d := range spec.Disks {
			disks = append(disks, &disk{ID: d.DiskID, PoolID: d.PoolID, Slot: d.Slot, Bus: d.Bus, Format: d.Format,
				SizeBytes: d.SizeBytes, VolumePath: d.VolumePath, IsBoot: d.IsBoot, IsCDROM: d.IsCDROM})
		}
		g.Disks = disks
	}
	if len(spec.NICs) > 0 {
		nics := make([]*nic, 0, len(spec.NICs))
		for _, n := range spec.NICs {
			nics = append(nics, &nic{ID: n.NICID, NetworkID: n.NetworkID, Bridge: n.Bridge, Slot: n.Slot, MAC: n.MAC, IP: ipFromMAC(n.MAC)})
		}
		g.NICs = nics
	}
}

func profileForFamily(family string) osProfile {
	switch family {
	case "windows":
		return osProfiles["win2022"]
	case "bsd":
		return osProfiles["freebsd14"]
	}
	return osProfiles["ubuntu2404"]
}

// ipFromMAC gives an adopted guest a stable address on the demo NAT range.
func ipFromMAC(mac string) string {
	v := estate.StableHash(mac)
	return fmt.Sprintf("10.250.%d.%d", byte(v>>8)%250, byte(v)%200+20)
}

func vmState(g *vm) coreagent.VMState {
	st := coreagent.VMState{VMID: g.ID, State: g.State, UnitName: "mcharbor-vm-" + g.ID, RunState: g.RunState, LastError: g.LastError}
	switch g.State {
	case stateRunning, statePaused:
		st.UnitActive, st.UnitSub, st.QMPReachable = "active", "running", true
		st.PID = 2000 + int(estate.StableHash(g.ID)%500000)
		st.StartedAt = g.StartedAt.UTC().Format(time.RFC3339)
	case stateCrashed:
		st.UnitActive, st.UnitSub, st.ExitReason = "failed", "failed", "qemu_exited"
	default:
		st.UnitActive, st.UnitSub = "inactive", "dead"
	}
	return st
}

// applyVM performs one vm-family command against hs; callers hold s.mu.
func (s *Sim) applyVM(hs *hostSim, op string, req *coreagent.VMPayload, reply *coreagent.VMPayload) (string, error) {
	now := s.now()
	switch op {
	case coreagent.MsgVMQuery:
		ids := make([]string, 0, len(req.States))
		for _, st := range req.States {
			ids = append(ids, st.VMID)
		}
		if len(ids) == 0 {
			ids = append(ids, hs.vmOrder...)
		}
		for _, id := range ids {
			reply.States = append(reply.States, vmState(s.vmFor(hs, id, nil)))
		}
		return "", nil
	case coreagent.MsgHostDeviceList:
		reply.HostDevices = hostDevices(hs)
		return "", nil
	case coreagent.MsgVMQEMUCapability:
		reply.QEMUCapability = qemuCapability(hs)
		return "", nil
	case coreagent.MsgVMDiskList:
		reply.Volumes = poolVolumes(hs, req.Disk)
		return "", nil
	case coreagent.MsgVMCancel, coreagent.MsgVMThrottleSet:
		return "", nil
	case coreagent.MsgVMImageInspect:
		info := map[string]any{"virtual-size": 80 * gib, "actual-size": 23 * gib, "format": "qcow2", "cluster-size": 65536, "dirty-flag": false, "check-errors": 0}
		raw, err := json.Marshal(info)
		if err != nil {
			return "", err
		}
		reply.ImageInfo = raw
		return "", nil
	}

	if req.VMID == "" {
		return "", errors.New("the command named no guest")
	}
	if op == coreagent.MsgVMDelete {
		if g, ok := hs.vms[req.VMID]; ok {
			delete(hs.vms, g.ID)
			hs.vmOrder = removeID(hs.vmOrder, g.ID)
		}
		return "", nil
	}
	g := s.vmFor(hs, req.VMID, req.Spec)
	switch op {
	case coreagent.MsgVMDefine:
		return "", nil
	case coreagent.MsgVMStart, coreagent.MsgVMShutdown, coreagent.MsgVMStop, coreagent.MsgVMReset,
		coreagent.MsgVMPause, coreagent.MsgVMResume:
		return vmTransition(g, op, now)
	case coreagent.MsgVMStats:
		if g.State != stateRunning && g.State != statePaused {
			return "", errNotRunning
		}
		reply.Stats = vmStats(g, now)
		return "", nil
	case coreagent.MsgVMLog:
		reply.LogLines = vmLog(g, req.LogSource, req.LogTail, now)
		return "", nil
	case coreagent.MsgVMDiskCreate, coreagent.MsgVMDiskResize, coreagent.MsgVMDiskDelete:
		return "", s.applyVMDisk(hs, g, op, req, reply)
	case coreagent.MsgVMSnapshotCreate:
		snap, err := vmSnapshotCreate(g, req.SnapshotID, req.SnapshotName, req.WithMemory, req.Consistency, now)
		if err != nil {
			return "", err
		}
		reply.SnapshotName, reply.Consistency = snap.Handle, snap.Consistency
		if g.OS.guestAgent && g.State == stateRunning && reply.Consistency == "crash" {
			reply.Consistency = "filesystem"
		}
		return "", nil
	case coreagent.MsgVMSnapshotRevert:
		vmSnapshotRevert(g, req.SnapshotID, req.SnapshotName, now)
		return "", nil
	case coreagent.MsgVMSnapshotDelete:
		vmSnapshotDelete(g, req.SnapshotID, req.SnapshotName)
		return "", nil
	case coreagent.MsgVMGuestQuery:
		if g.State != stateRunning {
			return "", errNotRunning
		}
		reply.Guest = guestInfo(g, now)
		return "", nil
	case coreagent.MsgVMGuestAgentDisc:
		reply.MediaPath = "/var/lib/mcharbor/compute/images/mcharbor-guest-agent-" + g.OS.family + ".iso"
		reply.MediaLabel = "McHarbor guest agent"
		return "", nil
	case coreagent.MsgVMMemorySet:
		target := req.TargetMemoryMB
		if target <= 0 {
			target = g.MemoryMB
		}
		reply.Resize = &coreagent.VMResizeResult{RequestedMemoryMB: target, ActualMemoryMB: target, BlockSizeMB: 128, Complied: true}
		g.MemoryMB = target
		return "", nil
	case coreagent.MsgVMDiskHotplug, coreagent.MsgVMMediaChange, coreagent.MsgVMNICHotplug:
		return "", applyHotplug(g, op, req, reply)
	case coreagent.MsgVMCPUSet:
		target := req.TargetVCPUs
		if target <= 0 {
			target = g.VCPUs
		}
		reply.Resize = &coreagent.VMResizeResult{RequestedVCPUs: target, ActualVCPUs: target, Complied: true}
		g.VCPUs = target
		return "", nil
	}
	return "", fmt.Errorf("%s is not simulated", op)
}

func removeID(list []string, id string) []string {
	out := list[:0]
	for _, other := range list {
		if other != id {
			out = append(out, other)
		}
	}
	return out
}

func (s *Sim) applyVMDisk(hs *hostSim, g *vm, op string, req, reply *coreagent.VMPayload) error {
	spec := req.Disk
	if spec == nil {
		return errors.New("the command carried no disk")
	}
	switch op {
	case coreagent.MsgVMDiskCreate:
		p := s.poolByID(hs, spec.PoolID, spec.PoolTarget)
		if p == nil {
			p = &pool{ID: spec.PoolID, Backend: stateOr(spec.PoolBackend, "dir"), Target: stateOr(spec.PoolTarget, "/var/lib/mcharbor/pools/local")}
		}
		format, volume := volumeFor(p, fmt.Sprintf("vm-%s-disk%d", g.ID, spec.Slot))
		if spec.Format != "" && p.Backend == "dir" {
			format = spec.Format
		}
		d := &disk{ID: spec.DiskID, PoolID: p.ID, Slot: spec.Slot, Bus: stateOr(spec.Bus, "virtio-blk"), Format: format,
			SizeBytes: spec.SizeBytes, VolumePath: volume, IsBoot: spec.IsBoot}
		kept := g.Disks[:0]
		for _, other := range g.Disks {
			if other.Slot != d.Slot {
				kept = append(kept, other)
			}
		}
		kept = append(kept, d)
		g.Disks = kept
		out := *spec
		out.VolumePath, out.Format, out.PoolID = volume, format, p.ID
		reply.Disk = &out
	case coreagent.MsgVMDiskResize:
		for _, d := range g.Disks {
			if d.ID == spec.DiskID || (spec.VolumePath != "" && d.VolumePath == spec.VolumePath) || (spec.DiskID == "" && d.Slot == spec.Slot) {
				if spec.SizeBytes < d.SizeBytes {
					return fmt.Errorf("a volume can only grow (%d < %d bytes)", spec.SizeBytes, d.SizeBytes)
				}
				d.SizeBytes = spec.SizeBytes
				out := *spec
				out.VolumePath = d.VolumePath
				reply.Disk = &out
				return nil
			}
		}
		out := *spec
		reply.Disk = &out
	default:
		kept := g.Disks[:0]
		for _, d := range g.Disks {
			if d.ID == spec.DiskID || (spec.VolumePath != "" && d.VolumePath == spec.VolumePath) {
				continue
			}
			kept = append(kept, d)
		}
		g.Disks = kept
	}
	return nil
}

func poolVolumes(hs *hostSim, spec *coreagent.VMDiskSpec) []coreagent.VMVolumeInfo {
	var poolID string
	if spec != nil {
		poolID = spec.PoolID
	}
	out := []coreagent.VMVolumeInfo{}
	for _, id := range hs.vmOrder {
		for _, d := range hs.vms[id].Disks {
			if poolID == "" || d.PoolID == poolID {
				out = append(out, coreagent.VMVolumeInfo{Name: path.Base(d.VolumePath), VolumePath: d.VolumePath, Format: d.Format, SizeBytes: d.SizeBytes, IsBlock: d.Format == "raw"})
			}
		}
	}
	for _, p := range hs.pools {
		if p.Backend == "dir" && (poolID == "" || p.ID == poolID) {
			orphan := strings.TrimRight(p.Target, "/") + "/old-import-disk0.qcow2"
			out = append(out, coreagent.VMVolumeInfo{Name: path.Base(orphan), VolumePath: orphan, Format: "qcow2", SizeBytes: 40 * gib})
			break
		}
	}
	return out
}

// wave is a smooth, deterministic 0..1 signal so a stats graph drifts like a
// real guest instead of jumping between random samples.
func wave(seed string, now time.Time, period float64) float64 {
	phase := float64(estate.StableHash(seed)%1000) / 1000 * 2 * math.Pi
	t := float64(now.Unix())
	return 0.5 + 0.35*math.Sin(t/period+phase) + 0.15*math.Sin(t/(period/3.7)+phase*1.3)
}

func vmStats(g *vm, now time.Time) *coreagent.VMStats {
	uptime := now.Sub(g.StartedAt).Seconds()
	if uptime < 1 {
		uptime = 1
	}
	cpu := g.load * 100 * (0.6 + 0.8*wave(g.ID+"cpu", now, 90))
	if g.State == statePaused {
		cpu = 0
	}
	mem := g.MemoryMB << 20
	used := int64(float64(mem) * (0.35 + 0.4*wave(g.ID+"mem", now, 600)))
	return &coreagent.VMStats{
		VMID: g.ID, CPUPercent: math.Round(cpu*10) / 10, CPUUsageNSec: int64(uptime * float64(g.VCPUs) * g.load * 1e9),
		MemoryBytes: used, MemoryMaxBytes: mem, BalloonBytes: mem,
		DiskReadBytes: int64(uptime * 180000 * g.load), DiskWriteBytes: int64(uptime * 95000 * g.load),
		NetRxBytes: int64(uptime * 52000 * g.load), NetTxBytes: int64(uptime * 31000 * g.load),
		SampledAt: now.Format(time.RFC3339),
	}
}

func guestInfo(g *vm, now time.Time) *coreagent.VMGuestInfo {
	if !g.OS.guestAgent {
		return &coreagent.VMGuestInfo{VMID: g.ID, AgentState: "absent"}
	}
	info := &coreagent.VMGuestInfo{VMID: g.ID, AgentState: "connected", Hostname: g.Name, OSID: g.OS.osID, OSName: g.OS.name, OSVersion: g.OS.version}
	for _, n := range g.NICs {
		if n.IP != "" {
			info.IPs = append(info.IPs, n.IP)
		}
	}
	total := int64(40) * gib
	if len(g.Disks) > 0 {
		total = g.Disks[0].SizeBytes
	}
	root := "/"
	fsType := "ext4"
	if g.OS.family == "windows" {
		root, fsType = "C:\\", "NTFS"
	} else if g.OS.osID == "rocky" {
		fsType = "xfs"
	}
	info.Filesystems = []coreagent.VMGuestFilesystem{{Mountpoint: root, Type: fsType, TotalBytes: total, UsedBytes: int64(float64(total) * (0.3 + 0.3*wave(g.ID+"fs", now, 86400)))}}
	if len(g.Disks) > 1 && g.OS.family != "windows" {
		info.Filesystems = append(info.Filesystems, coreagent.VMGuestFilesystem{Mountpoint: "/data", Type: fsType, TotalBytes: g.Disks[1].SizeBytes, UsedBytes: g.Disks[1].SizeBytes / 100 * 58})
	}
	l1 := math.Round(float64(g.VCPUs)*g.load*(0.5+wave(g.ID+"load", now, 120))*100) / 100
	l5, l15 := math.Round(l1*90)/100, math.Round(l1*80)/100
	skew := int64(estate.StableHash(g.ID)%40) - 20
	info.Load1, info.Load5, info.Load15, info.ClockSkewMs = &l1, &l5, &l15, &skew
	return info
}

func vmLog(g *vm, source string, tail int, now time.Time) []string {
	if tail <= 0 || tail > 200 {
		tail = 60
	}
	if source == "tpm" {
		return []string{now.Add(-2*time.Hour).Format(time.RFC3339) + " swtpm: TPM 2.0 state loaded (demo)"}
	}
	lines := []string{
		"Starting QEMU for %s (demo)", "qemu-system-x86_64: -machine %s: KVM acceleration enabled",
		"guest %s: virtio-net link up", "QMP: connected, capabilities negotiated",
		"guest agent: channel org.qemu.guest_agent.0 connected", "block: virtio-disk0 discard enabled",
		"balloon: target reached", "QMP event RTC_CHANGE", "guest agent: fsfreeze-status thawed",
		"monitor: query-status running",
	}
	out := make([]string, 0, tail)
	start := g.StartedAt
	if start.IsZero() {
		start = now.Add(-6 * time.Hour)
	}
	for i := 0; i < tail; i++ {
		at := start.Add(time.Duration(i) * 97 * time.Second).UTC().Format(time.RFC3339)
		line := lines[i%len(lines)]
		if strings.Contains(line, "%s") {
			arg := g.Name
			if strings.Contains(line, "-machine") {
				arg = g.MachineType
			}
			line = fmt.Sprintf(line, arg)
		}
		out = append(out, at+" "+line)
	}
	return out
}

func hostDevices(hs *hostSim) []coreagent.HostDevice {
	devices := []coreagent.HostDevice{
		{Kind: "pci", Address: "0000:3b:00.0", Description: "Intel Corporation Ethernet Controller X710 for 10GbE SFP+", Vendor: "8086", Product: "1572", Driver: "i40e", Class: "0200", IOMMUGroup: "31", Assignable: false, Reason: "bound to i40e, which carries the host's uplink"},
		{Kind: "pci", Address: "0000:3b:02.0", Description: "Intel Corporation Ethernet Virtual Function 700 Series", Vendor: "8086", Product: "154c", Driver: "vfio-pci", Class: "0200", IOMMUGroup: "44", Assignable: true},
		{Kind: "usb", Address: "1050:0407", Description: "Yubico YubiKey OTP+FIDO+CCID", Vendor: "1050", Product: "0407", Assignable: true},
	}
	if hs.host.Key == "homelab" || hs.host.Key == "data-platform" {
		devices = append(devices,
			coreagent.HostDevice{Kind: "pci", Address: "0000:01:00.0", Description: "NVIDIA Corporation GA104GL [RTX A4000]", Vendor: "10de", Product: "24b0", Driver: "vfio-pci", Class: "0300", IOMMUGroup: "16", Assignable: true},
			coreagent.HostDevice{Kind: "pci", Address: "0000:01:00.1", Description: "NVIDIA Corporation GA104 High Definition Audio Controller", Vendor: "10de", Product: "228b", Driver: "vfio-pci", Class: "0403", IOMMUGroup: "16", Assignable: true},
			coreagent.HostDevice{Kind: "usb", Address: "10c4:ea60", Description: "Silicon Labs CP210x UART Bridge (Zigbee dongle)", Vendor: "10c4", Product: "ea60", Assignable: true},
		)
	}
	return devices
}

func qemuCapability(hs *hostSim) *coreagent.VMQEMUCapability {
	c := capabilityFor(hs.host, hs, time.Now())
	b := c.QEMUBinaries[0]
	features := map[string]bool{}
	for _, f := range qemuFeatures {
		features[f] = true
	}
	return &coreagent.VMQEMUCapability{Binary: b.Path, Arch: b.Arch, Version: b.Version, MachineTypes: b.MachineTypes,
		Devices: b.Devices, CPUModels: b.CPUModels, QMPCommands: b.QMPCommands, Features: features,
		HugePages: c.HugePages, NUMANodes: c.NUMANodes}
}

// startVMClone copies a guest with a few progress frames, off the read loop.
func (s *Sim) startVMClone(ctx context.Context, host *estate.Host, msg *coreagent.WSMessage, req *coreagent.VMPayload, send fakeagent.Sender) {
	go func() {
		reply := &coreagent.VMPayload{Operation: msg.Type, EnvID: req.EnvID, VMID: req.VMID}
		out := &coreagent.WSMessage{Type: coreagent.MsgVMResult, ID: msg.ID, VM: reply}
		if req.Clone == nil || req.Clone.TargetVMID == "" {
			reply.Error = "the clone request named no target guest"
			s.deliver(send, out)
			return
		}
		for pct := 25; pct < 100; pct += 25 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(120 * time.Millisecond):
			}
			s.deliver(send, &coreagent.WSMessage{Type: coreagent.MsgVMCloneProgress, ID: msg.ID, VM: &coreagent.VMPayload{Operation: "progress", VMID: req.Clone.TargetVMID, Percent: pct}})
		}
		s.mu.Lock()
		hs := s.hostFor(host)
		if hs == nil {
			s.mu.Unlock()
			reply.Error = "this demo host runs no guests"
			s.deliver(send, out)
			return
		}
		source := s.vmFor(hs, req.VMID, req.Spec)
		clone := *source
		clone.ID, clone.UUID, clone.State, clone.RunState, clone.LastError = req.Clone.TargetVMID, stableUUID(req.Clone.TargetVMID), stateStopped, "", ""
		clone.Snapshots, clone.Current, clone.StartedAt, clone.CreatedAt, clone.IsTemplate = nil, "", time.Time{}, s.now(), false
		clone.Name = source.Name + "-clone"
		clone.Disks = nil
		for _, d := range source.Disks {
			copied := *d
			copied.VolumePath = strings.Replace(d.VolumePath, source.ID, clone.ID, 1)
			clone.Disks = append(clone.Disks, &copied)
			reply.Disks = append(reply.Disks, coreagent.VMDiskSpec{DiskID: d.ID, PoolID: d.PoolID, Slot: d.Slot, VolumePath: copied.VolumePath, Format: d.Format, SizeBytes: d.SizeBytes})
		}
		clone.NICs = nil
		if req.Clone.Spec != nil {
			applyVMSpec(&clone, req.Clone.Spec)
		}
		hs.vms[clone.ID] = &clone
		hs.vmOrder = append(hs.vmOrder, clone.ID)
		s.mu.Unlock()
		reply.Success = true
		s.deliver(send, out)
	}()
}

// applyHotplug answers the three live hardware changes the way a host with
// hot-plug ports does: a running guest takes the change, the boot disk is
// refused with the agent's own code, and a stopped guest is not a target.
func applyHotplug(g *vm, op string, req, reply *coreagent.VMPayload) error {
	change := req.Hotplug
	if change == nil {
		return errors.New("the command carried no change")
	}
	if g.State != stateRunning {
		return errNotRunning
	}
	switch op {
	case coreagent.MsgVMDiskHotplug:
		spec := change.Disk
		if spec == nil {
			return errors.New("the command carried no disk")
		}
		if spec.IsBoot && change.Action != "attach" {
			reply.ErrorCode = "hotplug_boot_disk"
			return nil
		}
		switch change.Action {
		case "attach":
			for _, d := range g.Disks {
				if d.Slot == spec.Slot {
					return nil
				}
			}
			g.Disks = append(g.Disks, &disk{ID: spec.DiskID, PoolID: spec.PoolID, Slot: spec.Slot,
				Bus: stateOr(spec.Bus, "virtio-blk"), Format: stateOr(spec.Format, "qcow2"),
				SizeBytes: spec.SizeBytes, VolumePath: spec.VolumePath})
		case "detach":
			kept := g.Disks[:0]
			for _, d := range g.Disks {
				if d.Slot != spec.Slot {
					kept = append(kept, d)
				}
			}
			g.Disks = kept
		case "resize":
			for _, d := range g.Disks {
				if d.Slot == spec.Slot {
					d.SizeBytes = spec.SizeBytes
				}
			}
			out := *spec
			reply.Disk = &out
		default:
			return fmt.Errorf("%q is not a disk hot-plug action", change.Action)
		}
	case coreagent.MsgVMNICHotplug:
		if change.NIC == nil {
			return errors.New("the command carried no NIC")
		}
		switch change.Action {
		case "attach":
			for _, n := range g.NICs {
				if n.Slot == change.NIC.Slot {
					return nil
				}
			}
			g.NICs = append(g.NICs, &nic{ID: change.NIC.NICID, NetworkID: change.NIC.NetworkID,
				Bridge: change.NIC.Bridge, Slot: change.NIC.Slot, MAC: change.NIC.MAC})
		case "detach":
			kept := g.NICs[:0]
			for _, n := range g.NICs {
				if n.Slot != change.NIC.Slot {
					kept = append(kept, n)
				}
			}
			g.NICs = kept
		case "link":
		default:
			return fmt.Errorf("%q is not a NIC action", change.Action)
		}
	default:
		if change.Disk == nil {
			return errors.New("the command carried no drive")
		}
		if change.Action != "insert" && change.Action != "eject" {
			return fmt.Errorf("%q is not a media action", change.Action)
		}
	}
	return nil
}
