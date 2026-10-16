// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/capability"
	"github.com/therealmcsparrow/mcharbor-agent/compute/cloudinit"
	"github.com/therealmcsparrow/mcharbor-agent/compute/dhcp"
	"github.com/therealmcsparrow/mcharbor-agent/compute/guestdisc"
	"github.com/therealmcsparrow/mcharbor-agent/compute/hostdev"
	"github.com/therealmcsparrow/mcharbor-agent/compute/imagefetch"
	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
	"github.com/therealmcsparrow/mcharbor-agent/compute/netconf"
	"github.com/therealmcsparrow/mcharbor-agent/compute/storage"
	"github.com/therealmcsparrow/mcharbor-agent/compute/supervisor"
	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Per-family timeouts.
//
// One budget for everything would be wrong in both directions: a capability
// probe that took 30 minutes would hide a broken host, and a 500 GB disk
// allocation that was cut off at 60 seconds would fail every large VM.
// guestDiscLabel names the guest-agent disc in the UI's media list, where a
// cache path would tell the operator nothing.
const guestDiscLabel = "McHarbor guest agent"

// poolInUseCode tells the server a pool removal was refused because guest volumes
// remain on it, so it can say so without reading the sentence.
const poolInUseCode = "pool_in_use"

// guestDiscPendingCode tells the server the disc is still being prepared,
// which is a "try again shortly", not a failure.
const guestDiscPendingCode = "guest_disc_pending"

// guestDiscPrepareTimeout bounds the whole prepare pass, whose slow step is
// the one-off virtio download for the Windows disc.
const guestDiscPrepareTimeout = 30 * time.Minute

const (
	computeProbeTimeout  = 2 * time.Minute
	computeQuickTimeout  = 60 * time.Second
	computeMediumTimeout = 15 * time.Minute
	computeLongTimeout   = 6 * time.Hour
)

// handleComputeMessage is the entry point for every compute request.
//
// It runs on its own goroutine (see the ReadLoop dispatch) so a guest operation
// that takes minutes does not stall console traffic for every other guest on
// the host.
func (a *Agent) handleComputeMessage(parent context.Context, msg WSMessage) {
	rt, err := a.compute(parent)
	if err != nil {
		a.replyComputeError(msg, err)
		return
	}

	ctx, cancel := context.WithTimeout(parent, timeoutFor(msg.Type))
	defer cancel()

	switch msg.Type {
	case MsgComputeProbeRequest:
		rt.handleProbe(ctx, msg)

	case MsgComputeInstallRequest:
		rt.handleInstall(ctx, msg)

	case MsgComputeConfigRequest:
		rt.handleComputeConfig(ctx, a, msg)

	case MsgComputeMigrateRequest:
		rt.handleComputeMigrate(ctx, a, msg)

	case MsgPoolDetectRequest, MsgPoolCreateRequest, MsgPoolStatsRequest, MsgPoolRemoveRequest:
		rt.handlePool(ctx, msg)

	case MsgPoolBrowseRequest:
		rt.handlePoolBrowse(ctx, msg)

	case MsgPoolFileRequest:
		rt.handlePoolFile(ctx, msg)

	case MsgNetDetectRequest, MsgNetEnsureRequest, MsgNetRemoveRequest:
		rt.handleNet(ctx, msg)

	case MsgImageFetchRequest, MsgImageListRequest, MsgImageRemoveRequest, MsgImageCancelRequest:
		rt.handleImage(ctx, msg)

	default:
		if msg.VM != nil {
			rt.handleVM(ctx, msg)
			return
		}
		if msg.CT != nil {
			rt.handleCT(ctx, msg)
			return
		}
		a.replyComputeError(msg, fmt.Errorf("compute: %q carried no payload", msg.Type))
	}
}

// timeoutFor picks the budget for a message type.
func timeoutFor(messageType string) time.Duration {
	switch messageType {
	case MsgComputeProbeRequest:
		return computeProbeTimeout

	case MsgComputeInstallRequest:
		// apt-get update and the install itself, bounded by the mirror and the package size.
		return computeMediumTimeout

	case MsgComputeConfigRequest:
		// Bounded by a vm_query against the supervisor, which is the check
		// that stops a move from stranding a running guest.
		return computeQuickTimeout

	case MsgComputeMigrateRequest:
		// Bounded by how much data is being moved, which McHarbor does not
		// know and the host does not decide.
		return computeLongTimeout

	case MsgImageFetchRequest:
		// Bounded only by the mirror and the disk. The server's cancel path is
		// the real control here.
		return computeLongTimeout

	case MsgVMImportInspect, MsgVMImportConvert, MsgVMImportStage:
		// Converting a disk from another hypervisor is bounded by the size of
		// that disk, not by anything McHarbor decides. The server's cancel
		// path is the real control here, exactly as it is for an image fetch.
		// Staging is the same story: it is the upload itself crossing to this
		// host.
		return computeLongTimeout

	case MsgVMMigrate:
		// A cold half streams whole disks and a live send waits out the mirror
		// and the RAM transfer; both are bounded by the data, and McHarbor
		// cancels a migration it gives up on.
		return computeLongTimeout

	case MsgVMBackupRun, MsgVMBackupRestore, MsgCTBackupRun, MsgCTBackupRestore:
		// The copy and the upload share this one context, and the upload's own
		// deadline is derived from it, so the quick budget here cut every run off
		// at sixty seconds whatever the host and the upload grant allowed.
		return computeLongTimeout

	case MsgVMShutdown, MsgVMStop:
		// A shutdown waits out the guest's ACPI grace by design, and the grace
		// alone can be the whole quick budget. Sixty seconds of grace inside a
		// sixty second deadline is a call that always expires — which is what
		// made a revert fail before it had reverted anything.
		return computeMediumTimeout

	case MsgVMDefine, MsgVMDiskCreate, MsgVMDiskResize, MsgVMDelete,
		MsgVMSnapshotCreate, MsgVMSnapshotRevert, MsgVMCloneRequest,
		MsgVMGuestAgentDisc, MsgVMDiskHotplug,
		MsgCTCreate, MsgCTDelete, MsgCTSnapshotCreate, MsgCTSnapshotRevert,
		MsgCTCloneRequest, MsgPoolCreateRequest, MsgPoolRemoveRequest:
		return computeMediumTimeout

	default:
		return computeQuickTimeout
	}
}

// handleProbe answers a capability request.
func (rt *computeRuntime) handleProbe(ctx context.Context, msg WSMessage) {
	// A probe request from the server is always a deliberate refresh — it runs
	// on connect, on demand from the UI, and on the six-hour timer — so the
	// cache is bypassed rather than possibly answering with a stale picture of
	// a host the operator just changed.
	report := rt.Capability(ctx, true)

	errlog.Log(rt.send(WSMessage{
		Type: MsgComputeProbeResponse,
		ID:   msg.ID,
		Compute: &ComputePayload{
			Operation:  "probe",
			Success:    true,
			Capability: toProtocolCapability(report),
		},
	}))
}

// handleInstall installs the packages for one capability item and answers with what it ran and
// a fresh probe, so the page shows the new state without a second round trip. The item is the
// only input; the packages come from the agent's own table.
func (rt *computeRuntime) handleInstall(ctx context.Context, msg WSMessage) {
	item := ""
	if msg.Compute != nil && msg.Compute.Install != nil {
		item = msg.Compute.Install.Item
	}
	packages, output, err := capability.Install(ctx, item)
	reply := &ComputePayload{
		Operation: "install",
		Success:   err == nil,
		Install:   &ComputeInstall{Item: item, Packages: packages, Output: output},
	}
	if err != nil {
		reply.Error = err.Error()
	}
	reply.Capability = toProtocolCapability(rt.Capability(ctx, true))
	errlog.Log(rt.send(WSMessage{Type: MsgComputeInstallResponse, ID: msg.ID, Compute: reply}))
}

