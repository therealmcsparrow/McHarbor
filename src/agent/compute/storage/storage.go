// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package storage owns guest volumes: where they live, how they are created,
// cloned and snapshotted, and how much room is left.
//
// Four backends sit behind one interface. Three of them — zfs, btrfs, lvm —
// are the same three the agent's snapshot driver detection already knows about,
// which is the single largest reuse win in the compute plan: choosing one of
// those means guest snapshots and clones come from mechanisms already exercised
// in production rather than from new code.
//
// The difference between them is not performance trivia, it is the product:
//
//	dir    a qcow2 file. Works anywhere, needs nothing. Snapshots are external
//	       qcow2 overlays, which chain and slow down; clones are a full copy.
//	zfs    a zvol or dataset. Snapshots and clones are metadata operations,
//	       so they are instant regardless of size.
//	btrfs  a subvolume or reflinked file. Same instant story as zfs.
//	lvm    a thin logical volume. Instant snapshots, needs a thin pool.
//
// The setup wizard recommends a CoW backend when one is detected, and says why,
// because "create ten containers from this template" is the difference between
// under a second and several minutes.
package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Backend names, matching the values stored in compute_pools.backend.
const (
	BackendDir   = "dir"
	BackendZFS   = "zfs"
	BackendLVM   = "lvm"
	BackendBtrfs = "btrfs"
)

// Volume shapes. A VM wants a block device; a container wants a directory tree.
const (
	// VolumeDisk is a VM disk: a qcow2 file, a zvol, or a thin LV.
	VolumeDisk = "disk"
	// VolumeRootfs is a container rootfs: a directory, a dataset, or a
	// subvolume.
	VolumeRootfs = "rootfs"
)

var (
	// ErrUnsupported means the backend cannot do this operation at all — a
	// `dir` pool asked for a quota, for instance. The UI states the limitation
	// rather than showing a control that silently does nothing.
	ErrUnsupported = errors.New("storage: operation not supported by this backend")
	// ErrNotFound means the volume or snapshot does not exist.
	ErrNotFound = errors.New("storage: not found")
	// ErrExists means the target already exists.
	ErrExists = errors.New("storage: already exists")
	// ErrShrink is returned for any request to make a volume smaller.
	// Shrinking a filesystem from outside destroys data, and offering it with
	// a warning is not enough, so it is refused here as well as on the
	// backend.
	ErrShrink = errors.New("storage: refusing to shrink a volume")
	// ErrInUse means a template or image still has clones depending on it.
	ErrInUse = errors.New("storage: still in use")
)

// Pool identifies where a volume lives.
type Pool struct {
	ID      string
	Name    string
	Backend string
	// Target is backend-specific: an absolute path for dir and btrfs, a
	// dataset name for zfs, a "vg/thinpool" pair for lvm.
	Target string
}

// Stats is one capacity sample.
type Stats struct {
	CapacityBytes int64
	UsedBytes     int64
	FreeBytes     int64
	// AllocatedBytes is the sum of provisioned volume sizes, which on a thin
	// backend can exceed CapacityBytes. The difference is the overcommit the
	// UI warns about, because a thin pool that fills pauses every guest on it.
	AllocatedBytes int64
	VolumeCount    int
}

// Candidate is a place on the host where a pool of some backend could live.
type Candidate struct {
	Target        string
	Mountpoint    string
	CapacityBytes int64
	FreeBytes     int64
}

// Detection is one backend's availability on this host.
type Detection struct {
	Backend     string
	Available   bool
	Detail      string
	Remediation string
	// Instant reports whether clones and snapshots are metadata operations.
	// This drives the wizard's recommendation.
	Instant    bool
	Candidates []Candidate
}

// CreateVolumeRequest describes a volume to allocate.
type CreateVolumeRequest struct {
	Pool Pool
	// Name is the volume's identity within the pool, normally the guest id
	// plus a slot. It is validated before it reaches any backend.
	Name string
	Kind string // VolumeDisk | VolumeRootfs
	Size int64
	// BackingPath, when set, makes this a copy-on-write volume over an
	// existing image or template: a qcow2 backing file on dir, a clone of a
	// snapshot on zfs and btrfs.
	BackingPath string
	// Format applies to dir volumes only.
	Format string
	// Sparse allocates thin where the backend supports it.
	Sparse bool
}

// Volume is an allocated volume.
type Volume struct {
	Path string
	// Format is what QEMU should be told: qcow2 for a file, raw for a block
	// device.
	Format    string
	SizeBytes int64
	// IsBlock distinguishes a zvol or LV from a file, which changes both the
	// QEMU arguments and how the volume is snapshotted.
	IsBlock bool
}

