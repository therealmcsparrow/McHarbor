// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// errCTSnapshotNoSpec is the refusal for a snapshot request from a plugin too old
// to say where the rootfs is. Guessing a path on a root-privileged host is not
// an option.
var errCTSnapshotNoSpec = errors.New("compute: container snapshots need the container spec; " +
	"update the System Containers plugin")

// handleCTSnapshot answers ct_snapshot_create, ct_snapshot_revert and
// ct_snapshot_delete. See ct_snapshot_paths.go for why a snapshot is a copy.
func (rt *computeRuntime) handleCTSnapshot(ctx context.Context, messageType string, req *CTPayload, reply *CTPayload) error {
	if req.Spec == nil {
		return errCTSnapshotNoSpec
	}
	snapshot, err := ctSnapshotRootfs(req.Spec.RootfsPath, req.Spec.PoolTarget, req.ContainerID, req.SnapshotID)
	if err != nil {
		return err
	}
	shift := lxc.ShiftFor(toCTSpec(req.ContainerID, req.Spec))

	switch messageType {
	case MsgCTSnapshotCreate:
		// The plugin freezes a running container around this call, so the copy
		// is of one moment; unfrozen, it is crash-consistent and recorded so.
		if err := lxc.CopyTemplateRootfs(ctx, req.Spec.RootfsPath, snapshot, shift); err != nil {
			removeSnapshotDir(snapshot)
			return fmt.Errorf("compute: copying the rootfs into the snapshot: %w", err)
		}
		reply.SnapshotName = filepath.Base(filepath.Dir(snapshot))
		reply.SizeBytes = treeBytes(snapshot)
		return nil

	case MsgCTSnapshotRevert:
		if running, err := rt.ctRunning(ctx, req.ContainerID); err != nil {
			return err
		} else if running {
			return fmt.Errorf("compute: %s is running; stop it before reverting", req.ContainerID)
		}
		if _, err := os.Stat(snapshot); err != nil {
			return fmt.Errorf("compute: snapshot %s is not on this host: %w", req.SnapshotID, err)
		}
		return revertCTRootfs(ctx, req.Spec.RootfsPath, snapshot, shift)

	case MsgCTSnapshotDelete:
		if err := os.RemoveAll(filepath.Dir(snapshot)); err != nil {
			return fmt.Errorf("compute: removing snapshot %s: %w", req.SnapshotID, err)
		}
		return nil
	}
	return fmt.Errorf("compute: %s is not a snapshot operation", messageType)
}

// revertCTRootfs replaces a stopped container's rootfs with a snapshot's copy.
//
// The current tree is moved aside first and only removed once the copy is in
// place, so a revert that fails half way puts the container back exactly as it
// was rather than leaving it with no filesystem.
func revertCTRootfs(ctx context.Context, rootfs, snapshot string, shift lxc.IDShift) error {
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return fmt.Errorf("compute: naming the revert scratch: %w", err)
	}
	aside := rootfs + ".revert-" + hex.EncodeToString(suffix)
	if err := os.Rename(rootfs, aside); err != nil {
		return fmt.Errorf("compute: moving the current rootfs aside: %w", err)
	}
	if err := lxc.CopyTemplateRootfs(ctx, snapshot, rootfs, shift); err != nil {
		if removeErr := os.RemoveAll(rootfs); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			return errors.Join(fmt.Errorf("compute: restoring the snapshot: %w", err),
				fmt.Errorf("compute: the original rootfs is kept at %s: %w", aside, removeErr))
		}
		if renameErr := os.Rename(aside, rootfs); renameErr != nil {
			return errors.Join(fmt.Errorf("compute: restoring the snapshot: %w", err),
				fmt.Errorf("compute: the original rootfs is kept at %s: %w", aside, renameErr))
		}
		return fmt.Errorf("compute: restoring the snapshot, the container is unchanged: %w", err)
	}
	if err := os.RemoveAll(aside); err != nil {
		return fmt.Errorf("compute: the snapshot is restored but the old tree at %s was not removed: %w", aside, err)
	}
	return nil
}