// toProtocolCapability converts a probe report into the wire shape.
func toProtocolCapability(in capability.Report) *ComputeCapability {
	out := &ComputeCapability{
		OS:                in.OS,
		Arch:              in.Arch,
		KernelVersion:     in.KernelVersion,
		AgentVersion:      in.AgentVersion,
		Supported:         in.Supported,
		UnsupportedWhy:    in.UnsupportedWhy,
		CanRunVMs:         in.CanRunVMs,
		CanRunContainers:  in.CanRunContainers,
		SeccompBuiltIn:    in.SeccompBuiltIn,
		SupervisorMode:    in.SupervisorMode,
		HostCPUs:          in.HostCPUs,
		HostMemoryBytes:   in.HostMemoryBytes,
		HostSockets:       in.HostSockets,
		HostPhysicalCores: in.HostPhysicalCores,
		ProbedAt:          time.Now().UTC().Format(time.RFC3339),
	}
	for _, item := range in.Items {
		out.Items = append(out.Items, ComputeCapabilityItem{
			Key:         item.Key,
			Present:     item.Present,
			Detail:      item.Detail,
			Remediation: item.Remediation,
			Critical:    item.Critical,
			Blocks:      item.Blocks,
			Installable: item.Installable,
		})
	}
	for _, binary := range in.QEMUBinaries {
		out.QEMUBinaries = append(out.QEMUBinaries, ComputeQEMUBinary{
			Path:         binary.Path,
			Arch:         binary.Arch,
			Version:      binary.Version,
			MachineTypes: binary.MachineTypes,
			KVMAccel:     binary.KVMAccel,
			Devices:      binary.Devices,
			CPUModels:    binary.CPUModels,
			QMPCommands:  binary.QMPCommands,
		})
	}
	if in.HugePages != nil {
		out.HugePages = &ComputeHugePages{
			Total:    in.HugePages.Total,
			Free:     in.HugePages.Free,
			SizeKB:   in.HugePages.SizeKB,
			Surplus:  in.HugePages.Surplus,
			Reserved: in.HugePages.Reserved,
		}
	}
	for _, node := range in.NUMANodes {
		out.NUMANodes = append(out.NUMANodes, ComputeNUMANode{
			ID:          node.ID,
			CPUs:        node.CPUs,
			MemoryBytes: node.MemoryBytes,
			FreeBytes:   node.FreeBytes,
		})
	}
	for _, r := range in.SubuidRanges {
		out.SubuidRanges = append(out.SubuidRanges, ComputeIDRange{Start: r.Start, Count: r.Count})
	}
	for _, r := range in.SubgidRanges {
		out.SubgidRanges = append(out.SubgidRanges, ComputeIDRange{Start: r.Start, Count: r.Count})
	}
	if links, err := netconf.Bridges(); err == nil {
		out.Bridges = toProtocolLinks(links)
	}
	return out
}

func toProtocolLinks(in []netconf.Link) []ComputeHostLink {
	out := make([]ComputeHostLink, 0, len(in))
	for _, link := range in {
		out = append(out, ComputeHostLink{
			Name:            link.Name,
			Kind:            link.Kind,
			MAC:             link.MAC,
			MTU:             link.MTU,
			Up:              link.Up,
			Addresses:       link.Addresses,
			Master:          link.Master,
			Managed:         link.Managed,
			HasDefaultRoute: link.HasDefaultRoute,
			Upper:           link.Upper,
		})
	}
	return out
}

// handlePool answers the storage pool family.
func (rt *computeRuntime) handlePool(ctx context.Context, msg WSMessage) {
	req := msg.Pool
	reply := &ComputePoolPayload{Operation: req.Operation, EnvID: req.EnvID, PoolID: req.PoolID}
	replyType := MsgPoolDetectResponse

	switch msg.Type {
	case MsgPoolDetectRequest:
		for _, detection := range rt.pools.DetectAll(ctx) {
			entry := ComputePoolBackend{
				Backend:     detection.Backend,
				Available:   detection.Available,
				Detail:      detection.Detail,
				Remediation: detection.Remediation,
				Instant:     detection.Instant,
			}
			for _, candidate := range detection.Candidates {
				entry.Candidates = append(entry.Candidates, ComputePoolCandidate{
					Target:        candidate.Target,
					Mountpoint:    candidate.Mountpoint,
					CapacityBytes: candidate.CapacityBytes,
					FreeBytes:     candidate.FreeBytes,
				})
			}
			reply.Detected = append(reply.Detected, entry)
		}
		reply.Success = true

	case MsgPoolCreateRequest:
		replyType = MsgPoolCreateResponse
		driver, pool, err := rt.resolvePool(req)
		if err != nil {
			reply.Error = err.Error()
			break
		}
		stats, err := driver.EnsurePool(ctx, pool)
		if err != nil {
			reply.Error = err.Error()
			break
		}
		reply.Stats = toProtocolPoolStats(stats)
		reply.Success = true

	case MsgPoolStatsRequest:
		replyType = MsgPoolStatsResponse
		driver, pool, err := rt.resolvePool(req)
		if err != nil {
			reply.Error = err.Error()
			break
		}
		stats, err := driver.Stats(ctx, pool)
		if err != nil {
			reply.Error = err.Error()
			break
		}
		reply.Stats = toProtocolPoolStats(stats)
		reply.Success = true

	case MsgPoolRemoveRequest:
		replyType = MsgPoolRemoveResponse
		driver, pool, err := rt.resolvePool(req)
		if err != nil {
			reply.Error = err.Error()
			break
		}
		if err := driver.RemovePool(ctx, pool, req.Force); err != nil {
			// A refusal because the pool still holds volumes names them, so
			// the operator learns what is in the way rather than only that
			// something is; the code is what the server branches on.
			reply.Error = err.Error()
			if errors.Is(err, storage.ErrInUse) {
				reply.ErrorCode = poolInUseCode
			}
			break
		}
		reply.Success = true
	}

	errlog.Log(rt.send(WSMessage{Type: replyType, ID: msg.ID, Pool: reply}))
}

// resolvePool validates a pool request and returns its driver.
//
// The target is re-validated here even though the server checked it. The
// server's check is UX — it stops a typo before it becomes a request — and this
// one is security: this process is root, and it must not accept a pool rooted
// at /etc whatever the control plane sent.
func (rt *computeRuntime) resolvePool(req *ComputePoolPayload) (storage.Driver, storage.Pool, error) {
	driver, err := rt.pools.Get(req.Backend)
	if err != nil {
		return nil, storage.Pool{}, err
	}
	if req.Backend == storage.BackendDir || req.Backend == storage.BackendBtrfs {
		if err := storage.ValidatePoolTarget(req.Target); err != nil {
			return nil, storage.Pool{}, err
		}
	}
	return driver, storage.Pool{
		ID:      req.PoolID,
		Name:    req.Name,
		Backend: req.Backend,
		Target:  req.Target,
	}, nil
}

func toProtocolPoolStats(in storage.Stats) *ComputePoolStats {
	return &ComputePoolStats{
		CapacityBytes:  in.CapacityBytes,
		UsedBytes:      in.UsedBytes,
		FreeBytes:      in.FreeBytes,
		AllocatedBytes: in.AllocatedBytes,
		VolumeCount:    in.VolumeCount,
		SampledAt:      time.Now().UTC().Format(time.RFC3339),
	}
}

// adoptConfirmWindow is how long an external switch waits for McHarbor to
// confirm the host is still reachable before putting the adapter back.
//
// Long enough for a reply to cross a slow link and for the server to answer,
// short enough that an operator watching a dead session gets their host back
// without a reboot.
const adoptConfirmWindow = 45 * time.Second

// handleNet answers the network family.
func (rt *computeRuntime) handleNet(ctx context.Context, msg WSMessage) {
	req := msg.Net
	reply := &ComputeNetPayload{Operation: req.Operation, EnvID: req.EnvID, NetworkID: req.NetworkID}
	replyType := MsgNetDetectResponse

	switch msg.Type {
	case MsgNetDetectRequest:
		links, err := netconf.Links()
		if err != nil {
			reply.Error = err.Error()
			break
		}
		reply.Links = toProtocolLinks(links)
		reply.Success = true

	case MsgNetEnsureRequest:
		replyType = MsgNetEnsureResponse
		if err := rt.ensureNetwork(ctx, req); err != nil {
			reply.Error = err.Error()
			if errors.Is(err, netconf.ErrUplinkBusy) {
				// The operator's own host to fix, so the server needs to tell
				// them rather than log it and answer with a gateway error.
				reply.ErrorCode = "uplink_busy"
			} else {
				// name_taken, parent_not_found and the rest of the host link
				// vocabulary, so a shim refusal reaches the operator by name.
				reply.ErrorCode = hostLinkErrorCode(err)
			}
			break
		}
		reply.Success = true

	case MsgNetRemoveRequest:
		replyType = MsgNetRemoveResponse
		if err := rt.removeNetwork(ctx, req); err != nil {
			reply.Error = err.Error()
			break
		}
		reply.Success = true

	case MsgNetAdoptConfirm:
		replyType = MsgNetAdoptConfirmed
		rt.confirmAdoption(req.NetworkID)
		reply.Success = true
	}

	errlog.Log(rt.send(WSMessage{Type: replyType, ID: msg.ID, Net: reply}))
}

// ensureNetwork creates or verifies a network.
//
// The three modes differ in how much of the host they own, and the ordering
// here follows that: bridged owns nothing, managed owns its own bridge, and
// external takes over one of the host's adapters.
func (rt *computeRuntime) ensureNetwork(ctx context.Context, req *ComputeNetPayload) error {
	switch req.Mode {
	case "bridged":
		return rt.ensureBridged(req)
	case "external":
		return rt.ensureExternal(req)
	case netconf.DriverMacvlan:
		return rt.ensureDirect(req)
	case "ipvlan":
		// Withdrawn: a guest on an ipvlan network shares the parent's MAC, so
		// no DHCP server can tell two of them apart and no VM can attach at
		// all. Networks recorded before it went can still be removed below.
		return fmt.Errorf("ipvlan networks are no longer supported")
	default:
		return rt.ensureManaged(ctx, req)
	}
}

