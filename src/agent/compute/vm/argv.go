// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package vm turns a VM specification into a running QEMU process and back.
//
// The argv this package builds is the entire device model. There is no domain
// XML, no libvirt translation layer, and no template file an operator can
// half-edit — the definition in McHarbor's database is the only source of
// truth, and `args.json` in the runtime directory records exactly what was
// executed so support can see the real picture without being able to change it.
package vm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
)

// DefaultRuntimeRoot is where per-VM sockets and state live when McHarbor
// names no location of its own.
const DefaultRuntimeRoot = "/var/lib/mcharbor/compute/vm"

// VhostEnvVar names the environment variable that turns vhost-net off.
const VhostEnvVar = "MCHARBOR_VM_VHOST"

// vhostOption returns the netdev option that selects the RX/TX path.
//
// vhost-net is on by default and is what makes virtio-net perform: the kernel
// moves frames between the tap socket and the guest's virtqueues without waking
// QEMU. Setting MCHARBOR_VM_VHOST=off puts QEMU back in that path, which is
// slower and is the diagnostic for a host where inbound frames are dropped at
// the tap before they reach the guest — the two paths size the virtio header
// differently, so a disagreement breaks one and not the other.
func vhostOption() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(VhostEnvVar))) {
	case "off", "0", "false", "no":
		return "vhost=off"
	default:
		return "vhost=on"
	}
}

// runtimeRoot holds the live value behind RuntimeRoot. It is read on every
// PathsFor call from whichever goroutine is driving a guest, so it is atomic
// rather than a plain package variable.
var runtimeRoot atomic.Value

func init() { runtimeRoot.Store(DefaultRuntimeRoot) }

// RuntimeRoot reports where per-VM sockets and state currently live.
func RuntimeRoot() string {
	root, _ := runtimeRoot.Load().(string)
	if root == "" {
		return DefaultRuntimeRoot
	}
	return root
}

// SetRuntimeRoot moves where per-VM state is kept, so an operator can put it on
// shared storage. Callers validate the path first; this only publishes it.
func SetRuntimeRoot(root string) {
	if root == "" {
		root = DefaultRuntimeRoot
	}
	runtimeRoot.Store(root)
}

// TPMRoot is where each guest's software TPM keeps its socket and state.
//
// It is not under RuntimeRoot, and that is not a style choice: Debian and
// Ubuntu ship an AppArmor profile for /usr/bin/swtpm that permits
// `owner /var/lib/swtpm/**` and a handful of libvirt paths, and nothing else.
// A socket anywhere else is denied with mknod EACCES no matter that swtpm runs
// as root. Living inside the path the distribution already sanctions means an
// operator never has to relax a security policy to run a guest with a TPM.
const TPMRoot = "/var/lib/swtpm/mcharbor"

// Guest OS families, which decide display adapter and driver defaults.
const (
	OSFamilyLinux   = "linux"
	OSFamilyWindows = "windows"
	OSFamilyBSD     = "bsd"
	OSFamilyOther   = "other"
)

// Spec is one guest, mirroring the protocol's VMSpec.
//
// It is a separate type rather than the protocol struct so this package can be
// tested and reasoned about without the transport, and so a protocol change is
// a deliberate translation rather than an implicit one.
type Spec struct {
	ID             string
	Name           string
	UUID           string
	GuestOSFamily  string
	GuestOSVariant string

	VCPUs       int
	CPUModel    string
	CPUSockets  int
	CPUCores    int
	CPUThreads  int
	MemoryMB    int64
	MemoryMaxMB int64

	// VCPUsMax is the hotplug ceiling. It is fixed at boot — raising it needs a
	// restart — so a guest that may ever grow has to be started with headroom
	// it is not using.
	VCPUsMax int

	// VirtioMem replaces the balloon with a resizable memory device. The two
	// overlap confusingly and QEMU documents them as fighting, so exactly one
	// is emitted per guest.
	VirtioMem bool
	// HugePages backs guest RAM with a memory-backend-file on /dev/hugepages.
	// Not swappable, so an over-committed host with hugepage guests OOMs
	// rather than degrading — the caller checks the pool before setting it.
	HugePages bool
	// NUMAPolicy is "" / none, "pin" or "mirror". A policy rather than a
	// topology: hand-building a NUMA layout is not something a management UI
	// should ask an operator to do.
	NUMAPolicy string
	// NUMAHostNodes are the host nodes the policy resolved to, chosen by the
	// caller because only it can see what is free.
	NUMAHostNodes []int
	// VSockCID is the guest's AF_VSOCK context id. 0, 1 and 2 are reserved, so
	// any value below 3 means "no vsock".
	VSockCID int
	// IOThread moves block I/O off QEMU's main loop, which is what stops one
	// guest's disk pressure stalling the whole process. One per guest — per
	// disk is a tuning exercise nobody asked for.
	IOThread bool

	MachineType string
	Firmware    string // uefi | bios
	SecureBoot  bool
	TPMEnabled  bool
	DisplayType string
	// DisplayWidth and DisplayHeight are the boot resolution virtio-vga and
	// qxl-vga advertise; zero on either leaves QEMU's own default.
	DisplayWidth  int
	DisplayHeight int

	Disks   []Disk
	NICs    []NIC
	Devices []Device
	// Shares are virtiofs directories. Each one is a separate virtiofsd
	// process the agent starts before QEMU and stops with it, so the list
	// changes how the guest is launched rather than only what it sees.
	Shares []Share

	ISOPaths []string

	// FirmwareCode and FirmwareVars are resolved by the caller from the
	// capability probe, so the argv builder never guesses at a path that
	// differs per distribution.
	FirmwareCode string
	// FirmwareVarsTemplate is the read-only template the guest's own variable
	// store is seeded from. A Secure Boot guest needs one with Microsoft's keys
	// already enrolled; an empty store means setup mode, which Windows 11
	// treats as Secure Boot being off.
	FirmwareVarsTemplate string
	FirmwareVars         string

	// IncomingURI starts the guest in migration-incoming mode, loading a saved
	// machine state instead of booting. It is how a memory-state snapshot is
	// restored, and it is never set from the protocol — only by the snapshot
	// path, which knows the file it just wrote.
	//
	// A guest started this way is paused when the load completes; the caller
	// resumes it.
	IncomingURI string

	// SPICE adds a SPICE server on a unix socket beside the VNC one. Set by the
	// agent from its own probe, never from the protocol: a QEMU built without
	// SPICE refuses to start at all when asked for it.
	SPICE bool
	// SpiceOptions tune that server, from the guest's display profile. Only
	// read when SPICE is on; an empty or unknown value keeps QEMU's default.
	SpiceOptions SpiceOptions

	// LegacyLayout builds the guest without its PCIe hot-plug ports. Only a
	// live-migration target sets it, for a source that was booted before the
	// ports existed: the two device models must match or the state is refused.
	LegacyLayout bool
}

