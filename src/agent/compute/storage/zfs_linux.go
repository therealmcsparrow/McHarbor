// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package storage

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// The ZFS backend, and the reason the setup wizard recommends it.
//
// A VM disk is a zvol — a block device carved out of the pool — and a container
// rootfs is a dataset. Both snapshot and clone as metadata operations, so
// "snapshot this 500 GB VM" and "create ten containers from this template" are
// constant-time rather than proportional to the data. That is not a marginal
// improvement over the dir backend; it changes which workflows are practical.
//
// It is driven through the zfs(8) CLI with `-H -p` (script mode, exact byte
// values) rather than through a library. The one maintained Go binding is a
// wrapper over the same CLI, so it would add a dependency without removing the
// subprocess, and the parsing it saves is the handful of tab-split lines below.
// Dataset names are always passed as separate argv entries, never interpolated
// into a shell string.

// zfsTimeout bounds an ordinary zfs command. Snapshots and clones are metadata
// operations and answer immediately.
const zfsTimeout = 60 * time.Second

// zfsLongTimeout bounds the operations that actually move data: creating a
// large zvol with reservation, or destroying a dataset with many snapshots.
const zfsLongTimeout = 2 * time.Hour

type zfsDriver struct{}

var _ Driver = (*zfsDriver)(nil)

// NewZFSDriver builds the ZFS backend.
func NewZFSDriver() Driver { return &zfsDriver{} }

func (z *zfsDriver) Name() string { return BackendZFS }

// SupportsQuota is true: a dataset takes a refquota, which is a real,
// enforced limit rather than an advisory number.
func (z *zfsDriver) SupportsQuota() bool { return true }

func (z *zfsDriver) Detect(ctx context.Context) Detection {
	det := Detection{Backend: BackendZFS, Instant: true}

	path, err := exec.LookPath("zfs")
	if err != nil {
		det.Detail = "The zfs command is not on PATH."
		det.Remediation = "Install ZFS (zfsutils-linux on Debian/Ubuntu, zfs on RHEL/Fedora) " +
			"to get instant guest snapshots and clones."
		return det
	}

	// `zfs list` failing while the binary exists usually means the module is
	// not loaded — a different problem with a different fix, so it is reported
	// as such rather than as "ZFS not installed".
	out, err := runZFS(ctx, zfsTimeout, "list", "-H", "-p", "-o", "name,available,used,type")
	if err != nil {
		det.Detail = fmt.Sprintf("%s is installed but no pools could be listed: %s", path, firstLine(out))
		det.Remediation = "Check that the zfs kernel module is loaded (sudo modprobe zfs) and " +
			"that at least one pool is imported (zpool import)."
		return det
	}

	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) < 4 || fields[3] != "filesystem" {
			continue
		}
		available := errlog.Value(strconv.ParseInt(fields[1], 10, 64))
		used := errlog.Value(strconv.ParseInt(fields[2], 10, 64))
		det.Candidates = append(det.Candidates, Candidate{
			Target:        fields[0],
			CapacityBytes: available + used,
			FreeBytes:     available,
		})
	}

	det.Available = true
	det.Detail = fmt.Sprintf("ZFS is available with %d datasets. Guest snapshots and clones "+
		"are metadata operations, so they complete instantly regardless of disk size.",
		len(det.Candidates))
	return det
}

// Root resolves the dataset's mountpoint, because a zfs pool is named by its
// dataset and that name is not a path anything can open.
func (z *zfsDriver) Root(ctx context.Context, pool Pool) (string, error) {
	return z.mountpoint(ctx, pool.Target)
}

func (z *zfsDriver) EnsurePool(ctx context.Context, pool Pool) (Stats, error) {
	if err := validateDatasetName(pool.Target); err != nil {
		return Stats{}, err
	}
	// The pool's dataset is created if missing. -p creates the parents too, so
	// an operator can name tank/mcharbor/vms without creating each level.
	if _, err := runZFS(ctx, zfsTimeout, "list", "-H", "-o", "name", pool.Target); err != nil {
		if out, err := runZFS(ctx, zfsTimeout, "create", "-p", pool.Target); err != nil {
			return Stats{}, fmt.Errorf("storage: create dataset %s: %w (%s)",
				pool.Target, err, firstLine(out))
		}
	}
	return z.Stats(ctx, pool)
}

