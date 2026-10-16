// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/snapshotdrivers"
)

// snapshotHold is what a backup run carries between taking a snapshot
// and releasing it. The zero value means "no snapshot was taken",
// which is the state every plan in pause or live mode stays in.
type snapshotHold struct {
	active bool
	driver string
	ref    plugin.SnapshotRef
	// mountPaths maps a container mount destination to the frozen host
	// path holding that mount's data. Empty when the driver could not
	// expose a readable view, which is the signal to keep pausing.
	mountPaths map[string]string
}

// pauseFree reports whether the archive can be read without stopping
// the container. It takes both a live snapshot and at least one
// readable path: a snapshot nobody can read saves no downtime.
func (h snapshotHold) pauseFree() bool {
	return h.active && len(h.mountPaths) > 0
}

// freezeForBackup takes the snapshot a plan asked for. Every failure
// path returns the zero hold rather than an error: a backup that
// cannot freeze must still run, and the caller falls back to pausing.
// The reason is logged and recorded on the run so the operator can see
// why a plan they configured for snapshots paused anyway.
func (s *Service) freezeForBackup(ctx context.Context, plan *BackupPlan, runID string) snapshotHold {
	if plan == nil || plan.ConsistencyMode != ConsistencySnapshot {
		return snapshotHold{}
	}
	if plan.SnapshotConfigID == "" {
		s.noteSnapshotFallback(ctx, runID, "plan asks for snapshot consistency but names no snapshot configuration")
		return snapshotHold{}
	}
	config, err := s.SnapshotConfigByID(ctx, plan.SnapshotConfigID)
	if err != nil || config == nil {
		s.noteSnapshotFallback(ctx, runID, fmt.Sprintf("snapshot configuration %s could not be loaded", plan.SnapshotConfigID))
		return snapshotHold{}
	}
	if !config.Enabled {
		s.noteSnapshotFallback(ctx, runID, fmt.Sprintf("snapshot configuration %q is disabled", config.Name))
		return snapshotHold{}
	}

	targets := s.resolveSnapshotTargets(ctx, plan, config)
	if len(targets) == 0 {
		s.noteSnapshotFallback(ctx, runID, "no snapshot targets resolved from the plan's mounts")
		return snapshotHold{}
	}
	request := plugin.SnapshotCreateRequest{
		Targets:    targets,
		SnapshotID: runID,
		ReadOnly:   true,
		Timeout:    3600,
	}

	s.updateRunProgress(ctx, runID, "snapshot", "")

	// Builtin drivers can hand back the frozen path of every target,
	// which is what the archive needs. A plugin-owned driver only
	// promises a single mount path, so it gets the narrower call.
	if snapshotdrivers.IsBuiltin(config.Driver) && s.builtinSnapshots != nil {
		if _, _, pluginOwned := s.pluginSnapshotAdapter(config.Driver); !pluginOwned {
			ref, paths, createErr := s.builtinSnapshots.CreateWithPaths(ctx, config.EnvironmentID, config.Driver, request)
			if createErr != nil {
				s.noteSnapshotFallback(ctx, runID, createErr.Error())
				return snapshotHold{}
			}
			s.persistSnapshotRef(ctx, runID, ref, config.Driver)
			return snapshotHold{active: true, driver: config.Driver, ref: ref, mountPaths: paths}
		}
	}

	ref, driver, err := s.snapshotForBackup(ctx, plan, runID)
	if err != nil {
		s.noteSnapshotFallback(ctx, runID, err.Error())
		return snapshotHold{}
	}
	s.persistSnapshotRef(ctx, runID, ref, driver)
	hold := snapshotHold{active: true, driver: driver, ref: ref}
	if ref.MountPath != "" && len(targets) == 1 {
		hold.mountPaths = map[string]string{targets[0]: ref.MountPath}
	}
	return hold
}

// pluginSnapshotAdapter reports whether a plugin owns the driver, so
// the builtin path can step aside for it.
func (s *Service) pluginSnapshotAdapter(driver string) (plugin.SnapshotAdapterClient, plugin.SnapshotAdapterSpec, bool) {
	if s.snapshotAdapter == nil {
		return nil, plugin.SnapshotAdapterSpec{}, false
	}
	source, ok := s.snapshotAdapter.(plugin.SnapshotAdapterSource)
	if !ok {
		return nil, plugin.SnapshotAdapterSpec{}, false
	}
	return source.SnapshotAdapter(driver)
}

// releaseSnapshot destroys the snapshot after the archive is written.
// Best-effort by design: the archive is already on disk, and an
// operator would rather deal with a leaked snapshot than a failed
// backup. The outcome is stamped on the run either way.
func (s *Service) releaseSnapshot(ctx context.Context, plan *BackupPlan, runID string, hold snapshotHold) {
	if !hold.active {
		return
	}
	if err := s.removeSnapshotAfterBackup(ctx, plan, hold.ref, hold.driver); err != nil {
		if s.logger != nil {
			s.logger.Warn("container backup: snapshot removal failed",
				"run", runID, "driver", hold.driver, "snapshot", hold.ref.SnapshotName, "error", err)
		}
		s.markSnapshotRemoved(ctx, runID, err.Error())
		return
	}
	s.markSnapshotRemoved(ctx, runID, "")
}

