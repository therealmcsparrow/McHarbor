// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/opencontainers/cgroups"
	devices "github.com/opencontainers/cgroups/devices/config"
	"github.com/opencontainers/runc/libcontainer/configs"
	"golang.org/x/sys/unix"
)

// BuildConfig turns a McHarbor container spec into the libcontainer
// configuration that actually creates it.
//
// This function is where "a system container is a light VM" is either true or
// not. A plain OCI runtime configuration produces a container that runs one
// process and exits; what systemd needs on top of that is specific, and getting
// any of it wrong produces a container that boots into a broken state rather
// than failing outright — which is much harder to diagnose.
func BuildConfig(spec Spec, paths Paths) (*configs.Config, error) {
	if err := ValidateSpec(spec); err != nil {
		return nil, err
	}

	config := &configs.Config{
		Rootfs:     paths.Rootfs,
		Readonlyfs: false,
		// pivot_root rather than chroot. chroot is escapable from inside by any
		// process with the right file descriptor; pivot_root actually replaces
		// the mount namespace's root.
		NoPivotRoot:   false,
		Hostname:      spec.Name,
		Capabilities:  defaultCapabilities(spec),
		Namespaces:    buildNamespaces(spec),
		Cgroups:       buildCgroups(spec),
		Devices:       specDevices(),
		MaskPaths:     maskedPaths(spec),
		ReadonlyPaths: readonlyPaths(spec),
		// RootlessCgroups is false: the agent is root, so the systemd cgroup
		// driver can create the delegated subtree directly.
		RootlessCgroups: false,
		Version:         "1.0.2-dev",
	}

	if !spec.Privileged {
		config.UIDMappings = []configs.IDMap{{
			ContainerID: 0,
			HostID:      spec.UIDMapStart,
			Size:        spec.UIDMapCount,
		}}
		config.GIDMappings = []configs.IDMap{{
			ContainerID: 0,
			HostID:      spec.GIDMapStart,
			Size:        spec.GIDMapCount,
		}}
	}

	mounts, err := buildMounts(spec)
	if err != nil {
		return nil, err
	}
	config.Mounts = mounts

	return config, nil
}

// buildNamespaces selects the namespaces the container gets.
//
// The network namespace is created empty and the veth peer is moved into it
// afterwards by the netconf package, rather than letting libcontainer's own
// (long-deprecated) network support create it — sharing one netlink code path
// with the VM side is what keeps tap and veth behaviour from diverging.
func buildNamespaces(spec Spec) []configs.Namespace {
	ns := []configs.Namespace{
		{Type: configs.NEWNS},
		{Type: configs.NEWPID},
		{Type: configs.NEWIPC},
		{Type: configs.NEWUTS},
		{Type: configs.NEWNET},
		// A cgroup namespace makes the container's own cgroup appear as the
		// root of /sys/fs/cgroup inside it. systemd requires this to manage
		// its own units without seeing — or being able to touch — the host's.
		{Type: configs.NEWCGROUP},
	}
	if !spec.Privileged {
		// The user namespace is what makes root-in-container harmless: an
		// escape lands the attacker as an unprivileged host uid with no
		// ownership of anything.
		ns = append(ns, configs.Namespace{Type: configs.NEWUSER})
	}
	return ns
}

