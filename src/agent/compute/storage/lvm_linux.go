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
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// The LVM thin backend.
//
// This is the documented exception in the dependency policy: there is no
// maintained Go library for LVM, so this driver shells out to lvm2. It is
// isolated behind this one type precisely so a future library swap touches a
// single file, and every call uses `--reportformat json` rather than parsing
// the human-readable columns, whose widths and headers change between releases.
//
// A pool here is a thin pool inside a volume group, named "vg/thinpool". Thin
// LVs snapshot and clone as metadata operations, so this sits in the instant
// tier with zfs and btrfs — but unlike those two it needs the operator to have
// created a thin pool first, which is why detection reports thin pools rather
// than volume groups.

const lvmTimeout = 60 * time.Second
const lvmLongTimeout = 2 * time.Hour

type lvmDriver struct{}

var _ Driver = (*lvmDriver)(nil)

// NewLVMDriver builds the LVM thin backend.
func NewLVMDriver() Driver { return &lvmDriver{} }

func (l *lvmDriver) Name() string { return BackendLVM }

// SupportsQuota is false in the sense the container resources panel means it:
// an LV's size is its limit, so there is no separate quota to set. The caller
// resizes the volume instead.
func (l *lvmDriver) SupportsQuota() bool { return false }

// Root reports ErrUnsupported: an lvm pool's volumes are block devices in a
// volume group, so there is no directory tree to browse.
func (l *lvmDriver) Root(_ context.Context, _ Pool) (string, error) {
	return "", ErrUnsupported
}

func (l *lvmDriver) SetQuota(_ context.Context, _ Pool, _ string, _ int64) error {
	return fmt.Errorf("%w: an LVM volume's size is its limit; resize it instead", ErrUnsupported)
}

// lvsReport is the shape of `lvs --reportformat json`.
type lvsReport struct {
	Report []struct {
		LV []struct {
			LVName      string `json:"lv_name"`
			VGName      string `json:"vg_name"`
			LVSize      string `json:"lv_size"`
			LVAttr      string `json:"lv_attr"`
			PoolLV      string `json:"pool_lv"`
			DataPercent string `json:"data_percent"`
		} `json:"lv"`
	} `json:"report"`
}

func (l *lvmDriver) Detect(ctx context.Context) Detection {
	det := Detection{Backend: BackendLVM, Instant: true}

	if _, err := exec.LookPath("lvs"); err != nil {
		det.Detail = "LVM tools are not on PATH."
		det.Remediation = "Install lvm2 and create a thin pool to use LVM-backed guest volumes."
		return det
	}

	out, err := runLVM(ctx, lvmTimeout, "lvs", "--reportformat", "json", "--units", "b",
		"--nosuffix", "-o", "lv_name,vg_name,lv_size,lv_attr,pool_lv,data_percent")
	if err != nil {
		det.Detail = fmt.Sprintf("LVM is installed but volumes could not be listed: %s", firstLine(out))
		det.Remediation = "Check that the agent can run lvs (it needs root) and that at least " +
			"one volume group exists."
		return det
	}

	var report lvsReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		det.Detail = "LVM reported volumes in a format this agent could not parse."
		det.Remediation = "Report this with the output of: lvs --reportformat json"
		return det
	}

	for _, group := range report.Report {
		for _, lv := range group.LV {
			// lv_attr's first character is the volume type; 't' is a thin
			// pool. Only thin pools can host the thin LVs this driver
			// allocates, so a plain volume group is not a candidate.
			if len(lv.LVAttr) == 0 || lv.LVAttr[0] != 't' {
				continue
			}
			size := errlog.Value(strconv.ParseFloat(lv.LVSize, 64))
			used := 0.0
			if pct, err := strconv.ParseFloat(lv.DataPercent, 64); err == nil {
				used = size * pct / 100
			}
			det.Candidates = append(det.Candidates, Candidate{
				Target:        lv.VGName + "/" + lv.LVName,
				CapacityBytes: int64(size),
				FreeBytes:     int64(size - used),
			})
		}
	}

	if len(det.Candidates) == 0 {
		det.Detail = "LVM is available but no thin pool was found. McHarbor allocates thin " +
			"logical volumes, which need one."
		det.Remediation = "Create a thin pool, for example: " +
			"sudo lvcreate --type thin-pool -l 90%FREE -n mcharbor <volume-group>"
		return det
	}

	det.Available = true
	det.Detail = fmt.Sprintf("LVM is available with %d thin pool(s). Snapshots and clones are "+
		"metadata operations, so they complete instantly.", len(det.Candidates))
	return det
}