func (z *zfsDriver) Stats(ctx context.Context, pool Pool) (Stats, error) {
	out, err := runZFS(ctx, zfsTimeout, "list", "-H", "-p",
		"-o", "available,used,logicalused", pool.Target)
	if err != nil {
		return Stats{}, fmt.Errorf("storage: stats for %s: %w (%s)", pool.Target, err, firstLine(out))
	}
	fields := strings.Split(strings.TrimSpace(out), "\t")
	if len(fields) < 2 {
		return Stats{}, fmt.Errorf("storage: unexpected zfs list output for %s", pool.Target)
	}
	available := errlog.Value(strconv.ParseInt(fields[0], 10, 64))
	used := errlog.Value(strconv.ParseInt(fields[1], 10, 64))

	stats := Stats{
		CapacityBytes: available + used,
		UsedBytes:     used,
		FreeBytes:     available,
	}
	if len(fields) >= 3 {
		// logicalused is the pre-compression, pre-dedup size, which is what
		// the guests actually think they have written. Reporting it as
		// allocated is what makes the overcommit visible.
		stats.AllocatedBytes = errlog.Value(strconv.ParseInt(fields[2], 10, 64))
	}

	// Direct children of the pool dataset are its volumes.
	if children, err := runZFS(ctx, zfsTimeout, "list", "-H", "-o", "name", "-r", "-d", "1", pool.Target); err == nil {
		sc := bufio.NewScanner(strings.NewReader(children))
		for sc.Scan() {
			if strings.TrimSpace(sc.Text()) != pool.Target {
				stats.VolumeCount++
			}
		}
	}
	return stats, nil
}

func (z *zfsDriver) RemovePool(ctx context.Context, pool Pool, force bool) error {
	if err := validateDatasetName(pool.Target); err != nil {
		return err
	}
	children, err := z.children(ctx, pool.Target)
	if err != nil {
		return err
	}
	if len(children) > 0 && !force {
		return fmt.Errorf("%w: dataset %s still holds %d volumes: %s",
			ErrInUse, pool.Target, len(children), strings.Join(children, ", "))
	}
	args := []string{"destroy"}
	if force {
		// -r takes the children, -f detaches anything still holding it open.
		args = append(args, "-r", "-f")
	}
	args = append(args, pool.Target)
	if out, err := runZFS(ctx, zfsLongTimeout, args...); err != nil {
		return fmt.Errorf("storage: destroy dataset %s: %w (%s)", pool.Target, err, firstLine(out))
	}
	return nil
}

// children lists a dataset's direct descendants.
func (z *zfsDriver) children(ctx context.Context, dataset string) ([]string, error) {
	out, err := runZFS(ctx, zfsTimeout, "list", "-H", "-o", "name", "-r", "-d", "1", dataset)
	if err != nil {
		return nil, fmt.Errorf("storage: list children of %s: %w (%s)", dataset, err, firstLine(out))
	}
	var names []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		name := strings.TrimSpace(sc.Text())
		if name != "" && name != dataset {
			names = append(names, name)
		}
	}
	return names, nil
}