// ensureDirect prepares a macvlan network.
//
// It creates almost nothing: the guests' own children are made at guest start,
// so all that happens here is resolving the VLAN sub-interface a tagged network
// needs and, when the operator asked for it, the shim that lets the host reach
// its own guests. The parent adapter is never reconfigured, which is the whole
// difference between this and external adoption.
func (rt *computeRuntime) ensureDirect(req *ComputeNetPayload) error {
	if req.UplinkLink == "" {
		return fmt.Errorf("a %s network needs a parent interface", req.Mode)
	}
	if err := netconf.ValidateLinkMode(req.Mode, req.LinkMode); err != nil {
		return err
	}

	if _, err := netconf.EnsureVLANParent(req.UplinkLink, req.VLANTag); err != nil {
		return err
	}

	hostLinkApplyMu.Lock()
	defer hostLinkApplyMu.Unlock()
	if !req.HostAccess {
		// A shim from an earlier version of this network is removed rather
		// than left behind holding an address on the segment.
		errlog.Log(netconf.RemoveHostShim(req.NetworkID))
		rt.forgetHostShim(req.NetworkID)
		return nil
	}
	if req.Mode == netconf.DriverMacvlan && req.LinkMode == "passthru" {
		// passthru hands the parent to one child, so the host cannot have a
		// second one alongside the guest's.
		return netconf.ErrHostAccessUnsupported
	}
	// The uplink and tag are recorded rather than the resolved parent, so a
	// replay after reboot can recreate a VLAN sub-interface McHarbor made.
	rec := hostLinkRecord{
		Owner:       HostLinkOwnerCompute,
		OwnerID:     req.NetworkID,
		Name:        req.HostLinkName,
		Parent:      req.UplinkLink,
		VLANTag:     req.VLANTag,
		IPv4CIDR:    req.HostAccessIP,
		IPv6Enabled: req.IPv6Enabled,
		IPv6CIDR:    req.HostAccessIPv6,
	}
	name, err := applyHostLinkRecord(rec, rt.previousHostShim(req.NetworkID).staleCIDRs())
	if err != nil {
		return err
	}
	rec.Name = name
	rt.rememberHostShim(rec)
	return nil
}

// previousHostShim reads the record the last ensure of this network saved.
func (rt *computeRuntime) previousHostShim(networkID string) *hostLinkRecord {
	if rt.hostLinks == nil {
		return nil
	}
	prev, err := rt.hostLinks.load(HostLinkOwnerCompute, networkID)
	if err != nil {
		rt.logger.Warn("compute: previous host shim record unreadable", "network", networkID, "error", err)
		return nil
	}
	return prev
}

// rememberHostShim persists a shim's desired state for the startup replay. A
// failure is logged, not returned: the shim is up, only its reboot survival is lost.
func (rt *computeRuntime) rememberHostShim(rec hostLinkRecord) {
	if rt.hostLinks == nil {
		return
	}
	if err := rt.hostLinks.save(rec); err != nil {
		rt.logger.Error("compute: persisting host shim failed", "network", rec.OwnerID, "error", err)
	}
}

// forgetHostShim drops a network's shim record, so a reboot does not bring back
// a shim the network no longer asks for.
func (rt *computeRuntime) forgetHostShim(networkID string) {
	if rt.hostLinks == nil {
		return
	}
	if err := rt.hostLinks.forget(HostLinkOwnerCompute, networkID); err != nil {
		rt.logger.Warn("compute: forgetting host shim failed", "network", networkID, "error", err)
	}
}

// ensureBridged verifies the operator's own bridge and stops.
//
// It deliberately creates nothing: the whole safety property of bridged mode is
// that McHarbor only ever adds a port to a bridge the operator already made, so
// a misconfiguration cannot strand their own SSH session.
func (rt *computeRuntime) ensureBridged(req *ComputeNetPayload) error {
	links, err := netconf.Bridges()
	if err != nil {
		return err
	}
	for _, link := range links {
		if link.Name == req.Bridge {
			return nil
		}
	}
	return fmt.Errorf("bridge %q does not exist on this host", req.Bridge)
}

// ensureManaged builds McHarbor's own bridge and makes it actually usable —
// an address on the bridge, a DHCP server handing out the range, and NAT out
// through the uplink.
//
// The uplink is never enslaved here and the host's own addressing is never
// touched. It names the interface masquerade leaves by, which is why choosing
// the one carrying the default route is the normal answer rather than a
// dangerous one.
func (rt *computeRuntime) ensureManaged(ctx context.Context, req *ComputeNetPayload) error {
	if req.Bridge == "" {
		return fmt.Errorf("a managed network needs a bridge name")
	}
	if _, err := netconf.EnsureBridge(req.Bridge, 0); err != nil {
		return err
	}
	if req.GatewayIP != "" && req.SubnetCIDR != "" {
		if err := netconf.SetBridgeAddress(req.Bridge, gatewayCIDR(req)); err != nil {
			return err
		}
	}

	if req.NATEnabled {
		if err := netconf.EnsureNAT(ctx, netconf.NATSpec{
			NetworkID:  req.NetworkID,
			Bridge:     req.Bridge,
			SubnetCIDR: req.SubnetCIDR,
			Uplink:     req.UplinkLink,
		}); err != nil {
			return err
		}
	} else {
		errlog.Log(netconf.RemoveNAT(ctx, req.NetworkID))
	}

	if req.DHCPEnabled {
		network, err := dhcpNetwork(req)
		if err != nil {
			return err
		}
		if err := rt.dhcp.Ensure(network); err != nil {
			return err
		}
	} else if err := rt.dhcp.Remove(req.NetworkID); err != nil {
		return err
	}
	return nil
}

// ensureExternal turns one of the host's own adapters into a bridge guests can
// join, the way Hyper-V's external switch and VMware's bridged networking do.
//
// The adoption is armed with a rollback timer before the reply is written. If
// the change breaks the host's connectivity, the reply never reaches McHarbor,
// no confirmation comes back, and the timer restores the adapter — so the
// failure mode is a network that did not get created rather than a host that
// cannot be reached.
func (rt *computeRuntime) ensureExternal(req *ComputeNetPayload) error {
	if req.Bridge == "" || req.UplinkLink == "" {
		return fmt.Errorf("an external network needs a bridge name and an adapter")
	}

	rt.adoptedMu.Lock()
	_, already := rt.adopted[req.NetworkID]
	rt.adoptedMu.Unlock()
	if already {
		return nil
	}

	state, err := netconf.AdoptLink(netconf.AdoptSpec{
		Bridge: req.Bridge,
		Uplink: req.UplinkLink,
	})
	if err != nil {
		return err
	}

	rt.adoptedMu.Lock()
	rt.adopted[req.NetworkID] = state
	rt.adoptedMu.Unlock()
	rt.armRevert(req.NetworkID)
	return nil
}

// removeNetwork tears down whatever the network actually owns.
func (rt *computeRuntime) removeNetwork(ctx context.Context, req *ComputeNetPayload) error {
	switch req.Mode {
	case "bridged":
		// Bridged networks own nothing on the host — the bridge is the
		// operator's — so removing one is purely a database operation on the
		// server side. Reporting success without touching anything is the
		// correct answer, not a silent no-op.
		return nil

	case netconf.DriverMacvlan, "ipvlan":
		// ipvlan is withdrawn, but a network recorded under it still owns a
		// shim and possibly a VLAN sub-interface, and refusing to remove one
		// would leave the operator with an entry they cannot get rid of.
		// The guests' own children went with the guests. What is left is the
		// host shim and, when McHarbor made it, the VLAN sub-interface — and
		// the latter only once nothing else is still stacked on it.
		hostLinkApplyMu.Lock()
		err := netconf.RemoveHostShim(req.NetworkID)
		hostLinkApplyMu.Unlock()
		if err != nil {
			return err
		}
		rt.forgetHostShim(req.NetworkID)
		if req.VLANTag == 0 {
			return nil
		}
		return netconf.RemoveVLANParent(netconf.VLANParentName(req.VLANTag))

	case "external":
		rt.adoptedMu.Lock()
		state, ok := rt.adopted[req.NetworkID]
		delete(rt.adopted, req.NetworkID)
		if timer, pending := rt.pendingRevert[req.NetworkID]; pending {
			timer.Stop()
			delete(rt.pendingRevert, req.NetworkID)
		}
		rt.adoptedMu.Unlock()
		if !ok {
			// The agent restarted since the adoption, so the bookkeeping is
			// gone. Rebuild just enough to put the adapter back: its addresses
			// are on the bridge now, and that is where they have to come from.
			state = netconf.AdoptState{
				Bridge:        req.Bridge,
				Uplink:        req.UplinkLink,
				BridgeCreated: true,
			}
		}
		return netconf.ReleaseLink(state)

	default:
		if err := rt.dhcp.Remove(req.NetworkID); err != nil {
			return err
		}
		if err := netconf.RemoveNAT(ctx, req.NetworkID); err != nil {
			return err
		}
		return netconf.DeleteBridge(req.Bridge)
	}
}

