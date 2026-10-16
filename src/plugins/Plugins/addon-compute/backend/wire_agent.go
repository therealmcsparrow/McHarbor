// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

// The agent protocol as this plugin reads and writes it.
//
// The plugin reaches the agent through the host's agent-rpc bridge, which takes one
// envelope slot as JSON and hands the reply slot back as JSON. These structs hold only the
// fields this plugin sends or decodes; the host decodes what it is given into its own
// protocol struct and drops anything else. addon-kvm and addon-lxc keep their own copies of
// the fields they need, which is the accepted duplication family addon-agent-wire.
//
// src/contracts/agentwire/slots holds fully populated fixtures generated from the host's
// protocol structs, and wire_agent_test.go decodes them into these, so a renamed or dropped
// wire field fails a test here instead of silently decoding to zero.

// Agent message types this plugin sends, each declared under agent-rpc send in plugin.yaml.
// The host's protocol constants are not part of the plugin surface, so they are named here.
const (
	msgComputeProbe   = "compute_probe_request"
	msgComputeInstall = "compute_install_request"
	msgComputeConfig  = "compute_config_request"
	msgComputeMigrate = "compute_migrate_request"
	msgPoolDetect     = "pool_detect_request"
	msgPoolCreate     = "pool_create_request"
	msgPoolStats      = "pool_stats_request"
	msgPoolRemove     = "pool_remove_request"
	msgPoolBrowse     = "pool_browse_request"
	msgPoolFile       = "pool_file_request"
	msgNetDetect      = "net_detect_request"
	msgImageFetch     = "compute_image_fetch"
	msgImageProgress  = "compute_image_progress"
	msgImageList      = "compute_image_list"
	msgImageRemove    = "compute_image_remove"
)

// Unsolicited frames this plugin observes, declared under agent-rpc observe.
const (
	msgNetLeaseEvent          = "net_lease_event"
	msgComputeMigrateProgress = "compute_migrate_progress"
)

// agentComputePayload is the compute slot: the probe, the locations and a relocation.
type agentComputePayload struct {
	Roots      *agentRoots      `json:"roots,omitempty"`
	Migrate    *agentMigrate    `json:"migrate,omitempty"`
	Capability *agentCapability `json:"capability,omitempty"`
	Install    *agentInstall    `json:"install,omitempty"`
	Success    bool             `json:"success,omitempty"`
	Error      string           `json:"error,omitempty"`
}

type agentRoots struct {
	VMRoot    string `json:"vmRoot,omitempty"`
	CTRoot    string `json:"ctRoot,omitempty"`
	ImageRoot string `json:"imageRoot,omitempty"`
}

// agentMigrate is a relocation request and, on the way back, its plan or result. Entries
// doubles as the percentage on a compute_migrate_progress frame.
type agentMigrate struct {
	Plan      bool         `json:"plan,omitempty"`
	Repair    bool         `json:"repair,omitempty"`
	Roots     *agentRoots  `json:"roots,omitempty"`
	Volumes   []agentMove  `json:"volumes,omitempty"`
	VMFrom    string       `json:"vmFrom,omitempty"`
	CTFrom    string       `json:"ctFrom,omitempty"`
	ImageFrom string       `json:"imageFrom,omitempty"`
	DiskFrom  string       `json:"diskFrom,omitempty"`
	DiskTo    string       `json:"diskTo,omitempty"`
	Entries   int          `json:"entries,omitempty"`
	Bytes     int64        `json:"bytes,omitempty"`
	Moved     []agentMove  `json:"moved,omitempty"`
	Failures  []agentFault `json:"failures,omitempty"`
	Notes     []string     `json:"notes,omitempty"`
	Completed bool         `json:"completed,omitempty"`
}

type agentMove struct {
	Kind    string `json:"kind"`
	From    string `json:"from"`
	To      string `json:"to"`
	GuestID string `json:"guestId,omitempty"`
	Slot    int    `json:"slot,omitempty"`
	Bytes   int64  `json:"bytes,omitempty"`
}

