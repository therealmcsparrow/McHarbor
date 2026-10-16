// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "time"

// Guest states. These are the values the agent confirms and this plugin stores.
//
// `unknown` is a first-class state, not an error. It is what a guest is when
// its host's agent is offline: the UI greys the last known state, badges it
// "agent offline" and disables every action, because guessing that a VM is
// still running because it was ten minutes ago is how a management UI lies to
// its operator.
const (
	StateDefined   = "defined"
	StateRunning   = "running"
	StatePaused    = "paused"
	StateStopping  = "stopping"
	StateStopped   = "stopped"
	StateDeleting  = "deleting"
	StateCrashed   = "crashed"
	StateMigrating = "migrating"
	StateUnknown   = "unknown"
)

// Guest OS families, which drive display adapter and driver defaults.
const (
	OSFamilyLinux   = "linux"
	OSFamilyWindows = "windows"
	OSFamilyBSD     = "bsd"
	OSFamilyOther   = "other"
)

// Snapshot consistency levels, surfaced honestly in the UI.
//
// Without a guest agent a snapshot is crash-consistent — the equivalent of
// pulling the power cord. It is usable, and the UI says exactly that rather
// than implying more.
const (
	ConsistencyCrash       = "crash"
	ConsistencyPaused      = "paused"
	ConsistencyFilesystem  = "filesystem"
	ConsistencyApplication = "application"
)

// maxSnapshotChain caps how deep a snapshot chain may go, and warnAtChain is
// where the UI starts saying so.
//
// Deep external-qcow2 chains destroy read performance and are the classic way a
// homelab VM becomes mysteriously slow. Copy-on-write pools do not have the
// problem, but the cap is uniform so an operator does not learn one rule per
// backend.
const (
	maxSnapshotChain = 16
	warnAtChain      = 8
)