func (z *zfsDriver) CreateVolume(ctx context.Context, req CreateVolumeRequest) (Volume, error) {
	if err := ValidateVolumeName(req.Name); err != nil {
		return Volume{}, err
	}
	if err := validateDatasetName(req.Pool.Target); err != nil {
		return Volume{}, err
	}
	dataset := req.Pool.Target + "/" + req.Name

	// A clone of an existing snapshot is the instant path, and it is what
	// makes "create ten containers from this template" fast. The caller passes
	// the source snapshot as BackingPath.
	if req.BackingPath != "" {
		return z.cloneFromSnapshot(ctx, req, dataset)
	}

	if req.Kind == VolumeRootfs {
		if out, err := runZFS(ctx, zfsTimeout, "create", "-p", dataset); err != nil {
			return Volume{}, fmt.Errorf("storage: create dataset %s: %w (%s)",
				dataset, err, firstLine(out))
		}
		if req.Size > 0 {
			if err := z.SetQuota(ctx, req.Pool, dataset, req.Size); err != nil {
				return Volume{}, err
			}
		}
		mount, err := z.mountpoint(ctx, dataset)
		if err != nil {
			return Volume{}, err
		}
		return Volume{Path: mount, Format: "dir", SizeBytes: req.Size}, nil
	}

	if req.Size <= 0 {
		return Volume{}, fmt.Errorf("storage: a zvol needs a size")
	}
	args := []string{"create", "-p"}
	if req.Sparse {
		// -s makes the zvol sparse: it reserves nothing up front, which is
		// what allows thin provisioning and also what makes pool exhaustion
		// possible. The caller pairs this with the pool capacity alert.
		args = append(args, "-s")
	}
	// 16k volblocksize is a better default than the 8k historical one for
	// guest filesystems, which almost all use 4k pages and benefit from fewer,
	// larger records.
	args = append(args, "-b", "16384", "-V", strconv.FormatInt(req.Size, 10), dataset)

	if out, err := runZFS(ctx, zfsLongTimeout, args...); err != nil {
		return Volume{}, fmt.Errorf("storage: create zvol %s: %w (%s)", dataset, err, firstLine(out))
	}
	return Volume{
		Path:      "/dev/zvol/" + dataset,
		Format:    "raw",
		SizeBytes: req.Size,
		IsBlock:   true,
	}, nil
}

// cloneFromSnapshot creates a dataset or zvol as a clone.
//
// ZFS can only clone a snapshot, not a live dataset, so if the caller handed a
// plain dataset a snapshot is taken first. That snapshot then permanently
// anchors the clone — ZFS refuses to destroy a snapshot with clones — which is
// exactly the dependency the UI reports as "this template has linked clones".
func (z *zfsDriver) cloneFromSnapshot(ctx context.Context, req CreateVolumeRequest, dataset string) (Volume, error) {
	source := req.BackingPath
	if !strings.Contains(source, "@") {
		snapName := source + "@mcharbor-template"
		if _, err := runZFS(ctx, zfsTimeout, "list", "-H", "-t", "snapshot", "-o", "name", snapName); err != nil {
			if out, err := runZFS(ctx, zfsTimeout, "snapshot", snapName); err != nil {
				return Volume{}, fmt.Errorf("storage: snapshot template %s: %w (%s)",
					source, err, firstLine(out))
			}
		}
		source = snapName
	}

	if out, err := runZFS(ctx, zfsTimeout, "clone", "-p", source, dataset); err != nil {
		return Volume{}, fmt.Errorf("storage: clone %s to %s: %w (%s)",
			source, dataset, err, firstLine(out))
	}

	if req.Kind == VolumeRootfs {
		mount, err := z.mountpoint(ctx, dataset)
		if err != nil {
			return Volume{}, err
		}
		if req.Size > 0 {
			if err := z.SetQuota(ctx, req.Pool, dataset, req.Size); err != nil {
				return Volume{}, err
			}
		}
		return Volume{Path: mount, Format: "dir", SizeBytes: req.Size}, nil
	}

	// A cloned zvol inherits the source's size; growing it to the requested
	// size is a separate step because a clone may legitimately be larger than
	// its template.
	if req.Size > 0 {
		if _, err := z.ResizeVolume(ctx, req.Pool, dataset, req.Size); err != nil {
			return Volume{}, err
		}
	}
	return Volume{
		Path:      "/dev/zvol/" + dataset,
		Format:    "raw",
		SizeBytes: req.Size,
		IsBlock:   true,
	}, nil
}

// mountpoint reads where a dataset is mounted, which is the path a container
// rootfs actually lives at.
func (z *zfsDriver) mountpoint(ctx context.Context, dataset string) (string, error) {
	out, err := runZFS(ctx, zfsTimeout, "list", "-H", "-o", "mountpoint", dataset)
	if err != nil {
		return "", fmt.Errorf("storage: mountpoint of %s: %w (%s)", dataset, err, firstLine(out))
	}
	mount := strings.TrimSpace(out)
	if mount == "" || mount == "none" || mount == "legacy" {
		return "", fmt.Errorf("storage: dataset %s has no mountpoint (%q); "+
			"container rootfs datasets need one", dataset, mount)
	}
	return mount, nil
}