type agentFault struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type agentCapability struct {
	OS                string                `json:"os"`
	Arch              string                `json:"arch"`
	KernelVersion     string                `json:"kernelVersion,omitempty"`
	AgentVersion      string                `json:"agentVersion,omitempty"`
	Supported         bool                  `json:"supported"`
	UnsupportedWhy    string                `json:"unsupportedWhy,omitempty"`
	CanRunVMs         bool                  `json:"canRunVms"`
	CanRunContainers  bool                  `json:"canRunContainers"`
	SeccompBuiltIn    bool                  `json:"seccompBuiltIn"`
	SupervisorMode    string                `json:"supervisorMode,omitempty"`
	HostCPUs          int                   `json:"hostCpus,omitempty"`
	HostMemoryBytes   int64                 `json:"hostMemoryBytes,omitempty"`
	HostSockets       int                   `json:"hostSockets,omitempty"`
	HostPhysicalCores int                   `json:"hostPhysicalCores,omitempty"`
	Items             []agentCapabilityItem `json:"items,omitempty"`
	QEMUBinaries      []agentQEMUBinary     `json:"qemuBinaries,omitempty"`
	Bridges           []agentHostLink       `json:"bridges,omitempty"`
	ProbedAt          string                `json:"probedAt,omitempty"`
}

type agentCapabilityItem struct {
	Key         string `json:"key"`
	Present     bool   `json:"present"`
	Detail      string `json:"detail,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	Critical    bool   `json:"critical,omitempty"`
	Blocks      string `json:"blocks,omitempty"`
	Installable bool   `json:"installable,omitempty"`
}

// agentInstall carries the item out and, on the way back, the packages the agent chose and
// the package manager's output.
type agentInstall struct {
	Item     string   `json:"item"`
	Packages []string `json:"packages,omitempty"`
	Output   string   `json:"output,omitempty"`
}

type agentQEMUBinary struct {
	Path         string   `json:"path"`
	Arch         string   `json:"arch"`
	Version      string   `json:"version"`
	MachineTypes []string `json:"machineTypes,omitempty"`
	KVMAccel     bool     `json:"kvmAccel"`
}

type agentHostLink struct {
	Name            string   `json:"name"`
	Kind            string   `json:"kind"`
	MAC             string   `json:"mac,omitempty"`
	MTU             int      `json:"mtu,omitempty"`
	Up              bool     `json:"up"`
	Addresses       []string `json:"addresses,omitempty"`
	Master          string   `json:"master,omitempty"`
	Managed         bool     `json:"managed,omitempty"`
	HasDefaultRoute bool     `json:"hasDefaultRoute,omitempty"`
	Upper           []string `json:"upper,omitempty"`
}

// agentPoolPayload is the pool slot, request and reply.
type agentPoolPayload struct {
	Operation string `json:"operation,omitempty"`
	PoolID    string `json:"poolId,omitempty"`
	Name      string `json:"name,omitempty"`
	Backend   string `json:"backend,omitempty"`
	Target    string `json:"target,omitempty"`
	Force     bool   `json:"force,omitempty"`
	Path      string `json:"path,omitempty"`
	NewName   string `json:"newName,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Content   []byte `json:"content,omitempty"`
	Recursive bool   `json:"recursive,omitempty"`

	Detected []agentPoolBackend `json:"detected,omitempty"`
	Stats    *agentPoolStats    `json:"stats,omitempty"`
	Entries  []agentPoolFile    `json:"entries,omitempty"`
	// Error and ErrorCode come back on a refusal; the code is what a branch reads.
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

type agentPoolBackend struct {
	Backend     string               `json:"backend"`
	Available   bool                 `json:"available"`
	Detail      string               `json:"detail,omitempty"`
	Remediation string               `json:"remediation,omitempty"`
	Candidates  []agentPoolCandidate `json:"candidates,omitempty"`
	Instant     bool                 `json:"instant,omitempty"`
}

type agentPoolCandidate struct {
	Target        string `json:"target"`
	Mountpoint    string `json:"mountpoint,omitempty"`
	CapacityBytes int64  `json:"capacityBytes,omitempty"`
	FreeBytes     int64  `json:"freeBytes,omitempty"`
}

type agentPoolStats struct {
	CapacityBytes  int64  `json:"capacityBytes"`
	UsedBytes      int64  `json:"usedBytes"`
	FreeBytes      int64  `json:"freeBytes"`
	AllocatedBytes int64  `json:"allocatedBytes,omitempty"`
	VolumeCount    int    `json:"volumeCount,omitempty"`
	SampledAt      string `json:"sampledAt,omitempty"`
}

type agentPoolFile struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsDir      bool   `json:"isDir,omitempty"`
	IsSymlink  bool   `json:"isSymlink,omitempty"`
	LinkTarget string `json:"linkTarget,omitempty"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	Mode       string `json:"mode,omitempty"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
	OwnerUID   int    `json:"ownerUid,omitempty"`
	OwnerGID   int    `json:"ownerGid,omitempty"`
}

