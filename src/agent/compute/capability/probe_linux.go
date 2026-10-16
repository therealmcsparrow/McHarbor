// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package capability

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/netconf"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"golang.org/x/sys/unix"
)

// probeTimeout bounds each external version query. These are all short
// `--version` calls; one that hangs means a broken binary, and waiting on it
// would stall the whole probe.
const probeTimeout = 10 * time.Second

// Probe runs every check and returns the report.
//
// It never fails as a whole: a check that cannot run reports itself absent with
// the reason as its Detail. A probe that returned an error would leave the UI
// with nothing to show, which is exactly the "capability check failed" outcome
// this package exists to prevent.
func Probe(ctx context.Context, seccompBuiltIn bool, supervisorMode string) Report {
	r := Report{
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		Supported:      true,
		SeccompBuiltIn: seccompBuiltIn,
		SupervisorMode: supervisorMode,
		KernelVersion:  kernelVersion(),

		HostCPUs:        runtime.NumCPU(),
		HostMemoryBytes: hostMemoryBytes(),
	}
	r.HostSockets, r.HostPhysicalCores = cpuTopology(r.HostCPUs)

	r.add(probeKVMDevice())
	r.add(probeCPUVirt())

	binaries := probeQEMUBinaries(ctx)
	r.QEMUBinaries = binaries
	r.add(qemuItem(binaries))
	r.add(probeBinary(ctx, ItemQEMUImg, "qemu-img", BlocksVM, true,
		"qemu-img creates and resizes guest disks.", installHint("qemu-utils", "qemu-img")))
	r.add(probeOVMF())
	r.add(probeOVMFSecure())
	r.add(probeBinary(ctx, ItemSwtpm, "swtpm", BlocksNone, false,
		"swtpm provides the vTPM that Windows 11 requires.", installHint("swtpm", "swtpm")))

	r.add(probeCgroupV2())
	r.add(probeUserns())

	subuid, subgid, subItem := probeSubIDs()
	r.SubuidRanges = subuid
	r.SubgidRanges = subgid
	r.add(subItem)

	r.add(probeIdmappedMounts())
	r.add(probeSystemd(ctx))
	r.add(probeDBus())
	r.add(seccompItem(seccompBuiltIn))
	r.add(probeIOMMU())
	r.add(probeHostNetwork())

	r.HugePages = probeHugePages()
	r.NUMANodes = probeNUMANodes()

	r.deriveConclusions()
	markInstallable(&r)
	return r
}

// kernelVersion reads the running kernel release.
func kernelVersion() string {
	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		return ""
	}
	return unix.ByteSliceToString(uts.Release[:])
}

// probeKVMDevice checks for /dev/kvm and, crucially, whether the agent can open
// it.
//
// Present-but-unopenable is the interesting case and the one a naive existence
// check gets wrong: the node is there, the operator sees nothing missing, and
// every VM start fails with a permission error whose cause is never named. So
// the check actually opens it.
func probeKVMDevice() Item {
	item := Item{Key: ItemKVMDevice, Critical: true, Blocks: BlocksVM}

	info, err := os.Stat("/dev/kvm")
	if err != nil {
		if os.IsNotExist(err) {
			item.Detail = "/dev/kvm does not exist. The kvm kernel module is not loaded, " +
				"or hardware virtualization is disabled in firmware."
			item.Remediation = "Check that virtualization (VT-x / AMD-V) is enabled in the " +
				"BIOS or UEFI setup, then run: sudo modprobe kvm_intel || sudo modprobe kvm_amd"
			return item
		}
		item.Detail = fmt.Sprintf("/dev/kvm could not be inspected: %v", err)
		return item
	}
	_ = info

	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		item.Detail = fmt.Sprintf("/dev/kvm exists but this agent cannot open it: %v", err)
		item.Remediation = "Add the agent's user to the kvm group and restart the agent: " +
			"sudo usermod -aG kvm $(whoami)"
		return item
	}
	errlog.Close(f)

	item.Present = true
	item.Detail = "/dev/kvm is present and writable."
	return item
}

// cpuFlagRE matches the flags line in /proc/cpuinfo.
var cpuFlagRE = regexp.MustCompile(`^flags\s*:\s*(.*)$`)

