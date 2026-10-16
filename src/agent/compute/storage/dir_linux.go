// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"golang.org/x/sys/unix"
)

// The dir backend: qcow2 files for VM disks, plain directory trees for
// container rootfs. It needs nothing beyond qemu-img, works on any filesystem,
// and is therefore the honest default — but it is also the worst experience,
// and the UI says so rather than letting the operator discover it later.
//
// Two consequences the operator should meet in the create dialog, not in
// production:
//
//   - Snapshots are external qcow2 overlays. Each one adds a link to a read
//     chain, and a deep chain is the classic way a homelab VM becomes
//     mysteriously slow. Chain depth is capped and surfaced.
//   - Clones are copies. A "linked" clone uses a qcow2 backing file and is
//     fast, but a full clone reads and writes the whole disk, and a container
//     clone is a directory copy with no CoW at all.

// qemuImgTimeout bounds a short qemu-img query. Creation and conversion get
// their own, much longer budget below — a 500 GB allocate is not a 30-second
// operation and treating it as one would fail every large disk.
const qemuImgTimeout = 30 * time.Second

// qemuImgLongTimeout bounds allocation and conversion.
const qemuImgLongTimeout = 6 * time.Hour

type dirDriver struct{}

var _ Driver = (*dirDriver)(nil)

// NewDirDriver builds the filesystem backend.
func NewDirDriver() Driver { return &dirDriver{} }

func (d *dirDriver) Name() string { return BackendDir }

// SupportsQuota is false: a plain directory on a shared filesystem has no
// enforceable per-volume limit. The UI shows the container disk-quota control
// as unsupported-with-explanation on this backend rather than accepting a value
// it cannot honour.
func (d *dirDriver) SupportsQuota() bool { return false }

func (d *dirDriver) SetQuota(_ context.Context, _ Pool, _ string, _ int64) error {
	return fmt.Errorf("%w: dir pools cannot enforce a disk quota", ErrUnsupported)
}

func (d *dirDriver) Detect(ctx context.Context) Detection {
	det := Detection{Backend: BackendDir, Instant: false}

	// Available whether or not qemu-img is installed. Creating a dir pool is a
	// mkdir; qemu-img is a volume-allocation dependency that does not apply
	// yet, and reporting the backend unavailable for it leaves a host with no
	// reachable pool at all, which is worse than a pool that cannot yet hold
	// a disk.
	det.Available = true
	det.Detail = "Directory pools work on any filesystem. Snapshots use chained qcow2 " +
		"overlays and clones are copies, so snapshot and clone operations are slower " +
		"than on a copy-on-write backend."

	if _, err := exec.LookPath("qemu-img"); err != nil {
		det.Detail = "Directory pools work on any filesystem. qemu-img is not on PATH " +
			"on this host, so the pool can be created but no disk can be allocated in " +
			"it until the package is installed."
		det.Remediation = "Install the qemu-utils (Debian/Ubuntu) or qemu-img (RHEL/Fedora) package."
	}

	// Offer the agent's own data root as the obvious candidate, plus anything
	// already mounted with room on it, so the wizard can propose a location
	// instead of asking the operator to invent one.
	for _, candidate := range []string{"/var/lib/mcharbor/compute", "/var/lib/mcharbor"} {
		if stats, err := statfsBytes(filepath.Dir(candidate)); err == nil {
			det.Candidates = append(det.Candidates, Candidate{
				Target:        candidate,
				Mountpoint:    filepath.Dir(candidate),
				CapacityBytes: stats.CapacityBytes,
				FreeBytes:     stats.FreeBytes,
			})
			break
		}
	}
	return det
}

func (d *dirDriver) EnsurePool(ctx context.Context, pool Pool) (Stats, error) {
	if err := ValidatePoolTarget(pool.Target); err != nil {
		return Stats{}, err
	}
	// 0700: guest disks and container rootfs trees are readable only by the
	// agent's user. A cloud-init seed image lives in here and, while it never
	// carries a plaintext secret, a world-readable pool would still expose
	// every guest's disk to any local user.
	if err := os.MkdirAll(pool.Target, 0o700); err != nil {
		return Stats{}, fmt.Errorf("storage: create pool directory %s: %w", pool.Target, err)
	}
	return d.Stats(ctx, pool)
}

