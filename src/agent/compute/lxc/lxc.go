// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package lxc runs system containers through libcontainer.
//
// A system container is not a Docker container. It runs a full init and many
// services, it is a pet rather than cattle, its filesystem is a persistent
// rootfs the operator owns, and it is configured by a package manager and SSH
// rather than by an image and environment variables. The product framing is "a
// VM without the hypervisor overhead", and the implementation has to earn that:
// a container that cannot run systemd is not a system container.
//
// # Why libcontainer and not a daemon
//
// libcontainer is runc's own package — the code that creates the namespaces,
// applies the cgroups and pivots the root. Using it as a library means no
// second daemon, no second database that can drift from McHarbor's, and no CLI
// whose stderr has to be scraped. runc is a runtime component rather than a
// management product, so it is a dependency McHarbor controls and could vendor,
// not a control plane McHarbor would be subordinate to.
//
// # The CGO consequence
//
// libcontainer/nsenter contains a C constructor that runs before the Go runtime
// starts, because the setns/clone dance for user namespaces cannot be done
// safely from Go once the runtime has spawned threads. Importing it requires
// CGO_ENABLED=1 for the agent binary. This does not touch the backend
// container, which is a separate module and stays pure Go — the
// modernc.org/sqlite constraint is unaffected.
//
// See init_linux.go for the re-exec requirement, which is the single most
// likely thing to get wrong in this package.
package lxc

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// DefaultRuntimeRoot is where per-container state and rootfs trees live when
// McHarbor names no location of its own.
const DefaultRuntimeRoot = "/var/lib/mcharbor/compute/ct"

// runtimeRoot holds the live value behind RuntimeRoot. Read from whichever
// goroutine is driving a container, so it is atomic rather than a plain
// package variable.
var runtimeRoot atomic.Value

func init() { runtimeRoot.Store(DefaultRuntimeRoot) }

// RuntimeRoot reports where per-container state and rootfs trees currently live.
func RuntimeRoot() string {
	root, _ := runtimeRoot.Load().(string)
	if root == "" {
		return DefaultRuntimeRoot
	}
	return root
}

// SetRuntimeRoot moves where container rootfs trees are kept, so an operator can
// put them on shared storage. Callers validate the path first.
func SetRuntimeRoot(root string) {
	if root == "" {
		root = DefaultRuntimeRoot
	}
	runtimeRoot.Store(root)
}

// StateRoot is libcontainer's own state directory.
const StateRoot = "/run/mcharbor/ct"

// Init systems a container's rootfs may run.
const (
	InitSystemd = "systemd"
	InitOpenRC  = "openrc"
	InitNone    = "none"
)

// Container states, mirroring the VM vocabulary so the two guest kinds read the
// same way in the UI.
const (
	StateDefined  = "defined"
	StateRunning  = "running"
	StateFrozen   = "frozen"
	StateStopping = "stopping"
	StateStopped  = "stopped"
	StateCrashed  = "crashed"
	StateUnknown  = "unknown"
)

// IDRangeSize is how many uids and gids each container gets.
//
// 65536 is the full uid_t range a distribution expects to exist inside a
// container. Allocating less produces a rootfs whose high-numbered system
// accounts map to nothing, which fails in ways that never point at id mapping.
const IDRangeSize = 65536

// SignalSystemdShutdown is SIGRTMIN+3, the signal systemd treats as a clean shutdown request.
//
// This is not a detail. SIGTERM to systemd's PID 1 does not shut a container
// down cleanly — systemd ignores it — so the container sits there until the
// stop timeout expires and is then killed. That is exactly the "container takes
// 90 seconds to stop" complaint, and it is entirely avoidable by sending the
// signal systemd actually listens for. Go has no SIGRTMIN constant because the
// value is platform-specific; on Linux SIGRTMIN is 34.
const SignalSystemdShutdown = 37 // SIGRTMIN+3