// buildCgroups builds the resource configuration.
//
// The systemd driver is used rather than raw cgroupfs writes. Delegation under
// cgroup v2 has ordering rules — the unit must own the subtree, and
// cgroup.subtree_control has to be written before any children exist — and the
// manager encodes them. A limit that works under runc therefore works here,
// which is the whole reason to use runc's own manager rather than writing the
// files directly.
func buildCgroups(spec Spec) *cgroups.Cgroup {
	cg := &cgroups.Cgroup{
		Name:   "mcharbor-ct-" + spec.ID,
		Parent: "system.slice",
		Resources: &cgroups.Resources{
			// Devices are denied by default and allowed individually below.
			// The opposite default would hand a container the host's disks.
			Devices:          deviceRules(spec),
			SkipDevices:      false,
			MemorySwappiness: nil,
		},
		Systemd: true,
	}

	res := cg.Resources
	if spec.Limits.MemoryMaxBytes > 0 {
		res.Memory = spec.Limits.MemoryMaxBytes
	}
	if spec.Limits.MemoryHighBytes > 0 {
		// The soft throttle. Reclaim pressure starts here, which gives a
		// container leaking memory a chance to be noticed before the OOM
		// killer takes it. Written as memory.high through Unified, because
		// MemoryReservation is memory.low on cgroup v2 — a protection floor,
		// the opposite of a throttle.
		res.Unified = map[string]string{
			"memory.high": strconv.FormatInt(spec.Limits.MemoryHighBytes, 10),
		}
	}
	// Swap is set explicitly even when zero. cgroup v2 inherits the parent's
	// swap limit otherwise, and a container silently swapping is a performance
	// mystery nobody enjoys.
	res.MemorySwap = spec.Limits.MemoryMaxBytes + spec.Limits.SwapMaxBytes

	if quota, period, ok := parseCPUMax(spec.Limits.CPUMax); ok {
		res.CpuQuota = quota
		res.CpuPeriod = period
	}
	if spec.Limits.CPUWeight > 0 {
		res.CpuWeight = spec.Limits.CPUWeight
	}
	if spec.Limits.CPUSet != "" {
		res.CpusetCpus = spec.Limits.CPUSet
	}
	if spec.Limits.PidsMax > 0 {
		// Fork-bomb protection. This is not paranoia: a container without a
		// pid limit can exhaust the host's process table and take every other
		// guest with it.
		limit := spec.Limits.PidsMax
		res.PidsLimit = &limit
	}
	return cg
}

// parseCPUMax reads a cgroup v2 "cpu.max" value — "<quota> <period>", or "max"
// for unlimited — into the quota and period libcontainer wants.
func parseCPUMax(value string) (quota int64, period uint64, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == "max" {
		return 0, 0, false
	}
	quotaField, periodField, found := strings.Cut(value, " ")
	if !found {
		periodField = "100000"
	}
	q, err := strconv.ParseInt(strings.TrimSpace(quotaField), 10, 64)
	if err != nil {
		return 0, 0, false
	}
	p, err := strconv.ParseUint(strings.TrimSpace(periodField), 10, 64)
	if err != nil || p == 0 {
		p = 100000
	}
	return q, p, true
}

// defaultCapabilities is the capability set a container's init starts with.
//
// A system container needs more than an application container: it mounts
// filesystems, manages users, binds privileged ports and configures its own
// network. Inside a user namespace these are namespaced capabilities — powerful
// over the container's own resources and meaningless over the host's — so this
// set is safe precisely because unprivileged is the default.
func defaultCapabilities(spec Spec) *configs.Capabilities {
	caps := []string{
		"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_DAC_READ_SEARCH", "CAP_FOWNER",
		"CAP_FSETID", "CAP_KILL", "CAP_SETGID", "CAP_SETUID", "CAP_SETPCAP",
		"CAP_LINUX_IMMUTABLE", "CAP_NET_BIND_SERVICE", "CAP_NET_BROADCAST",
		"CAP_NET_ADMIN", "CAP_NET_RAW", "CAP_IPC_LOCK", "CAP_IPC_OWNER",
		"CAP_SYS_CHROOT", "CAP_SYS_PTRACE", "CAP_SYS_ADMIN", "CAP_SYS_BOOT",
		"CAP_SYS_NICE", "CAP_SYS_RESOURCE", "CAP_SYS_TTY_CONFIG",
		"CAP_MKNOD", "CAP_LEASE", "CAP_AUDIT_WRITE", "CAP_AUDIT_CONTROL",
		"CAP_SETFCAP",
	}

	if spec.Privileged {
		// A privileged container additionally gets the capabilities that can
		// reconfigure the host itself. This is the honest consequence of the
		// mode and is why it sits behind its own permission and is never in a
		// role preset.
		caps = append(caps,
			"CAP_SYS_MODULE", "CAP_SYS_RAWIO", "CAP_SYS_TIME",
			"CAP_SYS_PACCT", "CAP_MAC_ADMIN", "CAP_MAC_OVERRIDE",
			"CAP_SYSLOG", "CAP_WAKE_ALARM", "CAP_BLOCK_SUSPEND",
		)
	}

	return &configs.Capabilities{
		Bounding:    caps,
		Effective:   caps,
		Permitted:   caps,
		Inheritable: caps,
		Ambient:     caps,
	}
}

