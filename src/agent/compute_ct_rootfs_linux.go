// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
	"github.com/therealmcsparrow/mcharbor-agent/compute/storage"
)

// errCTDeleteNoSpec is the refusal for a delete that asks for the rootfs to go
// without saying where it is; replying success there left an invisible orphan.
var errCTDeleteNoSpec = errors.New("compute: ct_delete asked to remove the rootfs but carried no container spec; " +
	"update the System Containers plugin")

// createCT answers ct_create: it lays the rootfs down from the named image or
// template when there is one, then writes the libcontainer state. A rootfs this
// call laid down is removed again if the container cannot be created on it.
func (rt *computeRuntime) createCT(ctx context.Context, req *CTPayload) error {
	if req.Spec == nil {
		return fmt.Errorf("compute: ct_create carried no spec")
	}
	spec := toCTSpec(req.ContainerID, req.Spec)

	populated, err := rt.populateCTRootfs(ctx, req, spec)
	if err != nil {
		return err
	}
	if err := rt.cts.Create(ctx, spec); err != nil {
		if populated {
			if removeErr := os.RemoveAll(req.Spec.RootfsPath); removeErr != nil {
				fmt.Fprintf(os.Stderr, "compute: removing the rootfs of a failed create container=%s error=%v\n",
					req.ContainerID, removeErr)
			}
			removeEmptyContainerDir(req.ContainerID, req.Spec.RootfsPath, req.Spec.PoolTarget)
		}
		return err
	}
	return nil
}

// populateCTRootfs builds the rootfs a create names, reporting whether it did.
// No image and no template means the rootfs is already there, which is the
// recreate path: the restore laid it down first.
func (rt *computeRuntime) populateCTRootfs(ctx context.Context, req *CTPayload, spec lxc.Spec) (bool, error) {
	source := req.Spec.TemplatePath
	if req.ImageID == "" && source == "" {
		return false, nil
	}
	// The same confinement a delete gets: whatever a create writes must land
	// strictly inside the container's pool, never over a host path.
	if err := lxc.CheckRootfsRemovable(req.Spec.RootfsPath, req.Spec.PoolTarget); err != nil {
		return false, err
	}

	shift := lxc.ShiftFor(spec)
	if req.ImageID != "" {
		archive, err := rt.cachedRootfsImage(req.ImageID)
		if err != nil {
			return false, err
		}
		if err := lxc.UnpackRootfs(ctx, archive, req.Spec.RootfsPath, shift); err != nil {
			return false, err
		}
		// Best-effort: a container whose console shows no shell is still a
		// container, and failing the create over it would cost the operator more.
		done, err := lxc.PrepareConsoleInit(req.Spec.RootfsPath, shift)
		if err != nil {
			rt.logger.Warn("compute: preparing the console of a new container failed",
				"container", req.ContainerID, "error", err)
		} else if done != lxc.InittabUntouched {
			rt.logger.Info("compute: busybox init found; /etc/inittab set up for /dev/console",
				"container", req.ContainerID, "inittab", done)
		}
		return true, nil
	}
	if err := lxc.CopyTemplateRootfs(ctx, source, req.Spec.RootfsPath, shift); err != nil {
		return false, err
	}
	return true, nil
}

// cachedRootfsImage finds a rootfs image in this host's image cache. The cache
// is the image library's host side, so an image the operator has not fetched to
// this host is named as such rather than reported as a missing rootfs.
func (rt *computeRuntime) cachedRootfsImage(imageID string) (string, error) {
	if rt.images == nil {
		return "", fmt.Errorf("compute: this host has no image cache, so image %s cannot be unpacked", imageID)
	}
	cached, err := rt.images.List(nil)
	if err != nil {
		return "", err
	}
	for _, image := range cached {
		if image.ImageID == imageID {
			return image.LocalPath, nil
		}
	}
	return "", fmt.Errorf("compute: image %s is not in this host's image cache; download it to this host "+
		"from the image library first", imageID)
}

// deleteCT answers ct_delete. A rootfs removal is checked before anything is
// torn down, so a refused request leaves the container exactly as it was.
func (rt *computeRuntime) deleteCT(ctx context.Context, req *CTPayload) error {
	if req.DeleteRootfs {
		if req.Spec == nil {
			return errCTDeleteNoSpec
		}
		if err := lxc.CheckRootfsRemovable(req.Spec.RootfsPath, req.Spec.PoolTarget); err != nil {
			return err
		}
	}

	spec := lxc.Spec{ID: req.ContainerID}
	if req.Spec != nil {
		spec = toCTSpec(req.ContainerID, req.Spec)
	}
	if err := rt.cts.Delete(ctx, spec); err != nil {
		return err
	}
	if !req.DeleteRootfs {
		return nil
	}
	return rt.deleteCTRootfs(ctx, req.ContainerID, req.Spec)
}

// deleteCTRootfs removes a container's filesystem from its pool. The guard is
// re-checked against the resolved path, so a symlinked parent cannot redirect
// the recursive delete outside the pool.
func (rt *computeRuntime) deleteCTRootfs(ctx context.Context, ctID string, spec *CTSpec) error {
	if err := lxc.CheckRootfsRemovable(spec.RootfsPath, spec.PoolTarget); err != nil {
		return err
	}
	resolvedTarget, err := filepath.EvalSymlinks(spec.PoolTarget)
	if err != nil {
		return fmt.Errorf("compute: resolving pool %s: %w", spec.PoolTarget, err)
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(spec.RootfsPath))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("compute: resolving %s: %w", spec.RootfsPath, err)
	}
	resolved := filepath.Join(resolvedParent, filepath.Base(spec.RootfsPath))
	if err := lxc.CheckRootfsRemovable(resolved, resolvedTarget); err != nil {
		return err
	}

	driver, err := rt.pools.Get(spec.PoolBackend)
	if err != nil {
		return err
	}
	// Snapshots first: they are unreachable once the container is gone, and a
	// failure here stops the delete before anything is lost.
	if err := removeCTSnapshots(ctID, resolved, resolvedTarget); err != nil {
		return err
	}
	if err := driver.DeleteVolume(ctx, storage.Pool{
		ID:      spec.PoolID,
		Backend: spec.PoolBackend,
		Target:  resolvedTarget,
	}, resolved); err != nil {
		return err
	}
	removeEmptyContainerDir(ctID, resolved, resolvedTarget)
	return nil
}

// removeEmptyContainerDir drops the per-container directory a rootfs sat in and
// then the pool's containers/ directory, each only once empty: os.Remove is a
// plain rmdir, and EmptyDirsAfterRootfsRemoval never names the pool root.
func removeEmptyContainerDir(ctID, rootfs, poolTarget string) {
	for _, dir := range lxc.EmptyDirsAfterRootfsRemoval(ctID, filepath.ToSlash(rootfs), filepath.ToSlash(poolTarget)) {
		if err := os.Remove(filepath.FromSlash(dir)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			// Still holding something, such as another container: nothing
			// further up can be empty either.
			return
		}
	}
}