var (
	// ErrNotFound means this host has no such container.
	ErrNotFound = errors.New("lxc: not found")
	// ErrNoCgroupV2 means the host is on the v1 hierarchy, which is refused
	// rather than half-supported.
	ErrNoCgroupV2 = errors.New("lxc: system containers require the cgroup v2 unified hierarchy")
	// ErrNoSubuid means there is not enough subordinate id space to allocate a
	// container a range of its own.
	ErrNoSubuid = errors.New("lxc: no subordinate uid/gid space available")
	// ErrPrivilegedDenied means a privileged container was requested without
	// the permission that gates it.
	ErrPrivilegedDenied = errors.New("lxc: privileged containers require ct.privileged")
	// ErrBindPathDenied means a host bind mount pointed somewhere it may not.
	ErrBindPathDenied = errors.New("lxc: refusing to bind this host path into a container")
)

// Spec is one container, mirroring the protocol's CTSpec.
type Spec struct {
	ID          string
	Name        string
	RootfsPath  string
	PoolBackend string

	InitSystem string
	InitArgv   []string

	// Privileged removes the user namespace entirely. Root inside a privileged
	// container is root on the host in every way that matters.
	Privileged bool
	// Nesting allows a container to run its own container runtime, which needs
	// a writable cgroup subtree and relaxed masking.
	Nesting bool

	UIDMapStart int64
	UIDMapCount int64
	GIDMapStart int64
	GIDMapCount int64
	// IDMapMech is "idmapped" when the kernel supports idmapped mounts and
	// "chown" when the rootfs had to be shifted on unpack. It changes clone
	// speed materially, so the UI states which is in use.
	IDMapMech string

	Limits Limits
	NICs   []NIC
	Mounts []Mount
}

// Limits are cgroup v2 values.
type Limits struct {
	CPUMax          string
	CPUWeight       uint64
	CPUSet          string
	MemoryMaxBytes  int64
	MemoryHighBytes int64
	SwapMaxBytes    int64
	PidsMax         int64
	IOMax           string
}

// NIC is one container interface: a veth pair onto a bridge, or a macvlan
// child stacked straight on a host adapter.
type NIC struct {
	ID      string
	Slot    int
	Bridge  string
	VLANTag int
	// AttachKind is empty or "bridge" for the veth pair, "macvlan" for a
	// direct child of ParentLink.
	AttachKind string
	// ParentLink is the host interface a direct NIC stacks on, and is the
	// network's uplink rather than its VLAN sub-interface: the manager resolves
	// VLANTag itself when the container starts.
	ParentLink string
	// LinkMode is the direct sub-mode — bridge | vepa | private | passthru.
	LinkMode    string
	GuestName   string
	MAC         string
	VethHost    string
	VethPeer    string
	AddressMode string
	StaticIP    string
	StaticGW    string
	StaticDNS   []string
	// StaticIPv6 and StaticGW6 are applied beside the IPv4 pair whatever
	// AddressMode says, since a DHCPv4 guest can still want a fixed v6 address.
	StaticIPv6 string
	StaticGW6  string
}

// Mount is one mount into the container.
type Mount struct {
	ID          string
	Kind        string // bind | volume | device
	Source      string
	Destination string
	ReadOnly    bool
	ShiftIDs    bool
}

// Paths are one container's on-disk locations.
type Paths struct {
	Dir         string
	Rootfs      string
	ConfigJSON  string
	ConsoleSock string
	StateDir    string
}

// PathsFor builds a container's paths.
func PathsFor(ctID string) Paths {
	dir := filepath.Join(RuntimeRoot(), ctID)
	return Paths{
		Dir:         dir,
		Rootfs:      filepath.Join(dir, "rootfs"),
		ConfigJSON:  filepath.Join(dir, "config.json"),
		ConsoleSock: filepath.Join(dir, "console.sock"),
		StateDir:    filepath.Join(StateRoot, ctID),
	}
}