// noteSnapshotFallback records why a plan configured for snapshots is
// pausing instead. Silence here would leave the operator believing
// their containers stay up during backups when they do not.
func (s *Service) noteSnapshotFallback(ctx context.Context, runID, reason string) {
	if s.logger != nil {
		s.logger.Warn("container backup: falling back to pause", "run", runID, "reason", reason)
	}
	s.markSnapshotRemoved(ctx, runID, "fell back to pause: "+reason)
}

// writeFrozenMount archives a mount from its frozen snapshot path
// instead of copying it out of the running container.
//
// A mount is stored in the archive as a nested tar (`mounts/<path>.tar`)
// holding exactly what `docker cp` would have streamed, so the restore
// path can stay unchanged. The frozen walk therefore builds that inner
// tar itself, spooled to a temp file so a large mount never has to fit
// in memory, and hands it to the same writeStream the live path uses.
func (s *Service) writeFrozenMount(ctx context.Context, tw *tar.Writer, entryName, frozenPath, mountDestination, tempDir string, onProgress func()) error {
	root, err := os.Stat(frozenPath)
	if err != nil {
		return fmt.Errorf("reading frozen mount %s: %w", frozenPath, err)
	}
	if err := os.MkdirAll(tempDir, 0o750); err != nil {
		return fmt.Errorf("creating snapshot spool directory: %w", err)
	}
	spool, err := os.CreateTemp(tempDir, "mcharbor-frozen-*.tar")
	if err != nil {
		return fmt.Errorf("creating snapshot spool file: %w", err)
	}
	spoolPath := spool.Name()
	defer func() {
		if removeErr := os.Remove(spoolPath); removeErr != nil && !os.IsNotExist(removeErr) && s.logger != nil {
			s.logger.Warn("container backup: removing snapshot spool file failed", "path", spoolPath, "error", removeErr)
		}
	}()

	// `docker cp` names its entries after the source's last path
	// element, and the restore splits on that, so the frozen walk uses
	// the same prefix.
	prefix := filepath.Base(strings.TrimRight(mountDestination, "/"))
	if prefix == "" || prefix == "/" || prefix == "." {
		prefix = "root"
	}

	inner := tar.NewWriter(spool)
	writeErr := writeFrozenTree(ctx, inner, frozenPath, prefix, root)
	closeErr := inner.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if syncErr := spool.Close(); writeErr == nil {
		writeErr = syncErr
	}
	if writeErr != nil {
		return fmt.Errorf("building frozen archive for %s: %w", mountDestination, writeErr)
	}

	reader, err := os.Open(spoolPath) // #nosec G304 -- path is the temp file created just above
	if err != nil {
		return fmt.Errorf("reading snapshot spool file: %w", err)
	}
	return writeStream(ctx, tw, entryName, reader, tempDir, onProgress)
}

// writeFrozenTree walks the frozen view and writes it into the inner
// tar under prefix.
func writeFrozenTree(ctx context.Context, tw *tar.Writer, frozenPath, prefix string, root os.FileInfo) error {
	if !root.IsDir() {
		return writeFrozenFile(tw, frozenPath, prefix, root)
	}
	return filepath.Walk(frozenPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, relErr := filepath.Rel(frozenPath, path)
		if relErr != nil {
			return relErr
		}
		name := prefix
		if relative != "." {
			name = prefix + "/" + filepath.ToSlash(relative)
		}
		return writeFrozenFile(tw, path, name, info)
	})
}

// writeFrozenFile writes one entry from the frozen view. Symlinks are
// stored as links rather than followed, so a link pointing outside the
// snapshot cannot pull unrelated host files into the archive.
func writeFrozenFile(tw *tar.Writer, path, name string, info os.FileInfo) error {
	link := ""
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return fmt.Errorf("reading symlink %s: %w", path, err)
		}
		link = target
	}
	header, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return fmt.Errorf("building header for %s: %w", path, err)
	}
	header.Name = name
	if info.IsDir() {
		header.Name = name + "/"
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("writing header for %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	file, err := os.Open(path) // #nosec G304 -- path comes from walking the snapshot the host itself created
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer closeLogged(file, "backup archive")
	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("copying %s: %w", path, err)
	}
	return nil
}

// frozenMountPath finds the snapshot path holding a container mount.
// The map is keyed by whatever the snapshot targets were, which is
// usually the mount destination but can be the bind source, so both
// are tried before giving up and reading the live path.
func frozenMountPath(mountPaths map[string]string, mountDestination string) (string, bool) {
	if len(mountPaths) == 0 {
		return "", false
	}
	if path, ok := mountPaths[mountDestination]; ok && path != "" {
		return path, true
	}
	clean := strings.TrimRight(mountDestination, "/")
	for target, path := range mountPaths {
		if path == "" {
			continue
		}
		if strings.TrimRight(target, "/") == clean {
			return path, true
		}
	}
	return "", false
}
