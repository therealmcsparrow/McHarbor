// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "time"

// Container states.
//
// These mirror the agent's `CTState.State` exactly rather than being translated
// into a prettier set, because a translation layer between what the agent
// observed and what the UI shows is where a management tool starts telling
// comfortable lies. `StateUnknown` in particular is a real state — it means the
// owning agent is offline and nobody knows — and is never collapsed into
// "stopped".
const (
	StateDefined  = "defined"
	StateRunning  = "running"
	StateFrozen   = "frozen"
	StateStopping = "stopping"
	StateStopped  = "stopped"
	StateCrashed  = "crashed"
	StateUnknown  = "unknown"
)

// Init systems. See CTSpec.InitSystem in the agent protocol for what each
// implies about the container profile.
const (
	InitSystemd = "systemd"
	InitOpenRC  = "openrc"
	InitNone    = "none"
)

// Mount kinds. The distinction between volume and the other two is a security
// boundary — see the note on plugin_addon_lxc_mounts.kind in the migration.
const (
	MountVolume = "volume"
	MountBind   = "bind"
	MountDevice = "device"
)

// Snapshot mechanisms, in decreasing order of how much the operator will enjoy
// them. `rsync` is the dir-pool fallback: a real copy rather than a snapshot,
// correct but slow, and labelled as such in the UI.
const (
	MechZFS   = "zfs"
	MechBtrfs = "btrfs"
	MechLVM   = "lvm"
	MechRsync = "rsync"
)

// Container is one system container as stored and as returned by the API.
//
// The JSON shape is the API shape: this struct is what the frontend's
// `Container` type mirrors. Fields the frontend has no business seeing — none
// here today — would be dropped in a separate response type rather than
// silenced with `json:"-"`, which is too easy to undo by accident.
type Container struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`

	State string `json:"state"`
	// HostAvailable is not stored. It is computed per response from whether
	// the owning agent is connected, and it is what lets the UI grey a
	// container and disable its actions instead of showing ten-minute-old
	// state beside buttons that will fail.
	HostAvailable bool `json:"hostAvailable"`
	// UnavailableReason names which of "not an agent environment", "agent
	// offline" or "agent too old" is the problem, so the UI can say so rather
	// than showing a generic failure.
	UnavailableReason string `json:"unavailableReason,omitempty"`

	OSFamily  string `json:"osFamily,omitempty"`
	OSVersion string `json:"osVersion,omitempty"`

	PoolID       string `json:"poolId"`
	PoolBackend  string `json:"poolBackend"`
	PoolTarget   string `json:"poolTarget"`
	RootfsPath   string `json:"rootfsPath"`
	TemplatePath string `json:"templatePath,omitempty"`

	InitSystem string   `json:"initSystem"`
	InitArgv   []string `json:"initArgv,omitempty"`

	Privileged bool `json:"privileged"`
	Nesting    bool `json:"nesting"`

	UIDMapStart    int64  `json:"uidMapStart,omitempty"`
	UIDMapCount    int64  `json:"uidMapCount,omitempty"`
	GIDMapStart    int64  `json:"gidMapStart,omitempty"`
	GIDMapCount    int64  `json:"gidMapCount,omitempty"`
	IDMapMechanism string `json:"idmapMechanism,omitempty"`

	// Limits is what was asked for. Applied is what the kernel took. They are
	// separate on the wire as well as in the schema, because the UI shows the
	// second and edits the first, and merging them would hide every case where
	// the kernel clamped a value.
	Limits  Limits  `json:"limits"`
	Applied *Limits `json:"appliedLimits,omitempty"`
	// LimitsAppliedAt is nil for a container whose limits have never reached a
	// running kernel — a stopped container's limits are a plan, not a fact.
	LimitsAppliedAt *time.Time `json:"limitsAppliedAt,omitempty"`

	Autostart       bool `json:"autostart"`
	AutostartOrder  int  `json:"autostartOrder"`
	AutostartDelayS int  `json:"autostartDelaySeconds"`

	IsTemplate       bool   `json:"isTemplate"`
	TemplateSourceID string `json:"templateSourceId,omitempty"`

	// ObservedIPs come from the agent reading the container's network
	// namespace — not from anything inside the container. Still rendered as
	// text and never as a link.
	ObservedIPs []string `json:"observedIps,omitempty"`

	UnitName string `json:"unitName,omitempty"`
	InitPID  int    `json:"initPid,omitempty"`

	LastStartedAt *time.Time `json:"lastStartedAt,omitempty"`
	ExitReason    string     `json:"exitReason,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`

	// Loaded on the detail response only. A list of two hundred containers
	// should not carry every mount of every one of them.
	NICs      []NIC      `json:"nics,omitempty"`
	Mounts    []Mount    `json:"mounts,omitempty"`
	Snapshots []Snapshot `json:"snapshots,omitempty"`
}

