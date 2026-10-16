// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package ctsnap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/hostmnt"
)

// releaseTimeout bounds the whole of a release or a sweep step. It runs from a
// context detached from the backup's, because a cancelled run still has to clean up.
const releaseTimeout = 2 * time.Minute

// sweepMu serialises sweeps, so the startup sweep and a backup's never race to
// unmount the same directory.
var sweepMu sync.Mutex

// ExecRunner runs a host command for real, with the timeout and an argv rather
// than a shell string.
func ExecRunner(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

// Mounts reads the agent's own mount table. Not /proc/self: after an unprivileged
// start runc can leave the thread-group leader in a dead container's namespace.
func Mounts() ([]MountEntry, error) {
	data, err := hostmnt.Read("mountinfo")
	if err != nil {
		return nil, fmt.Errorf("ctsnap: reading the mount table: %w", err)
	}
	return ParseMountInfo(string(data)), nil
}

// TakeZFS snapshots the dataset and returns once the rootfs is readable through
// .zfs/snapshot, or destroys the snapshot again and reports why it is not.
func TakeZFS(ctx context.Context, run Runner, plan ZFSPlan) (release func() error, err error) {
	if out, err := run(ctx, commandTimeout, "zfs", "snapshot", plan.Snapshot); err != nil {
		return nil, fmt.Errorf("ctsnap: zfs snapshot %s: %w (%s)", plan.Snapshot, err, firstLine(out))
	}
	release = func() error {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
		defer cancel()
		return destroyZFS(releaseCtx, run, plan.Snapshot)
	}
	// The stat is what triggers the snapshot's automount, so it doubles as the
	// proof that snapdir, the mountpoint and the kernel all agree.
	info, statErr := os.Stat(plan.Tree)
	if statErr != nil || !info.IsDir() {
		cause := statErr
		if cause == nil {
			cause = errors.New("not a directory")
		}
		return nil, errors.Join(
			fmt.Errorf("%w: %s: %w", ErrNotReadable, plan.Tree, cause),
			release())
	}
	return release, nil
}

func destroyZFS(ctx context.Context, run Runner, snapshot string) error {
	out, err := run(ctx, commandTimeout, "zfs", "destroy", snapshot)
	if err != nil && !containsAny(out, "could not find", "does not exist") {
		return fmt.Errorf("ctsnap: zfs destroy %s: %w (%s)", snapshot, err, firstLine(out))
	}
	return nil
}

// TakeLVM creates, activates and mounts a thin snapshot read-only, and returns
// the call that undoes all three. Every failure undoes what was already done.
func TakeLVM(ctx context.Context, run Runner, plan LVMPlan) (release func() error, err error) {
	var undo []func(context.Context) error
	unwind := func() error {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
		defer cancel()
		var errs []error
		for i := len(undo) - 1; i >= 0; i-- {
			errs = append(errs, undo[i](releaseCtx))
		}
		return errors.Join(errs...)
	}
	fail := func(cause error) (func() error, error) {
		return nil, errors.Join(cause, unwind())
	}

	lv := plan.VG + "/" + plan.Snapshot
	if out, err := run(ctx, commandTimeout, "lvcreate", "-s", "-n", plan.Snapshot, plan.VG+"/"+plan.Origin); err != nil {
		return nil, fmt.Errorf("ctsnap: lvcreate snapshot of %s/%s: %w (%s)", plan.VG, plan.Origin, err, firstLine(out))
	}
	undo = append(undo, func(c context.Context) error { return removeLV(c, run, lv) })

	// A thin snapshot is created with activation skipped, so without -K its
	// device node never appears and the mount fails with nothing to explain it.
	if out, err := run(ctx, commandTimeout, "lvchange", "-ay", "-K", lv); err != nil {
		return fail(fmt.Errorf("ctsnap: activating %s: %w (%s)", lv, err, firstLine(out)))
	}

	if err := os.MkdirAll(MountRoot, 0o700); err != nil {
		return fail(fmt.Errorf("ctsnap: preparing %s: %w", MountRoot, err))
	}
	if err := os.Mkdir(plan.MountDir, 0o700); err != nil {
		return fail(fmt.Errorf("ctsnap: preparing %s: %w", plan.MountDir, err))
	}
	undo = append(undo, func(context.Context) error { return removeMountDir(plan.MountDir) })

	if out, err := run(ctx, commandTimeout, "mount", "-t", plan.FSType, "-o", plan.Options,
		plan.Device, plan.MountDir); err != nil {
		return fail(fmt.Errorf("ctsnap: mounting %s: %w (%s)", lv, err, firstLine(out)))
	}
	undo = append(undo, func(c context.Context) error { return unmount(c, run, plan.MountDir) })

	info, statErr := os.Stat(plan.Tree)
	if statErr != nil || !info.IsDir() {
		cause := statErr
		if cause == nil {
			cause = errors.New("not a directory")
		}
		return fail(fmt.Errorf("%w: %s: %w", ErrNotReadable, plan.Tree, cause))
	}
	return unwind, nil
}

func removeLV(ctx context.Context, run Runner, lv string) error {
	out, err := run(ctx, commandTimeout, "lvremove", "-f", lv)
	if err != nil && !containsAny(out, "Failed to find", "not found") {
		return fmt.Errorf("ctsnap: lvremove %s: %w (%s)", lv, err, firstLine(out))
	}
	return nil
}

// unmount tries a clean unmount first and a lazy one second: a lazy detach
// always succeeds, and a mount nobody can see any more is the lesser leak.
func unmount(ctx context.Context, run Runner, dir string) error {
	out, err := run(ctx, commandTimeout, "umount", dir)
	if err == nil || containsAny(out, "not mounted") {
		return nil
	}
	if lazyOut, lazyErr := run(ctx, commandTimeout, "umount", "-l", dir); lazyErr != nil {
		return fmt.Errorf("ctsnap: umount %s: %w (%s; lazy: %s)", dir, err, firstLine(out), firstLine(lazyOut))
	}
	return nil
}

// removeMountDir is rmdir, deliberately: removing recursively through a mount
// that failed to come out would empty the snapshot, or worse.
func removeMountDir(dir string) error {
	if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ctsnap: removing %s: %w", dir, err)
	}
	return nil
}