// hostMemoryBytes reads MemTotal, which is the installed RAM the kernel can
// hand out. Zero when it cannot be read, and the UI then bounds nothing rather
// than bounding by a guess.
func hostMemoryBytes() int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer func() { errlog.Close(f) }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		// "MemTotal:  16316360 kB"
		if len(fields) < 2 || fields[0] != "MemTotal:" {
			continue
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
}

// cpuTopology returns the socket and physical-core counts behind the thread
// count, so an overcommit ratio can be judged against cores rather than SMT
// siblings.
//
// Both fall back to threads when the topology cannot be read, which makes a
// 1:1 ratio the conservative answer on a host that will not say.
func cpuTopology(threads int) (sockets, cores int) {
	if sockets, cores = cpuTopologyProc(); cores > 0 {
		return sockets, cores
	}
	if sockets, cores = cpuTopologySysfs(); cores > 0 {
		return sockets, cores
	}
	return 1, threads
}

// cpuTopologyProc counts the distinct (physical id, core id) pairs in
// /proc/cpuinfo, which is one entry per thread.
func cpuTopologyProc() (sockets, cores int) {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return 0, 0
	}
	defer func() { errlog.Close(f) }()

	packages := map[string]struct{}{}
	pairs := map[string]struct{}{}
	var pkg, core string

	flush := func() {
		if pkg == "" || core == "" {
			return
		}
		packages[pkg] = struct{}{}
		pairs[pkg+":"+core] = struct{}{}
		pkg, core = "", ""
	}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			// A blank line ends one processor block.
			flush()
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "physical id":
			pkg = strings.TrimSpace(value)
		case "core id":
			core = strings.TrimSpace(value)
		}
	}
	flush()

	return len(packages), len(pairs)
}

// cpuTopologySysfs reads the per-CPU topology directory, which ARM and other
// architectures populate even though they omit the x86 fields /proc/cpuinfo
// carries.
func cpuTopologySysfs() (sockets, cores int) {
	entries, err := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/topology")
	if err != nil || len(entries) == 0 {
		return 0, 0
	}

	packages := map[string]struct{}{}
	pairs := map[string]struct{}{}
	for _, dir := range entries {
		pkg, err := os.ReadFile(filepath.Join(dir, "physical_package_id"))
		if err != nil {
			continue
		}
		core, err := os.ReadFile(filepath.Join(dir, "core_id"))
		if err != nil {
			continue
		}
		p := strings.TrimSpace(string(pkg))
		c := strings.TrimSpace(string(core))
		packages[p] = struct{}{}
		pairs[p+":"+c] = struct{}{}
	}
	return len(packages), len(pairs)
}

// probeCPUVirt reports the CPU vendor and whether it advertises virtualization
// extensions.
//
// This is what makes the /dev/kvm message actionable: a CPU that reports vmx or
// svm while /dev/kvm is missing means the extensions exist and are switched off
// in firmware, which is a completely different fix from "this CPU cannot do it".
func probeCPUVirt() Item {
	item := Item{Key: ItemCPUVirt, Blocks: BlocksVM}

	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		item.Detail = fmt.Sprintf("/proc/cpuinfo unreadable: %v", err)
		return item
	}
	defer func() { errlog.Close(f) }()

	var vendor, model string
	var flags []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "vendor_id"):
			if vendor == "" {
				vendor = valueAfterColon(line)
			}
		case strings.HasPrefix(line, "model name"):
			if model == "" {
				model = valueAfterColon(line)
			}
		case cpuFlagRE.MatchString(line):
			if flags == nil {
				flags = strings.Fields(valueAfterColon(line))
			}
		}
	}

	hasVMX := contains(flags, "vmx")
	hasSVM := contains(flags, "svm")
	switch {
	case hasVMX:
		item.Present = true
		item.Detail = fmt.Sprintf("%s reports Intel VT-x (vmx).", displayCPU(vendor, model))
	case hasSVM:
		item.Present = true
		item.Detail = fmt.Sprintf("%s reports AMD-V (svm).", displayCPU(vendor, model))
	default:
		item.Detail = fmt.Sprintf("%s does not report virtualization extensions. "+
			"They are usually disabled in firmware rather than absent.", displayCPU(vendor, model))
		item.Remediation = "Enable Intel VT-x or AMD-V (sometimes called SVM Mode) in the " +
			"BIOS or UEFI setup and reboot."
	}
	return item
}

