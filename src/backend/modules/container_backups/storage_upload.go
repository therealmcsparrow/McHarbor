// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"
	"golang.org/x/oauth2"
)

const (
	backupUploadTimeout          = 2 * time.Hour
	backupGraphChunkSize   int64 = 40 * 1024 * 1024
	defaultLocalBackupPath       = "/mnt/backup"
)

type backupStorageDestination struct {
	ID              string
	Name            string
	LocationType    string
	Enabled         bool
	BasePath        string
	Host            string
	Port            int
	Username        string
	Password        string
	ShareName       string
	DriveID         string
	TenantID        string
	ClientID        string
	ClientSecret    string
	RefreshToken    string
	Token           string
	AccessKeyID     string
	SecretAccessKey string
}

func (s *Service) uploadArchiveDestinations(ctx context.Context, plan *BackupPlan, runID string, archive archiveResult) error {
	ids := backupStorageLocationIDs(plan.StorageLocationID, plan.StorageLocationIDs)
	if len(ids) == 0 {
		return nil
	}
	if strings.TrimSpace(archive.path) == "" {
		return fmt.Errorf("backup archive path is empty")
	}

	locations, err := s.backupStorageDestinations(ctx, ids)
	if err != nil {
		return err
	}
	if len(locations) != len(ids) {
		return fmt.Errorf("one or more backup storage locations were not found")
	}

	// Each destination runs in its own goroutine so a slow OneDrive upload
	// doesn't gate the local copy and vice versa. The run row gets marked
	// `failure` if any destination fails, but the scratch copy stays on disk
	// so the user can still download.
	type destResult struct {
		name  string
		err   error
	}
	results := make(chan destResult, len(locations))
	var wg sync.WaitGroup

	for _, location := range locations {
		location := location
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- destResult{
				name: location.Name,
				err:  s.uploadToSingleDestination(ctx, runID, plan, location, archive),
			}
		}()
	}

	wg.Wait()
	close(results)

	var firstErr error
	for res := range results {
		if res.err != nil && firstErr == nil {
			firstErr = res.err
		}
	}
	return firstErr
}

// uploadToSingleDestination uploads the archive to one storage location and
// records per-destination status. Safe to call concurrently.
//
// Transient errors (network blip, 5xx response from Microsoft Graph) are
// retried with exponential backoff: 2s, 8s, 32s. Permanent errors (auth
// failure, missing destination, 4xx) fail immediately — retrying those
// would just waste time and bandwidth.
func (s *Service) uploadToSingleDestination(ctx context.Context, runID string, plan *BackupPlan, location backupStorageDestination, archive archiveResult) error {
	envSegment, err := s.resolveBackupEnvironmentSegment(ctx, plan.EnvironmentID)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(
				"backup environment segment lookup failed; falling back to environment id",
				"run", runID, "env", plan.EnvironmentID, "error", err,
			)
		}
	}
	remotePath := backupStorageRemotePath(location, plan, runID, envSegment)
	destinationID, err := s.createRunDestination(ctx, runID, location, remotePath)
	if err != nil {
		return err
	}
	if !location.Enabled {
		uploadErr := fmt.Errorf("backup storage location is disabled")
		if markErr := s.finishRunDestination(ctx, destinationID, "failure", "", uploadErr); markErr != nil && s.logger != nil {
			s.logger.Warn("container backup destination failure update failed", "run", runID, "destination", destinationID, "error", markErr)
		}
		return fmt.Errorf("uploading backup destination %s: %w", location.Name, uploadErr)
	}

	s.updateRunProgress(ctx, runID, "uploading", "Uploading backup to "+location.Name+".")

	const maxAttempts = 3
	backoff := 2 * time.Second
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			s.recordDestinationCancelled(runID, destinationID, ctx.Err())
			return ctx.Err()
		}
		uploadCtx, cancel := context.WithTimeout(ctx, backupUploadTimeout)
		uploadedPath, uploadErr := s.uploadBackupToStorageForDestination(uploadCtx, runID, destinationID, location, archive.path, remotePath, archive.size)
		cancel()
		if uploadErr == nil {
			if err := s.finishRunDestination(ctx, destinationID, "success", uploadedPath, nil); err != nil {
				return err
			}
			return nil
		}
		lastErr = uploadErr
		if !isRetryableUploadError(uploadErr) {
			break
		}
		if attempt < maxAttempts {
			if s.logger != nil {
				s.logger.Warn(
					"container backup upload failed, will retry",
					"run", runID, "storage", location.ID, "name", location.Name,
					"attempt", attempt, "max", maxAttempts, "backoff", backoff.String(),
					"error", uploadErr,
				)
			}
			select {
			case <-ctx.Done():
				s.recordDestinationCancelled(runID, destinationID, ctx.Err())
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 4
		}
	}

	if s.logger != nil {
		s.logger.Error("container backup storage upload failed", "run", runID, "storage", location.ID, "name", location.Name, "type", location.LocationType, "error", lastErr)
	}
	// If the parent run context was cancelled (e.g. the user pressed
	// Cancel in the UI), the in-flight HTTP request to OneDrive /
	// S3 / etc unwinds with "context canceled". That isn't a real
	// upload failure — the operator didn't ask for the upload to be
	// retried, they asked for the run to stop — so record a clear
	// "Cancelled by user" message instead of a generic failure.
	finalErr := lastErr
	if parentErr := ctx.Err(); parentErr != nil {
		finalErr = fmt.Errorf("cancelled by user: %w", parentErr)
	}
	// Use a fresh context for the destination-status write — when
	// `ctx` is cancelled, passing it to s.db.ExecContext causes
	// "context canceled" errors and leaves the destination stuck on
	// `uploading` even though the run row already says `cancelled`.
	if parentErr := ctx.Err(); parentErr != nil {
		s.recordDestinationCancelled(runID, destinationID, parentErr)
	} else {
		if markErr := s.finishRunDestination(ctx, destinationID, "failure", "", finalErr); markErr != nil && s.logger != nil {
			s.logger.Warn("container backup destination failure update failed", "run", runID, "destination", destinationID, "error", markErr)
		}
	}
	return fmt.Errorf("uploading backup destination %s: %w", location.Name, finalErr)
}

