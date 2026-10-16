// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"
)

// PluginSlot names the one WSMessage field a plugin-routable message travels in. The host
// decodes a plugin's payload into that field's struct and nothing else, so a plugin cannot
// populate a second envelope field the agent would also act on.
type PluginSlot string

// The envelope slots a plugin-routable or observable message may use.
const (
	// SlotVM carries a VMPayload.
	SlotVM PluginSlot = "vm"
	// SlotCT carries a CTPayload.
	SlotCT PluginSlot = "ct"
	// SlotNet carries a ComputeNetPayload.
	SlotNet PluginSlot = "net"
	// SlotCompute carries a ComputePayload: the capability probe, the compute locations and
	// a relocation, and the relocation's progress frames.
	SlotCompute PluginSlot = "compute"
	// SlotPool carries a ComputePoolPayload.
	SlotPool PluginSlot = "pool"
	// SlotComputeImage carries a ComputeImagePayload.
	SlotComputeImage PluginSlot = "compute_image"
)

// PluginBudget is a time-budget class. A plugin names a message type, never a timeout, so a
// plugin cannot hold a root-privileged agent call open longer than the host allows.
type PluginBudget string

// The budget classes, each bound to one of the transport's existing timeouts.
const (
	// BudgetQuick is the ordinary round trip, answered from local state or a short syscall.
	BudgetQuick PluginBudget = "quick"
	// BudgetLong is for agent-side work that legitimately takes minutes.
	BudgetLong PluginBudget = "long"
	// BudgetTransfer is bounded by a disk crossing the wire rather than by agent work.
	BudgetTransfer PluginBudget = "transfer"
	// BudgetRelocate bounds a compute relocation: an apply copies a host's guests across
	// filesystems, while a plan or a repair only walks the trees and takes the quick budget.
	BudgetRelocate PluginBudget = "relocate"
)

// Timeout returns the duration a budget class stands for; an unknown class gets the quick one.
func (b PluginBudget) Timeout() time.Duration {
	switch b {
	case BudgetLong:
		return computeLongTimeout
	case BudgetTransfer:
		return computeImportTimeout
	case BudgetRelocate:
		return computeMigrateTimeout
	default:
		return computeTimeout
	}
}

// PluginFloor names a version floor a message must clear before it is sent. An older agent
// has no default branch and drops an unknown message, so the caller would wait out the
// whole budget for a reply that never comes; the floor refuses first instead.
type PluginFloor string

// The version floors a plugin-routable message may carry. The empty floor means none.
const (
	// FloorNone applies no version check beyond the compute floor every call clears.
	FloorNone PluginFloor = ""
	// FloorFreeVolume is FreeVolumeMinVersion, for vm_disk_list and vm_log.
	FloorFreeVolume PluginFloor = "free_volume"
	// FloorCTBackup is CTBackupMinVersion plus its build stamp.
	FloorCTBackup PluginFloor = "ct_backup"
	// FloorCTRootfs is CTRootfsMinVersion plus its build stamp, only for a create that
	// builds a rootfs or a delete that removes one.
	FloorCTRootfs PluginFloor = "ct_rootfs"
	// FloorGuestNetworking is GuestNetworkingMinVersion, for any network mode but bridged.
	FloorGuestNetworking PluginFloor = "guest_networking"
	// FloorImageLibrary is ImageLibraryMinVersion, only for a fetch that adopts an uploaded
	// library image rather than dialling a mirror.
	FloorImageLibrary   PluginFloor = "image_library"
	FloorPackageInstall PluginFloor = "package_install"
)