// Disk is one block device.
type Disk struct {
	ID          string
	Slot        int
	Bus         string
	Path        string
	Format      string
	IsBlock     bool
	ReadOnly    bool
	IsBoot      bool
	IsCDROM     bool
	Discard     bool
	CacheMode   string
	SizeBytes   int64
	BackingPath string

	// BootIndex, when set, places this device explicitly in the boot order.
	// Zero means "derive it from IsBoot and IsCDROM as before", which is what
	// every guest created before boot order was selectable still relies on.
	BootIndex int
	// ShareRW lets a second guest open this disk at the same time. Required
	// for a cluster filesystem and catastrophic without one, which is why it
	// is off unless the operator confirmed the risk by name.
	ShareRW bool

	// I/O limits, zero meaning unlimited. Separate read and write because a
	// guest running a restore saturates writes while reading almost nothing,
	// and one number for both would throttle the wrong half.
	ReadIOPS  int64
	WriteIOPS int64
	ReadBPS   int64
	WriteBPS  int64
}

// throttleOpts renders a disk's I/O limits as device properties.
//
// Emitted on the device rather than as a throttle-group because a group is
// shared state between disks and these limits are per disk; the live change
// path uses block_set_io_throttle on the same device id.
func (d Disk) throttleOpts() string {
	var opts []string
	for _, limit := range []struct {
		name  string
		value int64
	}{
		{"throttling.iops-read", d.ReadIOPS},
		{"throttling.iops-write", d.WriteIOPS},
		{"throttling.bps-read", d.ReadBPS},
		{"throttling.bps-write", d.WriteBPS},
	} {
		if limit.value > 0 {
			opts = append(opts, fmt.Sprintf("%s=%d", limit.name, limit.value))
		}
	}
	if len(opts) == 0 {
		return ""
	}
	return "," + strings.Join(opts, ",")
}

// bootOpts renders a disk's place in the boot order.
//
// An explicit BootIndex wins; otherwise the old derivation stands, so guests
// created before the boot order was selectable keep booting from the same
// device. bootindex and `-boot order=` must never be mixed, and McHarbor emits
// only `-boot menu=on`.
func (d Disk) bootOpts() string {
	// Negative opts out of the boot order entirely: the cloud-init seed is a
	// CD-ROM that must never be booted from.
	if d.BootIndex < 0 {
		return ""
	}
	if d.BootIndex > 0 {
		return fmt.Sprintf(",bootindex=%d", d.BootIndex)
	}
	if d.IsBoot {
		return ",bootindex=0"
	}
	if d.IsCDROM {
		// An installer ISO boots after the disks, so a reinstall does not
		// happen every time the guest restarts with the ISO still attached.
		return ",bootindex=1"
	}
	return ""
}

// NIC is one network interface.
type NIC struct {
	ID      string
	Slot    int
	Model   string
	MAC     string
	TapName string
	// PCIAddr pins the NIC to the slot it was first given, as 0x04. Empty on a
	// NIC McHarbor has not observed yet, and QEMU then allocates as before.
	//
	// Without it the slot follows command-line order, so attaching a disc adds
	// an AHCI controller ahead of the NIC, the guest renames its interface and
	// loses its address.
	PCIAddr string
	// DevicePath is the macvtap character device for a NIC on a direct
	// network. Empty on a bridged NIC, which QEMU opens by interface name.
	DevicePath string
	// FD is the descriptor DevicePath is placed on before QEMU is executed.
	// Meaningless without DevicePath.
	FD int
	// LinkDown cuts the NIC's link before the vCPUs are released, so a link the
	// operator took down stays down across a restart.
	LinkDown bool
}

// Device is a passed-through host device.
type Device struct {
	ID          string
	Kind        string // pci | usb | block
	HostAddress string
}

// Share is one virtiofs directory handed to the guest.
//
// Tag is what the guest mounts by (`mount -t virtiofs <tag> /mnt`), and it is
// unique per guest rather than per host. HostPath is validated against the
// operator's configured roots before it ever reaches here: a share of / or
// /etc hands the host to the guest.
type Share struct {
	ID       string
	Tag      string
	HostPath string
	ReadOnly bool
	// Socket is where this share's virtiofsd listens. Assigned by the caller
	// under the guest's runtime directory, so it is torn down with the guest.
	Socket string
}

// Paths are the per-VM runtime files.
type Paths struct {
	Dir        string
	QMPSocket  string
	VNCSocket  string
	SerialSock string
	GuestAgent string
	// SPICESocket exists only for a guest started with SPICE. Every SPICE
	// channel is its own connection to it, which is why the console gateway
	// opens one stream per channel rather than multiplexing.
	SPICESocket string
	VarsFile    string
	ArgsFile    string
	// SeedISO is the cloud-init seed written at define; SeedMarker records that
	// the guest has booted with it, after which it is no longer attached.
	SeedISO    string
	SeedMarker string
	PIDFile    string
	LockFile   string
	// TPMDir, TPMState and TPMSocket live under TPMRoot rather than beside the
	// guest's other sockets, because swtpm is AppArmor-confined on Debian and
	// Ubuntu and may not create a socket in an arbitrary directory.
	TPMDir    string
	TPMState  string
	TPMSocket string
}

// PathsFor builds the runtime paths for a VM.
func PathsFor(vmID string) Paths {
	dir := filepath.Join(RuntimeRoot(), vmID)
	return Paths{
		Dir:         dir,
		QMPSocket:   filepath.Join(dir, "qmp.sock"),
		VNCSocket:   filepath.Join(dir, "vnc.sock"),
		SerialSock:  filepath.Join(dir, "serial.sock"),
		GuestAgent:  filepath.Join(dir, "qga.sock"),
		SPICESocket: filepath.Join(dir, "spice.sock"),
		VarsFile:    filepath.Join(dir, "OVMF_VARS.fd"),
		ArgsFile:    filepath.Join(dir, "args.json"),
		SeedISO:     filepath.Join(dir, "seed.iso"),
		SeedMarker:  filepath.Join(dir, "seed.delivered"),
		PIDFile:     filepath.Join(dir, "qemu.pid"),
		LockFile:    filepath.Join(dir, "owner.lock"),
		TPMDir:      filepath.Join(TPMRoot, vmID),
		TPMState:    filepath.Join(TPMRoot, vmID, "state"),
		TPMSocket:   filepath.Join(TPMRoot, vmID, "swtpm.sock"),
	}
}