func displayCPU(vendor, model string) string {
	if model != "" {
		return model
	}
	if vendor != "" {
		return vendor
	}
	return "This CPU"
}

func valueAfterColon(line string) string {
	_, v, ok := strings.Cut(line, ":")
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// qemuArches are the emulators McHarbor drives. Foreign-architecture emulation
// is a non-goal, so this list is exactly the host architectures supported and
// nothing else.
var qemuArches = []string{"x86_64", "aarch64"}

// probeQEMUBinaries finds the qemu-system-* binaries and reads their versions
// and machine types.
func probeQEMUBinaries(ctx context.Context) []QEMUBinary {
	type found struct {
		arch string
		path string
	}
	var present []found
	for _, arch := range qemuArches {
		path, err := exec.LookPath("qemu-system-" + arch)
		if err != nil {
			continue
		}
		present = append(present, found{arch: arch, path: path})
	}

	// Every query below is an independent subprocess bounded by probeTimeout.
	// Run serially they add up to two minutes on a host where QEMU is slow to
	// start, which is longer than the deadline the caller gives this probe;
	// run concurrently the whole thing costs one timeout at worst.
	out := make([]QEMUBinary, len(present))
	var wg sync.WaitGroup
	for i, entry := range present {
		wg.Add(1)
		go func(i int, arch, path string) {
			defer wg.Done()
			bin := QEMUBinary{Path: path, Arch: arch}
			var inner sync.WaitGroup
			inner.Add(6)
			go func() { defer inner.Done(); bin.Version = qemuVersion(ctx, path) }()
			go func() { defer inner.Done(); bin.MachineTypes = qemuMachineTypes(ctx, path) }()
			go func() { defer inner.Done(); bin.Devices = qemuDevices(ctx, path) }()
			go func() { defer inner.Done(); bin.CPUModels = qemuCPUModels(ctx, path) }()
			go func() { defer inner.Done(); bin.QMPCommands = qemuQMPCommands(ctx, path) }()
			spiceUsable := false
			go func() { defer inner.Done(); spiceUsable = qemuSPICEUsable(ctx, path) }()
			// KVM acceleration is only meaningful for the host's own
			// architecture. A qemu-system-aarch64 on an x86 host can only do
			// TCG, which the roadmap refuses rather than offers.
			hasKVM := matchesHostArch(arch) && qemuHasKVM(ctx, path)
			inner.Wait()
			bin.KVMAccel = hasKVM
			// query-spice is the SPICE gate everywhere downstream, so a binary that
			// lists it but cannot start a SPICE server must not report it.
			if !spiceUsable {
				bin.QMPCommands = withoutCommand(bin.QMPCommands, "query-spice")
			}
			out[i] = bin
		}(i, entry.arch, entry.path)
	}
	wg.Wait()
	return out
}

func matchesHostArch(arch string) bool {
	switch runtime.GOARCH {
	case "amd64":
		return arch == "x86_64"
	case "arm64":
		return arch == "aarch64"
	default:
		return false
	}
}

var qemuVersionRE = regexp.MustCompile(`version\s+([0-9]+\.[0-9]+\.[0-9]+)`)

func qemuVersion(ctx context.Context, path string) string {
	out, err := runBounded(ctx, path, "--version")
	if err != nil {
		return ""
	}
	if m := qemuVersionRE.FindStringSubmatch(out); len(m) == 2 {
		return m[1]
	}
	return ""
}

// machineTypeRE pulls the machine name out of a `-machine help` line, which
// looks like "pc-q35-8.2             Standard PC (Q35 + ICH9, 2009)".
var machineTypeRE = regexp.MustCompile(`^([a-zA-Z0-9._-]+)\s{2,}`)

// qemuMachineTypes lists the machine types this binary offers.
//
// The list matters because McHarbor pins a concrete versioned machine type per
// VM (pc-q35-8.2, not q35) at creation. An unpinned type is how guests break on
// a host package update, and pinning requires knowing what is actually
// available before the create dialog renders rather than at boot-failure time.
func qemuMachineTypes(ctx context.Context, path string) []string {
	out, err := runBounded(ctx, path, "-machine", "help")
	if err != nil {
		return nil
	}
	var types []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "Supported machines") {
			continue
		}
		if m := machineTypeRE.FindStringSubmatch(line); len(m) == 2 {
			if m[1] == "none" || strings.HasSuffix(m[1], "(alias") {
				continue
			}
			types = append(types, m[1])
		}
	}
	return types
}