// Limits are the cgroup v2 values, in the units the API speaks rather than the
// units the kernel does.
//
// CPUMax is the exception: it stays in the kernel's own "quota period" form
// because that is what `cpu.max` contains, and inventing a percentage here
// would mean two lossy conversions between the operator and the file. The
// frontend does the percentage conversion for display, where a rounding error
// is cosmetic rather than persisted.
type Limits struct {
	CPUMax          string `json:"cpuMax,omitempty"`
	CPUWeight       int    `json:"cpuWeight,omitempty"`
	CPUSet          string `json:"cpuset,omitempty"`
	MemoryMaxBytes  int64  `json:"memoryMaxBytes,omitempty"`
	MemoryHighBytes int64  `json:"memoryHighBytes,omitempty"`
	// Zero, and deliberately so. Swap inside a container is shared with the
	// host's swap and turns one container's memory pressure into everything
	// else's latency; an operator who wants it sets it explicitly.
	SwapMaxBytes int64 `json:"swapMaxBytes"`
	// Fork-bomb protection. Not optional in practice: without it, one
	// container can exhaust the host's pid space and take down every other
	// container and the agent with it.
	PidsMax        int64  `json:"pidsMax,omitempty"`
	IOMax          string `json:"ioMax,omitempty"`
	DiskQuotaBytes int64  `json:"diskQuotaBytes,omitempty"`
}

