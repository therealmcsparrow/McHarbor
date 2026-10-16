// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// linuxSnapshotDrivers is the probe order for detection. ZFS first
// because it is the only one that exposes a frozen view without a
// mount call, then btrfs, then LVM, which needs a real mount and is
// therefore the most intrusive of the three.
var linuxSnapshotDrivers = []string{"zfs", "btrfs", "lvm"}

// lvmSnapshotMountRoot is where the agent mounts LVM snapshot volumes
// so the backup can read a frozen copy. One directory per snapshot,
// removed by snapshotRemove.
const lvmSnapshotMountRoot = "/var/lib/mcharbor/snapshots"

// snapshotDetect probes every Linux driver and reports what this host
// can actually do. Unlike capability it never stops at the first hit:
// the operator is choosing between drivers and needs to see all of
// them, including the ones that are missing and why.
func snapshotDetect(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	out := make([]SnapshotDriverOut, 0, len(linuxSnapshotDrivers))
	for _, driver := range linuxSnapshotDrivers {
		probe := payload
		probe.Driver = driver
		capability, err := driverCapability(ctx, driver, probe)
		if err != nil || capability == nil {
			message := "probe failed"
			if err != nil {
				message = err.Error()
			}
			out = append(out, SnapshotDriverOut{Driver: driver, Available: false, Error: message})
			continue
		}
		out = append(out, SnapshotDriverOut{
			Driver:    driver,
			ToolPath:  capability.ToolPath,
			Version:   capability.Version,
			Available: capability.Available,
			Error:     capability.Error,
			Targets:   capability.TargetsOut,
		})
	}
	return &SnapshotPayload{Operation: "detect", Success: true, Detected: out}, nil
}

// snapshotCapability probes one driver. When the request names no
// driver it falls back to the first one that is actually usable, which
// keeps the legacy "tell me what this host can do" call working.
func snapshotCapability(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	if driver := strings.TrimSpace(payload.Driver); driver != "" {
		return driverCapability(ctx, driver, payload)
	}
	for _, driver := range linuxSnapshotDrivers {
		capability, err := driverCapability(ctx, driver, payload)
		if err == nil && capability != nil && capability.Available {
			return capability, nil
		}
	}
	return &SnapshotPayload{
		Driver:    payload.Driver,
		Available: false,
		Error:     "no snapshot-capable CLI installed (zfs, btrfs, or lvm)",
	}, nil
}

func driverCapability(ctx context.Context, driver string, payload SnapshotPayload) (*SnapshotPayload, error) {
	switch driver {
	case "zfs":
		path, ok := which("zfs")
		if !ok {
			return unavailableDriver("zfs", "the zfs CLI is not on PATH"), nil
		}
		return zfsCapability(ctx, path, payload)
	case "btrfs":
		path, ok := which("btrfs")
		if !ok {
			return unavailableDriver("btrfs", "the btrfs CLI is not on PATH"), nil
		}
		return btrfsCapability(ctx, path, payload)
	case "lvm":
		path, ok := which("lvs")
		if !ok {
			return unavailableDriver("lvm", "the lvs CLI is not on PATH (install lvm2)"), nil
		}
		return lvmCapability(ctx, path, payload)
	}
	return unavailableDriver(driver, fmt.Sprintf("unsupported snapshot driver %q on linux", driver)), nil
}

func unavailableDriver(driver, reason string) *SnapshotPayload {
	return &SnapshotPayload{Driver: driver, Available: false, Error: reason}
}

// snapshotCreate delegates to the driver-specific create. The driver
// string comes from the host so the agent does not re-discover it.
func snapshotCreate(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	switch payload.Driver {
	case "zfs":
		return zfsCreate(ctx, payload)
	case "btrfs":
		return btrfsCreate(ctx, payload)
	case "lvm":
		return lvmCreate(ctx, payload)
	}
	return &SnapshotPayload{
		Driver:  payload.Driver,
		Success: false,
		Error:   fmt.Sprintf("unsupported snapshot driver %q on linux", payload.Driver),
	}, nil
}

