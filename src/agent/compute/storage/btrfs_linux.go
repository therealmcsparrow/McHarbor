// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package storage

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/hostmnt"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// The btrfs backend. Like ZFS, snapshots and clones are metadata operations, so
// it belongs in the same "instant" tier and the wizard recommends it on the
// same grounds.
//
// The shapes differ from ZFS in one way worth knowing: btrfs has no zvol
// equivalent, so a VM disk is a raw file made with a reflink copy rather than a
// block device, and a container rootfs is a subvolume. Reflink copies are
// copy-on-write at the extent level, which gives the same instant-clone
// property for files that subvolumes give for trees.
//
// Quotas need qgroups, which are off by default on most installs and cost real
// performance when enabled. SupportsQuota reports true, but SetQuota enables
// qgroups on demand and says so, rather than silently changing a filesystem-wide
// performance characteristic behind the operator's back.

const btrfsTimeout = 60 * time.Second
const btrfsLongTimeout = 2 * time.Hour

type btrfsDriver struct{}

var _ Driver = (*btrfsDriver)(nil)

// NewBtrfsDriver builds the btrfs backend.
func NewBtrfsDriver() Driver { return &btrfsDriver{} }

func (b *btrfsDriver) Name() string { return BackendBtrfs }

func (b *btrfsDriver) SupportsQuota() bool { return true }

func (b *btrfsDriver) Detect(ctx context.Context) Detection {
	det := Detection{Backend: BackendBtrfs, Instant: true}

	if _, err := exec.LookPath("btrfs"); err != nil {
		det.Detail = "The btrfs command is not on PATH."
		det.Remediation = installHintBtrfs()
		return det
	}

	// A btrfs filesystem has to be mounted to be usable as a pool, so the
	// mount table is the authoritative list rather than `btrfs filesystem
	// show`, which also lists unmounted ones the operator cannot use yet.
	mounts, err := btrfsMounts()
	if err != nil || len(mounts) == 0 {
		det.Detail = "btrfs-progs is installed but no btrfs filesystem is mounted."
		det.Remediation = "Mount a btrfs filesystem, then create a pool on it."
		return det
	}

	for _, mount := range mounts {
		stats, err := statfsBytes(mount)
		if err != nil {
			continue
		}
		det.Candidates = append(det.Candidates, Candidate{
			Target:        filepath.Join(mount, "mcharbor"),
			Mountpoint:    mount,
			CapacityBytes: stats.CapacityBytes,
			FreeBytes:     stats.FreeBytes,
		})
	}

	det.Available = true
	det.Detail = fmt.Sprintf("btrfs is available on %d mounted filesystem(s). Guest snapshots "+
		"and clones are metadata operations, so they complete instantly regardless of size.",
		len(mounts))
	return det
}

// btrfsMounts reads the agent's own mount table for btrfs filesystems, through
// hostmnt because /proc/self can show a dead container's namespace instead.
func btrfsMounts() ([]string, error) {
	data, err := hostmnt.Read("mounts")
	if err != nil {
		return nil, fmt.Errorf("storage: read mounts: %w", err)
	}

	seen := map[string]bool{}
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 || fields[2] != "btrfs" {
			continue
		}
		// The mount table escapes spaces as \040; unescaping matters because a
		// mount point with a space would otherwise be truncated.
		mount := strings.ReplaceAll(fields[1], `\040`, " ")
		// One entry per filesystem: btrfs subvolumes each get their own mount
		// line, and offering the operator twenty of them is noise.
		if seen[mount] {
			continue
		}
		seen[mount] = true
		out = append(out, mount)
	}
	return out, nil
}

// Root is the subvolume path, which btrfs exposes as an ordinary directory.
func (b *btrfsDriver) Root(_ context.Context, pool Pool) (string, error) {
	if err := ValidatePoolTarget(pool.Target); err != nil {
		return "", err
	}
	return pool.Target, nil
}