// SweepOptions says which pool mechanisms a sweep looks at. Mounts under
// MountRoot are always swept; they are cheap to find and the costliest to leak.
type SweepOptions struct {
	LVM bool
	// ZFSDataset scopes the ZFS sweep to one dataset's snapshots.
	ZFSDataset string
	// AllZFS sweeps every dataset on the host, which is what startup does.
	AllZFS bool
}

// Sweep removes what a killed agent left: mounts and directories under
// MountRoot, snapshot LVs and ZFS snapshots of ours, skipping every run in flight.
func Sweep(ctx context.Context, run Runner, active *Active, opts SweepOptions) error {
	sweepMu.Lock()
	defer sweepMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, releaseTimeout)
	defer cancel()

	var errs []error
	if mounts, err := Mounts(); err != nil {
		errs = append(errs, err)
	} else {
		for _, dir := range StaleMounts(mounts, MountRoot, active) {
			errs = append(errs, unmount(ctx, run, dir))
		}
	}

	if entries, err := os.ReadDir(MountRoot); err == nil {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				names = append(names, entry.Name())
			}
		}
		for _, name := range StaleDirs(names, active) {
			errs = append(errs, removeMountDir(MountRoot+"/"+name))
		}
	} else if !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("ctsnap: reading %s: %w", MountRoot, err))
	}

	if opts.LVM && onPath("lvs") {
		if rows, err := ListLVs(ctx, run); err != nil {
			errs = append(errs, err)
		} else {
			for _, lv := range StaleLVs(rows, active) {
				errs = append(errs, removeLV(ctx, run, lv))
			}
		}
	}

	if (opts.AllZFS || opts.ZFSDataset != "") && onPath("zfs") {
		dataset := opts.ZFSDataset
		if opts.AllZFS {
			dataset = ""
		}
		if names, err := ListZFSSnapshots(ctx, run, dataset); err != nil {
			errs = append(errs, err)
		} else {
			for _, name := range StaleZFSSnapshots(names, active) {
				errs = append(errs, destroyZFS(ctx, run, name))
			}
		}
	}
	return errors.Join(errs...)
}

func onPath(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