// BuildArgv assembles the QEMU command line.
//
// Every value that could contain a user-supplied string is validated before it
// reaches here or escaped as it goes in. QEMU's `-drive`/`-device` syntax is
// comma-separated with `,,` as the escape, so a path containing a comma would
// otherwise silently become two options.
func BuildArgv(binary string, spec Spec, paths Paths) ([]string, error) {
	if binary == "" {
		return nil, fmt.Errorf("vm: no qemu binary for %s", spec.Name)
	}
	if spec.MachineType == "" {
		return nil, fmt.Errorf("vm: %s has no pinned machine type", spec.Name)
	}
	if spec.VCPUs <= 0 || spec.MemoryMB <= 0 {
		return nil, fmt.Errorf("vm: %s needs at least one vcpu and some memory", spec.Name)
	}

	argv := []string{binary}

	// -nodefaults removes QEMU's implicit devices — a parallel port, a second
	// serial, a floppy controller. Without it the guest gets hardware nobody
	// asked for and the device model stops matching what the UI shows.
	argv = append(argv, "-nodefaults", "-no-user-config")

	// The name is what shows up in `ps` and in the window title. It is passed
	// as one argument, so a guest name with a space stays one name.
	argv = append(argv, "-name", "guest="+escapeOptValue(spec.Name)+",debug-threads=on")
	argv = append(argv, "-uuid", spec.UUID)

	// accel=kvm with no TCG fallback: an emulated guest is slow enough to look
	// broken, and offering it would generate support load for a mode nobody
	// wants. QEMU refuses to start rather than silently degrading.
	machine := spec.MachineType + ",accel=kvm"
	if spec.Firmware == "uefi" && spec.SecureBoot {
		// Secure Boot needs SMM, and the firmware must be the secboot build.
		machine += ",smm=on"
	}
	// A guest with an explicit memory backend has to have the machine pointed
	// at it, on this same option — a second -machine would not merge reliably
	// across QEMU versions. Without it QEMU allocates anonymous memory beside
	// the backend and the hugepages, NUMA binding and share=on it carries are
	// all silently ignored.
	if usesSingleMemoryBackend(spec) {
		machine += ",memory-backend=" + memoryBackendID
	}
	argv = append(argv, "-machine", machine)

	argv = append(argv, cpuArgs(spec)...)
	argv = append(argv, memoryArgs(spec)...)
	argv = append(argv, ioThreadArgs(spec)...)
	argv = append(argv, vsockArgs(spec)...)
	argv = append(argv, shareArgs(spec)...)

	firmware, err := firmwareArgs(spec, paths)
	if err != nil {
		return nil, err
	}
	argv = append(argv, firmware...)

	// Sockets rather than TCP ports for every control channel. A TCP QMP or
	// VNC port is reachable from the LAN by anyone who finds it; a unix socket
	// under a 0700 directory is reachable only by the agent, and the console
	// gateway is what mediates browser access.
	argv = append(argv, "-qmp", "unix:"+paths.QMPSocket+",server=on,wait=off")
	argv = append(argv, "-pidfile", paths.PIDFile)

	// Restoring a memory-state snapshot. QEMU sets the machine up, loads the
	// saved state from the stream and stops, so the guest resumes exactly where
	// it was rather than booting.
	if spec.IncomingURI != "" {
		argv = append(argv, "-incoming", spec.IncomingURI)
	}

	// QEMU stays in the foreground: systemd's Type=exec supervises the process
	// directly, so daemonizing would hand it a pid that exits immediately and
	// leave the real guest unsupervised.

	// The hot-plug ports go before every auto-addressed device. They take
	// fixed addresses high on pcie.0, so nothing below them moves.
	layout := usesHotplugLayout(spec)
	if layout {
		argv = append(argv, rootPortArgs()...)
	}

	argv = append(argv, displayArgs(spec, paths)...)
	argv = append(argv, serialArgs(paths)...)
	argv = append(argv, guestAgentArgs(paths)...)
	// After the guest agent's arguments, because the vdagent port sits on the
	// virtio-serial controller that list creates.
	argv = append(argv, spiceArgs(spec, paths)...)

	diskArgs, err := buildDiskArgs(spec, layout)
	if err != nil {
		return nil, err
	}
	argv = append(argv, diskArgs...)

	nicArgs, err := buildNICArgs(spec, layout)
	if err != nil {
		return nil, err
	}
	argv = append(argv, nicArgs...)

	deviceArgs, err := buildDeviceArgs(spec)
	if err != nil {
		return nil, err
	}
	argv = append(argv, deviceArgs...)

	if spec.TPMEnabled {
		argv = append(argv, tpmArgs(paths)...)
	}

	// A balloon device is what makes memory reclaim reporting possible, and an
	// entropy source is what stops a guest stalling at boot waiting for the
	// kernel's random pool.
	argv = append(argv, "-device", "virtio-balloon-pci,id=balloon0")
	argv = append(argv, "-object", "rng-random,id=rng0,filename=/dev/urandom")
	argv = append(argv, "-device", "virtio-rng-pci,rng=rng0,id=rng-dev0")

	// A USB controller, because q35 has no USB bus of its own. Without one
	// QEMU refuses to start at all — "No 'usb-bus' bus found for device
	// 'usb-tablet'" — so the tablet below, and any passed-through usb-host
	// device, need this line to exist before them.
	argv = append(argv, "-device", "qemu-xhci,id=usb0")

	// An absolute-position pointer. Without it VNC mouse tracking is relative
	// and unusable — the pointer drifts away from the cursor and never comes
	// back, which reads as a broken console.
	argv = append(argv, "-device", "usb-tablet,id=tablet0,bus=usb0.0")

	// A boot menu, and a window in which to reach it.
	//
	// Without this there is no way into the firmware from the console at all:
	// the guest goes straight to whatever bootindex says, and an operator who
	// needs to change boot order, enable a setting or pick a one-off device has
	// nowhere to do it. The splash time is what makes the menu reachable in
	// practice — the key has to be pressed while the firmware is watching, and
	// on a remote console the first frames are often still arriving.
	argv = append(argv, "-boot", "menu=on,splash-time=3000")

	// The guest starts paused so the caller can attach the QMP monitor,
	// confirm the device model came up, and only then release the vCPUs. It
	// also gives the console gateway a chance to be ready before the firmware
	// paints its first frame.
	argv = append(argv, "-S")

	return argv, nil
}