// splitPoolTarget parses the "vg/thinpool" form.
func splitPoolTarget(target string) (vg, thinPool string, err error) {
	vg, thinPool, ok := strings.Cut(target, "/")
	if !ok || vg == "" || thinPool == "" {
		return "", "", fmt.Errorf("storage: LVM pool target %q must be in the form "+
			"<volume-group>/<thin-pool>", target)
	}
	if err := validateLVMName(vg); err != nil {
		return "", "", err
	}
	if err := validateLVMName(thinPool); err != nil {
		return "", "", err
	}
	return vg, thinPool, nil
}

// validateLVMName rejects anything that could be read as an option or reshape a
// command. A name starting with a dash would be parsed as a flag by every lvm2
// tool, which is the classic argument-injection shape.
func validateLVMName(name string) error {
	if name == "" {
		return fmt.Errorf("storage: empty LVM name")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("storage: LVM name %q may not start with a dash", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == '+':
		default:
			return fmt.Errorf("storage: LVM name %q contains an invalid character %q", name, r)
		}
	}
	return nil
}

func (l *lvmDriver) EnsurePool(ctx context.Context, pool Pool) (Stats, error) {
	vg, thinPool, err := splitPoolTarget(pool.Target)
	if err != nil {
		return Stats{}, err
	}
	// The thin pool must already exist. Creating one means deciding how much
	// of a volume group to consume, which is the operator's call and not
	// something to infer — so this reports what is missing rather than
	// guessing.
	if _, err := runLVM(ctx, lvmTimeout, "lvs", "--noheadings", "-o", "lv_name",
		vg+"/"+thinPool); err != nil {
		return Stats{}, fmt.Errorf("storage: thin pool %s does not exist; create it with "+
			"lvcreate --type thin-pool before using it as a McHarbor pool", pool.Target)
	}
	return l.Stats(ctx, pool)
}

func (l *lvmDriver) Stats(ctx context.Context, pool Pool) (Stats, error) {
	vg, thinPool, err := splitPoolTarget(pool.Target)
	if err != nil {
		return Stats{}, err
	}

	out, err := runLVM(ctx, lvmTimeout, "lvs", "--reportformat", "json", "--units", "b",
		"--nosuffix", "-o", "lv_name,vg_name,lv_size,lv_attr,pool_lv,data_percent", vg)
	if err != nil {
		return Stats{}, fmt.Errorf("storage: stats for %s: %w (%s)", pool.Target, err, firstLine(out))
	}
	var report lvsReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		return Stats{}, fmt.Errorf("storage: parse lvs output for %s: %w", pool.Target, err)
	}

	var stats Stats
	for _, group := range report.Report {
		for _, lv := range group.LV {
			size := errlog.Value(strconv.ParseFloat(lv.LVSize, 64))
			switch {
			case lv.LVName == thinPool:
				stats.CapacityBytes = int64(size)
				if pct, err := strconv.ParseFloat(lv.DataPercent, 64); err == nil {
					stats.UsedBytes = int64(size * pct / 100)
				}
			case lv.PoolLV == thinPool:
				// A thin LV in our pool. Summing their sizes is what shows the
				// overcommit: the total can exceed the pool, and a thin pool
				// that fills pauses every guest on it.
				stats.VolumeCount++
				stats.AllocatedBytes += int64(size)
			}
		}
	}
	stats.FreeBytes = stats.CapacityBytes - stats.UsedBytes
	return stats, nil
}

func (l *lvmDriver) RemovePool(ctx context.Context, pool Pool, force bool) error {
	vg, thinPool, err := splitPoolTarget(pool.Target)
	if err != nil {
		return err
	}
	volumes, err := l.poolVolumes(ctx, vg, thinPool)
	if err != nil {
		return err
	}
	if len(volumes) > 0 && !force {
		return fmt.Errorf("%w: thin pool %s still holds %d volumes: %s",
			ErrInUse, pool.Target, len(volumes), strings.Join(volumes, ", "))
	}
	// The thin pool itself is deliberately NOT removed. The operator created
	// it, it may hold volumes McHarbor never made, and detaching a McHarbor
	// pool should not destroy an LVM object the operator owns. Force only
	// removes the volumes McHarbor put there.
	for _, name := range volumes {
		if out, err := runLVM(ctx, lvmLongTimeout, "lvremove", "-f", vg+"/"+name); err != nil {
			return fmt.Errorf("storage: remove %s/%s: %w (%s)", vg, name, err, firstLine(out))
		}
	}
	return nil
}