// recordDestinationCancelled marks a destination row as failed with a
// clear "Cancelled by user" message, using a fresh context so the DB
// write succeeds even when the run's own context is already cancelled.
// Without this, the destination row is left in `uploading` state and
// the UI shows a half-finished backup.
func (s *Service) recordDestinationCancelled(runID, destinationID string, cause error) {
	bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errMsg := fmt.Errorf("cancelled by user: %w", cause)
	if markErr := s.finishRunDestination(bgCtx, destinationID, "failure", "", errMsg); markErr != nil && s.logger != nil {
		s.logger.Warn("container backup destination failure update failed", "run", runID, "destination", destinationID, "error", markErr)
	}
}

// isRetryableUploadError returns true for transient failures that are
// worth retrying. Permanent failures (auth, validation, missing
// destination, 4xx client errors) are not retried.
func isRetryableUploadError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	networkMarkers := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"i/o timeout",
		"TLS handshake timeout",
		"unexpected EOF",
		"broken pipe",
		"connection closed",
	}
	for _, marker := range networkMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	// Microsoft Graph 5xx responses (server-side issues that may clear up)
	for _, code := range []string{"returned status 500", "returned status 502", "returned status 503", "returned status 504", "returned status 429"} {
		if strings.Contains(msg, code) {
			return true
		}
	}
	return false
}

type backupMigrationRun struct {
	ID            string
	PlanID        string
	EnvironmentID string
	ContainerID   string
	ContainerName string
	ArchivePath   string
	ArchiveSize   int64
	StartedAt     string
}

// MigrateCompletedRunsToLocalStorage copies existing completed backup archives to a local storage location.
func (s *Service) MigrateCompletedRunsToLocalStorage(ctx context.Context, storageLocationID string) (*BackupMigrationResult, error) {
	location, err := s.backupStorageDestination(ctx, storageLocationID)
	if err != nil {
		return nil, err
	}
	if location == nil {
		return nil, sql.ErrNoRows
	}
	if location.LocationType != "local" {
		return nil, ErrBackupMigrationStorageNotLocal
	}
	if !location.Enabled {
		return nil, ErrBackupMigrationStorageDisabled
	}

	result := &BackupMigrationResult{StorageLocationID: storageLocationID}
	skipped, err := s.countMigratedBackupRuns(ctx, storageLocationID)
	if err != nil {
		return nil, err
	}
	result.Skipped = skipped

	lastID := ""
	for {
		runs, err := s.backupMigrationRuns(ctx, storageLocationID, lastID)
		if err != nil {
			return nil, err
		}
		if len(runs) == 0 {
			break
		}

		for _, run := range runs {
			lastID = run.ID
			result.Total++
			migEnvSegment, migEnvErr := s.resolveBackupEnvironmentSegment(ctx, run.EnvironmentID)
			if migEnvErr != nil && s.logger != nil {
				s.logger.Warn(
					"backup migration environment segment lookup failed; falling back to environment id",
					"run", run.ID, "env", run.EnvironmentID, "error", migEnvErr,
				)
			}
			targetPath := backupMigrationRemotePath(*location, run, migEnvSegment)
			sourcePath, err := s.cleanBackupArchiveSource(run.ArchivePath)
			if err == nil {
				err = s.copyBackupFileAtomic(ctx, sourcePath, targetPath, run.ArchiveSize, nil)
			}
			if err != nil {
				result.Failed++
				if s.logger != nil {
					s.logger.Error("container backup local migration failed", "run", run.ID, "storage", storageLocationID, "error", err)
				}
				if markErr := s.upsertMigratedRunDestination(ctx, run.ID, *location, targetPath, "", err); markErr != nil && s.logger != nil {
					s.logger.Warn("container backup local migration failure update failed", "run", run.ID, "storage", storageLocationID, "error", markErr)
				}
				continue
			}
			if err := s.upsertMigratedRunDestination(ctx, run.ID, *location, targetPath, targetPath, nil); err != nil {
				result.Failed++
				if s.logger != nil {
					s.logger.Error("container backup local migration destination update failed", "run", run.ID, "storage", storageLocationID, "error", err)
				}
				continue
			}
			result.Migrated++
		}
	}
	result.Total += result.Skipped
	return result, nil
}