// cpuArgs builds the CPU model and topology.
func cpuArgs(spec Spec) []string {
	model := spec.CPUModel
	if model == "" {
		// `host` passes the physical CPU's full instruction set through, which
		// is the right default for a guest that will not be migrated. A VM
		// intended for migration takes a named baseline instead, and the UI
		// explains the trade at the point the operator chooses.
		model = "host"
	}

	sockets := spec.CPUSockets
	cores := spec.CPUCores
	threads := spec.CPUThreads
	if sockets <= 0 {
		sockets = 1
	}
	if threads <= 0 {
		threads = 1
	}
	if cores <= 0 {
		cores = spec.VCPUs / (sockets * threads)
		if cores <= 0 {
			cores = 1
		}
	}

	// maxcpus reserves the slots device_add later fills. It cannot be raised on
	// a running guest, so a VM that may ever grow has to be started with the
	// headroom — which is why the UI distinguishes "change now" from "change
	// at next boot".
	topology := fmt.Sprintf("%d,sockets=%d,cores=%d,threads=%d",
		spec.VCPUs, sockets, cores, threads)
	if spec.VCPUsMax > spec.VCPUs {
		topology = fmt.Sprintf("%d,maxcpus=%d,sockets=%d,cores=%d,threads=%d",
			spec.VCPUs, spec.VCPUsMax, sockets, cores, threads)
	}

	return []string{"-cpu", model, "-smp", topology}
}

// hugePagePath is where a Linux host mounts its hugetlbfs by default.
const hugePagePath = "/dev/hugepages"

// usesSingleMemoryBackend reports whether this guest gets one named backend for
// all of its RAM.
//
// The mirror policy is excluded because it builds one backend per guest node
// instead, and a machine-wide backend beside those would double-allocate.
func usesSingleMemoryBackend(spec Spec) bool {
	if spec.NUMAPolicy == NUMAPolicyMirror {
		return false
	}
	return spec.HugePages || spec.VirtioMem ||
		spec.NUMAPolicy == NUMAPolicyPin || spec.needsSharedMemory()
}

// memoryBackendID is the id every guest's main RAM backend takes. One per
// guest, so a fixed id is clearer than a generated one.
const memoryBackendID = "mem0"

// memoryBackendArgs builds an explicit memory backend when the guest needs one.
//
// A plain `-m` is enough until something wants to name the memory: hugepages
// need a file backend, NUMA needs one per node, and vhost-user — which virtiofs
// is — needs `share=on` or the daemon cannot map the guest's RAM at all. So the
// backend is emitted whenever any of those is in play and omitted otherwise,
// because an explicit backend changes how QEMU allocates and there is no reason
// to change that for a guest which asked for none of it.
func memoryBackendArgs(spec Spec) []string {
	if !usesSingleMemoryBackend(spec) {
		return nil
	}

	kind := "memory-backend-ram"
	options := []string{
		"id=" + memoryBackendID,
		fmt.Sprintf("size=%dM", spec.MemoryMB),
	}
	if spec.HugePages {
		kind = "memory-backend-file"
		options = append(options, "mem-path="+hugePagePath, "prealloc=on")
	}
	// share=on is what lets a vhost-user daemon map the guest's memory. It is
	// also what VM templating needs, so it is set for every explicit backend
	// rather than only for virtiofs — the cost is nil for a guest that has no
	// second process looking at its RAM.
	options = append(options, "share=on")

	args := []string{"-object", kind + "," + strings.Join(options, ",")}
	if len(spec.NUMAHostNodes) > 0 && spec.NUMAPolicy == NUMAPolicyPin {
		// bind, not preferred: preferred silently falls back to another node
		// under pressure, which produces exactly the unpredictable performance
		// the policy was chosen to remove.
		args[1] += "," + numaBindOptions(spec.NUMAHostNodes)
	}
	return args
}

// NUMA policies. Three, because most operators should be choosing an intent
// rather than laying out a topology.
const (
	NUMAPolicyNone   = "none"
	NUMAPolicyPin    = "pin"
	NUMAPolicyMirror = "mirror"
)

// numaBindOptions renders a host-nodes binding.
func numaBindOptions(nodes []int) string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, strconv.Itoa(node))
	}
	return "host-nodes=" + strings.Join(ids, ",") + ",policy=bind"
}

// needsSharedMemory reports whether a second process has to map this guest's
// RAM, which today means a virtiofs daemon.
func (s Spec) needsSharedMemory() bool { return len(s.Shares) > 0 }

// numaArgs presents a NUMA topology to the guest.
//
// Only the mirror policy builds one. Pinning a guest that fits inside a single
// host node is done through its memory backend and thread affinity, and giving
// such a guest a one-node topology it would have had anyway adds nothing.
func numaArgs(spec Spec) []string {
	if spec.NUMAPolicy != NUMAPolicyMirror || len(spec.NUMAHostNodes) < 2 {
		return nil
	}

	var args []string
	perNode := spec.MemoryMB / int64(len(spec.NUMAHostNodes))
	cpusPerNode := spec.VCPUs / len(spec.NUMAHostNodes)
	if cpusPerNode < 1 {
		cpusPerNode = 1
	}
	for i, hostNode := range spec.NUMAHostNodes {
		id := fmt.Sprintf("numamem%d", i)
		args = append(args, "-object", fmt.Sprintf(
			"memory-backend-ram,id=%s,size=%dM,%s",
			id, perNode, numaBindOptions([]int{hostNode})))

		first := i * cpusPerNode
		last := first + cpusPerNode - 1
		if i == len(spec.NUMAHostNodes)-1 {
			// The last node absorbs the remainder, so a vCPU count that does
			// not divide evenly still has every vCPU in exactly one node —
			// an unassigned vCPU is a guest QEMU refuses to start.
			last = spec.VCPUs - 1
		}
		args = append(args, "-numa", fmt.Sprintf("node,nodeid=%d,memdev=%s,cpus=%d-%d",
			i, id, first, last))
	}
	return args
}

