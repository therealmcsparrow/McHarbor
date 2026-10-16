// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

// The agent protocol as this plugin reads and writes it.
//
// The plugin reaches the agent through the host's agent-rpc bridge, which takes one
// envelope slot as JSON and hands the reply slot back as JSON. These structs hold only the
// fields this plugin sends or decodes; the host decodes what it is given into its own
// protocol struct and drops anything else. addon-compute and addon-kvm keep their own copies
// of the fields they need, which is the accepted duplication family addon-agent-wire.
//
// src/contracts/agentwire/slots holds fully populated fixtures generated from the host's
// protocol structs, and agent_bridge_test.go decodes them into these, so a renamed or
// dropped wire field fails a test here instead of silently decoding to zero.

// Agent message types this plugin sends or observes. They are the protocol's own strings —
// the host validates them against its allow-list before anything reaches the
// agent, so a typo here fails closed rather than reaching a root process.
const (
	msgCTCreate   = "ct_create"
	msgCTStart    = "ct_start"
	msgCTShutdown = "ct_shutdown"
	msgCTStop     = "ct_stop"
	msgCTRestart  = "ct_restart"
	msgCTFreeze   = "ct_freeze"
	msgCTUnfreeze = "ct_unfreeze"
	msgCTDelete   = "ct_delete"
	msgCTQuery    = "ct_query"

	msgCTUpdateLimits = "ct_update_limits"
	msgCTProcesses    = "ct_processes"

	msgCTSnapshotCreate = "ct_snapshot_create"
	msgCTSnapshotRevert = "ct_snapshot_revert"
	msgCTSnapshotDelete = "ct_snapshot_delete"

	msgCTCloneRequest = "ct_clone_request"

	msgCTBackupRun     = "ct_backup_run"
	msgCTBackupRestore = "ct_backup_restore"

	msgComputeProbe = "compute_probe_request"
	msgImageList    = "compute_image_list"
	msgPoolFile     = "pool_file_request"

	// msgCTBackupProgress is the one frame this plugin observes rather than sends.
	msgCTBackupProgress = "ct_backup_progress"
)

// agentPayload is the ct-family payload this plugin builds.
//
// It mirrors the protocol's CTPayload without importing it: the host's
// GuestCommand takes opaque JSON precisely so the guest command surface stays
// with the plugins that own it rather than being mirrored into core.
type agentPayload struct {
	Operation   string `json:"operation,omitempty"`
	EnvID       string `json:"envId,omitempty"`
	ContainerID string `json:"containerId,omitempty"`

	Spec         *agentSpec   `json:"spec,omitempty"`
	GraceSeconds int          `json:"graceSeconds,omitempty"`
	DeleteRootfs bool         `json:"deleteRootfs,omitempty"`
	Limits       *agentLimits `json:"limits,omitempty"`

	SnapshotID   string      `json:"snapshotId,omitempty"`
	SnapshotName string      `json:"snapshotName,omitempty"`
	Clone        *agentClone `json:"clone,omitempty"`

	ImageID string `json:"imageId,omitempty"`
	PoolID  string `json:"poolId,omitempty"`

	// Reply-only.
	States    []agentState   `json:"states,omitempty"`
	State     *agentState    `json:"state,omitempty"`
	Stats     *agentStats    `json:"stats,omitempty"`
	Processes []agentProcess `json:"processes,omitempty"`
	Applied   *agentLimits   `json:"applied,omitempty"`

	// Backup and restore. Both carry their own spec rather than reusing the
	// snapshot fields: a snapshot stays on the pool, a backup leaves the host.
	Backup       *agentBackupSpec   `json:"backup,omitempty"`
	Restore      *agentRestoreSpec  `json:"restore,omitempty"`
	BackupResult *agentBackupResult `json:"backupResult,omitempty"`
	BackupBytes  int64              `json:"backupBytes,omitempty"`
	Percent      int                `json:"percent,omitempty"`
	SizeBytes    int64              `json:"sizeBytes,omitempty"`
	Orphans      []string           `json:"orphans,omitempty"`
	Success      bool               `json:"success,omitempty"`
	Error        string             `json:"error,omitempty"`
	ErrorCode    string             `json:"errorCode,omitempty"`

	// Progress is the detail map of an unsolicited ct_backup_progress frame.
	Progress map[string]string `json:"progress,omitempty"`
}