// qemuHasKVM checks that the binary offers the kvm accelerator.
func qemuHasKVM(ctx context.Context, path string) bool {
	out, err := runBounded(ctx, path, "-accel", "help")
	if err != nil {
		return false
	}
	return strings.Contains(out, "kvm")
}

// qemuItem summarizes the binaries into one checklist line.
func qemuItem(binaries []QEMUBinary) Item {
	item := Item{Key: ItemQEMU, Critical: true, Blocks: BlocksVM}
	if len(binaries) == 0 {
		item.Detail = "No qemu-system-* binary found on PATH."
		item.Remediation = installHint("qemu-system-x86", "qemu-system-x86_64")
		return item
	}

	var usable []string
	for _, b := range binaries {
		if b.KVMAccel {
			usable = append(usable, fmt.Sprintf("%s %s", filepath.Base(b.Path), b.Version))
		}
	}
	if len(usable) == 0 {
		// TCG fallback is refused rather than offered: a guest emulated
		// without hardware acceleration is slow enough to look broken, and
		// offering it would generate support load for a mode nobody wants.
		item.Detail = "QEMU is installed but offers no KVM acceleration for this host's " +
			"architecture. McHarbor does not fall back to software emulation."
		item.Remediation = "Ensure /dev/kvm is available and the kvm module is loaded."
		return item
	}

	item.Present = true
	item.Detail = strings.Join(usable, ", ") + " with KVM acceleration."
	return item
}

// probeOVMF locates the UEFI firmware.
//
// UEFI is the default firmware and is required for Windows 11, so its absence
// is worth naming even though BIOS guests still work without it.
func probeOVMF() Item {
	item := Item{Key: ItemOVMF, Blocks: BlocksNone}
	code, vars := OVMFPaths(false)
	if code != "" && vars != "" {
		item.Present = true
		item.Detail = "UEFI firmware at " + code
		return item
	}
	item.Detail = "No OVMF/EDK2 UEFI firmware found. VMs can still boot with BIOS " +
		"firmware, but Windows 11 and Secure Boot need UEFI."
	item.Remediation = installHint("ovmf", "edk2-ovmf")
	return item
}

// probeOVMFSecure reports whether this host can enforce Secure Boot.
//
// Both halves are required and they fail differently: the SMM build without
// enrolled keys boots with Secure Boot in setup mode, which Windows 11 treats
// as off, so a host with only one of them is reported as not capable.
func probeOVMFSecure() Item {
	item := Item{Key: ItemOVMFSecure, Blocks: BlocksNone}
	code, vars := OVMFPaths(true)
	switch {
	case code != "" && vars != "":
		item.Present = true
		item.Detail = "Secure Boot firmware at " + code + ", keys from " + vars
	case code == "" && vars != "":
		item.Detail = "The enrolled Secure Boot keys are here but the SMM firmware build is not."
		item.Remediation = installHint("ovmf", "edk2-ovmf")
	case code != "" && vars == "":
		item.Detail = "The SMM firmware build is here but no variable template with " +
			"Microsoft's keys enrolled was found. Secure Boot would start in setup mode, " +
			"which Windows 11 counts as disabled."
		item.Remediation = installHint("ovmf", "edk2-ovmf")
	default:
		item.Detail = "No Secure Boot firmware found. Windows 11 needs it, alongside a TPM."
		item.Remediation = installHint("ovmf", "edk2-ovmf")
	}
	return item
}