func (z *zfsDriver) ResizeVolume(ctx context.Context, pool Pool, path string, newSize int64) (int64, error) {
	dataset := datasetFromPath(pool, path)
	out, err := runZFS(ctx, zfsTimeout, "get", "-H", "-p", "-o", "value", "volsize", dataset)
	if err != nil {
		// Not a zvol: this is a rootfs dataset, so "resize" means the quota.
		if err := z.SetQuota(ctx, pool, path, newSize); err != nil {
			return 0, err
		}
		return newSize, nil
	}
	current := errlog.Value(strconv.ParseInt(strings.TrimSpace(out), 10, 64))
	if newSize < current {
		return 0, fmt.Errorf("%w: %s is %d bytes and %d was requested", ErrShrink, dataset, current, newSize)
	}
	if newSize == current {
		return current, nil
	}
	if out, err := runZFS(ctx, zfsLongTimeout, "set",
		"volsize="+strconv.FormatInt(newSize, 10), dataset); err != nil {
		return 0, fmt.Errorf("storage: resize %s: %w (%s)", dataset, err, firstLine(out))
	}
	return newSize, nil
}

func (z *zfsDriver) DeleteVolume(ctx context.Context, pool Pool, path string) error {
	dataset := datasetFromPath(pool, path)
	if err := assertWithinDataset(pool, dataset); err != nil {
		return err
	}
	// -r removes the volume's own snapshots with it. A volume whose snapshots
	// anchor someone else's clone fails here, and the error names the clone,
	// which is the useful answer.
	if out, err := runZFS(ctx, zfsLongTimeout, "destroy", "-r", dataset); err != nil {
		if strings.Contains(out, "does not exist") {
			return nil
		}
		return fmt.Errorf("storage: destroy %s: %w (%s)", dataset, err, firstLine(out))
	}
	return nil
}

// ListVolumes reports the zvols directly under a ZFS pool dataset.
//
// Filesystem children are container rootfs trees, not VM disks, so only the
// `volume` type is returned; `volsize` is the guest-visible size, which is the
// one that matters here rather than what the zvol has actually consumed.
func (z *zfsDriver) ListVolumes(ctx context.Context, pool Pool) ([]VolumeInfo, error) {
	if err := validateDatasetName(pool.Target); err != nil {
		return nil, err
	}
	out, err := runZFS(ctx, zfsTimeout, "list", "-H", "-p",
		"-o", "name,type,volsize", "-r", "-d", "1", pool.Target)
	if err != nil {
		if strings.Contains(out, "does not exist") {
			return nil, nil
		}
		return nil, fmt.Errorf("storage: list volumes in %s: %w (%s)", pool.Target, err, firstLine(out))
	}

	var volumes []VolumeInfo
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		fields := strings.Split(strings.TrimSpace(sc.Text()), "	")
		if len(fields) < 3 || fields[0] == pool.Target || fields[1] != "volume" {
			continue
		}
		size := errlog.Value(strconv.ParseInt(fields[2], 10, 64))
		volumes = append(volumes, VolumeInfo{
			Name:      strings.TrimPrefix(fields[0], pool.Target+"/"),
			Path:      "/dev/zvol/" + fields[0],
			Format:    "raw",
			SizeBytes: size,
			IsBlock:   true,
		})
	}
	return volumes, nil
}

func (z *zfsDriver) CloneVolume(ctx context.Context, pool Pool, source, target string, linked bool) (Volume, error) {
	if err := ValidateVolumeName(target); err != nil {
		return Volume{}, err
	}
	sourceDataset := datasetFromPath(pool, source)
	if err := assertWithinDataset(pool, sourceDataset); err != nil {
		return Volume{}, err
	}
	targetDataset := pool.Target + "/" + target

	snapName := sourceDataset + "@clone-" + target
	if out, err := runZFS(ctx, zfsTimeout, "snapshot", snapName); err != nil {
		if !strings.Contains(out, "already exists") {
			return Volume{}, fmt.Errorf("storage: snapshot %s: %w (%s)",
				sourceDataset, err, firstLine(out))
		}
	}

	if linked {
		if out, err := runZFS(ctx, zfsTimeout, "clone", "-p", snapName, targetDataset); err != nil {
			return Volume{}, fmt.Errorf("storage: clone %s: %w (%s)", snapName, err, firstLine(out))
		}
	} else {
		// A full clone is send|recv into an independent dataset, then the
		// staging snapshots go away so the copy has no lingering dependency on
		// its source. Slower, but the source stays deletable.
		if err := z.sendReceive(ctx, snapName, targetDataset); err != nil {
			return Volume{}, err
		}
		bestEffort(runZFS(ctx, zfsTimeout, "destroy", snapName))
		bestEffort(runZFS(ctx, zfsTimeout, "destroy", targetDataset+"@clone-"+target))
	}

	return z.describe(ctx, pool, targetDataset)
}

