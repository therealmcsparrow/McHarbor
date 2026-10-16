// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package computesim

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/demo/fakeagent"
)

const demoAgentVersion = "2026.10.01"

var machineTypes = []string{"pc-q35-8.2", "pc-q35-8.1", "pc-q35-8.0", "pc-q35-7.2", "q35", "pc-i440fx-8.2", "pc"}

var qemuFeatures = []string{
	coreagent.VMFeatureVirtioMem, coreagent.VMFeatureCPUHotplug, coreagent.VMFeatureHugePages,
	coreagent.VMFeatureNUMA, coreagent.VMFeatureVirtioFS, coreagent.VMFeatureVSock,
	coreagent.VMFeatureIOThrottle, coreagent.VMFeatureIOThread, coreagent.VMFeatureDirtyBitmap,
	coreagent.VMFeatureBlockdevBkp, coreagent.VMFeatureVirtioGPU, coreagent.VMFeatureVirtioSnd,
	coreagent.VMFeatureUSBU2F, coreagent.VMFeatureClone, coreagent.VMFeatureHotplug,
}

// capabilityFor is the probe report for a host. A host that runs no guests
// still answers, with the reason, the way a real host without KVM does.
func capabilityFor(h *estate.Host, hs *hostSim, at time.Time) *coreagent.ComputeCapability {
	c := &coreagent.ComputeCapability{
		OS: "linux", Arch: h.Arch, KernelVersion: h.Kernel, AgentVersion: demoAgentVersion,
		HostCPUs: h.CPUs, HostMemoryBytes: h.MemTotal, HostSockets: 2, HostPhysicalCores: h.CPUs / 2,
		ProbedAt: at.UTC().Format(time.RFC3339),
	}
	if c.HostSockets > c.HostPhysicalCores {
		c.HostSockets = 1
	}
	if hs == nil {
		c.UnsupportedWhy = "This demo host has no hypervisor; guests run on the compute hosts (demo)."
		c.Items = []coreagent.ComputeCapabilityItem{{Key: "kvm_device", Present: false, Detail: "/dev/kvm is absent on this demo host", Critical: true, Blocks: "both"}}
		return c
	}
	c.Supported, c.CanRunVMs, c.CanRunContainers, c.SeccompBuiltIn = true, true, true, true
	c.SupervisorMode = "systemd"
	for _, key := range []string{"kvm_device", "cgroup2", "userns", "subuid", "systemd", "dbus", "qemu_img", "ovmf", "swtpm", "idmapped_mounts", "iommu", "seccomp"} {
		c.Items = append(c.Items, coreagent.ComputeCapabilityItem{Key: key, Present: true, Detail: "present (demo)"})
	}
	arch := "x86_64"
	if h.Arch == "aarch64" {
		arch = "aarch64"
	}
	c.QEMUBinaries = []coreagent.ComputeQEMUBinary{{
		Path: "/usr/bin/qemu-system-" + arch, Arch: arch, Version: "QEMU emulator version 8.2.2 (Debian 1:8.2.2+ds-0ubuntu1)",
		MachineTypes: machineTypes, KVMAccel: true,
		Devices:     []string{"virtio-blk-pci", "virtio-scsi-pci", "virtio-net-pci", "e1000e", "virtio-gpu-pci", "virtio-mem-pci", "vhost-user-fs-pci", "vhost-vsock-pci", "qxl-vga", "tpm-crb"},
		CPUModels:   []string{"host", "max", "EPYC-v4", "Icelake-Server-v6", "Skylake-Server-v5", "x86-64-v3"},
		QMPCommands: []string{"query-status", "system_powerdown", "quit", "stop", "cont", "system_reset", "blockdev-snapshot-sync", "block-set-io-throttle"},
	}}
	half := int64(h.CPUs / 2)
	c.HugePages = &coreagent.ComputeHugePages{Total: 4096, Free: 2048, SizeKB: 2048}
	c.NUMANodes = []coreagent.ComputeNUMANode{
		{ID: 0, CPUs: cpuRange(0, int(half)), MemoryBytes: h.MemTotal / 2, FreeBytes: h.MemTotal / 5},
		{ID: 1, CPUs: cpuRange(int(half), h.CPUs), MemoryBytes: h.MemTotal / 2, FreeBytes: h.MemTotal / 6},
	}
	c.Bridges = hostLinks(hs, true)
	c.SubuidRanges = []coreagent.ComputeIDRange{{Start: 100000, Count: 65536 * 1000}}
	c.SubgidRanges = []coreagent.ComputeIDRange{{Start: 100000, Count: 65536 * 1000}}
	c.StorageDrivers = []coreagent.SnapshotDriverOut{
		{Driver: "zfs", ToolPath: "/usr/sbin/zfs", Version: "zfs-2.2.2", Available: true},
		{Driver: "lvm", ToolPath: "/usr/sbin/lvm", Version: "2.03.16", Available: true},
	}
	return c
}