func (s *Service) countMigratedBackupRuns(ctx context.Context, storageLocationID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM container_backup_runs r
		WHERE r.status = 'success'
		  AND COALESCE(r.archive_path, '') <> ''
		  AND r.archive_size > 0
		  AND EXISTS (
			SELECT 1
			FROM container_backup_run_destinations d
			WHERE d.run_id = r.id
			  AND d.storage_location_id = ?
			  AND d.status = 'success'
			LIMIT 1
		  )`, storageLocationID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting migrated backup runs: %w", err)
	}
	return count, nil
}

func (s *Service) backupMigrationRuns(ctx context.Context, storageLocationID, afterID string) ([]backupMigrationRun, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, COALESCE(r.plan_id, ''), r.environment_id, r.container_id,
		       COALESCE(p.container_name, ''), COALESCE(r.archive_path, ''),
		       r.archive_size, r.started_at
		FROM container_backup_runs r
		LEFT JOIN container_backup_plans p ON p.id = r.plan_id
		WHERE r.status = 'success'
		  AND r.id > ?
		  AND COALESCE(r.archive_path, '') <> ''
		  AND r.archive_size > 0
		  AND NOT EXISTS (
			SELECT 1
			FROM container_backup_run_destinations d
			WHERE d.run_id = r.id
			  AND d.storage_location_id = ?
			  AND d.status = 'success'
			LIMIT 1
		  )
		ORDER BY r.id ASC
		LIMIT 100`, afterID, storageLocationID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing backup runs for local migration: %w", err)
	}
	defer rows.Close()

	runs := []backupMigrationRun{}
	for rows.Next() {
		var run backupMigrationRun
		if err := rows.Scan(
			&run.ID, &run.PlanID, &run.EnvironmentID, &run.ContainerID,
			&run.ContainerName, &run.ArchivePath, &run.ArchiveSize, &run.StartedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning backup run for local migration: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating backup runs for local migration: %w", err)
	}
	return runs, nil
}

func (s *Service) upsertMigratedRunDestination(ctx context.Context, runID string, location backupStorageDestination, remotePath, uploadedPath string, uploadErr error) error {
	now := time.Now().UTC().Format(time.RFC3339)
	status := "success"
	errorText := ""
	uploadedAt := sql.NullString{String: now, Valid: true}
	if uploadErr != nil {
		status = "failure"
		errorText = "Migration failed. Check McHarbor logs."
		uploadedAt = sql.NullString{}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting backup migration destination update: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) && s.logger != nil {
			s.logger.Warn("backup migration destination rollback failed", "run", runID, "storage", location.ID, "error", rollbackErr)
		}
	}()

	var existingID string
	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM container_backup_run_destinations
		WHERE run_id = ? AND storage_location_id = ?
		ORDER BY created_at ASC
		LIMIT 1`, runID, location.ID,
	).Scan(&existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("reading backup migration destination: %w", err)
	}

	storedPath := uploadedPath
	if storedPath == "" {
		storedPath = remotePath
	}
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO container_backup_run_destinations (
				id, run_id, storage_location_id, storage_location_name, location_type,
				status, remote_path, error, uploaded_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			xid.New().String(), runID, location.ID, location.Name, location.LocationType,
			status, storedPath, errorText, uploadedAt, now, now,
		)
		if err != nil {
			return fmt.Errorf("creating backup migration destination: %w", err)
		}
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE container_backup_run_destinations
			SET storage_location_name = ?, location_type = ?, status = ?, remote_path = ?,
			    error = ?, uploaded_at = ?, updated_at = ?
			WHERE id = ?`,
			location.Name, location.LocationType, status, storedPath,
			errorText, uploadedAt, now, existingID,
		)
		if err != nil {
			return fmt.Errorf("updating backup migration destination: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing backup migration destination: %w", err)
	}
	committed = true
	return nil
}

