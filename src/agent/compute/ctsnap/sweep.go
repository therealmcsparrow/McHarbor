// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ctsnap

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
)

// StaleMounts selects the mounts under root that belong to no run in flight,
// deepest first so a nested mount comes out before the one it sits on.
func StaleMounts(entries []MountEntry, root string, active *Active) []string {
	root = path.Clean(root)
	var out []string
	for _, entry := range entries {
		mountPoint := path.Clean(entry.MountPoint)
		if !strings.HasPrefix(mountPoint, root+"/") {
			continue
		}
		first, _, _ := strings.Cut(strings.TrimPrefix(mountPoint, root+"/"), "/")
		runID, ours := RunIDFromName(first)
		if !ours || active.Has(runID) {
			continue
		}
		out = append(out, mountPoint)
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// StaleDirs selects the directories under the mount root that belong to no run
// in flight. The caller removes them with rmdir, never recursively: a directory
// that is somehow still a mount must fail rather than empty the snapshot.
func StaleDirs(names []string, active *Active) []string {
	var out []string
	for _, name := range names {
		if runID, ours := RunIDFromName(name); ours && !active.Has(runID) {
			out = append(out, name)
		}
	}
	return out
}

// ListLVs reads every logical volume with its origin, which is what separates a
// snapshot of ours from an ordinary volume that happens to share the prefix.
func ListLVs(ctx context.Context, run Runner) ([]LV, error) {
	out, err := run(ctx, commandTimeout, "lvs", "--reportformat", "json", "-o", "vg_name,lv_name,origin")
	if err != nil {
		return nil, fmt.Errorf("ctsnap: listing logical volumes: %w (%s)", err, firstLine(out))
	}
	return parseLVReport(out)
}

// StaleLVs selects the snapshot volumes this package created for runs no longer
// in flight, as vg/lv. A volume with no origin is never selected, whatever its name.
func StaleLVs(rows []LV, active *Active) []string {
	var out []string
	for _, row := range rows {
		if row.Origin == "" || !validLVMName(row.VG) {
			continue
		}
		if runID, ours := RunIDFromName(row.Name); ours && !active.Has(runID) {
			out = append(out, row.VG+"/"+row.Name)
		}
	}
	return out
}

// ListZFSSnapshots names the snapshots of dataset, or of every dataset when it
// is empty; one dataset is what a backup sweeps, the whole host is what startup does.
func ListZFSSnapshots(ctx context.Context, run Runner, dataset string) ([]string, error) {
	args := []string{"list", "-H", "-t", "snapshot", "-o", "name"}
	if dataset != "" {
		args = append(args, "-d", "1", dataset)
	}
	out, err := run(ctx, commandTimeout, "zfs", args...)
	if err != nil {
		return nil, fmt.Errorf("ctsnap: listing zfs snapshots: %w (%s)", err, firstLine(out))
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// StaleZFSSnapshots selects the snapshots of ours whose run is no longer in
// flight. Only a snapshot is ever selected; a dataset never is.
func StaleZFSSnapshots(names []string, active *Active) []string {
	var out []string
	for _, name := range names {
		dataset, snap, ok := strings.Cut(name, "@")
		if !ok || !validDataset(dataset) {
			continue
		}
		if runID, ours := RunIDFromName(snap); ours && !active.Has(runID) {
			out = append(out, name)
		}
	}
	return out
}

// StaleSiblings selects the dir and btrfs pool copies of rootfs left by runs no
// longer in flight. names are the entries of rootfs's parent directory, and the
// match is exact so no other container's copy can be selected.
func StaleSiblings(rootfs string, names []string, active *Active) []string {
	rootfs = path.Clean(rootfs)
	stem := path.Base(rootfs) + ".snap."
	var out []string
	for _, name := range names {
		if !strings.HasPrefix(name, stem) {
			continue
		}
		if runID, ours := RunIDFromName(strings.TrimPrefix(name, stem)); ours && !active.Has(runID) {
			out = append(out, path.Join(path.Dir(rootfs), name))
		}
	}
	return out
}
