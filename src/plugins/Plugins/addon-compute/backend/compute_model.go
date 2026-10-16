// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "time"

// The compute foundation's own model: what it stores, returns and renders about hosts,
// pools, networks, images and compute locations.
//
// These types came from the host's shared compute spec, which every plugin linked against,
// so a field added there stopped every installed plugin loading. They live here now and the
// agent's JSON is decoded into wire_agent.go's structs and converted in agent_bridge.go.
// The JSON tags are unchanged: capability reports are stored as JSON and served to the UI,
// and src/contracts/agentwire/converted pins them to what the host used to produce.

// Pool backends.
const (
	BackendDir   = "dir"
	BackendZFS   = "zfs"
	BackendLVM   = "lvm"
	BackendBtrfs = "btrfs"
)

// Guest kinds a workflow node may require of a host.
const (
	guestVM = "vm"
	guestCT = "ct"
)

// Capability is one host's answer to "what can you run?". Supported is false with
// UnsupportedWhy set on a Windows or macOS agent, so the UI explains the gap.
type Capability struct {
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	KernelVersion  string `json:"kernelVersion,omitempty"`
	AgentVersion   string `json:"agentVersion,omitempty"`
	Supported      bool   `json:"supported"`
	UnsupportedWhy string `json:"unsupportedWhy,omitempty"`
	CanRunVMs      bool   `json:"canRunVms"`
	CanRunCTs      bool   `json:"canRunContainers"`
	SeccompBuiltIn bool   `json:"seccompBuiltIn"`
	SupervisorMode string `json:"supervisorMode,omitempty"`
	// HostCPUs and HostMemoryBytes bound a guest's processor and memory controls.
	HostCPUs        int   `json:"hostCpus,omitempty"`
	HostMemoryBytes int64 `json:"hostMemoryBytes,omitempty"`
	// HostSockets and HostPhysicalCores describe the topology behind HostCPUs, which counts
	// threads; overcommit is judged against real cores.
	HostSockets       int `json:"hostSockets,omitempty"`
	HostPhysicalCores int `json:"hostPhysicalCores,omitempty"`

	Items        []CapabilityItem `json:"items,omitempty"`
	QEMUBinaries []QEMUBinary     `json:"qemuBinaries,omitempty"`
	Bridges      []HostLink       `json:"bridges,omitempty"`
	ProbedAt     time.Time        `json:"probedAt,omitempty"`
}

// CapabilityItem is one line of the dependency checklist; Remediation is what lets the
// operator fix it instead of being stuck.
type CapabilityItem struct {
	Key         string `json:"key"`
	Present     bool   `json:"present"`
	Detail      string `json:"detail,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	Critical    bool   `json:"critical,omitempty"`
	Blocks      string `json:"blocks,omitempty"`
	// Installable marks a missing item the host's agent can install for itself.
	Installable bool `json:"installable,omitempty"`
}

// InstallResult is what installing a capability item did: the packages the agent chose, the
// tail of the package manager's output, and the host's capability afterwards.
type InstallResult struct {
	Item       string      `json:"item"`
	Packages   []string    `json:"packages,omitempty"`
	Output     string      `json:"output,omitempty"`
	Capability *Capability `json:"capability,omitempty"`
}

// installableItems are the capability items a package can provide. Anything else is a kernel
// feature or a host setting, and is refused before the agent is asked.
var installableItems = map[string]bool{"qemu": true, "qemu_img": true, "ovmf": true, "ovmf_secboot": true, "swtpm": true}

// QEMUBinary is one emulator found on the host.
type QEMUBinary struct {
	Path         string   `json:"path"`
	Arch         string   `json:"arch"`
	Version      string   `json:"version"`
	MachineTypes []string `json:"machineTypes,omitempty"`
	KVMAccel     bool     `json:"kvmAccel"`
}

// HostLink is one network interface on the host. HasDefaultRoute marks the operator's
// uplink, and Upper lists what is stacked on it, so the picker refuses both up front.
type HostLink struct {
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

// RootsConfig names where a host keeps its compute state; an empty field leaves that
// location alone.
type RootsConfig struct {
	VMRoot    string `json:"vmRoot,omitempty"`
	CTRoot    string `json:"ctRoot,omitempty"`
	ImageRoot string `json:"imageRoot,omitempty"`
}

// PoolRef is the four fields the agent needs to find a pool; nothing else of the stored
// row crosses to it.
type PoolRef struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Backend string `json:"backend"`
	Target  string `json:"target"`
}

// PoolBackend is one backend's availability on a host. Instant is what the setup wizard's
// recommendation is built on.
type PoolBackend struct {
	Backend     string          `json:"backend"`
	Available   bool            `json:"available"`
	Detail      string          `json:"detail,omitempty"`
	Remediation string          `json:"remediation,omitempty"`
	Instant     bool            `json:"instant,omitempty"`
	Candidates  []PoolCandidate `json:"candidates,omitempty"`
}

// PoolCandidate is somewhere a pool could live.
type PoolCandidate struct {
	Target        string `json:"target"`
	Mountpoint    string `json:"mountpoint,omitempty"`
	CapacityBytes int64  `json:"capacityBytes,omitempty"`
	FreeBytes     int64  `json:"freeBytes,omitempty"`
}

// PoolStats is one capacity sample. AllocatedBytes can exceed CapacityBytes on a thin
// backend, and that overcommit is surfaced rather than hidden.
type PoolStats struct {
	CapacityBytes  int64     `json:"capacityBytes"`
	UsedBytes      int64     `json:"usedBytes"`
	FreeBytes      int64     `json:"freeBytes"`
	AllocatedBytes int64     `json:"allocatedBytes,omitempty"`
	VolumeCount    int       `json:"volumeCount,omitempty"`
	SampledAt      time.Time `json:"sampledAt,omitempty"`
}

// PoolRequest asks the agent to create or remove a pool; without Force a pool that still
// holds volumes is refused.
type PoolRequest struct {
	PoolID  string `json:"poolId,omitempty"`
	Name    string `json:"name,omitempty"`
	Backend string `json:"backend,omitempty"`
	Target  string `json:"target,omitempty"`
	Force   bool   `json:"force,omitempty"`
}

// PoolFile is one entry in a pool's directory listing. ModifiedAt stays RFC3339 text, as
// every timestamp crossing the agent transport is.
type PoolFile struct {
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

// PoolFileRequest is one read, write, mkdir, delete or rename; the path is relative to the
// pool root and confined on the agent.
type PoolFileRequest struct {
	Operation string `json:"operation"`
	Path      string `json:"path,omitempty"`
	NewName   string `json:"newName,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Content   []byte `json:"content,omitempty"`
	Recursive bool   `json:"recursive,omitempty"`
}