// Check reports whether conn clears the floor for this message. The error wraps
// ErrComputeUnsupported and says "needs agent" or names the version, because addon-lxc
// matches on that wording to tell an outdated agent from an offline one.
func (f PluginFloor) Check(conn *AgentConnection, msgType string, payload []byte) error {
	switch f {
	case FloorFreeVolume:
		if conn == nil || !conn.SupportsFeature(FreeVolumeMinVersion) {
			return fmt.Errorf("%w: %s needs agent %s or newer",
				ErrComputeUnsupported, msgType, FreeVolumeMinVersion)
		}
	case FloorCTBackup:
		if conn == nil || !clearsBuildFloor(conn, CTBackupMinVersion, CTBackupMinBuild) {
			return fmt.Errorf("%w: %s needs agent %s build %s or newer",
				ErrComputeUnsupported, msgType, CTBackupMinVersion, CTBackupMinBuild)
		}
	case FloorCTRootfs:
		if !buildsOrRemovesRootfs(msgType, payload) {
			return nil
		}
		if conn == nil || !clearsBuildFloor(conn, CTRootfsMinVersion, CTRootfsMinBuild) {
			return fmt.Errorf("%w: %s needs agent %s build %s or newer",
				ErrComputeUnsupported, msgType, CTRootfsMinVersion, CTRootfsMinBuild)
		}
	case FloorGuestNetworking:
		// A payload that does not decode is left to the decode step, which names it.
		if mode, ok := networkMode(payload); ok {
			return guestNetworkingFloor(conn, mode)
		}
	case FloorPackageInstall:
		if conn == nil || !clearsBuildFloor(conn, PackageInstallMinVersion, PackageInstallMinBuild) {
			return fmt.Errorf("%w: %s needs agent %s build %s or newer",
				ErrComputeUnsupported, msgType, PackageInstallMinVersion, PackageInstallMinBuild)
		}
	case FloorImageLibrary:
		var probe struct {
			FromLibrary bool `json:"fromLibrary"`
		}
		if len(payload) > 0 && json.Unmarshal(payload, &probe) == nil {
			return imageLibraryFloor(conn, probe.FromLibrary)
		}
	}
	return nil
}

// imageLibraryFloor is the one check ImageFetch and the plugin route share. Its wording is the
// typed fetch's, which never said "needs agent"; nothing matches on it.
func imageLibraryFloor(conn *AgentConnection, fromLibrary bool) error {
	if !fromLibrary || (conn != nil && conn.SupportsFeature(ImageLibraryMinVersion)) {
		return nil
	}
	version := ""
	if conn != nil {
		version = conn.Version
	}
	return fmt.Errorf("%w: this agent is %s and the shared image library needs %s or newer",
		ErrComputeUnsupported, version, ImageLibraryMinVersion)
}

// networkMode reads the mode a net payload asks for; ok is false when it does not decode.
func networkMode(payload []byte) (string, bool) {
	var probe struct {
		Mode string `json:"mode"`
	}
	if len(payload) > 0 && json.Unmarshal(payload, &probe) != nil {
		return "", false
	}
	return probe.Mode, true
}

// guestNetworkingFloor is the one check NetEnsure and the plugin route share, so the typed
// call and the raw one cannot disagree about which agents can build a network.
func guestNetworkingFloor(conn *AgentConnection, mode string) error {
	if mode == "bridged" || (conn != nil && conn.SupportsFeature(GuestNetworkingMinVersion)) {
		return nil
	}
	version := ""
	if conn != nil {
		version = conn.Version
	}
	return fmt.Errorf("%w: this agent is %s and managed or shared networks need %s or newer",
		ErrComputeUnsupported, version, GuestNetworkingMinVersion)
}

// clearsBuildFloor reads the connection through its accessors rather than its fields, which
// another goroutine writes on every reconnect. Only an agent at exactly the floor version
// needs its build stamp compared.
func clearsBuildFloor(conn *AgentConnection, version, build string) bool {
	if !conn.SupportsFeature(version) {
		return false
	}
	if !conn.VersionEquals(version) {
		return true
	}
	return conn.BuildAtLeast(build)
}

// buildsOrRemovesRootfs reports whether a ct command asks the agent to build a rootfs from
// an image or template, or to remove one. A create over a rootfs a restore already laid
// down works on every agent, so only these are gated.
func buildsOrRemovesRootfs(msgType string, payload []byte) bool {
	if msgType != MsgCTCreate && msgType != MsgCTDelete {
		return false
	}
	var probe struct {
		ImageID      string `json:"imageId"`
		DeleteRootfs bool   `json:"deleteRootfs"`
		Spec         *struct {
			TemplatePath string `json:"templatePath"`
		} `json:"spec"`
	}
	if len(payload) == 0 || json.Unmarshal(payload, &probe) != nil {
		return false
	}
	if msgType == MsgCTDelete {
		return probe.DeleteRootfs
	}
	return probe.ImageID != "" || (probe.Spec != nil && probe.Spec.TemplatePath != "")
}