// snapshotRemove destroys a previously created snapshot.
func snapshotRemove(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	switch payload.Driver {
	case "zfs":
		return zfsRemove(ctx, payload)
	case "btrfs":
		return btrfsRemove(ctx, payload)
	case "lvm":
		return lvmRemove(ctx, payload)
	}
	return &SnapshotPayload{
		Driver:  payload.Driver,
		Success: false,
		Error:   fmt.Sprintf("unsupported snapshot driver %q on linux", payload.Driver),
	}, nil
}

// snapshotTag is the snapshot name the agent uses for a run. Keeping
// the run id in the name is what lets an operator reading
// `zfs list -t snapshot` match a snapshot back to a backup.
func snapshotTag(id string) string {
	if strings.TrimSpace(id) == "" {
		return "mcharbor-agent"
	}
	return "mcharbor-" + id
}

// ---------------------------------------------------------------- zfs

func zfsCapability(ctx context.Context, toolPath string, payload SnapshotPayload) (*SnapshotPayload, error) {
	out, err := runCmd(ctx, toolPath, "list", "-H", "-o", "name,mountpoint,used", "-t", "filesystem")
	if err != nil {
		return &SnapshotPayload{ //nolint:nilerr // the failure is reported to the server inside the SnapshotPayload Error field by protocol
			Driver:    "zfs",
			ToolPath:  toolPath,
			Available: false,
			Error:     "zfs list failed: " + err.Error(),
		}, nil
	}
	targets := parseZfsList(string(out))
	version := ""
	if v, versionErr := runCmd(ctx, toolPath, "version"); versionErr == nil {
		version = strings.SplitN(strings.TrimSpace(string(v)), "\n", 2)[0]
	}
	_ = payload
	return &SnapshotPayload{
		Driver:     "zfs",
		ToolPath:   toolPath,
		Version:    version,
		Available:  len(targets) > 0,
		TargetsOut: targets,
	}, nil
}

// parseZfsList turns `zfs list -H -o name,mountpoint,used` output into
// target rows. The used column is human readable, so UsedBytes stays 0
// rather than carrying a number wrong by a factor of 1024.
func parseZfsList(out string) []SnapshotTargetOut {
	var rows []SnapshotTargetOut
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		rows = append(rows, SnapshotTargetOut{
			Name:       fields[0],
			Mountpoint: fields[1],
			Driver:     "zfs",
		})
	}
	return rows
}

// zfsDatasetForPath finds the dataset whose mountpoint is the longest
// prefix of path, and returns the dataset name with that mountpoint. A
// caller passing a dataset name rather than a path gets it back
// unchanged, which is how a config with an explicit dataset prefix
// keeps working.
func zfsDatasetForPath(ctx context.Context, toolPath, path string) (string, string, bool) {
	out, err := runCmd(ctx, toolPath, "list", "-H", "-o", "name,mountpoint", "-t", "filesystem")
	if err != nil {
		return "", "", false
	}
	type datasetRow struct{ name, mount string }
	var rows []datasetRow
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == path {
			return fields[0], fields[1], true
		}
		rows = append(rows, datasetRow{fields[0], fields[1]})
	}
	sort.Slice(rows, func(i, j int) bool { return len(rows[i].mount) > len(rows[j].mount) })
	for _, row := range rows {
		if row.mount == "" || row.mount == "none" || row.mount == "legacy" {
			continue
		}
		if pathWithin(path, row.mount) {
			return row.name, row.mount, true
		}
	}
	return "", "", false
}