// sendReceive streams a snapshot into a new dataset, which is ZFS's own
// mechanism for an independent copy.
func (z *zfsDriver) sendReceive(ctx context.Context, snapshot, target string) error {
	ctx, cancel := context.WithTimeout(ctx, zfsLongTimeout)
	defer cancel()

	send := exec.CommandContext(ctx, "zfs", "send", snapshot)
	recv := exec.CommandContext(ctx, "zfs", "receive", target)

	pipe, err := send.StdoutPipe()
	if err != nil {
		return fmt.Errorf("storage: pipe zfs send: %w", err)
	}
	recv.Stdin = pipe

	if err := send.Start(); err != nil {
		return fmt.Errorf("storage: start zfs send: %w", err)
	}
	out, err := recv.CombinedOutput()
	if err != nil {
		errlog.Log(send.Process.Kill())
		errlog.Log(send.Wait())
		return fmt.Errorf("storage: zfs receive %s: %w (%s)", target, err, firstLine(string(out)))
	}
	if err := send.Wait(); err != nil {
		return fmt.Errorf("storage: zfs send %s: %w", snapshot, err)
	}
	return nil
}

// describe reports what a dataset turned out to be, so the caller does not have
// to assume whether it got a zvol or a filesystem.
func (z *zfsDriver) describe(ctx context.Context, pool Pool, dataset string) (Volume, error) {
	out, err := runZFS(ctx, zfsTimeout, "get", "-H", "-p", "-o", "value", "type", dataset)
	if err != nil {
		return Volume{}, fmt.Errorf("storage: describe %s: %w (%s)", dataset, err, firstLine(out))
	}
	if strings.TrimSpace(out) == "volume" {
		size := int64(0)
		if v, err := runZFS(ctx, zfsTimeout, "get", "-H", "-p", "-o", "value", "volsize", dataset); err == nil {
			size = errlog.Value(strconv.ParseInt(strings.TrimSpace(v), 10, 64))
		}
		return Volume{Path: "/dev/zvol/" + dataset, Format: "raw", SizeBytes: size, IsBlock: true}, nil
	}
	mount, err := z.mountpoint(ctx, dataset)
	if err != nil {
		return Volume{}, err
	}
	return Volume{Path: mount, Format: "dir"}, nil
}

func (z *zfsDriver) Snapshot(ctx context.Context, pool Pool, path, name string) (Snapshot, error) {
	if err := ValidateSnapshotName(name); err != nil {
		return Snapshot{}, err
	}
	dataset := datasetFromPath(pool, path)
	if err := assertWithinDataset(pool, dataset); err != nil {
		return Snapshot{}, err
	}
	handle := dataset + "@" + name

	if out, err := runZFS(ctx, zfsTimeout, "snapshot", handle); err != nil {
		if strings.Contains(out, "already exists") {
			return Snapshot{}, fmt.Errorf("%w: snapshot %s", ErrExists, name)
		}
		return Snapshot{}, fmt.Errorf("storage: snapshot %s: %w (%s)", handle, err, firstLine(out))
	}

	size := int64(0)
	if out, err := runZFS(ctx, zfsTimeout, "get", "-H", "-p", "-o", "value", "used", handle); err == nil {
		size = errlog.Value(strconv.ParseInt(strings.TrimSpace(out), 10, 64))
	}
	return Snapshot{Handle: handle, SizeBytes: size, Mechanism: BackendZFS}, nil
}