func (d *dirDriver) Stats(_ context.Context, pool Pool) (Stats, error) {
	stats, err := statfsBytes(pool.Target)
	if err != nil {
		return Stats{}, err
	}
	// Walking the pool for allocated size is bounded to one level: pools hold
	// per-guest entries, and a recursive walk of a pool with terabytes of
	// container rootfs would take long enough to matter on a sampling path.
	entries, err := os.ReadDir(pool.Target)
	if err == nil {
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			stats.VolumeCount++
			stats.AllocatedBytes += info.Size()
		}
	}
	return stats, nil
}

// Root is the pool directory itself: a `dir` pool is a path and nothing else.
func (d *dirDriver) Root(_ context.Context, pool Pool) (string, error) {
	if err := ValidatePoolTarget(pool.Target); err != nil {
		return "", err
	}
	return pool.Target, nil
}

func (d *dirDriver) RemovePool(_ context.Context, pool Pool, force bool) error {
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
		// Naming what is in the way turns a refusal into something the
		// operator can act on.
		return fmt.Errorf("%w: pool %s still holds %d volumes: %s",
			ErrInUse, pool.Name, len(entries), strings.Join(names, ", "))
	}
	// Only the pool directory itself is removed, never its parents, and only
	// after ValidatePoolTarget has refused every system path.
	if err := os.RemoveAll(pool.Target); err != nil {
		return fmt.Errorf("storage: remove pool %s: %w", pool.Target, err)
	}
	return nil
}

func (d *dirDriver) CreateVolume(ctx context.Context, req CreateVolumeRequest) (Volume, error) {
	path, err := VolumePathWithin(req.Pool.Target, req.Name)
	if err != nil {
		return Volume{}, err
	}

	// The guest's own directory. Created here rather than at pool creation
	// because a pool does not know its guests.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Volume{}, fmt.Errorf("storage: create %s: %w", filepath.Dir(path), err)
	}

	if req.Kind == VolumeRootfs {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return Volume{}, fmt.Errorf("storage: create rootfs %s: %w", path, err)
		}
		return Volume{Path: path, Format: "dir"}, nil
	}

	if _, err := os.Stat(path); err == nil {
		return Volume{}, fmt.Errorf("%w: %s", ErrExists, path)
	}

	format := req.Format
	if format == "" {
		format = "qcow2"
	}

	args := []string{"create", "-f", format}
	if req.BackingPath != "" {
		// A backing file makes this a copy-on-write volume over the cached
		// cloud image or a template disk: instant, and it only stores what the
		// guest changes. The backing format is stated explicitly because
		// qemu-img refuses to probe it, and probing would be a security
		// problem in its own right.
		backingFormat, err := qemuImgFormat(ctx, req.BackingPath)
		if err != nil {
			return Volume{}, err
		}
		args = append(args, "-b", BackingRef(path, req.BackingPath), "-F", backingFormat)
	}
	args = append(args, path)
	if req.Size > 0 {
		args = append(args, fmt.Sprintf("%d", req.Size))
	}

	if out, err := runQemuImg(ctx, qemuImgLongTimeout, args...); err != nil {
		return Volume{}, fmt.Errorf("storage: create %s: %w (%s)", path, err, firstLine(out))
	}
	// 0600 for the same reason the pool is 0700: this is a guest's disk.
	errlog.Log(os.Chmod(path, 0o600))

	info, err := qemuImgInfo(ctx, path)
	if err != nil {
		return Volume{}, err
	}
	return Volume{Path: path, Format: format, SizeBytes: info.VirtualSize}, nil
}

