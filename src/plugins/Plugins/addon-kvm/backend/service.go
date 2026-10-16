// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Service holds every query and every agent call.
type Service struct {
	db     plugin.DBHandle
	rpc    *agentBridge
	logger pluginLogger
	// audit records what the background workers do. The handlers audit their
	// own requests; a scheduled snapshot and a conversion that finishes long
	// after its request have no handler left to do it for them.
	audit auditEmitter
	// lifecycle puts a guest's state changes on the Logging timeline, which
	// before this carried Docker subjects only. nil on a host that wired no
	// recorder, which every write below tolerates.
	lifecycle plugin.LifecycleHandle
	// workflows fires the vm-trigger node on the transitions recordLifecycle writes.
	// nil on a host that wired no workflow bridge, which leaves the trigger inert.
	workflows plugin.WorkflowHandle
	// vault reads the secrets a create names; nil when the host gave none.
	vault vaultView
	// backupLog puts this plugin's backup and restore events in the host's
	// Backups log, beside the container ones. nil on a host that wired none.
	backupLog plugin.BackupLogHandle
	// environments names the host's environments, which this plugin's own
	// tables do not carry and its DB handle cannot reach.
	environments plugin.EnvironmentHandle
	// workDir is the host's data directory. An import is staged under it until
	// the target host has pulled and converted it.
	workDir string
	// imports authorizes an agent to pull one staged upload over HTTP.
	imports *importGrants
	// importRuns holds the inspect and convert jobs in flight, which outlive the
	// requests that started them.
	importRuns *importJobs

	// deleteProgress holds how far each in-flight delete has got, keyed by
	// guest id. In memory rather than a column on purpose: a plugin migration
	// does not run on an install that already exists, and a percentage that
	// dies with the process is the right lifetime for one anyway — an
	// interrupted delete is not resumed, it is retried.
	deleteMu       sync.Mutex
	deleteProgress map[string]DeleteProgress
	// snapshotProgress is the same for a snapshot being taken, and
	// revertProgress for one being restored, under the same mutex: all three
	// are short-lived counters on the same guests.
	snapshotProgress map[string]DeleteProgress
	revertProgress   map[string]DeleteProgress

	// storage is the host's storage bridge, used for backup destinations. nil
	// on a host that wired none, which the backup paths refuse by name.
	storage plugin.StorageHandle
	// backupGrants authorises an agent to stream one run's artifacts back.
	backupGrants *backupGrants
	// backupMu guards backupUploads, which holds where an in-flight run's
	// incoming artifacts should go. In memory because it is only meaningful
	// while the run is live — a McHarbor restart ends the run either way.
	backupMu        sync.RWMutex
	backupUploads   map[string]backupUploadTarget
	backupDownloads map[string]backupDownload

	// capabilities caches each host's QEMU feature set. In memory because it
	// describes the host's installed packages rather than anything McHarbor
	// owns: persisting it would survive the package upgrade that invalidates
	// it, and re-asking costs one message against an answer the agent has
	// already cached.
	capabilityMu sync.RWMutex
	capabilities map[string]*cachedCapability

	// migrationJobs and relay are built on first use (migrationState), so the
	// constructor's signature stays what every caller and test already passes.
	// In memory because a migration dies with the process that drives it.
	migrationOnce sync.Once
	migrationJobs *migrationJobs
	relay         *migrationRelay

	// cloneProgress is how far a clone's disk copies have got, keyed by the
	// clone's id, under deleteMu like the other progress maps.
	cloneProgress map[string]DeleteProgress
}

// DeleteProgress is how far a guest's removal has got, as the list and the
// dialog read it.
type DeleteProgress struct {
	Percent int    `json:"percent"`
	Step    string `json:"step,omitempty"`
}

// recordDeleteProgress stores one progress report from the host.
func (s *Service) recordDeleteProgress(vmID, percent, step string) {
	if vmID == "" {
		return
	}
	value, err := strconv.Atoi(percent)
	if err != nil {
		return
	}
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	if s.deleteProgress == nil {
		s.deleteProgress = make(map[string]DeleteProgress)
	}
	s.deleteProgress[vmID] = DeleteProgress{Percent: value, Step: step}
}

// recordSnapshotProgress stores how far a snapshot has got.
//
// Same shape as the delete counter beside it, and for the same reason: the row
// the UI wants to draw does not exist yet, so the progress hangs off the guest
// until it does.
func (s *Service) recordSnapshotProgress(vmID, percent, step string) {
	if vmID == "" {
		return
	}
	value, err := strconv.Atoi(percent)
	if err != nil {
		return
	}
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	if s.snapshotProgress == nil {
		s.snapshotProgress = make(map[string]DeleteProgress)
	}
	if step == "done" || value >= 100 {
		delete(s.snapshotProgress, vmID)
		return
	}
	s.snapshotProgress[vmID] = DeleteProgress{Percent: value, Step: step}
}

// snapshotProgressFor reports a guest's snapshot progress, if one is running.
func (s *Service) snapshotProgressFor(vmID string) *DeleteProgress {
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	progress, ok := s.snapshotProgress[vmID]
	if !ok {
		return nil
	}
	return &progress
}

// clearSnapshotProgress drops the counter once the call that was making it has
// returned, however it returned.
func (s *Service) clearSnapshotProgress(vmID string) {
	s.deleteMu.Lock()
	delete(s.snapshotProgress, vmID)
	s.deleteMu.Unlock()
}

// recordRevertProgress stores how far a revert has got.
func (s *Service) recordRevertProgress(vmID string, percent int, step string) {
	if vmID == "" {
		return
	}
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	if s.revertProgress == nil {
		s.revertProgress = make(map[string]DeleteProgress)
	}
	if step == "done" {
		delete(s.revertProgress, vmID)
		return
	}
	s.revertProgress[vmID] = DeleteProgress{Percent: percent, Step: step}
}

// revertProgressFor reports a guest's revert progress, if one is running.
func (s *Service) revertProgressFor(vmID string) *DeleteProgress {
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	progress, ok := s.revertProgress[vmID]
	if !ok {
		return nil
	}
	return &progress
}

// recordCloneProgress stores one clone progress report, keyed by the clone.
func (s *Service) recordCloneProgress(vmID, percent, step string) {
	value, err := strconv.Atoi(percent)
	if vmID == "" || err != nil {
		return
	}
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	if step == "done" {
		delete(s.cloneProgress, vmID)
		return
	}
	if s.cloneProgress == nil {
		s.cloneProgress = make(map[string]DeleteProgress)
	}
	s.cloneProgress[vmID] = DeleteProgress{Percent: value, Step: step}
}

// cloneProgressFor reports a clone's copy progress, if one is running.
func (s *Service) cloneProgressFor(vmID string) *DeleteProgress {
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	progress, ok := s.cloneProgress[vmID]
	if !ok {
		return nil
	}
	return &progress
}

// deleteProgressFor reports a guest's removal progress, if one is running.
func (s *Service) deleteProgressFor(vmID string) *DeleteProgress {
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	progress, ok := s.deleteProgress[vmID]
	if !ok {
		return nil
	}
	return &progress
}

// forgetDeleteProgress drops a finished delete's entry, so a guest that failed
// and is tried again does not start at the last attempt's percentage.
func (s *Service) forgetDeleteProgress(vmID string) {
	s.deleteMu.Lock()
	delete(s.deleteProgress, vmID)
	s.deleteMu.Unlock()
}

// Errors the handlers map onto stable codes. The text never reaches a client.
var (
	ErrNotFound   = errors.New("not found")
	ErrNameTaken  = errors.New("name already in use")
	ErrRunning    = errors.New("this guest is running")
	ErrNotRunning = errors.New("this guest is not running")
	// ErrStatsUnavailable means the agent answered but reported no sample.
	// Distinct from an error so the UI can say "this agent does not report
	// metrics" rather than drawing an idle guest.
	ErrStatsUnavailable = errors.New("this agent does not report guest metrics")
	ErrHostUnavailable  = errors.New("this guest's host cannot be reached")
	ErrDiskShrink       = errors.New("a disk cannot be made smaller")
	ErrChainTooDeep     = errors.New("this guest's snapshot chain is too deep")
	ErrTemplateInUse    = errors.New("this template still has linked clones")
	ErrTemplateRunning  = errors.New("a template cannot be started")
	ErrNoCapability     = errors.New("this host cannot run virtual machines")
	ErrMediaPath        = errors.New("that is not a usable installer disc")
	// ErrNoResizableMemory means the guest has no virtio-mem device, so its
	// memory cannot change while it runs. A restart with the feature enabled
	// is the fix, which is a different sentence from "resizing failed".
	ErrNoResizableMemory = errors.New("this guest has no resizable memory device")
	// ErrInvalidResize covers a target outside what the guest was started
	// with, and a limit so small the guest would look hung.
	ErrInvalidResize = errors.New("that is not a usable size")
	// ErrUnsupportedImageOp means the caller asked for a qemu-img subcommand
	// outside the read-only set.
	ErrUnsupportedImageOp = errors.New("that image operation is not available")
	// ErrShareTagTaken means another share on the same guest already mounts
	// under that tag, which the guest would resolve to one of them at random.
	ErrShareTagTaken = errors.New("that mount tag is already used by this guest")
	// ErrSharePathRefused means the host path is outside the roots the
	// operator allowed. Sharing / or /etc hands the host to the guest.
	ErrSharePathRefused = errors.New("that path is not inside an allowed share root")
	// ErrGuestDiscPending is the host still building the disc at startup, which
	// is a "come back in a moment" rather than a failure.
	ErrGuestDiscPending = errors.New("the host is still preparing the guest agent disc")
	ErrNotMedia         = errors.New("that disk is not an installer disc")
	ErrNameRequired     = errors.New("a name is required")
	ErrNameTooLong      = errors.New("that name is too long")
	ErrNameInvalid      = errors.New("that name contains a line break")
	// ErrSnapshotBranched means the snapshot has more than one branch below it.
	// Merging it away would rewrite a sibling branch's history, so it is
	// refused rather than offered and regretted.
	ErrSnapshotBranched = errors.New("this snapshot has more than one branch below it")
	ErrSnapshotTooMany  = errors.New("this guest has too many snapshots")
)

// deleteCeiling bounds a background delete. Generous because removing a large
// volume is disk-bound, finite because a wedged host must not leave a guest
// reading "removing" forever.
const deleteCeiling = 2 * time.Hour

// maxRows bounds every unpaginated select.
const maxRows = 1000

// Agent message types this plugin sends. They are the protocol's own strings —
// the host validates them against its allow-list before anything reaches the
// agent, so a typo here fails closed rather than reaching a root process.
const (
	msgVMDefine    = "vm_define"
	msgVMStart     = "vm_start"
	msgVMShutdown  = "vm_shutdown"
	msgVMStop      = "vm_stop"
	msgVMReset     = "vm_reset"
	msgVMPause     = "vm_pause"
	msgVMResume    = "vm_resume"
	msgVMDelete    = "vm_delete"
	msgVMQuery     = "vm_query"
	msgVMStats     = "vm_stats"
	msgHostDevList = "host_device_list"

	msgVMLog        = "vm_log"
	msgVMDiskList   = "vm_disk_list"
	msgVMDiskCreate = "vm_disk_create"
	msgVMDiskResize = "vm_disk_resize"
	msgVMDiskDelete = "vm_disk_delete"

	msgVMSnapshotCreate = "vm_snapshot_create"
	msgVMSnapshotRevert = "vm_snapshot_revert"
	msgVMSnapshotDelete = "vm_snapshot_delete"
	msgVMImportInspect  = "vm_import_inspect"
	msgVMImportConvert  = "vm_import_convert"
	msgVMImportStage    = "vm_import_stage"
	msgVMImportDiscard  = "vm_import_discard"

	msgVMCloneRequest   = "vm_clone_request"
	msgVMGuestQuery     = "vm_guest_query"
	msgVMGuestAgentDisc = "vm_guest_agent_disc"
)

// agentGuestDiscPending is the reply code the host sends while its startup
// pass is still building the disc.
const agentGuestDiscPending = "guest_disc_pending"

const ()

// NewService builds the service.
func NewService(
	db plugin.DBHandle,
	rpc any,
	auditor auditEmitter,
	lifecycle plugin.LifecycleHandle,
	backupLog plugin.BackupLogHandle,
	environments plugin.EnvironmentHandle,
	storage plugin.StorageHandle,
	workDir string,
	logger pluginLogger,
) *Service {
	return &Service{db: db, rpc: newAgentBridge(rpc), audit: auditor, lifecycle: lifecycle,
		backupLog: backupLog, environments: environments,
		storage: storage, workDir: workDir, logger: logger,
		imports: newImportGrants(), importRuns: newImportJobs(),
		backupGrants:  newBackupGrants(),
		backupUploads: map[string]backupUploadTarget{}}
}

// recordAudit writes an audit row for work nobody asked for directly.
//
// Unattributed by design: a schedule firing on its own has no actor, and
// naming the operator who created the schedule would be a lie about who acted.
func (s *Service) recordAudit(event string, fields map[string]any) {
	if s.audit == nil {
		return
	}
	if err := s.audit.Emit(event, fields); err != nil {
		s.logger.Warn("kvm: audit emit failed", "event", event, "error", err)
	}
}

// --- Agent plumbing -------------------------------------------------------

// agentPayload is the vm-family payload this plugin builds.
//
// It mirrors the protocol's VMPayload without importing it: the host's agent-rpc
// bridge takes opaque JSON precisely so the guest command surface stays
// with the plugins that own it rather than being mirrored into core.
type agentPayload struct {
	VMID          string           `json:"vmId,omitempty"`
	Spec          *agentSpec       `json:"spec,omitempty"`
	GraceSeconds  int              `json:"graceSeconds,omitempty"`
	DeleteVolumes bool             `json:"deleteVolumes,omitempty"`
	Disk          *agentDisk       `json:"disk,omitempty"`
	SnapshotID    string           `json:"snapshotId,omitempty"`
	SnapshotName  string           `json:"snapshotName,omitempty"`
	ChildSnapshot string           `json:"childSnapshot,omitempty"`
	WithMemory    bool             `json:"withMemory,omitempty"`
	PauseGuest    bool             `json:"pauseGuest,omitempty"`
	Consistency   string           `json:"consistency,omitempty"`
	Clone         *agentClone      `json:"clone,omitempty"`
	Import        *agentImportSpec `json:"import,omitempty"`
	// Migrate drives one host's half of a migration; the reply carries it
	// back with what that half produced. See migrate_wire.go.
	Migrate *agentMigrate `json:"migrate,omitempty"`
	// SpiceTicket answers vm_spice_ticket. Its password goes into the .vv file
	// the operator downloads and nowhere else.
	SpiceTicket *agentSpiceTicket `json:"spiceTicket,omitempty"`
	GuestQuery  string            `json:"guestQuery,omitempty"`
	// Live resize requests. Named for the target rather than reusing the
	// spec's own fields, because a resize is not a redefinition: the guest's
	// boot size stays what it was.
	TargetMemoryMB int64             `json:"targetMemoryMb,omitempty"`
	TargetVCPUs    int               `json:"targetVcpus,omitempty"`
	ImageOp        *agentImageOp     `json:"imageOp,omitempty"`
	Backup         *agentBackupSpec  `json:"backup,omitempty"`
	Restore        *agentRestoreSpec `json:"restore,omitempty"`
	GuestFamily    string            `json:"guestFamily,omitempty"`
	States         []agentState      `json:"states,omitempty"`
	// Hotplug is one live hardware change; Spec travels with it.
	Hotplug *agentHotplug `json:"hotplug,omitempty"`

	// Reply-only.
	//
	// Disks carries a disk whose active file moved. A live qcow2 snapshot
	// freezes the file the guest was writing to and hands QEMU a new overlay,
	// so the path stored here stops being the one the guest actually uses;
	// persisting what comes back is what stops the next start opening a frozen
	// snapshot instead of the live disk.
	Disks        []agentDisk        `json:"disks,omitempty"`
	ImportResult *agentImportResult `json:"importResult,omitempty"`
	State        *agentState        `json:"state,omitempty"`
	Guest        *agentGuest        `json:"guest,omitempty"`
	Stats        *agentStats        `json:"stats,omitempty"`
	HostDevices  []HostDevice       `json:"hostDevices,omitempty"`
	// QEMUCapability answers vm_qemu_capability. It is absent from every other
	// reply, and absent from this one on an agent too old to know the message
	// — which reads as "no features", the safe direction.
	QEMUCapability *agentQEMUCapability `json:"qemuCapability,omitempty"`
	// Resize answers vm_memory_set and vm_cpu_set with what the guest actually
	// did, which on an unplug is regularly not what was asked.
	Resize *agentResizeResult `json:"resize,omitempty"`
	// ImageInfo is qemu-img's own JSON, passed through: the shape differs per
	// subcommand and per QEMU version, and the consumer is a diagnostic panel.
	ImageInfo json.RawMessage `json:"imageInfo,omitempty"`
	// BackupResult answers vm_backup_run, one entry per disk. Its
	// FellBackToFull fields are what stop the database recording an
	// incremental the agent did not actually take.
	BackupResult []agentBackupDiskResult `json:"backupResult,omitempty"`
	BackupBytes  int64                   `json:"backupBytes,omitempty"`
	Volumes      []agentVolume           `json:"volumes,omitempty"`
	LogSource    string                  `json:"logSource,omitempty"`
	LogTail      int                     `json:"logTail,omitempty"`
	LogLines     []string                `json:"logLines,omitempty"`
	LogNotices   []string                `json:"logNotices,omitempty"`
	MediaPath    string                  `json:"mediaPath,omitempty"`
	NICAddrs     map[string]string       `json:"nicAddrs,omitempty"`
	MediaLabel   string                  `json:"mediaLabel,omitempty"`
	Orphans      []string                `json:"orphans,omitempty"`
	Success      bool                    `json:"success,omitempty"`
	Error        string                  `json:"error,omitempty"`
	ErrorCode    string                  `json:"errorCode,omitempty"`
	// Notice qualifies a successful reply; see agentHotplugUnclaimed.
	Notice string `json:"notice,omitempty"`
}