// virtioMemArgs adds resizable memory.
//
// requested-size starts at zero: the guest boots with its `-m` size and grows
// from there, so a resize that is never asked for costs nothing. The host's
// block size rounds requests, which is why the caller reports back what was
// actually plugged rather than what was typed.
func virtioMemArgs(spec Spec) []string {
	if !spec.VirtioMem || spec.MemoryMaxMB <= spec.MemoryMB {
		return nil
	}
	return []string{
		"-object", fmt.Sprintf("memory-backend-ram,id=vmem0,size=%dM,share=on",
			spec.MemoryMaxMB-spec.MemoryMB),
		"-device", "virtio-mem-pci,id=virtiomem0,memdev=vmem0,requested-size=0",
	}
}

// vsockArgs gives host and guest a socket channel with no network involved.
//
// CIDs 0, 1 and 2 are reserved by the kernel, so anything below 3 means the
// guest has none. Allocation happens against a database constraint rather than
// here, because a collision is silent and lands on the wrong guest.
func vsockArgs(spec Spec) []string {
	if spec.VSockCID < 3 {
		return nil
	}
	return []string{"-device", fmt.Sprintf("vhost-vsock-pci,id=vsock0,guest-cid=%d", spec.VSockCID)}
}

// shareArgs wires each virtiofs daemon into the guest.
//
// The daemon is a separate process per share, already listening on Socket by
// the time this runs — QEMU refuses to start against a vhost-user socket that
// nobody is on the other end of.
func shareArgs(spec Spec) []string {
	var args []string
	for i, share := range spec.Shares {
		id := fmt.Sprintf("fs%d", i)
		args = append(args,
			"-chardev", fmt.Sprintf("socket,id=%s,path=%s", id, escapeOptValue(share.Socket)),
			"-device", fmt.Sprintf("vhost-user-fs-pci,chardev=%s,tag=%s",
				id, escapeOptValue(share.Tag)))
	}
	return args
}

// ioThreadID is the single I/O thread each guest gets.
const ioThreadID = "iothread0"

// ioThreadArgs moves block I/O off the main loop.
func ioThreadArgs(spec Spec) []string {
	if !spec.IOThread {
		return nil
	}
	return []string{"-object", "iothread,id=" + ioThreadID}
}

// memoryArgs builds the memory configuration, including the hotplug headroom a
// balloon ceiling needs.
func memoryArgs(spec Spec) []string {
	args := memoryBackendArgs(spec)

	size := strconv.FormatInt(spec.MemoryMB, 10) + "M"
	if spec.MemoryMaxMB > spec.MemoryMB {
		// maxmem reserves address space for later growth. slots is how many
		// separate additions are possible; four is enough for any realistic
		// resize sequence and costs nothing when unused.
		size = fmt.Sprintf("%dM,slots=4,maxmem=%dM", spec.MemoryMB, spec.MemoryMaxMB)
	}
	args = append(args, "-m", size)
	args = append(args, numaArgs(spec)...)
	return append(args, virtioMemArgs(spec)...)
}

// firmwareArgs selects UEFI or BIOS.
//
// UEFI needs a per-VM writable copy of the variable store: the shared
// OVMF_VARS.fd template is read-only, and a guest that cannot persist its own
// boot entries loses them on every restart. The caller creates that copy; this
// only wires it up.
func firmwareArgs(spec Spec, paths Paths) ([]string, error) {
	if spec.Firmware != "uefi" {
		// SeaBIOS is QEMU's default, so BIOS guests need nothing.
		return nil, nil
	}
	if spec.FirmwareCode == "" {
		return nil, fmt.Errorf("vm: %s is set to UEFI but no OVMF firmware was found on this host",
			spec.Name)
	}
	varsPath := spec.FirmwareVars
	if varsPath == "" {
		varsPath = paths.VarsFile
	}

	// pflash units in order: unit 0 is the read-only code, unit 1 the writable
	// variables. Reversing them produces a guest that boots once and then
	// cannot.
	return []string{
		"-drive", "if=pflash,format=raw,readonly=on,file=" + escapeOptValue(spec.FirmwareCode),
		"-drive", "if=pflash,format=raw,file=" + escapeOptValue(varsPath),
	}, nil
}

// displayArgs wires the graphical console to a unix socket.
func displayArgs(spec Spec, paths Paths) []string {
	display := spec.DisplayType
	if display == "" {
		display = defaultDisplay(spec.GuestOSFamily)
	}
	if display == "none" {
		return []string{"-display", "none", "-vga", "none"}
	}

	args := []string{
		// The VNC server listens on a unix socket only. The browser reaches it
		// through the console gateway, which enforces the ticket and the
		// permission check; exposing a TCP VNC port would bypass both.
		"-vnc", "unix:" + paths.VNCSocket,
	}
	switch display {
	case "virtio-gpu":
		// `-vga none` is not optional: without it QEMU keeps its implicit
		// default adapter, the guest ends up with two consoles, and VNC
		// attaches to the one nothing is drawing on — which renders as
		// "Display output is not active." virtio-vga rather than
		// virtio-gpu-pci because the latter is not VGA-compatible, so a BIOS
		// guest would have no output at all until its kernel driver loads.
		args = append(args, "-vga", "none", "-device", "virtio-vga,id=video0"+resolutionOpts(spec))
	case "qxl":
		args = append(args, "-vga", "none", "-device", "qxl-vga,id=video0"+resolutionOpts(spec))
	default:
		args = append(args, "-vga", "std")
	}
	return args
}

// resolutionOpts is the boot resolution as device properties. Both sides or
// neither: QEMU takes one alone, but a guest offered 1920 by its default height
// boots into a mode nobody chose.
func resolutionOpts(spec Spec) string {
	if spec.DisplayWidth <= 0 || spec.DisplayHeight <= 0 {
		return ""
	}
	return fmt.Sprintf(",xres=%d,yres=%d", spec.DisplayWidth, spec.DisplayHeight)
}

// defaultDisplay picks a display adapter by guest family.
//
// virtio-gpu needs a driver Windows does not ship, so a Windows guest given one
// boots to a blank console — which reads as a broken VM rather than a missing
// driver. QXL works out of the box there, and SPICE's guest tools resize it.
//
// Every other family gets virtio-vga. It carries the same VGA framebuffer std
// does, so a guest without the driver sees what std gave it, and it is the only
// adapter that follows the console window under VNC (QEMU's SetDesktopSize needs
// a device with ui_info, which std and QXL lack) as well as under SPICE.
func defaultDisplay(family string) string {
	if family == OSFamilyWindows {
		return "qxl"
	}
	return "virtio-gpu"
}