// RevertSnapshot rolls the dataset back.
//
// -r destroys every snapshot taken after this one, because ZFS cannot roll back
// past them. That is genuinely destructive and is why the caller shows the
// operator the exact list of snapshots that will be discarded and requires the
// guest's name typed out before it gets here.
func (z *zfsDriver) RevertSnapshot(ctx context.Context, pool Pool, _ string, snap Snapshot) error {
	if err := assertWithinDataset(pool, snap.Handle); err != nil {
		return err
	}
	if out, err := runZFS(ctx, zfsLongTimeout, "rollback", "-r", snap.Handle); err != nil {
		return fmt.Errorf("storage: rollback %s: %w (%s)", snap.Handle, err, firstLine(out))
	}
	return nil
}

func (z *zfsDriver) DeleteSnapshot(ctx context.Context, pool Pool, _ string, snap Snapshot) error {
	if err := assertWithinDataset(pool, snap.Handle); err != nil {
		return err
	}
	if out, err := runZFS(ctx, zfsTimeout, "destroy", snap.Handle); err != nil {
		if strings.Contains(out, "does not exist") {
			return nil
		}
		if strings.Contains(out, "has dependent clones") {
			return fmt.Errorf("%w: snapshot %s still backs a linked clone", ErrInUse, snap.Handle)
		}
		return fmt.Errorf("storage: destroy snapshot %s: %w (%s)", snap.Handle, err, firstLine(out))
	}
	return nil
}

// SetQuota applies a refquota, which bounds the dataset's own data without
// counting its snapshots. Counting snapshots would make a container hit its
// limit because of its backup history rather than its contents, which is not
// what the operator asked for.
func (z *zfsDriver) SetQuota(ctx context.Context, pool Pool, path string, bytes int64) error {
	dataset := datasetFromPath(pool, path)
	if err := assertWithinDataset(pool, dataset); err != nil {
		return err
	}
	value := "none"
	if bytes > 0 {
		value = strconv.FormatInt(bytes, 10)
	}
	if out, err := runZFS(ctx, zfsTimeout, "set", "refquota="+value, dataset); err != nil {
		return fmt.Errorf("storage: set quota on %s: %w (%s)", dataset, err, firstLine(out))
	}
	return nil
}

// datasetFromPath maps a stored volume path back to its dataset name.
//
// Volumes are recorded by the path the guest uses — /dev/zvol/tank/x for a
// zvol, a mountpoint for a dataset — so this reverses that for the operations
// that need the ZFS name.
func datasetFromPath(pool Pool, path string) string {
	if strings.HasPrefix(path, "/dev/zvol/") {
		return strings.TrimPrefix(path, "/dev/zvol/")
	}
	if strings.HasPrefix(path, pool.Target) {
		return path
	}
	// A mountpoint: take the trailing component as the volume name under the
	// pool dataset.
	if i := strings.LastIndex(path, "/"); i >= 0 && i+1 < len(path) {
		return pool.Target + "/" + path[i+1:]
	}
	return path
}

// validateDatasetName rejects a dataset name that could reshape a command or
// address something outside the operator's intent.
func validateDatasetName(name string) error {
	if name == "" {
		return fmt.Errorf("storage: empty dataset name")
	}
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "/") {
		return fmt.Errorf("storage: invalid dataset name %q", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == ':', r == '/':
		default:
			return fmt.Errorf("storage: dataset name %q contains an invalid character %q", name, r)
		}
	}
	return nil
}

// assertWithinDataset refuses to destroy or modify anything outside the pool's
// own dataset, for the same reason the dir driver checks paths: the agent runs
// as root, and a bad id must not become `zfs destroy -r tank`.
func assertWithinDataset(pool Pool, dataset string) error {
	base := pool.Target
	name := dataset
	if i := strings.Index(name, "@"); i >= 0 {
		name = name[:i]
	}
	if name != base && !strings.HasPrefix(name, base+"/") {
		return fmt.Errorf("storage: refusing to operate on %q outside pool dataset %q", dataset, base)
	}
	return nil
}

// runZFS executes a zfs command with an explicit timeout, passing arguments as
// an argv rather than a shell string.
func runZFS(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "zfs", args...).CombinedOutput()
	return string(out), err
}