func (d *dirDriver) ResizeVolume(ctx context.Context, _ Pool, path string, newSize int64) (int64, error) {
	info, err := qemuImgInfo(ctx, path)
	if err != nil {
		return 0, err
	}
	if newSize < info.VirtualSize {
		return 0, fmt.Errorf("%w: %s is %d bytes and %d was requested",
			ErrShrink, path, info.VirtualSize, newSize)
	}
	if newSize == info.VirtualSize {
		return info.VirtualSize, nil
	}
	if out, err := runQemuImg(ctx, qemuImgLongTimeout, "resize", path, fmt.Sprintf("%d", newSize)); err != nil {
		return 0, fmt.Errorf("storage: resize %s: %w (%s)", path, err, firstLine(out))
	}
	after, err := qemuImgInfo(ctx, path)
	if err != nil {
		return 0, err
	}
	return after.VirtualSize, nil
}

func (d *dirDriver) DeleteVolume(_ context.Context, pool Pool, path string) error {
	if err := assertWithinPool(pool, path); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("storage: delete %s: %w", path, err)
	}
	return nil
}

// ListVolumes reports the disk images sitting in a dir pool.
//
// The seed images cloud-init writes are skipped: they are attached as CD-ROMs
// to exactly one guest and offering one as a disk would only ever be a mistake.
func (d *dirDriver) ListVolumes(ctx context.Context, pool Pool) ([]VolumeInfo, error) {
	entries, err := os.ReadDir(pool.Target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("storage: read pool %s: %w", pool.Target, err)
	}

	// One level down as well as the top: a guest's disks live in its own
	// directory, and a pool that only listed its root would report an empty
	// pool full of data.
	type candidate struct{ name, path string }
	files := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			nested, err := os.ReadDir(filepath.Join(pool.Target, entry.Name()))
			if err != nil {
				continue
			}
			for _, child := range nested {
				if child.IsDir() {
					continue
				}
				files = append(files, candidate{
					name: entry.Name() + "/" + child.Name(),
					path: filepath.Join(pool.Target, entry.Name(), child.Name()),
				})
			}
			continue
		}
		files = append(files, candidate{
			name: entry.Name(),
			path: filepath.Join(pool.Target, entry.Name()),
		})
	}

	out := make([]VolumeInfo, 0, len(files))
	for _, entry := range files {
		if strings.HasSuffix(entry.name, ".iso") {
			continue
		}
		info, err := os.Stat(entry.path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		path := entry.path
		// A file qemu-img cannot read is not a disk, so it is skipped rather
		// than offered with a guessed format — attaching one would fail at
		// start time with an error that names QEMU rather than the file.
		img, err := qemuImgInfo(ctx, path)
		if err != nil {
			continue
		}
		out = append(out, VolumeInfo{
			Name:      entry.name,
			Path:      path,
			Format:    img.Format,
			SizeBytes: img.VirtualSize,
		})
	}
	return out, nil
}

func (d *dirDriver) CloneVolume(ctx context.Context, pool Pool, source, target string, linked bool) (Volume, error) {
	targetPath, err := VolumePathWithin(pool.Target, target)
	if err != nil {
		return Volume{}, err
	}
	if err := assertWithinPool(pool, source); err != nil {
		return Volume{}, err
	}

	sourceInfo, err := os.Stat(source)
	if err != nil {
		return Volume{}, fmt.Errorf("%w: %s", ErrNotFound, source)
	}

	if sourceInfo.IsDir() {
		// A container rootfs on a dir pool. There is no CoW to be had, so this
		// is an honest recursive copy and the caller reports progress for it.
		if err := copyTree(ctx, source, targetPath); err != nil {
			return Volume{}, err
		}
		return Volume{Path: targetPath, Format: "dir"}, nil
	}

	// The target sits in the new guest's own directory, which nothing has
	// created yet; qemu-img will not create it and fails with ENOENT.
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return Volume{}, fmt.Errorf("storage: creating %s: %w", filepath.Dir(targetPath), err)
	}

	if linked {
		// A linked clone is a new qcow2 whose backing file is the source. It
		// is instant and space-efficient, and the source can never be deleted
		// while it exists — the caller enforces that with the blocking list.
		format, err := qemuImgFormat(ctx, source)
		if err != nil {
			return Volume{}, err
		}
		out, err := runQemuImg(ctx, qemuImgTimeout,
			"create", "-f", "qcow2", "-b", source, "-F", format, targetPath)
		if err != nil {
			return Volume{}, fmt.Errorf("storage: linked clone %s: %w (%s)", targetPath, err, failureLine(out))
		}
	} else {
		// A full clone via convert rather than a file copy, so the result is a
		// standalone image with no backing chain even when the source had one.
		out, err := runQemuImg(ctx, qemuImgLongTimeout,
			"convert", "-O", "qcow2", source, targetPath)
		if err != nil {
			return Volume{}, fmt.Errorf("storage: full clone %s: %w (%s)", targetPath, err, failureLine(out))
		}
	}
	errlog.Log(os.Chmod(targetPath, 0o600))

	info, err := qemuImgInfo(ctx, targetPath)
	if err != nil {
		return Volume{}, err
	}
	return Volume{Path: targetPath, Format: "qcow2", SizeBytes: info.VirtualSize}, nil
}