type agentSpec struct {
	Name           string `json:"name"`
	UUID           string `json:"uuid"`
	GuestOSFamily  string `json:"guestOsFamily,omitempty"`
	GuestOSVariant string `json:"guestOsVariant,omitempty"`
	VCPUs          int    `json:"vcpus"`
	CPUModel       string `json:"cpuModel,omitempty"`
	CPUSockets     int    `json:"cpuSockets,omitempty"`
	CPUCores       int    `json:"cpuCores,omitempty"`
	CPUThreads     int    `json:"cpuThreads,omitempty"`
	MemoryMB       int64  `json:"memoryMb"`
	MemoryMaxMB    int64  `json:"memoryMaxMb,omitempty"`
	MachineType    string `json:"machineType"`
	Firmware       string `json:"firmware,omitempty"`
	SecureBoot     bool   `json:"secureBoot,omitempty"`
	TPMEnabled     bool   `json:"tpmEnabled,omitempty"`
	DisplayType    string `json:"displayType,omitempty"`
	DisplayWidth   int    `json:"displayWidth,omitempty"`
	DisplayHeight  int    `json:"displayHeight,omitempty"`
	// SpiceOptions come from the guest's display profile; the agent applies them
	// only when it starts a SPICE server.
	SpiceOptions *agentSpiceOptions `json:"spiceOptions,omitempty"`

	// The QEMU feature set. Each of these changes how the guest is launched,
	// so a change to any of them is a restart rather than a live edit — with
	// the two exceptions the resize endpoints cover.
	VCPUsMax      int          `json:"vcpusMax,omitempty"`
	VirtioMem     bool         `json:"virtioMem,omitempty"`
	HugePages     bool         `json:"hugePages,omitempty"`
	NUMAPolicy    string       `json:"numaPolicy,omitempty"`
	NUMAHostNodes []int        `json:"numaHostNodes,omitempty"`
	VSockCID      int          `json:"vsockCid,omitempty"`
	IOThread      bool         `json:"ioThread,omitempty"`
	Shares        []agentShare `json:"shares,omitempty"`

	Disks     []agentDisk     `json:"disks,omitempty"`
	NICs      []agentNIC      `json:"nics,omitempty"`
	Devices   []agentDevice   `json:"devices,omitempty"`
	CloudInit *agentCloudInit `json:"cloudInit,omitempty"`
	ISOPaths  []string        `json:"isoPaths,omitempty"`
	UnitName  string          `json:"unitName,omitempty"`
}

type agentVolume struct {
	Name       string `json:"name"`
	VolumePath string `json:"volumePath"`
	Format     string `json:"format,omitempty"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	IsBlock    bool   `json:"isBlock,omitempty"`
}

type agentDisk struct {
	DiskID      string `json:"diskId,omitempty"`
	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend,omitempty"`
	PoolTarget  string `json:"poolTarget,omitempty"`
	Slot        int    `json:"slot"`
	Bus         string `json:"bus,omitempty"`
	VolumePath  string `json:"volumePath,omitempty"`
	Format      string `json:"format,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
	BackingPath string `json:"backingPath,omitempty"`
	CacheMode   string `json:"cacheMode,omitempty"`
	Discard     bool   `json:"discard,omitempty"`
	ReadOnly    bool   `json:"readonly,omitempty"`
	IsBoot      bool   `json:"isBoot,omitempty"`
	IsCDROM     bool   `json:"isCdrom,omitempty"`

	BootIndex int   `json:"bootIndex,omitempty"`
	ShareRW   bool  `json:"shareRw,omitempty"`
	ReadIOPS  int64 `json:"readIops,omitempty"`
	WriteIOPS int64 `json:"writeIops,omitempty"`
	ReadBPS   int64 `json:"readBps,omitempty"`
	WriteBPS  int64 `json:"writeBps,omitempty"`
}

// agentShare is one virtiofs directory on the wire.
type agentShare struct {
	ShareID  string `json:"shareId"`
	Tag      string `json:"tag"`
	HostPath string `json:"hostPath"`
	ReadOnly bool   `json:"readOnly,omitempty"`
}

type agentNIC struct {
	NICID      string `json:"nicId,omitempty"`
	NetworkID  string `json:"networkId,omitempty"`
	Bridge     string `json:"bridge,omitempty"`
	VLANTag    int    `json:"vlanTag,omitempty"`
	AttachKind string `json:"attachKind,omitempty"`
	ParentLink string `json:"parentLink,omitempty"`
	LinkMode   string `json:"linkMode,omitempty"`
	Slot       int    `json:"slot"`
	Model      string `json:"model,omitempty"`
	MAC        string `json:"mac"`
	TapName    string `json:"tapName,omitempty"`
	PCIAddr    string `json:"pciAddr,omitempty"`
	LinkDown   bool   `json:"linkDown,omitempty"`
}

type agentDevice struct {
	DeviceID    string `json:"deviceId,omitempty"`
	Kind        string `json:"kind"`
	HostAddress string `json:"hostAddress"`
	Description string `json:"description,omitempty"`
}

type agentCloudInit struct {
	InstanceID    string   `json:"instanceId"`
	Hostname      string   `json:"hostname,omitempty"`
	Timezone      string   `json:"timezone,omitempty"`
	SSHKeys       []string `json:"sshKeys,omitempty"`
	DefaultUser   string   `json:"defaultUser,omitempty"`
	PasswordHash  string   `json:"passwordHash,omitempty"`
	NetworkMode   string   `json:"networkMode,omitempty"`
	StaticIP      string   `json:"staticIp,omitempty"`
	StaticGateway string   `json:"staticGateway,omitempty"`
	StaticDNS     []string `json:"staticDns,omitempty"`
	UserDataExtra string   `json:"userDataExtra,omitempty"`
}

type agentClone struct {
	TargetVMID string     `json:"targetVmId"`
	Linked     bool       `json:"linked,omitempty"`
	Spec       *agentSpec `json:"spec,omitempty"`
}

type agentState struct {
	VMID         string `json:"vmId"`
	State        string `json:"state"`
	UnitName     string `json:"unitName,omitempty"`
	PID          int    `json:"pid,omitempty"`
	QMPReachable bool   `json:"qmpReachable,omitempty"`
	StartedAt    string `json:"startedAt,omitempty"`
	ExitReason   string `json:"exitReason,omitempty"`
	LastError    string `json:"lastError,omitempty"`
	// RunState is QEMU's own run state. It is what tells a deliberately
	// paused guest from one stopped dead by a failed disk backend, which the
	// mapped State cannot.
	RunState string `json:"runState,omitempty"`
	// BlockJobs are the mirrors, commits and backups in flight, so a snapshot
	// still flattening does not read as a hung guest.
	BlockJobs []agentBlockJob `json:"blockJobs,omitempty"`
}

// agentBlockJob is one long-running block operation QEMU is driving.
type agentBlockJob struct {
	Device string `json:"device"`
	Type   string `json:"type"`
	Len    int64  `json:"len,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

type agentGuest struct {
	VMID       string   `json:"vmId"`
	AgentState string   `json:"agentState"`
	Hostname   string   `json:"hostname,omitempty"`
	OSID       string   `json:"osId,omitempty"`
	OSName     string   `json:"osName,omitempty"`
	OSVersion  string   `json:"osVersion,omitempty"`
	IPs        []string `json:"ips,omitempty"`
	Frozen     bool     `json:"frozen,omitempty"`

	// Filesystems, the load averages, ClockSkewMs and Unsupported answer the
	// "health" query. Every value is guest-reported and untrusted.
	Filesystems []agentGuestFilesystem `json:"filesystems,omitempty"`
	Load1       *float64               `json:"load1,omitempty"`
	Load5       *float64               `json:"load5,omitempty"`
	Load15      *float64               `json:"load15,omitempty"`
	ClockSkewMs *int64                 `json:"clockSkewMs,omitempty"`
	Unsupported []string               `json:"unsupported,omitempty"`
}

// agentGuestFilesystem is one guest filesystem with usage, as qemu-ga reported it.
type agentGuestFilesystem struct {
	Mountpoint string `json:"mountpoint"`
	Type       string `json:"type,omitempty"`
	UsedBytes  int64  `json:"usedBytes"`
	TotalBytes int64  `json:"totalBytes"`
}

type agentStats struct {
	VMID           string  `json:"vmId"`
	CPUPercent     float64 `json:"cpuPercent,omitempty"`
	MemoryBytes    int64   `json:"memoryBytes,omitempty"`
	MemoryMaxBytes int64   `json:"memoryMaxBytes,omitempty"`
	BalloonBytes   int64   `json:"balloonBytes,omitempty"`
	DiskReadBytes  int64   `json:"diskReadBytes,omitempty"`
	DiskWriteBytes int64   `json:"diskWriteBytes,omitempty"`
	NetRxBytes     int64   `json:"netRxBytes,omitempty"`
	NetTxBytes     int64   `json:"netTxBytes,omitempty"`
	SampledAt      string  `json:"sampledAt,omitempty"`
}

// call sends one guest command and decodes the reply.
//
// Nothing here is queued for replay when the agent is offline. A "start this
// VM" replayed twenty minutes after the operator gave up and started it
// elsewhere is a correctness hazard, not a convenience, so this fails fast and
// the UI prompts to reconnect.
func (s *Service) call(
	ctx context.Context,
	envID, messageType string,
	payload agentPayload,
) (*agentPayload, error) {
	if s.rpc == nil {
		return nil, ErrHostUnavailable
	}
	if available, reason := s.rpc.Available(ctx, envID); !available {
		return nil, fmt.Errorf("%w: %s", ErrHostUnavailable, reason)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encoding %s: %w", messageType, err)
	}

	raw, err := s.rpc.send(ctx, envID, messageType, body)
	if err != nil {
		return nil, err
	}

	var reply agentPayload
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, fmt.Errorf("decoding %s reply: %w", messageType, err)
	}
	return &reply, nil
}

// --- Reads ----------------------------------------------------------------

const vmColumns = `
	id, environment_id, name, COALESCE(description, ''), state,
	COALESCE(guest_os_family, ''), COALESCE(guest_os_variant, ''),
	vcpus, cpu_model, cpu_sockets, cpu_cores, cpu_threads,
	memory_mb, COALESCE(memory_max_mb, 0),
	machine_type, firmware, secure_boot, tpm_enabled, display_type,
	autostart, autostart_order, autostart_delay_s,
	is_template, COALESCE(template_source_id, ''),
	COALESCE(guest_agent_state, ''), COALESCE(guest_reported_ips, ''),
	uuid, COALESCE(unit_name, ''),
	COALESCE(last_started_at, ''), COALESCE(last_error, ''), created_at, updated_at,
	COALESCE(current_snapshot_id, ''),
	COALESCE(run_state, ''), COALESCE(guest_hostname, ''),
	COALESCE(guest_os_name, ''), COALESCE(guest_os_version, ''),
	COALESCE(vcpus_max, 0), COALESCE(memory_current_mb, 0),
	virtio_mem, hugepages,
	COALESCE(numa_policy, ''), COALESCE(numa_host_nodes, ''),
	COALESCE(vsock_cid, 0), iothread,
	COALESCE(display_profile_id, ''),
	display_width, display_height`

// List returns the guests on one environment, or all of them.
func (s *Service) List(ctx context.Context, envID string, templates bool) ([]VM, error) {
	query := `SELECT ` + vmColumns + ` FROM plugin_addon_kvm_vms WHERE is_template = ?`
	args := []any{boolToInt(templates)}
	if envID != "" {
		query += ` AND environment_id = ?`
		args = append(args, envID)
	}
	query += ` ORDER BY name LIMIT ?`
	args = append(args, maxRows)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing virtual machines: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []VM
	for rows.Next() {
		vm, err := scanVM(rows)
		if err != nil {
			return nil, err
		}
		s.applyAvailability(ctx, &vm)
		out = append(out, vm)
	}
	return out, rows.Err()
}

// applyAvailability marks a guest whose host cannot be reached.
//
// The state itself is left as last known and the UI greys it — replacing it
// with "stopped" would be asserting something nobody confirmed.
func (s *Service) applyAvailability(ctx context.Context, vm *VM) {
	// Attached here because every read path already calls this, so the list,
	// the detail view and the dialog all see the same number without each
	// remembering to ask for it.
	vm.DeleteProgress = s.deleteProgressFor(vm.ID)
	vm.SnapshotProgress = s.snapshotProgressFor(vm.ID)
	vm.RevertProgress = s.revertProgressFor(vm.ID)
	vm.CloneProgress = s.cloneProgressFor(vm.ID)

	if s.rpc == nil {
		vm.HostAvailable = false
		vm.UnavailableReason = "agent_offline"
		vm.State = StateUnknown
		return
	}
	available, reason := s.rpc.Available(ctx, vm.EnvironmentID)
	vm.HostAvailable = available
	vm.UnavailableReason = reason
	if !available {
		vm.State = StateUnknown
	}
	// Last, because it reads the state the lines above may have just replaced.
	vm.EditableNow = editableSettings(vm.State)
}