func zfsCreate(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	if len(payload.Targets) == 0 {
		return &SnapshotPayload{Driver: "zfs", Success: false, Error: "no targets"}, nil
	}
	toolPath, ok := which("zfs")
	if !ok {
		return &SnapshotPayload{Driver: "zfs", Success: false, Error: "the zfs CLI is not on PATH"}, nil
	}
	tag := snapshotTag(payload.SnapshotID)
	handles := make([]string, 0, len(payload.Targets))
	seen := map[string]bool{}
	mountPaths := map[string]string{}

	for _, target := range payload.Targets {
		dataset, mountpoint, found := zfsDatasetForPath(ctx, toolPath, target)
		if !found {
			return zfsRollback(ctx, toolPath, handles, fmt.Sprintf("no zfs dataset holds %q", target)), nil
		}
		handle := dataset + "@" + tag
		if !seen[handle] {
			if out, err := runCmd(ctx, toolPath, "snapshot", handle); err != nil {
				return zfsRollback(ctx, toolPath, handles, "zfs snapshot failed: "+err.Error()+": "+strings.TrimSpace(string(out))), nil //nolint:nilerr // the failure is reported to the server inside the SnapshotPayload Error field by protocol
			}
			seen[handle] = true
			handles = append(handles, handle)
		}
		// The .zfs/snapshot directory is a frozen read-only view of the
		// dataset at snapshot time. Reading the backup from here is
		// what makes the pause unnecessary.
		frozen := filepath.Join(mountpoint, ".zfs", "snapshot", tag)
		if rel, err := filepath.Rel(mountpoint, filepath.Clean(target)); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			frozen = filepath.Join(frozen, rel)
		}
		mountPaths[target] = frozen
	}

	return &SnapshotPayload{
		Driver:       "zfs",
		Success:      true,
		SnapshotName: strings.Join(handles, ","),
		MountPath:    mountPaths[payload.Targets[0]],
		MountPaths:   mountPaths,
		Targets:      payload.Targets,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// zfsRollback destroys the snapshots taken so far after a later target
// failed, so a partial create never leaves orphans behind.
func zfsRollback(ctx context.Context, toolPath string, handles []string, message string) *SnapshotPayload {
	for _, handle := range handles {
		if _, err := runCmd(ctx, toolPath, "destroy", handle); err != nil {
			continue
		}
	}
	return &SnapshotPayload{Driver: "zfs", Success: false, Error: message}
}

func zfsRemove(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	if payload.SnapshotName == "" {
		return &SnapshotPayload{Driver: "zfs", Success: false, Error: "snapshot name required"}, nil
	}
	toolPath, ok := which("zfs")
	if !ok {
		return &SnapshotPayload{Driver: "zfs", Success: false, Error: "the zfs CLI is not on PATH"}, nil
	}
	var failures []string
	for _, handle := range strings.Split(payload.SnapshotName, ",") {
		handle = strings.TrimSpace(handle)
		if handle == "" {
			continue
		}
		if out, err := runCmd(ctx, toolPath, "destroy", handle); err != nil {
			failures = append(failures, handle+": "+err.Error()+": "+strings.TrimSpace(string(out)))
		}
	}
	if len(failures) > 0 {
		return &SnapshotPayload{Driver: "zfs", Success: false, Error: "zfs destroy failed: " + strings.Join(failures, "; ")}, nil
	}
	return &SnapshotPayload{Driver: "zfs", Success: true}, nil
}

// -------------------------------------------------------------- btrfs

func btrfsCapability(ctx context.Context, toolPath string, payload SnapshotPayload) (*SnapshotPayload, error) {
	out, err := runCmd(ctx, toolPath, "subvolume", "list", "/")
	if err != nil {
		return &SnapshotPayload{ //nolint:nilerr // the failure is reported to the server inside the SnapshotPayload Error field by protocol
			Driver:    "btrfs",
			ToolPath:  toolPath,
			Available: false,
			Error:     "btrfs subvolume list failed: " + err.Error(),
		}, nil
	}
	version := ""
	if v, versionErr := runCmd(ctx, toolPath, "--version"); versionErr == nil {
		version = strings.TrimSpace(string(v))
	}
	targets := parseBtrfsList(string(out))
	_ = payload
	return &SnapshotPayload{
		Driver:     "btrfs",
		ToolPath:   toolPath,
		Version:    version,
		Available:  len(targets) > 0,
		TargetsOut: targets,
	}, nil
}

// parseBtrfsList reads `btrfs subvolume list` rows, which look like
// "ID 256 gen 5 top level 5 path @home".
func parseBtrfsList(out string) []SnapshotTargetOut {
	var rows []SnapshotTargetOut
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		var name string
		for i, field := range fields {
			if field == "path" && i+1 < len(fields) {
				name = "/" + strings.TrimPrefix(fields[i+1], "/")
				break
			}
		}
		if name == "" {
			continue
		}
		rows = append(rows, SnapshotTargetOut{Name: name, Mountpoint: name, Driver: "btrfs"})
	}
	return rows
}