type agentSpec struct {
	Name         string `json:"name"`
	RootfsPath   string `json:"rootfsPath"`
	PoolID       string `json:"poolId,omitempty"`
	PoolBackend  string `json:"poolBackend,omitempty"`
	PoolTarget   string `json:"poolTarget,omitempty"`
	TemplatePath string `json:"templatePath,omitempty"`

	InitSystem string   `json:"initSystem,omitempty"`
	InitArgv   []string `json:"initArgv,omitempty"`

	Privileged bool `json:"privileged,omitempty"`
	Nesting    bool `json:"nesting,omitempty"`

	UIDMapStart int64  `json:"uidMapStart,omitempty"`
	UIDMapCount int64  `json:"uidMapCount,omitempty"`
	GIDMapStart int64  `json:"gidMapStart,omitempty"`
	GIDMapCount int64  `json:"gidMapCount,omitempty"`
	IDMapMech   string `json:"idmapMechanism,omitempty"`

	Limits *agentLimits `json:"limits,omitempty"`
	NICs   []agentNIC   `json:"nics,omitempty"`
	Mounts []agentMount `json:"mounts,omitempty"`

	Hostname     string   `json:"hostname,omitempty"`
	SSHKeys      []string `json:"sshKeys,omitempty"`
	PasswordHash string   `json:"passwordHash,omitempty"`
	NetConfigFmt string   `json:"netConfigFormat,omitempty"`

	UnitName string `json:"unitName,omitempty"`
}

type agentLimits struct {
	CPUMax          string `json:"cpuMax,omitempty"`
	CPUWeight       int    `json:"cpuWeight,omitempty"`
	CPUSet          string `json:"cpuset,omitempty"`
	MemoryMaxBytes  int64  `json:"memoryMaxBytes,omitempty"`
	MemoryHighBytes int64  `json:"memoryHighBytes,omitempty"`
	SwapMaxBytes    int64  `json:"swapMaxBytes,omitempty"`
	PidsMax         int64  `json:"pidsMax,omitempty"`
	IOMax           string `json:"ioMax,omitempty"`
	DiskQuotaBytes  int64  `json:"diskQuotaBytes,omitempty"`
}

type agentNIC struct {
	NICID       string   `json:"nicId,omitempty"`
	NetworkID   string   `json:"networkId,omitempty"`
	Bridge      string   `json:"bridge,omitempty"`
	VLANTag     int      `json:"vlanTag,omitempty"`
	AttachKind  string   `json:"attachKind,omitempty"`
	ParentLink  string   `json:"parentLink,omitempty"`
	LinkMode    string   `json:"linkMode,omitempty"`
	Slot        int      `json:"slot"`
	GuestName   string   `json:"guestName,omitempty"`
	MAC         string   `json:"mac"`
	VethHost    string   `json:"vethHost,omitempty"`
	AddressMode string   `json:"addressMode,omitempty"`
	StaticIP    string   `json:"staticIp,omitempty"`
	StaticGW    string   `json:"staticGw,omitempty"`
	StaticIPv6  string   `json:"staticIpv6,omitempty"`
	StaticGW6   string   `json:"staticGw6,omitempty"`
	StaticDNS   []string `json:"staticDns,omitempty"`
}

type agentMount struct {
	MountID     string `json:"mountId,omitempty"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"readonly,omitempty"`
	ShiftIDs    bool   `json:"shiftIds,omitempty"`
}