// changedColdSettings names the stopped-only fields this patch would change.
func changedColdSettings(req UpdateRequest) []string {
	present := map[string]bool{
		"vcpus":         req.VCPUs != nil,
		"memoryMb":      req.MemoryMB != nil,
		"memoryMaxMb":   req.MemoryMaxMB != nil,
		"cpuModel":      req.CPUModel != nil,
		"machineType":   req.MachineType != nil,
		"firmware":      req.Firmware != nil,
		"secureBoot":    req.SecureBoot != nil,
		"tpmEnabled":    req.TPMEnabled != nil,
		"displayType":   req.DisplayType != nil,
		"displayWidth":  req.DisplayWidth != nil,
		"displayHeight": req.DisplayHeight != nil,
		"vcpusMax":      req.VCPUsMax != nil,
		"virtioMem":     req.VirtioMem != nil,
		"hugePages":     req.HugePages != nil,
		"numaPolicy":    req.NUMAPolicy != nil,
		"ioThread":      req.IOThread != nil,
		"vsockEnabled":  req.VSockEnabled != nil,
	}
	var changed []string
	for _, field := range coldSettings {
		if present[field] {
			changed = append(changed, field)
		}
	}
	return changed
}

// Get reads one guest with its hardware.
func (s *Service) Get(ctx context.Context, id string) (VM, error) {
	rows, err := s.db.Query(ctx, `SELECT `+vmColumns+` FROM plugin_addon_kvm_vms WHERE id = ?`, id)
	if err != nil {
		return VM{}, fmt.Errorf("reading virtual machine: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return VM{}, err
		}
		return VM{}, ErrNotFound
	}
	vm, err := scanVM(rows)
	if err != nil {
		return VM{}, err
	}
	_ = rows.Close()

	if vm.Disks, err = s.Disks(ctx, id); err != nil {
		return VM{}, err
	}
	if vm.NICs, err = s.NICs(ctx, id); err != nil {
		return VM{}, err
	}
	if vm.Snapshots, err = s.Snapshots(ctx, id); err != nil {
		return VM{}, err
	}
	if vm.Devices, err = s.Devices(ctx, id); err != nil {
		return VM{}, err
	}
	if cloudInit, err := s.CloudInit(ctx, id); err == nil {
		vm.CloudInit = &cloudInit
	}

	s.applyAvailability(ctx, &vm)
	return vm, nil
}

// Disks reads a guest's block devices.
func (s *Service) Disks(ctx context.Context, vmID string) ([]Disk, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, vm_id, pool_id, pool_backend, pool_target, slot, bus, volume_path,
		       format, size_bytes, COALESCE(backing_path, ''), cache_mode,
		       discard, readonly, is_boot, is_cdrom, created_at, updated_at,
		       COALESCE(boot_index, 0), share_rw,
		       COALESCE(io_read_iops, 0), COALESCE(io_write_iops, 0),
		       COALESCE(io_read_bps, 0), COALESCE(io_write_bps, 0)
		  FROM plugin_addon_kvm_disks WHERE vm_id = ? ORDER BY slot LIMIT ?`, vmID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading disks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Disk
	for rows.Next() {
		disk, err := scanDisk(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, disk)
	}
	return out, rows.Err()
}

// NICs reads a guest's interfaces.
func (s *Service) NICs(ctx context.Context, vmID string) ([]NIC, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, vm_id, network_id, bridge, COALESCE(vlan_tag, 0),
		       COALESCE(attach_kind, ''), COALESCE(parent_link, ''), COALESCE(link_mode, ''),
		       slot, model, mac, COALESCE(tap_name, ''), COALESCE(pci_addr, ''),
		       COALESCE(link_down, 0), created_at, updated_at
		  FROM plugin_addon_kvm_nics WHERE vm_id = ? ORDER BY slot LIMIT ?`, vmID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading interfaces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []NIC
	for rows.Next() {
		nic, err := scanNIC(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, nic)
	}
	return out, rows.Err()
}

// Snapshots reads a guest's snapshots.
func (s *Service) Snapshots(ctx context.Context, vmID string) ([]Snapshot, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, vm_id, COALESCE(parent_id, ''), name, COALESCE(description, ''),
		       mechanism, consistency, with_memory, COALESCE(size_bytes, 0),
		       chain_depth, handle, created_at, updated_at
		  FROM plugin_addon_kvm_snapshots WHERE vm_id = ?
		 ORDER BY created_at LIMIT ?`, vmID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Snapshot
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	countChildren(out)
	return out, nil
}

// Devices reads a guest's passed-through host devices.
func (s *Service) Devices(ctx context.Context, vmID string) ([]Device, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, vm_id, kind, host_address, COALESCE(description, ''), created_at, updated_at
		  FROM plugin_addon_kvm_devices WHERE vm_id = ? LIMIT ?`, vmID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading devices: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Device
	for rows.Next() {
		var device Device
		var created, updated string
		if err := rows.Scan(&device.ID, &device.VMID, &device.Kind, &device.HostAddress,
			&device.Description, &created, &updated); err != nil {
			return nil, err
		}
		device.CreatedAt = parseTime(created)
		device.UpdatedAt = parseTime(updated)
		out = append(out, device)
	}
	return out, rows.Err()
}

// CloudInit reads a guest's first-boot configuration.
//
// The password hash is read but never returned: HasPassword is what the edit
// form needs, and shipping the hash to a browser serves nothing.
func (s *Service) CloudInit(ctx context.Context, vmID string) (CloudInit, error) {
	rows, err := s.db.Query(ctx, `
		SELECT vm_id, COALESCE(hostname, ''), COALESCE(timezone, ''),
		       COALESCE(ssh_keys, ''), COALESCE(default_user, ''),
		       COALESCE(password_hash, ''), network_mode, COALESCE(static_ip, ''),
		       COALESCE(static_gateway, ''), COALESCE(static_dns, ''),
		       COALESCE(user_data_extra, ''), instance_id
		  FROM plugin_addon_kvm_cloud_init WHERE vm_id = ?`, vmID)
	if err != nil {
		return CloudInit{}, fmt.Errorf("reading cloud-init: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return CloudInit{}, ErrNotFound
	}

	var out CloudInit
	var sshKeys, passwordHash, staticDNS string
	if err := rows.Scan(&out.VMID, &out.Hostname, &out.Timezone, &sshKeys,
		&out.DefaultUser, &passwordHash, &out.NetworkMode, &out.StaticIP,
		&out.StaticGateway, &staticDNS, &out.UserDataExtra, &out.InstanceID); err != nil {
		return CloudInit{}, err
	}
	out.SSHKeys = splitList(sshKeys)
	out.StaticDNS = splitList(staticDNS)
	out.HasPassword = passwordHash != ""
	return out, nil
}

// Stats samples a running guest.
func (s *Service) Stats(ctx context.Context, id string) (Stats, error) {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return Stats{}, err
	}
	if !vm.Running() {
		return Stats{}, ErrNotRunning
	}

	// vm_stats, not vm_query: the query handler reconciles state and never
	// fills in Stats, so asking it for a sample returned nil every time and the
	// zero-filled fallback this replaced rendered every guest as permanently
	// idle. An agent too old to answer vm_stats reports unavailable instead.
	reply, err := s.call(ctx, vm.EnvironmentID, msgVMStats, agentPayload{VMID: id})
	if err != nil {
		return Stats{}, err
	}
	if reply.Stats == nil {
		return Stats{}, ErrStatsUnavailable
	}

	return Stats{
		VMID:           id,
		CPUPercent:     reply.Stats.CPUPercent,
		MemoryBytes:    reply.Stats.MemoryBytes,
		MemoryMaxBytes: memoryCeiling(reply.Stats.MemoryMaxBytes, vm),
		BalloonBytes:   reply.Stats.BalloonBytes,
		DiskReadBytes:  reply.Stats.DiskReadBytes,
		DiskWriteBytes: reply.Stats.DiskWriteBytes,
		NetRxBytes:     reply.Stats.NetRxBytes,
		NetTxBytes:     reply.Stats.NetTxBytes,
		SampledAt:      parseTime(reply.Stats.SampledAt),
	}, nil
}

// HostDevices asks a host what it has that a guest could be given.
//
// Read-only: it enumerates sysfs and changes nothing. The answer includes
// devices that cannot currently be assigned, each carrying the reason, because
// "your GPU is bound to amdgpu" is the thing an operator needs to be told —
// hiding it would leave them looking for a device that is plainly in the box.
func (s *Service) HostDevices(ctx context.Context, envID string) ([]HostDevice, error) {
	if envID == "" {
		return nil, ErrNotFound
	}
	reply, err := s.call(ctx, envID, msgHostDevList, agentPayload{})
	if err != nil {
		return nil, err
	}
	if reply.HostDevices == nil {
		return []HostDevice{}, nil
	}
	holders, err := s.deviceHolders(ctx, envID)
	if err != nil {
		return nil, err
	}
	for i := range reply.HostDevices {
		device := &reply.HostDevices[i]
		device.AttachedTo = holders[device.Kind+"|"+device.Address]
	}
	return reply.HostDevices, nil
}

// deviceHolders maps kind|address to the guests on envID that already have that
// device, which is how one host's hardware stays visible across every VM form.
func (s *Service) deviceHolders(ctx context.Context, envID string) (map[string][]DeviceHolder, error) {
	rows, err := s.db.Query(ctx, `
		SELECT d.kind, d.host_address, v.id, v.name
		  FROM plugin_addon_kvm_devices d
		  JOIN plugin_addon_kvm_vms v ON v.id = d.vm_id
		 WHERE v.environment_id = ?
		 ORDER BY v.name LIMIT ?`, envID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading attached devices: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]DeviceHolder{}
	for rows.Next() {
		var kind, address string
		var holder DeviceHolder
		if err := rows.Scan(&kind, &address, &holder.VMID, &holder.Name); err != nil {
			return nil, err
		}
		out[kind+"|"+address] = append(out[kind+"|"+address], holder)
	}
	return out, rows.Err()
}

// PoolVolumes lists the disks in a pool that no guest is using.
//
// The agent reports everything the pool holds, because it cannot know what is
// attached; the subtraction happens here, against every disk row this plugin
// has. A volume left behind by a guest deleted without its disks is exactly
// what this is for, and it is indistinguishable on the host from a live one.
func (s *Service) PoolVolumes(ctx context.Context, envID string, pool PoolRef) ([]PoolVolume, error) {
	if envID == "" {
		return nil, invalid(codePoolRequired, "a storage pool is required")
	}
	pool, err := s.resolvePool(ctx, envID, pool)
	if err != nil {
		return nil, err
	}

	reply, err := s.call(ctx, envID, msgVMDiskList, agentPayload{
		Disk: &agentDisk{
			PoolID:      pool.ID,
			PoolBackend: pool.Backend,
			PoolTarget:  pool.Target,
		},
	})
	if err != nil {
		return nil, err
	}

	attached, err := s.attachedVolumePaths(ctx)
	if err != nil {
		return nil, err
	}

	free := make([]PoolVolume, 0, len(reply.Volumes))
	for _, volume := range reply.Volumes {
		if volume.VolumePath == "" || attached[volume.VolumePath] {
			continue
		}
		free = append(free, PoolVolume(volume))
	}
	return free, nil
}

// attachedVolumePaths is every volume path a disk row points at, on any host.
//
// Not scoped to one environment on purpose: a path is only meaningful on the
// host it lives on, but two hosts sharing a mount is a real homelab shape and
// offering the same volume to a second guest would corrupt both.
func (s *Service) attachedVolumePaths(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `
		SELECT volume_path FROM plugin_addon_kvm_disks
		WHERE volume_path <> '' LIMIT ?`, maxRows)
	if err != nil {
		return nil, fmt.Errorf("reading attached volumes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	attached := make(map[string]bool)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("reading attached volumes: %w", err)
		}
		attached[path] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading attached volumes: %w", err)
	}
	return attached, nil
}

// GuestLog tails one of the systemd units McHarbor created for a guest.
//
// Read-only and host-side: the unit name is built on the agent from the guest
// id, so this cannot be pointed at another unit. source is "vm" for QEMU itself
// or "tpm" for its software TPM — the second is worth having separately because
// a vTPM that died takes the guest down with it and says nothing in QEMU's log.
func (s *Service) GuestLog(ctx context.Context, id, source string, tail int) (GuestLog, error) {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return GuestLog{}, err
	}
	switch source {
	case "", "vm", "tpm":
	default:
		return GuestLog{}, invalid(codeLogSource, fmt.Sprintf("%q is not a log source", source))
	}

	reply, err := s.call(ctx, vm.EnvironmentID, msgVMLog, agentPayload{
		VMID: id, LogSource: source, LogTail: tail,
	})
	if err != nil {
		return GuestLog{}, err
	}
	return GuestLog{
		Source:  defaultString(source, "vm"),
		Lines:   reply.LogLines,
		Notices: reply.LogNotices,
	}, nil
}

// --- Writes ---------------------------------------------------------------

