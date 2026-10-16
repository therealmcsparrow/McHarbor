// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package capability answers "what can this host actually run?".
//
// This is the first screen an operator sees when they turn compute on, and the
// standard it has to hit is set in the roadmap: "`/dev/kvm` missing — your CPU
// reports VT-x but it is disabled in firmware" is the target, not "capability
// check failed".
//
// So every check produces an Item carrying three things: whether it is present,
// what was actually observed, and — when it is absent — the exact command that
// fixes it on the distro this host is running. A check that cannot say how to
// fix itself is a check that leaves the operator stuck, which is the failure
// mode this whole package exists to avoid.
//
// The report is also honest about scope. A host missing `/dev/kvm` can still
// run containers, and a host on cgroup v1 can still run VMs, so items name what
// they block rather than collapsing into one pass/fail.
package capability

// Item keys. These are stable identifiers the frontend maps to translated
// labels — the Detail and Remediation strings are host-specific and travel as
// text, but the key is what the UI switches on.
const (
	ItemKVMDevice      = "kvm_device"
	ItemCPUVirt        = "cpu_virt"
	ItemQEMU           = "qemu"
	ItemQEMUImg        = "qemu_img"
	ItemOVMF           = "ovmf"
	ItemOVMFSecure     = "ovmf_secboot"
	ItemSwtpm          = "swtpm"
	ItemCgroupV2       = "cgroup2"
	ItemUserns         = "userns"
	ItemSubuid         = "subuid"
	ItemIdmappedMounts = "idmapped_mounts"
	ItemSystemd        = "systemd"
	ItemDBus           = "dbus"
	ItemSeccomp        = "seccomp"
	ItemIOMMU          = "iommu"
	ItemBridge         = "bridge"
	ItemHostNetwork    = "host_network"
)

// What an item blocks when it is absent.
const (
	BlocksVM   = "vm"
	BlocksCT   = "ct"
	BlocksBoth = "both"
	BlocksNone = ""
)

// Item is one line of the dependency-floor checklist.
type Item struct {
	Key         string
	Present     bool
	Detail      string
	Remediation string
	// Critical marks an item whose absence blocks its capability outright, so
	// the UI can separate "you cannot do this at all" from "this option is
	// unavailable".
	Critical bool
	Blocks   string
	// Installable marks a missing item the agent can install itself: it knows the packages
	// for this distribution, runs as root and has apt-get or dnf.
	Installable bool
}

// QEMUBinary is one qemu-system-* found on PATH.
type QEMUBinary struct {
	Path         string
	Arch         string
	Version      string
	MachineTypes []string
	KVMAccel     bool

	// Devices, CPUModels and QMPCommands are the capability gate: every VM
	// feature McHarbor offers is a device to emit or a QMP verb to call, and a
	// host whose QEMU has neither should show the feature disabled rather than
	// fail at use time with QEMU's own wording.
	Devices     []string
	CPUModels   []string
	QMPCommands []string
}

// HugePages is the host's hugepage pool, read from /proc/meminfo.
//
// McHarbor never allocates hugepages on the operator's behalf — reserving them
// takes memory away from everything else on the box — so this is reported to
// gate the per-VM toggle, not to act on.
type HugePages struct {
	Total    int64
	Free     int64
	SizeKB   int64
	Surplus  int64
	Reserved int64
}

// NUMANode is one host NUMA node and what it has free right now.
//
// FreeBytes is what a pinning pre-flight checks: a guest pinned to a node with
// no memory left fails to start with an error that names neither the node nor
// the reason.
type NUMANode struct {
	ID          int
	CPUs        []int
	MemoryBytes int64
	FreeBytes   int64
}

// IDRange is one /etc/subuid or /etc/subgid entry.
type IDRange struct {
	Start int64
	Count int64
}

// Report is the whole answer for one host.
type Report struct {
	OS             string
	Arch           string
	KernelVersion  string
	Supported      bool
	UnsupportedWhy string

	CanRunVMs        bool
	CanRunContainers bool
	SeccompBuiltIn   bool
	AgentVersion     string
	SupervisorMode   string

	// HostCPUs and HostMemoryBytes are what the host physically has. The UI
	// bounds a guest's processor and memory controls with them: a guest wider
	// than its host gains nothing, and one sized to every byte of RAM leaves
	// none for the host or for QEMU's own overhead.
	HostCPUs        int
	HostMemoryBytes int64

	// HostSockets and HostPhysicalCores describe the topology behind HostCPUs,
	// which counts threads. Overcommit ratios are judged against real cores
	// because SMT adds roughly a third more throughput, not a second core.
	HostSockets       int
	HostPhysicalCores int

	// HugePages and NUMANodes are absent on a host that has neither, which is
	// the ordinary case; the UI disables the matching VM toggles rather than
	// offering a setting whose start would fail.
	HugePages *HugePages
	NUMANodes []NUMANode

	Items        []Item
	QEMUBinaries []QEMUBinary
	SubuidRanges []IDRange
	SubgidRanges []IDRange
}

// Item returns a check by key.
func (r *Report) Item(key string) (Item, bool) {
	for _, item := range r.Items {
		if item.Key == key {
			return item, true
		}
	}
	return Item{}, false
}