// agentNetReply is the net slot as this plugin decodes it: a link inventory or a lease.
// Requests go out as agentNetwork (directnet.go).
type agentNetReply struct {
	Links []agentHostLink `json:"links,omitempty"`
	Lease *agentLease     `json:"lease,omitempty"`
}

type agentLease struct {
	NetworkID string `json:"networkId"`
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	Hostname  string `json:"hostname,omitempty"`
	GuestKind string `json:"guestKind,omitempty"`
	GuestID   string `json:"guestId,omitempty"`
	Action    string `json:"action,omitempty"`
}

// agentImagePayload is the compute_image slot: a fetch request, a progress frame, a result,
// or the cache inventory.
type agentImagePayload struct {
	ImageID         string `json:"imageId,omitempty"`
	Kind            string `json:"kind,omitempty"`
	SourceURL       string `json:"sourceUrl,omitempty"`
	ChecksumURL     string `json:"checksumUrl,omitempty"`
	Checksum        string `json:"checksum,omitempty"`
	ChecksumAlgo    string `json:"checksumAlgo,omitempty"`
	SignatureURL    string `json:"signatureUrl,omitempty"`
	AllowUnverified bool   `json:"allowUnverified,omitempty"`
	FromLibrary     bool   `json:"fromLibrary,omitempty"`
	Filename        string `json:"filename,omitempty"`
	LibraryURL      string `json:"libraryUrl,omitempty"`
	LibraryToken    string `json:"libraryToken,omitempty"`

	LocalPath  string             `json:"localPath,omitempty"`
	BytesDone  int64              `json:"bytesDone,omitempty"`
	BytesTotal int64              `json:"bytesTotal,omitempty"`
	Percent    int                `json:"percent,omitempty"`
	Phase      string             `json:"phase,omitempty"`
	SizeBytes  int64              `json:"sizeBytes,omitempty"`
	Verified   bool               `json:"verified,omitempty"`
	Signed     bool               `json:"signed,omitempty"`
	Cached     []agentCachedImage `json:"cached,omitempty"`
}

type agentCachedImage struct {
	ImageID    string `json:"imageId"`
	Kind       string `json:"kind,omitempty"`
	LocalPath  string `json:"localPath"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	Verified   bool   `json:"verified,omitempty"`
	InUse      bool   `json:"inUse,omitempty"`
	LastUsedAt string `json:"lastUsedAt,omitempty"`
}

// agentVMStates is the one field of a vm_query reply a pool move reads.
type agentVMStates struct {
	States []struct {
		VMID  string `json:"vmId"`
		State string `json:"state"`
	} `json:"states"`
}