// removeSnapshotDir drops a snapshot directory a failed create left behind.
func removeSnapshotDir(snapshot string) {
	if err := os.RemoveAll(filepath.Dir(snapshot)); err != nil {
		fmt.Fprintf(os.Stderr, "compute: removing a failed snapshot %s: %v\n", snapshot, err)
	}
}

// removeCTSnapshots drops every snapshot of a container whose rootfs is being
// removed; snapshots of a container that no longer exists are unreachable.
func removeCTSnapshots(ctID, rootfs, poolTarget string) error {
	matches, err := filepath.Glob(ctSnapshotGlob(rootfs, ctID))
	if err != nil {
		return fmt.Errorf("compute: listing %s's snapshots: %w", ctID, err)
	}
	var failures []error
	for _, dir := range matches {
		if err := lxc.CheckRootfsRemovable(filepath.Join(dir, "rootfs"), poolTarget); err != nil {
			failures = append(failures, err)
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			failures = append(failures, fmt.Errorf("compute: removing %s: %w", dir, err))
		}
	}
	return errors.Join(failures...)
}

// treeBytes sums the apparent size of a tree, best effort, for the UI's size
// column. A walk error stops the count; the partial total is still the answer.
func treeBytes(root string) int64 {
	var total int64
	errlog.Log(filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			if info, err := entry.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	}))
	return total
}

// ctRunning reports whether a container has a live init.
func (rt *computeRuntime) ctRunning(ctx context.Context, ctID string) (bool, error) {
	states, _, err := rt.cts.Query(ctx, []string{ctID})
	if err != nil {
		return false, err
	}
	for _, state := range states {
		if state.ContainerID == ctID && (state.State == lxc.StateRunning || state.State == lxc.StateFrozen) {
			return true, nil
		}
	}
	return false, nil
}

// handleCTClone answers ct_clone_request by creating the new container from the
// source's rootfs, re-owned into the clone's own id range.
//
// That re-owning is why a container clone is always a full copy: every inode
// changes owner, so nothing can be shared with the source. The reply says so,
// and the plugin records the clone as independent — deleting the source stays
// safe. A clone is torn down again if it fails or is cancelled with ct_cancel
// on the clone's id.
func (rt *computeRuntime) handleCTClone(ctx context.Context, req *CTPayload, reply *CTPayload) error {
	if req.Clone == nil || req.Clone.Spec == nil {
		return errors.New("compute: ct_clone_request carried no clone spec")
	}
	if err := validateGuestID(req.ContainerID); err != nil {
		return err
	}
	target := req.Clone.TargetContainerID
	if err := validateGuestID(target); err != nil {
		return err
	}
	source := req.Clone.Spec.TemplatePath
	if req.Spec != nil && req.Spec.RootfsPath != "" {
		source = req.Spec.RootfsPath
	}
	if source == "" {
		return errCTSnapshotNoSpec
	}
	if running, err := rt.ctRunning(ctx, req.ContainerID); err != nil {
		return err
	} else if running {
		return fmt.Errorf("compute: %s is running; stop it before cloning", req.ContainerID)
	}

	cloneCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	rt.cts.RegisterCancel(target, cancel)
	defer rt.cts.Cancel(target)

	spec := *req.Clone.Spec
	spec.TemplatePath = source
	if err := rt.createCT(cloneCtx, &CTPayload{ContainerID: target, Spec: &spec}); err != nil {
		if cloneCtx.Err() != nil && ctx.Err() == nil {
			return fmt.Errorf("compute: the clone of %s was cancelled", req.ContainerID)
		}
		return err
	}
	reply.Clone = &CTCloneSpec{TargetContainerID: target, Linked: false}
	reply.Percent = 100
	return nil
}