// VM is one guest.
type VM struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	State         string `json:"state"`

	GuestOSFamily  string `json:"guestOsFamily,omitempty"`
	GuestOSVariant string `json:"guestOsVariant,omitempty"`

	VCPUs       int    `json:"vcpus"`
	CPUModel    string `json:"cpuModel"`
	CPUSockets  int    `json:"cpuSockets"`
	CPUCores    int    `json:"cpuCores"`
	CPUThreads  int    `json:"cpuThreads"`
	MemoryMB    int64  `json:"memoryMb"`
	MemoryMaxMB int64  `json:"memoryMaxMb,omitempty"`

	MachineType string `json:"machineType"`
	Firmware    string `json:"firmware"`
	SecureBoot  bool   `json:"secureBoot"`
	TPMEnabled  bool   `json:"tpmEnabled"`
	DisplayType string `json:"displayType"`
	// DisplayWidth and DisplayHeight are the boot resolution the adapter offers
	// the guest; zero leaves QEMU's default. A change applies at the next start.
	DisplayWidth  int `json:"displayWidth"`
	DisplayHeight int `json:"displayHeight"`
	// DisplayAdapter is DisplayType with Auto resolved, which is what the guest
	// boots with. The console reads it to say whether the guest can follow the window.
	DisplayAdapter string `json:"displayAdapter,omitempty"`
	// DisplayProfileID is the SPICE display profile the guest launches with,
	// Fast LAN when it never chose one. A change applies at the next start.
	DisplayProfileID string `json:"displayProfileId"`

	Autostart       bool `json:"autostart"`
	AutostartOrder  int  `json:"autostartOrder"`
	AutostartDelayS int  `json:"autostartDelaySeconds"`

	IsTemplate       bool   `json:"isTemplate"`
	TemplateSourceID string `json:"templateSourceId,omitempty"`

	// RunState is QEMU's own run state behind the mapped State. It is what
	// separates a deliberately paused guest from one QEMU stopped because its
	// disk backend failed, which State alone cannot express.
	RunState string `json:"runState,omitempty"`

	// VCPUsMax and MemoryCurrentMB are the hotplug ceiling and what the guest
	// actually holds now. MemoryMB stays the boot size: after a virtio-mem
	// resize the two differ, and showing only one of them is how a resize
	// looks like it did nothing.
	VCPUsMax        int   `json:"vcpusMax,omitempty"`
	MemoryCurrentMB int64 `json:"memoryCurrentMb,omitempty"`
	VirtioMem       bool  `json:"virtioMem,omitempty"`
	HugePages       bool  `json:"hugePages,omitempty"`

	// NUMAPolicy is none | pin | mirror. Most operators should never build a
	// topology by hand, so the choice is a policy and the agent derives the
	// -numa lines from it.
	NUMAPolicy    string `json:"numaPolicy,omitempty"`
	NUMAHostNodes []int  `json:"numaHostNodes,omitempty"`

	// VSockCID is this guest's AF_VSOCK context id, unique per host. Read-only
	// in the UI: it is allocated against a database constraint because a
	// collision is silent and reaches the wrong guest.
	VSockCID int  `json:"vsockCid,omitempty"`
	IOThread bool `json:"ioThread,omitempty"`

	GuestAgentState string `json:"guestAgentState,omitempty"`
	// GuestHostname, GuestOSName and GuestOSVersion are what the guest said it
	// is, which is the real answer — GuestOSFamily is only what was guessed at
	// create time. All three are untrusted and display-only.
	GuestHostname  string `json:"guestHostname,omitempty"`
	GuestOSName    string `json:"guestOsName,omitempty"`
	GuestOSVersion string `json:"guestOsVersion,omitempty"`
	// GuestReportedIPs is what the guest said its addresses are. Display only,
	// escaped at render, never turned into a link and never used for any
	// authorization decision.
	GuestReportedIPs []string `json:"guestReportedIps,omitempty"`

	UUID     string `json:"uuid"`
	UnitName string `json:"unitName,omitempty"`

	LastStartedAt *time.Time `json:"lastStartedAt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`

	// Computed per request rather than stored, because a stored answer would
	// be a guess about the present.
	HostAvailable     bool   `json:"hostAvailable"`
	UnavailableReason string `json:"unavailableReason,omitempty"`
	// DeleteProgress is set only while a removal is running, so the list can
	// show how far it has got without the caller holding the dialog open.
	DeleteProgress *DeleteProgress `json:"deleteProgress,omitempty"`
	// SnapshotProgress is set only while a snapshot is being taken, which is
	// what the row the UI draws before the snapshot exists reads.
	SnapshotProgress *DeleteProgress `json:"snapshotProgress,omitempty"`
	// RevertProgress is the same for a snapshot being restored. A revert stops
	// the guest, swaps its disks and starts it again, which is minutes rather
	// than the seconds an HTTP request should be held open for.
	RevertProgress *DeleteProgress `json:"revertProgress,omitempty"`
	// CloneProgress is how far this guest's disks have been copied while it
	// is being made as a clone, which a full clone of a large disk needs.
	CloneProgress *DeleteProgress `json:"cloneProgress,omitempty"`
	// EditableNow names the settings this guest will accept in its present
	// state, so the form disables the rest instead of deriving the rule again.
	EditableNow []string `json:"editableNow,omitempty"`

	// CurrentSnapshotID is the node the guest's disks currently descend from,
	// which is what makes the snapshot list a tree rather than a ladder: a new
	// snapshot hangs off this one, and reverting moves it rather than deleting
	// everything below.
	CurrentSnapshotID string `json:"currentSnapshotId,omitempty"`

	Disks     []Disk     `json:"disks,omitempty"`
	NICs      []NIC      `json:"nics,omitempty"`
	Snapshots []Snapshot `json:"snapshots,omitempty"`
	Devices   []Device   `json:"devices,omitempty"`
	CloudInit *CloudInit `json:"cloudInit,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Running reports whether the guest is up. A guest whose host is unreachable is
// not running as far as this plugin is concerned — it is unknown.
func (v VM) Running() bool { return v.State == StateRunning }

// Disk is one block device.
type Disk struct {
	ID   string `json:"id"`
	VMID string `json:"vmId"`
	// Notice is returned by a live add and never stored: "hotplug_unclaimed" when the
	// running guest did not pick the disk up (no PCI hot-plug support in its kernel).
	Notice string `json:"notice,omitempty"`
	// PoolID names addon-compute's row; PoolBackend and PoolTarget are the copy
	// this plugin can act on, because a plugin may only read its own tables.
	PoolID      string `json:"poolId"`
	PoolBackend string `json:"poolBackend"`
	PoolTarget  string `json:"poolTarget"`

	Slot        int    `json:"slot"`
	Bus         string `json:"bus"`
	VolumePath  string `json:"volumePath"`
	Format      string `json:"format"`
	SizeBytes   int64  `json:"sizeBytes"`
	BackingPath string `json:"backingPath,omitempty"`
	CacheMode   string `json:"cacheMode"`
	Discard     bool   `json:"discard"`
	ReadOnly    bool   `json:"readonly"`
	IsBoot      bool   `json:"isBoot"`
	IsCDROM     bool   `json:"isCdrom"`

	// BootIndex places this device explicitly in the boot order. Zero keeps
	// the old derivation from IsBoot and IsCDROM, which every guest created
	// before the order was selectable still boots by.
	BootIndex int `json:"bootIndex,omitempty"`
	// ShareRW lets a second guest open this disk at the same time. Required by
	// a cluster filesystem and catastrophic without one.
	ShareRW bool `json:"shareRw,omitempty"`

	// I/O limits, zero meaning unlimited.
	ReadIOPS  int64 `json:"readIops,omitempty"`
	WriteIOPS int64 `json:"writeIops,omitempty"`
	ReadBPS   int64 `json:"readBps,omitempty"`
	WriteBPS  int64 `json:"writeBps,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// GuestLog is a bounded tail of one of a guest's host-side units.