func (s *Service) cleanBackupArchiveSource(sourcePath string) (string, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return "", fmt.Errorf("backup archive path is empty")
	}
	clean := filepath.Clean(sourcePath)
	archivePath, err := filepath.Abs(clean)
	if err != nil {
		return "", fmt.Errorf("resolving backup archive path: %w", err)
	}
	backupRoot, err := filepath.Abs(s.backupDir())
	if err != nil {
		return "", fmt.Errorf("resolving backup root path: %w", err)
	}
	rel, err := filepath.Rel(backupRoot, archivePath)
	if err != nil {
		return "", fmt.Errorf("checking backup archive path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("backup archive path is outside the managed backup directory")
	}
	if filepath.Base(archivePath) != "mcharbor.tar" {
		return "", fmt.Errorf("backup archive file name is not valid")
	}
	info, err := os.Stat(archivePath)
	if err != nil {
		return "", fmt.Errorf("checking backup archive: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("backup archive path is a directory")
	}
	if info.Size() <= 0 {
		return "", fmt.Errorf("backup archive is empty")
	}
	return archivePath, nil
}

func backupMigrationRemotePath(location backupStorageDestination, run backupMigrationRun, envSegment string) string {
	basePath := location.BasePath
	if location.LocationType == "local" {
		basePath = localBackupBasePath(basePath)
	}
	containerName := safeArchiveName.ReplaceAllString(strings.TrimSpace(run.ContainerName), "-")
	if containerName == "" {
		containerName = shortID(run.ContainerID)
	}
	startedAt := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, run.StartedAt); err == nil {
		startedAt = parsed.UTC()
	}
	fileName := startedAt.Format("20060102T150405Z") + "-" + safeArchiveName.ReplaceAllString(run.ID, "-") + ".tar"
	return path.Join("/", strings.TrimSpace(basePath), envSegment, "containers", containerName, fileName)
}

func (s *Service) backupStorageDestinations(ctx context.Context, ids []string) ([]backupStorageDestination, error) {
	destinations := make([]backupStorageDestination, 0, len(ids))
	for _, id := range ids {
		destination, err := s.backupStorageDestination(ctx, id)
		if err != nil {
			return nil, err
		}
		if destination != nil {
			destinations = append(destinations, *destination)
		}
	}
	return destinations, nil
}

func (s *Service) backupStorageDestination(ctx context.Context, id string) (*backupStorageDestination, error) {
	if s.enc == nil {
		return nil, fmt.Errorf("storage credential encryption service is not configured")
	}
	var item backupStorageDestination
	var basePath, host, username, password, shareName, driveID, tenantID sql.NullString
	var clientID, clientSecret, refreshToken, token, accessKeyID, secretAccessKey sql.NullString
	var port sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, location_type, enabled,
		       COALESCE(base_path, ''), COALESCE(host, ''), COALESCE(port, 0),
		       COALESCE(username, ''), COALESCE(password, ''), COALESCE(share_name, ''),
		       COALESCE(drive_id, ''), COALESCE(tenant_id, ''),
		       client_id, client_secret, refresh_token, token,
		       access_key_id, secret_access_key
		FROM storage_locations
		WHERE id = ?`, id,
	).Scan(
		&item.ID, &item.Name, &item.LocationType, &item.Enabled,
		&basePath, &host, &port, &username, &password, &shareName,
		&driveID, &tenantID, &clientID, &clientSecret, &refreshToken, &token,
		&accessKeyID, &secretAccessKey,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading backup storage location: %w", err)
	}
	item.BasePath = basePath.String
	item.Host = host.String
	if port.Valid {
		item.Port = int(port.Int64)
	}
	item.Username = username.String
	item.ShareName = shareName.String
	item.DriveID = driveID.String
	item.TenantID = tenantID.String
	// Decrypt every secret column. We never store plaintext for
	// anything that could be a credential.
	if password.Valid && strings.TrimSpace(password.String) != "" {
		decrypted, err := s.enc.Decrypt(password.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage password: %w", err)
		}
		item.Password = decrypted
	}
	if clientID.Valid && strings.TrimSpace(clientID.String) != "" {
		decrypted, err := s.enc.Decrypt(clientID.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage client id: %w", err)
		}
		item.ClientID = decrypted
	}
	if clientSecret.Valid && strings.TrimSpace(clientSecret.String) != "" {
		decrypted, err := s.enc.Decrypt(clientSecret.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage client secret: %w", err)
		}
		item.ClientSecret = decrypted
	}
	if refreshToken.Valid && strings.TrimSpace(refreshToken.String) != "" {
		decrypted, err := s.enc.Decrypt(refreshToken.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage refresh token: %w", err)
		}
		item.RefreshToken = decrypted
	}
	if token.Valid && strings.TrimSpace(token.String) != "" {
		decrypted, err := s.enc.Decrypt(token.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage access token: %w", err)
		}
		item.Token = decrypted
	}
	if accessKeyID.Valid && strings.TrimSpace(accessKeyID.String) != "" {
		decrypted, err := s.enc.Decrypt(accessKeyID.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage access key: %w", err)
		}
		item.AccessKeyID = decrypted
	}
	if secretAccessKey.Valid && strings.TrimSpace(secretAccessKey.String) != "" {
		decrypted, err := s.enc.Decrypt(secretAccessKey.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage secret key: %w", err)
		}
		item.SecretAccessKey = decrypted
	}
	return &item, nil
}

func (s *Service) createRunDestination(ctx context.Context, runID string, location backupStorageDestination, remotePath string) (string, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO container_backup_run_destinations (
			id, run_id, storage_location_id, storage_location_name, location_type,
			status, remote_path, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 'uploading', ?, ?, ?)`,
		id, runID, location.ID, location.Name, location.LocationType, remotePath, now, now)
	if err != nil {
		return "", fmt.Errorf("creating backup run destination: %w", err)
	}
	return id, nil
}