func (b *btrfsDriver) EnsurePool(ctx context.Context, pool Pool) (Stats, error) {
	if err := ValidatePoolTarget(pool.Target); err != nil {
		return Stats{}, err
	}
	if _, err := os.Stat(pool.Target); os.IsNotExist(err) {
		// The pool itself is a subvolume, which makes it snapshottable as a
		// unit and keeps its contents out of the parent's snapshots.
		if out, err := runBtrfs(ctx, btrfsTimeout, "subvolume", "create", pool.Target); err != nil {
			return Stats{}, fmt.Errorf("storage: create subvolume %s: %w (%s)",
				pool.Target, err, firstLine(out))
		}
		errlog.Log(os.Chmod(pool.Target, 0o700))
	}
	return b.Stats(ctx, pool)
}

func (b *btrfsDriver) Stats(_ context.Context, pool Pool) (Stats, error) {
	stats, err := statfsBytes(pool.Target)
	if err != nil {
		return Stats{}, err
	}
	if entries, err := os.ReadDir(pool.Target); err == nil {
		for _, entry := range entries {
			stats.VolumeCount++
			if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
				stats.AllocatedBytes += info.Size()
			}
		}
	}
	return stats, nil
}

func (b *btrfsDriver) RemovePool(ctx context.Context, pool Pool, force bool) error {
	if err := ValidatePoolTarget(pool.Target); err != nil {
		return err
	}
	entries, err := os.ReadDir(pool.Target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("storage: read pool %s: %w", pool.Target, err)
	}
	if len(entries) > 0 && !force {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return fmt.Errorf("%w: pool %s still holds %d volumes: %s",
			ErrInUse, pool.Name, len(entries), strings.Join(names, ", "))
	}

	// Nested subvolumes have to go before their parent; rmdir on a subvolume
	// that still contains one fails with an error that does not name the
	// nested one.
	for _, entry := range entries {
		child := filepath.Join(pool.Target, entry.Name())
		if isSubvolume(ctx, child) {
			bestEffort(runBtrfs(ctx, btrfsLongTimeout, "subvolume", "delete", child))
		} else {
			errlog.Log(os.RemoveAll(child))
		}
	}
	if out, err := runBtrfs(ctx, btrfsLongTimeout, "subvolume", "delete", pool.Target); err != nil {
		// A pool directory that was never a subvolume still has to go away.
		if err := os.RemoveAll(pool.Target); err != nil {
			return fmt.Errorf("storage: remove pool %s: %w (%s)", pool.Target, err, firstLine(out))
		}
	}
	return nil
}