// poolVolumes lists the thin LVs inside a thin pool.
func (l *lvmDriver) poolVolumes(ctx context.Context, vg, thinPool string) ([]string, error) {
	out, err := runLVM(ctx, lvmTimeout, "lvs", "--reportformat", "json",
		"-o", "lv_name,pool_lv", vg)
	if err != nil {
		return nil, fmt.Errorf("storage: list volumes in %s: %w (%s)", vg, err, firstLine(out))
	}
	var report lvsReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		return nil, fmt.Errorf("storage: parse lvs output for %s: %w", vg, err)
	}
	var names []string
	for _, group := range report.Report {
		for _, lv := range group.LV {
			if lv.PoolLV == thinPool {
				names = append(names, lv.LVName)
			}
		}
	}
	return names, nil
}

func (l *lvmDriver) CreateVolume(ctx context.Context, req CreateVolumeRequest) (Volume, error) {
	vg, thinPool, err := splitPoolTarget(req.Pool.Target)
	if err != nil {
		return Volume{}, err
	}
	if err := ValidateVolumeName(req.Name); err != nil {
		return Volume{}, err
	}
	if err := validateLVMName(req.Name); err != nil {
		return Volume{}, err
	}

	devicePath := "/dev/" + vg + "/" + req.Name

	if req.BackingPath != "" {
		// A thin snapshot of the template LV: instant, and the clone diverges
		// only where it is written.
		source := lvNameFromPath(req.BackingPath)
		if out, err := runLVM(ctx, lvmTimeout, "lvcreate", "-s",
			"-n", req.Name, vg+"/"+source); err != nil {
			return Volume{}, fmt.Errorf("storage: thin snapshot %s: %w (%s)",
				source, err, firstLine(out))
		}
		// Thin snapshots start inactive and with activation skipped, so
		// without this the device node never appears and the guest gets no
		// disk with no error to explain it.
		if out, err := runLVM(ctx, lvmTimeout, "lvchange", "-ay", "-K", vg+"/"+req.Name); err != nil {
			return Volume{}, fmt.Errorf("storage: activate %s: %w (%s)",
				req.Name, err, firstLine(out))
		}
	} else {
		if req.Size <= 0 {
			return Volume{}, fmt.Errorf("storage: a thin volume needs a size")
		}
		if out, err := runLVM(ctx, lvmLongTimeout, "lvcreate",
			"--type", "thin",
			"--virtualsize", strconv.FormatInt(req.Size, 10)+"b",
			"--thinpool", thinPool,
			"-n", req.Name, vg); err != nil {
			return Volume{}, fmt.Errorf("storage: create thin volume %s: %w (%s)",
				req.Name, err, firstLine(out))
		}
	}

	if req.Kind == VolumeRootfs {
		// A container rootfs needs a filesystem on the LV and a mountpoint;
		// the block device on its own is not something a container can pivot
		// into.
		mountPath, err := l.prepareRootfs(ctx, devicePath, req.Name)
		if err != nil {
			bestEffort(runLVM(ctx, lvmTimeout, "lvremove", "-f", vg+"/"+req.Name))
			return Volume{}, err
		}
		return Volume{Path: mountPath, Format: "dir", SizeBytes: req.Size}, nil
	}

	return Volume{Path: devicePath, Format: "raw", SizeBytes: req.Size, IsBlock: true}, nil
}

// rootfsMountRoot is where LVM-backed container rootfs volumes are mounted.
const rootfsMountRoot = "/var/lib/mcharbor/compute/lvm"