// NIC is one container interface: a veth pair onto a bridge, or a macvlan
// child stacked straight on a host adapter.
type NIC struct {
	ID          string `json:"id"`
	ContainerID string `json:"containerId"`

	NetworkID string `json:"networkId"`
	Bridge    string `json:"bridge"`
	VLANTag   int    `json:"vlanTag,omitempty"`
	// AttachKind is empty or "bridge" for the veth pair, "macvlan" for a
	// child stacked straight on ParentLink. Denormalised from the
	// network, like Bridge, so a container start needs no network lookup.
	AttachKind string `json:"attachKind,omitempty"`
	ParentLink string `json:"parentLink,omitempty"`
	LinkMode   string `json:"linkMode,omitempty"`

	Slot      int    `json:"slot"`
	GuestName string `json:"guestName"`
	MAC       string `json:"mac"`
	// VethHost is what the interface is called on the host. Surfaced because
	// finding a specific container's veth in `ip link` is otherwise a process
	// of elimination.
	VethHost string `json:"vethHost,omitempty"`

	AddressMode string `json:"addressMode"`
	StaticIP    string `json:"staticIp,omitempty"`
	StaticGW    string `json:"staticGw,omitempty"`
	// StaticIPv6 and StaticGW6 are applied independently of AddressMode, so a
	// container on DHCPv4 can still hold a fixed IPv6; macvlan attachments only.
	StaticIPv6 string   `json:"staticIpv6,omitempty"`
	StaticGW6  string   `json:"staticGw6,omitempty"`
	StaticDNS  []string `json:"staticDns,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Mount is one mount into the container.
type Mount struct {
	ID          string    `json:"id"`
	ContainerID string    `json:"containerId"`
	Kind        string    `json:"kind"`
	Source      string    `json:"source"`
	Destination string    `json:"destination"`
	ReadOnly    bool      `json:"readonly"`
	ShiftIDs    bool      `json:"shiftIds"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Snapshot is one point-in-time copy of a container's rootfs.
type Snapshot struct {
	ID          string `json:"id"`
	ContainerID string `json:"containerId"`
	ParentID    string `json:"parentId,omitempty"`

	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Mechanism   string `json:"mechanism"`
	// WasFrozen decides whether this is a consistent snapshot or a
	// crash-consistent one. Shown next to the name rather than buried, because
	// it is the difference between a restore that boots and a restore that
	// runs fsck.
	WasFrozen  bool   `json:"wasFrozen"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	ChainDepth int    `json:"chainDepth"`
	Handle     string `json:"handle"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Provisioning is the first-boot setup written into the rootfs.
//
// Note what is absent: any plaintext password. The create request carries one,
// the service hashes it immediately, and only the hash is stored or sent. See
// hashPassword in scan.go.
type Provisioning struct {
	ContainerID string `json:"containerId"`

	Hostname string   `json:"hostname,omitempty"`
	Timezone string   `json:"timezone,omitempty"`
	SSHKeys  []string `json:"sshKeys,omitempty"`
	// HasPassword rather than the hash. A crypt(3) hash is not a secret in the
	// sense a token is, but it is offline-crackable, and there is no reason for
	// it to travel to a browser.
	HasPassword bool `json:"hasPassword"`

	NetConfigFormat string `json:"netConfigFormat"`

	AppliedAt *time.Time `json:"appliedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// Stats is one cgroup sample as returned to the frontend.
type Stats struct {
	ContainerID     string    `json:"containerId"`
	CPUPercent      float64   `json:"cpuPercent"`
	MemoryBytes     int64     `json:"memoryBytes"`
	MemoryMaxBytes  int64     `json:"memoryMaxBytes"`
	MemoryHighBytes int64     `json:"memoryHighBytes"`
	SwapBytes       int64     `json:"swapBytes"`
	PidsCurrent     int64     `json:"pidsCurrent"`
	PidsMax         int64     `json:"pidsMax"`
	IOReadBytes     int64     `json:"ioReadBytes"`
	IOWriteBytes    int64     `json:"ioWriteBytes"`
	NetRxBytes      int64     `json:"netRxBytes"`
	NetTxBytes      int64     `json:"netTxBytes"`
	RootfsUsedBytes int64     `json:"rootfsUsedBytes"`
	SampledAt       time.Time `json:"sampledAt"`
}

// Process is one process in the container's cgroup.
//
// A genuinely useful thing a system container has and a VM does not: the host
// can enumerate the guest's processes without anything cooperating from inside
// it. Command strings come from the container and are untrusted — rendered as
// text, never as markup.
type Process struct {
	PID         int     `json:"pid"`
	HostPID     int     `json:"hostPid,omitempty"`
	PPID        int     `json:"ppid,omitempty"`
	User        string  `json:"user,omitempty"`
	Command     string  `json:"command,omitempty"`
	CPUPercent  float64 `json:"cpuPercent,omitempty"`
	MemoryBytes int64   `json:"memoryBytes,omitempty"`
}

// --- Request bodies -------------------------------------------------

// CreateRequest is the create-container body.
type CreateRequest struct {
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`

	// Exactly one source. An image is a rootfs tarball addon-compute has
	// already fetched and verified; a template is another container in this
	// plugin marked as one.
	ImageID    string `json:"imageId,omitempty"`
	TemplateID string `json:"templateId,omitempty"`

	PoolID      string `json:"poolId"`
	PoolBackend string `json:"poolBackend"`
	PoolTarget  string `json:"poolTarget"`

	InitSystem string `json:"initSystem,omitempty"`

	// Both need ct.privileged. The handler checks, and the agent checks again.
	Privileged bool `json:"privileged,omitempty"`
	Nesting    bool `json:"nesting,omitempty"`

	Limits Limits `json:"limits"`

	NICs   []CreateNICRequest   `json:"nics,omitempty"`
	Mounts []CreateMountRequest `json:"mounts,omitempty"`

	Hostname string   `json:"hostname,omitempty"`
	Timezone string   `json:"timezone,omitempty"`
	SSHKeys  []string `json:"sshKeys,omitempty"`
	// Plaintext, and it lives exactly as long as this request. The service
	// hashes it before the row is written and never stores or forwards the
	// original.
	Password string `json:"password,omitempty"`

	Autostart       bool `json:"autostart,omitempty"`
	AutostartOrder  int  `json:"autostartOrder,omitempty"`
	AutostartDelayS int  `json:"autostartDelaySeconds,omitempty"`

	// Start after creating. Off by default: a container that starts before the
	// operator has looked at its network config is a container that has
	// already been on the LAN unconfigured.
	StartAfterCreate bool `json:"startAfterCreate,omitempty"`
}

// UpdateRequest is the edit-container body. Limits are deliberately absent —
// they have their own route because they apply live. See HandleUpdateLimits.
type UpdateRequest struct {
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	Autostart       *bool   `json:"autostart,omitempty"`
	AutostartOrder  *int    `json:"autostartOrder,omitempty"`
	AutostartDelayS *int    `json:"autostartDelaySeconds,omitempty"`
	Nesting         *bool   `json:"nesting,omitempty"`
}

// CreateNICRequest adds one interface.
type CreateNICRequest struct {
	NetworkID   string   `json:"networkId"`
	Bridge      string   `json:"bridge"`
	VLANTag     int      `json:"vlanTag,omitempty"`
	AttachKind  string   `json:"attachKind,omitempty"`
	ParentLink  string   `json:"parentLink,omitempty"`
	LinkMode    string   `json:"linkMode,omitempty"`
	GuestName   string   `json:"guestName,omitempty"`
	AddressMode string   `json:"addressMode,omitempty"`
	StaticIP    string   `json:"staticIp,omitempty"`
	StaticGW    string   `json:"staticGw,omitempty"`
	StaticIPv6  string   `json:"staticIpv6,omitempty"`
	StaticGW6   string   `json:"staticGw6,omitempty"`
	StaticDNS   []string `json:"staticDns,omitempty"`
}

// CreateMountRequest adds one mount.
type CreateMountRequest struct {
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"readonly,omitempty"`
	ShiftIDs    *bool  `json:"shiftIds,omitempty"`
}

// CreateSnapshotRequest takes a snapshot.
type CreateSnapshotRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Freeze the container for the duration. Default true for a running
	// container: the pause is milliseconds on a CoW pool and it is the
	// difference between a snapshot that restores cleanly and one that does
	// not.
	Freeze *bool `json:"freeze,omitempty"`
}

// CloneRequest clones a container or a template.
type CloneRequest struct {
	Name string `json:"name"`
	// Where the clone goes. Empty means the source's pool, which is the only
	// choice that can be a CoW clone rather than a copy.
	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend,omitempty"`
	PoolTarget  string `json:"poolTarget,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
	Start       bool   `json:"start,omitempty"`
	// Linked asks for a clone sharing the source's data. The host decides; the
	// clone is recorded as linked only if it reports one.
	Linked bool `json:"linked,omitempty"`
}

// StopRequest carries the grace period for a shutdown.
type StopRequest struct {
	// Seconds to wait for init to exit before the cgroup is killed. Zero means
	// the service's default; the UI offers it because "how long is this
	// allowed to take" is a per-workload answer.
	GraceSeconds int `json:"graceSeconds,omitempty"`
}

// DeleteRequest destroys a container.
type DeleteRequest struct {
	// Keep the rootfs. Offered because a rootfs on a CoW pool is cheap to keep
	// and expensive to have deleted by mistake.
	KeepRootfs bool `json:"keepRootfs,omitempty"`
}

// LimitsResponse is what the limits route returns: what was asked for, what the
// kernel took, and whether the two differ.
type LimitsResponse struct {
	Requested Limits `json:"requested"`
	Applied   Limits `json:"applied"`
	// Clamped names the fields the kernel changed. The UI shows these
	// specifically rather than a generic "some values were adjusted", because
	// which value moved is the whole of the useful information.
	Clamped []string `json:"clamped,omitempty"`
	// Live is false when the container was not running, in which case the
	// values are stored for next start and nothing was written to a cgroup.
	Live bool `json:"live"`
}