// armRevert schedules the rollback that makes an external switch safe to try.
func (rt *computeRuntime) armRevert(networkID string) {
	timer := time.AfterFunc(adoptConfirmWindow, func() {
		rt.adoptedMu.Lock()
		state, ok := rt.adopted[networkID]
		delete(rt.adopted, networkID)
		delete(rt.pendingRevert, networkID)
		rt.adoptedMu.Unlock()
		if !ok {
			return
		}
		rt.logger.Warn("external switch not confirmed; restoring the adapter",
			"network", networkID, "uplink", state.Uplink, "bridge", state.Bridge)
		if err := netconf.ReleaseLink(state); err != nil {
			rt.logger.Error("restoring the adapter failed",
				"network", networkID, "uplink", state.Uplink, "error", err)
		}
	})

	rt.adoptedMu.Lock()
	rt.pendingRevert[networkID] = timer
	rt.adoptedMu.Unlock()
}

// confirmAdoption cancels the rollback, which McHarbor asks for only once it
// has seen the agent still answering after the change.
func (rt *computeRuntime) confirmAdoption(networkID string) {
	rt.adoptedMu.Lock()
	timer, ok := rt.pendingRevert[networkID]
	delete(rt.pendingRevert, networkID)
	rt.adoptedMu.Unlock()
	if ok {
		timer.Stop()
		rt.logger.Info("external switch confirmed", "network", networkID)
	}
}

// gatewayCIDR renders the bridge address with the network's prefix length.
//
// The gateway arrives as a bare address and netlink needs a prefix; using the
// subnet's own length is what makes the bridge answer for the whole range
// rather than for a single host.
func gatewayCIDR(req *ComputeNetPayload) string {
	if strings.Contains(req.GatewayIP, "/") {
		return req.GatewayIP
	}
	if _, subnet, err := net.ParseCIDR(req.SubnetCIDR); err == nil {
		ones, _ := subnet.Mask.Size()
		return fmt.Sprintf("%s/%d", req.GatewayIP, ones)
	}
	return req.GatewayIP + "/24"
}

// dhcpNetwork translates the wire payload into a server definition.
func dhcpNetwork(req *ComputeNetPayload) (dhcp.Network, error) {
	gateway := net.ParseIP(strings.SplitN(req.GatewayIP, "/", 2)[0])
	if gateway == nil {
		return dhcp.Network{}, fmt.Errorf("gateway %q is not an address", req.GatewayIP)
	}
	_, subnet, err := net.ParseCIDR(req.SubnetCIDR)
	if err != nil {
		return dhcp.Network{}, fmt.Errorf("subnet %q is not a CIDR: %w", req.SubnetCIDR, err)
	}
	return dhcp.Network{
		NetworkID:  req.NetworkID,
		Bridge:     req.Bridge,
		ServerIP:   gateway,
		Subnet:     subnet,
		RangeStart: net.ParseIP(req.DHCPRangeStart),
		RangeEnd:   net.ParseIP(req.DHCPRangeEnd),
	}, nil
}

// handleImage answers the image family.
func (rt *computeRuntime) handleImage(ctx context.Context, msg WSMessage) {
	req := msg.ComputeImage
	reply := &ComputeImagePayload{Operation: req.Operation, EnvID: req.EnvID, ImageID: req.ImageID}
	replyType := MsgImageFetchResult

	switch msg.Type {
	case MsgImageFetchRequest:
		result, err := rt.images.Fetch(ctx, imagefetch.Request{
			ImageID:         req.ImageID,
			Kind:            req.Kind,
			SourceURL:       req.SourceURL,
			ChecksumURL:     req.ChecksumURL,
			Checksum:        req.Checksum,
			ChecksumAlgo:    req.ChecksumAlgo,
			SignatureURL:    req.SignatureURL,
			AllowUnverified: req.AllowUnverified,
			FromLibrary:     req.FromLibrary,
			Filename:        req.Filename,
			LibraryURL:      rt.absoluteServerURL(req.LibraryURL),
			LibraryToken:    req.LibraryToken,
		}, func(progress imagefetch.Progress) {
			// Progress rides the same request id so the server routes it to
			// whoever is showing the dialog. A dropped frame is harmless: the
			// next one supersedes it.
			errlog.Log(rt.send(WSMessage{
				Type: MsgImageFetchProgress,
				ID:   msg.ID,
				ComputeImage: &ComputeImagePayload{
					Operation:  "progress",
					ImageID:    progress.ImageID,
					BytesDone:  progress.BytesDone,
					BytesTotal: progress.BytesTotal,
					Percent:    progress.Percent,
					Phase:      progress.Phase,
				},
			}))
		})
		if err != nil {
			reply.Error = err.Error()
			break
		}
		reply.LocalPath = result.LocalPath
		reply.SizeBytes = result.SizeBytes
		reply.Checksum = result.Checksum
		reply.Verified = result.Verified
		reply.Signed = result.Signed
		reply.Success = true

	case MsgImageListRequest:
		replyType = MsgImageListResponse
		cached, err := rt.images.List(rt.imagesInUse(ctx))
		if err != nil {
			reply.Error = err.Error()
			break
		}
		for _, image := range cached {
			reply.Cached = append(reply.Cached, ComputeCachedImage{
				ImageID:    image.ImageID,
				Kind:       image.Kind,
				LocalPath:  image.LocalPath,
				SizeBytes:  image.SizeBytes,
				Verified:   image.Verified,
				InUse:      image.InUse,
				LastUsedAt: image.LastUsedAt.UTC().Format(time.RFC3339),
			})
		}
		reply.Success = true

	case MsgImageRemoveRequest:
		replyType = MsgImageRemoveResult
		if err := rt.images.Remove(req.ImageID, rt.imagesInUse(ctx)); err != nil {
			reply.Error = err.Error()
			break
		}
		reply.Success = true

	case MsgImageCancelRequest:
		replyType = MsgImageRemoveResult
		reply.Success = rt.images.Cancel(req.ImageID)
	}

	errlog.Log(rt.send(WSMessage{Type: replyType, ID: msg.ID, ComputeImage: reply}))
}

// imagesInUseTimeout bounds one images-in-use pass: one `qemu-img info` per
// link of every guest disk's chain.
const imagesInUseTimeout = 2 * time.Minute

// imagesInUse reports which cached images a guest disk on this host backs onto.
//
// Read from the disks' own qcow2 chains at query time (see images_in_use.go).
// It is the agent's line of defence beside the server's rows: an image a chain
// still reaches is never evicted, whatever McHarbor believes. A pass that cannot
// complete errs towards "in use", because a full cache is the cheaper mistake.
func (rt *computeRuntime) imagesInUse(parent context.Context) map[string]bool {
	if rt.images == nil {
		return map[string]bool{}
	}
	cached, err := rt.images.List(nil)
	if err != nil {
		rt.logger.Warn("compute: listing the image cache for the in-use check failed", "error", err)
		return map[string]bool{}
	}
	ctx, cancel := context.WithTimeout(parent, imagesInUseTimeout)
	defer cancel()

	disks := append(guestArgvFiles(vm.RuntimeRoot()), rt.backedVolumes.volumes()...)
	inUse := backedImages(disks, func(path string) string {
		backing, _ := imageBacking(ctx, path)
		return backing
	}, cached)
	if ctx.Err() != nil {
		rt.logger.Warn("compute: the images-in-use pass ran out of time; treating every image as in use")
		for _, image := range cached {
			inUse[image.ImageID] = true
		}
	}
	return inUse
}

// handleGuestAgentDisc answers with the guest-agent disc this host prepared.
//
// It deliberately does not build: the discs are made when the agent starts, so
// that inserting one is a stat rather than a download an operator waits out.
// A disc that is not there yet is reported as pending, not built on the spot.
func (rt *computeRuntime) handleGuestAgentDisc(_ context.Context, req *VMPayload, reply *VMPayload) error {
	if rt.images == nil {
		return fmt.Errorf("compute: %s needs an image cache this host has not configured", MsgVMGuestAgentDisc)
	}

	root := rt.images.Root()
	if !guestdisc.Ready(root, req.GuestFamily) {
		// Not an error: the prepare pass may still be running, or its last
		// attempt failed and the next agent start will retry. Either way the
		// operator should be told to come back, not shown a broken host.
		reply.ErrorCode = guestDiscPendingCode
		return nil
	}

	reply.MediaPath = guestdisc.Path(root, req.GuestFamily)
	reply.MediaLabel = guestDiscLabel
	return nil
}

// prepareGuestDiscs builds every guest-agent disc this host can serve, once,
// in the background at agent start.
//
// The Windows disc downloads its payload here rather than on the request path;
// that download is why inserting a disc used to need a 30 minute budget.
func (rt *computeRuntime) prepareGuestDiscs(parent context.Context) {
	if rt.images == nil {
		return
	}

	ctx, cancel := context.WithTimeout(parent, guestDiscPrepareTimeout)
	defer cancel()

	root := rt.images.Root()
	for _, family := range guestdisc.Families() {
		if guestdisc.Ready(root, family) {
			continue
		}
		if _, err := guestdisc.Build(ctx, root, family); err != nil {
			// Logged and dropped. A host that cannot reach fedorapeople still
			// serves its Linux and BSD discs, and the button says "preparing"
			// for the one that is missing instead of the host looking broken.
			rt.logger.Warn("compute: preparing the guest agent disc failed",
				"family", family, "error", err)
			continue
		}
		rt.logger.Info("compute: guest agent disc ready", "family", family)
	}
}