// prepareRootfs makes an LV usable as a container rootfs: put ext4 on it if it
// is blank, then mount it.
func (l *lvmDriver) prepareRootfs(ctx context.Context, devicePath, name string) (string, error) {
	// blkid answering means there is already a filesystem, so a clone's
	// filesystem is not destroyed by re-formatting it.
	if _, err := runCommand(ctx, lvmTimeout, "blkid", devicePath); err != nil {
		if out, err := runCommand(ctx, lvmLongTimeout, "mkfs.ext4", "-q", devicePath); err != nil {
			return "", fmt.Errorf("storage: format %s: %w (%s)", devicePath, err, firstLine(out))
		}
	}
	mountPath := rootfsMountRoot + "/" + name
	if err := os.MkdirAll(mountPath, 0o700); err != nil {
		return "", fmt.Errorf("storage: create mountpoint %s: %w", mountPath, err)
	}
	if out, err := runCommand(ctx, lvmTimeout, "mount", devicePath, mountPath); err != nil {
		if !strings.Contains(out, "already mounted") {
			return "", fmt.Errorf("storage: mount %s: %w (%s)", devicePath, err, firstLine(out))
		}
	}
	return mountPath, nil
}

func (l *lvmDriver) ResizeVolume(ctx context.Context, pool Pool, path string, newSize int64) (int64, error) {
	vg, _, err := splitPoolTarget(pool.Target)
	if err != nil {
		return 0, err
	}
	name := lvNameFromPath(path)
	if err := validateLVMName(name); err != nil {
		return 0, err
	}

	current, err := l.volumeSize(ctx, vg, name)
	if err != nil {
		return 0, err
	}
	if newSize < current {
		return 0, fmt.Errorf("%w: %s is %d bytes and %d was requested", ErrShrink, name, current, newSize)
	}
	if newSize == current {
		return current, nil
	}
	if out, err := runLVM(ctx, lvmLongTimeout, "lvresize",
		"-L", strconv.FormatInt(newSize, 10)+"b", vg+"/"+name); err != nil {
		return 0, fmt.Errorf("storage: resize %s: %w (%s)", name, err, firstLine(out))
	}
	return newSize, nil
}

func (l *lvmDriver) volumeSize(ctx context.Context, vg, name string) (int64, error) {
	out, err := runLVM(ctx, lvmTimeout, "lvs", "--noheadings", "--units", "b", "--nosuffix",
		"-o", "lv_size", vg+"/"+name)
	if err != nil {
		return 0, fmt.Errorf("%w: %s/%s", ErrNotFound, vg, name)
	}
	size, err := strconv.ParseFloat(strings.TrimSpace(out), 64)
	if err != nil {
		return 0, fmt.Errorf("storage: parse size of %s/%s: %w", vg, name, err)
	}
	return int64(size), nil
}

func (l *lvmDriver) DeleteVolume(ctx context.Context, pool Pool, path string) error {
	vg, _, err := splitPoolTarget(pool.Target)
	if err != nil {
		return err
	}
	name := lvNameFromPath(path)
	if err := validateLVMName(name); err != nil {
		return err
	}
	// A rootfs LV is mounted; lvremove on a mounted volume fails, so it comes
	// out first. Failure to unmount is not fatal here — lvremove -f reports
	// the real reason if it is still busy.
	if strings.HasPrefix(path, rootfsMountRoot+"/") {
		bestEffort(runCommand(ctx, lvmTimeout, "umount", path))
		errlog.Log(os.Remove(path))
	}
	if out, err := runLVM(ctx, lvmLongTimeout, "lvremove", "-f", vg+"/"+name); err != nil {
		if strings.Contains(out, "Failed to find") || strings.Contains(out, "not found") {
			return nil
		}
		return fmt.Errorf("storage: remove %s/%s: %w (%s)", vg, name, err, firstLine(out))
	}
	return nil
}

// ListVolumes reports the thin LVs inside a pool's thin pool.
//
// A rootfs LV is indistinguishable from a disk LV at this level, so both come
// back; the caller subtracts the ones it has records for, and an LV holding a
// container rootfs is one McHarbor already refers to.
func (l *lvmDriver) ListVolumes(ctx context.Context, pool Pool) ([]VolumeInfo, error) {
	vg, thinPool, err := splitPoolTarget(pool.Target)
	if err != nil {
		return nil, err
	}
	out, err := runLVM(ctx, lvmTimeout, "lvs", "--reportformat", "json", "--units", "b",
		"--nosuffix", "-o", "lv_name,lv_size,pool_lv", vg)
	if err != nil {
		return nil, fmt.Errorf("storage: list volumes in %s: %w (%s)", vg, err, firstLine(out))
	}
	var report lvsReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		return nil, fmt.Errorf("storage: parse lvs output for %s: %w", vg, err)
	}

	var volumes []VolumeInfo
	for _, group := range report.Report {
		for _, lv := range group.LV {
			if lv.PoolLV != thinPool {
				continue
			}
			size := errlog.Value(strconv.ParseFloat(lv.LVSize, 64))
			volumes = append(volumes, VolumeInfo{
				Name:      lv.LVName,
				Path:      "/dev/" + vg + "/" + lv.LVName,
				Format:    "raw",
				SizeBytes: int64(size),
				IsBlock:   true,
			})
		}
	}
	return volumes, nil
}