// Create defines a guest, allocates its disk and materializes it on the host.
//
// The order matters and is not arbitrary. The row is written first so a failure
// part-way leaves something the operator can see and delete, rather than an
// orphaned disk on a host with nothing pointing at it.
func (s *Service) Create(ctx context.Context, req CreateRequest) (VM, error) {
	pool, err := s.resolvePool(ctx, req.EnvironmentID, PoolRef{ID: req.PoolID, Backend: req.PoolBackend, Target: req.PoolTarget})
	if err != nil {
		return VM{}, err
	}
	req.PoolBackend, req.PoolTarget = pool.Backend, pool.Target
	if err := validateCreate(req); err != nil {
		return VM{}, err
	}
	if s.rpc == nil {
		return VM{}, ErrHostUnavailable
	}

	capability, err := s.rpc.Probe(ctx, req.EnvironmentID)
	if err != nil {
		return VM{}, err
	}
	if !capability.CanRunVMs {
		return VM{}, ErrNoCapability
	}

	machineType := req.MachineType
	if machineType == "" {
		// Pin a concrete version rather than the `q35` alias. An unpinned
		// machine type is how guests break on a host package update.
		machineType = pickMachineType(capability)
		if machineType == "" {
			return VM{}, fmt.Errorf("this host's QEMU offers no usable machine type")
		}
	}

	now := time.Now().UTC()
	vm := VM{
		ID:             xid.New().String(),
		EnvironmentID:  req.EnvironmentID,
		Name:           req.Name,
		Description:    req.Description,
		State:          StateDefined,
		GuestOSFamily:  defaultString(req.GuestOSFamily, OSFamilyLinux),
		GuestOSVariant: req.GuestOSVariant,
		VCPUs:          req.VCPUs,
		CPUModel:       defaultString(req.CPUModel, "host"),
		CPUSockets:     1,
		CPUCores:       req.VCPUs,
		CPUThreads:     1,
		MemoryMB:       req.MemoryMB,
		MemoryMaxMB:    req.MemoryMaxMB,
		MachineType:    machineType,
		Firmware:       defaultString(req.Firmware, "uefi"),
		SecureBoot:     req.SecureBoot,
		TPMEnabled:     req.TPMEnabled,
		DisplayType:    defaultDisplay(req.GuestOSFamily),
		Autostart:      req.Autostart,
		// A real UUID, not the xid: SMBIOS wants one, and anything in the guest
		// keying on the machine UUID — licensing especially — needs it stable
		// and distinct per guest.
		UUID:      uuid.NewString(),
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Before anything exists: a secret the user may not read, or that is not
	// there, must not leave a half-made guest behind.
	if req.CloudInit != nil {
		if err := s.resolveCloudInitSecrets(ctx, req.RequestedBy, vm, req.CloudInit); err != nil {
			s.releaseVault(vm.ID)
			return VM{}, err
		}
	}

	if err := s.insertVM(ctx, vm); err != nil {
		s.releaseVault(vm.ID)
		return VM{}, err
	}

	// Roll the row back on any later failure, so a half-created guest does not
	// linger in a list the operator cannot act on.
	rollback := func() {
		if _, err := s.db.Exec(context.Background(),
			`DELETE FROM plugin_addon_kvm_vms WHERE id = ?`, vm.ID); err != nil {
			s.logger.Warn("rolling back a failed VM create", "vm", vm.ID, "error", err)
		}
		s.releaseVault(vm.ID)
	}

	disk, err := s.allocateBootDisk(ctx, vm, req)
	if err != nil {
		rollback()
		return VM{}, err
	}
	vm.Disks = []Disk{disk}

	nic, err := s.attachNIC(ctx, vm.ID, NICRequest{
		NetworkID: req.NetworkID, Bridge: req.Bridge, VLANTag: req.VLANTag,
		AttachKind: req.AttachKind, ParentLink: req.ParentLink, LinkMode: req.LinkMode,
	})
	if err != nil {
		rollback()
		return VM{}, err
	}
	vm.NICs = []NIC{nic}

	// Installer discs are persisted as CD-ROM rows rather than passed as the
	// spec's ISOPaths, which the agent declares and never reads: a disc that
	// lives only in the define call would vanish on the next start.
	for _, path := range req.ISOPaths {
		if err := validateMediaPath(path); err != nil {
			rollback()
			return VM{}, err
		}
		slot, err := s.nextDiskSlot(ctx, vm.ID)
		if err != nil {
			rollback()
			return VM{}, err
		}
		media, err := s.recordMedia(ctx, vm.ID, slot, path)
		if err != nil {
			rollback()
			return VM{}, err
		}
		vm.Disks = append(vm.Disks, media)
	}

	if req.CloudInit != nil {
		cloudInit, err := s.writeCloudInit(ctx, vm.ID, *req.CloudInit)
		if err != nil {
			rollback()
			return VM{}, err
		}
		vm.CloudInit = &cloudInit
	}

	// Materialize on the host. Everything above is McHarbor's own record; this
	// is where the guest starts to exist.
	if _, err := s.call(ctx, vm.EnvironmentID, msgVMDefine, agentPayload{
		VMID: vm.ID,
		Spec: s.buildSpec(ctx, vm),
	}); err != nil {
		rollback()
		return VM{}, err
	}

	s.applyAvailability(ctx, &vm)
	return vm, nil
}

// insertVM writes one guest's row.
//
// Shared by Create and the import path, which build the same row from very
// different inputs and must not drift in what they store.
func (s *Service) insertVM(ctx context.Context, vm VM) error {
	stamp := vm.CreatedAt.Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_vms
			(id, environment_id, name, description, state, guest_os_family, guest_os_variant,
			 vcpus, cpu_model, cpu_sockets, cpu_cores, cpu_threads, memory_mb, memory_max_mb,
			 machine_type, firmware, secure_boot, tpm_enabled, display_type,
			 autostart, autostart_order, autostart_delay_s, is_template,
			 uuid, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, ?, ?, ?)`,
		vm.ID, vm.EnvironmentID, vm.Name, nullIfEmpty(vm.Description), vm.State,
		nullIfEmpty(vm.GuestOSFamily), nullIfEmpty(vm.GuestOSVariant),
		vm.VCPUs, vm.CPUModel, vm.CPUSockets, vm.CPUCores, vm.CPUThreads,
		vm.MemoryMB, nullIfZero64(vm.MemoryMaxMB),
		vm.MachineType, vm.Firmware, boolToInt(vm.SecureBoot), boolToInt(vm.TPMEnabled),
		vm.DisplayType, boolToInt(vm.Autostart), vm.UUID,
		stamp, stamp); err != nil {
		if isUniqueViolation(err) {
			return ErrNameTaken
		}
		return fmt.Errorf("recording virtual machine: %w", err)
	}
	return nil
}

// allocateBootDisk creates the guest's first disk.
//
// With an image, the disk is copy-on-write over it — instant on a CoW pool,
// seconds on a directory pool — rather than a copy of the whole thing.
func (s *Service) allocateBootDisk(ctx context.Context, vm VM, req CreateRequest) (Disk, error) {
	if req.DiskVolumePath != "" {
		return s.adoptBootDisk(ctx, vm, req)
	}

	sizeBytes := req.DiskSizeGB * 1024 * 1024 * 1024

	reply, err := s.call(ctx, vm.EnvironmentID, msgVMDiskCreate, agentPayload{
		VMID: vm.ID,
		Disk: &agentDisk{
			PoolID:      req.PoolID,
			PoolBackend: req.PoolBackend,
			PoolTarget:  req.PoolTarget,
			Slot:        0,
			SizeBytes:   sizeBytes,
			BackingPath: req.ImageLocalPath,
			Format:      "qcow2",
			Discard:     true,
			IsBoot:      true,
		},
	})
	if err != nil {
		return Disk{}, err
	}

	volumePath := ""
	format := "qcow2"
	if reply.Disk != nil {
		volumePath = reply.Disk.VolumePath
		if reply.Disk.Format != "" {
			format = reply.Disk.Format
		}
		if reply.Disk.SizeBytes > 0 {
			sizeBytes = reply.Disk.SizeBytes
		}
	}

	now := time.Now().UTC()
	disk := Disk{
		ID:          xid.New().String(),
		VMID:        vm.ID,
		PoolID:      req.PoolID,
		PoolBackend: req.PoolBackend,
		PoolTarget:  req.PoolTarget,
		Slot:        0,
		Bus:         defaultString(req.DiskBus, defaultDiskBus(vm.GuestOSFamily)),
		VolumePath:  volumePath,
		Format:      format,
		SizeBytes:   sizeBytes,
		BackingPath: req.ImageLocalPath,
		CacheMode:   "none",
		Discard:     true,
		IsBoot:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_disks
			(id, vm_id, pool_id, pool_backend, pool_target, slot, bus, volume_path,
			 format, size_bytes, backing_path, cache_mode, discard, readonly,
			 is_boot, is_cdrom, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, 1, 0, ?, ?)`,
		disk.ID, disk.VMID, disk.PoolID, disk.PoolBackend, disk.PoolTarget,
		disk.Slot, disk.Bus, disk.VolumePath, disk.Format, disk.SizeBytes,
		nullIfEmpty(disk.BackingPath), disk.CacheMode,
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return Disk{}, fmt.Errorf("recording disk: %w", err)
	}
	return disk, nil
}

// adoptBootDisk records a volume that already exists in the pool as the guest's
// boot disk, without writing to it.
//
// The path is re-listed from the agent rather than trusted from the request:
// the caller could otherwise name any file the agent can open, and a stale
// browser tab could name a volume that has since been attached elsewhere.
func (s *Service) adoptBootDisk(ctx context.Context, vm VM, req CreateRequest) (Disk, error) {
	pool := PoolRef{ID: req.PoolID, Backend: req.PoolBackend, Target: req.PoolTarget}
	free, err := s.PoolVolumes(ctx, vm.EnvironmentID, pool)
	if err != nil {
		return Disk{}, err
	}

	var adopted *PoolVolume
	for i := range free {
		if free[i].VolumePath == req.DiskVolumePath {
			adopted = &free[i]
			break
		}
	}
	if adopted == nil {
		return Disk{}, invalid(codeVolumeUnavailable,
			fmt.Sprintf("%q is not a free volume in this pool", req.DiskVolumePath))
	}

	now := time.Now().UTC()
	disk := Disk{
		ID:          xid.New().String(),
		VMID:        vm.ID,
		PoolID:      req.PoolID,
		PoolBackend: req.PoolBackend,
		PoolTarget:  req.PoolTarget,
		Slot:        0,
		Bus:         defaultString(req.DiskBus, defaultDiskBus(vm.GuestOSFamily)),
		VolumePath:  adopted.VolumePath,
		Format:      defaultString(adopted.Format, "qcow2"),
		SizeBytes:   adopted.SizeBytes,
		CacheMode:   "none",
		Discard:     true,
		IsBoot:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_disks
			(id, vm_id, pool_id, pool_backend, pool_target, slot, bus, volume_path,
			 format, size_bytes, backing_path, cache_mode, discard, readonly,
			 is_boot, is_cdrom, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, 1, 0, 1, 0, ?, ?)`,
		disk.ID, disk.VMID, disk.PoolID, disk.PoolBackend, disk.PoolTarget,
		disk.Slot, disk.Bus, disk.VolumePath, disk.Format, disk.SizeBytes,
		disk.CacheMode,
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return Disk{}, fmt.Errorf("recording disk: %w", err)
	}
	return disk, nil
}

// attachNIC records an interface and allocates its MAC.
func (s *Service) attachNIC(ctx context.Context, vmID string, req NICRequest) (NIC, error) {
	slot, err := s.nextNICSlot(ctx, vmID)
	if err != nil {
		return NIC{}, err
	}

	now := time.Now().UTC()
	// A bounded retry, because the MAC column is globally unique and a
	// collision — however unlikely — should cost a retry rather than a failed
	// create.
	for attempt := 0; attempt < 5; attempt++ {
		mac, err := generateMAC()
		if err != nil {
			return NIC{}, err
		}

		nic := NIC{
			ID:         xid.New().String(),
			VMID:       vmID,
			NetworkID:  req.NetworkID,
			Bridge:     req.Bridge,
			VLANTag:    req.VLANTag,
			AttachKind: req.AttachKind,
			ParentLink: req.ParentLink,
			LinkMode:   req.LinkMode,
			Slot:       slot,
			Model:      defaultString(req.Model, "virtio-net"),
			MAC:        mac,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		_, err = s.db.Exec(ctx, `
			INSERT INTO plugin_addon_kvm_nics
				(id, vm_id, network_id, bridge, vlan_tag, attach_kind, parent_link, link_mode,
				 slot, model, mac, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			nic.ID, nic.VMID, nic.NetworkID, nic.Bridge, nullIfZero(nic.VLANTag),
			nullIfEmpty(nic.AttachKind), nullIfEmpty(nic.ParentLink), nullIfEmpty(nic.LinkMode),
			nic.Slot, nic.Model, nic.MAC,
			now.Format(time.RFC3339), now.Format(time.RFC3339))
		if err == nil {
			return nic, nil
		}
		if !isUniqueViolation(err) {
			return NIC{}, fmt.Errorf("recording interface: %w", err)
		}
	}
	return NIC{}, fmt.Errorf("could not allocate a unique MAC address")
}

func (s *Service) nextNICSlot(ctx context.Context, vmID string) (int, error) {
	rows, err := s.db.Query(ctx,
		`SELECT COALESCE(MAX(slot), -1) FROM plugin_addon_kvm_nics WHERE vm_id = ?`, vmID)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var highest int
		if err := rows.Scan(&highest); err != nil {
			return 0, err
		}
		return highest + 1, nil
	}
	return 0, rows.Err()
}

// writeCloudInit stores the first-boot configuration, hashing the password.
func (s *Service) writeCloudInit(ctx context.Context, vmID string, req CloudInitRequest) (CloudInit, error) {
	passwordHash := ""
	if req.Password != "" {
		hash, err := hashPassword(req.Password)
		if err != nil {
			return CloudInit{}, err
		}
		passwordHash = hash
	}

	now := time.Now().UTC()
	out := CloudInit{
		VMID: vmID,
		// Slugged rather than trusted: a guest's name is free text, and the UI
		// falls back to it when the operator leaves the hostname blank.
		Hostname:      guestHostname(req.Hostname),
		Timezone:      req.Timezone,
		SSHKeys:       req.SSHKeys,
		DefaultUser:   defaultString(req.DefaultUser, "mcharbor"),
		HasPassword:   passwordHash != "",
		NetworkMode:   defaultString(req.NetworkMode, "dhcp"),
		StaticIP:      req.StaticIP,
		StaticGateway: req.StaticGateway,
		StaticDNS:     req.StaticDNS,
		UserDataExtra: req.UserDataExtra,
		// A fresh instance-id per guest, regenerated on clone. This is what
		// makes cloud-init re-run its first-boot setup rather than a clone
		// inheriting the template's hostname and SSH host keys.
		InstanceID: xid.New().String(),
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_cloud_init
			(vm_id, hostname, timezone, ssh_keys, default_user, password_hash,
			 network_mode, static_ip, static_gateway, static_dns, user_data_extra,
			 instance_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		vmID, nullIfEmpty(out.Hostname), nullIfEmpty(out.Timezone),
		nullIfEmpty(joinList(out.SSHKeys)), out.DefaultUser, nullIfEmpty(passwordHash),
		out.NetworkMode, nullIfEmpty(out.StaticIP), nullIfEmpty(out.StaticGateway),
		nullIfEmpty(joinList(out.StaticDNS)), nullIfEmpty(out.UserDataExtra),
		out.InstanceID, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return CloudInit{}, fmt.Errorf("recording cloud-init: %w", err)
	}
	return out, nil
}

// buildSpec assembles what the agent needs to materialize and boot a guest.
//
// Installer discs are not passed through the spec's ISOPaths field, which the
// agent declares and never reads; they are CD-ROM entries in Disks like any
// other attached media.
func (s *Service) buildSpec(ctx context.Context, vm VM) *agentSpec {
	spec := &agentSpec{
		Name:           vm.Name,
		UUID:           vm.UUID,
		GuestOSFamily:  vm.GuestOSFamily,
		GuestOSVariant: vm.GuestOSVariant,
		VCPUs:          vm.VCPUs,
		CPUModel:       vm.CPUModel,
		CPUSockets:     vm.CPUSockets,
		CPUCores:       vm.CPUCores,
		CPUThreads:     vm.CPUThreads,
		MemoryMB:       vm.MemoryMB,
		MemoryMaxMB:    vm.MemoryMaxMB,
		MachineType:    vm.MachineType,
		Firmware:       vm.Firmware,
		SecureBoot:     vm.SecureBoot,
		TPMEnabled:     vm.TPMEnabled,
		DisplayType:    effectiveDisplay(vm.DisplayType, vm.GuestOSFamily),
		DisplayWidth:   vm.DisplayWidth,
		DisplayHeight:  vm.DisplayHeight,
		VCPUsMax:       vm.VCPUsMax,
		VirtioMem:      vm.VirtioMem,
		HugePages:      vm.HugePages,
		NUMAPolicy:     vm.NUMAPolicy,
		NUMAHostNodes:  vm.NUMAHostNodes,
		VSockCID:       vm.VSockCID,
		IOThread:       vm.IOThread,
		SpiceOptions:   s.spiceOptionsFor(ctx, vm.DisplayProfileID),
	}

	// Shares are read here rather than carried on the model, because every
	// caller of buildSpec is about to launch the guest and none of the list
	// views need them.
	if shares, err := s.listShares(ctx, vm.ID); err == nil {
		for _, share := range shares {
			spec.Shares = append(spec.Shares, agentShare{
				ShareID:  share.ID,
				Tag:      share.Tag,
				HostPath: share.HostPath,
				ReadOnly: share.ReadOnly,
			})
		}
	} else {
		s.logger.Warn("reading virtiofs shares for a guest start",
			"vm", vm.ID, "error", err)
	}

	for _, disk := range vm.Disks {
		spec.Disks = append(spec.Disks, agentDisk{
			DiskID: disk.ID, PoolID: disk.PoolID, PoolBackend: disk.PoolBackend,
			PoolTarget: disk.PoolTarget, Slot: disk.Slot, Bus: disk.Bus,
			VolumePath: disk.VolumePath, Format: disk.Format, SizeBytes: disk.SizeBytes,
			BackingPath: disk.BackingPath, CacheMode: disk.CacheMode,
			Discard: disk.Discard, ReadOnly: disk.ReadOnly,
			IsBoot: disk.IsBoot, IsCDROM: disk.IsCDROM,
			BootIndex: disk.BootIndex, ShareRW: disk.ShareRW,
			ReadIOPS: disk.ReadIOPS, WriteIOPS: disk.WriteIOPS,
			ReadBPS: disk.ReadBPS, WriteBPS: disk.WriteBPS,
		})
	}
	for _, nic := range vm.NICs {
		spec.NICs = append(spec.NICs, agentNIC{
			NICID: nic.ID, NetworkID: nic.NetworkID, Bridge: nic.Bridge,
			VLANTag: nic.VLANTag, AttachKind: nic.AttachKind,
			ParentLink: nic.ParentLink, LinkMode: nic.LinkMode,
			Slot: nic.Slot, Model: nic.Model,
			MAC: nic.MAC, TapName: nic.TapName, PCIAddr: nic.PCIAddr,
			LinkDown: nic.LinkDown,
		})
	}
	for _, device := range vm.Devices {
		spec.Devices = append(spec.Devices, agentDevice{
			DeviceID: device.ID, Kind: device.Kind,
			HostAddress: device.HostAddress, Description: device.Description,
		})
	}

	if vm.CloudInit != nil {
		// The hash is read fresh rather than carried on the model, because the
		// model deliberately does not hold it.
		spec.CloudInit = s.cloudInitSpec(ctx, vm.ID, *vm.CloudInit)
	}
	return spec
}

// cloudInitSpec builds the seed configuration, reading the stored hash.
func (s *Service) cloudInitSpec(ctx context.Context, vmID string, cloudInit CloudInit) *agentCloudInit {
	out := &agentCloudInit{
		InstanceID:    cloudInit.InstanceID,
		Hostname:      cloudInit.Hostname,
		Timezone:      cloudInit.Timezone,
		SSHKeys:       cloudInit.SSHKeys,
		DefaultUser:   cloudInit.DefaultUser,
		NetworkMode:   cloudInit.NetworkMode,
		StaticIP:      cloudInit.StaticIP,
		StaticGateway: cloudInit.StaticGateway,
		StaticDNS:     cloudInit.StaticDNS,
		UserDataExtra: cloudInit.UserDataExtra,
	}

	rows, err := s.db.Query(ctx,
		`SELECT COALESCE(password_hash, '') FROM plugin_addon_kvm_cloud_init WHERE vm_id = ?`, vmID)
	if err != nil {
		return out
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err == nil {
			out.PasswordHash = hash
		}
	}
	return out
}

// Start boots a guest.
func (s *Service) Start(ctx context.Context, id string) error {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if vm.IsTemplate {
		// Templates are immutable. Starting one would let it drift from what
		// its clones were made from, which is the whole point of a template.
		return ErrTemplateRunning
	}
	if vm.Running() {
		return nil
	}

	intent := s.beginIntent(ctx, vm, intentStart)
	reply, err := s.call(ctx, vm.EnvironmentID, msgVMStart, agentPayload{
		VMID: id,
		Spec: s.buildSpec(ctx, vm),
	})
	s.finishIntent(ctx, intent, err)
	if err != nil {
		return err
	}
	if reply != nil {
		s.recordNICAddrs(ctx, reply.NICAddrs)
	}
	// The one caller that watched it start, so the one that may set the clock.
	// The next reconcile replaces this with the host's own figure.
	return s.setStateAt(ctx, id, StateRunning, "", time.Now().UTC())
}

// Shutdown asks the guest to power down, reporting whether it had to escalate.
//
// The escalation is returned rather than hidden: a guest that ignores ACPI is
// something the operator should see in the audit row, not a shutdown that
// merely looked slow.
func (s *Service) Shutdown(ctx context.Context, id string, graceSeconds int) (escalated bool, err error) {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return false, err
	}
	if !vm.Running() {
		return false, nil
	}

	// Recorded before the state moves, so no reader can see the guest stopping
	// without the action that explains it.
	intent := s.beginIntent(ctx, vm, intentShutdown)
	if err := s.setState(ctx, id, StateStopping, ""); err != nil {
		s.finishIntent(ctx, intent, err)
		return false, err
	}

	reply, err := s.call(ctx, vm.EnvironmentID, msgVMShutdown, agentPayload{
		VMID: id, GraceSeconds: graceSeconds,
	})
	s.finishIntent(ctx, intent, err)
	if err != nil {
		return false, err
	}

	escalated = reply.ErrorCode == "escalated_to_hard_stop"
	return escalated, s.setState(ctx, id, StateStopped, "")
}