// serialArgs always attaches a serial console.
//
// This is not optional. Serial is the fallback whenever VNC is unavailable or
// the guest's graphics stack is broken, and it is the only console that shows
// kernel output during a failed boot. A VM without one is a VM nobody can
// diagnose.
func serialArgs(paths Paths) []string {
	return []string{
		"-chardev", "socket,id=serial0,path=" + escapeOptValue(paths.SerialSock) + ",server=on,wait=off",
		"-serial", "chardev:serial0",
	}
}

// guestAgentArgs wires the virtio-serial channel the QEMU guest agent uses.
//
// The channel name is fixed by convention: the in-guest agent connects to
// org.qemu.guest_agent.0 and nothing else.
func guestAgentArgs(paths Paths) []string {
	return []string{
		"-device", "virtio-serial-pci,id=vserial0",
		"-chardev", "socket,id=qga0,path=" + escapeOptValue(paths.GuestAgent) + ",server=on,wait=off",
		"-device", "virtserialport,bus=vserial0.0,chardev=qga0,name=org.qemu.guest_agent.0",
	}
}

// spiceArgs adds a SPICE server on a unix socket, with ticketing off.
//
// Ticketing is off because nothing but the agent can reach the socket: the
// browser comes through the console gateway's permission check, and a
// remote-viewer through the agent's password-checking relay. The vdagent port
// is what gives a SPICE client clipboard sharing and resize-to-window.
func spiceArgs(spec Spec, paths Paths) []string {
	if !spec.SPICE || spec.DisplayType == "none" {
		return nil
	}
	return []string{
		"-spice", "unix=on,addr=" + escapeOptValue(paths.SPICESocket) + ",disable-ticketing=on" +
			spiceOptionSuffix(spec.SpiceOptions),
		"-chardev", "spicevmc,id=vdagent0,name=vdagent",
		"-device", "virtserialport,bus=vserial0.0,chardev=vdagent0,name=com.redhat.spice.0",
	}
}

// SpiceOptions are the SPICE server settings a display profile chooses.
type SpiceOptions struct {
	ImageCompression      string
	JPEGWANCompression    string
	ZlibGLZWANCompression string
	StreamingVideo        string
	PlaybackCompression   string
	AgentMouse            string
}

// spiceOptionValues is the closed set QEMU's -spice accepts for each option.
// Anything outside it is dropped rather than passed on, because QEMU refuses
// to start on a value it does not know and the guest would not boot at all.
var spiceOptionValues = []struct {
	key     string
	value   func(SpiceOptions) string
	allowed map[string]bool
}{
	{"image-compression", func(o SpiceOptions) string { return o.ImageCompression },
		map[string]bool{"auto_glz": true, "auto_lz": true, "quic": true, "glz": true, "lz": true, "off": true}},
	{"jpeg-wan-compression", func(o SpiceOptions) string { return o.JPEGWANCompression },
		map[string]bool{"auto": true, "never": true, "always": true}},
	{"zlib-glz-wan-compression", func(o SpiceOptions) string { return o.ZlibGLZWANCompression },
		map[string]bool{"auto": true, "never": true, "always": true}},
	{"streaming-video", func(o SpiceOptions) string { return o.StreamingVideo },
		map[string]bool{"off": true, "all": true, "filter": true}},
	{"playback-compression", func(o SpiceOptions) string { return o.PlaybackCompression },
		map[string]bool{"on": true, "off": true}},
	{"agent-mouse", func(o SpiceOptions) string { return o.AgentMouse },
		map[string]bool{"on": true, "off": true}},
}

// spiceOptionSuffix renders the known options as ",key=value" pairs in a fixed
// order, so the same profile always produces the same argv.
func spiceOptionSuffix(options SpiceOptions) string {
	var b strings.Builder
	for _, option := range spiceOptionValues {
		if value := option.value(options); option.allowed[value] {
			b.WriteString("," + option.key + "=" + value)
		}
	}
	return b.String()
}

// tpmArgs wires an emulated TPM 2.0, which Windows 11 requires. The socket on
// the other end is swtpm's control channel, started by the manager first.
func tpmArgs(paths Paths) []string {
	return []string{
		"-chardev", "socket,id=chrtpm,path=" + escapeOptValue(paths.TPMSocket),
		"-tpmdev", "emulator,id=tpm0,chardev=chrtpm",
		"-device", "tpm-tis,tpmdev=tpm0",
	}
}

// buildDiskArgs turns disks into blockdev and device pairs.
//
// The modern -blockdev form is used rather than the legacy -drive, because it
// separates the file from the format from the guest device, which is what makes
// runtime operations — snapshot, resize, hotplug — addressable by node name.
func buildDiskArgs(spec Spec, layout bool) ([]string, error) {
	var argv []string
	state := &busState{layout: layout}

	for _, disk := range spec.Disks {
		if disk.Path == "" && disk.IsCDROM {
			// An empty drive: a disc ejected while the guest ran leaves its
			// drive behind, because SATA cannot unplug one, and the next disc
			// goes into it.
			deviceOpts, err := diskDeviceOpts("sata", "", disk, state, &argv, false)
			if err != nil {
				return nil, err
			}
			argv = append(argv, "-device", deviceOpts)
			continue
		}
		if disk.Path == "" {
			return nil, fmt.Errorf("vm: disk in slot %d has no path", disk.Slot)
		}
		nodeName := fmt.Sprintf("blk%d", disk.Slot)
		fileNode := nodeName + "-file"

		driver := "file"
		if disk.IsBlock {
			// host_device rather than file for a zvol or an LV: QEMU uses
			// different IO paths and a block device opened as a file loses
			// discard support.
			driver = "host_device"
		}

		fileOpts := []string{
			"driver=" + driver,
			"node-name=" + fileNode,
			"filename=" + escapeOptValue(disk.Path),
		}
		cache := disk.CacheMode
		if cache == "" {
			// cache.direct with no writeback is the safe default: the guest's
			// flushes reach the physical device, so a host power loss does not
			// silently discard writes the guest believes are durable.
			cache = "none"
		}
		if cache == "none" {
			fileOpts = append(fileOpts, "cache.direct=on", "cache.no-flush=off")
		}
		if disk.Discard {
			fileOpts = append(fileOpts, "discard=unmap")
		}
		if disk.ReadOnly || disk.IsCDROM {
			fileOpts = append(fileOpts, "read-only=on")
		}
		if disk.ShareRW {
			// The image is opened without the usual exclusive lock. QEMU's
			// file locking is what normally stops a second guest opening a
			// running guest's disk, and turning it off is only safe when
			// something inside the guests coordinates instead. It belongs on
			// the protocol node, so it must join fileOpts before emission.
			fileOpts = append(fileOpts, "locking=off")
		}
		argv = append(argv, "-blockdev", strings.Join(fileOpts, ","))

		format := disk.Format
		if format == "" {
			format = "qcow2"
		}
		formatOpts := []string{
			"driver=" + format,
			"node-name=" + nodeName,
			"file=" + fileNode,
		}
		if disk.ReadOnly || disk.IsCDROM {
			formatOpts = append(formatOpts, "read-only=on")
		}
		if disk.Discard && format == "qcow2" {
			// detect-zeroes turns a guest's zero writes into holes, so a
			// deleted file inside the guest actually returns capacity to a
			// thin pool instead of merely being marked free.
			formatOpts = append(formatOpts, "discard=unmap", "detect-zeroes=unmap")
		}
		argv = append(argv, "-blockdev", strings.Join(formatOpts, ","))

		bus := disk.Bus
		if disk.IsCDROM {
			// A CD-ROM on a SATA controller, because virtio-blk has no
			// removable-media semantics and Windows cannot boot an installer
			// from a virtio CD without drivers it does not have yet.
			bus = "sata"
		}
		if bus == "" {
			bus = "virtio-blk"
		}

		// A CD-ROM never takes the iothread: it is a read-mostly device on a
		// SATA controller, and binding it would only move the thread's work
		// somewhere it does not help.
		deviceOpts, err := diskDeviceOpts(bus, nodeName, disk, state, &argv,
			spec.IOThread && !disk.IsCDROM)
		if err != nil {
			return nil, err
		}
		argv = append(argv, "-device", deviceOpts)
	}
	return argv, nil
}