func (l *lvmDriver) CloneVolume(ctx context.Context, pool Pool, source, target string, _ bool) (Volume, error) {
	// A thin snapshot is the only clone shape LVM offers, and it is already
	// instant, so linked and full are the same operation here. Returning the
	// same thing for both is honest; pretending there is an independent copy
	// would not be.
	return l.CreateVolume(ctx, CreateVolumeRequest{
		Pool:        pool,
		Name:        target,
		Kind:        VolumeDisk,
		BackingPath: source,
	})
}

func (l *lvmDriver) Snapshot(ctx context.Context, pool Pool, path, name string) (Snapshot, error) {
	vg, _, err := splitPoolTarget(pool.Target)
	if err != nil {
		return Snapshot{}, err
	}
	if err := ValidateSnapshotName(name); err != nil {
		return Snapshot{}, err
	}
	source := lvNameFromPath(path)
	snapName := source + "_snap_" + name
	if err := validateLVMName(snapName); err != nil {
		return Snapshot{}, err
	}

	if out, err := runLVM(ctx, lvmTimeout, "lvcreate", "-s", "-n", snapName, vg+"/"+source); err != nil {
		if strings.Contains(out, "already exists") {
			return Snapshot{}, fmt.Errorf("%w: snapshot %s", ErrExists, name)
		}
		return Snapshot{}, fmt.Errorf("storage: snapshot %s: %w (%s)", source, err, firstLine(out))
	}

	size := errlog.Value(l.volumeSize(ctx, vg, snapName))
	return Snapshot{Handle: vg + "/" + snapName, SizeBytes: size, Mechanism: BackendLVM}, nil
}

// RevertSnapshot merges a thin snapshot back into its origin.
//
// lvconvert --merge is deferred while the origin is open, and takes effect on
// its next activation. The caller therefore reverts only a stopped guest, which
// is also what makes the operation safe — merging under a running guest would
// change the disk beneath it.
func (l *lvmDriver) RevertSnapshot(ctx context.Context, pool Pool, _ string, snap Snapshot) error {
	if err := assertWithinVG(pool, snap.Handle); err != nil {
		return err
	}
	if out, err := runLVM(ctx, lvmLongTimeout, "lvconvert", "--merge", snap.Handle); err != nil {
		return fmt.Errorf("storage: merge snapshot %s: %w (%s)", snap.Handle, err, firstLine(out))
	}
	return nil
}

func (l *lvmDriver) DeleteSnapshot(ctx context.Context, pool Pool, _ string, snap Snapshot) error {
	if err := assertWithinVG(pool, snap.Handle); err != nil {
		return err
	}
	if out, err := runLVM(ctx, lvmLongTimeout, "lvremove", "-f", snap.Handle); err != nil {
		if strings.Contains(out, "Failed to find") || strings.Contains(out, "not found") {
			return nil
		}
		return fmt.Errorf("storage: remove snapshot %s: %w (%s)", snap.Handle, err, firstLine(out))
	}
	return nil
}

// lvNameFromPath recovers a logical volume's name from a device path or a
// mountpoint.
func lvNameFromPath(path string) string {
	trimmed := strings.TrimSuffix(path, "/")
	if i := strings.LastIndex(trimmed, "/"); i >= 0 && i+1 < len(trimmed) {
		return trimmed[i+1:]
	}
	return trimmed
}

// assertWithinVG refuses to remove or merge anything outside the pool's own
// volume group — the LVM counterpart of the path and dataset checks in the
// other drivers, and for the same reason.
func assertWithinVG(pool Pool, handle string) error {
	vg, _, err := splitPoolTarget(pool.Target)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(handle, vg+"/") {
		return fmt.Errorf("storage: refusing to operate on %q outside volume group %q", handle, vg)
	}
	return nil
}

// runLVM executes an lvm2 tool with an explicit timeout and an argv.
func runLVM(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	return runCommand(ctx, timeout, name, args...)
}