func (s *Service) finishRunDestination(ctx context.Context, id, status, remotePath string, uploadErr error) error {
	now := time.Now().UTC().Format(time.RFC3339)
	errorText := ""
	uploadedAt := sql.NullString{}
	if uploadErr != nil {
		// Surface the real error to the operator instead of a generic
		// stub. Callers may pass either a transport-level error from
		// the uploader or a cancellation sentinel from the run's
		// parent context; the message we persist lets the UI tell
		// those apart ("Cancelled by user" vs. "uploading OneDrive
		// chunk returned status 503: ...").
		errorText = uploadErr.Error()
	} else if status == "success" {
		uploadedAt = sql.NullString{String: now, Valid: true}
	}
	if strings.TrimSpace(remotePath) == "" {
		_, err := s.db.ExecContext(ctx, `
			UPDATE container_backup_run_destinations
			SET status = ?, error = ?, uploaded_at = ?, updated_at = ?
			WHERE id = ?`,
			status, errorText, uploadedAt, now, id)
		if err != nil {
			return fmt.Errorf("finishing backup run destination: %w", err)
		}
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE container_backup_run_destinations
		SET status = ?, remote_path = ?, error = ?, uploaded_at = ?, updated_at = ?
		WHERE id = ?`,
		status, remotePath, errorText, uploadedAt, now, id)
	if err != nil {
		return fmt.Errorf("finishing backup run destination: %w", err)
	}
	return nil
}

// updateDestinationProgress writes the running byte counter for one
// destination row. Called by the upload progress callbacks; safe to
// invoke often — the query is keyed on the destination id and updates
// only two columns.
func (s *Service) updateDestinationProgress(ctx context.Context, destinationID string, uploaded, total int64) {
	if s.db == nil {
		return
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := s.db.ExecContext(queryCtx,
		"UPDATE container_backup_run_destinations SET bytes_uploaded = ?, bytes_total = ?, updated_at = ? WHERE id = ?",
		uploaded, total, time.Now().UTC().Format(time.RFC3339), destinationID,
	); err != nil && s.logger != nil {
		s.logger.Warn("container backup destination progress update failed", "destination", destinationID, "error", err)
	}
}

func (s *Service) hydrateRunDestinations(ctx context.Context, run *BackupRun) error {
	destinations := []BackupRunDestination{}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, COALESCE(storage_location_id, ''), storage_location_name, location_type,
		       status, remote_path, COALESCE(error, ''), COALESCE(uploaded_at, ''),
		       bytes_uploaded, bytes_total, created_at, updated_at
		FROM container_backup_run_destinations
		WHERE run_id = ?
		ORDER BY created_at ASC
		LIMIT 100`, run.ID)
	if err != nil {
		return fmt.Errorf("listing backup run destinations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var destination BackupRunDestination
		if err := rows.Scan(
			&destination.ID, &destination.RunID, &destination.StorageLocationID,
			&destination.StorageLocationName, &destination.LocationType,
			&destination.Status, &destination.Path, &destination.Error, &destination.UploadedAt,
			&destination.BytesUploaded, &destination.BytesTotal,
			&destination.CreatedAt, &destination.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scanning backup run destination: %w", err)
		}
		destinations = append(destinations, destination)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating backup run destinations: %w", err)
	}
	run.Destinations = destinations
	return nil
}

func (s *Service) uploadBackupToStorage(ctx context.Context, runID string, location backupStorageDestination, archivePath, remotePath string, size int64) (string, error) {
	return s.uploadBackupToStorageForDestination(ctx, runID, "", location, archivePath, remotePath, size)
}

func (s *Service) uploadBackupToStorageForDestination(ctx context.Context, runID, destinationID string, location backupStorageDestination, archivePath, remotePath string, size int64) (string, error) {
	switch location.LocationType {
	case "local":
		return s.uploadBackupToLocalWithDestination(ctx, runID, destinationID, location, archivePath, remotePath, size)
	case "samba":
		return s.uploadBackupToSambaWithDestination(ctx, runID, destinationID, location, archivePath, remotePath, size)
	case "onedrive_personal", "onedrive_business", "sharepoint":
		return s.uploadBackupToOneDriveWithDestination(ctx, runID, destinationID, location, archivePath, remotePath, size)
	default:
		return "", fmt.Errorf("storage location type %s is not supported for container backup upload", location.LocationType)
	}
}

func (s *Service) uploadBackupToLocal(ctx context.Context, runID string, location backupStorageDestination, archivePath, targetPath string, size int64) (string, error) {
	return s.uploadBackupToLocalWithDestination(ctx, runID, "", location, archivePath, targetPath, size)
}

// uploadBackupToLocalWithDestination is uploadBackupToLocal with an
// attached destination id so the per-destination byte counter can be
// updated in lockstep with the run-level progress message.
func (s *Service) uploadBackupToLocalWithDestination(ctx context.Context, runID, destinationID string, location backupStorageDestination, archivePath, targetPath string, size int64) (string, error) {
	targetPath, err := cleanLocalBackupPath(targetPath)
	if err != nil {
		return "", err
	}
	err = s.copyBackupFileAtomic(ctx, archivePath, targetPath, size, func(uploaded int64) {
		s.updateRunProgress(ctx, runID, "uploading", backupUploadProgressMessage(location.Name, uploaded, size))
		if destinationID != "" {
			s.updateDestinationProgress(ctx, destinationID, uploaded, size)
		}
	})
	if err != nil {
		return "", err
	}
	return targetPath, nil
}

func (s *Service) copyBackupFileAtomic(ctx context.Context, archivePath, targetPath string, size int64, onProgress func(uploaded int64)) error {
	targetPath, err := cleanLocalBackupPath(targetPath)
	if err != nil {
		return err
	}
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0750); err != nil {
		return fmt.Errorf("creating local backup destination directory: %w", err)
	}

	source, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening backup archive for local copy: %w", err)
	}
	defer source.Close()

	tmp, err := os.CreateTemp(targetDir, ".mcharbor-backup-*")
	if err != nil {
		return fmt.Errorf("creating local backup temp file: %w", err)
	}
	tmpName := tmp.Name()
	keepTemp := false
	defer func() {
		if !keepTemp {
			if removeErr := os.Remove(tmpName); removeErr != nil && !os.IsNotExist(removeErr) && s.logger != nil {
				s.logger.Warn("local backup temp cleanup failed", "path", tmpName, "error", removeErr)
			}
		}
	}()

	progressWriter := &backupUploadProgressWriter{
		writer:     tmp,
		total:      size,
		onProgress: onProgress,
	}
	if _, err := io.CopyBuffer(progressWriter, &backupContextReader{ctx: ctx, reader: source}, make([]byte, 1024*1024)); err != nil {
		if closeErr := tmp.Close(); closeErr != nil && s.logger != nil {
			s.logger.Warn("local backup temp close after copy failure failed", "path", tmpName, "error", closeErr)
		}
		return fmt.Errorf("copying backup to local storage: %w", err)
	}
	progressWriter.flush()
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing local backup temp file: %w", err)
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return fmt.Errorf("moving local backup into place: %w", err)
	}
	keepTemp = true
	return nil
}