// ahciPortsPerController is what ich9-ahci provides. A seventh SATA device
// needs a second controller rather than a port that does not exist.
const ahciPortsPerController = 6

// busState carries what one guest's disks have already added to the command
// line, so each controller appears once and every port is used once.
type busState struct {
	// layout is whether this guest has its hot-plug root ports, which is what
	// puts a data disk behind one instead of on pcie.0.
	layout    bool
	scsiAdded bool
	// ahciPort counts SATA devices placed so far; the controller index and the
	// port within it are both derived from it.
	ahciPort int
}

// diskDeviceOpts builds the guest-visible device for one disk, adding the
// controller it needs on first use.
func diskDeviceOpts(
	bus, nodeName string,
	disk Disk,
	state *busState,
	argv *[]string,
	useIOThread bool,
) (string, error) {
	id := fmt.Sprintf("disk%d", disk.Slot)

	switch bus {
	case "virtio-blk":
		opts := fmt.Sprintf("virtio-blk-pci,drive=%s,id=%s", nodeName, id)
		if state.layout && diskHasPort(disk) {
			// Behind its own root port, so the disk can be unplugged while the
			// guest runs; the boot disk stays on pcie.0 and never can be.
			opts += ",bus=" + DiskPortID(disk.Slot)
		}
		if useIOThread {
			opts += ",iothread=" + ioThreadID
		}
		if disk.ShareRW {
			opts += ",share-rw=on"
		}
		opts += disk.throttleOpts() + disk.bootOpts()
		return opts, nil

	case "virtio-scsi", "scsi":
		if !state.scsiAdded {
			// The iothread binds to the controller here rather than to the
			// disk: virtio-scsi drives every LUN behind it from one queue, so
			// a per-disk binding is not a thing QEMU accepts.
			controller := "virtio-scsi-pci,id=scsi0"
			if useIOThread {
				controller += ",iothread=" + ioThreadID
			}
			*argv = append(*argv, "-device", controller)
			state.scsiAdded = true
		}
		opts := fmt.Sprintf("scsi-hd,drive=%s,id=%s,bus=scsi0.0", nodeName, id)
		if disk.ShareRW {
			opts += ",share-rw=on"
		}
		opts += disk.throttleOpts() + disk.bootOpts()
		return opts, nil

	case "sata":
		// Explicitly onto an AHCI controller. Left to itself QEMU puts an
		// ide-hd on q35's legacy IDE bus, which holds exactly one unit — so a
		// guest with a SATA disk and an installer CD refuses to start, and the
		// only symptom is a monitor socket that never appears.
		controller := state.ahciPort / ahciPortsPerController
		port := state.ahciPort % ahciPortsPerController
		if port == 0 {
			*argv = append(*argv, "-device", fmt.Sprintf("ich9-ahci,id=ahci%d", controller))
		}
		state.ahciPort++

		kind := "ide-hd"
		if disk.IsCDROM {
			kind = "ide-cd"
		}
		opts := fmt.Sprintf("%s,id=%s,bus=ahci%d.%d", kind, id, controller, port)
		if nodeName != "" {
			opts = fmt.Sprintf("%s,drive=%s,id=%s,bus=ahci%d.%d", kind, nodeName, id, controller, port)
			opts += disk.throttleOpts() + disk.bootOpts()
		}
		return opts, nil

	case "ide":
		kind := "ide-hd"
		if disk.IsCDROM {
			kind = "ide-cd"
		}
		opts := fmt.Sprintf("%s,drive=%s,id=%s", kind, nodeName, id)
		opts += disk.throttleOpts() + disk.bootOpts()
		return opts, nil

	default:
		return "", fmt.Errorf("vm: unknown disk bus %q", bus)
	}
}