// bindDenyPrefixes are host paths that may never be bind-mounted into a
// container.
//
// A bind of any of these hands the container the ability to rewrite the host:
// /etc gives it the host's passwd and sudoers, /boot its kernel, the Docker
// socket its whole container fleet. The API separates these from pool volumes
// with a stronger permission, and this is the agent's own independent refusal —
// the backend's check is UX, this one is the boundary.
var bindDenyPrefixes = []string{
	"/boot", "/dev", "/etc", "/proc", "/sys",
	"/var/lib/docker", "/var/run/docker.sock", "/run/docker.sock",
	"/var/lib/mcharbor",
}

// ValidateBindSource refuses a dangerous host bind path.
func ValidateBindSource(source string) error {
	if source == "" {
		return errors.New("lxc: empty bind source")
	}
	if !filepath.IsAbs(source) {
		return fmt.Errorf("lxc: bind source %q must be an absolute path", source)
	}
	clean := filepath.Clean(source)
	if clean != source {
		return fmt.Errorf("lxc: bind source %q is not a normalized path", source)
	}
	if clean == "/" {
		return fmt.Errorf("%w: %q would give the container the whole host filesystem",
			ErrBindPathDenied, clean)
	}
	for _, denied := range bindDenyPrefixes {
		if clean == denied || strings.HasPrefix(clean, denied+"/") {
			return fmt.Errorf("%w: %q is inside %q", ErrBindPathDenied, clean, denied)
		}
	}
	return nil
}

// ValidateMountDestination refuses a destination that would break the
// container's own init.
func ValidateMountDestination(dest string) error {
	if dest == "" {
		return errors.New("lxc: empty mount destination")
	}
	if !filepath.IsAbs(dest) {
		return fmt.Errorf("lxc: mount destination %q must be absolute", dest)
	}
	clean := filepath.Clean(dest)
	if clean != dest {
		return fmt.Errorf("lxc: mount destination %q is not a normalized path", dest)
	}
	// Mounting over these replaces the container's own filesystem layout, and
	// the container then fails to boot with an error that blames init.
	for _, reserved := range []string{"/", "/proc", "/sys", "/dev"} {
		if clean == reserved {
			return fmt.Errorf("lxc: %q is managed by the container runtime and cannot be "+
				"a mount destination", clean)
		}
	}
	return nil
}

// InitArgvFor returns the command a container's init system needs.
func InitArgvFor(spec Spec) []string {
	if len(spec.InitArgv) > 0 {
		return spec.InitArgv
	}
	switch spec.InitSystem {
	case InitSystemd:
		// /sbin/init is a symlink to systemd on every distribution that uses
		// it, and following the symlink ourselves would break on the ones that
		// do it differently.
		return []string{"/sbin/init"}
	case InitOpenRC:
		return []string{"/sbin/init"}
	default:
		return []string{"/bin/sh"}
	}
}

// StopSignalFor returns the signal that shuts a container's init down cleanly.
func StopSignalFor(initSystem string) int {
	if initSystem == InitSystemd {
		return SignalSystemdShutdown
	}
	return 15 // SIGTERM
}

// ValidateSpec checks a container specification before anything is created.
func ValidateSpec(spec Spec) error {
	if spec.ID == "" {
		return errors.New("lxc: container has no id")
	}
	if spec.RootfsPath == "" {
		return fmt.Errorf("lxc: %s has no rootfs path", spec.Name)
	}
	if !spec.Privileged {
		if spec.UIDMapCount < IDRangeSize || spec.GIDMapCount < IDRangeSize {
			return fmt.Errorf("%w: %s needs a block of %d uids and gids",
				ErrNoSubuid, spec.Name, IDRangeSize)
		}
	}
	for _, mount := range spec.Mounts {
		if err := ValidateMountDestination(mount.Destination); err != nil {
			return err
		}
		if mount.Kind == "bind" {
			if err := ValidateBindSource(mount.Source); err != nil {
				return err
			}
		}
	}
	return nil
}