// btrfsSubvolumeForPath walks up from path until `btrfs subvolume show`
// accepts a directory, which is the subvolume that holds the data.
func btrfsSubvolumeForPath(ctx context.Context, toolPath, path string) (string, bool) {
	current := filepath.Clean(path)
	for {
		if _, err := runCmd(ctx, toolPath, "subvolume", "show", current); err == nil {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func btrfsCreate(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	if len(payload.Targets) == 0 {
		return &SnapshotPayload{Driver: "btrfs", Success: false, Error: "no targets"}, nil
	}
	toolPath, ok := which("btrfs")
	if !ok {
		return &SnapshotPayload{Driver: "btrfs", Success: false, Error: "the btrfs CLI is not on PATH"}, nil
	}
	tag := snapshotTag(payload.SnapshotID)
	created := map[string]string{}
	var handles []string
	mountPaths := map[string]string{}

	for _, target := range payload.Targets {
		subvol, found := btrfsSubvolumeForPath(ctx, toolPath, target)
		if !found {
			return btrfsRollback(ctx, toolPath, handles, fmt.Sprintf("no btrfs subvolume holds %q", target)), nil
		}
		destination, done := created[subvol]
		if !done {
			// The snapshot is a sibling of the subvolume rather than a
			// child of it, so it never ends up inside the data being
			// archived.
			destination = filepath.Join(filepath.Dir(strings.TrimRight(subvol, "/")), filepath.Base(subvol)+"-"+tag)
			if out, err := runCmd(ctx, toolPath, "subvolume", "snapshot", "-r", subvol, destination); err != nil {
				return btrfsRollback(ctx, toolPath, handles, "btrfs snapshot failed: "+err.Error()+": "+strings.TrimSpace(string(out))), nil //nolint:nilerr // the failure is reported to the server inside the SnapshotPayload Error field by protocol
			}
			created[subvol] = destination
			handles = append(handles, destination)
		}
		frozen := destination
		if rel, err := filepath.Rel(subvol, filepath.Clean(target)); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			frozen = filepath.Join(destination, rel)
		}
		mountPaths[target] = frozen
	}

	return &SnapshotPayload{
		Driver:       "btrfs",
		Success:      true,
		SnapshotName: strings.Join(handles, ","),
		MountPath:    mountPaths[payload.Targets[0]],
		MountPaths:   mountPaths,
		Targets:      payload.Targets,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func btrfsRollback(ctx context.Context, toolPath string, handles []string, message string) *SnapshotPayload {
	for _, handle := range handles {
		if _, err := runCmd(ctx, toolPath, "subvolume", "delete", handle); err != nil {
			continue
		}
	}
	return &SnapshotPayload{Driver: "btrfs", Success: false, Error: message}
}

func btrfsRemove(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	if payload.SnapshotName == "" {
		return &SnapshotPayload{Driver: "btrfs", Success: false, Error: "snapshot name required"}, nil
	}
	toolPath, ok := which("btrfs")
	if !ok {
		return &SnapshotPayload{Driver: "btrfs", Success: false, Error: "the btrfs CLI is not on PATH"}, nil
	}
	var failures []string
	for _, handle := range strings.Split(payload.SnapshotName, ",") {
		handle = strings.TrimSpace(handle)
		if handle == "" {
			continue
		}
		if out, err := runCmd(ctx, toolPath, "subvolume", "delete", handle); err != nil {
			failures = append(failures, handle+": "+err.Error()+": "+strings.TrimSpace(string(out)))
		}
	}
	if len(failures) > 0 {
		return &SnapshotPayload{Driver: "btrfs", Success: false, Error: "btrfs delete failed: " + strings.Join(failures, "; ")}, nil
	}
	return &SnapshotPayload{Driver: "btrfs", Success: true}, nil
}

// ---------------------------------------------------------------- lvm

func lvmCapability(ctx context.Context, toolPath string, payload SnapshotPayload) (*SnapshotPayload, error) {
	out, err := runCmd(ctx, toolPath, "--noheadings", "--nosuffix", "-o", "lv_name,lv_path,lv_size", "--units", "b")
	if err != nil {
		return &SnapshotPayload{ //nolint:nilerr // the failure is reported to the server inside the SnapshotPayload Error field by protocol
			Driver:    "lvm",
			ToolPath:  toolPath,
			Available: false,
			Error:     "lvs failed: " + err.Error(),
		}, nil
	}
	version := ""
	if v, versionErr := runCmd(ctx, toolPath, "--version"); versionErr == nil {
		version = strings.SplitN(strings.TrimSpace(string(v)), "\n", 2)[0]
	}
	targets := parseLvsList(string(out))
	_ = payload
	return &SnapshotPayload{
		Driver:     "lvm",
		ToolPath:   toolPath,
		Version:    version,
		Available:  len(targets) > 0,
		TargetsOut: targets,
	}, nil
}

func parseLvsList(out string) []SnapshotTargetOut {
	var rows []SnapshotTargetOut
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			size = 0
		}
		rows = append(rows, SnapshotTargetOut{
			Name:       fields[0],
			Mountpoint: fields[1],
			UsedBytes:  size,
			Driver:     "lvm",
		})
	}
	return rows
}

// lvmVolumeForPath asks findmnt which block device backs path. LVM
// snapshots are per logical volume, so a container mount has to be
// mapped to its volume before anything can be frozen.
func lvmVolumeForPath(ctx context.Context, path string) (string, string, bool) {
	finder, found := which("findmnt")
	if !found {
		return "", "", false
	}
	out, err := runCmd(ctx, finder, "-n", "-o", "SOURCE,TARGET", "--target", path)
	if err != nil {
		return "", "", false
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return "", "", false
	}
	return fields[0], fields[1], true
}

// lvmCreate snapshots each backing logical volume and mounts the
// snapshot read-only so the backup can be read from a frozen copy. The
// copy-on-write space is fixed at 2G, the smallest allocation that
// reliably survives a long archive on a busy volume.
func lvmCreate(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	if len(payload.Targets) == 0 {
		return &SnapshotPayload{Driver: "lvm", Success: false, Error: "no targets"}, nil
	}
	lvcreate, ok := which("lvcreate")
	if !ok {
		return &SnapshotPayload{Driver: "lvm", Success: false, Error: "the lvcreate CLI is not on PATH (install lvm2)"}, nil
	}
	mountTool, mountFound := which("mount")
	if !mountFound {
		return &SnapshotPayload{Driver: "lvm", Success: false, Error: "the mount CLI is not on PATH"}, nil
	}
	tag := snapshotTag(payload.SnapshotID)
	created := map[string]string{}
	var handles []string
	mountPaths := map[string]string{}

	for i, target := range payload.Targets {
		device, mountpoint, found := lvmVolumeForPath(ctx, target)
		if !found {
			return lvmRollback(ctx, handles, fmt.Sprintf("no logical volume backs %q", target)), nil
		}
		snapshotDir, done := created[device]
		if !done {
			name := fmt.Sprintf("%s-%d", tag, i)
			if out, err := runCmd(ctx, lvcreate, "-s", "-L", "2G", "-n", name, device); err != nil {
				return lvmRollback(ctx, handles, "lvcreate failed: "+err.Error()+": "+strings.TrimSpace(string(out))), nil //nolint:nilerr // the failure is reported to the server inside the SnapshotPayload Error field by protocol
			}
			snapshotDevice := filepath.Join(filepath.Dir(device), name)
			snapshotDir = filepath.Join(lvmSnapshotMountRoot, name)
			if err := os.MkdirAll(snapshotDir, 0o750); err != nil {
				return lvmRollback(ctx, append(handles, snapshotDevice), "creating snapshot mount dir: "+err.Error()), nil //nolint:nilerr // the failure is reported to the server inside the SnapshotPayload Error field by protocol
			}
			if out, err := runCmd(ctx, mountTool, "-o", "ro", snapshotDevice, snapshotDir); err != nil {
				return lvmRollback(ctx, append(handles, snapshotDevice), "mounting lvm snapshot: "+err.Error()+": "+strings.TrimSpace(string(out))), nil //nolint:nilerr // the failure is reported to the server inside the SnapshotPayload Error field by protocol
			}
			created[device] = snapshotDir
			handles = append(handles, snapshotDevice)
		}
		frozen := snapshotDir
		if rel, err := filepath.Rel(mountpoint, filepath.Clean(target)); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			frozen = filepath.Join(snapshotDir, rel)
		}
		mountPaths[target] = frozen
	}

	return &SnapshotPayload{
		Driver:       "lvm",
		Success:      true,
		SnapshotName: strings.Join(handles, ","),
		MountPath:    mountPaths[payload.Targets[0]],
		MountPaths:   mountPaths,
		Targets:      payload.Targets,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func lvmRollback(ctx context.Context, handles []string, message string) *SnapshotPayload {
	for _, handle := range handles {
		if err := lvmTeardown(ctx, handle); err != nil {
			continue
		}
	}
	return &SnapshotPayload{Driver: "lvm", Success: false, Error: message}
}

// lvmTeardown unmounts a snapshot volume and removes it. The unmount is
// best-effort: failing because nothing was mounted must not stop the
// lvremove that follows it.
func lvmTeardown(ctx context.Context, snapshotDevice string) error {
	dir := filepath.Join(lvmSnapshotMountRoot, filepath.Base(snapshotDevice))
	if umount, ok := which("umount"); ok {
		if _, err := runCmd(ctx, umount, dir); err != nil {
			// Already unmounted, or never mounted. Either way the
			// volume below still has to go.
			_ = err
		}
	}
	if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing snapshot mount dir: %w", err)
	}
	lvremove, ok := which("lvremove")
	if !ok {
		return errors.New("the lvremove CLI is not on PATH")
	}
	if out, err := runCmd(ctx, lvremove, "-f", snapshotDevice); err != nil {
		return fmt.Errorf("lvremove failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func lvmRemove(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	if payload.SnapshotName == "" {
		return &SnapshotPayload{Driver: "lvm", Success: false, Error: "snapshot name required"}, nil
	}
	var failures []string
	for _, handle := range strings.Split(payload.SnapshotName, ",") {
		handle = strings.TrimSpace(handle)
		if handle == "" {
			continue
		}
		if err := lvmTeardown(ctx, handle); err != nil {
			failures = append(failures, handle+": "+err.Error())
		}
	}
	if len(failures) > 0 {
		return &SnapshotPayload{Driver: "lvm", Success: false, Error: strings.Join(failures, "; ")}, nil
	}
	return &SnapshotPayload{Driver: "lvm", Success: true}, nil
}

// ------------------------------------------------------------ helpers

// pathWithin reports whether path sits inside root, comparing whole
// path segments so /var/lib2 is never treated as living under /var/lib.
func pathWithin(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if root == "/" {
		return true
	}
	return path == root || strings.HasPrefix(path, root+"/")
}

// which looks up a CLI tool on PATH. A missing tool means an
// unavailable driver, not an error worth propagating.
func which(name string) (string, bool) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return path, true
}

// runCmd runs a CLI tool and captures combined output. The context
// carries the caller's deadline so a wedged snapshot tool cannot hang
// the agent's read loop.
func runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "" {
		return nil, errors.New("empty command path")
	}
	// #nosec G204 -- name is resolved through exec.LookPath in `which`,
	// and args are built from driver-native identifiers, never from raw
	// user input.
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}
