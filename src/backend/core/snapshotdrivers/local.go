// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package snapshotdrivers

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/snapshot"
)

// The local path covers environments McHarbor talks to directly rather
// than through an agent: a mounted Docker socket on the same machine.
// McHarbor itself runs in a container, so the snapshot CLIs are only
// reachable when the operator deliberately made them reachable, by
// bind-mounting the host tools and giving the container the privileges
// to use them. When they are absent, detection says so plainly instead
// of pretending the driver is broken.

// localUnsupported is returned for drivers that cannot work from
// inside the McHarbor container even when their CLI is present.
const localUnsupported = "this driver needs the McHarbor agent: mounting a snapshot volume is not something the McHarbor container can do for a local environment"

// detectLocal probes the McHarbor process's own PATH for each builtin
// driver.
func detectLocal(ctx context.Context, specs map[string]snapshot.SnapshotAdapterSpec) []Status {
	order := []string{DriverZFS, DriverBtrfs, DriverLVM}
	if runtime.GOOS == "windows" {
		order = []string{DriverVSS}
	}
	out := make([]Status, 0, len(order))
	for _, driver := range order {
		spec := specs[driver]
		capability := localCapability(ctx, driver)
		out = append(out, Status{
			Driver:      driver,
			DisplayName: displayName(spec, driver),
			Available:   capability.Available,
			Origin:      "local",
			ToolPath:    capability.ToolPath,
			Version:     capability.Version,
			Error:       capability.Error,
			Targets:     capability.Targets,
			PauseFree:   spec.Capabilities.SupportsMount,
		})
	}
	return out
}

// localCapability lists the snapshot targets one driver exposes to the
// McHarbor process.
func localCapability(ctx context.Context, driver string) snapshot.SnapshotCapability {
	switch driver {
	case DriverZFS:
		toolPath, ok := lookPath("zfs")
		if !ok {
			return unavailableLocal(driver, "the zfs CLI is not available to McHarbor (bind-mount it from the host, or manage this host with an agent)")
		}
		out, err := run(ctx, toolPath, "list", "-H", "-o", "name,mountpoint,used", "-t", "filesystem")
		if err != nil {
			return unavailableLocal(driver, "zfs list failed: "+err.Error())
		}
		targets := parseZFSRows(string(out))
		return snapshot.SnapshotCapability{
			Driver:    driver,
			ToolPath:  toolPath,
			Version:   firstLine(runQuiet(ctx, toolPath, "version")),
			Available: len(targets) > 0,
			Targets:   targets,
		}
	case DriverBtrfs:
		toolPath, ok := lookPath("btrfs")
		if !ok {
			return unavailableLocal(driver, "the btrfs CLI is not available to McHarbor (bind-mount it from the host, or manage this host with an agent)")
		}
		out, err := run(ctx, toolPath, "subvolume", "list", "/")
		if err != nil {
			return unavailableLocal(driver, "btrfs subvolume list failed: "+err.Error())
		}
		targets := parseBtrfsRows(string(out))
		return snapshot.SnapshotCapability{
			Driver:    driver,
			ToolPath:  toolPath,
			Version:   firstLine(runQuiet(ctx, toolPath, "--version")),
			Available: len(targets) > 0,
			Targets:   targets,
		}
	case DriverLVM:
		toolPath, ok := lookPath("lvs")
		if !ok {
			return unavailableLocal(driver, "the lvs CLI is not available to McHarbor (install lvm2 on the host and manage it with an agent)")
		}
		return snapshot.SnapshotCapability{
			Driver:    driver,
			ToolPath:  toolPath,
			Available: false,
			Error:     localUnsupported,
		}
	case DriverVSS:
		return unavailableLocal(driver, localUnsupported)
	}
	return unavailableLocal(driver, fmt.Sprintf("unknown snapshot driver %q", driver))
}

func unavailableLocal(driver, reason string) snapshot.SnapshotCapability {
	return snapshot.SnapshotCapability{Driver: driver, Available: false, Error: reason}
}