// Stop terminates a guest immediately.
func (s *Service) Stop(ctx context.Context, id string) error {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	// Marked stopping for the duration of the call, because the agent only
	// returns once QEMU is really gone. Writing "stopped" up front is what let
	// the UI offer a Start that the agent then refused as already running.
	intent := s.beginIntent(ctx, vm, intentKill)
	if err := s.setState(ctx, id, StateStopping, ""); err != nil {
		s.finishIntent(ctx, intent, err)
		return err
	}
	_, err = s.call(ctx, vm.EnvironmentID, msgVMStop, agentPayload{VMID: id})
	s.finishIntent(ctx, intent, err)
	if err != nil {
		return err
	}
	return s.setState(ctx, id, StateStopped, "")
}

// Simple lifecycle passthroughs.
func (s *Service) Reset(ctx context.Context, id string) error {
	return s.simpleLifecycle(ctx, id, msgVMReset, "")
}

func (s *Service) Pause(ctx context.Context, id string) error {
	return s.simpleLifecycle(ctx, id, msgVMPause, StatePaused)
}

func (s *Service) Resume(ctx context.Context, id string) error {
	return s.simpleLifecycle(ctx, id, msgVMResume, StateRunning)
}

func (s *Service) simpleLifecycle(ctx context.Context, id, messageType, newState string) error {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !vm.Running() && vm.State != StatePaused {
		return ErrNotRunning
	}
	intent := s.beginIntent(ctx, vm, lifecycleIntentAction(messageType))
	_, err = s.call(ctx, vm.EnvironmentID, messageType, agentPayload{VMID: id})
	s.finishIntent(ctx, intent, err)
	if err != nil {
		return err
	}
	if newState == "" {
		return nil
	}
	return s.setState(ctx, id, newState, "")
}

// setState records a state McHarbor observed, without claiming the guest just
// started. See setStateAt for why that distinction matters.
func (s *Service) setState(ctx context.Context, id, state, lastError string) error {
	return s.setStateAt(ctx, id, state, lastError, time.Time{})
}

// setStateAt records a confirmed state, and when the guest started if the
// caller knows.
//
// Only a caller that watched the guest start may move last_started_at. The
// reconciler writes the running state once a minute for every guest, and when
// this wrote time.Now() unconditionally the column tracked the last sweep
// rather than the boot — so every guest read "less than a minute ago" forever.
// A zero startedAt therefore keeps what is already recorded; the host's own
// figure, which is systemd's ActiveEnterTimestamp and survives both a McHarbor
// and an agent restart, wins over everything.
func (s *Service) setStateAt(ctx context.Context, id, state, lastError string, startedAt time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339)
	query := `UPDATE plugin_addon_kvm_vms SET state = ?, last_error = ?, updated_at = ? WHERE id = ?`
	args := []any{state, nullIfEmpty(lastError), now, id}

	if state == StateRunning {
		stamp := now
		if !startedAt.IsZero() {
			stamp = startedAt.UTC().Format(time.RFC3339)
			query = `UPDATE plugin_addon_kvm_vms
			            SET state = ?, last_error = ?, updated_at = ?, last_started_at = ?
			          WHERE id = ?`
		} else {
			// An observation, not a start: fill the column only if nothing has.
			query = `UPDATE plugin_addon_kvm_vms
			            SET state = ?, last_error = ?, updated_at = ?,
			                last_started_at = COALESCE(last_started_at, ?)
			          WHERE id = ?`
		}
		args = []any{state, nullIfEmpty(lastError), now, stamp, id}
	}

	// Read before writing so the timeline carries transitions rather than
	// writes. The reconciler re-asserts "running" for every guest once a
	// minute; emitting on each of those would bury the real events under a
	// row per guest per minute.
	previous, name, envID := s.stateSnapshot(ctx, id)

	if _, err := s.db.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("recording guest state: %w", err)
	}

	if previous != state {
		s.recordLifecycle(ctx, lifecycleGuest{
			id: id, name: name, envID: envID, state: state, lastError: lastError,
		})
	}
	return nil
}

// lifecycleGuest is what one timeline row needs about the guest it describes.
type lifecycleGuest struct {
	id        string
	name      string
	envID     string
	state     string
	lastError string
}

// stateSnapshot reads the stored state, name and environment of a guest.
//
// A miss returns zero values, which makes setStateAt treat the write as a
// transition — a row that cannot be read is better over-reported than silent.
func (s *Service) stateSnapshot(ctx context.Context, id string) (state, name, envID string) {
	if s.lifecycle == nil {
		return "", "", ""
	}
	rows, err := s.db.Query(ctx,
		`SELECT state, name, environment_id FROM plugin_addon_kvm_vms WHERE id = ?`, id)
	if err != nil {
		return "", "", ""
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		if err := rows.Scan(&state, &name, &envID); err != nil {
			return "", "", ""
		}
	}
	return state, name, envID
}

// setWorkflowTrigger wires the host's workflow bridge after construction, so the
// constructor's callers and tests stay as they are.
func (s *Service) setWorkflowTrigger(w plugin.WorkflowHandle) {
	s.workflows = w
}