// buildNICArgs turns NICs into netdev and device pairs.
//
// The tap is created by the netconf package before QEMU starts and passed by
// name with script=no: letting QEMU run its own ifup script would put bridge
// configuration back into a shell script the operator cannot see.
//
// A NIC on a direct network is passed as a descriptor instead, because a
// macvtap has no name QEMU can reopen — see NetdevExecArgv for how the
// descriptor survives the detached start.
//
// Both forms take whatever vhostOption reports, so MCHARBOR_VM_VHOST=off moves
// every guest on the host onto QEMU's own RX path in one restart.
func buildNICArgs(spec Spec, layout bool) ([]string, error) {
	var argv []string
	for _, nic := range spec.NICs {
		if nic.TapName == "" && nic.DevicePath == "" {
			return nil, fmt.Errorf("vm: NIC in slot %d has no tap device", nic.Slot)
		}
		if nic.MAC == "" {
			return nil, fmt.Errorf("vm: NIC in slot %d has no MAC address", nic.Slot)
		}

		netID := fmt.Sprintf("net%d", nic.Slot)
		vhost := vhostOption()
		if nic.DevicePath != "" {
			if nic.FD < netdevFDBase {
				return nil, fmt.Errorf("vm: NIC in slot %d has no usable descriptor", nic.Slot)
			}
			argv = append(argv, "-netdev", fmt.Sprintf(
				"tap,id=%s,fd=%d,%s", netID, nic.FD, vhost))
		} else {
			argv = append(argv, "-netdev", fmt.Sprintf(
				"tap,id=%s,ifname=%s,script=no,downscript=no,%s",
				netID, nic.TapName, vhost))
		}

		model := nic.Model
		if model == "" {
			model = "virtio-net"
		}
		device := "virtio-net-pci"
		if model != "virtio-net" {
			// e1000e for guests too old to have virtio drivers.
			device = model
		}
		opts := fmt.Sprintf("%s,netdev=%s,id=nic%d,mac=%s", device, netID, nic.Slot, nic.MAC)
		if layout && nicHasPort(nic) {
			// An unpinned NIC sits behind its own port. The port is always
			// emitted, so the NIC's bus number, and with it the guest's name
			// for the interface, never moves.
			opts += ",bus=" + NICPortID(nic.Slot)
		}
		if nic.PCIAddr != "" {
			if !validPCIAddr(nic.PCIAddr) {
				return nil, fmt.Errorf("vm: NIC in slot %d has an unusable PCI address %q",
					nic.Slot, nic.PCIAddr)
			}
			opts += ",addr=" + nic.PCIAddr
		}
		argv = append(argv, "-device", opts)
	}
	return argv, nil
}

// validPCIAddr accepts the 0x00-0x1f slot form and nothing else, because this
// value reaches the QEMU command line.
func validPCIAddr(addr string) bool {
	if !strings.HasPrefix(addr, "0x") || len(addr) != 4 {
		return false
	}
	slot, err := strconv.ParseInt(addr[2:], 16, 32)
	return err == nil && slot >= 0 && slot <= 0x1f
}

// buildDeviceArgs passes host devices through.
//
// Every one of these can compromise the host rather than only the guest, which
// is why they sit behind vm.privileged. The address format is validated because
// it goes straight onto the command line.
func buildDeviceArgs(spec Spec) ([]string, error) {
	var argv []string
	for i, device := range spec.Devices {
		switch device.Kind {
		case "pci":
			if err := validatePCIAddress(device.HostAddress); err != nil {
				return nil, err
			}
			argv = append(argv, "-device", fmt.Sprintf("vfio-pci,host=%s,id=hostpci%d",
				device.HostAddress, i))
		case "usb":
			vendor, product, err := parseUSBAddress(device.HostAddress)
			if err != nil {
				return nil, err
			}
			argv = append(argv, "-device", fmt.Sprintf(
				"usb-host,vendorid=0x%s,productid=0x%s,id=hostusb%d,bus=usb0.0",
				vendor, product, i))
		case "block":
			// A raw host block device. The path is validated by the caller
			// against the same deny list pool targets use.
			argv = append(argv, "-blockdev", fmt.Sprintf(
				"driver=host_device,node-name=hostblk%d,filename=%s",
				i, escapeOptValue(device.HostAddress)))
			argv = append(argv, "-device", fmt.Sprintf(
				"virtio-blk-pci,drive=hostblk%d,id=hostblkdev%d", i, i))
		case "u2f":
			// Emulated rather than passed through: there is no host device
			// behind it, which is why it carries no address. It exists for
			// guests testing WebAuthn, where a real token would have to be
			// physically present at the host.
			argv = append(argv, "-device", fmt.Sprintf("u2f-emulated,id=u2f%d,bus=usb0.0", i))
		default:
			return nil, fmt.Errorf("vm: unknown device kind %q", device.Kind)
		}
	}
	return argv, nil
}

// validatePCIAddress checks the BDF form, e.g. 0000:03:00.0 or 03:00.0.
func validatePCIAddress(addr string) error {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return fmt.Errorf("vm: %q is not a PCI address", addr)
	}
	for _, part := range parts[:len(parts)-1] {
		if _, err := strconv.ParseUint(part, 16, 32); err != nil {
			return fmt.Errorf("vm: %q is not a PCI address", addr)
		}
	}
	slot, function, ok := strings.Cut(parts[len(parts)-1], ".")
	if !ok {
		return fmt.Errorf("vm: %q is not a PCI address", addr)
	}
	if _, err := strconv.ParseUint(slot, 16, 32); err != nil {
		return fmt.Errorf("vm: %q is not a PCI address", addr)
	}
	if _, err := strconv.ParseUint(function, 16, 32); err != nil {
		return fmt.Errorf("vm: %q is not a PCI address", addr)
	}
	return nil
}

// parseUSBAddress splits a vendor:product pair and checks both are hex.
func parseUSBAddress(addr string) (vendor, product string, err error) {
	vendor, product, ok := strings.Cut(addr, ":")
	if !ok {
		return "", "", fmt.Errorf("vm: %q is not a USB vendor:product address", addr)
	}
	for _, part := range []string{vendor, product} {
		if len(part) != 4 {
			return "", "", fmt.Errorf("vm: %q is not a USB vendor:product address", addr)
		}
		if _, err := strconv.ParseUint(part, 16, 32); err != nil {
			return "", "", fmt.Errorf("vm: %q is not a USB vendor:product address", addr)
		}
	}
	return vendor, product, nil
}

// escapeOptValue escapes a value going into a comma-separated QEMU option.
//
// QEMU splits these on commas and treats `,,` as a literal comma. A pool path
// containing a comma would otherwise be read as the start of a second option —
// which, in a `-blockdev`, means arbitrary block-layer options chosen by
// whoever named the pool.
func escapeOptValue(value string) string {
	return strings.ReplaceAll(value, ",", ",,")
}

// WriteArgsFile records the exact argv used, for support and for a reproducible
// restart.
//
// This is deliberately the read-only counterpart to an "advanced raw args"
// field the product does not offer: support gets the full picture without the
// unbounded surface of letting an operator edit it.
func WriteArgsFile(path string, argv []string) error {
	payload, err := json.MarshalIndent(map[string]any{
		"argv": argv,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("vm: render args.json: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("vm: write %s: %w", path, err)
	}
	return nil
}