func cpuRange(from, to int) []int {
	out := make([]int, 0, to-from)
	for i := from; i < to; i++ {
		out = append(out, i)
	}
	return out
}

// hostLinks is the link inventory: the uplinks, br0 and every managed bridge.
func hostLinks(hs *hostSim, bridgesOnly bool) []coreagent.ComputeHostLink {
	n := estate.StableHash(hs.host.Key)
	mac := func(i int) string {
		return fmt.Sprintf("3c:ec:ef:%02x:%02x:%02x", byte(n>>8), byte(n>>16), byte(i))
	}
	links := []coreagent.ComputeHostLink{
		{Name: "br0", Kind: "bridge", MAC: mac(1), MTU: 1500, Up: true, Addresses: []string{fmt.Sprintf("10.%d.0.%d/16", 40+hs.index, 10+hs.index)}, HasDefaultRoute: true},
	}
	for _, net := range hs.networks {
		if net.Mode == "managed" {
			links = append(links, coreagent.ComputeHostLink{Name: net.Bridge, Kind: "bridge", MAC: mac(10 + len(links)), MTU: 1500, Up: true, Addresses: []string{net.Gateway + "/24"}, Managed: true})
		}
	}
	if bridgesOnly {
		return links
	}
	return append(links,
		coreagent.ComputeHostLink{Name: "eno1", Kind: "physical", MAC: mac(2), MTU: 1500, Up: true, Master: "bond0"},
		coreagent.ComputeHostLink{Name: "eno2", Kind: "physical", MAC: mac(3), MTU: 1500, Up: true, Master: "bond0"},
		coreagent.ComputeHostLink{Name: "bond0", Kind: "bond", MAC: mac(2), MTU: 1500, Up: true, Master: "br0", Upper: []string{"br0"}},
		coreagent.ComputeHostLink{Name: "enp65s0f0", Kind: "physical", MAC: mac(4), MTU: 9000, Up: false},
	)
}

func (s *Sim) handleHostFamily(ctx context.Context, host *estate.Host, msg *coreagent.WSMessage, send fakeagent.Sender) {
	switch msg.Type {
	case coreagent.MsgComputeProbeRequest, coreagent.MsgComputeConfigRequest, coreagent.MsgComputeMigrateRequest:
		s.deliver(send, s.computeReply(host, msg))
	case coreagent.MsgPoolDetectRequest, coreagent.MsgPoolCreateRequest, coreagent.MsgPoolStatsRequest,
		coreagent.MsgPoolRemoveRequest, coreagent.MsgPoolBrowseRequest, coreagent.MsgPoolFileRequest:
		s.deliver(send, s.poolReply(host, msg))
	case coreagent.MsgNetDetectRequest, coreagent.MsgNetEnsureRequest, coreagent.MsgNetRemoveRequest, coreagent.MsgNetAdoptConfirm:
		s.deliver(send, s.netReply(host, msg))
	case coreagent.MsgImageFetchRequest:
		s.startImageFetch(ctx, host, msg, send)
	default:
		s.deliver(send, s.imageReply(host, msg))
	}
}