// VolumeInfo is one volume found in a pool.
//
// It describes what is on the host, not what McHarbor knows about: the caller
// is expected to subtract the volumes it already has records for, because the
// whole point of listing is to find the ones nothing points at any more.
type VolumeInfo struct {
	// Name is the volume's identity within the pool, without the pool's path
	// or dataset prefix.
	Name string
	// Path is what QEMU would be given: an absolute file path, a zvol device
	// node, or an LV device node.
	Path string
	// Format is qcow2 for a file created by the dir driver and raw for a block
	// device or a reflinked btrfs file.
	Format string
	// SizeBytes is the guest-visible size, not the space consumed.
	SizeBytes int64
	// IsBlock distinguishes a zvol or LV from a file.
	IsBlock bool
}

// Snapshot is one point-in-time copy.
type Snapshot struct {
	// Handle is the backend-native name used to revert or delete: a zfs
	// snapshot name, a btrfs subvolume path, an LV name, a qcow2 overlay path.
	Handle    string
	SizeBytes int64
	Mechanism string
}

// Driver is what each backend implements.
//
// Every method takes a context and every implementation honours it, because
// several of these shell out and a hung `zfs` or `lvs` must not wedge the
// agent.
type Driver interface {
	// Name returns the backend identifier.
	Name() string
	// Detect reports whether this backend is usable here and where a pool
	// could live.
	Detect(ctx context.Context) Detection
	// EnsurePool prepares a pool, creating whatever container the backend
	// needs, and returns its initial stats.
	EnsurePool(ctx context.Context, pool Pool) (Stats, error)
	// Stats samples capacity.
	Stats(ctx context.Context, pool Pool) (Stats, error)
	// RemovePool detaches a pool. force removes it even when volumes remain;
	// without it, a pool with volumes is refused so the caller can list what
	// is in the way.
	RemovePool(ctx context.Context, pool Pool, force bool) error
	// Root is the directory a pool's contents can be read through, which is
	// not the same thing as its Target: a zfs pool is named by its dataset and
	// reached through that dataset's mountpoint. A backend whose volumes are
	// block devices rather than files has no such directory and returns
	// ErrUnsupported, which is what the pool browser shows instead of an
	// empty listing that would read as "this pool is empty".
	Root(ctx context.Context, pool Pool) (string, error)

	// CreateVolume allocates a volume.
	CreateVolume(ctx context.Context, req CreateVolumeRequest) (Volume, error)
	// ResizeVolume grows a volume. Shrinking returns ErrShrink.
	ResizeVolume(ctx context.Context, pool Pool, path string, newSize int64) (int64, error)
	// DeleteVolume removes a volume.
	DeleteVolume(ctx context.Context, pool Pool, path string) error
	// ListVolumes reports the VM disks present in a pool, so a caller can
	// offer the ones no guest refers to rather than only ever creating new
	// ones. Container rootfs trees are left out: they are directories, not
	// block devices, and nothing can attach one to a VM.
	ListVolumes(ctx context.Context, pool Pool) ([]VolumeInfo, error)
	// CloneVolume copies a volume. linked asks for copy-on-write, which the
	// backend may decline by returning a full copy — the caller reads
	// Volume.Path and does not assume.
	CloneVolume(ctx context.Context, pool Pool, source, target string, linked bool) (Volume, error)

	// Snapshot takes a point-in-time copy.
	Snapshot(ctx context.Context, pool Pool, path, name string) (Snapshot, error)
	// RevertSnapshot rolls a volume back. Destructive to everything after it.
	RevertSnapshot(ctx context.Context, pool Pool, path string, snap Snapshot) error
	// DeleteSnapshot drops a snapshot.
	DeleteSnapshot(ctx context.Context, pool Pool, path string, snap Snapshot) error

	// SupportsQuota reports whether a per-volume size limit is enforceable.
	// `dir` cannot enforce one, and the UI says so rather than offering a
	// control that does nothing.
	SupportsQuota() bool
	// SetQuota applies a size limit to a rootfs volume.
	SetQuota(ctx context.Context, pool Pool, path string, bytes int64) error
}

// deniedPathPrefixes are directories a pool may never be rooted at.
//
// This list is the agent's own check, deliberately duplicated from the
// backend's. The backend's copy is UX — it stops a typo before it becomes a
// request. This one is security: the agent runs as root and must not accept a
// pool target that would put guest volumes on top of the host's own system
// directories, whatever the control plane sent.
var deniedPathPrefixes = []string{
	"/", "/bin", "/boot", "/dev", "/etc", "/lib", "/lib64", "/proc", "/root",
	"/run", "/sbin", "/sys", "/usr", "/var/lib/docker", "/var/lib/kubelet",
}