// probeCgroupV2 checks for the unified hierarchy.
//
// Cgroup v1 hosts are refused for containers rather than half-supported: the
// limit semantics differ enough that a "working" v1 path would silently apply
// different limits than the UI showed.
func probeCgroupV2() Item {
	item := Item{Key: ItemCgroupV2, Critical: true, Blocks: BlocksCT}

	var st unix.Statfs_t
	if err := unix.Statfs("/sys/fs/cgroup", &st); err != nil {
		item.Detail = fmt.Sprintf("/sys/fs/cgroup is not mounted: %v", err)
		return item
	}
	// CGROUP2_SUPER_MAGIC identifies the unified hierarchy; the v1 hierarchy
	// reports a different magic even at the same path.
	if st.Type != unix.CGROUP2_SUPER_MAGIC {
		item.Detail = "/sys/fs/cgroup is a cgroup v1 hierarchy. McHarbor system " +
			"containers require the v2 unified hierarchy."
		item.Remediation = "Boot with systemd.unified_cgroup_hierarchy=1 on the kernel " +
			"command line, or upgrade to a distribution release that defaults to cgroup v2."
		return item
	}

	item.Present = true
	item.Detail = "cgroup v2 unified hierarchy mounted at /sys/fs/cgroup."
	if controllers, err := os.ReadFile("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		item.Detail += " Controllers: " + strings.TrimSpace(string(controllers)) + "."
	}
	return item
}

// probeUserns checks that unprivileged user namespaces are available, which is
// what makes unprivileged containers possible at all.
func probeUserns() Item {
	item := Item{Key: ItemUserns, Critical: true, Blocks: BlocksCT}

	if _, err := os.Stat("/proc/self/ns/user"); err != nil {
		item.Detail = "The kernel does not expose user namespaces."
		item.Remediation = "Use a kernel built with CONFIG_USER_NS=y."
		return item
	}

	// Debian and Ubuntu ship a sysctl that disables unprivileged userns
	// creation. It exists, it defaults on in some releases, and a container
	// that fails because of it gives an error that never mentions it.
	if data, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone"); err == nil {
		if strings.TrimSpace(string(data)) == "0" {
			item.Detail = "User namespaces exist but unprivileged creation is disabled by " +
				"kernel.unprivileged_userns_clone=0."
			item.Remediation = "sudo sysctl -w kernel.unprivileged_userns_clone=1 " +
				"(persist it in /etc/sysctl.d/)"
			return item
		}
	}
	if data, err := os.ReadFile("/proc/sys/user/max_user_namespaces"); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && n == 0 {
			item.Detail = "user.max_user_namespaces is 0, so no user namespace can be created."
			item.Remediation = "sudo sysctl -w user.max_user_namespaces=15000 " +
				"(persist it in /etc/sysctl.d/)"
			return item
		}
	}

	item.Present = true
	item.Detail = "User namespaces are available."
	return item
}

// minSubIDSpace is the smallest usable subordinate id envelope: one container
// needs 65536 ids, so anything less cannot host even one unprivileged
// container.
const minSubIDSpace = 65536

// probeSubIDs reads the agent user's /etc/subuid and /etc/subgid envelopes.
//
// These are read once, to establish the envelope the allocator may carve up.
// They are deliberately NOT used to work out which sub-ranges are free — that
// allocation lives in the McHarbor database so it stays auditable and stable
// across agent restarts, rather than being inferred from what other tools may
// have written.
func probeSubIDs() ([]IDRange, []IDRange, Item) {
	item := Item{Key: ItemSubuid, Critical: true, Blocks: BlocksCT}

	user := currentUserName()
	uidRanges := readSubIDFile("/etc/subuid", user)
	gidRanges := readSubIDFile("/etc/subgid", user)

	// As root the envelope is not the limit: McHarbor allocates the ranges and
	// the agent writes the map, so "room for N containers" computed from
	// /etc/subuid described nothing that actually happens.
	if os.Geteuid() == 0 {
		return uidRanges, gidRanges, rootSubIDItem(readAllSubIDs("/etc/subuid"), readAllSubIDs("/etc/subgid"))
	}

	var uidTotal, gidTotal int64
	for _, r := range uidRanges {
		uidTotal += r.Count
	}
	for _, r := range gidRanges {
		gidTotal += r.Count
	}

	switch {
	case uidTotal < minSubIDSpace || gidTotal < minSubIDSpace:
		item.Detail = fmt.Sprintf(
			"User %q has %d subordinate uids and %d subordinate gids. "+
				"Each unprivileged container needs a distinct, non-overlapping block of %d.",
			user, uidTotal, gidTotal, minSubIDSpace)
		item.Remediation = fmt.Sprintf(
			"sudo usermod --add-subuids 100000-165535 --add-subgids 100000-165535 %s", user)
	default:
		item.Present = true
		item.Detail = fmt.Sprintf(
			"User %q has %d subordinate uids and %d subordinate gids — room for %d containers.",
			user, uidTotal, gidTotal, uidTotal/minSubIDSpace)
	}
	return uidRanges, gidRanges, item
}