func (s *Sim) computeReply(host *estate.Host, msg *coreagent.WSMessage) *coreagent.WSMessage {
	req := msg.Compute
	if req == nil {
		req = &coreagent.ComputePayload{}
	}
	reply := &coreagent.ComputePayload{Operation: req.Operation, EnvID: req.EnvID}
	out := &coreagent.WSMessage{ID: msg.ID, Compute: reply}
	s.mu.Lock()
	defer s.mu.Unlock()
	hs := s.hostFor(host)
	switch msg.Type {
	case coreagent.MsgComputeProbeRequest:
		out.Type = coreagent.MsgComputeProbeResponse
		reply.Operation = "probe"
		if host == nil {
			reply.Error = "unknown demo host"
			return out
		}
		reply.Success = true
		reply.Capability = capabilityFor(host, hs, s.now())
	case coreagent.MsgComputeConfigRequest:
		out.Type = coreagent.MsgComputeConfigResponse
		roots := coreagent.ComputeRoots{VMRoot: "/var/lib/mcharbor/compute/vm", CTRoot: "/var/lib/mcharbor/compute/ct", ImageRoot: "/var/lib/mcharbor/compute/images"}
		if req.Roots != nil {
			if req.Roots.VMRoot != "" {
				roots.VMRoot = req.Roots.VMRoot
			}
			if req.Roots.CTRoot != "" {
				roots.CTRoot = req.Roots.CTRoot
			}
			if req.Roots.ImageRoot != "" {
				roots.ImageRoot = req.Roots.ImageRoot
			}
		}
		reply.Roots = &roots
		reply.Success = true
	default:
		out.Type = coreagent.MsgComputeMigrateResponse
		migrate := &coreagent.ComputeMigrate{}
		if req.Migrate != nil {
			migrate.Plan, migrate.Repair, migrate.Roots = req.Migrate.Plan, req.Migrate.Repair, req.Migrate.Roots
		}
		reply.Migrate = migrate
		if migrate.Plan || migrate.Repair {
			migrate.Notes = []string{"Demo host: nothing to move."}
			migrate.Completed = migrate.Repair
			reply.Success = true
			return out
		}
		reply.Error = ErrDemoRefused
	}
	return out
}

func (s *Sim) poolByID(hs *hostSim, id, target string) *pool {
	for _, p := range hs.pools {
		if (id != "" && p.ID == id) || (id == "" && target != "" && p.Target == target) {
			return p
		}
	}
	return nil
}

func statsPayload(hs *hostSim, p *pool, at time.Time) *coreagent.ComputePoolStats {
	capacity, used, allocated, volumes := poolStats(hs, p)
	return &coreagent.ComputePoolStats{
		CapacityBytes: capacity, UsedBytes: used, FreeBytes: capacity - used,
		AllocatedBytes: allocated, VolumeCount: volumes, SampledAt: at.UTC().Format(time.RFC3339),
	}
}