// Snapshot on a dir pool creates an external qcow2 overlay: the current file
// becomes the snapshot and writes continue into a new file at the original
// path, so the guest's disk path never changes.
func (d *dirDriver) Snapshot(ctx context.Context, pool Pool, path, name string) (Snapshot, error) {
	if err := ValidateSnapshotName(name); err != nil {
		return Snapshot{}, err
	}
	if err := assertWithinPool(pool, path); err != nil {
		return Snapshot{}, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: %s", ErrNotFound, path)
	}
	if info.IsDir() {
		// A container rootfs on a dir pool: copy it aside. Slow, and the UI
		// says so before the operator commits.
		snapPath := path + ".snap." + name
		if err := copyTree(ctx, path, snapPath); err != nil {
			return Snapshot{}, err
		}
		size := errlog.Value(treeSize(snapPath))
		return Snapshot{Handle: snapPath, SizeBytes: size, Mechanism: "copy"}, nil
	}

	snapPath := path + ".snap." + name
	if _, err := os.Stat(snapPath); err == nil {
		return Snapshot{}, fmt.Errorf("%w: snapshot %s", ErrExists, name)
	}

	// Move the current image aside, then create a fresh overlay backed by it.
	// The rename is atomic and happens while the guest is stopped or its
	// writes are quiesced, so there is no window where the guest's path is
	// missing.
	if err := os.Rename(path, snapPath); err != nil {
		return Snapshot{}, fmt.Errorf("storage: stage snapshot %s: %w", name, err)
	}
	format, err := qemuImgFormat(ctx, snapPath)
	if err != nil {
		errlog.Log(os.Rename(snapPath, path))
		return Snapshot{}, err
	}
	out, err := runQemuImg(ctx, qemuImgTimeout,
		"create", "-f", "qcow2", "-b", BackingRef(path, snapPath), "-F", format, path)
	if err != nil {
		// Put it back: a failed snapshot must not leave the guest without a
		// disk at the path its definition names.
		errlog.Log(os.Rename(snapPath, path))
		return Snapshot{}, fmt.Errorf("storage: snapshot overlay %s: %w (%s)", path, err, firstLine(out))
	}
	errlog.Log(os.Chmod(path, 0o600))

	size := int64(0)
	if st, err := os.Stat(snapPath); err == nil {
		size = st.Size()
	}
	return Snapshot{Handle: snapPath, SizeBytes: size, Mechanism: "qcow2"}, nil
}