//
// Notices are the reasons a read was partial — journalctl absent, or the unit
// having no entries yet — kept separate from the lines so the panel can say
// why it is empty instead of looking like a healthy guest.
type GuestLog struct {
	Source  string   `json:"source"`
	Lines   []string `json:"lines"`
	Notices []string `json:"notices,omitempty"`
}

// PoolRef names one of addon-compute's pools.
//
// The backend and target travel with the id because this plugin may only read
// its own tables and the agent acts on a target rather than an id — the same
// denormalization the disk rows carry.
type PoolRef struct {
	ID      string
	Backend string
	Target  string
}

// PoolVolume is one disk image in a pool that no guest refers to.
//
// It exists so a machine can be built on a disk that is already there — the one
// a guest deleted without its volumes left behind, or one restored from a
// backup — rather than only ever on a freshly allocated one.
type PoolVolume struct {
	Name       string `json:"name"`
	VolumePath string `json:"volumePath"`
	Format     string `json:"format,omitempty"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	IsBlock    bool   `json:"isBlock,omitempty"`
}

// NIC is one network interface.
type NIC struct {
	ID   string `json:"id"`
	VMID string `json:"vmId"`
	// Notice is Disk.Notice for an interface.
	Notice    string `json:"notice,omitempty"`
	NetworkID string `json:"networkId"`
	Bridge    string `json:"bridge"`
	VLANTag   int    `json:"vlanTag,omitempty"`
	// AttachKind is empty or "bridge" for a tap on a bridge, "macvlan" for a
	// child stacked straight on ParentLink. Denormalised from
	// the network, like Bridge, so a guest start needs no network lookup.
	AttachKind string `json:"attachKind,omitempty"`
	ParentLink string `json:"parentLink,omitempty"`
	LinkMode   string `json:"linkMode,omitempty"`

	Slot    int    `json:"slot"`
	Model   string `json:"model"`
	MAC     string `json:"mac"`
	TapName string `json:"tapName,omitempty"`
	// PCIAddr is the slot QEMU gave this NIC the first time it ran, recorded
	// so every later start pins it. Without the pin, any device added ahead of
	// the NIC renames the guest's interface and costs it its address.
	PCIAddr string `json:"pciAddr,omitempty"`
	// LinkDown is a cable the operator pulled. It survives a restart, which is
	// what makes a live link change a setting rather than a moment.
	LinkDown bool `json:"linkDown,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Snapshot is one point-in-time copy.
type Snapshot struct {
	ID          string `json:"id"`
	VMID        string `json:"vmId"`
	ParentID    string `json:"parentId,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Mechanism   string `json:"mechanism"`
	Consistency string `json:"consistency"`
	WithMemory  bool   `json:"withMemory"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
	ChainDepth  int    `json:"chainDepth"`
	Handle      string `json:"handle"`
	// ChildCount is how many snapshots hang directly off this one. Two or more
	// cannot be merged away without rewriting a sibling branch's history, so
	// the UI refuses the delete rather than offering one that corrupts.
	ChildCount int `json:"childCount"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Device is a host device passed through.
type Device struct {
	ID          string    `json:"id"`
	VMID        string    `json:"vmId"`
	Kind        string    `json:"kind"`
	HostAddress string    `json:"hostAddress"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// HostDevice is one passthrough candidate on a host, as the attach dialog
// lists it. Assignable is false for a device the host is still using; Reason
// says which of the two reasons applies.
type HostDevice struct {
	Kind        string `json:"kind"`
	Address     string `json:"address"`
	Description string `json:"description,omitempty"`
	Vendor      string `json:"vendor,omitempty"`
	Product     string `json:"product,omitempty"`
	Driver      string `json:"driver,omitempty"`
	Class       string `json:"class,omitempty"`
	IOMMUGroup  string `json:"iommuGroup,omitempty"`
	Assignable  bool   `json:"assignable"`
	Reason      string `json:"reason,omitempty"`
	// AttachedTo lists the guests that already have this device, filled in by
	// the backend from its own rows; the agent never sets it.
	AttachedTo []DeviceHolder `json:"attachedTo,omitempty"`
}

// DeviceHolder is a guest that already has a passthrough device, so the attach
// dialog can warn before a second guest is given the same hardware.
type DeviceHolder struct {
	VMID string `json:"vmId"`
	Name string `json:"name"`
}

// CloudInit is a guest's first-boot configuration.
//
// PasswordHash is deliberately not in the JSON: it is write-only on this API.
// It is a hash rather than a plaintext, but there is still no reason for it to
// travel back to a browser, and HasPassword is what lets the edit form say
// "a password is set" instead of rendering an empty field that looks like the
// value was lost.
type CloudInit struct {
	VMID        string   `json:"vmId"`
	Hostname    string   `json:"hostname,omitempty"`
	Timezone    string   `json:"timezone,omitempty"`
	SSHKeys     []string `json:"sshKeys,omitempty"`
	DefaultUser string   `json:"defaultUser,omitempty"`
	HasPassword bool     `json:"hasPassword"`

	NetworkMode   string   `json:"networkMode"`
	StaticIP      string   `json:"staticIp,omitempty"`
	StaticGateway string   `json:"staticGateway,omitempty"`
	StaticDNS     []string `json:"staticDns,omitempty"`

	UserDataExtra string `json:"userDataExtra,omitempty"`
	InstanceID    string `json:"instanceId"`
}

// Stats is one sample of a running guest.
type Stats struct {
	VMID           string    `json:"vmId"`
	CPUPercent     float64   `json:"cpuPercent"`
	MemoryBytes    int64     `json:"memoryBytes"`
	MemoryMaxBytes int64     `json:"memoryMaxBytes"`
	BalloonBytes   int64     `json:"balloonBytes,omitempty"`
	DiskReadBytes  int64     `json:"diskReadBytes"`
	DiskWriteBytes int64     `json:"diskWriteBytes"`
	NetRxBytes     int64     `json:"netRxBytes"`
	NetTxBytes     int64     `json:"netTxBytes"`
	SampledAt      time.Time `json:"sampledAt"`
}

// --- Request bodies -------------------------------------------------------

// CreateRequest is the create dialog's body.
//
// Two paths, both first class: a cloud image plus cloud-init (the fast path,
// and the dialog's default), or an ISO to install from manually.
type CreateRequest struct {
	// RequestedBy is the signed-in user, set by the handler and never read from
	// the body: vault secrets are read with that user's secrets.view.
	RequestedBy string `json:"-"`

	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`

	GuestOSFamily  string `json:"guestOsFamily,omitempty"`
	GuestOSVariant string `json:"guestOsVariant,omitempty"`

	VCPUs       int    `json:"vcpus"`
	CPUModel    string `json:"cpuModel,omitempty"`
	MemoryMB    int64  `json:"memoryMb"`
	MemoryMaxMB int64  `json:"memoryMaxMb,omitempty"`

	MachineType string `json:"machineType,omitempty"`
	Firmware    string `json:"firmware,omitempty"`
	SecureBoot  bool   `json:"secureBoot,omitempty"`
	TPMEnabled  bool   `json:"tpmEnabled,omitempty"`

	// Storage. PoolBackend and PoolTarget come from the compute plugin's pool
	// list via the frontend, because this plugin cannot read that table.
	PoolID      string `json:"poolId"`
	PoolBackend string `json:"poolBackend"`
	PoolTarget  string `json:"poolTarget"`
	DiskSizeGB  int64  `json:"diskSizeGb"`
	// DiskBus overrides the bus the guest's OS family would otherwise get.
	DiskBus string `json:"diskBus,omitempty"`
	// DiskVolumePath adopts a volume that already exists in the pool instead of
	// allocating one. Nothing is written to it, so whatever is on it — an
	// installed system, a disk restored from a backup — is what the guest
	// boots, and DiskSizeGB is ignored.
	DiskVolumePath string `json:"diskVolumePath,omitempty"`
	// DiskFormat and DiskSizeBytes describe an adopted volume as the host
	// reported it. They are recorded rather than trusted for anything: the
	// agent is asked for the pool's volumes again before one is accepted.
	DiskFormat    string `json:"diskFormat,omitempty"`
	DiskSizeBytes int64  `json:"diskSizeBytes,omitempty"`

	// Networking. Same story for the bridge.
	NetworkID string `json:"networkId"`
	Bridge    string `json:"bridge"`
	VLANTag   int    `json:"vlanTag,omitempty"`
	// AttachKind is empty or "bridge" for a tap on a bridge, "macvlan" for a
	// child stacked straight on ParentLink. Denormalised from
	// the network, like Bridge, so a guest start needs no network lookup.
	AttachKind string `json:"attachKind,omitempty"`
	ParentLink string `json:"parentLink,omitempty"`
	LinkMode   string `json:"linkMode,omitempty"`

	// Path A: a cloud image plus cloud-init.
	ImageID        string            `json:"imageId,omitempty"`
	ImageLocalPath string            `json:"imageLocalPath,omitempty"`
	CloudInit      *CloudInitRequest `json:"cloudInit,omitempty"`

	// Path B: install from an ISO.
	ISOPaths []string `json:"isoPaths,omitempty"`

	// Path C: neither. The guest is defined with its disk and nothing to boot
	// from, which is what an adopted volume, a PXE install or a disc attached
	// later all want.

	Autostart bool `json:"autostart,omitempty"`
}

// CloudInitRequest carries the first-boot settings, including the one plaintext
// value in this whole API.
type CloudInitRequest struct {
	Hostname    string   `json:"hostname,omitempty"`
	Timezone    string   `json:"timezone,omitempty"`
	SSHKeys     []string `json:"sshKeys,omitempty"`
	DefaultUser string   `json:"defaultUser,omitempty"`
	// Password is hashed immediately on receipt and never stored or forwarded
	// in this form. The seed image is readable by anything that can read the
	// pool, so a plaintext there would be a credential on every guest's disk.
	Password string `json:"password,omitempty"`
	// PasswordSecret and SSHKeySecrets name vault secrets read, as the
	// requesting user, at create time. The values are copied once — cloud-init
	// applies them on the first boot only — so a later rotation does not reach
	// the guest. PasswordSecret and Password are mutually exclusive.
	PasswordSecret string   `json:"passwordSecret,omitempty"`
	SSHKeySecrets  []string `json:"sshKeySecrets,omitempty"`

	NetworkMode   string   `json:"networkMode,omitempty"`
	StaticIP      string   `json:"staticIp,omitempty"`
	StaticGateway string   `json:"staticGateway,omitempty"`
	StaticDNS     []string `json:"staticDns,omitempty"`

	UserDataExtra string `json:"userDataExtra,omitempty"`
	// InstallGuestAgent is on by default in the dialog: it is what makes
	// filesystem-consistent snapshots and real IP reporting possible.
	InstallGuestAgent bool `json:"installGuestAgent,omitempty"`
}

// UpdateRequest edits a defined guest.
type UpdateRequest struct {
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	VCPUs           *int    `json:"vcpus,omitempty"`
	MemoryMB        *int64  `json:"memoryMb,omitempty"`
	MemoryMaxMB     *int64  `json:"memoryMaxMb,omitempty"`
	CPUModel        *string `json:"cpuModel,omitempty"`
	Autostart       *bool   `json:"autostart,omitempty"`
	AutostartOrder  *int    `json:"autostartOrder,omitempty"`
	AutostartDelayS *int    `json:"autostartDelaySeconds,omitempty"`
	MachineType     *string `json:"machineType,omitempty"`
	Firmware        *string `json:"firmware,omitempty"`
	SecureBoot      *bool   `json:"secureBoot,omitempty"`
	TPMEnabled      *bool   `json:"tpmEnabled,omitempty"`
	DisplayType     *string `json:"displayType,omitempty"`
	DisplayWidth    *int    `json:"displayWidth,omitempty"`
	DisplayHeight   *int    `json:"displayHeight,omitempty"`

	// The QEMU feature set. Every one of these is read while building the
	// command line, so all of them are cold settings: changing one describes a
	// machine other than the one currently executing.
	VCPUsMax      *int    `json:"vcpusMax,omitempty"`
	VirtioMem     *bool   `json:"virtioMem,omitempty"`
	HugePages     *bool   `json:"hugePages,omitempty"`
	NUMAPolicy    *string `json:"numaPolicy,omitempty"`
	NUMAHostNodes *[]int  `json:"numaHostNodes,omitempty"`
	IOThread      *bool   `json:"ioThread,omitempty"`
	// VSockEnabled asks for a context id rather than naming one: allocation is
	// the server's job, against a unique index, because a collision is silent
	// and lands the channel on the wrong guest.
	VSockEnabled *bool `json:"vsockEnabled,omitempty"`
	// DisplayProfileID picks the guest's SPICE display profile; empty means
	// Fast LAN. Allowed while running: it is read only when the guest starts.
	DisplayProfileID *string `json:"displayProfileId,omitempty"`
}

// LifecycleRequest carries the options a start/stop takes.
type LifecycleRequest struct {
	// GraceSeconds is the ACPI powerdown window before escalating to a hard
	// stop. Zero takes the default.
	GraceSeconds int `json:"graceSeconds,omitempty"`
}

// RevertRequest is the revert dialog's body.
//
// StopFirst is consent, not convenience: reverting swaps the disks under a
// running QEMU, so the guest has to go down and the operator is the one who
// says it may.
type RevertRequest struct {
	StopFirst    bool `json:"stopFirst,omitempty"`
	RestartAfter bool `json:"restartAfter,omitempty"`
}

// DiskRequest adds or resizes a disk.
type DiskRequest struct {
	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend,omitempty"`
	PoolTarget  string `json:"poolTarget,omitempty"`
	SizeGB      int64  `json:"sizeGb"`
	Bus         string `json:"bus,omitempty"`
	// ShareRW and BootIndex are pointers because both have a meaningful zero:
	// "not shared" and "derive the boot order as before" are settings, not
	// absences, and a plain value could not tell them from an unset field.
	ShareRW   *bool `json:"shareRw,omitempty"`
	BootIndex *int  `json:"bootIndex,omitempty"`
}

// NICRequest adds an interface.
type NICRequest struct {
	NetworkID  string `json:"networkId"`
	Bridge     string `json:"bridge"`
	VLANTag    int    `json:"vlanTag,omitempty"`
	AttachKind string `json:"attachKind,omitempty"`
	ParentLink string `json:"parentLink,omitempty"`
	LinkMode   string `json:"linkMode,omitempty"`
	Model      string `json:"model,omitempty"`
}

// NICUpdateRequest changes an interface. LinkUp applies live; Model needs the
// guest stopped, because the device type is fixed once QEMU has created it.
type NICUpdateRequest struct {
	LinkUp *bool  `json:"linkUp,omitempty"`
	Model  string `json:"model,omitempty"`
}

// SnapshotRequest takes a snapshot.
type SnapshotRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// WithMemory captures the guest's RAM as well, so a revert resumes exactly
	// where it was rather than booting.
	WithMemory bool `json:"withMemory,omitempty"`
	// PauseGuest stops the guest's vCPUs while the disks are frozen. It is the
	// fallback for a guest with no agent to quiesce it, and the scheduler turns
	// it on by itself in that case.
	PauseGuest bool `json:"pauseGuest,omitempty"`
}

// CloneRequest copies a guest.
type CloneRequest struct {
	Name string `json:"name"`
	// Linked shares data with the source until either is written. Instant and
	// space-efficient, at the cost of the source becoming undeletable while
	// the clone exists.
	Linked bool `json:"linked,omitempty"`
}

// DeviceRequest attaches a host device.
type DeviceRequest struct {
	Kind        string `json:"kind"`
	HostAddress string `json:"hostAddress"`
	Description string `json:"description,omitempty"`
}

// MediaRequest inserts an installer disc.
//
// SourcePath is a path on the host, normally an ISO from the compute plugin's
// image library; ImageID is recorded only so the UI can show where it came from.
type MediaRequest struct {
	ImageID    string `json:"imageId,omitempty"`
	SourcePath string `json:"sourcePath"`
	Label      string `json:"label,omitempty"`
}

// DeleteOptions govern a destroy.
type DeleteOptions struct {
	// DeleteVolumes destroys the guest's disks with it. Without it the disks
	// are left in the pool, which is what an operator moving a guest between
	// hosts wants.
	DeleteVolumes bool `json:"deleteVolumes,omitempty"`
}
