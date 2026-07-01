// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"
)

// ErrRetryUploadNotEligible is returned when a destination cannot be
// retried: the destination is not in `failure` state, the parent run
// is still running, the local archive is gone, or the destination
// already has an in-flight retry.
var ErrRetryUploadNotEligible = errors.New("backup destination is not eligible for retry upload")

// retryCancelers tracks cancel functions for in-flight destination
// retry uploads. Lets the operator cancel a stuck retry without
// cancelling the parent run. Separate from runCancelers because
// retries outlive the HTTP request and should not be tied to the
// run's overall cancellation lifecycle.
var retryCancelers sync.Map

// RetryDestinationUpload re-uploads the on-disk archive of an
// existing backup run to one previously-failed storage destination.
// The local archive must still exist on disk (i.e. a local
// destination succeeded earlier) and the destination row must
// currently be in `failure` state.
//
// Returns the destination row immediately (with status flipped to
// `uploading`) so the caller can poll it via the regular run
// query. The actual upload runs in a background goroutine with a
// fresh context so the HTTP response returns immediately and the
// operator UI can poll for progress via the existing per-destination
// bytes_uploaded / bytes_total columns.
func (s *Service) RetryDestinationUpload(ctx context.Context, runID, destinationID string) (*BackupRunDestination, error) {
	run, err := s.runByID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("loading backup run: %w", err)
	}
	if run == nil {
		return nil, fmt.Errorf("backup run not found")
	}
	if run.Status == "running" {
		return nil, ErrRetryUploadNotEligible
	}
	if run.Operation != "backup" {
		return nil, fmt.Errorf("only backup runs support destination retry")
	}
	if strings.TrimSpace(run.ArchivePath) == "" {
		return nil, fmt.Errorf("backup archive is no longer available on disk")
	}
	if run.ArchiveSize <= 0 {
		return nil, fmt.Errorf("backup archive size is unknown")
	}

	var destination BackupRunDestination
	var storageID sql.NullString
	err = s.db.QueryRowContext(ctx, `
		SELECT id, run_id, COALESCE(storage_location_id, ''), storage_location_name, location_type,
		       status, remote_path, COALESCE(error, ''), COALESCE(uploaded_at, ''),
		       bytes_uploaded, bytes_total, created_at, updated_at
		FROM container_backup_run_destinations
		WHERE id = ? AND run_id = ?
		LIMIT 1`, destinationID, runID).Scan(
		&destination.ID, &destination.RunID, &storageID, &destination.StorageLocationName, &destination.LocationType,
		&destination.Status, &destination.Path, &destination.Error, &destination.UploadedAt,
		&destination.BytesUploaded, &destination.BytesTotal,
		&destination.CreatedAt, &destination.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("backup destination not found")
	}
	if err != nil {
		return nil, fmt.Errorf("loading backup destination: %w", err)
	}
	if storageID.Valid {
		destination.StorageLocationID = storageID.String
	}
	if destination.Status == "uploading" {
		return nil, ErrRetryUploadNotEligible
	}
	if destination.Status == "success" {
		return nil, ErrRetryUploadNotEligible
	}
	if _, busy := retryCancelers.Load(destinationID); busy {
		return nil, ErrRetryUploadNotEligible
	}
	if strings.TrimSpace(destination.StorageLocationID) == "" {
		return nil, fmt.Errorf("backup destination is missing storage location reference")
	}
	if strings.TrimSpace(destination.Path) == "" {
		return nil, fmt.Errorf("backup destination is missing a remote path")
	}

	archivePath, err := s.cleanBackupArchiveSource(run.ArchivePath)
	if err != nil {
		return nil, fmt.Errorf("local backup archive is no longer available: %w", err)
	}

	location, err := s.backupStorageDestination(ctx, destination.StorageLocationID)
	if err != nil {
		return nil, fmt.Errorf("loading storage location: %w", err)
	}
	if location == nil {
		return nil, fmt.Errorf("storage location not found")
	}
	if !location.Enabled {
		return nil, fmt.Errorf("storage location is disabled")
	}

	// Reset the destination row so the existing polling queries pick
	// it up as `uploading` again. Reuse the destination's existing
	// remote path so the retry overwrites the same logical file at
	// the destination (avoiding filename collisions on providers
	// like OneDrive / SharePoint where duplicate names would create
	// a new file).
	now := time.Now().UTC().Format(time.RFC3339)
	remotePath := strings.TrimSpace(destination.Path)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE container_backup_run_destinations
		SET status = 'uploading', error = '', uploaded_at = NULL,
		    bytes_uploaded = 0, bytes_total = ?, updated_at = ?
		WHERE id = ?`,
		run.ArchiveSize, now, destinationID); err != nil {
		return nil, fmt.Errorf("resetting destination for retry: %w", err)
	}

	destination.Status = "uploading"
	destination.Error = ""
	destination.BytesUploaded = 0
	destination.BytesTotal = run.ArchiveSize
	destination.UpdatedAt = now

	// Run the upload in a background goroutine with a fresh context
	// so the HTTP handler returns immediately and the operator UI
	// can poll the regular run query to watch progress.
	retryCtx, cancel := context.WithTimeout(context.Background(), backupUploadTimeout)
	retryCancelers.Store(destinationID, cancel)
	retryID := xid.New().String()
	if s.logger != nil {
		s.logger.Info(
			"container backup destination retry upload started",
			"retryId", retryID,
			"run", runID,
			"destination", destinationID,
			"storage", location.ID,
			"name", location.Name,
			"type", location.LocationType,
			"archiveSize", run.ArchiveSize,
		)
	}
	go s.runDestinationRetryUpload(retryID, retryCtx, runID, destinationID, archivePath, remotePath, run.ArchiveSize, *location, s.logger)

	return &destination, nil
}

func (s *Service) runDestinationRetryUpload(
	retryID string,
	ctx context.Context,
	runID, destinationID, archivePath, remotePath string,
	size int64,
	location backupStorageDestination,
	logger *slog.Logger,
) {
	defer func() {
		if value, ok := retryCancelers.LoadAndDelete(destinationID); ok {
			if cancel, isCancel := value.(context.CancelFunc); isCancel {
				cancel()
			}
		}
	}()

	s.updateRunProgress(context.Background(), runID, "uploading", backupUploadProgressMessage(location.Name, 0, size))

	uploadedPath, err := s.uploadBackupToStorageForDestination(ctx, runID, destinationID, location, archivePath, remotePath, size)
	if err != nil {
		// Cancellation: leave the run row alone but mark the
		// destination with a clear "cancelled by user" message
		// using a fresh context (the upload ctx is already
		// cancelled).
		if parentErr := ctx.Err(); parentErr != nil {
			s.recordDestinationCancelled(runID, destinationID, parentErr)
		} else {
			if markErr := s.finishRunDestination(context.Background(), destinationID, "failure", "", err); markErr != nil && logger != nil {
				logger.Warn("container backup destination failure update failed", "run", runID, "destination", destinationID, "error", markErr)
			}
		}
		if logger != nil {
			logger.Error(
				"container backup destination retry upload failed",
				"retryId", retryID, "run", runID, "destination", destinationID,
				"storage", location.ID, "name", location.Name, "type", location.LocationType,
				"error", err,
			)
		}
		return
	}

	if err := s.finishRunDestination(context.Background(), destinationID, "success", uploadedPath, nil); err != nil {
		if logger != nil {
			logger.Warn("container backup destination success update failed", "run", runID, "destination", destinationID, "error", err)
		}
		return
	}
	if logger != nil {
		logger.Info(
			"container backup destination retry upload succeeded",
			"retryId", retryID, "run", runID, "destination", destinationID,
			"storage", location.ID, "name", location.Name,
		)
	}
}

// CancelRetryUpload cancels an in-flight retry upload by
// destination id. Safe to call even when no retry is running.
func (s *Service) CancelRetryUpload(destinationID string) {
	if value, ok := retryCancelers.LoadAndDelete(destinationID); ok {
		if cancel, isCancel := value.(context.CancelFunc); isCancel {
			cancel()
		}
	}
}

// HasRetryUploadRunning returns true when a retry upload is in
// flight for the destination.
func (s *Service) HasRetryUploadRunning(destinationID string) bool {
	_, ok := retryCancelers.Load(destinationID)
	return ok
}