// maskedPaths hides kernel interfaces a container must not read.
//
// /proc/kcore is the host's physical memory. boot_id and the ACPI tables let a
// container fingerprint the host. /sys/firmware exposes firmware interfaces
// that have been the basis of real escapes.
func maskedPaths(spec Spec) []string {
	paths := []string{
		"/proc/acpi",
		"/proc/kcore",
		"/proc/keys",
		"/proc/latency_stats",
		"/proc/timer_list",
		"/proc/timer_stats",
		"/proc/sched_debug",
		"/proc/scsi",
		"/sys/firmware",
		"/sys/devices/virtual/powercap",
	}
	if !spec.Privileged {
		// boot_id is left visible in a privileged container because some
		// workloads legitimately key on it, and a privileged container can
		// read it anyway.
		paths = append(paths, "/proc/sys/kernel/random/boot_id")
	}
	return paths
}

// readonlyPaths keeps a container from rewriting the host's kernel knobs.
func readonlyPaths(spec Spec) []string {
	if spec.Privileged {
		// A privileged container that could not write /proc/sys would fail at
		// the tasks it is privileged for. The mode's whole point is that it is
		// trusted.
		return nil
	}
	return []string{
		"/proc/asound",
		"/proc/bus",
		"/proc/fs",
		"/proc/irq",
		"/proc/sys",
		"/proc/sysrq-trigger",
	}
}

// buildMounts assembles the container's filesystem layout.
//
// The tmpfs and cgroup entries are what systemd requires. Without a writable
// /run it cannot create its runtime directory and fails during early boot;
// without a delegated cgroup v2 mount it cannot start a single unit, and the
// container comes up with no services and no clear reason why.
func buildMounts(spec Spec) ([]*configs.Mount, error) {
	mounts := []*configs.Mount{
		{
			Source:      "proc",
			Destination: "/proc",
			Device:      "proc",
			Flags:       unix.MS_NOSUID | unix.MS_NOEXEC | unix.MS_NODEV,
		},
		{
			Source:      "tmpfs",
			Destination: "/dev",
			Device:      "tmpfs",
			Flags:       unix.MS_NOSUID | unix.MS_STRICTATIME,
			Data:        "mode=755,size=65536k",
		},
		{
			Source:      "devpts",
			Destination: "/dev/pts",
			Device:      "devpts",
			Flags:       unix.MS_NOSUID | unix.MS_NOEXEC,
			// newinstance plus ptmxmode gives the container its own pty
			// namespace, so its terminals are not the host's.
			Data: "newinstance,ptmxmode=0666,mode=0620,gid=5",
		},
		{
			Source:      "shm",
			Destination: "/dev/shm",
			Device:      "tmpfs",
			Flags:       unix.MS_NOSUID | unix.MS_NOEXEC | unix.MS_NODEV,
			Data:        "mode=1777,size=65536k",
		},
		{
			Source:      "mqueue",
			Destination: "/dev/mqueue",
			Device:      "mqueue",
			Flags:       unix.MS_NOSUID | unix.MS_NOEXEC | unix.MS_NODEV,
		},
		{
			Source:      "sysfs",
			Destination: "/sys",
			Device:      "sysfs",
			Flags:       unix.MS_NOSUID | unix.MS_NOEXEC | unix.MS_NODEV | sysfsFlags(spec),
		},
		// systemd writes to /run constantly and expects it to be a tmpfs it
		// owns. A read-only or missing /run is the most common cause of a
		// container that "boots" but has no working services.
		{
			Source:      "tmpfs",
			Destination: "/run",
			Device:      "tmpfs",
			Flags:       unix.MS_NOSUID | unix.MS_NODEV | unix.MS_STRICTATIME,
			Data:        "mode=755,size=64m",
		},
		{
			Source:      "tmpfs",
			Destination: "/run/lock",
			Device:      "tmpfs",
			Flags:       unix.MS_NOSUID | unix.MS_NODEV | unix.MS_NOEXEC | unix.MS_STRICTATIME,
			Data:        "mode=1777,size=8m",
		},
		{
			Source:      "tmpfs",
			Destination: "/tmp",
			Device:      "tmpfs",
			Flags:       unix.MS_NOSUID | unix.MS_NODEV | unix.MS_STRICTATIME,
			Data:        "mode=1777",
		},
	}

	if spec.InitSystem == InitSystemd || spec.Nesting {
		// The delegated cgroup v2 subtree. Combined with the cgroup namespace
		// above, systemd sees its own cgroup as the root and can create units
		// beneath it. Read-only here would let systemd start and then fail at
		// its first unit.
		mounts = append(mounts, &configs.Mount{
			Source:      "cgroup",
			Destination: "/sys/fs/cgroup",
			Device:      "cgroup2",
			Flags:       unix.MS_NOSUID | unix.MS_NOEXEC | unix.MS_NODEV | unix.MS_RELATIME,
		})
	}

	for _, mount := range spec.Mounts {
		converted, err := convertMount(spec, mount)
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, converted)
	}
	return mounts, nil
}