// NetworkRequest describes a bridged or managed network to create or remove. The direct
// modes use directnet.go's agentNetwork instead.
type NetworkRequest struct {
	NetworkID      string `json:"networkId,omitempty"`
	Name           string `json:"name,omitempty"`
	Mode           string `json:"mode,omitempty"`
	Bridge         string `json:"bridge,omitempty"`
	VLANTag        int    `json:"vlanTag,omitempty"`
	SubnetCIDR     string `json:"subnetCidr,omitempty"`
	GatewayIP      string `json:"gatewayIp,omitempty"`
	DHCPEnabled    bool   `json:"dhcpEnabled,omitempty"`
	DHCPRangeStart string `json:"dhcpRangeStart,omitempty"`
	DHCPRangeEnd   string `json:"dhcpRangeEnd,omitempty"`
	NATEnabled     bool   `json:"natEnabled,omitempty"`
	UplinkLink     string `json:"uplinkLink,omitempty"`
}

// ImageRequest asks the agent to fetch an image. AllowUnverified applies only to an
// operator-supplied URL; LibraryURL and LibraryToken let an agent that does not share the
// library pull an upload from the server.
type ImageRequest struct {
	ImageID         string `json:"imageId"`
	Kind            string `json:"kind"`
	SourceURL       string `json:"sourceUrl"`
	ChecksumURL     string `json:"checksumUrl,omitempty"`
	Checksum        string `json:"checksum,omitempty"`
	ChecksumAlgo    string `json:"checksumAlgo,omitempty"`
	SignatureURL    string `json:"signatureUrl,omitempty"`
	AllowUnverified bool   `json:"allowUnverified,omitempty"`
	FromLibrary     bool   `json:"fromLibrary,omitempty"`
	Filename        string `json:"filename,omitempty"`
	LibraryURL      string
	LibraryToken    string
}

// ImageResult is a completed fetch.
type ImageResult struct {
	ImageID   string `json:"imageId"`
	LocalPath string `json:"localPath"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	Checksum  string `json:"checksum,omitempty"`
	Verified  bool   `json:"verified"`
	Signed    bool   `json:"signed,omitempty"`
}

// ImageProgress is one sample during a download; Phase says what the percentage measures.
type ImageProgress struct {
	ImageID    string `json:"imageId"`
	BytesDone  int64  `json:"bytesDone"`
	BytesTotal int64  `json:"bytesTotal"`
	Percent    int    `json:"percent"`
	Phase      string `json:"phase,omitempty"`
}

// CachedImage is one image already on a host; InUse ones are never evicted.
type CachedImage struct {
	ImageID    string    `json:"imageId"`
	Kind       string    `json:"kind,omitempty"`
	LocalPath  string    `json:"localPath"`
	SizeBytes  int64     `json:"sizeBytes,omitempty"`
	Verified   bool      `json:"verified,omitempty"`
	InUse      bool      `json:"inUse,omitempty"`
	LastUsedAt time.Time `json:"lastUsedAt,omitempty"`
}

// GuestEvent is one unsolicited event about a guest. This plugin receives only DHCP
// leases, which arrive as Kind "lease_<action>" with the address in Detail.
type GuestEvent struct {
	EnvID     string            `json:"envId"`
	GuestKind string            `json:"guestKind"`
	GuestID   string            `json:"guestId"`
	Kind      string            `json:"kind"`
	Device    string            `json:"device,omitempty"`
	Detail    map[string]string `json:"detail,omitempty"`
	At        time.Time         `json:"at"`
}

// MigrationRequest asks a host to move the compute state it holds. Plan reports and
// touches nothing; Repair fixes the paths inside files a move already relocated.
type MigrationRequest struct {
	Plan      bool
	Repair    bool
	Roots     RootsConfig
	Volumes   []MigrationMove
	VMFrom    string
	CTFrom    string
	ImageFrom string
	DiskFrom  string
	DiskTo    string
}

// MigrationMove is one path before and after, with the guest it belongs to for a disk.
type MigrationMove struct {
	Kind    string
	From    string
	To      string
	GuestID string
	Slot    int
	Bytes   int64
}

// MigrationResult is what a plan found or what an apply moved; Notes make a zero count
// interpretable.
type MigrationResult struct {
	Entries   int
	Bytes     int64
	Moved     []MigrationMove
	Failures  []MigrationFailure
	Notes     []string
	Completed bool
}

// MigrationFailure is one path that could not be moved, and why.
type MigrationFailure struct {
	Path   string
	Reason string
}

// MigrationProgress is the last thing a running relocation reported.
type MigrationProgress struct {
	Percent int
	Bytes   int64
	At      time.Time
}