// localCreate freezes the targets using the CLIs visible to the
// McHarbor process. Only the two drivers that expose a frozen view
// without a mount call are supported here; the rest need an agent.
func localCreate(ctx context.Context, driver string, req snapshot.SnapshotCreateRequest) (snapshot.SnapshotRef, error) {
	tag := localTag(req.SnapshotID)
	switch driver {
	case DriverZFS:
		toolPath, ok := lookPath("zfs")
		if !ok {
			return snapshot.SnapshotRef{}, fmt.Errorf("snapshot zfs: the zfs CLI is not available to McHarbor")
		}
		datasets, err := zfsMountpoints(ctx, toolPath)
		if err != nil {
			return snapshot.SnapshotRef{}, err
		}
		var handles []string
		seen := map[string]bool{}
		for _, target := range req.Targets {
			dataset, _, found := longestMatch(datasets, target)
			if !found {
				return snapshot.SnapshotRef{}, rollbackLocal(ctx, toolPath, "destroy", handles, fmt.Errorf("snapshot zfs: no dataset holds %q", target))
			}
			handle := dataset + "@" + tag
			if seen[handle] {
				continue
			}
			if out, err := run(ctx, toolPath, "snapshot", handle); err != nil {
				return snapshot.SnapshotRef{}, rollbackLocal(ctx, toolPath, "destroy", handles,
					fmt.Errorf("snapshot zfs: %w: %s", err, strings.TrimSpace(string(out))))
			}
			seen[handle] = true
			handles = append(handles, handle)
		}
		return snapshot.SnapshotRef{
			SnapshotName: strings.Join(handles, ","),
			Targets:      req.Targets,
			MountPath:    localZFSMountPath(datasets, req.Targets, tag),
			CreatedAt:    time.Now().UTC(),
		}, nil
	case DriverBtrfs:
		toolPath, ok := lookPath("btrfs")
		if !ok {
			return snapshot.SnapshotRef{}, fmt.Errorf("snapshot btrfs: the btrfs CLI is not available to McHarbor")
		}
		var handles []string
		for _, target := range req.Targets {
			destination := filepath.Join(filepath.Dir(strings.TrimRight(target, "/")), filepath.Base(target)+"-"+tag)
			if out, err := run(ctx, toolPath, "subvolume", "snapshot", "-r", target, destination); err != nil {
				return snapshot.SnapshotRef{}, rollbackLocalBtrfs(ctx, toolPath, handles,
					fmt.Errorf("snapshot btrfs: %w: %s", err, strings.TrimSpace(string(out))))
			}
			handles = append(handles, destination)
		}
		mountPath := ""
		if len(handles) > 0 {
			mountPath = handles[0]
		}
		return snapshot.SnapshotRef{
			SnapshotName: strings.Join(handles, ","),
			Targets:      req.Targets,
			MountPath:    mountPath,
			CreatedAt:    time.Now().UTC(),
		}, nil
	}
	return snapshot.SnapshotRef{}, fmt.Errorf("snapshot %s: %s", driver, localUnsupported)
}

// localRemove destroys what localCreate made.
func localRemove(ctx context.Context, driver string, ref snapshot.SnapshotRef) error {
	handles := strings.Split(ref.SnapshotName, ",")
	var failures []string
	switch driver {
	case DriverZFS:
		toolPath, ok := lookPath("zfs")
		if !ok {
			return fmt.Errorf("snapshot zfs: the zfs CLI is not available to McHarbor")
		}
		for _, handle := range handles {
			handle = strings.TrimSpace(handle)
			if handle == "" {
				continue
			}
			if out, err := run(ctx, toolPath, "destroy", handle); err != nil {
				failures = append(failures, handle+": "+strings.TrimSpace(string(out)))
			}
		}
	case DriverBtrfs:
		toolPath, ok := lookPath("btrfs")
		if !ok {
			return fmt.Errorf("snapshot btrfs: the btrfs CLI is not available to McHarbor")
		}
		for _, handle := range handles {
			handle = strings.TrimSpace(handle)
			if handle == "" {
				continue
			}
			if out, err := run(ctx, toolPath, "subvolume", "delete", handle); err != nil {
				failures = append(failures, handle+": "+strings.TrimSpace(string(out)))
			}
		}
	default:
		return fmt.Errorf("snapshot %s: %s", driver, localUnsupported)
	}
	if len(failures) > 0 {
		return fmt.Errorf("snapshot %s remove failed: %s", driver, strings.Join(failures, "; "))
	}
	return nil
}