// RevertSnapshot discards the live overlay and restores the snapshot in its
// place. Everything written since the snapshot is gone, which is why the caller
// requires a typed confirmation of the guest name first.
func (d *dirDriver) RevertSnapshot(ctx context.Context, pool Pool, path string, snap Snapshot) error {
	if err := assertWithinPool(pool, path); err != nil {
		return err
	}
	if err := assertWithinPool(pool, snap.Handle); err != nil {
		return err
	}
	if _, err := os.Stat(snap.Handle); err != nil {
		return fmt.Errorf("%w: snapshot %s", ErrNotFound, snap.Handle)
	}

	if snap.Mechanism == "copy" {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("storage: clear rootfs %s: %w", path, err)
		}
		return copyTree(ctx, snap.Handle, path)
	}

	// The overlay is discarded and a new empty one is created over the same
	// snapshot, so the snapshot itself survives the revert and can be reverted
	// to again.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: discard overlay %s: %w", path, err)
	}
	format, err := qemuImgFormat(ctx, snap.Handle)
	if err != nil {
		return err
	}
	out, err := runQemuImg(ctx, qemuImgTimeout,
		"create", "-f", "qcow2", "-b", BackingRef(path, snap.Handle), "-F", format, path)
	if err != nil {
		return fmt.Errorf("storage: revert %s: %w (%s)", path, err, firstLine(out))
	}
	errlog.Log(os.Chmod(path, 0o600))
	return nil
}

// DeleteSnapshot removes a snapshot by folding it back into the chain.
//
// A qcow2 overlay cannot simply be deleted: the live image is backed by it, and
// removing it would leave a disk with a missing backing file, which is a
// corrupted guest. Committing the live overlay down into the snapshot and then
// rebasing is the operation that actually shortens the chain.
func (d *dirDriver) DeleteSnapshot(ctx context.Context, pool Pool, path string, snap Snapshot) error {
	if err := assertWithinPool(pool, snap.Handle); err != nil {
		return err
	}
	if snap.Mechanism == "copy" {
		if err := os.RemoveAll(snap.Handle); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("storage: delete snapshot %s: %w", snap.Handle, err)
		}
		return nil
	}

	// Rebase the live image onto the snapshot's own backing file, which folds
	// the snapshot's data into the live image where needed, then the snapshot
	// file is genuinely unreferenced and safe to remove.
	backing, err := qemuImgBacking(ctx, snap.Handle)
	if err != nil {
		return err
	}
	args := []string{"rebase", "-f", "qcow2"}
	if backing == "" {
		// No grandparent: the rebase is onto nothing, which makes the live
		// image standalone.
		args = append(args, "-b", "")
	} else {
		backingFormat, err := qemuImgFormat(ctx, backing)
		if err != nil {
			return err
		}
		args = append(args, "-b", BackingRef(path, backing), "-F", backingFormat)
	}
	args = append(args, path)

	if out, err := runQemuImg(ctx, qemuImgLongTimeout, args...); err != nil {
		return fmt.Errorf("storage: rebase %s: %w (%s)", path, err, firstLine(out))
	}
	if err := os.Remove(snap.Handle); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: delete snapshot %s: %w", snap.Handle, err)
	}
	return nil
}

// imgInfo is the subset of `qemu-img info --output=json` this package reads.
type imgInfo struct {
	VirtualSize int64  `json:"virtual-size"`
	ActualSize  int64  `json:"actual-size"`
	Format      string `json:"format"`
	BackingFile string `json:"backing-filename"`
}

func qemuImgInfo(ctx context.Context, path string) (imgInfo, error) {
	out, err := runQemuImg(ctx, qemuImgTimeout, "info", "--output=json", path)
	if err != nil {
		return imgInfo{}, fmt.Errorf("storage: qemu-img info %s: %w (%s)", path, err, firstLine(out))
	}
	var info imgInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return imgInfo{}, fmt.Errorf("storage: parse qemu-img info for %s: %w", path, err)
	}
	return info, nil
}

// qemuImgFormat reads an image's format.
//
// It is read rather than assumed because qemu-img requires the backing format
// to be stated explicitly when creating an overlay, and guessing it wrong
// produces an image that either refuses to open or — worse — opens with the
// wrong interpretation of its own bytes.
func qemuImgFormat(ctx context.Context, path string) (string, error) {
	info, err := qemuImgInfo(ctx, path)
	if err != nil {
		return "", err
	}
	if info.Format == "" {
		return "", fmt.Errorf("storage: qemu-img reported no format for %s", path)
	}
	return info.Format, nil
}