// currentUserName resolves the agent's own username without pulling in os/user,
// which needs cgo for a complete answer on glibc hosts.
func currentUserName() string {
	uid := os.Getuid()
	if name := lookupPasswdName(uid); name != "" {
		return name
	}
	return strconv.Itoa(uid)
}

func lookupPasswdName(uid int) string {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return ""
	}
	defer func() { errlog.Close(f) }()

	want := strconv.Itoa(uid)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) >= 3 && fields[2] == want {
			return fields[0]
		}
	}
	return ""
}

// readAllSubIDs parses every user's entries, for the overlap check; a missing
// file is simply no entries.
func readAllSubIDs(path string) []SubIDEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { errlog.Close(f) }()
	return parseSubIDs(f)
}

// readSubIDFile parses /etc/subuid or /etc/subgid for one user.
func readSubIDFile(path, user string) []IDRange {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { errlog.Close(f) }()

	var out []IDRange
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(strings.TrimSpace(sc.Text()), ":")
		if len(fields) != 3 || fields[0] != user {
			continue
		}
		start, err1 := strconv.ParseInt(fields[1], 10, 64)
		count, err2 := strconv.ParseInt(fields[2], 10, 64)
		if err1 != nil || err2 != nil || count <= 0 {
			continue
		}
		out = append(out, IDRange{Start: start, Count: count})
	}
	return out
}

// probeIdmappedMounts checks for the kernel feature that makes container clones
// instant.
//
// Without it the fallback is a recursive chown on unpack, which works
// everywhere but costs a pass over the tree per container and breaks CoW
// cloning entirely. That difference is large enough that the UI states which
// mechanism is in use — an operator whose clones are suddenly slow deserves to
// know why.
func probeIdmappedMounts() Item {
	item := Item{Key: ItemIdmappedMounts, Blocks: BlocksNone}

	// mount_setattr arrived in 5.12 and matured by 5.19. Rather than parse the
	// kernel version, call it with deliberately invalid arguments: ENOSYS
	// means the syscall is absent, any other errno means it is there.
	err := unix.MountSetattr(-1, "", 0, nil)
	if err == unix.ENOSYS {
		item.Detail = "This kernel has no mount_setattr, so idmapped mounts are unavailable. " +
			"Container rootfs ownership will be applied by a recursive chown on unpack, " +
			"which makes clones slower and copy-on-write cloning unavailable."
		item.Remediation = "Upgrade to kernel 5.12 or newer (5.19+ recommended)."
		return item
	}

	item.Present = true
	item.Detail = "Idmapped mounts are available, so container rootfs trees can be shared " +
		"and cloned without a chown pass."
	return item
}

// probeSystemd checks that the host is running systemd as PID 1.
//
// A host without systemd is refused for guest hosting rather than given a
// bespoke supervisor: writing a second init system is not a reasonable scope
// for this project, and a half-supervised guest is worse than an unsupported
// one.
func probeSystemd(ctx context.Context) Item {
	item := Item{Key: ItemSystemd, Critical: true, Blocks: BlocksBoth}

	if _, err := os.Stat("/run/systemd/system"); err != nil {
		item.Detail = "This host is not running systemd. McHarbor supervises guests with " +
			"systemd transient units so they survive an agent restart or self-update."
		item.Remediation = "Compute requires a systemd host."
		return item
	}

	item.Present = true
	item.Detail = "systemd is PID 1."
	if out, err := runBounded(ctx, "systemctl", "--version"); err == nil {
		if line := firstLine(out); line != "" {
			item.Detail = line + " is PID 1."
		}
	}
	return item
}