func (s *Sim) poolReply(host *estate.Host, msg *coreagent.WSMessage) *coreagent.WSMessage {
	req := msg.Pool
	if req == nil {
		req = &coreagent.ComputePoolPayload{}
	}
	reply := &coreagent.ComputePoolPayload{Operation: req.Operation, EnvID: req.EnvID, PoolID: req.PoolID}
	out := &coreagent.WSMessage{Type: poolReplyType(msg.Type), ID: msg.ID, Pool: reply}
	s.mu.Lock()
	defer s.mu.Unlock()
	hs := s.hostFor(host)
	if hs == nil {
		reply.Error = "this demo host runs no guests"
		return out
	}
	now := s.now()
	switch msg.Type {
	case coreagent.MsgPoolDetectRequest:
		free := hs.host.DiskTotal - hs.host.DiskUsed
		reply.Detected = []coreagent.ComputePoolBackend{
			{Backend: "dir", Available: true, Detail: "any directory on a local filesystem", Candidates: []coreagent.ComputePoolCandidate{{Target: "/var/lib/mcharbor/pools/local", Mountpoint: "/", CapacityBytes: hs.host.DiskTotal, FreeBytes: free}}},
			{Backend: "zfs", Available: true, Instant: true, Detail: "zfs-2.2.2", Candidates: []coreagent.ComputePoolCandidate{{Target: "tank/vms", Mountpoint: "/tank/vms", CapacityBytes: hs.host.DiskTotal / 2, FreeBytes: hs.host.DiskTotal / 3}}},
			{Backend: "lvm", Available: true, Instant: true, Detail: "LVM thin pools", Candidates: []coreagent.ComputePoolCandidate{{Target: "vg_nvme/thinpool", CapacityBytes: 7680 * gib, FreeBytes: 5000 * gib}}},
			{Backend: "btrfs", Available: false, Detail: "no btrfs filesystem is mounted", Remediation: "mkfs.btrfs /dev/sdX && mount /dev/sdX /srv/pool"},
		}
		reply.Success = true
	case coreagent.MsgPoolCreateRequest:
		p := s.poolByID(hs, req.PoolID, req.Target)
		if p == nil {
			p = &pool{ID: req.PoolID, Name: req.Name, Backend: req.Backend, Target: req.Target, Capacity: 4 * tib, baseUsed: 40 * gib, CreatedAt: now}
			hs.pools = append(hs.pools, p)
		}
		reply.Stats = statsPayload(hs, p, now)
		reply.Success = true
	case coreagent.MsgPoolStatsRequest:
		p := s.poolByID(hs, req.PoolID, req.Target)
		if p == nil {
			reply.Error = fmt.Sprintf("pool %q is not present on this demo host", req.Name)
			return out
		}
		reply.Stats = statsPayload(hs, p, now)
		reply.Success = true
	case coreagent.MsgPoolRemoveRequest:
		p := s.poolByID(hs, req.PoolID, req.Target)
		if p != nil {
			if _, _, _, volumes := poolStats(hs, p); volumes > 0 && !req.Force {
				reply.Error = fmt.Sprintf("pool %q still holds %d guest volumes", p.Name, volumes)
				return out
			}
			kept := hs.pools[:0]
			for _, other := range hs.pools {
				if other != p {
					kept = append(kept, other)
				}
			}
			hs.pools = kept
		}
		reply.Success = true
	case coreagent.MsgPoolBrowseRequest:
		p := s.poolByID(hs, req.PoolID, req.Target)
		if p == nil {
			reply.Error = "pool is not present on this demo host"
			return out
		}
		reply.Entries = browsePool(hs, p, strings.Trim(req.Path, "/"), now)
		reply.Success = true
	default:
		if req.Operation == "read" {
			reply.Content = []byte("# McHarbor demo mode\n# This file is simulated; nothing on a real host was read.\n")
			reply.Success = true
			return out
		}
		reply.Error = ErrDemoRefused
	}
	return out
}

func poolReplyType(t string) string {
	switch t {
	case coreagent.MsgPoolCreateRequest:
		return coreagent.MsgPoolCreateResponse
	case coreagent.MsgPoolStatsRequest:
		return coreagent.MsgPoolStatsResponse
	case coreagent.MsgPoolRemoveRequest:
		return coreagent.MsgPoolRemoveResponse
	case coreagent.MsgPoolBrowseRequest:
		return coreagent.MsgPoolBrowseResponse
	case coreagent.MsgPoolFileRequest:
		return coreagent.MsgPoolFileResponse
	}
	return coreagent.MsgPoolDetectResponse
}

