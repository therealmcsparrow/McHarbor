// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"
)

// ErrBackupDestinationDeleteFailed is returned by destination delete
// helpers when the remote provider rejects the delete. The caller
// surfaces this in the response and the audit log so operators can
// clean up stragglers by hand.
var ErrBackupDestinationDeleteFailed = errors.New("backup destination delete failed")

// deleteDestinationFile removes the backup archive at one
// destination. `local` is removed directly; every cloud / shared
// destination goes through the plugin adapter that uploaded it.
//
// Errors are logged but never returned as fatal — DeleteRun continues
// even when one destination can't be cleaned up, because the
// operator still wants the DB row gone and the local archive gone.
func (s *Service) deleteDestinationFile(
	ctx context.Context,
	destination BackupRunDestination,
	location *backupStorageDestination,
	logger *slog.Logger,
) error {
	if strings.TrimSpace(destination.Path) == "" {
		return nil
	}
	if destination.LocationType == "local" {
		return deleteLocalDestinationFile(destination.Path, logger)
	}
	// The upload stored the adapter's own key as the path, so the same adapter
	// can remove it; skipping here is what left every cloud copy behind.
	if location == nil {
		return fmt.Errorf("%w: storage location %s could not be loaded",
			ErrBackupDestinationDeleteFailed, destination.StorageLocationID)
	}
	adapter, _, ok := s.resolveStorageAdapter(destination.LocationType)
	if !ok {
		return fmt.Errorf("%w: no installed plugin handles %s",
			ErrBackupDestinationDeleteFailed, destination.LocationType)
	}
	if err := adapter.Delete(ctx, s.buildLocationConfigForUpload(location), destination.Path); err != nil {
		return fmt.Errorf("%w: %w", ErrBackupDestinationDeleteFailed, err)
	}
	if logger != nil {
		logger.Info("container backup destination file removed",
			"type", destination.LocationType, "path", destination.Path)
	}
	return nil
}

// deleteLocalDestinationFile removes the archive file from a local
// storage destination. The path stored on the destination row is the
// absolute file path written by uploadBackupToLocal. We treat missing
// files as success — the operator's intent (no leftover file) is met.
func deleteLocalDestinationFile(remotePath string, logger *slog.Logger) error {
	cleaned, err := cleanLocalBackupPath(remotePath)
	if err != nil {
		return fmt.Errorf("invalid local destination path: %w", err)
	}
	if err := os.Remove(cleaned); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("removing local destination file %s: %w", cleaned, err)
	}
	if logger != nil {
		logger.Info("container backup destination local file removed", "path", cleaned)
	}
	return nil
}