func (b *btrfsDriver) CreateVolume(ctx context.Context, req CreateVolumeRequest) (Volume, error) {
	path, err := VolumePathWithin(req.Pool.Target, req.Name)
	if err != nil {
		return Volume{}, err
	}

	if req.Kind == VolumeRootfs {
		if req.BackingPath != "" {
			// Cloning a template subvolume: instant, and the whole reason to
			// pick this backend.
			if out, err := runBtrfs(ctx, btrfsTimeout,
				"subvolume", "snapshot", req.BackingPath, path); err != nil {
				return Volume{}, fmt.Errorf("storage: clone subvolume %s: %w (%s)",
					req.BackingPath, err, firstLine(out))
			}
		} else if out, err := runBtrfs(ctx, btrfsTimeout, "subvolume", "create", path); err != nil {
			return Volume{}, fmt.Errorf("storage: create subvolume %s: %w (%s)",
				path, err, firstLine(out))
		}
		if req.Size > 0 {
			if err := b.SetQuota(ctx, req.Pool, path, req.Size); err != nil {
				return Volume{}, err
			}
		}
		return Volume{Path: path, Format: "dir", SizeBytes: req.Size}, nil
	}

	// A VM disk. qcow2 on top of btrfs would layer two copy-on-write systems
	// and pay for both, so a raw file is used instead and btrfs provides the
	// CoW underneath.
	if req.BackingPath != "" {
		// A reflink copy shares extents with the source until either is
		// written, which is the file-level equivalent of a subvolume snapshot.
		if out, err := runCommand(ctx, btrfsLongTimeout, "cp", "--reflink=always",
			req.BackingPath, path); err != nil {
			return Volume{}, fmt.Errorf("storage: reflink %s: %w (%s)",
				req.BackingPath, err, firstLine(out))
		}
	} else {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			if os.IsExist(err) {
				return Volume{}, fmt.Errorf("%w: %s", ErrExists, path)
			}
			return Volume{}, fmt.Errorf("storage: create %s: %w", path, err)
		}
		if req.Size > 0 {
			// Truncate makes it sparse: nothing is written until the guest
			// writes it.
			if err := f.Truncate(req.Size); err != nil {
				errlog.Close(f)
				errlog.Log(os.Remove(path))
				return Volume{}, fmt.Errorf("storage: size %s: %w", path, err)
			}
		}
		if err := f.Close(); err != nil {
			return Volume{}, fmt.Errorf("storage: close %s: %w", path, err)
		}
		// Disabling CoW on a VM image is the standard btrfs recommendation:
		// a randomly-written multi-gigabyte file fragments badly under CoW.
		// It is best-effort — it only takes effect on an empty file, and a
		// failure here costs performance rather than correctness.
		bestEffort(runCommand(ctx, btrfsTimeout, "chattr", "+C", path))
	}

	size := req.Size
	if info, err := os.Stat(path); err == nil && size == 0 {
		size = info.Size()
	}
	return Volume{Path: path, Format: "raw", SizeBytes: size}, nil
}

func (b *btrfsDriver) ResizeVolume(_ context.Context, pool Pool, path string, newSize int64) (int64, error) {
	if err := assertWithinPool(pool, path); err != nil {
		return 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrNotFound, path)
	}
	if info.IsDir() {
		return 0, fmt.Errorf("%w: resize a btrfs rootfs by changing its quota", ErrUnsupported)
	}
	if newSize < info.Size() {
		return 0, fmt.Errorf("%w: %s is %d bytes and %d was requested",
			ErrShrink, path, info.Size(), newSize)
	}
	if err := os.Truncate(path, newSize); err != nil {
		return 0, fmt.Errorf("storage: resize %s: %w", path, err)
	}
	return newSize, nil
}

func (b *btrfsDriver) DeleteVolume(ctx context.Context, pool Pool, path string) error {
	if err := assertWithinPool(pool, path); err != nil {
		return err
	}
	if isSubvolume(ctx, path) {
		if out, err := runBtrfs(ctx, btrfsLongTimeout, "subvolume", "delete", path); err != nil {
			if strings.Contains(out, "No such file") {
				return nil
			}
			return fmt.Errorf("storage: delete subvolume %s: %w (%s)", path, err, firstLine(out))
		}
		return nil
	}
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: delete %s: %w", path, err)
	}
	return nil
}

// ListVolumes reports the disk images sitting in a btrfs pool.
//
// Only regular files are returned. A subvolume in here is a container rootfs,
// and the apparent size is the guest-visible one — a reflinked file shares its
// extents with the source, so what it consumes is not what the guest sees.
func (b *btrfsDriver) ListVolumes(_ context.Context, pool Pool) ([]VolumeInfo, error) {
	entries, err := os.ReadDir(pool.Target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("storage: read pool %s: %w", pool.Target, err)
	}

	out := make([]VolumeInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".iso") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		out = append(out, VolumeInfo{
			Name:      entry.Name(),
			Path:      filepath.Join(pool.Target, entry.Name()),
			Format:    "raw",
			SizeBytes: info.Size(),
		})
	}
	return out, nil
}