// recordLifecycle puts one guest transition on the Logging timeline and fires the
// vm-trigger workflows that accept it.
func (s *Service) recordLifecycle(ctx context.Context, guest lifecycleGuest) {
	if s.lifecycle == nil {
		return
	}
	severity := transitionSeverity(guest.state, guest.lastError)
	metadata := map[string]any{}
	if guest.lastError != "" {
		metadata["error"] = guest.lastError
	}
	if err := s.lifecycle.Record(ctx, plugin.LifecycleEvent{
		EnvironmentID: guest.envID,
		SubjectType:   "vm",
		SubjectID:     guest.id,
		SubjectName:   guest.name,
		EventType:     "guest",
		Action:        guest.state,
		State:         guest.state,
		Severity:      severity,
		Metadata:      metadata,
	}); err != nil {
		s.logger.Warn("kvm: recording a guest transition on the timeline failed",
			"guest", guest.id, "state", guest.state, "error", err)
	}
	s.fireGuestTrigger(ctx, guestTransition{
		GuestID: guest.id, GuestName: guest.name, EnvironmentID: guest.envID,
		State: guest.state, Severity: severity, Metadata: metadata,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// Update edits a defined guest.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (VM, error) {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return VM{}, err
	}

	// Every cold setting is read while building the QEMU command line, so
	// changing one on a guest that is not confirmed stopped would leave the UI
	// describing a machine other than the one executing.
	if vm.State != StateStopped && len(changedColdSettings(req)) > 0 {
		return VM{}, ErrRunning
	}

	if req.Name != nil {
		if err := validateGuestName(*req.Name); err != nil {
			return VM{}, err
		}
		vm.Name = *req.Name
	}
	if req.Description != nil {
		vm.Description = *req.Description
	}
	if req.VCPUs != nil {
		vm.VCPUs = *req.VCPUs
		vm.CPUCores = *req.VCPUs
	}
	if req.MemoryMB != nil {
		vm.MemoryMB = *req.MemoryMB
	}
	if req.MemoryMaxMB != nil {
		vm.MemoryMaxMB = *req.MemoryMaxMB
	}
	if req.CPUModel != nil {
		vm.CPUModel = *req.CPUModel
	}
	if req.Autostart != nil {
		vm.Autostart = *req.Autostart
	}
	if req.AutostartOrder != nil {
		vm.AutostartOrder = *req.AutostartOrder
	}
	if req.AutostartDelayS != nil {
		vm.AutostartDelayS = *req.AutostartDelayS
	}
	if req.MachineType != nil {
		vm.MachineType = *req.MachineType
	}
	if req.Firmware != nil {
		vm.Firmware = *req.Firmware
	}
	if req.SecureBoot != nil {
		vm.SecureBoot = *req.SecureBoot
	}
	if req.TPMEnabled != nil {
		vm.TPMEnabled = *req.TPMEnabled
	}
	if req.DisplayType != nil {
		vm.DisplayType = *req.DisplayType
	}
	if req.DisplayWidth != nil {
		vm.DisplayWidth = *req.DisplayWidth
	}
	if req.DisplayHeight != nil {
		vm.DisplayHeight = *req.DisplayHeight
	}
	if req.VCPUsMax != nil {
		vm.VCPUsMax = *req.VCPUsMax
	}
	if req.VirtioMem != nil {
		vm.VirtioMem = *req.VirtioMem
	}
	if req.HugePages != nil {
		vm.HugePages = *req.HugePages
	}
	if req.NUMAPolicy != nil {
		vm.NUMAPolicy = *req.NUMAPolicy
	}
	if req.NUMAHostNodes != nil {
		vm.NUMAHostNodes = *req.NUMAHostNodes
	}
	if req.IOThread != nil {
		vm.IOThread = *req.IOThread
	}
	if req.DisplayProfileID != nil {
		if err := s.validateDisplayProfileChoice(ctx, *req.DisplayProfileID); err != nil {
			return VM{}, err
		}
		vm.DisplayProfileID = effectiveProfileID(*req.DisplayProfileID)
	}
	if req.VSockEnabled != nil {
		// Allocation, not assignment. The id is unique per host and the index
		// enforces it; letting a caller name one would make a collision
		// possible, and a collision here is silent.
		if *req.VSockEnabled {
			if vm.VSockCID < 3 {
				cid, err := s.allocateVSockCID(ctx, vm.EnvironmentID)
				if err != nil {
					return VM{}, err
				}
				vm.VSockCID = cid
			}
		} else {
			vm.VSockCID = 0
		}
	}

	if err := validateFeatureSet(vm); err != nil {
		return VM{}, err
	}

	// Validated on the merged row rather than on the patch: raising memory past
	// an existing ceiling, or turning Secure Boot on while the firmware stays
	// BIOS, is only visible once the change is applied to what is already there.
	if err := validateVCPUs(vm.VCPUs); err != nil {
		return VM{}, err
	}
	if err := validateMemory(vm.MemoryMB, vm.MemoryMaxMB); err != nil {
		return VM{}, err
	}
	if err := validateMachineType(vm.MachineType); err != nil {
		return VM{}, err
	}
	if err := validateFirmware(vm.Firmware, vm.SecureBoot); err != nil {
		return VM{}, err
	}
	if err := validateDisplayType(vm.DisplayType); err != nil {
		return VM{}, err
	}
	if err := validateDisplayResolution(vm.DisplayWidth, vm.DisplayHeight); err != nil {
		return VM{}, err
	}
	if err := validateCPUModel(vm.CPUModel); err != nil {
		return VM{}, err
	}
	vm.UpdatedAt = time.Now().UTC()

	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_vms
		   SET name = ?, description = ?, vcpus = ?, cpu_cores = ?, cpu_model = ?,
		       memory_mb = ?, memory_max_mb = ?,
		       autostart = ?, autostart_order = ?, autostart_delay_s = ?,
		       machine_type = ?, firmware = ?, secure_boot = ?, tpm_enabled = ?,
		       display_type = ?,
		       vcpus_max = ?, virtio_mem = ?, hugepages = ?,
		       numa_policy = ?, numa_host_nodes = ?, vsock_cid = ?, iothread = ?,
		       display_profile_id = ?, display_width = ?, display_height = ?,
		       updated_at = ?
		 WHERE id = ?`,
		vm.Name, nullIfEmpty(vm.Description), vm.VCPUs, vm.CPUCores, vm.CPUModel,
		vm.MemoryMB, nullIfZero64(vm.MemoryMaxMB),
		boolToInt(vm.Autostart), vm.AutostartOrder, vm.AutostartDelayS,
		vm.MachineType, vm.Firmware, boolToInt(vm.SecureBoot), boolToInt(vm.TPMEnabled),
		vm.DisplayType,
		nullIfZero(vm.VCPUsMax), boolToInt(vm.VirtioMem), boolToInt(vm.HugePages),
		nullIfEmpty(vm.NUMAPolicy), encodeNodes(vm.NUMAHostNodes),
		nullIfZero(vm.VSockCID), boolToInt(vm.IOThread),
		storedProfileID(vm.DisplayProfileID), vm.DisplayWidth, vm.DisplayHeight,
		vm.UpdatedAt.Format(time.RFC3339), id); err != nil {
		if isUniqueViolation(err) {
			return VM{}, ErrNameTaken
		}
		return VM{}, fmt.Errorf("updating virtual machine: %w", err)
	}
	return vm, nil
}

// Delete destroys a guest.
func (s *Service) Delete(ctx context.Context, id string, opts DeleteOptions) error {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	// A guest with linked clones cannot go: their disks are copy-on-write over
	// its own, and removing it would corrupt every one of them. Any guest can be
	// a linked clone's source, not only one marked as a template.
	clones, err := s.linkedClones(ctx, id)
	if err != nil {
		return err
	}
	if len(clones) > 0 {
		return fmt.Errorf("%w: %s", ErrTemplateInUse, strings.Join(clones, ", "))
	}

	// The spec is built here, on the caller's context, because the row is about
	// to be marked deleting and the disks it names are what the host needs to
	// remove them.
	spec := s.buildSpec(ctx, vm)
	intent := s.beginIntent(ctx, vm, intentDelete)
	if err := s.setState(ctx, id, StateDeleting, ""); err != nil {
		s.finishIntent(ctx, intent, err)
		return err
	}

	// Removing a guest's volumes is not quick — a large qcow2 on a slow pool
	// takes minutes — so the work runs on its own context and the caller gets
	// its answer now. The row stays visible as "removing" until it is gone,
	// which is what lets the dialog be closed and the list still tell the
	// truth.
	go s.runDelete(vm, spec, opts, intent)
	return nil
}

// runDelete removes a guest from its host and then from McHarbor.
//
// Its context is deliberately unrelated to the request that started it: the
// operator is expected to close the dialog, and a delete cancelled by that
// would leave a guest half removed.
func (s *Service) runDelete(vm VM, spec *agentSpec, opts DeleteOptions, intent string) {
	ctx, cancel := context.WithTimeout(context.Background(), deleteCeiling)
	defer cancel()

	_, err := s.call(ctx, vm.EnvironmentID, msgVMDelete, agentPayload{
		VMID:          vm.ID,
		DeleteVolumes: opts.DeleteVolumes,
		Spec:          spec,
	})
	s.finishIntent(ctx, intent, err)
	if err != nil {
		// A host that cannot be reached leaves the row in place. Deleting the
		// record of a guest that may still be running would lose the operator's
		// only handle on it, so the state goes back and the reason is kept.
		s.forgetDeleteProgress(vm.ID)
		s.logger.Warn("kvm: delete failed", "vm", vm.ID, "error", err)
		if stateErr := s.setState(ctx, vm.ID, vm.State, err.Error()); stateErr != nil {
			s.logger.Warn("kvm: could not restore state after a failed delete",
				"vm", vm.ID, "error", stateErr)
		}
		return
	}

	// The child rows go with it through ON DELETE CASCADE.
	if _, err := s.db.Exec(ctx, `DELETE FROM plugin_addon_kvm_vms WHERE id = ?`, vm.ID); err != nil {
		// Cleared even here: the row is staying, and leaving it reading
		// "removing 100%" forever would be the least useful of the three
		// possible outcomes.
		s.forgetDeleteProgress(vm.ID)
		s.logger.Warn("kvm: host removed the guest but the row remains",
			"vm", vm.ID, "error", err)
		return
	}
	s.forgetDeleteProgress(vm.ID)
	s.releaseVault(vm.ID)
	s.logger.Info("kvm: guest removed", "vm", vm.ID, "name", vm.Name)
}

// linkedClones names the guests whose disks back onto a template's.
func (s *Service) linkedClones(ctx context.Context, templateID string) ([]string, error) {
	rows, err := s.db.Query(ctx,
		`SELECT name FROM plugin_addon_kvm_vms WHERE template_source_id = ? LIMIT ?`,
		templateID, maxRows)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// AddDisk allocates and attaches another disk.
func (s *Service) AddDisk(ctx context.Context, vmID string, req DiskRequest) (Disk, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return Disk{}, err
	}
	live, err := s.liveGuest(ctx, vm)
	if err != nil {
		return Disk{}, err
	}
	if bus := defaultString(req.Bus, defaultDiskBus(vm.GuestOSFamily)); live && (bus == "sata" || bus == "ide") {
		// Refused before a volume is allocated for a disk that could never be
		// plugged in: SATA and IDE have no hot-plug.
		return Disk{}, ErrHotplugBus
	}

	pool, err := s.resolvePool(ctx, vm.EnvironmentID, PoolRef{ID: req.PoolID, Backend: req.PoolBackend, Target: req.PoolTarget})
	if err != nil {
		return Disk{}, err
	}

	slot, err := s.nextDiskSlot(ctx, vmID)
	if err != nil {
		return Disk{}, err
	}
	sizeBytes := req.SizeGB * 1024 * 1024 * 1024

	reply, err := s.call(ctx, vm.EnvironmentID, msgVMDiskCreate, agentPayload{
		VMID: vmID,
		Disk: &agentDisk{
			PoolID: pool.ID, PoolBackend: pool.Backend, PoolTarget: pool.Target,
			Slot: slot, SizeBytes: sizeBytes, Format: "qcow2", Discard: true,
		},
	})
	if err != nil {
		return Disk{}, err
	}

	volumePath := ""
	if reply.Disk != nil {
		volumePath = reply.Disk.VolumePath
		if reply.Disk.SizeBytes > 0 {
			sizeBytes = reply.Disk.SizeBytes
		}
	}

	now := time.Now().UTC()
	disk := Disk{
		ID: xid.New().String(), VMID: vmID,
		PoolID: pool.ID, PoolBackend: pool.Backend, PoolTarget: pool.Target,
		Slot: slot, Bus: defaultString(req.Bus, defaultDiskBus(vm.GuestOSFamily)),
		VolumePath: volumePath, Format: "qcow2", SizeBytes: sizeBytes,
		CacheMode: "none", Discard: true,
		CreatedAt: now, UpdatedAt: now,
	}

	if live {
		return s.addDiskLive(ctx, vm, disk)
	}
	if err := s.insertDisk(ctx, disk); err != nil {
		return Disk{}, err
	}
	return disk, nil
}

// insertDisk records one data disk.
func (s *Service) insertDisk(ctx context.Context, disk Disk) error {
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_disks
			(id, vm_id, pool_id, pool_backend, pool_target, slot, bus, volume_path,
			 format, size_bytes, cache_mode, discard, readonly, is_boot, is_cdrom,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, 0, 0, ?, ?)`,
		disk.ID, disk.VMID, disk.PoolID, disk.PoolBackend, disk.PoolTarget,
		disk.Slot, disk.Bus, disk.VolumePath, disk.Format, disk.SizeBytes, disk.CacheMode,
		disk.CreatedAt.Format(time.RFC3339), disk.UpdatedAt.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("recording disk: %w", err)
	}
	return nil
}

// ResizeDisk grows a disk.
//
// Shrinking is refused outright rather than warned about. Shrinking a live
// filesystem from outside destroys data, and there is no confirmation dialog
// that makes that acceptable.
func (s *Service) ResizeDisk(ctx context.Context, vmID, diskID string, sizeGB int64) (Disk, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return Disk{}, err
	}

	var target *Disk
	for i := range vm.Disks {
		if vm.Disks[i].ID == diskID {
			target = &vm.Disks[i]
			break
		}
	}
	if target == nil {
		return Disk{}, ErrNotFound
	}

	if target.IsCDROM {
		// A disc is a file on the host, not a volume this plugin allocated.
		return Disk{}, ErrNotMedia
	}

	newSize := sizeGB * 1024 * 1024 * 1024
	if newSize < target.SizeBytes {
		return Disk{}, ErrDiskShrink
	}

	live, err := s.liveGuest(ctx, vm)
	if live && isBootDisk(vm.Disks, *target) {
		// Checked before the host is asked anything: the boot disk never
		// changes under the operating system that runs from it.
		return Disk{}, ErrBootDiskRunning
	}
	if err != nil {
		return Disk{}, err
	}

	var reply *agentPayload
	if live {
		grown := *toAgentDisk(*target)
		grown.SizeBytes = newSize
		reply, err = s.hotplug(ctx, vm, msgVMDiskHotplug, agentHotplug{Action: "resize", Disk: &grown})
	} else {
		reply, err = s.call(ctx, vm.EnvironmentID, msgVMDiskResize, agentPayload{
			VMID: vmID,
			Disk: &agentDisk{
				DiskID: diskID, PoolID: target.PoolID, PoolBackend: target.PoolBackend,
				PoolTarget: target.PoolTarget, Slot: target.Slot,
				VolumePath: target.VolumePath, SizeBytes: newSize,
			},
		})
	}
	if err != nil {
		return Disk{}, err
	}
	if reply.Disk != nil && reply.Disk.SizeBytes > 0 {
		newSize = reply.Disk.SizeBytes
	}

	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx,
		`UPDATE plugin_addon_kvm_disks SET size_bytes = ?, updated_at = ? WHERE id = ?`,
		newSize, now.Format(time.RFC3339), diskID); err != nil {
		return Disk{}, fmt.Errorf("recording disk size: %w", err)
	}

	target.SizeBytes = newSize
	target.UpdatedAt = now

	// A grown disk no longer matches the dirty bitmap tracking it: the new
	// clusters were never recorded as dirty, so an incremental taken after
	// this would omit them and restore to a disk missing everything written
	// into the new space.
	if err := s.InvalidateChain(ctx, diskID,
		"this disk was resized, so its change log no longer covers all of it"); err != nil {
		s.logger.Warn("kvm: invalidating a chain after a resize", "disk", diskID, "error", err)
	}
	return *target, nil
}

// ChangeDiskBus swaps the bus a disk is attached to, which only matters before
// the guest has an operating system that depends on it.
//
// Refused while the guest runs: the bus is part of the QEMU command line, and a
// guest cannot have its controller changed underneath it. It is also why this
// is a deliberate action rather than a default that follows the OS family
// forever — a Windows guest that installed onto virtio with hand-loaded drivers
// would be made unbootable by a silent switch.
func (s *Service) ChangeDiskBus(ctx context.Context, vmID, diskID, bus string) (Disk, error) {
	if err := validateDiskBus(bus); err != nil {
		return Disk{}, err
	}

	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return Disk{}, err
	}
	if vm.State != StateStopped {
		return Disk{}, ErrRunning
	}

	var target *Disk
	for i := range vm.Disks {
		if vm.Disks[i].ID == diskID {
			target = &vm.Disks[i]
			break
		}
	}
	if target == nil {
		return Disk{}, ErrNotFound
	}
	if target.IsCDROM {
		// A disc is always SATA: virtio-blk has no removable-media semantics.
		return Disk{}, ErrNotMedia
	}

	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx,
		`UPDATE plugin_addon_kvm_disks SET bus = ?, updated_at = ? WHERE id = ?`,
		bus, now.Format(time.RFC3339), diskID); err != nil {
		return Disk{}, fmt.Errorf("recording disk bus: %w", err)
	}

	target.Bus = bus
	target.UpdatedAt = now
	return *target, nil
}

// DeleteDisk removes a disk.
func (s *Service) DeleteDisk(ctx context.Context, vmID, diskID string) error {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return err
	}

	for _, disk := range vm.Disks {
		if disk.ID != diskID {
			continue
		}
		if disk.IsCDROM {
			// Ejecting must not reach vm_disk_delete: the volume path is the
			// library's own ISO, and the agent would delete the file itself.
			return s.EjectMedia(ctx, vmID, diskID)
		}
		live, err := s.liveGuest(ctx, vm)
		if live && isBootDisk(vm.Disks, disk) {
			return ErrBootDiskRunning
		}
		if err != nil {
			return err
		}
		if disk.IsBoot {
			return fmt.Errorf("the boot disk cannot be removed; delete the virtual machine instead")
		}
		if live {
			return s.deleteDiskLive(ctx, vm, disk)
		}
		if _, err := s.call(ctx, vm.EnvironmentID, msgVMDiskDelete, agentPayload{
			VMID: vmID,
			Disk: &agentDisk{
				DiskID: diskID, PoolID: disk.PoolID, PoolBackend: disk.PoolBackend,
				PoolTarget: disk.PoolTarget, VolumePath: disk.VolumePath,
			},
		}); err != nil {
			return err
		}
		// The chain goes with the disk: its bitmap lived inside the image that
		// has just been removed, so nothing it recorded can be built on.
		return s.forgetDisk(ctx, diskID)
	}
	return ErrNotFound
}

// AttachMedia inserts an installer disc.
//
// A disc is recorded as a read-only CD-ROM row in the disks table rather than
// as its own kind: that is the only shape the agent turns into QEMU arguments,
// and it survives a restart because Start rebuilds the spec from these rows.
func (s *Service) AttachMedia(ctx context.Context, vmID string, req MediaRequest) (Disk, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return Disk{}, err
	}
	if err := validateMediaPath(req.SourcePath); err != nil {
		return Disk{}, err
	}
	return s.placeMedia(ctx, vm, req.SourcePath)
}

// AttachGuestAgentDisc inserts the guest-agent installer disc the host built
// at startup, so the operator never has to find an image.
//
// One disc per family, reused across every guest on that host; this call is a
// stat on the agent, never a build.
func (s *Service) AttachGuestAgentDisc(ctx context.Context, vmID string) (Disk, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return Disk{}, err
	}
	// Asked before the disc is built, so a guest that cannot take it live
	// hears why rather than waiting for a disc it will not get.
	live, err := s.liveGuest(ctx, vm)
	if err != nil {
		return Disk{}, err
	}
	if _, found := emptyDrive(vm.Disks); live && !found {
		return Disk{}, ErrMediaNoDrive
	}

	reply, err := s.call(ctx, vm.EnvironmentID, msgVMGuestAgentDisc, agentPayload{
		VMID:        vmID,
		GuestFamily: vm.GuestOSFamily,
	})
	if err != nil {
		return Disk{}, err
	}
	if reply.ErrorCode == agentGuestDiscPending {
		return Disk{}, ErrGuestDiscPending
	}
	// The path is held to the same rules as an operator-supplied one. The agent
	// is trusted to build the disc, not to name an arbitrary host file that
	// QEMU would then open.
	if err := validateMediaPath(reply.MediaPath); err != nil {
		return Disk{}, err
	}
	return s.placeMedia(ctx, vm, reply.MediaPath)
}

// recordNICAddrs stores the PCI slot the host gave each NIC.
//
// Written once, on the first start that reports one, and never overwritten: the
// recorded value is what later starts pin, so taking a fresh reading would
// re-record whatever a shifted layout produced and defeat the pinning.
func (s *Service) recordNICAddrs(ctx context.Context, addrs map[string]string) {
	for nicID, addr := range addrs {
		if nicID == "" || addr == "" {
			continue
		}
		if _, err := s.db.Exec(ctx, `
			UPDATE plugin_addon_kvm_nics
			   SET pci_addr = ?, updated_at = ?
			 WHERE id = ? AND COALESCE(pci_addr, '') = ''`,
			addr, time.Now().UTC().Format(time.RFC3339), nicID); err != nil {
			// Not fatal: the guest is running, and the next start reports the
			// same address again.
			s.logger.Warn("kvm: recording the NIC PCI address failed",
				"nic", nicID, "error", err)
		}
	}
}