// handleVM answers the KVM family.
func (rt *computeRuntime) handleVM(ctx context.Context, msg WSMessage) {
	req := msg.VM
	reply := &VMPayload{Operation: msg.Type, EnvID: req.EnvID, VMID: req.VMID}

	// A host with no supervisor still runs the image and storage half, so the
	// runtime exists and only this manager is missing. Checking once here beats
	// a nil dereference inside whichever operation was asked for.
	if rt.vms == nil {
		reply.Error = errNoSupervisor.Error()
		errlog.Log(rt.send(WSMessage{Type: MsgVMResult, ID: msg.ID, VM: reply}))
		return
	}

	var err error
	switch msg.Type {
	case MsgVMDefine:
		err = rt.defineVM(ctx, req)
	case MsgVMStart:
		err = rt.startVM(ctx, req, reply)
	case MsgVMShutdown:
		var escalated bool
		escalated, err = rt.vms.Shutdown(ctx, req.VMID, time.Duration(req.GraceSeconds)*time.Second)
		if err == nil {
			rt.releaseGuestTaps(req.VMID)
		}
		if escalated {
			// Recorded so the audit row and the UI can say the guest ignored
			// ACPI, rather than leaving the operator wondering why a graceful
			// stop took a minute and a half.
			reply.ErrorCode = "escalated_to_hard_stop"
		}
	case MsgVMStop:
		err = rt.vms.Stop(ctx, req.VMID)
		if err == nil {
			rt.releaseGuestTaps(req.VMID)
		}
	case MsgVMReset:
		err = rt.vms.Reset(ctx, req.VMID)
	case MsgVMPause:
		err = rt.vms.Pause(ctx, req.VMID)
	case MsgVMResume:
		err = rt.vms.Resume(ctx, req.VMID)
	case MsgVMDelete:
		err = rt.deleteVM(ctx, req)
	case MsgVMQuery:
		var states []vm.State
		var orphans []string
		states, orphans, err = rt.vms.Query(ctx, knownVMIDs(req))
		for _, state := range states {
			reply.States = append(reply.States, *toProtocolVMState(state))
		}
		reply.Orphans = orphans
		// Reconciliation is also where a guest stranded frozen by an agent
		// that died mid-snapshot is found. It runs on connect and on the
		// reconcile timer, which are exactly the moments after such a crash.
		if err == nil {
			rt.thawStrandedGuests(ctx, runningVMIDs(states))
		}
	case MsgHostDeviceList:
		var found []hostdev.Device
		found, err = hostdev.List()
		if err == nil {
			for _, device := range found {
				reply.HostDevices = append(reply.HostDevices, toProtocolHostDevice(device))
			}
		}
	case MsgVMStats:
		var sample vm.Stats
		sample, err = rt.vms.Stats(ctx, req.VMID)
		if err == nil {
			reply.Stats = toProtocolVMStats(sample)
		}
	case MsgVMDiskCreate, MsgVMDiskResize, MsgVMDiskDelete:
		err = rt.handleVMDisk(ctx, msg.Type, req, reply)
	case MsgVMDiskList:
		err = rt.handleVMDiskList(ctx, req, reply)
	case MsgVMGuestAgentDisc:
		err = rt.handleGuestAgentDisc(ctx, req, reply)
	case MsgVMLog:
		err = rt.handleVMLog(ctx, req, reply)
	case MsgVMSnapshotCreate, MsgVMSnapshotRevert, MsgVMSnapshotDelete:
		err = rt.handleVMSnapshot(ctx, msg.Type, req, reply)
	case MsgVMImportInspect, MsgVMImportConvert, MsgVMImportStage, MsgVMImportDiscard:
		err = rt.handleVMImport(ctx, msg.Type, req, reply)
	case MsgVMQEMUCapability:
		err = rt.handleQEMUCapability(ctx, reply)
	case MsgVMGuestQuery:
		err = rt.handleGuestQuery(ctx, req, reply)
	case MsgVMMemorySet:
		err = rt.handleMemorySet(ctx, req, reply)
	case MsgVMCPUSet:
		err = rt.handleCPUSet(ctx, req, reply)
	case MsgVMThrottleSet:
		err = rt.handleThrottleSet(ctx, req, reply)
	case MsgVMImageInspect:
		err = rt.handleImageInspect(ctx, req, reply)
	case MsgVMBackupRun:
		err = rt.handleVMBackupRun(ctx, req, reply)
	case MsgVMBackupRestore:
		err = rt.handleVMBackupRestore(ctx, req, reply)
	case MsgVMCancel:
		reply.Success = rt.vms.Cancel(req.VMID)
		errlog.Log(rt.send(WSMessage{Type: MsgVMResult, ID: msg.ID, VM: reply}))
		return
	case MsgVMCloneRequest:
		err = rt.handleVMClone(ctx, req, reply)
	case MsgVMMigrate:
		err = rt.handleVMMigrate(ctx, req, reply)
	case MsgVMSpiceTicket:
		err = rt.handleSpiceTicket(ctx, req, reply)
	case MsgVMDiskHotplug, MsgVMMediaChange, MsgVMNICHotplug:
		err = rt.handleHotplug(ctx, msg.Type, req, reply)
	default:
		// Every vm-family message this agent knows has a case above, guest-agent
		// queries included (handleGuestQuery). Anything else is from a newer
		// McHarbor, and a clear refusal beats silence, which would hang the
		// caller until its own timeout.
		err = fmt.Errorf("compute: %s is not a message this agent understands", msg.Type)
	}

	if err != nil {
		reply.Error = err.Error()
	} else if reply.Error == "" {
		reply.Success = true
	}
	errlog.Log(rt.send(WSMessage{Type: MsgVMResult, ID: msg.ID, VM: reply}))
}

// absoluteServerURL resolves a server-relative path against the McHarbor
// address this agent dials.
//
// The server cannot build this itself: it does not know which of its addresses
// this host can reach, which is why it sends a path and the agent joins it —
// the same split the backup archive download uses. An unresolvable value comes
// back empty so the caller treats it as "no pull URL" rather than attempting a
// request with no scheme.
func (rt *computeRuntime) absoluteServerURL(value string) string {
	if value == "" || rt.resolveServerURL == nil {
		return ""
	}
	resolved, err := rt.resolveServerURL(value)
	if err != nil {
		rt.logger.Warn("compute: could not resolve server URL", "value", value, "error", err)
		return ""
	}
	return resolved
}

// releaseGuestTaps removes a stopped guest's interfaces.
//
// Nothing did this before: CleanupNICs only ran when a start failed, so a
// normally stopped guest left its taps behind and the next start hit EBUSY on
// a name that was still there. The start path recovers from that now, but a
// host should not accumulate dead interfaces in the meantime.
func (rt *computeRuntime) releaseGuestTaps(vmID string) {
	if removed := netconf.DeleteGuestTaps(vmID); removed > 0 {
		rt.logger.Debug("compute: released guest taps", "vm", vmID, "count", removed)
	}
	if removed := netconf.DeleteGuestDirect(vmID); removed > 0 {
		rt.logger.Debug("compute: released guest direct interfaces", "vm", vmID, "count", removed)
	}
}

// knownVMIDs extracts the server's list of VMs for reconciliation.
func knownVMIDs(req *VMPayload) []string {
	ids := make([]string, 0, len(req.States))
	for _, state := range req.States {
		ids = append(ids, state.VMID)
	}
	return ids
}

// runningVMIDs narrows a reconcile result to the guests that have a QEMU, and
// therefore a guest-agent socket worth dialling.
func runningVMIDs(states []vm.State) []string {
	ids := make([]string, 0, len(states))
	for _, state := range states {
		if state.State == vm.StateRunning || state.State == vm.StatePaused {
			ids = append(ids, state.VMID)
		}
	}
	return ids
}

// defineVM materializes a guest's runtime directory and its disks.
func (rt *computeRuntime) defineVM(ctx context.Context, req *VMPayload) error {
	if req.Spec == nil {
		return fmt.Errorf("compute: vm_define carried no spec")
	}
	spec := toVMSpec(req.VMID, req.Spec)
	if err := rt.applyFirmware(&spec); err != nil {
		return err
	}

	if err := rt.vms.Define(ctx, spec); err != nil {
		return err
	}
	if req.Spec.CloudInit != nil {
		if err := rt.writeSeed(spec, req.Spec.CloudInit); err != nil {
			return err
		}
	}
	return nil
}

// writeSeed builds the NoCloud seed image for a guest.
func (rt *computeRuntime) writeSeed(spec vm.Spec, cfg *VMCloudInitSpec) error {
	seed, err := cloudinit.Build(cloudinit.Config{
		InstanceID:            cfg.InstanceID,
		Hostname:              cfg.Hostname,
		FQDN:                  cfg.FQDN,
		Timezone:              cfg.Timezone,
		DefaultUser:           cfg.DefaultUser,
		SSHKeys:               cfg.SSHKeys,
		PasswordHash:          cfg.PasswordHash,
		NetworkMode:           cfg.NetworkMode,
		StaticIP:              cfg.StaticIP,
		StaticGateway:         cfg.StaticGateway,
		StaticDNS:             cfg.StaticDNS,
		RunCmd:                cfg.RunCmd,
		UserDataExtra:         cfg.UserDataExtra,
		InstallQEMUGuestAgent: true,
	})
	if err != nil {
		return err
	}
	paths := vm.PathsFor(spec.ID)
	if err := cloudinit.WriteSeedISO(paths.SeedISO, seed); err != nil {
		return err
	}
	// A new seed has not been delivered yet, whatever the last one was.
	return vm.ResetSeedDelivery(paths)
}