// probeDBus checks that the agent can reach the system bus.
//
// This is not critical: the systemd-run CLI fallback covers a host where the
// bus is unreachable. It is reported so the operator can see which supervision
// path is in use, because the two differ in how quickly a crashed guest
// surfaces.
func probeDBus() Item {
	item := Item{Key: ItemDBus, Blocks: BlocksNone}

	if _, err := os.Stat("/run/dbus/system_bus_socket"); err != nil {
		item.Detail = "The D-Bus system bus socket is not reachable. McHarbor will fall back " +
			"to systemd-run, which polls for guest state instead of receiving it as it changes."
		item.Remediation = "Ensure dbus is running and /run/dbus is visible to the agent."
		return item
	}

	item.Present = true
	item.Detail = "The systemd system bus is reachable, so guest state changes arrive as " +
		"they happen."
	return item
}

// seccompItem reports whether this agent binary carries a syscall filter.
//
// Built without the seccomp tag, libcontainer applies no filter at all. That is
// a real posture change and it must never be invisible, so it is a first-class
// checklist line rather than a footnote.
func seccompItem(builtIn bool) Item {
	item := Item{Key: ItemSeccomp, Present: builtIn, Blocks: BlocksNone}
	if builtIn {
		item.Detail = "This agent was built with seccomp support, so containers get a " +
			"syscall filter."
		return item
	}
	item.Detail = "This agent was built WITHOUT seccomp support. Containers will run with " +
		"no syscall filter — user namespace isolation still applies, but the additional " +
		"syscall restriction does not."
	item.Remediation = "Install an agent build compiled with the seccomp build tag."
	return item
}

// probeIOMMU reports whether device passthrough is possible at all.
func probeIOMMU() Item {
	item := Item{Key: ItemIOMMU, Blocks: BlocksNone}

	groups, err := filepath.Glob("/sys/kernel/iommu_groups/*")
	if err != nil || len(groups) == 0 {
		item.Detail = "No IOMMU groups found, so PCI and GPU passthrough are unavailable."
		item.Remediation = "Enable VT-d or AMD-Vi in firmware and add intel_iommu=on " +
			"(or amd_iommu=on) to the kernel command line."
		return item
	}

	item.Present = true
	item.Detail = fmt.Sprintf("IOMMU is active with %d groups, so device passthrough is "+
		"possible.", len(groups))
	return item
}

// hostNetworkRemediation names the one deployment change that fixes an agent
// with no view of the host's network.
const hostNetworkRemediation = "Run the agent in the host's network namespace: " +
	"docker run --network host --cap-add NET_ADMIN ... (or network_mode: host " +
	"with cap_add: [NET_ADMIN] in compose)."

// probeHostNetwork reports whether the agent can see and change the host's own
// interfaces.
//
// A containerised agent on a bridge network sees loopback and its own veth and
// nothing else, so every bridge and uplink picker comes up empty with no stated
// reason. This check names that, because the enumeration itself cannot fail —
// it succeeds and returns the wrong namespace.
func probeHostNetwork() Item {
	item := Item{Key: ItemHostNetwork, Blocks: BlocksNone}
	netAdmin := hasNetAdmin()

	links, err := netconf.Links()
	if err != nil {
		item.Detail = "This host's interfaces could not be enumerated: " + err.Error()
		item.Remediation = hostNetworkRemediation
		return item
	}

	// Counted with the same classification the network picker filters on, so a
	// pass here means the picker has something to offer.
	var bridges, uplinks int
	for _, link := range links {
		switch link.Kind {
		case "bridge":
			bridges++
		case "physical", "bond", "vlan":
			uplinks++
		}
	}

	if bridges == 0 && uplinks == 0 {
		item.Detail = fmt.Sprintf("No bridge or physical interface is visible — this "+
			"network namespace holds only %d loopback or virtual device(s). Guest "+
			"networks cannot be created or attached from here.", len(links))
		item.Remediation = hostNetworkRemediation
		return item
	}

	if !netAdmin {
		item.Detail = fmt.Sprintf("%d bridge(s) and %d uplink(s) are visible, but the "+
			"agent does not hold CAP_NET_ADMIN, so bridges and guest interfaces "+
			"cannot be created.", bridges, uplinks)
		item.Remediation = hostNetworkRemediation
		return item
	}

	item.Present = true
	item.Detail = fmt.Sprintf("%d bridge(s) and %d uplink(s) are visible and the agent "+
		"holds CAP_NET_ADMIN.", bridges, uplinks)
	return item
}