// localMountPaths maps each requested target to its frozen path, so a
// local backup can read the snapshot instead of the live directory.
func localMountPaths(ref snapshot.SnapshotRef, req snapshot.SnapshotCreateRequest) map[string]string {
	if ref.MountPath == "" || len(req.Targets) == 0 {
		return nil
	}
	// A single-target snapshot is the only case the local path can map
	// unambiguously; multi-target local snapshots fall back to pausing.
	if len(req.Targets) == 1 {
		return map[string]string{req.Targets[0]: ref.MountPath}
	}
	return nil
}

func localTag(id string) string {
	if strings.TrimSpace(id) == "" {
		return "mcharbor-local"
	}
	return "mcharbor-" + id
}

func rollbackLocal(ctx context.Context, toolPath, verb string, handles []string, cause error) error {
	for _, handle := range handles {
		if _, err := run(ctx, toolPath, verb, handle); err != nil {
			continue
		}
	}
	return cause
}

func rollbackLocalBtrfs(ctx context.Context, toolPath string, handles []string, cause error) error {
	for _, handle := range handles {
		if _, err := run(ctx, toolPath, "subvolume", "delete", handle); err != nil {
			continue
		}
	}
	return cause
}

type mountRow struct {
	name  string
	mount string
}

func zfsMountpoints(ctx context.Context, toolPath string) ([]mountRow, error) {
	out, err := run(ctx, toolPath, "list", "-H", "-o", "name,mountpoint", "-t", "filesystem")
	if err != nil {
		return nil, fmt.Errorf("listing zfs datasets: %w", err)
	}
	var rows []mountRow
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) < 2 {
			continue
		}
		rows = append(rows, mountRow{fields[0], fields[1]})
	}
	sort.Slice(rows, func(i, j int) bool { return len(rows[i].mount) > len(rows[j].mount) })
	return rows, nil
}

func longestMatch(rows []mountRow, path string) (string, string, bool) {
	clean := filepath.Clean(path)
	for _, row := range rows {
		if row.name == path {
			return row.name, row.mount, true
		}
	}
	for _, row := range rows {
		if row.mount == "" || row.mount == "none" || row.mount == "legacy" {
			continue
		}
		mount := filepath.Clean(row.mount)
		if mount == "/" || clean == mount || strings.HasPrefix(clean, mount+"/") {
			return row.name, row.mount, true
		}
	}
	return "", "", false
}

func localZFSMountPath(rows []mountRow, targets []string, tag string) string {
	if len(targets) == 0 {
		return ""
	}
	_, mount, ok := longestMatch(rows, targets[0])
	if !ok {
		return ""
	}
	frozen := filepath.Join(mount, ".zfs", "snapshot", tag)
	if rel, err := filepath.Rel(mount, filepath.Clean(targets[0])); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		frozen = filepath.Join(frozen, rel)
	}
	return frozen
}

func parseZFSRows(out string) []snapshot.SnapshotTarget {
	var rows []snapshot.SnapshotTarget
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		rows = append(rows, snapshot.SnapshotTarget{
			Name:       fields[0],
			Mountpoint: fields[1],
			Driver:     DriverZFS,
		})
	}
	return rows
}

func parseBtrfsRows(out string) []snapshot.SnapshotTarget {
	var rows []snapshot.SnapshotTarget
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i, field := range fields {
			if field != "path" || i+1 >= len(fields) {
				continue
			}
			name := "/" + strings.TrimPrefix(fields[i+1], "/")
			rows = append(rows, snapshot.SnapshotTarget{Name: name, Mountpoint: name, Driver: DriverBtrfs})
			break
		}
	}
	return rows
}

func firstLine(value string) string {
	return strings.TrimSpace(strings.SplitN(strings.TrimSpace(value), "\n", 2)[0])
}

func lookPath(name string) (string, bool) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return path, true
}

func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// #nosec G204 -- name comes from exec.LookPath and args are
	// driver-native identifiers, never raw user input.
	return exec.CommandContext(callCtx, name, args...).CombinedOutput()
}

func runQuiet(ctx context.Context, name string, args ...string) string {
	out, err := run(ctx, name, args...)
	if err != nil {
		return ""
	}
	return string(out)
}