// applyFirmware resolves this guest's UEFI paths from what it actually asked
// for.
//
// A Secure Boot guest needs the SMM firmware build and a variable template with
// Microsoft's keys enrolled. Falling back to the plain build would boot a guest
// that reports Secure Boot as off — which is how Windows 11 came to refuse to
// install on a guest McHarbor showed as Secure Boot enabled.
func (rt *computeRuntime) applyFirmware(spec *vm.Spec) error {
	secure := spec.Firmware == "uefi" && spec.SecureBoot
	code, varsTemplate := rt.firmwarePaths(secure)
	spec.FirmwareCode = code
	spec.FirmwareVarsTemplate = varsTemplate

	if secure && (code == "" || varsTemplate == "") {
		return fmt.Errorf("compute: %s asks for Secure Boot but this host has no "+
			"secure-boot OVMF build with enrolled keys", spec.ID)
	}
	return nil
}

// startVM boots a defined guest.
func (rt *computeRuntime) startVM(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	return rt.startVMWith(ctx, req, reply, nil)
}

// startVMWith boots a guest, letting the caller adjust the runtime spec last.
// A live-migration target uses it to wait for the incoming state instead of
// booting, with exactly the source's SPICE setting.
func (rt *computeRuntime) startVMWith(ctx context.Context, req *VMPayload, reply *VMPayload, adjust func(*vm.Spec)) error {
	if req.Spec == nil {
		return fmt.Errorf("compute: vm_start carried no spec")
	}
	spec := toVMSpec(req.VMID, req.Spec)
	if err := rt.applyFirmware(&spec); err != nil {
		return err
	}

	arch := "x86_64"
	binary, err := rt.qemuBinaryFor(ctx, arch)
	if err != nil {
		return err
	}
	// Decided here rather than sent by McHarbor: only this host knows whether
	// its QEMU was built with SPICE, and asking for it on one that was not
	// stops the guest from starting at all.
	spec.SPICE = rt.spiceSupported(ctx)
	if adjust != nil {
		adjust(&spec)
	}

	// Starting a guest that is already up would take its taps out from under
	// it below, so it is refused here rather than half-performed.
	if states, _, err := rt.vms.Query(ctx, []string{spec.ID}); err == nil {
		for _, state := range states {
			if state.VMID == spec.ID && state.State == vm.StateRunning {
				return fmt.Errorf("compute: %s is already running", spec.ID)
			}
		}
	}

	// The interfaces have to exist before QEMU opens them. A failed start
	// removes them again so a guest that never came up leaves nothing behind.
	for i := range spec.NICs {
		if err := prepareGuestNIC(spec.ID, &spec.NICs[i], nicSpecFor(req.Spec, spec.NICs[i].Slot)); err != nil {
			vm.CleanupNICs(spec)
			return err
		}
	}

	if err := rt.vms.Start(ctx, spec, binary); err != nil {
		vm.CleanupNICs(spec)
		return err
	}

	reply.NICAddrs = rt.observeNICAddrs(ctx, spec)
	return nil
}

// prepareGuestNIC creates the host side of one NIC: a macvtap on a direct
// network, a tap on its bridge otherwise. Shared by start and by hot-plug, so a
// NIC plugged into a running guest is wired exactly as one present at boot.
func prepareGuestNIC(vmID string, nic *vm.NIC, wanted *VMNICSpec) error {
	if wanted != nil && netconf.IsDirectMode(wanted.AttachKind) {
		// The NIC carries the network's uplink, so the tag is resolved here
		// rather than at network create: stacking the guest on the untagged
		// adapter would put it on the native VLAN, where the DHCP server it
		// is looking for never hears it.
		parent, err := netconf.EnsureVLANParent(wanted.ParentLink, wanted.VLANTag)
		if err != nil {
			return err
		}
		_, device, err := netconf.CreateDirect(netconf.DirectSpec{
			Name:     netconf.DirectName(vmID, nic.Slot),
			Driver:   wanted.AttachKind,
			Parent:   parent,
			LinkMode: wanted.LinkMode,
			MAC:      nic.MAC,
			Tap:      true,
		})
		if err != nil {
			return err
		}
		// TapName is left empty deliberately: this interface has no name
		// QEMU can reopen, and the descriptor is the only handle.
		nic.TapName = ""
		nic.DevicePath = device
		return nil
	}

	if nic.TapName == "" {
		nic.TapName = netconf.TapName(vmID, nic.Slot)
	}
	bridge := ""
	if wanted != nil {
		bridge = wanted.Bridge
	}
	tapSpec := netconf.TapSpec{Name: nic.TapName, Bridge: bridge}
	if _, err := netconf.CreateTap(tapSpec); err != nil {
		// A tap outlives the guest that used it: stopping a guest tears
		// down QEMU but nothing removes its interfaces, so the next start
		// hits EBUSY on a name that is still there. CreateTap tolerates
		// EEXIST but not EBUSY, which is what a tap still held by the
		// exiting process reports. Clear it and try once more — safe because
		// both callers have established that no running QEMU holds this NIC.
		errlog.Log(netconf.DeleteLink(nic.TapName))
		if _, retryErr := netconf.CreateTap(tapSpec); retryErr != nil {
			return retryErr
		}
	}
	return nil
}

// observeNICAddrs reads the PCI slot QEMU gave each NIC, so the server can pin
// it on every later start.
//
// Best effort by design: a guest that started is started, and failing the call
// because a monitor query did not answer would be a worse outcome than a NIC
// whose address is recorded on the next start instead.
func (rt *computeRuntime) observeNICAddrs(ctx context.Context, spec vm.Spec) map[string]string {
	if len(spec.NICs) == 0 {
		return nil
	}

	monitor, err := rt.vms.Monitor(ctx, spec.ID)
	if err != nil {
		rt.logger.Warn("compute: no monitor to read NIC addresses from",
			"vm", spec.ID, "error", err)
		return nil
	}
	slots, err := monitor.PCISlots(ctx)
	if err != nil {
		rt.logger.Warn("compute: reading NIC PCI addresses failed",
			"vm", spec.ID, "error", err)
		return nil
	}

	addrs := make(map[string]string, len(spec.NICs))
	for _, nic := range spec.NICs {
		// The qdev id argv gives a NIC, which is what query-pci reports it by.
		if addr, ok := slots[fmt.Sprintf("nic%d", nic.Slot)]; ok && nic.ID != "" {
			addrs[nic.ID] = addr
		}
	}
	if len(addrs) == 0 {
		return nil
	}
	return addrs
}

// nicSpecFor finds the requested NIC for a slot.
//
// The runtime spec deliberately carries less than the request does — it holds
// what QEMU needs — so how the interface is attached has to be read back from
// the request rather than from the converted spec.
func nicSpecFor(spec *VMSpec, slot int) *VMNICSpec {
	if spec == nil {
		return nil
	}
	for index := range spec.NICs {
		if spec.NICs[index].Slot == slot {
			return &spec.NICs[index]
		}
	}
	return nil
}