// browsePool lists the volumes a pool holds at its root, and a small ISO shelf.
func browsePool(hs *hostSim, p *pool, dir string, now time.Time) []coreagent.ComputePoolFile {
	stamp := now.Add(-26 * time.Hour).UTC().Format(time.RFC3339)
	if dir == "iso" {
		return []coreagent.ComputePoolFile{
			{Name: "ubuntu-24.04.1-live-server-amd64.iso", Path: "iso/ubuntu-24.04.1-live-server-amd64.iso", SizeBytes: 2773 << 20, Mode: "0644", ModifiedAt: stamp},
			{Name: "SERVER_EVAL_x64FRE_en-us.iso", Path: "iso/SERVER_EVAL_x64FRE_en-us.iso", SizeBytes: 4940 << 20, Mode: "0644", ModifiedAt: stamp},
			{Name: "virtio-win-0.1.262.iso", Path: "iso/virtio-win-0.1.262.iso", SizeBytes: 708 << 20, Mode: "0644", ModifiedAt: stamp},
		}
	}
	if dir != "" {
		return []coreagent.ComputePoolFile{}
	}
	out := []coreagent.ComputePoolFile{{Name: "iso", Path: "iso", IsDir: true, Mode: "0755", ModifiedAt: stamp}}
	for _, id := range hs.vmOrder {
		for _, d := range hs.vms[id].Disks {
			if d.PoolID == p.ID {
				name := path.Base(d.VolumePath)
				out = append(out, coreagent.ComputePoolFile{Name: name, Path: name, SizeBytes: d.SizeBytes, Mode: "0600", ModifiedAt: stamp})
			}
		}
	}
	return out
}