func (s *Service) uploadBackupToOneDrive(ctx context.Context, runID string, location backupStorageDestination, archivePath, remotePath string, size int64) (string, error) {
	return s.uploadBackupToOneDriveWithDestination(ctx, runID, "", location, archivePath, remotePath, size)
}

func (s *Service) uploadBackupToOneDriveWithDestination(ctx context.Context, runID, destinationID string, location backupStorageDestination, archivePath, remotePath string, size int64) (string, error) {
	accessToken, err := s.microsoftAccessToken(ctx, location)
	if err != nil {
		return "", err
	}
	uploadURL, err := s.createMicrosoftUploadSession(ctx, location, remotePath, accessToken)
	if err != nil {
		return "", err
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening backup archive for upload: %w", err)
	}
	defer file.Close()

	for start := int64(0); start < size; start += backupGraphChunkSize {
		chunkSize := backupGraphChunkSize
		if remaining := size - start; remaining < chunkSize {
			chunkSize = remaining
		}
		if err := uploadMicrosoftChunk(ctx, uploadURL, file, start, chunkSize, size); err != nil {
			return "", err
		}
		uploaded := start + chunkSize
		s.updateRunProgress(ctx, runID, "uploading", backupUploadProgressMessage(location.Name, uploaded, size))
		if destinationID != "" {
			s.updateDestinationProgress(ctx, destinationID, uploaded, size)
		}
	}
	return remotePath, nil
}