type agentClone struct {
	TargetContainerID string     `json:"targetContainerId"`
	Spec              *agentSpec `json:"spec,omitempty"`
	// Linked asks for shared data; the reply echoes what was actually made,
	// and only a reply that says linked makes the source undeletable.
	Linked bool `json:"linked,omitempty"`
}

type agentState struct {
	ContainerID string   `json:"containerId"`
	State       string   `json:"state"`
	UnitName    string   `json:"unitName,omitempty"`
	UnitActive  string   `json:"unitActive,omitempty"`
	UnitSub     string   `json:"unitSub,omitempty"`
	InitPID     int      `json:"initPid,omitempty"`
	StartedAt   string   `json:"startedAt,omitempty"`
	IPs         []string `json:"ips,omitempty"`
	ExitReason  string   `json:"exitReason,omitempty"`
	LastError   string   `json:"lastError,omitempty"`
}

type agentStats struct {
	ContainerID     string  `json:"containerId"`
	CPUPercent      float64 `json:"cpuPercent,omitempty"`
	CPUUsageNSec    int64   `json:"cpuUsageNsec,omitempty"`
	MemoryBytes     int64   `json:"memoryBytes,omitempty"`
	MemoryMaxBytes  int64   `json:"memoryMaxBytes,omitempty"`
	MemoryHighBytes int64   `json:"memoryHighBytes,omitempty"`
	SwapBytes       int64   `json:"swapBytes,omitempty"`
	PidsCurrent     int64   `json:"pidsCurrent,omitempty"`
	PidsMax         int64   `json:"pidsMax,omitempty"`
	IOReadBytes     int64   `json:"ioReadBytes,omitempty"`
	IOWriteBytes    int64   `json:"ioWriteBytes,omitempty"`
	NetRxBytes      int64   `json:"netRxBytes,omitempty"`
	NetTxBytes      int64   `json:"netTxBytes,omitempty"`
	RootfsUsedBytes int64   `json:"rootfsUsedBytes,omitempty"`
	SampledAt       string  `json:"sampledAt,omitempty"`
}

type agentProcess struct {
	PID         int     `json:"pid"`
	HostPID     int     `json:"hostPid,omitempty"`
	PPID        int     `json:"ppid,omitempty"`
	User        string  `json:"user,omitempty"`
	Command     string  `json:"command,omitempty"`
	CPUPercent  float64 `json:"cpuPercent,omitempty"`
	MemoryBytes int64   `json:"memoryBytes,omitempty"`
}

// agentComputePayload is the compute slot of a probe reply: the capability report.
type agentComputePayload struct {
	Capability *agentCapability `json:"capability,omitempty"`
}

// agentCapability holds the probe fields this plugin reads: whether the host can run
// containers, why not, the idmapped-mount item, and the host's size for limit validation.
type agentCapability struct {
	UnsupportedWhy   string                `json:"unsupportedWhy,omitempty"`
	CanRunContainers bool                  `json:"canRunContainers"`
	HostCPUs         int                   `json:"hostCpus,omitempty"`
	HostMemoryBytes  int64                 `json:"hostMemoryBytes,omitempty"`
	Items            []agentCapabilityItem `json:"items,omitempty"`
}

type agentCapabilityItem struct {
	Key     string `json:"key"`
	Present bool   `json:"present"`
}

// agentImagePayload is the compute_image slot of an image list reply.
type agentImagePayload struct {
	Cached []agentCachedImage `json:"cached,omitempty"`
}

type agentCachedImage struct {
	ImageID   string `json:"imageId"`
	LocalPath string `json:"localPath"`
}

// agentPoolFilePayload is the pool slot of a pool_file_request and its reply; only a read
// is sent, and the file's bytes come back in Content (base64 on the wire).
type agentPoolFilePayload struct {
	Operation string `json:"operation,omitempty"`
	PoolID    string `json:"poolId,omitempty"`
	Backend   string `json:"backend,omitempty"`
	Target    string `json:"target,omitempty"`
	Path      string `json:"path,omitempty"`
	Content   []byte `json:"content,omitempty"`
}