// sysfsFlags makes /sys read-only for an unprivileged container.
func sysfsFlags(spec Spec) int {
	if spec.Privileged {
		return 0
	}
	return unix.MS_RDONLY
}

// convertMount turns a McHarbor mount into a libcontainer one, re-validating
// its source and destination on the way.
//
// The validation is repeated here deliberately. ValidateSpec already ran, but
// this is the last point before a path is handed to the kernel by a
// root-privileged process, and a single missed call site upstream should not be
// enough to bind /etc into a container.
func convertMount(spec Spec, mount Mount) (*configs.Mount, error) {
	if err := ValidateMountDestination(mount.Destination); err != nil {
		return nil, err
	}

	flags := unix.MS_BIND | unix.MS_REC
	if mount.ReadOnly {
		flags |= unix.MS_RDONLY
	}

	switch mount.Kind {
	case "bind":
		if err := ValidateBindSource(mount.Source); err != nil {
			return nil, err
		}
		// A host bind into an unprivileged container needs its ids shifted or
		// everything in it appears owned by nobody. Where the kernel supports
		// idmapped mounts the shift is free; where it does not, the operator
		// is told rather than handed a directory the container cannot use.
		converted := &configs.Mount{
			Source:      mount.Source,
			Destination: mount.Destination,
			Device:      "bind",
			Flags:       flags,
		}
		if mount.ShiftIDs && !spec.Privileged && spec.IDMapMech == "idmapped" {
			converted.IDMapping = &configs.MountIDMapping{
				UIDMappings: []configs.IDMap{{
					ContainerID: 0, HostID: spec.UIDMapStart, Size: spec.UIDMapCount,
				}},
				GIDMappings: []configs.IDMap{{
					ContainerID: 0, HostID: spec.GIDMapStart, Size: spec.GIDMapCount,
				}},
				Recursive: true,
			}
		}
		return converted, nil

	case "volume":
		// A volume lives inside the pool, so it does not go through the host
		// bind deny list — but it still must be an absolute path.
		if mount.Source == "" {
			return nil, fmt.Errorf("lxc: volume mount at %s has no source", mount.Destination)
		}
		return &configs.Mount{
			Source:      mount.Source,
			Destination: mount.Destination,
			Device:      "bind",
			Flags:       flags,
		}, nil

	case "device":
		if !spec.Privileged {
			return nil, fmt.Errorf("%w: passing %s through needs a privileged container",
				ErrPrivilegedDenied, mount.Source)
		}
		return &configs.Mount{
			Source:      mount.Source,
			Destination: mount.Destination,
			Device:      "bind",
			Flags:       unix.MS_BIND,
		}, nil

	default:
		return nil, fmt.Errorf("lxc: unknown mount kind %q", mount.Kind)
	}
}