func (s *Service) microsoftAccessToken(ctx context.Context, location backupStorageDestination) (string, error) {
	if strings.TrimSpace(location.ClientID) == "" || strings.TrimSpace(location.ClientSecret) == "" || strings.TrimSpace(location.RefreshToken) == "" {
		return "", fmt.Errorf("OneDrive storage location is missing OAuth credentials")
	}
	cfg := oauth2.Config{
		ClientID:     location.ClientID,
		ClientSecret: location.ClientSecret,
		Scopes:       microsoftStorageScopes(location.LocationType),
		Endpoint:     microsoftStorageEndpoint(location),
	}
	tokenSource := cfg.TokenSource(ctx, &oauth2.Token{
		AccessToken:  location.Token,
		RefreshToken: location.RefreshToken,
		Expiry:       time.Now().Add(-time.Hour),
	})
	token, err := tokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("refreshing OneDrive access token: %w", err)
	}
	if token.AccessToken == "" {
		return "", fmt.Errorf("OneDrive access token refresh returned no token")
	}
	if err := s.storeMicrosoftTokens(ctx, location.ID, token.AccessToken, token.RefreshToken); err != nil && s.logger != nil {
		s.logger.Warn("storing refreshed OneDrive token failed", "storage", location.ID, "error", err)
	}
	return token.AccessToken, nil
}