// EjectMedia removes a disc, leaving the file on the host untouched.
func (s *Service) EjectMedia(ctx context.Context, vmID, diskID string) error {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return err
	}
	live, err := s.liveGuest(ctx, vm)
	if err != nil {
		return err
	}
	for _, disk := range vm.Disks {
		if disk.ID != diskID {
			continue
		}
		if !disk.IsCDROM {
			return ErrNotMedia
		}
		if live {
			// The drive stays: SATA cannot unplug it, and the row keeps
			// describing it, empty, so the next disc can go in live.
			if disk.VolumePath == "" {
				return nil
			}
			if _, err := s.hotplug(ctx, vm, msgVMMediaChange,
				agentHotplug{Action: "eject", Disk: toAgentDisk(disk)}); err != nil {
				return err
			}
			return s.setDrivePath(ctx, disk.ID, "")
		}
		_, err := s.db.Exec(ctx,
			`DELETE FROM plugin_addon_kvm_disks WHERE id = ? AND vm_id = ?`, diskID, vmID)
		return err
	}
	return ErrNotFound
}

// recordMedia writes one CD-ROM row.
//
// The pool columns are NOT NULL and deliberately empty: a disc belongs to no
// pool, and an empty target is what keeps it out of the volume paths.
func (s *Service) recordMedia(ctx context.Context, vmID string, slot int, path string) (Disk, error) {
	now := time.Now().UTC()
	disk := Disk{
		ID: xid.New().String(), VMID: vmID,
		Slot: slot, Bus: "sata", VolumePath: path, Format: "raw",
		CacheMode: "none", ReadOnly: true, IsCDROM: true,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_disks
			(id, vm_id, pool_id, pool_backend, pool_target, slot, bus, volume_path,
			 format, size_bytes, cache_mode, discard, readonly, is_boot, is_cdrom,
			 created_at, updated_at)
		VALUES (?, ?, '', '', '', ?, ?, ?, ?, 0, ?, 0, 1, 0, 1, ?, ?)`,
		disk.ID, disk.VMID, disk.Slot, disk.Bus, disk.VolumePath, disk.Format,
		disk.CacheMode, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return Disk{}, fmt.Errorf("recording installer disc: %w", err)
	}
	return disk, nil
}

// nextDiskSlot returns a free slot, which is one past the highest rather than
// the row count: ejecting a disc leaves a gap the UNIQUE (vm_id, slot) index
// would otherwise collide with.
func (s *Service) nextDiskSlot(ctx context.Context, vmID string) (int, error) {
	rows, err := s.db.Query(ctx,
		`SELECT COALESCE(MAX(slot), -1) FROM plugin_addon_kvm_disks WHERE vm_id = ?`, vmID)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var highest int
		if err := rows.Scan(&highest); err != nil {
			return 0, err
		}
		return highest + 1, nil
	}
	return 0, rows.Err()
}

// AddNIC attaches another interface.
func (s *Service) AddNIC(ctx context.Context, vmID string, req NICRequest) (NIC, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return NIC{}, err
	}
	if req.Model != "" && !nicModels[req.Model] {
		return NIC{}, invalid(codeBadRequest, fmt.Sprintf("%q is not an interface type", req.Model))
	}
	live, err := s.liveGuest(ctx, vm)
	if err != nil {
		return NIC{}, err
	}
	nic, err := s.attachNIC(ctx, vmID, req)
	if err != nil || !live {
		return nic, err
	}
	// The row comes first because it is what allocates the slot and a unique
	// MAC; a guest that will not take the NIC takes the row with it.
	reply, err := s.hotplug(ctx, vm, msgVMNICHotplug, agentHotplug{Action: "attach", NIC: toAgentNIC(nic)})
	if err != nil {
		if _, delErr := s.db.Exec(context.WithoutCancel(ctx),
			`DELETE FROM plugin_addon_kvm_nics WHERE id = ? AND vm_id = ?`, nic.ID, vmID); delErr != nil {
			s.logger.Warn("kvm: removing the row of a NIC the guest refused", "nic", nic.ID, "error", delErr)
		}
		return NIC{}, err
	}
	nic.Notice = reply.Notice
	return nic, nil
}

// DeleteNIC removes an interface.
func (s *Service) DeleteNIC(ctx context.Context, vmID, nicID string) error {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return err
	}
	live, err := s.liveGuest(ctx, vm)
	if err != nil {
		return err
	}
	if live {
		var target *NIC
		for i := range vm.NICs {
			if vm.NICs[i].ID == nicID {
				target = &vm.NICs[i]
			}
		}
		if target == nil {
			return ErrNotFound
		}
		if _, err := s.hotplug(ctx, vm, msgVMNICHotplug, agentHotplug{Action: "detach", NIC: toAgentNIC(*target)}); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx,
		`DELETE FROM plugin_addon_kvm_nics WHERE id = ? AND vm_id = ?`, nicID, vmID)
	return err
}

// CreateSnapshot takes a snapshot.
//
// The consistency level is decided by what is actually available rather than
// claimed: with a guest agent the filesystems are frozen first, without one it
// is crash-consistent and the UI says so.
func (s *Service) CreateSnapshot(ctx context.Context, vmID string, req SnapshotRequest) (Snapshot, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return Snapshot{}, err
	}

	name, err := validateSnapshotName(req.Name)
	if err != nil {
		return Snapshot{}, err
	}
	for _, existing := range vm.Snapshots {
		if strings.EqualFold(existing.Name, name) {
			return Snapshot{}, ErrNameTaken
		}
	}
	if len(vm.Snapshots) >= maxSnapshotTotal {
		return Snapshot{}, ErrSnapshotTooMany
	}

	// The new snapshot hangs off whatever the guest's disks currently descend
	// from, which is what turns a revert into a branch rather than a rewrite.
	parentID := vm.CurrentSnapshotID
	depth := branchDepth(vm.Snapshots, parentID)
	if depth >= maxSnapshotChain {
		return Snapshot{}, ErrChainTooDeep
	}

	consistency := ConsistencyCrash
	// A guest that can quiesce itself has nothing to gain from a pause, so the
	// pause is what a guest without an agent gets instead.
	pause := req.PauseGuest && vm.GuestAgentState != "connected"
	switch {
	case vm.GuestAgentState == "connected":
		consistency = ConsistencyFilesystem
	case pause && vm.Running():
		consistency = ConsistencyPaused
	}

	// The name the operator typed is theirs; the agent gets a slug, because the
	// same string becomes a path component, a ZFS snapshot name and an LVM
	// volume name on the other side.
	handle := snapshotSlug(name)

	// However this ends, the counter goes: a failed snapshot that left one
	// behind would show a guest snapshotting forever.
	defer s.clearSnapshotProgress(vmID)

	// Only a snapshot that halts a running guest is a lifecycle action; one taken
	// under a guest-agent freeze leaves the guest running throughout.
	var intent string
	if vm.Running() && (pause || req.WithMemory) {
		intent = s.beginIntent(ctx, vm, intentSnapshotPause)
	}
	reply, err := s.call(ctx, vm.EnvironmentID, msgVMSnapshotCreate, agentPayload{
		VMID:         vmID,
		SnapshotName: handle,
		WithMemory:   req.WithMemory,
		PauseGuest:   pause,
		Consistency:  consistency,
		Spec:         s.buildSpec(ctx, vm),
	})
	s.finishIntent(ctx, intent, err)
	if err != nil {
		return Snapshot{}, err
	}

	// A live qcow2 snapshot moves the guest's active file, so the rows are
	// brought back in line before anything else uses them. Done before the
	// snapshot row is written: a failure here must not leave a snapshot on
	// record whose disks point at the wrong files.
	s.persistMovedDisks(ctx, vmID, reply.Disks)

	mechanism := "qcow2"
	if len(vm.Disks) > 0 {
		mechanism = vm.Disks[0].PoolBackend
	}
	if reply.SnapshotName != "" {
		handle = reply.SnapshotName
	}
	if reply.Consistency != "" {
		// The agent is the only thing that knows whether the freeze actually
		// took, so its answer wins over what was asked for.
		consistency = reply.Consistency
	}

	now := time.Now().UTC()
	snapshot := Snapshot{
		ID: xid.New().String(), VMID: vmID, ParentID: parentID,
		Name: name, Description: req.Description,
		Mechanism: mechanism, Consistency: consistency,
		WithMemory: req.WithMemory, ChainDepth: depth + 1, Handle: handle,
		CreatedAt: now, UpdatedAt: now,
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_snapshots
			(id, vm_id, parent_id, name, description, mechanism, consistency,
			 with_memory, chain_depth, handle, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.ID, snapshot.VMID, nullIfEmpty(snapshot.ParentID), snapshot.Name,
		nullIfEmpty(snapshot.Description), snapshot.Mechanism, snapshot.Consistency,
		boolToInt(snapshot.WithMemory), snapshot.ChainDepth, snapshot.Handle,
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		if isUniqueViolation(err) {
			return Snapshot{}, ErrNameTaken
		}
		return Snapshot{}, fmt.Errorf("recording snapshot: %w", err)
	}
	if err := s.setCurrentSnapshot(ctx, vmID, snapshot.ID); err != nil {
		// The snapshot itself is taken and recorded. Failing here would tell
		// the operator it did not happen, so it is logged and the next
		// snapshot's parent is the only thing that suffers.
		s.logger.Error("kvm: recording the guest's current snapshot failed",
			"vm", vmID, "snapshot", snapshot.ID, "error", err)
	}
	return snapshot, nil
}

// persistMovedDisks writes back a disk whose active file the agent moved.
//
// Only the live qcow2 mechanism does this: `blockdev-snapshot-sync` freezes the
// file the guest was writing to and hands QEMU a new overlay, so the path on
// record stops being the one in use. Left unwritten, the guest's next start
// would open the frozen snapshot and every write since would be invisible.
//
// Best effort per row, and logged rather than returned: the snapshot itself
// succeeded, and failing the operation here would tell the operator their
// snapshot did not happen when it did.
func (s *Service) persistMovedDisks(ctx context.Context, vmID string, disks []agentDisk) {
	stamp := time.Now().UTC().Format(time.RFC3339)
	for _, disk := range disks {
		if disk.VolumePath == "" {
			continue
		}
		var err error
		if disk.DiskID != "" {
			_, err = s.db.Exec(ctx, `
				UPDATE plugin_addon_kvm_disks SET volume_path = ?, updated_at = ?
				 WHERE id = ? AND vm_id = ?`,
				disk.VolumePath, stamp, disk.DiskID, vmID)
		} else {
			// A disk the agent could only identify by slot. Still addressable,
			// because a slot is unique within a guest.
			_, err = s.db.Exec(ctx, `
				UPDATE plugin_addon_kvm_disks SET volume_path = ?, updated_at = ?
				 WHERE vm_id = ? AND slot = ?`,
				disk.VolumePath, stamp, vmID, disk.Slot)
		}
		if err != nil {
			s.logger.Error("kvm: recording a disk's new path after a snapshot failed; "+
				"the guest may start from the wrong file",
				"vm", vmID, "slot", disk.Slot, "path", disk.VolumePath, "error", err)
		}
	}
}

// revertGraceSeconds is how long a guest gets to shut down cleanly before a
// revert takes its disks. The same figure the Stop button defaults to.
const revertGraceSeconds = 60

// RevertSnapshot rolls a guest back.
//
// Everything written since the snapshot is discarded — that is what a revert
// is — but the snapshots taken since it are not: they stay as their own branch,
// and the guest's current-snapshot pointer moves to the target. A snapshot
// taken afterwards becomes the target's second child, which is how the list
// becomes a tree.
func (s *Service) RevertSnapshot(ctx context.Context, vmID, snapshotID string, req RevertRequest) error {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return err
	}
	if vm.Running() && !req.StopFirst {
		return ErrRunning
	}
	// Reverting moves every disk back in time while its dirty bitmap keeps
	// describing the future it came from. An incremental taken afterwards
	// would restore to a disk that is neither state, so every chain on this
	// machine ends here. Done before the revert rather than after: if the
	// revert half-succeeds, the chains must still be treated as broken.
	if err := s.InvalidateChainsForVM(ctx, vmID,
		"this machine was reverted to a snapshot, so its change logs no longer match its disks"); err != nil {
		s.logger.Warn("kvm: invalidating chains before a revert", "vm", vmID, "error", err)
	}
	var target *Snapshot
	for i := range vm.Snapshots {
		if vm.Snapshots[i].ID == snapshotID {
			target = &vm.Snapshots[i]
			break
		}
	}
	if target == nil {
		return ErrNotFound
	}
	if s.revertProgressFor(vmID) != nil {
		return fmt.Errorf("%w: a revert is already running for this guest", ErrRunning)
	}

	// Everything past here happens on its own context. A revert shuts the guest
	// down — which waits out the guest's ACPI grace — swaps its disks and
	// starts it again, and holding an HTTP request open for all of that is what
	// made this fail at exactly sixty seconds with "context deadline exceeded".
	s.recordRevertProgress(vmID, 5, "starting")
	go s.runRevert(vmID, snapshotID, target.Handle, target.ID, req, intentOriginFrom(ctx))
	return nil
}

// revertTimeout bounds the whole sequence: stop, swap, start. Generous, because
// the stop alone is the guest's grace plus however long it takes to flush.
const revertTimeout = 30 * time.Minute

// runRevert is the body RevertSnapshot detaches.
//
// Progress is recorded at each step rather than returned, because by the time
// anything interesting happens the request that asked for it is long answered.
func (s *Service) runRevert(vmID, snapshotID, handle, targetID string, req RevertRequest, origin intentOrigin) {
	// The caller's identity rides along so the stop and start this performs are
	// recorded as theirs, marked as part of a revert.
	ctx, cancel := context.WithTimeout(
		withIntentOrigin(context.Background(), origin.actor, "snapshot_revert"), revertTimeout)
	defer cancel()
	defer s.recordRevertProgress(vmID, 100, "done")

	fail := func(err error) {
		s.logger.Error("kvm: reverting a snapshot failed", "vm", vmID, "error", err)
		if stateErr := s.setState(ctx, vmID, StateStopped, err.Error()); stateErr != nil {
			s.logger.Warn("kvm: recording a failed revert", "vm", vmID, "error", stateErr)
		}
	}

	vm, err := s.Get(ctx, vmID)
	if err != nil {
		fail(err)
		return
	}

	restart := false
	if vm.Running() {
		s.recordRevertProgress(vmID, 20, "stopping")
		// The same shutdown the Stop button performs, escalation included: a
		// guest that ignores ACPI must not leave the revert half-done, and the
		// call returns only once QEMU is really gone.
		if _, err := s.Shutdown(ctx, vmID, revertGraceSeconds); err != nil {
			fail(err)
			return
		}
		restart = req.RestartAfter
		if vm, err = s.Get(ctx, vmID); err != nil {
			fail(err)
			return
		}
	}

	s.recordRevertProgress(vmID, 50, "reverting")
	reply, err := s.call(ctx, vm.EnvironmentID, msgVMSnapshotRevert, agentPayload{
		VMID: vmID, SnapshotID: snapshotID, SnapshotName: handle,
		Spec: s.buildSpec(ctx, vm),
	})
	if err != nil {
		fail(err)
		return
	}
	// A revert off a live qcow2 chain opens a fresh branch on top of the frozen
	// file, so the stored path changes again.
	s.persistMovedDisks(ctx, vmID, reply.Disks)

	if err := s.setCurrentSnapshot(ctx, vmID, targetID); err != nil {
		fail(err)
		return
	}
	if !restart {
		return
	}

	// The disks are already back, so a failure to boot is reported as itself
	// rather than as a failed revert.
	s.recordRevertProgress(vmID, 80, "starting_guest")
	if err := s.Start(ctx, vmID); err != nil {
		s.logger.Error("kvm: the guest could not be started after its revert",
			"vm", vmID, "error", err)
		if stateErr := s.setState(ctx, vmID, StateStopped,
			"the snapshot was restored but the guest did not start: "+err.Error()); stateErr != nil {
			s.logger.Warn("kvm: recording a failed start after a revert", "vm", vmID, "error", stateErr)
		}
	}
}

// DeleteSnapshot drops a snapshot and folds its data into the branch below it.
//
// A snapshot with two branches under it cannot go: the merge rewrites the layer
// its children read through, so removing it would take one sibling's history
// with it. That is refused here rather than discovered afterwards.
func (s *Service) DeleteSnapshot(ctx context.Context, vmID, snapshotID string) error {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return err
	}
	var target *Snapshot
	for i := range vm.Snapshots {
		if vm.Snapshots[i].ID == snapshotID {
			target = &vm.Snapshots[i]
			break
		}
	}
	if target == nil {
		return ErrNotFound
	}

	child := childOf(vm.Snapshots, snapshotID)
	if child == nil && target.ChildCount > 1 {
		return ErrSnapshotBranched
	}

	payload := agentPayload{
		VMID: vmID, SnapshotID: snapshotID, SnapshotName: target.Handle,
		Spec: s.buildSpec(ctx, vm),
	}
	if child != nil {
		payload.ChildSnapshot = child.Handle
	}
	reply, err := s.call(ctx, vm.EnvironmentID, msgVMSnapshotDelete, payload)
	if err != nil {
		return err
	}
	// Merging the last snapshot away under a running guest moves which file the
	// guest writes to, and the agent says so.
	s.persistMovedDisks(ctx, vmID, reply.Disks)

	// The children inherit the deleted node's parent: their data now reads
	// through the layer it was merged into.
	if _, err := s.db.Exec(ctx,
		`UPDATE plugin_addon_kvm_snapshots SET parent_id = ? WHERE vm_id = ? AND parent_id = ?`,
		nullIfEmpty(target.ParentID), vmID, snapshotID); err != nil {
		return fmt.Errorf("re-parenting the snapshots below: %w", err)
	}
	if _, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_kvm_snapshots WHERE id = ?`, snapshotID); err != nil {
		return err
	}
	if vm.CurrentSnapshotID == snapshotID {
		// The guest was sitting on the node that just went; it now descends
		// from that node's parent, which is where the merge left it.
		return s.setCurrentSnapshot(ctx, vmID, target.ParentID)
	}
	return s.reindexChainDepth(ctx, vmID)
}

