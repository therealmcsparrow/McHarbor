// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"golang.org/x/sys/unix"
)

// stageRootfs returns the path runc is given as the container's rootfs.
//
// runc's init resolves that path inside the new user namespace (prepareRoot's
// MS_PRIVATE remount, then the self-bind and pivot_root), so every ancestor must
// be searchable by the mapped root, which a pool's 0700 directories are not.
// The rootfs is bind-mounted at a McHarbor-owned 0711 path instead, and the
// mount is private so nothing the container mounts propagates back to the host.
func (m *Manager) stageRootfs(spec Spec) (string, error) {
	if _, err := os.Stat(spec.RootfsPath); err != nil {
		return "", fmt.Errorf("lxc: rootfs for %s is missing at %s: %w", spec.Name, spec.RootfsPath, err)
	}
	if !needsStaging(spec) {
		return spec.RootfsPath, nil
	}
	layout, err := stageLayoutFor(m.stageBase, spec.ID)
	if err != nil {
		return "", err
	}
	// A container that exited on its own still has its previous start's mount;
	// binding over it would stack a second one under the same target.
	if err := releaseStage(layout); err != nil {
		return "", err
	}

	for _, dir := range layout.Owned {
		if err := os.MkdirAll(dir, stageDirMode); err != nil {
			return "", fmt.Errorf("lxc: create staging directory %s: %w", dir, err)
		}
		// MkdirAll honours the umask and leaves an existing directory as it
		// was, so the mode is set explicitly on every directory McHarbor owns.
		if err := os.Chmod(dir, stageDirMode); err != nil {
			return "", fmt.Errorf("lxc: open staging directory %s to the container's root: %w", dir, err)
		}
	}
	closed, err := untraversableAncestor(m.stageBase, func(dir string) (fs.FileMode, error) {
		info, err := os.Stat(dir)
		if err != nil {
			return 0, err
		}
		return info.Mode(), nil
	})
	if err != nil {
		return "", err
	}
	if closed != "" {
		return "", fmt.Errorf("lxc: %s is not searchable by other users, so an unprivileged container's root cannot reach its staged rootfs under %s",
			closed, m.stageBase)
	}

	if err := os.Mkdir(layout.Target, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("lxc: create staging mount point for %s: %w", spec.Name, err)
	}
	if err := unix.Mount(spec.RootfsPath, layout.Target, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		errlog.Log(releaseStage(layout))
		return "", fmt.Errorf("lxc: stage the rootfs of %s: %w", spec.Name, err)
	}
	if err := unix.Mount("", layout.Target, "", unix.MS_PRIVATE|unix.MS_REC, ""); err != nil {
		errlog.Log(releaseStage(layout))
		return "", fmt.Errorf("lxc: make the staged rootfs of %s private: %w", spec.Name, err)
	}
	return layout.Target, nil
}

// releaseStaged removes ctID's staged rootfs, if it has one. Safe to call for a
// privileged container or one never staged: an absent target is a no-op.
func (m *Manager) releaseStaged(ctID string) error {
	layout, err := stageLayoutFor(m.stageBase, ctID)
	if err != nil {
		return err
	}
	return releaseStage(layout)
}

// releaseStage unmounts a staging target and removes its directories.
//
// Removal is rmdir, never RemoveAll: if an unmount did not take, the directory
// is the container's live rootfs, and a recursive remove would empty it.
func releaseStage(layout stageLayout) error {
	if _, err := os.Lstat(layout.Target); errors.Is(err, fs.ErrNotExist) {
		// Never staged, or already released. Checked first because umount(2)
		// needs CAP_SYS_ADMIN even to report that nothing is mounted.
		if err := os.Remove(layout.Dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("lxc: remove staging directory %s: %w", layout.Dir, err)
		}
		return nil
	}
	for i := 0; i < maxStageUnmounts; i++ {
		err := unix.Unmount(layout.Target, unix.UMOUNT_NOFOLLOW)
		if err == nil {
			continue
		}
		if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOENT) {
			break
		}
		if errors.Is(err, unix.EBUSY) {
			if lazyErr := unix.Unmount(layout.Target, unix.UMOUNT_NOFOLLOW|unix.MNT_DETACH); lazyErr == nil {
				continue
			}
		}
		return fmt.Errorf("lxc: unmount staged rootfs %s: %w", layout.Target, err)
	}
	for _, dir := range []string{layout.Target, layout.Dir} {
		if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("lxc: remove staging directory %s: %w", dir, err)
		}
	}
	return nil
}

// SweepStaging releases every staged rootfs a killed agent left behind, keeping
// those of containers that may still be running, which outlive an agent restart.
func (m *Manager) SweepStaging(ctx context.Context) error {
	root := filepath.Join(m.stageBase, "ct")
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("lxc: read staging root: %w", err)
	}

	var errs []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if !entry.IsDir() {
			continue
		}
		ctID := entry.Name()
		if err := m.sweepOne(ctID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// sweepOne releases one container's staging unless it may still be running.
// It holds the container's op lock so a start cannot stage it mid-sweep.
func (m *Manager) sweepOne(ctID string) error {
	lock := m.lockFor(ctID)
	lock.Lock()
	defer lock.Unlock()

	p, err := probePresence(m.stateDir(ctID))
	if err != nil {
		return err
	}
	state := ""
	if p == presenceLoaded {
		state = StateUnknown
		if container, loadErr := libcontainer.Load(m.root, ctID); loadErr == nil {
			state = containerState(container)
		}
	}
	if keepStageOnSweep(p, state) {
		return nil
	}
	return m.releaseStaged(ctID)
}

// stageDirMode is the mode of every McHarbor-owned directory on the way to a
// staged rootfs: searchable by a container's mapped root, listable by nobody else.
const stageDirMode fs.FileMode = 0o711

// maxStageUnmounts bounds how many stacked mounts a release peels off one
// target, so a mount that keeps coming back cannot spin a stop forever.
const maxStageUnmounts = 8