func (s *Service) storeMicrosoftTokens(ctx context.Context, locationID, accessToken, refreshToken string) error {
	if s.enc == nil {
		return fmt.Errorf("storage credential encryption service is not configured")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(accessToken) != "" {
		encrypted, err := s.enc.Encrypt(accessToken)
		if err != nil {
			return fmt.Errorf("encrypting OneDrive access token: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE storage_locations SET token = ?, updated_at = ? WHERE id = ?", encrypted, now, locationID); err != nil {
			return fmt.Errorf("updating OneDrive access token: %w", err)
		}
	}
	if strings.TrimSpace(refreshToken) != "" {
		encrypted, err := s.enc.Encrypt(refreshToken)
		if err != nil {
			return fmt.Errorf("encrypting OneDrive refresh token: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE storage_locations SET refresh_token = ?, updated_at = ? WHERE id = ?", encrypted, now, locationID); err != nil {
			return fmt.Errorf("updating OneDrive refresh token: %w", err)
		}
	}
	return nil
}

func (s *Service) createMicrosoftUploadSession(ctx context.Context, location backupStorageDestination, remotePath, accessToken string) (string, error) {
	endpoints := microsoftUploadSessionEndpoints(location, remotePath)
	var firstErr error
	for index, endpoint := range endpoints {
		uploadURL, err := createMicrosoftUploadSessionAtEndpoint(ctx, endpoint, remotePath, accessToken)
		if err == nil {
			if index > 0 && s.logger != nil {
				s.logger.Warn(
					"OneDrive configured drive upload failed; using default drive fallback",
					"storage", location.ID,
					"name", location.Name,
					"type", location.LocationType,
				)
			}
			return uploadURL, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if ctx.Err() != nil {
			return "", err
		}
	}
	if firstErr != nil {
		return "", firstErr
	}
	return "", fmt.Errorf("OneDrive upload session endpoint is not configured")
}

func createMicrosoftUploadSessionAtEndpoint(ctx context.Context, endpoint, remotePath, accessToken string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"item": map[string]any{
			"@microsoft.graph.conflictBehavior": "replace",
			"name":                              path.Base(remotePath),
		},
	})
	if err != nil {
		return "", fmt.Errorf("creating OneDrive upload session payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating OneDrive upload session request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("creating OneDrive upload session: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("creating OneDrive upload session returned status %d: %s", resp.StatusCode, limitedResponseBody(resp.Body))
	}
	var payload struct {
		UploadURL string `json:"uploadUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decoding OneDrive upload session: %w", err)
	}
	if strings.TrimSpace(payload.UploadURL) == "" {
		return "", fmt.Errorf("OneDrive upload session did not return an upload URL")
	}
	return payload.UploadURL, nil
}

func uploadMicrosoftChunk(ctx context.Context, uploadURL string, file *os.File, start, size, total int64) error {
	end := start + size - 1
	reader := io.NewSectionReader(file, start, size)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, reader)
	if err != nil {
		return fmt.Errorf("creating OneDrive upload chunk request: %w", err)
	}
	req.ContentLength = size
	req.Header.Set("Content-Length", strconv.FormatInt(size, 10))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("uploading OneDrive chunk: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	return fmt.Errorf("uploading OneDrive chunk returned status %d: %s", resp.StatusCode, limitedResponseBody(resp.Body))
}

func microsoftUploadSessionEndpoints(location backupStorageDestination, remotePath string) []string {
	encodedPath := microsoftGraphPath(remotePath)
	if location.LocationType == "onedrive_business" && strings.TrimSpace(location.DriveID) != "" {
		return []string{
			microsoftDriveUploadSessionEndpoint(location.DriveID, encodedPath),
			microsoftDefaultDriveUploadSessionEndpoint(encodedPath),
		}
	}
	if location.LocationType != "onedrive_personal" && strings.TrimSpace(location.DriveID) != "" {
		return []string{microsoftDriveUploadSessionEndpoint(location.DriveID, encodedPath)}
	}
	return []string{microsoftDefaultDriveUploadSessionEndpoint(encodedPath)}
}

func microsoftDriveUploadSessionEndpoint(driveID, encodedPath string) string {
	return "https://graph.microsoft.com/v1.0/drives/" + url.PathEscape(strings.TrimSpace(driveID)) + "/root:/" + encodedPath + ":/createUploadSession"
}

func microsoftDefaultDriveUploadSessionEndpoint(encodedPath string) string {
	return "https://graph.microsoft.com/v1.0/me/drive/root:/" + encodedPath + ":/createUploadSession"
}

func microsoftGraphPath(remotePath string) string {
	clean := strings.Trim(path.Clean("/"+strings.TrimSpace(remotePath)), "/")
	if clean == "" {
		return ""
	}
	parts := strings.Split(clean, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func microsoftStorageScopes(locationType string) []string {
	switch locationType {
	case "onedrive_personal":
		return []string{"offline_access", "Files.ReadWrite", "User.Read"}
	case "onedrive_business":
		return []string{"offline_access", "Files.ReadWrite.All", "User.Read"}
	case "sharepoint":
		return []string{"offline_access", "Files.ReadWrite.All", "Sites.ReadWrite.All", "User.Read"}
	default:
		return nil
	}
}

func microsoftStorageEndpoint(location backupStorageDestination) oauth2.Endpoint {
	switch location.LocationType {
	case "onedrive_personal":
		return oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/consumers/oauth2/v2.0/token",
		}
	case "onedrive_business", "sharepoint":
		tenant := strings.TrimSpace(location.TenantID)
		return oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/" + tenant + "/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/" + tenant + "/oauth2/v2.0/token",
		}
	default:
		return oauth2.Endpoint{}
	}
}

func backupStorageRemotePath(location backupStorageDestination, plan *BackupPlan, runID string, envSegment string) string {
	basePath := location.BasePath
	if location.LocationType == "local" {
		basePath = localBackupBasePath(basePath)
	}
	return backupRemotePath(basePath, plan, runID, envSegment)
}

func backupRemotePath(basePath string, plan *BackupPlan, runID string, envSegment string) string {
	containerName := safeArchiveName.ReplaceAllString(strings.TrimSpace(plan.ContainerName), "-")
	if containerName == "" {
		containerName = shortID(plan.ContainerID)
	}
	fileName := time.Now().UTC().Format("20060102T150405Z") + "-" + safeArchiveName.ReplaceAllString(runID, "-") + ".tar"
	remote := path.Join("/", strings.TrimSpace(basePath), envSegment, "containers", containerName, fileName)
	return remote
}

func localBackupBasePath(basePath string) string {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" {
		return defaultLocalBackupPath
	}
	clean := filepath.Clean(basePath)
	if !filepath.IsAbs(clean) {
		clean = string(filepath.Separator) + clean
	}
	return clean
}

func cleanLocalBackupPath(targetPath string) (string, error) {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return "", fmt.Errorf("local backup path is empty")
	}
	clean := filepath.Clean(targetPath)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("local backup path must be absolute")
	}
	if filepath.Base(clean) == "." || filepath.Base(clean) == string(filepath.Separator) {
		return "", fmt.Errorf("local backup target must be a file path")
	}
	return clean, nil
}

func limitedResponseBody(reader io.Reader) string {
	body, err := io.ReadAll(io.LimitReader(reader, 2048))
	if err != nil {
		return "unreadable response body"
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "empty response body"
	}
	return text
}

func formatBackupUploadBytes(value int64) string {
	if value <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	floatValue := float64(value)
	index := 0
	for floatValue >= 1024 && index < len(units)-1 {
		floatValue /= 1024
		index++
	}
	if index == 0 {
		return fmt.Sprintf("%d %s", value, units[index])
	}
	return fmt.Sprintf("%.1f %s", floatValue, units[index])
}

func backupUploadProgressMessage(locationName string, uploaded, total int64) string {
	return fmt.Sprintf(
		"Uploading backup to %s (%s of %s).",
		locationName,
		formatBackupUploadBytes(uploaded),
		formatBackupUploadBytes(total),
	)
}

type backupContextReader struct {
	ctx    context.Context
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

type backupUploadProgressWriter struct {
	writer     io.Writer
	total      int64
	uploaded   int64
	lastUpdate time.Time
	onProgress func(uploaded int64)
}

func (w *backupUploadProgressWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 {
		w.uploaded += int64(n)
		if time.Since(w.lastUpdate) >= 5*time.Second || w.uploaded >= w.total {
			w.flush()
		}
	}
	return n, err
}

func (w *backupUploadProgressWriter) flush() {
	if w.onProgress == nil || w.uploaded == 0 {
		return
	}
	w.lastUpdate = time.Now()
	w.onProgress(w.uploaded)
}