func (b *btrfsDriver) CloneVolume(ctx context.Context, pool Pool, source, target string, linked bool) (Volume, error) {
	targetPath, err := VolumePathWithin(pool.Target, target)
	if err != nil {
		return Volume{}, err
	}
	if err := assertWithinPool(pool, source); err != nil {
		return Volume{}, err
	}

	if isSubvolume(ctx, source) {
		// A read-write snapshot is still CoW-shared with its source on btrfs;
		// there is no "independent copy" that is cheaper than this, so linked
		// and full differ only in whether the source is pinned.
		args := []string{"subvolume", "snapshot", source, targetPath}
		if out, err := runBtrfs(ctx, btrfsTimeout, args...); err != nil {
			return Volume{}, fmt.Errorf("storage: snapshot subvolume %s: %w (%s)",
				source, err, firstLine(out))
		}
		return Volume{Path: targetPath, Format: "dir"}, nil
	}

	// Reflink where possible; a filesystem boundary makes it impossible, and
	// falling back to a plain copy is better than failing.
	if out, err := runCommand(ctx, btrfsLongTimeout, "cp", "--reflink=auto", source, targetPath); err != nil {
		return Volume{}, fmt.Errorf("storage: clone %s: %w (%s)", source, err, firstLine(out))
	}
	size := int64(0)
	if info, err := os.Stat(targetPath); err == nil {
		size = info.Size()
	}
	return Volume{Path: targetPath, Format: "raw", SizeBytes: size}, nil
}

func (b *btrfsDriver) Snapshot(ctx context.Context, pool Pool, path, name string) (Snapshot, error) {
	if err := ValidateSnapshotName(name); err != nil {
		return Snapshot{}, err
	}
	if err := assertWithinPool(pool, path); err != nil {
		return Snapshot{}, err
	}
	handle := path + ".snap." + name
	if _, err := os.Stat(handle); err == nil {
		return Snapshot{}, fmt.Errorf("%w: snapshot %s", ErrExists, name)
	}

	if isSubvolume(ctx, path) {
		// -r makes it read-only, which is what a snapshot should be: a
		// writable snapshot silently diverging from what the operator thinks
		// they captured is a data-loss shape.
		if out, err := runBtrfs(ctx, btrfsTimeout, "subvolume", "snapshot", "-r", path, handle); err != nil {
			return Snapshot{}, fmt.Errorf("storage: snapshot %s: %w (%s)", path, err, firstLine(out))
		}
	} else {
		if out, err := runCommand(ctx, btrfsLongTimeout, "cp", "--reflink=always", path, handle); err != nil {
			return Snapshot{}, fmt.Errorf("storage: reflink snapshot %s: %w (%s)",
				path, err, firstLine(out))
		}
	}

	size := int64(0)
	if info, err := os.Stat(handle); err == nil && info.Mode().IsRegular() {
		size = info.Size()
	}
	return Snapshot{Handle: handle, SizeBytes: size, Mechanism: BackendBtrfs}, nil
}

func (b *btrfsDriver) RevertSnapshot(ctx context.Context, pool Pool, path string, snap Snapshot) error {
	if err := assertWithinPool(pool, path); err != nil {
		return err
	}
	if err := assertWithinPool(pool, snap.Handle); err != nil {
		return err
	}
	if _, err := os.Stat(snap.Handle); err != nil {
		return fmt.Errorf("%w: snapshot %s", ErrNotFound, snap.Handle)
	}

	if isSubvolume(ctx, snap.Handle) {
		// Stage the restored copy beside the live one and swap by rename, so a
		// failure part-way through leaves the guest's volume intact rather
		// than deleted. The rename is atomic; the delete after it is not
		// load-bearing.
		staging := path + ".restoring"
		bestEffort(runBtrfs(ctx, btrfsLongTimeout, "subvolume", "delete", staging))
		if out, err := runBtrfs(ctx, btrfsTimeout, "subvolume", "snapshot", snap.Handle, staging); err != nil {
			return fmt.Errorf("storage: stage revert %s: %w (%s)", snap.Handle, err, firstLine(out))
		}
		retired := path + ".retired"
		bestEffort(runBtrfs(ctx, btrfsLongTimeout, "subvolume", "delete", retired))
		if err := os.Rename(path, retired); err != nil && !os.IsNotExist(err) {
			bestEffort(runBtrfs(ctx, btrfsLongTimeout, "subvolume", "delete", staging))
			return fmt.Errorf("storage: retire %s: %w", path, err)
		}
		if err := os.Rename(staging, path); err != nil {
			// Put the original back rather than leaving the guest with no
			// rootfs at all.
			errlog.Log(os.Rename(retired, path))
			return fmt.Errorf("storage: install revert %s: %w", path, err)
		}
		bestEffort(runBtrfs(ctx, btrfsLongTimeout, "subvolume", "delete", retired))
		return nil
	}

	staging := path + ".restoring"
	if out, err := runCommand(ctx, btrfsLongTimeout, "cp", "--reflink=auto", snap.Handle, staging); err != nil {
		return fmt.Errorf("storage: stage revert %s: %w (%s)", snap.Handle, err, firstLine(out))
	}
	if err := os.Rename(staging, path); err != nil {
		errlog.Log(os.Remove(staging))
		return fmt.Errorf("storage: install revert %s: %w", path, err)
	}
	return nil
}