// PluginRoute is one message type a plugin may ask the host to send to an agent.
type PluginRoute struct {
	// Type is the wire message type.
	Type string
	// Slot is the only envelope field the payload is decoded into.
	Slot PluginSlot
	// Budget bounds the round trip.
	Budget PluginBudget
	// Floor is checked against the connection before anything is written.
	Floor PluginFloor
	// Progress lists the frames the agent may stream back under the request's id.
	Progress []string
	// Operation is written into the payload's operation field by the host, for a message
	// whose operation is fixed; empty leaves the plugin's, which the route then checks.
	Operation string
	// CancelWith is the message the host sends under the request's id when the caller gives
	// up first, so the agent stops work nobody is waiting for. A plugin never sends it.
	CancelWith string
}

// poolFileOperations are the operations a pool_file_request may carry, the typed PoolFile's
// set. The agent confines every path to the pool; this only keeps the verb vocabulary closed.
var poolFileOperations = []string{"read", "write", "mkdir", "delete", "rename"}

// PluginObserved is one unsolicited frame type a plugin may subscribe to.
type PluginObserved struct {
	// Type is the wire message type.
	Type string
	// Slot is the envelope field whose payload the subscriber receives.
	Slot PluginSlot
}

// ErrNotPluginRoutable means a message type is outside the host's ceiling. A plugin must
// never make the host send an arbitrary message type to a root-privileged agent.
var ErrNotPluginRoutable = errors.New("not a plugin-routable agent message")