// ValidatePoolTarget checks a filesystem pool target.
func ValidatePoolTarget(target string) error {
	if target == "" {
		return errors.New("storage: empty pool target")
	}
	if !filepath.IsAbs(target) {
		return fmt.Errorf("storage: pool target %q must be an absolute path", target)
	}
	clean := filepath.Clean(target)
	if clean != target {
		// A path that does not survive Clean contains "..", a double slash or
		// a trailing slash — all of which mean the string the operator saw is
		// not the path that would be used.
		return fmt.Errorf("storage: pool target %q is not a normalized path (did you mean %q?)",
			target, clean)
	}
	for _, denied := range deniedPathPrefixes {
		if clean == denied {
			return fmt.Errorf("storage: refusing pool target %q: it is a system directory", clean)
		}
		if denied != "/" && strings.HasPrefix(clean, denied+"/") {
			return fmt.Errorf("storage: refusing pool target %q: it is inside the system directory %q",
				clean, denied)
		}
	}
	return nil
}

// ValidateVolumeName rejects anything that could escape its pool or reshape a
// command.
//
// Volume names are built from guest ids the control plane supplies, and they
// end up as path components, ZFS dataset names and LVM volume names. A name
// containing a slash, a leading dash, or a shell metacharacter is refused here
// rather than sanitized, because silently rewriting a name produces a volume
// the database's record no longer points at.
func ValidateVolumeName(name string) error {
	if name == "" {
		return errors.New("storage: empty volume name")
	}
	if len(name) > 128 {
		return fmt.Errorf("storage: volume name %q is too long", name)
	}
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, ".") {
		return fmt.Errorf("storage: volume name %q may not start with %q", name, name[:1])
	}
	// One level of nesting, and only one: a volume is `<guestId>/disk-0`, so a
	// guest's disks and their snapshots sit together in a directory that can be
	// moved, backed up or deleted as one thing. Two levels, a leading slash or
	// a `..` would all be ways out of the pool.
	parts := strings.Split(name, "/")
	if len(parts) > 2 {
		return fmt.Errorf("storage: volume name %q is nested too deeply", name)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("storage: volume name %q has an empty or relative segment", name)
		}
		for _, r := range part {
			switch {
			case r >= 'a' && r <= 'z',
				r >= 'A' && r <= 'Z',
				r >= '0' && r <= '9',
				r == '-', r == '_', r == '.':
			default:
				return fmt.Errorf("storage: volume name %q contains an invalid character %q", name, r)
			}
		}
	}
	return nil
}

// ValidateSnapshotName applies the same rules to snapshot names.
func ValidateSnapshotName(name string) error {
	if err := ValidateVolumeName(name); err != nil {
		return fmt.Errorf("storage: invalid snapshot name: %w", err)
	}
	return nil
}

// VolumePathWithin joins a pool target and a volume name, and verifies the
// result really is inside the pool.
//
// The join alone is not enough: filepath.Join cleans "..", so a name that
// escaped would produce a path outside the pool with no error. The containment
// check afterwards is what makes this safe, and ValidateVolumeName having
// already rejected slashes makes it belt-and-braces rather than the only
// defence.
func VolumePathWithin(poolTarget, name string) (string, error) {
	if err := ValidateVolumeName(name); err != nil {
		return "", err
	}
	base := filepath.Clean(poolTarget)
	full := filepath.Join(base, name)
	if full != base && !strings.HasPrefix(full, base+string(filepath.Separator)) {
		return "", fmt.Errorf("storage: %q would escape pool %q", name, poolTarget)
	}
	return full, nil
}

// BackingRef is how an image should name its backing file.
//
// A sibling is named by its basename, which is what makes a guest's directory
// movable: qcow2 resolves a relative backing path against the image's own
// directory, so the whole folder can be renamed, moved to another array or
// restored on another host and still open. An absolute path — what this used
// to always write — breaks the moment the file moves, which is exactly how a
// storage relocation took every guest down.
func BackingRef(imagePath, backingPath string) string {
	if backingPath == "" {
		return ""
	}
	if filepath.Dir(imagePath) == filepath.Dir(backingPath) {
		return filepath.Base(backingPath)
	}
	return backingPath
}

// Registry holds the drivers available on this host.
type Registry struct {
	drivers map[string]Driver
	order   []string
}

// NewRegistry builds a registry from the drivers this platform compiled in.
func NewRegistry(drivers ...Driver) *Registry {
	r := &Registry{drivers: make(map[string]Driver, len(drivers))}
	for _, d := range drivers {
		if d == nil {
			continue
		}
		r.drivers[d.Name()] = d
		r.order = append(r.order, d.Name())
	}
	return r
}

// Get returns a driver by backend name.
func (r *Registry) Get(backend string) (Driver, error) {
	d, ok := r.drivers[backend]
	if !ok {
		return nil, fmt.Errorf("storage: no driver for backend %q", backend)
	}
	return d, nil
}

// DetectAll probes every driver.
//
// Unavailable backends are returned too, with their remediation, so the UI can
// grey the row and tell the operator what to install rather than hiding an
// option they were looking for.
func (r *Registry) DetectAll(ctx context.Context) []Detection {
	out := make([]Detection, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.drivers[name].Detect(ctx))
	}
	return out
}
