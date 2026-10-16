// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

// The agent frames this plugin decodes that are not part of its vm payload.
//
// The plugin reaches the agent through the host's agent-rpc bridge, which carries one envelope
// slot as JSON. These structs hold only the fields this plugin reads; the request side stays
// in agentPayload (service.go) and migrate_wire.go. addon-compute and addon-lxc keep their own
// copies of what they need, which is the accepted duplication family addon-agent-wire.
//
// src/contracts/agentwire/slots holds fully populated fixtures generated from the host's
// protocol structs, and agent_bridge_test.go decodes them into these, so a renamed or dropped
// wire field fails a test instead of silently decoding to zero.

// Message types that are not guest commands: the capability probe this plugin makes before it
// creates a guest, and the unsolicited frames it observes.
const (
	msgComputeProbe = "compute_probe_request"

	msgVMEvent          = "vm_event"
	msgVMImportProgress = "vm_import_progress"
)

// sentMessageTypes is every message type this plugin sends, and so exactly what plugin.yaml
// declares under agent-rpc send. The host refuses anything outside the declaration, and a
// test holds the two lists equal in both directions.
var sentMessageTypes = []string{
	msgComputeProbe,
	msgVMDefine, msgVMStart, msgVMShutdown, msgVMStop, msgVMReset, msgVMPause, msgVMResume,
	msgVMDelete, msgVMQuery, msgVMStats, msgHostDevList,
	msgVMLog, msgVMDiskList, msgVMDiskCreate, msgVMDiskResize, msgVMDiskDelete,
	msgVMSnapshotCreate, msgVMSnapshotRevert, msgVMSnapshotDelete,
	msgVMImportInspect, msgVMImportConvert, msgVMImportStage, msgVMImportDiscard,
	msgVMCloneRequest, msgVMGuestQuery, msgVMGuestAgentDisc,
	msgVMBackupRun, msgVMBackupRestore,
	msgVMMigrate, msgVMSpiceTicket, msgVMCancel, msgVMQEMUCapability,
	msgVMMemorySet, msgVMCPUSet, msgVMThrottleSet, msgVMImageInspect,
	msgVMDiskHotplug, msgVMMediaChange, msgVMNICHotplug,
}

// observedMessageTypes is every unsolicited frame this plugin handles, declared under
// agent-rpc observe. net_lease_event and ct_backup_progress are deliberately absent: the
// lease and container frames never changed a guest row here.
var observedMessageTypes = []string{msgVMEvent, msgVMImportProgress}

// agentComputePayload is the compute slot of a probe reply.
type agentComputePayload struct {
	Capability *agentCapability `json:"capability,omitempty"`
}

// agentCapability is the part of the host's capability report this plugin reads: whether the
// host can run guests, and which machine types its QEMU offers.
type agentCapability struct {
	CanRunVMs    bool              `json:"canRunVms"`
	QEMUBinaries []agentQEMUBinary `json:"qemuBinaries,omitempty"`
}

type agentQEMUBinary struct {
	MachineTypes []string `json:"machineTypes,omitempty"`
	KVMAccel     bool     `json:"kvmAccel"`
}

// agentVMFrame is the vm slot of an unsolicited frame: a guest event, or one progress frame
// of a disk import.
type agentVMFrame struct {
	Operation    string             `json:"operation,omitempty"`
	Percent      int                `json:"percent,omitempty"`
	Event        *agentVMEvent      `json:"event,omitempty"`
	Import       *agentImportSpec   `json:"import,omitempty"`
	ImportResult *agentImportResult `json:"importResult,omitempty"`
}

type agentVMEvent struct {
	VMID      string            `json:"vmId"`
	Kind      string            `json:"kind"`
	Device    string            `json:"device,omitempty"`
	Detail    map[string]string `json:"detail,omitempty"`
	Timestamp string            `json:"timestamp,omitempty"`
}