// hasNetAdmin reads the effective capability set for CAP_NET_ADMIN.
//
// Reported rather than assumed from uid 0: a rootful agent in a container still
// loses NET_ADMIN unless it is granted explicitly, which is exactly the case
// this check has to catch.
func hasNetAdmin() bool {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return false
	}
	defer func() { errlog.Close(f) }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		value, ok := strings.CutPrefix(scanner.Text(), "CapEff:")
		if !ok {
			continue
		}
		mask, err := strconv.ParseUint(strings.TrimSpace(value), 16, 64)
		if err != nil {
			return false
		}
		return mask&(1<<uint(unix.CAP_NET_ADMIN)) != 0
	}
	return false
}

// probeBinary is the generic "is this tool on PATH" check.
func probeBinary(ctx context.Context, key, binary, blocks string, critical bool, why, remediation string) Item {
	item := Item{Key: key, Critical: critical, Blocks: blocks}

	path, err := exec.LookPath(binary)
	if err != nil {
		item.Detail = binary + " is not on PATH. " + why
		item.Remediation = remediation
		return item
	}

	item.Present = true
	item.Detail = path
	if out, err := runBounded(ctx, path, "--version"); err == nil {
		if line := firstLine(out); line != "" {
			item.Detail = line
		}
	}
	return item
}

// runBounded executes a short query under a timeout.
func runBounded(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// installHint builds a per-distro install command from the host's own
// /etc/os-release.
//
// A generic "install qemu" is the difference between a checklist an operator
// can act on and one they have to go research. Detecting the family costs one
// file read and turns the remediation into something copy-pasteable.
func installHint(debPackage, rpmPackage string) string {
	switch detectFamily() {
	case "debian":
		return "sudo apt install " + debPackage
	case "rhel":
		return "sudo dnf install " + rpmPackage
	case "suse":
		return "sudo zypper install " + rpmPackage
	case "arch":
		return "sudo pacman -S " + rpmPackage
	case "alpine":
		return "sudo apk add " + rpmPackage
	default:
		return "Install " + rpmPackage + " with this host's package manager."
	}
}

// detectFamily reads /etc/os-release and maps ID / ID_LIKE onto a package
// manager family.
func detectFamily() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer func() { errlog.Close(f) }()

	var id, idLike string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"`)
		switch key {
		case "ID":
			id = value
		case "ID_LIKE":
			idLike = value
		}
	}

	haystack := id + " " + idLike
	switch {
	case strings.Contains(haystack, "debian"), strings.Contains(haystack, "ubuntu"):
		return "debian"
	case strings.Contains(haystack, "rhel"), strings.Contains(haystack, "fedora"),
		strings.Contains(haystack, "centos"), strings.Contains(haystack, "rocky"),
		strings.Contains(haystack, "almalinux"):
		return "rhel"
	case strings.Contains(haystack, "suse"):
		return "suse"
	case strings.Contains(haystack, "arch"):
		return "arch"
	case strings.Contains(haystack, "alpine"):
		return "alpine"
	default:
		return ""
	}
}

// add appends an item and is the only way items enter a report, so the
// conclusions below can be derived rather than maintained separately.
func (r *Report) add(item Item) {
	r.Items = append(r.Items, item)
}

// present reports whether a check passed, treating an absent check as failed —
// a check that did not run is not evidence that the thing is there.
func (r *Report) present(key string) bool {
	item, ok := r.Item(key)
	return ok && item.Present
}

// deriveConclusions sets CanRunVMs and CanRunContainers from the items, so the
// two answers can never disagree with the evidence shown beside them.
func (r *Report) deriveConclusions() {
	r.CanRunVMs = r.present(ItemKVMDevice) &&
		r.present(ItemQEMU) &&
		r.present(ItemQEMUImg) &&
		r.present(ItemSystemd)

	r.CanRunContainers = r.present(ItemCgroupV2) &&
		r.present(ItemUserns) &&
		r.present(ItemSystemd)
}
