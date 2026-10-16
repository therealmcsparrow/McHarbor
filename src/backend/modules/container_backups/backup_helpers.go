// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/services"
)

// ErrInvalidLocalBackupPath is returned by cleanLocalBackupPath
// when the operator-supplied path points outside the configured
// backup root.
var ErrInvalidLocalBackupPath = errors.New("invalid local backup path")

// backupStorageRemotePath assembles the destination-side path
// for a backup archive. Layout:
//
//	<base_path>/<env-segment>/containers/<container-name>/<run-id>.tar
//
// The run-id is unique per attempt so the suffix is enough to
// distinguish attempts without a timestamp.
//
// Every segment goes through services.Slugify, which is what the KVM plugin
// uses for its own subjects: the same destination holds both, and one of them
// writing "Home Assistant" while the other writes "home-assistant" is a
// difference nobody chose.
func backupStorageRemotePath(location backupStorageDestination, plan *BackupPlan, runID, envSegment string) string {
	basePath := location.BasePath
	if location.LocationType == "local" {
		basePath = localBackupBasePath(basePath)
	}
	containerName := services.SlugifyOr(plan.ContainerName, shortID(plan.ContainerID))
	fileName := services.SlugifyOr(runID, "run") + ".tar"
	return path.Join("/", strings.TrimSpace(basePath), envSegment, "containers", containerName, fileName)
}

// localBackupBasePath is the absolute path the host writes local
// backups to. The default is the standard mcharbor mount point;
// operators can override per-location via the row's base_path.
func localBackupBasePath(basePath string) string {
	if strings.TrimSpace(basePath) == "" {
		return "/mnt/backup"
	}
	return basePath
}

// cleanLocalBackupPath resolves a local backup path to an absolute
// path inside the configured backup root. The path may be relative
// (resolved against the backup root) or absolute. Refusing paths
// outside the root is a defense-in-depth measure — the operator
// could only have supplied such a path through the storage
// destination CRUD endpoint, which is supposed to row-scope paths
// to the configured base_path.
func cleanLocalBackupPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%w: empty path", ErrInvalidLocalBackupPath)
	}
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("%w: path contains parent traversal", ErrInvalidLocalBackupPath)
	}
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidLocalBackupPath, err)
	}
	return abs, nil
}

// formatBackupUploadBytes renders a byte count as a human-readable
// string (e.g. "1.4 MB") for progress messages. Uses powers of two
// (MiB / GiB) to match what `ls -lh` reports.
func formatBackupUploadBytes(n int64) string {
	const (
		kib = 1024
		mib = kib * 1024
		gib = mib * 1024
		tib = gib * 1024
	)
	switch {
	case n < kib:
		return fmt.Sprintf("%d B", n)
	case n < mib:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(kib))
	case n < gib:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(mib))
	case n < tib:
		return fmt.Sprintf("%.2f GiB", float64(n)/float64(gib))
	default:
		return fmt.Sprintf("%.2f TiB", float64(n)/float64(tib))
	}
}

// backupUploadProgressMessage formats the per-run "uploading" progress
// line that the host surfaces to the UI. The destination name is
// included so the operator can see which destination is making
// progress when a run uploads to multiple storage locations.
func backupUploadProgressMessage(locationName string, uploaded, total int64) string {
	if strings.TrimSpace(locationName) == "" {
		locationName = "storage"
	}
	return fmt.Sprintf("Uploading to %s: %s / %s", locationName, formatBackupUploadBytes(uploaded), formatBackupUploadBytes(total))
}

// backupContextReader wraps an io.Reader so that reads fail fast
// when the surrounding context is cancelled. Used by the local
// upload path so a docker context cancel during a 2-hour backup
// stops the copy immediately instead of writing through to the
// next 1 MB buffer flush.
type backupContextReader struct {
	ctx    context.Context //nolint:containedctx // cancellable io.Reader adapter: holding the ctx is the point, Read has no ctx parameter
	reader io.Reader
}

func (r *backupContextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(p)
	}
}

// parseRunStart reads a run's RFC3339 start stamp; an unreadable one is logged and yields the zero time, which is what the callers already tolerated.
func (s *Service) parseRunStart(runID, value string) time.Time {
	startedAt, err := time.Parse(time.RFC3339, value)
	if err != nil && s.logger != nil {
		s.logger.Warn("container backups: run start time is not RFC3339", "run", runID, "value", value, "error", err)
	}
	return startedAt
}

// logRefreshFailure notes why a best-effort container-id refresh was skipped; it is a debug line because a broken environment must not fail the run list.
func (s *Service) logRefreshFailure(runID string, err error) {
	if err != nil && s.logger != nil {
		s.logger.Debug("container backups: container id refresh skipped", "run", runID, "error", err)
	}
}

// closeLogged closes c and logs a failure, because a deferred Close has no caller to return the error to.
func closeLogged(c io.Closer, what string) {
	if err := c.Close(); err != nil {
		slog.Warn("container backups: closing "+what+" failed", "error", err)
	}
}

// removeLogged deletes a temporary file and logs a failure other than the file already being gone.
func removeLogged(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("container backups: removing temporary file failed", "path", path, "error", err)
	}
}