func qemuImgBacking(ctx context.Context, path string) (string, error) {
	info, err := qemuImgInfo(ctx, path)
	if err != nil {
		return "", err
	}
	return info.BackingFile, nil
}

// runQemuImg executes qemu-img with an explicit timeout.
//
// Arguments are passed as an argv, never joined into a shell string, so a path
// containing a space or a semicolon stays one argument.
func runQemuImg(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "qemu-img", args...).CombinedOutput()
	return string(out), err
}

// statfsBytes reads capacity for a path, creating nothing.
func statfsBytes(path string) (Stats, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return Stats{}, fmt.Errorf("storage: statfs %s: %w", path, err)
	}
	blockSize := int64(st.Bsize)
	capacity := int64(st.Blocks) * blockSize
	// Bavail rather than Bfree: Bfree includes the root-reserved blocks, which
	// a guest disk cannot actually use, and reporting them as free is how a
	// pool "with room" fails to allocate.
	free := int64(st.Bavail) * blockSize
	return Stats{
		CapacityBytes: capacity,
		FreeBytes:     free,
		UsedBytes:     capacity - free,
	}, nil
}

// assertWithinPool refuses to touch a path outside its pool.
//
// Paths reaching the delete and snapshot methods come from database rows, which
// come from earlier agent replies — but a control plane that has been
// compromised, or simply a bug, must not be able to make the root-privileged
// agent remove an arbitrary path.
func assertWithinPool(pool Pool, path string) error {
	base := filepath.Clean(pool.Target)
	clean := filepath.Clean(path)
	if clean != base && !strings.HasPrefix(clean, base+string(filepath.Separator)) {
		return fmt.Errorf("storage: refusing to operate on %q outside pool %q", path, pool.Target)
	}
	return nil
}

// copyTree copies a directory recursively, preserving ownership and modes.
//
// It uses `cp -a` rather than a Go walk: a container rootfs contains hardlinks,
// symlinks, device nodes, sparse files and xattrs, and reimplementing all of
// that correctly is a much larger job than it looks. The command is invoked
// with an argv, so the paths cannot reshape it.
func copyTree(ctx context.Context, source, target string) error {
	ctx, cancel := context.WithTimeout(ctx, qemuImgLongTimeout)
	defer cancel()

	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("storage: prepare %s: %w", target, err)
	}
	// The trailing "/." copies the contents of source into target rather than
	// source itself into target, which is the difference between rootfs/ and
	// rootfs/rootfs/.
	out, err := exec.CommandContext(ctx, "cp", "-a", source+"/.", target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("storage: copy %s to %s: %w (%s)", source, target, err, firstLine(string(out)))
	}
	return nil
}

// treeSize sums a directory's apparent size, bounded so a pathological tree
// cannot stall a sampling call.
func treeSize(root string) (int64, error) {
	var total int64
	var count int
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // unreadable entries are skipped so the size stays an estimate
		}
		count++
		if count > 500_000 {
			return filepath.SkipAll
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// CreateOverlay makes a fresh qcow2 overlay at overlayPath backed by
// backingPath.
//
// Exported because reverting to a snapshot has to reopen the guest on a new
// branch rather than on the snapshot's own file: other branches are backed by
// that file, and writing into it would rewrite their history too.
func CreateOverlay(ctx context.Context, backingPath, overlayPath string) error {
	format, err := qemuImgFormat(ctx, backingPath)
	if err != nil {
		return err
	}
	out, err := runQemuImg(ctx, qemuImgTimeout,
		"create", "-f", "qcow2", "-b", BackingRef(overlayPath, backingPath), "-F", format, overlayPath)
	if err != nil {
		return fmt.Errorf("storage: overlay %s: %w (%s)", overlayPath, err, firstLine(out))
	}
	errlog.Log(os.Chmod(overlayPath, 0o600))
	return nil
}