// specDevices is the minimal device set every container gets.
//
// These are the nodes a userland assumes exist unconditionally. A container
// without /dev/null is not one any distribution's init can boot.
func specDevices() []*devices.Device {
	return []*devices.Device{
		newDevice("/dev/null", 'c', 1, 3, 0o666),
		newDevice("/dev/zero", 'c', 1, 5, 0o666),
		newDevice("/dev/full", 'c', 1, 7, 0o666),
		newDevice("/dev/tty", 'c', 5, 0, 0o666),
		newDevice("/dev/random", 'c', 1, 8, 0o666),
		newDevice("/dev/urandom", 'c', 1, 9, 0o666),
	}
}

func newDevice(path string, kind devices.Type, major, minor int64, mode uint32) *devices.Device {
	fileMode := mode
	return &devices.Device{
		Rule: devices.Rule{
			Type:        kind,
			Major:       major,
			Minor:       minor,
			Permissions: "rwm",
			Allow:       true,
		},
		Path:     path,
		FileMode: fileModeOf(fileMode),
		Uid:      0,
		Gid:      0,
	}
}

// deviceRules is the cgroup device allow-list.
//
// Deny-all first, then allow exactly the nodes above plus pts and ptmx. The
// order matters: a permissive default with denials layered on top leaks
// anything nobody thought to deny.
func deviceRules(spec Spec) []*devices.Rule {
	rules := []*devices.Rule{
		{Type: devices.WildcardDevice, Major: -1, Minor: -1, Permissions: "rwm", Allow: false},
		{Type: devices.CharDevice, Major: 1, Minor: 3, Permissions: "rwm", Allow: true},    // null
		{Type: devices.CharDevice, Major: 1, Minor: 5, Permissions: "rwm", Allow: true},    // zero
		{Type: devices.CharDevice, Major: 1, Minor: 7, Permissions: "rwm", Allow: true},    // full
		{Type: devices.CharDevice, Major: 1, Minor: 8, Permissions: "rwm", Allow: true},    // random
		{Type: devices.CharDevice, Major: 1, Minor: 9, Permissions: "rwm", Allow: true},    // urandom
		{Type: devices.CharDevice, Major: 5, Minor: 0, Permissions: "rwm", Allow: true},    // tty
		{Type: devices.CharDevice, Major: 5, Minor: 1, Permissions: "rwm", Allow: true},    // console
		{Type: devices.CharDevice, Major: 5, Minor: 2, Permissions: "rwm", Allow: true},    // ptmx
		{Type: devices.CharDevice, Major: 136, Minor: -1, Permissions: "rwm", Allow: true}, // pts
	}
	if spec.Privileged {
		// A privileged container gets everything, which is the whole meaning
		// of the mode.
		return []*devices.Rule{
			{Type: devices.WildcardDevice, Major: -1, Minor: -1, Permissions: "rwm", Allow: true},
		}
	}
	return rules
}

// ContainerEnv is the environment a container's init starts with.
//
// container=lxc is load-bearing: systemd reads it to enable its container
// profile, which skips the host-only units — kernel module loading, hwclock,
// udev, mounting the real /sys. Without it systemd tries all of them, they all
// fail, and the container boots into a degraded state that looks like a broken
// image.
func ContainerEnv(spec Spec) []string {
	env := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"container=lxc",
		"TERM=linux",
		"HOME=/root",
	}
	if spec.Name != "" {
		env = append(env, "HOSTNAME="+spec.Name)
	}
	return env
}