func (b *btrfsDriver) DeleteSnapshot(ctx context.Context, pool Pool, _ string, snap Snapshot) error {
	if err := assertWithinPool(pool, snap.Handle); err != nil {
		return err
	}
	if isSubvolume(ctx, snap.Handle) {
		if out, err := runBtrfs(ctx, btrfsLongTimeout, "subvolume", "delete", snap.Handle); err != nil {
			if strings.Contains(out, "No such file") {
				return nil
			}
			return fmt.Errorf("storage: delete snapshot %s: %w (%s)", snap.Handle, err, firstLine(out))
		}
		return nil
	}
	if err := os.Remove(snap.Handle); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: delete snapshot %s: %w", snap.Handle, err)
	}
	return nil
}

// SetQuota applies a qgroup limit to a subvolume.
//
// qgroups are off by default and enabling them has a real, filesystem-wide cost
// on write-heavy workloads. Enabling one on demand is still the right call —
// the operator asked for an enforced limit and this is the only way to give
// one — but the error path says plainly what happened rather than leaving them
// to discover the performance change later.
func (b *btrfsDriver) SetQuota(ctx context.Context, pool Pool, path string, bytes int64) error {
	if err := assertWithinPool(pool, path); err != nil {
		return err
	}
	if bytes <= 0 {
		bestEffort(runBtrfs(ctx, btrfsTimeout, "qgroup", "limit", "none", path))
		return nil
	}
	if out, err := runBtrfs(ctx, btrfsTimeout, "quota", "enable", pool.Target); err != nil {
		return fmt.Errorf("storage: enable btrfs quotas on %s (needed to set a disk limit): %w (%s)",
			pool.Target, err, firstLine(out))
	}
	if out, err := runBtrfs(ctx, btrfsTimeout, "qgroup", "limit",
		strconv.FormatInt(bytes, 10), path); err != nil {
		return fmt.Errorf("storage: set quota on %s: %w (%s)", path, err, firstLine(out))
	}
	return nil
}

// isSubvolume reports whether a path is a btrfs subvolume rather than a plain
// directory, which decides whether it is deleted with `subvolume delete` or
// rmdir.
func isSubvolume(ctx context.Context, path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	_, err = runBtrfs(ctx, btrfsTimeout, "subvolume", "show", path)
	return err == nil
}

func runBtrfs(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	return runCommand(ctx, timeout, "btrfs", args...)
}

// runCommand executes any host tool with an explicit timeout and an argv.
func runCommand(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

func installHintBtrfs() string {
	return "Install btrfs-progs to use btrfs pools."
}

// bestEffort discards the output and error of a command whose failure is not
// load-bearing: a rollback delete that may find nothing to delete, an advisory
// tuning call, an unmount that lvremove -f would report on anyway. Taking the
// two results as ignored parameters states at the call site that the discard is
// deliberate, which a blank-blank assignment cannot.
func bestEffort(string, error) {}