// pluginRoutes is the ceiling: the closed, host-owned set a plugin's agent-rpc declaration
// may narrow and never widen. It is the guest-command allow-list the retired typed compute
// bridge enforced (vm_*, ct_*, net ensure/remove) plus every message that bridge sent itself
// (probe, locations, relocation, pools, pool files, link detection, the image cache), each
// with the budget, floor, progress frames and cancellation it applied.
var pluginRoutes = func() map[string]PluginRoute {
	quick, long, transfer, relocate := BudgetQuick, BudgetLong, BudgetTransfer, BudgetRelocate
	entries := []PluginRoute{
		{Type: MsgVMDefine, Slot: SlotVM, Budget: long},
		{Type: MsgVMStart, Slot: SlotVM, Budget: quick},
		{Type: MsgVMShutdown, Slot: SlotVM, Budget: quick},
		{Type: MsgVMStop, Slot: SlotVM, Budget: quick},
		{Type: MsgVMReset, Slot: SlotVM, Budget: quick},
		{Type: MsgVMPause, Slot: SlotVM, Budget: quick},
		{Type: MsgVMResume, Slot: SlotVM, Budget: quick},
		{Type: MsgVMDelete, Slot: SlotVM, Budget: long},
		{Type: MsgVMQuery, Slot: SlotVM, Budget: quick},
		{Type: MsgVMStats, Slot: SlotVM, Budget: quick},
		{Type: MsgHostDeviceList, Slot: SlotVM, Budget: quick},
		{Type: MsgVMLog, Slot: SlotVM, Budget: quick, Floor: FloorFreeVolume},
		{Type: MsgVMDiskList, Slot: SlotVM, Budget: quick, Floor: FloorFreeVolume},
		{Type: MsgVMDiskCreate, Slot: SlotVM, Budget: long},
		{Type: MsgVMDiskResize, Slot: SlotVM, Budget: long},
		{Type: MsgVMDiskDelete, Slot: SlotVM, Budget: quick},
		{Type: MsgVMSnapshotCreate, Slot: SlotVM, Budget: long},
		{Type: MsgVMSnapshotRevert, Slot: SlotVM, Budget: long},
		{Type: MsgVMSnapshotDelete, Slot: SlotVM, Budget: quick},
		{Type: MsgVMCloneRequest, Slot: SlotVM, Budget: long, Progress: []string{MsgVMCloneProgress}},
		{Type: MsgVMImportInspect, Slot: SlotVM, Budget: transfer},
		{Type: MsgVMImportConvert, Slot: SlotVM, Budget: transfer},
		{Type: MsgVMImportStage, Slot: SlotVM, Budget: transfer},
		{Type: MsgVMImportDiscard, Slot: SlotVM, Budget: quick},
		{Type: MsgVMGuestQuery, Slot: SlotVM, Budget: quick},
		{Type: MsgVMGuestAgentDisc, Slot: SlotVM, Budget: quick},
		{Type: MsgVMBackupRun, Slot: SlotVM, Budget: transfer},
		{Type: MsgVMBackupRestore, Slot: SlotVM, Budget: transfer},
		{Type: MsgVMQEMUCapability, Slot: SlotVM, Budget: quick},
		{Type: MsgVMMemorySet, Slot: SlotVM, Budget: long},
		{Type: MsgVMCPUSet, Slot: SlotVM, Budget: long},
		{Type: MsgVMThrottleSet, Slot: SlotVM, Budget: quick},
		// Live hardware changes. A disk resize may first grow an LV or a zvol,
		// so it takes the long budget; the disc and NIC changes are QMP only, and
		// an unplug waits at most 30 s for the guest inside the quick one.
		{Type: MsgVMDiskHotplug, Slot: SlotVM, Budget: long},
		{Type: MsgVMMediaChange, Slot: SlotVM, Budget: quick},
		{Type: MsgVMNICHotplug, Slot: SlotVM, Budget: quick},
		{Type: MsgVMImageInspect, Slot: SlotVM, Budget: long},
		{Type: MsgVMMigrate, Slot: SlotVM, Budget: transfer, Progress: []string{MsgVMMigrateProgress}},
		{Type: MsgVMCancel, Slot: SlotVM, Budget: quick},
		{Type: MsgVMSpiceTicket, Slot: SlotVM, Budget: quick},

		{Type: MsgCTCreate, Slot: SlotCT, Budget: long, Floor: FloorCTRootfs},
		{Type: MsgCTStart, Slot: SlotCT, Budget: quick},
		{Type: MsgCTShutdown, Slot: SlotCT, Budget: quick},
		{Type: MsgCTStop, Slot: SlotCT, Budget: quick},
		{Type: MsgCTRestart, Slot: SlotCT, Budget: quick},
		{Type: MsgCTFreeze, Slot: SlotCT, Budget: quick},
		{Type: MsgCTUnfreeze, Slot: SlotCT, Budget: quick},
		{Type: MsgCTDelete, Slot: SlotCT, Budget: long, Floor: FloorCTRootfs},
		{Type: MsgCTQuery, Slot: SlotCT, Budget: quick},
		{Type: MsgCTProcesses, Slot: SlotCT, Budget: quick},
		{Type: MsgCTUpdateLimits, Slot: SlotCT, Budget: quick},
		{Type: MsgCTSnapshotCreate, Slot: SlotCT, Budget: long},
		{Type: MsgCTSnapshotRevert, Slot: SlotCT, Budget: long},
		{Type: MsgCTSnapshotDelete, Slot: SlotCT, Budget: quick},
		{Type: MsgCTCloneRequest, Slot: SlotCT, Budget: long, Progress: []string{MsgCTCloneProgress}},
		{Type: MsgCTCancel, Slot: SlotCT, Budget: quick},
		{Type: MsgCTBackupRun, Slot: SlotCT, Budget: transfer, Floor: FloorCTBackup},
		{Type: MsgCTBackupRestore, Slot: SlotCT, Budget: transfer, Floor: FloorCTBackup},

		// NetEnsure owns its own long budget and the external-switch confirmation.
		{Type: MsgNetEnsureRequest, Slot: SlotNet, Budget: long, Floor: FloorGuestNetworking},
		{Type: MsgNetRemoveRequest, Slot: SlotNet, Budget: quick},
		{Type: MsgNetDetectRequest, Slot: SlotNet, Budget: quick, Operation: "detect"},

		{Type: MsgComputeProbeRequest, Slot: SlotCompute, Budget: quick, Operation: "probe"},
		{Type: MsgComputeConfigRequest, Slot: SlotCompute, Budget: quick, Operation: "config"},
		{Type: MsgComputeInstallRequest, Slot: SlotCompute, Budget: long, Operation: "install", Floor: FloorPackageInstall},
		// A relocation's progress carries no request id: it is the observed
		// compute_migrate_progress frame, not a Progress entry here.
		{Type: MsgComputeMigrateRequest, Slot: SlotCompute, Budget: relocate, Operation: "migrate"},

		{Type: MsgPoolDetectRequest, Slot: SlotPool, Budget: quick, Operation: "detect"},
		{Type: MsgPoolCreateRequest, Slot: SlotPool, Budget: long, Operation: "create"},
		{Type: MsgPoolStatsRequest, Slot: SlotPool, Budget: quick, Operation: "stats"},
		{Type: MsgPoolRemoveRequest, Slot: SlotPool, Budget: long, Operation: "remove"},
		{Type: MsgPoolBrowseRequest, Slot: SlotPool, Budget: quick, Operation: "browse"},
		// One whole small file per message, as the typed PoolFile always sent it; a large
		// file goes over the HTTP proxy, never through this route.
		{Type: MsgPoolFileRequest, Slot: SlotPool, Budget: quick},

		{Type: MsgImageFetchRequest, Slot: SlotComputeImage, Budget: transfer, Floor: FloorImageLibrary,
			Operation: "fetch", Progress: []string{MsgImageFetchProgress}, CancelWith: MsgImageCancelRequest},
		{Type: MsgImageListRequest, Slot: SlotComputeImage, Budget: quick, Operation: "list"},
		{Type: MsgImageRemoveRequest, Slot: SlotComputeImage, Budget: quick, Operation: "remove"},
	}
	out := make(map[string]PluginRoute, len(entries))
	for _, e := range entries {
		out[e.Type] = e
	}
	return out
}()