// reindexChainDepth recomputes every snapshot's depth after the tree changed.
func (s *Service) reindexChainDepth(ctx context.Context, vmID string) error {
	snapshots, err := s.Snapshots(ctx, vmID)
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		depth := branchDepth(snapshots, snapshot.ParentID) + 1
		if depth == snapshot.ChainDepth {
			continue
		}
		if _, err := s.db.Exec(ctx,
			`UPDATE plugin_addon_kvm_snapshots SET chain_depth = ? WHERE id = ?`,
			depth, snapshot.ID); err != nil {
			return fmt.Errorf("reindexing snapshot depth: %w", err)
		}
	}
	return nil
}

// Clone copies a guest.
//
// Everything that makes a guest itself is regenerated: a new xid, a new machine
// UUID, new MAC addresses and a new cloud-init instance-id. Without the last
// two, a clone comes up answering to the template's hostname with the
// template's SSH host keys — the classic clone footgun.
func (s *Service) Clone(ctx context.Context, sourceID string, req CloneRequest) (VM, error) {
	source, err := s.Get(ctx, sourceID)
	if err != nil {
		return VM{}, err
	}
	if source.Running() {
		return VM{}, ErrRunning
	}

	now := time.Now().UTC()
	clone := source
	clone.ID = xid.New().String()
	clone.Name = req.Name
	clone.UUID = uuid.NewString()
	clone.State = StateDefined
	clone.IsTemplate = false
	clone.CreatedAt = now
	clone.UpdatedAt = now
	clone.LastStartedAt = nil
	if req.Linked {
		clone.TemplateSourceID = sourceID
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_vms
			(id, environment_id, name, description, state, guest_os_family, guest_os_variant,
			 vcpus, cpu_model, cpu_sockets, cpu_cores, cpu_threads, memory_mb, memory_max_mb,
			 machine_type, firmware, secure_boot, tpm_enabled, display_type,
			 autostart, autostart_order, autostart_delay_s, is_template, template_source_id,
			 uuid, created_at, updated_at, display_profile_id, display_width, display_height)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, 0, ?, ?, ?, ?, ?, ?, ?)`,
		clone.ID, clone.EnvironmentID, clone.Name, nullIfEmpty(clone.Description), clone.State,
		nullIfEmpty(clone.GuestOSFamily), nullIfEmpty(clone.GuestOSVariant),
		clone.VCPUs, clone.CPUModel, clone.CPUSockets, clone.CPUCores, clone.CPUThreads,
		clone.MemoryMB, nullIfZero64(clone.MemoryMaxMB), clone.MachineType, clone.Firmware,
		boolToInt(clone.SecureBoot), boolToInt(clone.TPMEnabled), clone.DisplayType,
		nullIfEmpty(clone.TemplateSourceID), clone.UUID,
		now.Format(time.RFC3339), now.Format(time.RFC3339), storedProfileID(clone.DisplayProfileID),
		clone.DisplayWidth, clone.DisplayHeight); err != nil {
		if isUniqueViolation(err) {
			return VM{}, ErrNameTaken
		}
		return VM{}, fmt.Errorf("recording clone: %w", err)
	}

	// New MACs, so the two guests can coexist on one segment.
	clone.NICs = nil
	for _, nic := range source.NICs {
		created, err := s.attachNIC(ctx, clone.ID, NICRequest{
			NetworkID: nic.NetworkID, Bridge: nic.Bridge,
			VLANTag: nic.VLANTag, AttachKind: nic.AttachKind,
			ParentLink: nic.ParentLink, LinkMode: nic.LinkMode,
			Model: nic.Model,
		})
		if err != nil {
			return VM{}, err
		}
		clone.NICs = append(clone.NICs, created)
	}

	// A new instance-id, so cloud-init re-runs.
	if source.CloudInit != nil {
		cloudInit := *source.CloudInit
		cloudInit.VMID = clone.ID
		cloudInit.InstanceID = xid.New().String()
		cloudInit.Hostname = guestHostname(req.Name)
		if _, err := s.db.Exec(ctx, `
			INSERT INTO plugin_addon_kvm_cloud_init
				(vm_id, hostname, timezone, ssh_keys, default_user, password_hash,
				 network_mode, static_ip, static_gateway, static_dns, user_data_extra,
				 instance_id, created_at, updated_at)
			SELECT ?, ?, timezone, ssh_keys, default_user, password_hash,
			       network_mode, static_ip, static_gateway, static_dns, user_data_extra,
			       ?, ?, ?
			  FROM plugin_addon_kvm_cloud_init WHERE vm_id = ?`,
			clone.ID, nullIfEmpty(cloudInit.Hostname), cloudInit.InstanceID,
			now.Format(time.RFC3339), now.Format(time.RFC3339), sourceID); err != nil {
			return VM{}, fmt.Errorf("copying cloud-init: %w", err)
		}
		clone.CloudInit = &cloudInit
	}

	reply, err := s.call(ctx, source.EnvironmentID, msgVMCloneRequest, agentPayload{
		VMID: sourceID,
		Clone: &agentClone{
			TargetVMID: clone.ID,
			Linked:     req.Linked,
			Spec:       s.buildSpec(ctx, clone),
		},
		Spec: s.buildSpec(ctx, source),
	})
	if err != nil {
		if _, dbErr := s.db.Exec(context.Background(),
			`DELETE FROM plugin_addon_kvm_vms WHERE id = ?`, clone.ID); dbErr != nil {
			s.logger.Warn("rolling back a failed clone", "vm", clone.ID, "error", dbErr)
		}
		return VM{}, err
	}
	// The clone's disks are the agent's answer, recorded against the new guest:
	// the path, format and backing reference the pool actually produced, which
	// on a CoW pool are not something this side could have predicted.
	arrived := make(map[int]agentDisk, len(reply.Disks))
	for _, disk := range reply.Disks {
		arrived[disk.Slot] = disk
	}
	clone.Disks = nil
	for _, disk := range source.Disks {
		copied := disk
		copied.ID = xid.New().String()
		copied.VMID = clone.ID
		copied.CreatedAt = now
		copied.UpdatedAt = now
		if disk.IsCDROM {
			// The disc stays the same file on the host; only real volumes are
			// copied or backed by the source.
			copied.BackingPath = ""
		} else if made, ok := arrived[disk.Slot]; ok && made.VolumePath != "" {
			copied.VolumePath = made.VolumePath
			copied.BackingPath = made.BackingPath
			if made.Format != "" {
				copied.Format = made.Format
			}
			if made.SizeBytes > 0 {
				copied.SizeBytes = made.SizeBytes
			}
		} else {
			// An agent that cloned without saying where. The guest cannot start
			// on a guessed path, so the row says so rather than pretending.
			s.logger.Warn("kvm: the host did not report a cloned disk's path",
				"vm", clone.ID, "slot", disk.Slot)
			if req.Linked {
				copied.BackingPath = disk.VolumePath
			}
			copied.VolumePath = fmt.Sprintf("%s/disk-%d", clone.ID, disk.Slot)
		}

		if _, err := s.db.Exec(ctx, `
			INSERT INTO plugin_addon_kvm_disks
				(id, vm_id, pool_id, pool_backend, pool_target, slot, bus, volume_path,
				 format, size_bytes, backing_path, cache_mode, discard, readonly,
				 is_boot, is_cdrom, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			copied.ID, copied.VMID, copied.PoolID, copied.PoolBackend, copied.PoolTarget,
			copied.Slot, copied.Bus, copied.VolumePath, copied.Format, copied.SizeBytes,
			nullIfEmpty(copied.BackingPath), copied.CacheMode, boolToInt(copied.Discard),
			boolToInt(copied.ReadOnly), boolToInt(copied.IsBoot), boolToInt(copied.IsCDROM),
			now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
			return VM{}, fmt.Errorf("recording clone disk: %w", err)
		}
		clone.Disks = append(clone.Disks, copied)
	}

	return clone, nil
}

// MakeTemplate marks a stopped guest as a template.
//
// Templates are immutable: starting one is refused, so it cannot drift from
// what its clones were made from.
func (s *Service) MakeTemplate(ctx context.Context, id string, isTemplate bool) error {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if isTemplate && vm.Running() {
		return ErrRunning
	}
	_, err = s.db.Exec(ctx,
		`UPDATE plugin_addon_kvm_vms SET is_template = ?, updated_at = ? WHERE id = ?`,
		boolToInt(isTemplate), time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// AddDevice passes a host device through.
func (s *Service) AddDevice(ctx context.Context, vmID string, req DeviceRequest) (Device, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return Device{}, err
	}
	if vm.Running() {
		// Hotplugging a passthrough device is possible on some guests and not
		// others; requiring a stopped guest is the honest, uniform rule.
		return Device{}, ErrRunning
	}
	if err := validateDeviceAddress(req.Kind, req.HostAddress); err != nil {
		return Device{}, err
	}

	now := time.Now().UTC()
	device := Device{
		ID: xid.New().String(), VMID: vmID, Kind: req.Kind,
		HostAddress: req.HostAddress, Description: req.Description,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_devices
			(id, vm_id, kind, host_address, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		device.ID, device.VMID, device.Kind, device.HostAddress,
		nullIfEmpty(device.Description),
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return Device{}, fmt.Errorf("recording device: %w", err)
	}
	return device, nil
}

// DeleteDevice detaches a host device.
func (s *Service) DeleteDevice(ctx context.Context, vmID, deviceID string) error {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return err
	}
	if vm.Running() {
		return ErrRunning
	}
	_, err = s.db.Exec(ctx,
		`DELETE FROM plugin_addon_kvm_devices WHERE id = ? AND vm_id = ?`, deviceID, vmID)
	return err
}

// guestEnvironmentIDs lists the hosts that currently hold guests.
//
// Read from the guest table rather than the host's environment list: a host
// with no VMs has nothing to reconcile, and asking it anyway would put a query
// a minute on every Docker environment in the install.
func (s *Service) guestEnvironmentIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx,
		`SELECT DISTINCT environment_id FROM plugin_addon_kvm_vms
		  WHERE environment_id <> '' LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing guest hosts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]string, 0, 4)
	for rows.Next() {
		var envID string
		if err := rows.Scan(&envID); err != nil {
			return nil, fmt.Errorf("scanning guest host: %w", err)
		}
		out = append(out, envID)
	}
	return out, rows.Err()
}

// runningGuestIDs lists the guests a host currently has running.
//
// Used by the reconcile sweep to decide who is worth asking for an address:
// vm_guest_query against a stopped guest is a round trip that can only fail.
func (s *Service) runningGuestIDs(ctx context.Context, envID string) ([]string, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id FROM plugin_addon_kvm_vms
		  WHERE environment_id = ? AND state = ? LIMIT ?`, envID, StateRunning, maxRows)
	if err != nil {
		return nil, fmt.Errorf("listing running guests: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]string, 0, 8)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning running guest: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Reconcile asks a host what is actually running and records the answer.
//
// Units the backend has no row for come back as orphans. They are reported, not
// killed: the operator may have a very good reason for them, and an agent that
// stops processes it does not recognise is not one anybody should run as root.
func (s *Service) Reconcile(ctx context.Context, envID string) ([]string, error) {
	guests, err := s.List(ctx, envID, false)
	if err != nil {
		return nil, err
	}

	states := make([]agentState, 0, len(guests))
	for _, vm := range guests {
		states = append(states, agentState{VMID: vm.ID})
	}

	reply, err := s.call(ctx, envID, msgVMQuery, agentPayload{States: states})
	if err != nil {
		return nil, err
	}

	for _, state := range reply.States {
		// The host's own start time, which outlives every restart on both sides.
		startedAt, _ := time.Parse(time.RFC3339, state.StartedAt)
		if err := s.setStateAt(ctx, state.VMID, state.State, state.LastError, startedAt); err != nil {
			s.logger.Warn("recording reconciled state", "vm", state.VMID, "error", err)
		}
		// Written separately from the mapped state so every other caller of
		// setStateAt keeps its signature: only reconciliation has QEMU's own
		// run state to record, and only it can clear a stale one.
		if err := s.setRunState(ctx, state.VMID, state.RunState); err != nil {
			s.logger.Warn("recording guest run state", "vm", state.VMID, "error", err)
		}
	}
	return reply.Orphans, nil
}

// RefreshGuestInfo asks the guest agent what the guest thinks it is.
//
// Everything it returns is display-only and untrusted. It is stored so the list
// can show an address without a round trip, and rendered as text everywhere.
func (s *Service) RefreshGuestInfo(ctx context.Context, id string) error {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !vm.Running() {
		return ErrNotRunning
	}

	reply, err := s.call(ctx, vm.EnvironmentID, msgVMGuestQuery, agentPayload{
		VMID: id, GuestQuery: "network",
	})
	if err != nil {
		return err
	}
	if reply.Guest == nil {
		return nil
	}

	payload, err := json.Marshal(reply.Guest.IPs)
	if err != nil {
		return nil
	}
	_, err = s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_vms
		   SET guest_agent_state = ?, guest_reported_ips = ?, updated_at = ?
		 WHERE id = ?`,
		reply.Guest.AgentState, string(payload),
		time.Now().UTC().Format(time.RFC3339), id)
	return err
}