// deleteVM removes a guest, and optionally its volumes.
//
// Progress is published as guest events rather than as replies to the request.
// Deleting a large volume is disk-bound and can run for minutes, and the server
// hands the operator a dialog they are told to close — so the report has to
// reach the list, not only the caller waiting on this one message.
func (rt *computeRuntime) deleteVM(ctx context.Context, req *VMPayload) error {
	rt.publishDeleteProgress(req.VMID, 0, "guest")
	if req.Spec != nil {
		vm.CleanupNICs(toVMSpec(req.VMID, req.Spec))
	}
	if err := rt.vms.Delete(ctx, req.VMID); err != nil {
		return err
	}
	if !req.DeleteVolumes || req.Spec == nil {
		rt.publishDeleteProgress(req.VMID, 100, "done")
		return nil
	}

	// Weighted by count rather than by size: the driver reports nothing while a
	// volume is being removed, so the honest unit is "disks finished", and a
	// bar that moves per disk beats one that invents a rate.
	// Failures are collected rather than logged and dropped. The operator
	// asked for these disks and typed the guest's name to confirm it; telling
	// them it worked while the volumes are still on the pool also deletes the
	// row that named them, which is the only handle anyone had on them.
	total := len(req.Spec.Disks)
	var failures []string
	for i, disk := range req.Spec.Disks {
		rt.publishDeleteProgress(req.VMID, percentOf(i, total), "disks")
		driver, err := rt.pools.Get(disk.PoolBackend)
		if err != nil {
			rt.logger.Warn("removing guest volume failed: no driver for its pool",
				"vm", req.VMID, "path", disk.VolumePath, "backend", disk.PoolBackend, "error", err)
			failures = append(failures, fmt.Sprintf("%s: %v", disk.VolumePath, err))
			continue
		}
		pool := storage.Pool{ID: disk.PoolID, Backend: disk.PoolBackend, Target: disk.PoolTarget}
		if err := driver.DeleteVolume(ctx, pool, disk.VolumePath); err != nil {
			rt.logger.Warn("removing guest volume failed",
				"vm", req.VMID, "path", disk.VolumePath, "error", err)
			failures = append(failures, fmt.Sprintf("%s: %v", disk.VolumePath, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("storage: the guest was removed but %d of its %d disks were not: %s",
			len(failures), total, strings.Join(failures, "; "))
	}
	rt.publishDeleteProgress(req.VMID, 100, "done")
	return nil
}

// percentOf reports how far through a countable job we are, leaving the last
// step to the caller so nothing claims 100% before it is finished.
func percentOf(done, total int) int {
	if total <= 0 {
		return 0
	}
	return done * 100 / total
}

// publishDeleteProgress reports a delete's progress through the guest event
// channel the server already subscribes to.
func (rt *computeRuntime) publishDeleteProgress(vmID string, percent int, step string) {
	rt.publishVMEvent(vm.Event{
		VMID: vmID,
		Kind: "delete_progress",
		At:   time.Now().UTC(),
		Detail: map[string]string{
			"percent": strconv.Itoa(percent),
			"step":    step,
		},
	})
}

// handleVMDisk allocates, grows or removes a guest volume.
func (rt *computeRuntime) handleVMDisk(ctx context.Context, messageType string, req *VMPayload, reply *VMPayload) error {
	if req.Disk == nil {
		return fmt.Errorf("compute: %s carried no disk", messageType)
	}
	disk := req.Disk

	driver, err := rt.pools.Get(disk.PoolBackend)
	if err != nil {
		return err
	}
	pool := storage.Pool{ID: disk.PoolID, Backend: disk.PoolBackend, Target: disk.PoolTarget}

	switch messageType {
	case MsgVMDiskCreate:
		volume, err := driver.CreateVolume(ctx, storage.CreateVolumeRequest{
			Pool:        pool,
			Name:        volumeNameFor(req.VMID, disk.Slot),
			Kind:        storage.VolumeDisk,
			Size:        disk.SizeBytes,
			BackingPath: disk.BackingPath,
			Format:      disk.Format,
			Sparse:      true,
		})
		if err != nil {
			return err
		}
		if disk.BackingPath != "" {
			if err := rt.backedVolumes.record(volume.Path); err != nil {
				rt.logger.Warn("compute: recording a backed volume failed; the images-in-use "+
					"check covers it once the guest starts", "volume", volume.Path, "error", err)
			}
		}
		reply.Disk = &VMDiskSpec{
			DiskID:     disk.DiskID,
			Slot:       disk.Slot,
			VolumePath: volume.Path,
			Format:     volume.Format,
			SizeBytes:  volume.SizeBytes,
		}
		return nil

	case MsgVMDiskResize:
		size, err := driver.ResizeVolume(ctx, pool, disk.VolumePath, disk.SizeBytes)
		if err != nil {
			return err
		}
		reply.Disk = &VMDiskSpec{DiskID: disk.DiskID, Slot: disk.Slot, SizeBytes: size}
		return nil

	case MsgVMDiskDelete:
		return driver.DeleteVolume(ctx, pool, disk.VolumePath)
	}
	return fmt.Errorf("compute: unknown disk operation %s", messageType)
}

// handleVMDiskList enumerates the disk images in one pool.
//
// It answers with everything the pool holds, attached or not: the agent has no
// view of which volumes belong to a guest, and inventing one here would mean
// two places deciding what "free" means.
func (rt *computeRuntime) handleVMDiskList(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	if req.Disk == nil {
		return fmt.Errorf("compute: %s carried no pool", MsgVMDiskList)
	}
	driver, err := rt.pools.Get(req.Disk.PoolBackend)
	if err != nil {
		return err
	}
	volumes, err := driver.ListVolumes(ctx, storage.Pool{
		ID:      req.Disk.PoolID,
		Backend: req.Disk.PoolBackend,
		Target:  req.Disk.PoolTarget,
	})
	if err != nil {
		return err
	}

	reply.Volumes = make([]VMVolumeInfo, 0, len(volumes))
	for _, volume := range volumes {
		reply.Volumes = append(reply.Volumes, VMVolumeInfo{
			Name:       volume.Name,
			VolumePath: volume.Path,
			Format:     volume.Format,
			SizeBytes:  volume.SizeBytes,
			IsBlock:    volume.IsBlock,
		})
	}
	return nil
}

// maxVMLogTail bounds one log read. A guest that has been restarting for a week
// has a journal nobody wants delivered over a WebSocket in one frame.
const maxVMLogTail = 2000

// handleVMLog tails the systemd units McHarbor itself created for a guest.
//
// The unit name is built here from the guest id, never taken from the request:
// this runs as root, and accepting a caller-supplied unit would turn a log
// viewer into "read any journal on the host". The id is checked against the
// same character set the supervisor uses before it reaches a command line.
func (rt *computeRuntime) handleVMLog(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	if err := validateGuestID(req.VMID); err != nil {
		return err
	}

	var unit string
	switch req.LogSource {
	case "", "vm":
		unit = supervisor.VMUnitName(req.VMID)
	case "tpm":
		unit = vm.TPMUnitName(req.VMID)
	default:
		return fmt.Errorf("compute: %q is not a guest log source", req.LogSource)
	}

	tail := req.LogTail
	if tail <= 0 || tail > maxVMLogTail {
		tail = 200
	}

	lines, notices, ok := readJournalLogs(ctx,
		fmt.Sprintf("journalctl -u %s -n %d --no-pager -o short-iso", unit, tail), tail)
	if !ok && len(notices) == 0 {
		notices = []string{"journalctl_failed"}
	}
	reply.LogLines = lines
	reply.LogNotices = notices
	return nil
}

// toVMSpec converts a protocol spec into the manager's own.
func toVMSpec(vmID string, in *VMSpec) vm.Spec {
	out := vm.Spec{
		ID:             vmID,
		Name:           in.Name,
		UUID:           in.UUID,
		GuestOSFamily:  in.GuestOSFamily,
		GuestOSVariant: in.GuestOSVariant,
		VCPUs:          in.VCPUs,
		CPUModel:       in.CPUModel,
		CPUSockets:     in.CPUSockets,
		CPUCores:       in.CPUCores,
		CPUThreads:     in.CPUThreads,
		MemoryMB:       in.MemoryMB,
		MemoryMaxMB:    in.MemoryMaxMB,
		MachineType:    in.MachineType,
		Firmware:       in.Firmware,
		SecureBoot:     in.SecureBoot,
		TPMEnabled:     in.TPMEnabled,
		DisplayType:    in.DisplayType,
		DisplayWidth:   in.DisplayWidth,
		DisplayHeight:  in.DisplayHeight,
		ISOPaths:       in.ISOPaths,
		VCPUsMax:       in.VCPUsMax,
		VirtioMem:      in.VirtioMem,
		HugePages:      in.HugePages,
		NUMAPolicy:     in.NUMAPolicy,
		NUMAHostNodes:  in.NUMAHostNodes,
		VSockCID:       in.VSockCID,
		IOThread:       in.IOThread,
	}
	if in.SpiceOptions != nil {
		out.SpiceOptions = vm.SpiceOptions{
			ImageCompression:      in.SpiceOptions.ImageCompression,
			JPEGWANCompression:    in.SpiceOptions.JPEGWANCompression,
			ZlibGLZWANCompression: in.SpiceOptions.ZlibGLZWANCompression,
			StreamingVideo:        in.SpiceOptions.StreamingVideo,
			PlaybackCompression:   in.SpiceOptions.PlaybackCompression,
			AgentMouse:            in.SpiceOptions.AgentMouse,
		}
	}
	for _, share := range in.Shares {
		out.Shares = append(out.Shares, vm.Share{
			ID:       share.ShareID,
			Tag:      share.Tag,
			HostPath: share.HostPath,
			ReadOnly: share.ReadOnly,
			Socket:   vm.ShareSocketPath(vmID, share.ShareID),
		})
	}
	for _, disk := range in.Disks {
		out.Disks = append(out.Disks, vm.Disk{
			ID:          disk.DiskID,
			Slot:        disk.Slot,
			Bus:         disk.Bus,
			Path:        disk.VolumePath,
			Format:      disk.Format,
			ReadOnly:    disk.ReadOnly,
			IsBoot:      disk.IsBoot,
			IsCDROM:     disk.IsCDROM,
			Discard:     disk.Discard,
			CacheMode:   disk.CacheMode,
			SizeBytes:   disk.SizeBytes,
			BackingPath: disk.BackingPath,
			BootIndex:   disk.BootIndex,
			ShareRW:     disk.ShareRW,
			ReadIOPS:    disk.ReadIOPS,
			WriteIOPS:   disk.WriteIOPS,
			ReadBPS:     disk.ReadBPS,
			WriteBPS:    disk.WriteBPS,
		})
	}
	for _, nic := range in.NICs {
		out.NICs = append(out.NICs, vm.NIC{
			ID:       nic.NICID,
			Slot:     nic.Slot,
			Model:    nic.Model,
			MAC:      nic.MAC,
			TapName:  nic.TapName,
			PCIAddr:  nic.PCIAddr,
			LinkDown: nic.LinkDown,
		})
	}
	for _, device := range in.Devices {
		out.Devices = append(out.Devices, vm.Device{
			ID:          device.DeviceID,
			Kind:        device.Kind,
			HostAddress: device.HostAddress,
		})
	}
	return out
}

// handleCT answers the system container family.
func (rt *computeRuntime) handleCT(ctx context.Context, msg WSMessage) {
	req := msg.CT
	reply := &CTPayload{Operation: msg.Type, EnvID: req.EnvID, ContainerID: req.ContainerID}

	var err error
	switch msg.Type {
	case MsgCTCreate:
		err = rt.createCT(ctx, req)

	case MsgCTStart:
		if req.Spec == nil {
			err = fmt.Errorf("compute: ct_start carried no spec")
			break
		}
		var pid int
		pid, err = rt.cts.Start(ctx, toCTSpec(req.ContainerID, req.Spec))
		if err == nil {
			reply.State = &CTState{ContainerID: req.ContainerID, State: lxc.StateRunning, InitPID: pid}
		}

	case MsgCTShutdown:
		initSystem := lxc.InitSystemd
		if req.Spec != nil && req.Spec.InitSystem != "" {
			initSystem = req.Spec.InitSystem
		}
		var escalated bool
		escalated, err = rt.cts.Shutdown(ctx, req.ContainerID, initSystem,
			time.Duration(req.GraceSeconds)*time.Second)
		if escalated {
			reply.ErrorCode = "escalated_to_hard_stop"
		}

	case MsgCTStop:
		err = rt.cts.Stop(ctx, req.ContainerID)

	case MsgCTRestart:
		// Refused before the shutdown: without a spec there is nothing to start,
		// and stopping anyway would answer success for a container left down.
		if req.Spec == nil {
			err = fmt.Errorf("compute: ct_restart carried no spec")
			break
		}
		initSystem := lxc.InitSystemd
		if req.Spec.InitSystem != "" {
			initSystem = req.Spec.InitSystem
		}
		if _, err = rt.cts.Shutdown(ctx, req.ContainerID, initSystem,
			time.Duration(req.GraceSeconds)*time.Second); err == nil {
			var pid int
			pid, err = rt.cts.Start(ctx, toCTSpec(req.ContainerID, req.Spec))
			if err == nil {
				reply.State = &CTState{ContainerID: req.ContainerID, State: lxc.StateRunning, InitPID: pid}
			}
		}

	case MsgCTFreeze:
		err = rt.cts.Freeze(ctx, req.ContainerID)

	case MsgCTUnfreeze:
		err = rt.cts.Unfreeze(ctx, req.ContainerID)

	case MsgCTDelete:
		err = rt.deleteCT(ctx, req)

	case MsgCTQuery:
		// A stats request rides ct_query with an operation, as the plugin sends
		// it; answering it as a reconcile left Stats nil and read as "not running".
		if req.Operation == ctQueryStats {
			var sample lxc.Stats
			sample, err = rt.cts.Sample(ctx, req.ContainerID)
			if err == nil {
				reply.Stats = toProtocolCTStats(sample)
			}
			break
		}
		var states []lxc.State
		var orphans []string
		states, orphans, err = rt.cts.Query(ctx, knownCTIDs(req))
		for _, state := range states {
			reply.States = append(reply.States, *toProtocolCTState(state))
		}
		reply.Orphans = orphans

	case MsgCTProcesses:
		var procs []lxc.Process
		procs, err = rt.cts.Processes(ctx, req.ContainerID)
		for _, proc := range procs {
			reply.Processes = append(reply.Processes, CTProcess{
				PID:         proc.PID,
				HostPID:     proc.HostPID,
				PPID:        proc.PPID,
				User:        proc.User,
				Command:     proc.Command,
				MemoryBytes: proc.MemoryBytes,
			})
		}

	case MsgCTUpdateLimits:
		if req.Spec == nil {
			err = fmt.Errorf("compute: ct_update_limits carried no spec")
			break
		}
		var applied lxc.Limits
		applied, err = rt.cts.UpdateLimits(ctx, toCTSpec(req.ContainerID, req.Spec))
		if err == nil {
			reply.Applied = toProtocolLimits(applied)
		}

	case MsgCTBackupRun:
		err = rt.handleCTBackupRun(ctx, req, reply)
		// Its own result type rather than ct_result: the server's backup path
		// waits on this one, and sharing the lifecycle reply would put a
		// twelve-hour run in the same waiter slot as a one-second freeze.
		if err != nil {
			reply.Error = err.Error()
		}
		errlog.Log(rt.send(WSMessage{Type: MsgCTBackupResult, ID: msg.ID, CT: reply}))
		return

	case MsgCTBackupRestore:
		err = rt.handleCTBackupRestore(ctx, req, reply)
		if err != nil {
			reply.Error = err.Error()
		}
		errlog.Log(rt.send(WSMessage{Type: MsgCTBackupResult, ID: msg.ID, CT: reply}))
		return

	case MsgCTCancel:
		reply.Success = rt.cts.Cancel(req.ContainerID)
		errlog.Log(rt.send(WSMessage{Type: MsgCTResult, ID: msg.ID, CT: reply}))
		return

	case MsgCTSnapshotCreate, MsgCTSnapshotRevert, MsgCTSnapshotDelete:
		err = rt.handleCTSnapshot(ctx, msg.Type, req, reply)

	case MsgCTCloneRequest:
		err = rt.handleCTClone(ctx, req, reply)

	default:
		// Anything else is from a newer McHarbor; refuse it rather than stay
		// silent and hang the caller.
		err = fmt.Errorf("compute: %s is not a message this agent understands", msg.Type)
	}

	if err != nil {
		reply.Error = err.Error()
	} else {
		reply.Success = true
	}
	errlog.Log(rt.send(WSMessage{Type: MsgCTResult, ID: msg.ID, CT: reply}))
}

func knownCTIDs(req *CTPayload) []string {
	ids := make([]string, 0, len(req.States))
	for _, state := range req.States {
		ids = append(ids, state.ContainerID)
	}
	return ids
}

// toCTSpec converts a protocol spec into the manager's own.
func toCTSpec(ctID string, in *CTSpec) lxc.Spec {
	out := lxc.Spec{
		ID:          ctID,
		Name:        in.Name,
		RootfsPath:  in.RootfsPath,
		PoolBackend: in.PoolBackend,
		InitSystem:  in.InitSystem,
		InitArgv:    in.InitArgv,
		Privileged:  in.Privileged,
		Nesting:     in.Nesting,
		UIDMapStart: in.UIDMapStart,
		UIDMapCount: in.UIDMapCount,
		GIDMapStart: in.GIDMapStart,
		GIDMapCount: in.GIDMapCount,
		IDMapMech:   in.IDMapMech,
	}
	if in.Limits != nil {
		out.Limits = lxc.Limits{
			CPUMax:          in.Limits.CPUMax,
			CPUWeight:       uint64(in.Limits.CPUWeight),
			CPUSet:          in.Limits.CPUSet,
			MemoryMaxBytes:  in.Limits.MemoryMaxBytes,
			MemoryHighBytes: in.Limits.MemoryHighBytes,
			SwapMaxBytes:    in.Limits.SwapMaxBytes,
			PidsMax:         in.Limits.PidsMax,
			IOMax:           in.Limits.IOMax,
		}
	}
	for _, nic := range in.NICs {
		out.NICs = append(out.NICs, lxc.NIC{
			ID:          nic.NICID,
			Slot:        nic.Slot,
			Bridge:      nic.Bridge,
			VLANTag:     nic.VLANTag,
			AttachKind:  nic.AttachKind,
			ParentLink:  nic.ParentLink,
			LinkMode:    nic.LinkMode,
			GuestName:   nic.GuestName,
			MAC:         nic.MAC,
			VethHost:    nic.VethHost,
			AddressMode: nic.AddressMode,
			StaticIP:    nic.StaticIP,
			StaticGW:    nic.StaticGW,
			StaticDNS:   nic.StaticDNS,
			StaticIPv6:  nic.StaticIPv6,
			StaticGW6:   nic.StaticGW6,
		})
	}
	for _, mount := range in.Mounts {
		out.Mounts = append(out.Mounts, lxc.Mount{
			ID:          mount.MountID,
			Kind:        mount.Kind,
			Source:      mount.Source,
			Destination: mount.Destination,
			ReadOnly:    mount.ReadOnly,
			ShiftIDs:    mount.ShiftIDs,
		})
	}
	return out
}

func toProtocolLimits(in lxc.Limits) *CTLimits {
	return &CTLimits{
		CPUMax:          in.CPUMax,
		CPUWeight:       int(in.CPUWeight),
		CPUSet:          in.CPUSet,
		MemoryMaxBytes:  in.MemoryMaxBytes,
		MemoryHighBytes: in.MemoryHighBytes,
		SwapMaxBytes:    in.SwapMaxBytes,
		PidsMax:         in.PidsMax,
		IOMax:           in.IOMax,
	}
}

// ctQueryStats is the ct_query operation that asks for one container's sample
// rather than a reconcile of every container.
const ctQueryStats = "stats"