// pluginObserved is the set of unsolicited frames a plugin may subscribe to: exactly what
// dispatchComputeEvent hands the plugin frame observer.
var pluginObserved = map[string]PluginObserved{
	MsgVMEvent:                {Type: MsgVMEvent, Slot: SlotVM},
	MsgVMImportProgress:       {Type: MsgVMImportProgress, Slot: SlotVM},
	MsgCTBackupProgress:       {Type: MsgCTBackupProgress, Slot: SlotCT},
	MsgNetLeaseEvent:          {Type: MsgNetLeaseEvent, Slot: SlotNet},
	MsgComputeMigrateProgress: {Type: MsgComputeMigrateProgress, Slot: SlotCompute},
}

// PluginRouteFor returns the ceiling entry for msgType, and false when no plugin may send it.
func PluginRouteFor(msgType string) (PluginRoute, bool) {
	r, ok := pluginRoutes[msgType]
	if ok {
		r.Progress = slices.Clone(r.Progress)
	}
	return r, ok
}

// PluginRoutes returns the whole ceiling sorted by type; the copy keeps callers from
// widening it.
func PluginRoutes() []PluginRoute {
	out := make([]PluginRoute, 0, len(pluginRoutes))
	for _, r := range pluginRoutes {
		r.Progress = slices.Clone(r.Progress)
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// pluginProgressFrames is every frame a ceiling route may stream back under its request id,
// read from the routes' own Progress lists so no second list can drift from them.
var pluginProgressFrames = func() map[string]bool {
	out := map[string]bool{}
	for _, r := range pluginRoutes {
		for _, frame := range r.Progress {
			out[frame] = true
		}
	}
	return out
}()

// IsPluginProgressFrame reports whether msgType is a progress frame some plugin route streams
// back. A relay may drop one for a slow reader, because the local caller drops it too.
func IsPluginProgressFrame(msgType string) bool {
	return pluginProgressFrames[msgType]
}

// PluginObservedFor returns the observe entry for msgType, and false when no plugin may
// subscribe to it.
func PluginObservedFor(msgType string) (PluginObserved, bool) {
	o, ok := pluginObserved[msgType]
	return o, ok
}

// PluginObservable returns every observable frame type, sorted.
func PluginObservable() []PluginObserved {
	out := make([]PluginObserved, 0, len(pluginObserved))
	for _, o := range pluginObserved {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// slotPayload is the envelope field slot names on msg, JSON-encoded, or nil when the frame
// carries nothing there. A subscriber sees that field and never the rest of the envelope.
func slotPayload(msg *WSMessage, slot PluginSlot) ([]byte, error) {
	var v any
	switch slot {
	case SlotVM:
		if msg.VM != nil {
			v = msg.VM
		}
	case SlotCT:
		if msg.CT != nil {
			v = msg.CT
		}
	case SlotNet:
		if msg.Net != nil {
			v = msg.Net
		}
	case SlotCompute:
		if msg.Compute != nil {
			v = msg.Compute
		}
	case SlotPool:
		if msg.Pool != nil {
			v = msg.Pool
		}
	case SlotComputeImage:
		if msg.ComputeImage != nil {
			v = msg.ComputeImage
		}
	}
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