func (s *Sim) netReply(host *estate.Host, msg *coreagent.WSMessage) *coreagent.WSMessage {
	req := msg.Net
	if req == nil {
		req = &coreagent.ComputeNetPayload{}
	}
	reply := &coreagent.ComputeNetPayload{Operation: req.Operation, EnvID: req.EnvID, NetworkID: req.NetworkID}
	out := &coreagent.WSMessage{ID: msg.ID, Net: reply}
	switch msg.Type {
	case coreagent.MsgNetEnsureRequest:
		out.Type = coreagent.MsgNetEnsureResponse
	case coreagent.MsgNetRemoveRequest:
		out.Type = coreagent.MsgNetRemoveResponse
	case coreagent.MsgNetAdoptConfirm:
		out.Type = coreagent.MsgNetAdoptConfirmed
	default:
		out.Type = coreagent.MsgNetDetectResponse
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	hs := s.hostFor(host)
	if hs == nil {
		reply.Error = "this demo host runs no guests"
		return out
	}
	switch msg.Type {
	case coreagent.MsgNetDetectRequest:
		reply.Links = hostLinks(hs, false)
	case coreagent.MsgNetEnsureRequest:
		if !s.hasNetwork(hs, req.NetworkID) {
			prefix := strings.TrimSuffix(req.GatewayIP, "1")
			hs.networks = append(hs.networks, &network{
				ID: req.NetworkID, Name: req.Name, Mode: req.Mode, Bridge: req.Bridge, VLAN: req.VLANTag,
				Subnet: req.SubnetCIDR, Gateway: req.GatewayIP, DHCP: req.DHCPEnabled,
				RangeStart: req.DHCPRangeStart, RangeEnd: req.DHCPRangeEnd, NAT: req.NATEnabled,
				Uplink: req.UplinkLink, CreatedAt: s.now(), prefix: prefix, nextHost: 100,
			})
		}
	case coreagent.MsgNetRemoveRequest:
		kept := hs.networks[:0]
		for _, n := range hs.networks {
			if n.ID != req.NetworkID {
				kept = append(kept, n)
			}
		}
		hs.networks = kept
	}
	reply.Success = true
	return out
}

func (s *Sim) hasNetwork(hs *hostSim, id string) bool {
	for _, n := range hs.networks {
		if n.ID == id {
			return true
		}
	}
	return false
}

func (s *Sim) imageReply(host *estate.Host, msg *coreagent.WSMessage) *coreagent.WSMessage {
	req := msg.ComputeImage
	if req == nil {
		req = &coreagent.ComputeImagePayload{}
	}
	reply := &coreagent.ComputeImagePayload{Operation: req.Operation, EnvID: req.EnvID, ImageID: req.ImageID}
	out := &coreagent.WSMessage{Type: coreagent.MsgImageRemoveResult, ID: msg.ID, ComputeImage: reply}
	s.mu.Lock()
	defer s.mu.Unlock()
	if msg.Type == coreagent.MsgImageCancelRequest {
		if cancel, ok := s.fetches[msg.ID]; ok {
			cancel()
			delete(s.fetches, msg.ID)
			reply.Success = true
		}
		return out
	}
	hs := s.hostFor(host)
	if hs == nil {
		if msg.Type == coreagent.MsgImageListRequest {
			out.Type = coreagent.MsgImageListResponse
		}
		reply.Error = "this demo host runs no guests"
		return out
	}
	if msg.Type == coreagent.MsgImageListRequest {
		out.Type = coreagent.MsgImageListResponse
		for _, img := range hs.images {
			reply.Cached = append(reply.Cached, coreagent.ComputeCachedImage{
				ImageID: img.ImageID, Kind: img.Kind, LocalPath: img.LocalPath, SizeBytes: img.SizeBytes,
				Verified: true, InUse: img.Kind == "cloud_image", LastUsedAt: img.LastUsed.UTC().Format(time.RFC3339),
			})
		}
		reply.Success = true
		return out
	}
	kept := hs.images[:0]
	for _, img := range hs.images {
		if img.ImageID != req.ImageID {
			kept = append(kept, img)
		}
	}
	hs.images = kept
	reply.Success = true
	return out
}

// startImageFetch plays a short download: a handful of progress frames and a
// verified result, on its own goroutine so the read loop keeps serving.
func (s *Sim) startImageFetch(ctx context.Context, host *estate.Host, msg *coreagent.WSMessage, send fakeagent.Sender) {
	req := msg.ComputeImage
	if req == nil {
		req = &coreagent.ComputeImagePayload{}
	}
	fetchCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	hs := s.hostFor(host)
	s.fetches[msg.ID] = cancel
	s.mu.Unlock()
	total := int64(612 << 20)
	if req.Kind == "rootfs" {
		total = 29 << 20
	}
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.fetches, msg.ID)
			s.mu.Unlock()
			cancel()
		}()
		result := &coreagent.ComputeImagePayload{Operation: "fetch", EnvID: req.EnvID, ImageID: req.ImageID}
		if hs == nil {
			result.Error = "this demo host runs no guests"
			s.deliver(send, &coreagent.WSMessage{Type: coreagent.MsgImageFetchResult, ID: msg.ID, ComputeImage: result})
			return
		}
		for i, phase := range []string{"downloading", "downloading", "downloading", "verifying", "finalizing"} {
			select {
			case <-fetchCtx.Done():
				return
			case <-time.After(150 * time.Millisecond):
			}
			pct := (i + 1) * 20
			s.deliver(send, &coreagent.WSMessage{Type: coreagent.MsgImageFetchProgress, ID: msg.ID, ComputeImage: &coreagent.ComputeImagePayload{
				Operation: "progress", ImageID: req.ImageID, BytesDone: total / 100 * int64(pct), BytesTotal: total, Percent: pct, Phase: phase,
			}})
		}
		ext := ".qcow2"
		if req.Kind == "rootfs" {
			ext = ".tar.gz"
		}
		result.LocalPath = "/var/lib/mcharbor/compute/images/" + req.ImageID + ext
		result.SizeBytes, result.Verified, result.Success = total, true, true
		result.Checksum = fmt.Sprintf("%064x", estate.StableHash(req.ImageID))[:64]
		s.mu.Lock()
		hs.images = append(hs.images, cachedImage{ImageID: req.ImageID, Kind: req.Kind, LocalPath: result.LocalPath, SizeBytes: total, LastUsed: s.now()})
		s.mu.Unlock()
		s.deliver(send, &coreagent.WSMessage{Type: coreagent.MsgImageFetchResult, ID: msg.ID, ComputeImage: result})
	}()
}
