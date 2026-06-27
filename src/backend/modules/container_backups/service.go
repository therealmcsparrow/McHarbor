// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/rs/xid"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/db"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// ErrBackupEncryptionKeyNotConfigured means encrypted backups cannot run until the Docker secret is mounted.
var ErrBackupEncryptionKeyNotConfigured = errors.New("backup encryption key is not configured")

// ErrBackupRunNotDownloadable means a backup run has no completed archive to download.
var ErrBackupRunNotDownloadable = errors.New("backup run is not downloadable")

// ErrBackupRestoreSecretRequired means the archive was encrypted with a different key.
var ErrBackupRestoreSecretRequired = errors.New("backup restore secret key is required")

// ErrBackupRestoreKeyInvalid means the supplied backup secret cannot decrypt the archive.
var ErrBackupRestoreKeyInvalid = errors.New("backup restore secret key is invalid")

// ErrBackupRestoreNoRestorableEntries means the archive has no image, filesystem, or mount entries.
var ErrBackupRestoreNoRestorableEntries = errors.New("backup archive has no restorable entries")

// ErrBackupRunActive means a backup run cannot be deleted while it is still active.
var ErrBackupRunActive = errors.New("backup run is still running")

// ErrBackupRunNotCancellable means the run is not currently running and cannot
// be cancelled.
var ErrBackupRunNotCancellable = errors.New("backup run is not cancellable")

// ErrBackupMigrationStorageNotLocal means the destination cannot receive local backup migrations.
var ErrBackupMigrationStorageNotLocal = errors.New("backup migration storage location is not local")

// ErrBackupMigrationStorageDisabled means the destination cannot be written while disabled.
var ErrBackupMigrationStorageDisabled = errors.New("backup migration storage location is disabled")

const (
	backupRunFinalizeTimeout     = 30 * time.Second
	backupRunProgressTimeout     = 5 * time.Second
	backupRunProgressStaleAfter  = 2 * time.Minute
	backupRunBackgroundTimeout   = 3 * time.Hour
	restoreRunProgressStaleAfter = backupRunBackgroundTimeout + 10*time.Minute
	defaultLocalStorageID        = "default-local-backup"
	agentBackupMinVersion        = "1.5.2"
)

// Service handles container backup plans and archive execution.
type Service struct {
	db           *sql.DB
	pool         *coredocker.ClientPool
	dataDir      string
	backupCrypto *backupcrypto.Service
	enc          *encryption.Service
	logger       *slog.Logger
}

var safeArchiveName = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

var activeBackupRuns sync.Map

// runCancelers tracks cancel functions for in-flight backup and restore runs so
// the user can cancel a long-running operation from the UI. Keys are run IDs.
var runCancelers sync.Map

// NewService creates a backup service.
func NewService(database *sql.DB, pool *coredocker.ClientPool, dataDir string, backupCrypto *backupcrypto.Service, enc *encryption.Service, logger *slog.Logger) *Service {
	return &Service{db: database, pool: pool, dataDir: dataDir, backupCrypto: backupCrypto, enc: enc, logger: logger}
}

func (s *Service) client(envID string) (*client.Client, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}
	return cli, nil
}

// Options returns the sensible backup choices for the inspected container.
func (s *Service) Options(ctx context.Context, envID, containerID string) (*BackupOptions, error) {
	cli, err := s.client(envID)
	if err != nil {
		return nil, err
	}
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	info, err := cli.ContainerInspect(opCtx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspecting container for backup options: %w", err)
	}

	name := strings.TrimPrefix(info.Name, "/")
	if name == "" {
		name = shortID(info.ID)
	}

	options := []BackupOption{
		{Key: "config", Type: "config", Label: "Container configuration", Description: "Inspect data, environment, labels, networks, ports, restart policy, and host config.", Default: true, Required: true},
	}
	if info.Config != nil && info.Config.Image != "" {
		options = append(options, BackupOption{Key: "image", Type: "image", Label: "Image archive", Description: "The Docker image used by this container.", Default: false})
	}
	options = append(options, BackupOption{Key: "filesystem", Type: "filesystem", Label: "Container filesystem", Description: "The writable container filesystem layer. Mounted volumes are backed up separately.", Default: false})
	options = append(options, BackupOption{Key: "logs", Type: "logs", Label: "Container logs", Description: "Available stdout and stderr logs from Docker.", Default: false})

	for _, m := range info.Mounts {
		if m.Destination == "" {
			continue
		}
		if m.Type != "volume" && m.Type != "bind" {
			continue
		}
		label := m.Destination
		description := "Mounted data"
		if m.Type == "volume" && m.Name != "" {
			label = m.Name + " -> " + m.Destination
			description = "Named Docker volume mounted in the container."
		}
		if m.Type == "bind" {
			description = "Bind-mounted host path as seen through the container mount."
		}
		options = append(options, BackupOption{
			Key:         "mount:" + m.Destination,
			Type:        string(m.Type),
			Label:       label,
			Description: description,
			Default:     m.Type == "volume",
		})
	}

	return &BackupOptions{ContainerID: info.ID, ContainerName: name, Options: options}, nil
}

// ListPlans returns backup plans for an environment and optional container.
func (s *Service) ListPlans(ctx context.Context, envID, containerID string) ([]BackupPlan, error) {
	query := `
		SELECT id, name, environment_id, container_id, container_name, COALESCE(storage_location_id, ''),
		       include_config, include_logs, include_filesystem, include_image, selected_mounts,
		       log_tail_lines,
		       COALESCE(cron, ''), enabled, retention_count, retention_days, COALESCE(last_run_at, ''), COALESCE(next_run_at, ''),
		       created_at, updated_at
		FROM container_backup_plans
		WHERE environment_id = ?`
	args := []any{envID}
	if containerID != "" {
		query += " AND container_id = ?"
		args = append(args, containerID)
	}
	query += " ORDER BY name ASC LIMIT 1000"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing backup plans: %w", err)
	}
	defer rows.Close()

	plans := []BackupPlan{}
	for rows.Next() {
		plan, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		if err := s.hydratePlanStorageLocations(ctx, &plan); err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}

// CreatePlan creates a backup plan.
func (s *Service) CreatePlan(ctx context.Context, envID string, input CreateBackupPlanInput) (*BackupPlan, error) {
	if input.ContainerID == "" {
		return nil, fmt.Errorf("container id is required")
	}
	if err := validatePlanRetention(input.RetentionCount, input.RetentionDays); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Container backup"
	}
	containerName, err := s.containerName(ctx, envID, input.ContainerID)
	if err != nil {
		return nil, err
	}
	selected, err := json.Marshal(input.SelectedMounts)
	if err != nil {
		return nil, fmt.Errorf("encoding selected mounts: %w", err)
	}
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	nextRun := ""
	if input.Enabled && strings.TrimSpace(input.Cron) != "" {
		nextRun = nextCronRun(input.Cron, time.Now().UTC()).Format(time.RFC3339)
	}
	storageIDs, err := s.requiredBackupStorageLocationIDs(ctx, input.StorageLocationID, input.StorageLocationIDs)
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO container_backup_plans (
			id, name, environment_id, container_id, container_name, storage_location_id,
			include_config, include_logs, include_filesystem, include_image, selected_mounts,
			log_tail_lines,
			cron, enabled, retention_count, retention_days, next_run_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, envID, input.ContainerID, containerName, nullString(firstStorageLocationID(storageIDs)),
		input.IncludeConfig, input.IncludeLogs, input.IncludeFilesystem, input.IncludeImage, string(selected),
		input.LogTailLines,
		nullString(input.Cron), input.Enabled, input.RetentionCount, input.RetentionDays, nullString(nextRun), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("creating backup plan: %w", err)
	}
	if err := s.setPlanStorageLocations(ctx, id, storageIDs); err != nil {
		return nil, err
	}
	return s.Plan(ctx, id)
}

// Plan returns one backup plan.
func (s *Service) Plan(ctx context.Context, id string) (*BackupPlan, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, environment_id, container_id, container_name, COALESCE(storage_location_id, ''),
		       include_config, include_logs, include_filesystem, include_image, selected_mounts,
		       log_tail_lines,
		       COALESCE(cron, ''), enabled, retention_count, retention_days, COALESCE(last_run_at, ''), COALESCE(next_run_at, ''),
		       created_at, updated_at
		FROM container_backup_plans
		WHERE id = ?`, id)
	plan, err := scanPlan(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.hydratePlanStorageLocations(ctx, &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

// UpdatePlan updates a backup plan.
func (s *Service) UpdatePlan(ctx context.Context, id string, input UpdateBackupPlanInput) (*BackupPlan, error) {
	plan, err := s.Plan(ctx, id)
	if err != nil || plan == nil {
		return plan, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if input.Name != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET name = ?, updated_at = ? WHERE id = ?", strings.TrimSpace(*input.Name), now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan name: %w", err)
		}
	}
	if input.StorageLocationID != nil && input.StorageLocationIDs == nil {
		ids, err := s.requiredBackupStorageLocationIDs(ctx, *input.StorageLocationID, nil)
		if err != nil {
			return nil, err
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET storage_location_id = ?, updated_at = ? WHERE id = ?", nullString(firstStorageLocationID(ids)), now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan storage location: %w", err)
		}
		if err := s.setPlanStorageLocations(ctx, id, ids); err != nil {
			return nil, err
		}
	}
	if input.StorageLocationIDs != nil {
		ids, err := s.requiredBackupStorageLocationIDs(ctx, "", *input.StorageLocationIDs)
		if err != nil {
			return nil, err
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET storage_location_id = ?, updated_at = ? WHERE id = ?", nullString(firstStorageLocationID(ids)), now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan primary storage location: %w", err)
		}
		if err := s.setPlanStorageLocations(ctx, id, ids); err != nil {
			return nil, err
		}
	}
	if input.IncludeConfig != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET include_config = ?, updated_at = ? WHERE id = ?", *input.IncludeConfig, now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan config flag: %w", err)
		}
	}
	if input.IncludeLogs != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET include_logs = ?, updated_at = ? WHERE id = ?", *input.IncludeLogs, now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan logs flag: %w", err)
		}
	}
	if input.IncludeFilesystem != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET include_filesystem = ?, updated_at = ? WHERE id = ?", *input.IncludeFilesystem, now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan filesystem flag: %w", err)
		}
	}
	if input.IncludeImage != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET include_image = ?, updated_at = ? WHERE id = ?", *input.IncludeImage, now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan image flag: %w", err)
		}
	}
	if input.LogTailLines != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET log_tail_lines = ?, updated_at = ? WHERE id = ?", *input.LogTailLines, now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan log tail lines: %w", err)
		}
	}
	if input.SelectedMounts != nil {
		selected, err := json.Marshal(*input.SelectedMounts)
		if err != nil {
			return nil, fmt.Errorf("encoding selected mounts: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET selected_mounts = ?, updated_at = ? WHERE id = ?", string(selected), now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan mounts: %w", err)
		}
	}
	if input.RetentionCount != nil || input.RetentionDays != nil {
		retentionCount := plan.RetentionCount
		retentionDays := plan.RetentionDays
		if input.RetentionCount != nil {
			retentionCount = *input.RetentionCount
		}
		if input.RetentionDays != nil {
			retentionDays = *input.RetentionDays
		}
		if err := validatePlanRetention(retentionCount, retentionDays); err != nil {
			return nil, err
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET retention_count = ?, retention_days = ?, updated_at = ? WHERE id = ?", retentionCount, retentionDays, now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan retention: %w", err)
		}
	}
	if input.Cron != nil || input.Enabled != nil {
		cron := plan.Cron
		enabled := plan.Enabled
		if input.Cron != nil {
			cron = strings.TrimSpace(*input.Cron)
		}
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		nextRun := ""
		if enabled && cron != "" {
			nextRun = nextCronRun(cron, time.Now().UTC()).Format(time.RFC3339)
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET cron = ?, enabled = ?, next_run_at = ?, updated_at = ? WHERE id = ?", nullString(cron), enabled, nullString(nextRun), now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan schedule: %w", err)
		}
	}
	return s.Plan(ctx, id)
}

// DeletePlan deletes a backup plan.
func (s *Service) DeletePlan(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM container_backup_plans WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting backup plan: %w", err)
	}
	return db.RowsAffected(result) > 0, nil
}

// ListRuns returns recent backup runs.
func (s *Service) ListRuns(ctx context.Context, envID, containerID string) ([]BackupRun, error) {
	if err := s.RecoverAbandonedRuns(ctx, envID, containerID); err != nil && s.logger != nil {
		s.logger.Warn("container backup abandoned run recovery failed", "env", envID, "container", containerID, "error", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(plan_id, ''), COALESCE(operation, 'backup'), COALESCE(source_run_id, ''), environment_id, container_id, status, COALESCE(archive_path, ''),
		       archive_size, COALESCE(archive_encryption, ''), COALESCE(archive_key_id, ''), COALESCE(error, ''),
		       COALESCE(progress_stage, ''), COALESCE(progress_message, ''), COALESCE(progress_updated_at, ''),
		       started_at, COALESCE(completed_at, ''), duration_ms, created_at, updated_at
		FROM container_backup_runs
		WHERE environment_id = ? AND container_id = ?
		ORDER BY started_at DESC
		LIMIT 100`, envID, containerID)
	if err != nil {
		return nil, fmt.Errorf("listing backup runs: %w", err)
	}

	runs := []BackupRun{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		s.annotateRunKeyRequirement(&run)
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing backup run rows: %w", err)
	}

	for i := range runs {
		if err := s.hydrateRunDestinations(ctx, &runs[i]); err != nil {
			return nil, err
		}
	}
	return runs, nil
}

// RecoverAbandonedRuns finalizes old running rows left behind by request cancellation or shutdown.
func (s *Service) RecoverAbandonedRuns(ctx context.Context, envID, containerID string) error {
	cutoff := time.Now().UTC().Add(-backupRunProgressStaleAfter).Format(time.RFC3339)
	query := `
		SELECT id, started_at, COALESCE(environment_id, ''), COALESCE(container_id, '')
		FROM container_backup_runs
		WHERE status = 'running'
		  AND COALESCE(operation, 'backup') = 'backup'
		  AND COALESCE(NULLIF(progress_updated_at, ''), started_at) < ?`
	args := []any{cutoff}
	if envID != "" {
		query += " AND environment_id = ?"
		args = append(args, envID)
	}
	if containerID != "" {
		query += " AND container_id = ?"
		args = append(args, containerID)
	}
	query += " ORDER BY started_at ASC LIMIT 1000"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("listing abandoned backup runs: %w", err)
	}

	type abandonedRun struct {
		id           string
		started      string
		environmentID string
		containerID  string
	}
	runs := []abandonedRun{}
	for rows.Next() {
		var run abandonedRun
		if err := rows.Scan(&run.id, &run.started, &run.environmentID, &run.containerID); err != nil {
			rows.Close()
			return fmt.Errorf("scanning abandoned backup run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("closing abandoned backup run rows: %w", err)
	}

	for _, run := range runs {
		if backupRunIsActive(run.id) {
			continue
		}
		// Best-effort unpause before finalising so a backup that crashed
		// during the archive-write phase does not leave the container in a
		// paused state on the Docker daemon. We use a fresh background
		// context so a near-deadline parent does not cancel the release.
		if run.environmentID != "" && run.containerID != "" {
			s.releaseStuckContainerPause(run.environmentID, run.containerID)
		}
		if err := s.recoverAbandonedRun(ctx, run.id); err != nil && s.logger != nil {
			s.logger.Warn("container backup abandoned run recovery failed", "run", run.id, "started", run.started, "error", err)
		}
	}
	if err := s.recoverAbandonedRestoreRuns(ctx, envID, containerID); err != nil {
		return err
	}
	if err := s.recoverOrphanUploadingDestinations(ctx); err != nil {
		return err
	}
	return nil
}

// releaseStuckContainerPause asks the Docker daemon (or remote agent) to
// unpause a container that was paused for snapshot consistency before the
// backup goroutine crashed. Errors are swallowed: a 404 / 304 / "not paused"
// response means the container is already in a healthy state and the
// orphaned pause is gone.
func (s *Service) releaseStuckContainerPause(envID, containerID string) {
	if s.pool == nil {
		return
	}
	cli, err := s.pool.Get(envID)
	if err != nil {
		if s.logger != nil {
			s.logger.Debug("abandoned backup: docker client unavailable for unpause", "env", envID, "container", containerID, "error", err)
		}
		return
	}
	unpauseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := cli.ContainerUnpause(unpauseCtx, containerID); err != nil {
		if s.logger != nil {
			s.logger.Info("abandoned backup: release paused container", "container", containerID, "result", err.Error())
		}
		return
	}
	if s.logger != nil {
		s.logger.Info("abandoned backup: released stuck container pause", "env", envID, "container", containerID)
	}
}

// RunPlan executes a saved backup plan synchronously.
func (s *Service) RunPlan(ctx context.Context, planID string) (*BackupRun, error) {
	if s.backupCrypto == nil {
		return nil, ErrBackupEncryptionKeyNotConfigured
	}
	plan, err := s.Plan(ctx, planID)
	if err != nil || plan == nil {
		return nil, err
	}
	run, err := s.createRun(ctx, plan.ID, plan.EnvironmentID, plan.ContainerID)
	if err != nil {
		return nil, err
	}
	return s.executeRun(ctx, plan, run.ID)
}

// StartPlan starts a saved backup plan in the background and returns the queued run.
func (s *Service) StartPlan(ctx context.Context, planID string) (*BackupRun, error) {
	if s.backupCrypto == nil {
		return nil, ErrBackupEncryptionKeyNotConfigured
	}
	plan, err := s.Plan(ctx, planID)
	if err != nil || plan == nil {
		return nil, err
	}
	run, err := s.createRun(ctx, plan.ID, plan.EnvironmentID, plan.ContainerID)
	if err != nil {
		return nil, err
	}
	s.startBackgroundRun(plan, run.ID)
	return run, nil
}

// RunAdhoc executes an unsaved backup request.
func (s *Service) RunAdhoc(ctx context.Context, envID, containerID string, input RunBackupInput) (*BackupRun, error) {
	if s.backupCrypto == nil {
		return nil, ErrBackupEncryptionKeyNotConfigured
	}
	plan, err := s.adhocPlan(ctx, envID, containerID, input)
	if err != nil {
		return nil, err
	}
	run, err := s.createRun(ctx, "", envID, containerID)
	if err != nil {
		return nil, err
	}
	return s.executeRun(ctx, plan, run.ID)
}

// StartAdhoc starts an unsaved backup request in the background and returns the queued run.
func (s *Service) StartAdhoc(ctx context.Context, envID, containerID string, input RunBackupInput) (*BackupRun, error) {
	if s.backupCrypto == nil {
		return nil, ErrBackupEncryptionKeyNotConfigured
	}
	plan, err := s.adhocPlan(ctx, envID, containerID, input)
	if err != nil {
		return nil, err
	}
	run, err := s.createRun(ctx, "", envID, containerID)
	if err != nil {
		return nil, err
	}
	s.startBackgroundRun(plan, run.ID)
	return run, nil
}

func (s *Service) adhocPlan(ctx context.Context, envID, containerID string, input RunBackupInput) (*BackupPlan, error) {
	name := input.Name
	if strings.TrimSpace(name) == "" {
		name = "Manual backup"
	}
	containerName, err := s.containerName(ctx, envID, containerID)
	if err != nil {
		return nil, err
	}
	storageIDs, err := s.requiredBackupStorageLocationIDs(ctx, input.StorageLocationID, input.StorageLocationIDs)
	if err != nil {
		return nil, err
	}
	return &BackupPlan{
		ID:                 "",
		Name:               name,
		EnvironmentID:      envID,
		ContainerID:        containerID,
		ContainerName:      containerName,
		StorageLocationID:  firstStorageLocationID(storageIDs),
		StorageLocationIDs: storageIDs,
		IncludeConfig:      true,
		IncludeLogs:        input.IncludeLogs,
		IncludeFilesystem:  input.IncludeFilesystem,
		IncludeImage:       input.IncludeImage,
		SelectedMounts:     input.SelectedMounts,
	}, nil
}

func (s *Service) executeRun(ctx context.Context, plan *BackupPlan, runID string) (*BackupRun, error) {
	done := markBackupRunActive(runID)
	defer done()

	if s.canRunAgentCentralizedBackup(ctx, plan) {
		return s.executeAgentCentralizedBackup(ctx, plan, runID)
	}

	archive, execErr := s.writeArchive(ctx, plan, runID)
	if execErr == nil {
		execErr = s.uploadArchiveDestinations(ctx, plan, runID, archive)
	}
	return s.finishRun(ctx, runID, archive, execErr)
}

// canRunAgentCentralizedBackup reports whether this plan should run through
// the agent-centralized pipeline: the agent creates the archive on the
// remote host, uploads it to a McHarbor-side temp path, and then the
// McHarbor server fans the archive out to every configured destination.
// This works for any destination type (local, OneDrive, SharePoint, ...)
// because the agent only needs to talk to the McHarbor server, never to
// the destination directly.
func (s *Service) canRunAgentCentralizedBackup(ctx context.Context, plan *BackupPlan) bool {
	if s.backupCrypto == nil || plan == nil || !s.pool.IsAgentEnv(plan.EnvironmentID) || !s.pool.AgentAtLeast(plan.EnvironmentID, agentBackupMinVersion) {
		return false
	}
	ids := backupStorageLocationIDs(plan.StorageLocationID, plan.StorageLocationIDs)
	if len(ids) == 0 {
		return false
	}
	locations, err := s.backupStorageDestinations(ctx, ids)
	if err != nil || len(locations) != len(ids) {
		return false
	}
	return true
}

// executeAgentCentralizedBackup drives an agent-side backup through the
// centralized-upload pipeline:
//
//  1. Pre-compute the McHarbor-side temp archive path ($DATA_DIR/backups/
//     containers/<runID>/mcharbor.tar) so the agent has a single,
//     known upload target.
//  2. Hand the agent a one-use upload transfer entry pointing at the
//     temp path; the agent creates the encrypted archive on its local
//     host and uploads it once via the standard agent-archives
//     endpoint, which lands the file on McHarbor.
//  3. After the agent completes, hand the on-disk archive to the
//     existing uploadArchiveDestinations pipeline, which fans it out
//     to every configured destination (local, OneDrive, SharePoint,
//     ...) in parallel with per-destination retry and progress.
//
// The agent no longer needs OAuth tokens for OneDrive/SharePoint — the
// server side already holds them — and the per-destination progress /
// retry logic that already powers local-env backups is now reused
// verbatim for agent-env backups.
func (s *Service) executeAgentCentralizedBackup(ctx context.Context, plan *BackupPlan, runID string) (*BackupRun, error) {
	agentConn, ok := s.pool.AgentConnection(plan.EnvironmentID)
	if !ok || agentConn.Transport == nil {
		return s.finishRun(ctx, runID, archiveResult{}, fmt.Errorf("agent not connected for environment %s", plan.EnvironmentID))
	}
	if err := os.MkdirAll(s.backupDir(), 0750); err != nil {
		return s.finishRun(ctx, runID, archiveResult{}, fmt.Errorf("creating backup directory: %w", err))
	}
	runDir := filepath.Join(s.backupDir(), safeArchiveName.ReplaceAllString(runID, "-"))
	if err := os.MkdirAll(runDir, 0750); err != nil {
		return s.finishRun(ctx, runID, archiveResult{}, fmt.Errorf("creating backup run directory: %w", err))
	}
	tempArchivePath := filepath.Join(runDir, "mcharbor.tar")

	entry, transferErr := agentArchiveTransfers.createUpload(runID, tempArchivePath)
	if transferErr != nil {
		return s.finishRun(ctx, runID, archiveResult{}, transferErr)
	}
	defer agentArchiveTransfers.cancel(entry.ID)

	// The agent receives a single StorageDestination pointing at the
	// McHarbor-side temp path. It does not need to know about the
	// user's real storage locations; the server-side fan-out handles
	// them after the upload completes.
	destination := coreagent.BackupStorageDestination{
		ID:           "mcharbor-temp",
		Name:         "mcharbor-temp",
		LocationType: "local",
		UploadURL:    "/api/container-backups/internal/agent-archives/" + entry.ID,
		Token:        entry.Token,
		RemotePath:   tempArchivePath,
	}

	payload := coreagent.BackupPayload{
		TransferID:          runID,
		ContainerID:         plan.ContainerID,
		ContainerName:       plan.ContainerName,
		IncludeConfig:       plan.IncludeConfig,
		IncludeLogs:         plan.IncludeLogs,
		IncludeFilesystem:   plan.IncludeFilesystem,
		IncludeImage:        plan.IncludeImage,
		SelectedMounts:      plan.SelectedMounts,
		EncryptionKey:       s.backupCrypto.KeyMaterialBase64(),
		StorageDestinations: []coreagent.BackupStorageDestination{destination},
	}
	s.updateRunProgress(ctx, runID, "agent_backup", "Agent is creating the backup archive locally.")
	result, execErr := agentConn.Transport.StartBackupRun(ctx, payload, func(progress coreagent.BackupPayload) {
		stage := strings.TrimSpace(progress.Stage)
		if stage == "" {
			stage = "agent_backup"
		}
		message := agentBackupProgressMessage(progress)
		s.updateRunProgress(ctx, runID, stage, message)
	})

	archive := archiveResult{
		path:       tempArchivePath,
		encryption: backupcrypto.Algorithm,
		keyID:      s.backupCrypto.KeyID(),
	}
	if result != nil {
		archive.size = result.Size
	}
	if execErr != nil {
		return s.finishRun(ctx, runID, archive, execErr)
	}
	// Fan the on-disk archive out to every configured destination in
	// parallel. uploadArchiveDestinations creates the per-destination
	// rows, runs the upload, and writes success/failure per destination.
	uploadErr := s.uploadArchiveDestinations(ctx, plan, runID, archive)
	return s.finishRun(ctx, runID, archive, uploadErr)
}

func (s *Service) startBackgroundRun(plan *BackupPlan, runID string) {
	planCopy := *plan
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), backupRunBackgroundTimeout)
		runCancelers.Store(runID, cancel)
		defer func() {
			cancel()
			runCancelers.Delete(runID)
		}()
		if _, err := s.executeRun(ctx, &planCopy, runID); err != nil && s.logger != nil {
			s.logger.Error("background container backup failed", "run", runID, "env", planCopy.EnvironmentID, "container", planCopy.ContainerID, "error", err)
		}
	}()
}

func (s *Service) containerName(ctx context.Context, envID, containerID string) (string, error) {
	cli, err := s.client(envID)
	if err != nil {
		return "", err
	}
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	info, err := cli.ContainerInspect(opCtx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspecting container: %w", err)
	}
	name := strings.TrimPrefix(info.Name, "/")
	if name == "" {
		name = shortID(info.ID)
	}
	return name, nil
}

// resolveContainerForBackup inspects the container by id, falling back to
// name lookup if the id has rotated (e.g. docker compose restart).
//
// Docker Compose typically assigns a fresh container id on every `docker
// compose up`, which breaks plans that captured a previous id. The plan
// stores both id and name; on inspect failure we list containers filtered
// by name and use the live id for this run. The plan row itself is not
// rewritten — next run will retry the lookup.
func (s *Service) resolveContainerForBackup(ctx context.Context, opCtx context.Context, cli *client.Client, plan *BackupPlan, logger *slog.Logger) (types.ContainerJSON, string, error) {
	info, err := cli.ContainerInspect(opCtx, plan.ContainerID)
	if err == nil {
		return info, plan.ContainerID, nil
	}
	if !isContainerNotFound(err) {
		return types.ContainerJSON{}, plan.ContainerID, err
	}
	if plan.ContainerName == "" {
		return types.ContainerJSON{}, plan.ContainerID, fmt.Errorf("inspecting container for backup: %w", err)
	}
	if logger != nil {
		logger.Info(
			"container backup: stored container id is stale, resolving by name",
			"container_id", plan.ContainerID, "container_name", plan.ContainerName,
		)
	}
	f := filters.NewArgs()
	f.Add("name", plan.ContainerName)
	list, listErr := cli.ContainerList(opCtx, container.ListOptions{All: true, Filters: f})
	if listErr != nil {
		return types.ContainerJSON{}, plan.ContainerID, fmt.Errorf("resolving container by name: %w", listErr)
	}
	for _, c := range list {
		if strings.TrimPrefix(c.Names[0], "/") == plan.ContainerName {
			info, inspectErr := cli.ContainerInspect(opCtx, c.ID)
			if inspectErr != nil {
				return types.ContainerJSON{}, plan.ContainerID, fmt.Errorf("inspecting resolved container: %w", inspectErr)
			}
			return info, c.ID, nil
		}
	}
	return types.ContainerJSON{}, plan.ContainerID, fmt.Errorf("no container named %q found in environment", plan.ContainerName)
}

// isContainerNotFound returns true for Docker SDK errors that mean
// "this container id no longer exists". Used to decide whether the
// backup should fall back to a name-based lookup.
func isContainerNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "No such container") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "404")
}

func (s *Service) createRun(ctx context.Context, planID, envID, containerID string) (*BackupRun, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO container_backup_runs (id, plan_id, operation, environment_id, container_id, status, started_at, created_at, updated_at)
		VALUES (?, ?, 'backup', ?, ?, 'running', ?, ?, ?)`, id, nullString(planID), envID, containerID, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("creating backup run: %w", err)
	}
	if err := s.setRunProgress(ctx, id, "queued", ""); err != nil && s.logger != nil {
		s.logger.Warn("container backup progress update failed", "run", id, "stage", "queued", "error", err)
	}
	return &BackupRun{ID: id, PlanID: planID, EnvironmentID: envID, ContainerID: containerID, Status: "running", ProgressStage: "queued", ProgressUpdatedAt: now, StartedAt: now, CreatedAt: now, UpdatedAt: now}, nil
}

type archiveResult struct {
	path       string
	size       int64
	encryption string
	keyID      string
}

func (s *Service) finishRun(ctx context.Context, runID string, archive archiveResult, execErr error) (*BackupRun, error) {
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backupRunFinalizeTimeout)
	defer cancel()

	now := time.Now().UTC()
	status := "success"
	errorText := ""
	var started, envID, containerID, operation, progressStage string
	if err := s.db.QueryRowContext(finishCtx, `
		SELECT started_at, environment_id, container_id, COALESCE(operation, 'backup'), COALESCE(progress_stage, '')
		FROM container_backup_runs
		WHERE id = ?`, runID).Scan(&started, &envID, &containerID, &operation, &progressStage); err != nil {
		return nil, fmt.Errorf("reading backup run start time: %w", err)
	}
	finalProgressStage := status
	progressMessage := ""
	if execErr != nil {
		status = "failure"
		finalProgressStage = backupRunFailureStage(operation, progressStage)
		errorText = backupRunFailureText(operation, progressStage)
		progressMessage = errorText
	}
	if execErr != nil && s.logger != nil {
		s.logger.Error("container backup run failed", "run", runID, "env", envID, "container", containerID, "stage", progressStage, "error", execErr)
	}
	startedAt, _ := time.Parse(time.RFC3339, started)
	duration := now.Sub(startedAt).Milliseconds()
	result, err := s.db.ExecContext(finishCtx, `
		UPDATE container_backup_runs
		SET status = ?, archive_path = ?, archive_size = ?, archive_encryption = ?, archive_key_id = ?,
		    error = ?, progress_stage = ?, progress_message = ?, progress_updated_at = ?,
		    completed_at = ?, duration_ms = ?, updated_at = ?
		WHERE id = ? AND status = 'running'`,
		status, nullString(archive.path), archive.size, archive.encryption, archive.keyID,
		nullString(errorText), finalProgressStage, progressMessage, now.Format(time.RFC3339),
		now.Format(time.RFC3339), duration, now.Format(time.RFC3339), runID)
	if err != nil {
		return nil, fmt.Errorf("finishing backup run: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		run, readErr := s.runByID(finishCtx, runID)
		if readErr != nil {
			return nil, readErr
		}
		if run.Status != "running" {
			return run, nil
		}
		return run, execErr
	}
	if execErr != nil {
		return nil, execErr
	}
	run, err := s.runByID(finishCtx, runID)
	if err != nil {
		return nil, err
	}
	if err := s.pruneBackupPlanRuns(finishCtx, run); err != nil && s.logger != nil {
		s.logger.Warn("container backup retention pruning failed", "plan", run.PlanID, "env", run.EnvironmentID, "container", run.ContainerID, "error", err)
	}
	return run, nil
}

func markBackupRunActive(runID string) func() {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return func() {}
	}
	activeBackupRuns.Store(runID, struct{}{})
	return func() {
		activeBackupRuns.Delete(runID)
	}
}

func backupRunIsActive(runID string) bool {
	_, ok := activeBackupRuns.Load(strings.TrimSpace(runID))
	return ok
}

func backupRunFailureStage(operation, stage string) string {
	stage = strings.TrimSpace(stage)
	if stage == "" || stage == "queued" || stage == "success" || stage == "failure" {
		return "failure"
	}
	if operation == "restore" && strings.HasPrefix(stage, "restore_") {
		return stage
	}
	return stage
}

func backupRunFailureText(operation, stage string) string {
	if operation == "restore" {
		switch backupRunFailureStage(operation, stage) {
		case "restore_connecting":
			return "Restore failed while connecting to the Docker environment. Check McHarbor logs."
		case "restore_inspecting":
			return "Restore failed while inspecting the target container. Check McHarbor logs."
		case "restore_scanning":
			return "Restore failed while reading the backup archive. Check McHarbor logs."
		case "restore_image":
			return "Restore failed while loading the container image. Check McHarbor logs."
		case "restore_filesystem":
			return "Restore failed while restoring the container filesystem. Check McHarbor logs."
		case "restore_mounts":
			return "Restore failed while restoring mounted data. Check McHarbor logs."
		default:
			return "Restore failed. Check McHarbor logs."
		}
	}
	switch backupRunFailureStage(operation, stage) {
	case "connecting":
		return "Backup failed while connecting to the Docker environment. Check McHarbor logs."
	case "inspecting":
		return "Backup failed while inspecting the container. Check McHarbor logs."
	case "preparing":
		return "Backup failed while preparing the archive. Check McHarbor logs."
	case "writing", "manifest", "config", "finalizing":
		return "Backup failed while writing the archive. Check McHarbor logs."
	case "logs":
		return "Backup failed while reading container logs. Check McHarbor logs."
	case "filesystem":
		return "Backup failed while exporting the container filesystem. Check McHarbor logs."
	case "image":
		return "Backup failed while saving the container image. Check McHarbor logs."
	case "mounts":
		return "Backup failed while copying mounted data. Check McHarbor logs."
	case "uploading":
		return "Backup failed while uploading to selected storage. Check McHarbor logs."
	default:
		return "Backup failed. Check McHarbor logs."
	}
}

func (s *Service) recoverAbandonedRun(ctx context.Context, runID string) error {
	archivePath := filepath.Join(s.backupDir(), safeArchiveName.ReplaceAllString(runID, "-"), "mcharbor.tar")
	_, statErr := os.Stat(archivePath)
	if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("checking abandoned backup archive: %w", statErr)
	}
	if statErr == nil {
		if err := os.RemoveAll(filepath.Dir(archivePath)); err != nil {
			return fmt.Errorf("removing abandoned backup archive directory: %w", err)
		}
	}
	if err := s.failUploadingDestinations(ctx, runID, "Upload did not finish before the backup run ended."); err != nil {
		return err
	}
	_, err := s.finishRun(ctx, runID, archiveResult{}, fmt.Errorf("backup abandoned before completion"))
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func (s *Service) recoverAbandonedRestoreRuns(ctx context.Context, envID, containerID string) error {
	cutoff := time.Now().UTC().Add(-restoreRunProgressStaleAfter).Format(time.RFC3339)
	query := `
		SELECT id, started_at
		FROM container_backup_runs
		WHERE status = 'running'
		  AND operation = 'restore'
		  AND COALESCE(NULLIF(progress_updated_at, ''), started_at) < ?`
	args := []any{cutoff}
	if envID != "" {
		query += " AND environment_id = ?"
		args = append(args, envID)
	}
	if containerID != "" {
		query += " AND container_id = ?"
		args = append(args, containerID)
	}
	query += " ORDER BY started_at ASC LIMIT 1000"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("listing abandoned restore runs: %w", err)
	}

	type abandonedRestoreRun struct {
		id      string
		started string
	}
	runs := []abandonedRestoreRun{}
	for rows.Next() {
		var run abandonedRestoreRun
		if err := rows.Scan(&run.id, &run.started); err != nil {
			rows.Close()
			return fmt.Errorf("scanning abandoned restore run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("closing abandoned restore run rows: %w", err)
	}

	for _, run := range runs {
		if backupRunIsActive(run.id) {
			continue
		}
		_, err := s.finishRun(ctx, run.id, archiveResult{}, fmt.Errorf("restore abandoned before completion"))
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && s.logger != nil {
			s.logger.Warn("container restore abandoned run recovery failed", "run", run.id, "started", run.started, "error", err)
		}
	}
	return nil
}

func (s *Service) recoverOrphanUploadingDestinations(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		UPDATE container_backup_run_destinations
		SET status = 'failure',
		    error = CASE WHEN COALESCE(error, '') = '' THEN 'Upload did not finish before the backup run ended.' ELSE error END,
		    updated_at = ?
		WHERE status = 'uploading'
		  AND run_id IN (SELECT id FROM container_backup_runs WHERE status <> 'running')`,
		now)
	if err != nil {
		return fmt.Errorf("recovering unfinished backup destinations: %w", err)
	}
	return nil
}

func (s *Service) failUploadingDestinations(ctx context.Context, runID, message string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		UPDATE container_backup_run_destinations
		SET status = 'failure',
		    error = CASE WHEN COALESCE(error, '') = '' THEN ? ELSE error END,
		    updated_at = ?
		WHERE run_id = ? AND status = 'uploading'`,
		message, now, runID)
	if err != nil {
		return fmt.Errorf("marking unfinished backup destinations failed: %w", err)
	}
	return nil
}

func (s *Service) setRunProgress(ctx context.Context, runID, stage, message string) error {
	progressCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backupRunProgressTimeout)
	defer cancel()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(progressCtx, `
		UPDATE container_backup_runs
		SET progress_stage = ?, progress_message = ?, progress_updated_at = ?, updated_at = ?
		WHERE id = ? AND status = 'running'`,
		stage, message, now, now, runID)
	if err != nil {
		return fmt.Errorf("updating backup progress: %w", err)
	}
	return nil
}

func (s *Service) updateRunProgress(ctx context.Context, runID, stage, message string) {
	if err := s.setRunProgress(ctx, runID, stage, message); err != nil && s.logger != nil {
		s.logger.Warn("container backup progress update failed", "run", runID, "stage", stage, "error", err)
	}
}

func (s *Service) runByID(ctx context.Context, id string) (*BackupRun, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(plan_id, ''), COALESCE(operation, 'backup'), COALESCE(source_run_id, ''), environment_id, container_id, status, COALESCE(archive_path, ''),
		       archive_size, COALESCE(archive_encryption, ''), COALESCE(archive_key_id, ''), COALESCE(error, ''),
		       COALESCE(progress_stage, ''), COALESCE(progress_message, ''), COALESCE(progress_updated_at, ''),
		       started_at, COALESCE(completed_at, ''), duration_ms, created_at, updated_at
		FROM container_backup_runs WHERE id = ?`, id)
	run, err := scanRun(row)
	if err != nil {
		return nil, err
	}
	s.annotateRunKeyRequirement(&run)
	if err := s.hydrateRunDestinations(ctx, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// HasRunningRunForPlan reports whether the plan currently has a backup
// run with status 'running'. Used by the scheduler to skip duplicate
// firings across process restarts or in-memory state loss.
func (s *Service) HasRunningRunForPlan(ctx context.Context, planID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM container_backup_runs
		WHERE plan_id = ? AND status = 'running'`,
		planID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// WaitForRun blocks until the given run reaches a terminal state
// (success, failure, or cancelled) or the context is cancelled. Used by
// the scheduler so the in-memory "running" flag stays set for the full
// duration of a background backup, while the actual archive and upload
// work runs in a goroutine spawned by StartPlan.
func (s *Service) WaitForRun(ctx context.Context, runID string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		run, err := s.runByID(ctx, runID)
		if err != nil {
			return err
		}
		if run.Status != "running" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// DeleteRun deletes a finished or failed backup run and its local archive files.
func (s *Service) DeleteRun(ctx context.Context, id string) (bool, error) {
	run, err := s.runByID(ctx, id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if run.Status == "running" {
		return false, ErrBackupRunActive
	}

	if strings.TrimSpace(run.ArchivePath) != "" {
		archivePath, err := s.validatedArchivePath(run.ArchivePath)
		if err == nil {
			if err := os.RemoveAll(filepath.Dir(archivePath)); err != nil {
				return false, fmt.Errorf("removing backup archive directory: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("validating backup archive before delete: %w", err)
		}
	} else {
		runDir := filepath.Join(s.backupDir(), safeArchiveName.ReplaceAllString(run.ID, "-"))
		if err := os.RemoveAll(runDir); err != nil {
			return false, fmt.Errorf("removing backup run directory: %w", err)
		}
	}

	result, err := s.db.ExecContext(ctx, "DELETE FROM container_backup_runs WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting backup run: %w", err)
	}
	return db.RowsAffected(result) > 0, nil
}

// CancelRun marks a running backup or restore as cancelled. The run is
// transitioned to a terminal "cancelled" state in the database and the
// background goroutine's context is cancelled so any in-flight Docker or
// storage call is aborted. The goroutine will eventually unwind and the
// existing finishRun call is a no-op because the WHERE clause filters on
// status = 'running'.
func (s *Service) CancelRun(ctx context.Context, id string) (*BackupRun, error) {
	run, err := s.runByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if run.Status != "running" {
		return run, ErrBackupRunNotCancellable
	}

	now := time.Now().UTC()
	startedAt, _ := time.Parse(time.RFC3339, run.StartedAt)
	duration := now.Sub(startedAt).Milliseconds()
	result, err := s.db.ExecContext(ctx, `
		UPDATE container_backup_runs
		SET status = 'cancelled',
		    progress_stage = 'cancelled',
		    progress_message = 'Cancelled by user',
		    error = 'cancelled by user',
		    progress_updated_at = ?,
		    completed_at = ?,
		    duration_ms = ?,
		    updated_at = ?
		WHERE id = ? AND status = 'running'`,
		now.Format(time.RFC3339), now.Format(time.RFC3339), duration, now.Format(time.RFC3339), id)
	if err != nil {
		return nil, fmt.Errorf("cancelling backup run: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		// Another caller beat us to it (e.g. stale-run reaper). Re-read and
		// return the current record.
		return s.runByID(ctx, id)
	}

	if value, ok := runCancelers.LoadAndDelete(id); ok {
		if cancel, isCancel := value.(context.CancelFunc); isCancel {
			cancel()
		}
	}

	if s.logger != nil {
		s.logger.Info("container backup run cancelled by user", "run", id, "env", run.EnvironmentID, "container", run.ContainerID, "operation", run.Operation)
	}

	updated, err := s.runByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.pruneBackupPlanRuns(ctx, updated); err != nil && s.logger != nil {
		s.logger.Warn("container backup retention pruning failed", "plan", updated.PlanID, "env", updated.EnvironmentID, "container", updated.ContainerID, "error", err)
	}
	return updated, nil
}

func (s *Service) annotateRunKeyRequirement(run *BackupRun) {
	if strings.TrimSpace(run.ArchiveEncryption) == "" {
		return
	}
	if s.backupCrypto == nil || strings.TrimSpace(run.ArchiveKeyID) != s.backupCrypto.KeyID() {
		run.RequiresSecretKey = true
	}
}

type backupRunRetentionRef struct {
	id          string
	archivePath string
	startedAt   string
}

func (s *Service) pruneBackupPlanRuns(ctx context.Context, current *BackupRun) error {
	if current == nil || strings.TrimSpace(current.PlanID) == "" {
		return nil
	}

	var retentionCount int
	var retentionDays int
	err := s.db.QueryRowContext(ctx, `
		SELECT retention_count, retention_days
		FROM container_backup_plans
		WHERE id = ?
		LIMIT 1`, current.PlanID).Scan(&retentionCount, &retentionDays)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading backup plan retention: %w", err)
	}
	if retentionCount <= 0 && retentionDays <= 0 {
		return nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(archive_path, ''), started_at
		FROM container_backup_runs
		WHERE plan_id = ? AND status = 'success'
		ORDER BY started_at DESC
		LIMIT 1000`, current.PlanID)
	if err != nil {
		return fmt.Errorf("listing backup runs for retention: %w", err)
	}
	defer rows.Close()

	runs := []backupRunRetentionRef{}
	for rows.Next() {
		var run backupRunRetentionRef
		if err := rows.Scan(&run.id, &run.archivePath, &run.startedAt); err != nil {
			return fmt.Errorf("scanning backup retention run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating backup retention runs: %w", err)
	}

	prune := map[string]backupRunRetentionRef{}
	if retentionCount > 0 && len(runs) > retentionCount {
		for _, run := range runs[retentionCount:] {
			prune[run.id] = run
		}
	}
	if retentionDays > 0 {
		cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
		for _, run := range runs {
			startedAt, err := time.Parse(time.RFC3339, run.startedAt)
			if err != nil {
				return fmt.Errorf("parsing backup retention start time: %w", err)
			}
			if startedAt.Before(cutoff) {
				prune[run.id] = run
			}
		}
	}
	if len(prune) == 0 {
		return nil
	}

	for _, run := range prune {
		if strings.TrimSpace(run.archivePath) != "" {
			archivePath, err := s.validatedArchivePath(run.archivePath)
			if err == nil {
				if err := os.RemoveAll(filepath.Dir(archivePath)); err != nil {
					return fmt.Errorf("removing backup archive directory: %w", err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("validating backup archive before retention cleanup: %w", err)
			}
		}
		if _, err := s.db.ExecContext(ctx, "DELETE FROM container_backup_runs WHERE id = ?", run.id); err != nil {
			return fmt.Errorf("deleting retained backup run %s: %w", run.id, err)
		}
	}

	return nil
}

func validatePlanRetention(count, days int) error {
	if count < 0 || count > 1000 {
		return fmt.Errorf("backup plan retention count must be between 0 and 1000")
	}
	if days < 0 || days > 3650 {
		return fmt.Errorf("backup plan retention days must be between 0 and 3650")
	}
	return nil
}

// Download returns a validated backup archive for a completed run.
func (s *Service) Download(ctx context.Context, runID string) (*BackupDownload, error) {
	run, err := s.runByID(ctx, strings.TrimSpace(runID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if run.Status != "success" || strings.TrimSpace(run.ArchivePath) == "" {
		return nil, ErrBackupRunNotDownloadable
	}

	archivePath, err := s.validatedArchivePath(run.ArchivePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	stat, err := os.Stat(archivePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stating backup archive: %w", err)
	}
	if stat.IsDir() {
		return nil, ErrBackupRunNotDownloadable
	}

	fileName, err := s.downloadFileName(ctx, run)
	if err != nil {
		return nil, err
	}

	return &BackupDownload{
		RunID:       run.ID,
		Path:        archivePath,
		FileName:    fileName,
		ContentType: "application/x-tar",
		Size:        stat.Size(),
		ModTime:     stat.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// RestoreOptions returns restorable entries found in a completed encrypted backup run.
func (s *Service) RestoreOptions(ctx context.Context, runID string, input RestoreBackupOptionsInput) (*RestoreBackupOptions, error) {
	run, err := s.runByID(ctx, strings.TrimSpace(runID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if run.Operation != "backup" || run.Status != "success" || strings.TrimSpace(run.ArchivePath) == "" {
		return nil, ErrBackupRunNotDownloadable
	}

	file, decrypted, err := s.openRestoreArchive(run, input.SecretKey)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	defer decrypted.Close()

	options, err := restoreArchiveOptions(decrypted)
	if err != nil {
		return nil, err
	}
	return &RestoreBackupOptions{RunID: run.ID, Items: options}, nil
}

// StartRestore starts a background restore run and returns the tracked run.
func (s *Service) StartRestore(ctx context.Context, runID string, input RestoreBackupInput) (*BackupRun, error) {
	sourceRun, err := s.runByID(ctx, strings.TrimSpace(runID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if sourceRun.Operation != "backup" || sourceRun.Status != "success" || strings.TrimSpace(sourceRun.ArchivePath) == "" {
		return nil, ErrBackupRunNotDownloadable
	}
	if _, err := s.restoreCrypto(sourceRun, input.SecretKey); err != nil {
		return nil, err
	}
	run, err := s.createRestoreRun(ctx, sourceRun)
	if err != nil {
		return nil, err
	}
	s.startBackgroundRestore(run.ID, sourceRun.ID, input)
	return run, nil
}

// Restore applies restorable entries from a completed encrypted backup run to its original container.
func (s *Service) Restore(ctx context.Context, runID string, input RestoreBackupInput) (*RestoreBackupResult, error) {
	run, err := s.runByID(ctx, strings.TrimSpace(runID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.restoreFromRun(ctx, "", run, input)
}

func (s *Service) createRestoreRun(ctx context.Context, sourceRun *BackupRun) (*BackupRun, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO container_backup_runs (id, plan_id, operation, source_run_id, environment_id, container_id, status, started_at, created_at, updated_at)
		VALUES (?, NULL, 'restore', ?, ?, ?, 'running', ?, ?, ?)`,
		id, sourceRun.ID, sourceRun.EnvironmentID, sourceRun.ContainerID, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("creating restore run: %w", err)
	}
	if err := s.setRunProgress(ctx, id, "queued", ""); err != nil && s.logger != nil {
		s.logger.Warn("container restore progress update failed", "run", id, "stage", "queued", "error", err)
	}
	return &BackupRun{ID: id, Operation: "restore", SourceRunID: sourceRun.ID, EnvironmentID: sourceRun.EnvironmentID, ContainerID: sourceRun.ContainerID, Status: "running", ProgressStage: "queued", ProgressUpdatedAt: now, StartedAt: now, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) startBackgroundRestore(restoreRunID, sourceRunID string, input RestoreBackupInput) {
	go func() {
		done := markBackupRunActive(restoreRunID)
		defer done()

		ctx, cancel := context.WithTimeout(context.Background(), backupRunBackgroundTimeout)
		runCancelers.Store(restoreRunID, cancel)
		defer func() {
			cancel()
			runCancelers.Delete(restoreRunID)
		}()

		sourceRun, err := s.runByID(ctx, sourceRunID)
		if err == nil && sourceRun != nil {
			_, err = s.restoreFromRun(ctx, restoreRunID, sourceRun, input)
		}
		archive := archiveResult{}
		if _, finishErr := s.finishRun(ctx, restoreRunID, archive, err); finishErr != nil && s.logger != nil {
			s.logger.Error("background container restore failed", "run", restoreRunID, "source", sourceRunID, "error", finishErr)
		}
	}()
}

func (s *Service) restoreFromRun(ctx context.Context, progressRunID string, run *BackupRun, input RestoreBackupInput) (*RestoreBackupResult, error) {
	if run.Operation != "backup" || run.Status != "success" || strings.TrimSpace(run.ArchivePath) == "" {
		return nil, ErrBackupRunNotDownloadable
	}
	if s.canRunAgentRestore(run) {
		return s.restoreFromRunViaAgentArchive(ctx, progressRunID, run, input)
	}
	if progressRunID != "" {
		s.updateRunProgress(ctx, progressRunID, "restore_scanning", "")
	}
	file, decrypted, err := s.openRestoreArchive(run, input.SecretKey)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	defer decrypted.Close()

	if progressRunID != "" {
		s.updateRunProgress(ctx, progressRunID, "restore_connecting", "")
	}
	cli, err := s.client(run.EnvironmentID)
	if err != nil {
		return nil, err
	}
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	if progressRunID != "" {
		s.updateRunProgress(opCtx, progressRunID, "restore_inspecting", "")
	}
	if _, err := cli.ContainerInspect(opCtx, run.ContainerID); err != nil {
		return nil, fmt.Errorf("inspecting container before restore: %w", err)
	}

	tr := tar.NewReader(decrypted)
	mountTargets := map[string]string{}
	restored := []string{}
	wanted := restoreSelection(input.RestoreItems)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading backup restore entry: %w", err)
		}
		if header == nil || header.Typeflag != tar.TypeReg {
			continue
		}

		switch {
		case header.Name == "manifest.json":
			mountTargets = restoreMountTargets(tr)
		case header.Name == "image/image.tar":
			if !wanted("image") {
				continue
			}
			if progressRunID != "" {
				s.updateRunProgress(opCtx, progressRunID, "restore_image", "")
			}
			if err := restoreImageArchive(opCtx, cli, tr); err != nil {
				return nil, err
			}
			restored = append(restored, "image")
		case header.Name == "container/filesystem.tar":
			if !wanted("filesystem") {
				continue
			}
			if progressRunID != "" {
				s.updateRunProgress(opCtx, progressRunID, "restore_filesystem", "")
			}
			reader := s.restoreProgressReader(opCtx, progressRunID, "restore_filesystem", "Restoring container filesystem", tr, header.Size)
			if err := s.copyRestoreEntryToContainer(opCtx, cli, run, progressRunID, "restore_filesystem", "Restoring container filesystem", header.Name, "/", reader, header.Size, input.SecretKey); err != nil {
				return nil, fmt.Errorf("restoring container filesystem: %w", err)
			}
			restored = append(restored, "filesystem")
		case strings.HasPrefix(header.Name, "mounts/") && strings.HasSuffix(header.Name, ".tar"):
			target := mountTargets[header.Name]
			if target == "" {
				return nil, fmt.Errorf("backup mount target is missing")
			}
			if !wanted("mount:" + target) {
				continue
			}
			if progressRunID != "" {
				s.updateRunProgress(opCtx, progressRunID, "restore_mounts", "Restoring mounted data "+target+".")
			}
			restoreTarget := filepath.Dir(target)
			if restoreTarget == "." {
				restoreTarget = "/"
			}
			reader := s.restoreProgressReader(opCtx, progressRunID, "restore_mounts", "Restoring mounted data "+target, tr, header.Size)
			if err := s.copyRestoreEntryToContainer(opCtx, cli, run, progressRunID, "restore_mounts", "Restoring mounted data "+target, header.Name, restoreTarget, reader, header.Size, input.SecretKey); err != nil {
				return nil, fmt.Errorf("restoring mounted data: %w", err)
			}
			restored = append(restored, "mount:"+target)
		}
	}
	if len(restored) == 0 {
		return nil, ErrBackupRestoreNoRestorableEntries
	}

	return &RestoreBackupResult{RunID: run.ID, Restored: restored}, nil
}

func (s *Service) canRunAgentRestore(run *BackupRun) bool {
	if run == nil || !s.pool.IsAgentEnv(run.EnvironmentID) || !s.pool.AgentAtLeast(run.EnvironmentID, agentBackupMinVersion) {
		return false
	}
	return strings.TrimSpace(run.ArchivePath) != ""
}

func (s *Service) restoreFromRunViaAgentArchive(ctx context.Context, progressRunID string, run *BackupRun, input RestoreBackupInput) (*RestoreBackupResult, error) {
	cryptoSvc, err := s.restoreCrypto(run, input.SecretKey)
	if err != nil {
		return nil, err
	}
	archivePath, err := s.validatedArchivePath(run.ArchivePath)
	if err != nil {
		return nil, err
	}
	agentConn, ok := s.pool.AgentConnection(run.EnvironmentID)
	if !ok || agentConn.Transport == nil {
		return nil, fmt.Errorf("agent not connected for environment %s", run.EnvironmentID)
	}
	entry, err := agentArchiveTransfers.createDownload(run.ID, archivePath)
	if err != nil {
		return nil, err
	}
	defer agentArchiveTransfers.cancel(entry.ID)

	if progressRunID != "" {
		s.updateRunProgress(ctx, progressRunID, "restore_download", "Agent is downloading backup archive.")
	}
	payload := coreagent.BackupPayload{
		TransferID:    entry.ID,
		ContainerID:   run.ContainerID,
		ArchiveURL:    "/api/container-backups/internal/agent-archives/" + entry.ID,
		ArchiveToken:  entry.Token,
		ArchiveSize:   run.ArchiveSize,
		EncryptionKey: cryptoSvc.KeyMaterialBase64(),
		RestoreItems:  input.RestoreItems,
	}
	result, err := agentConn.Transport.StartBackupRestore(ctx, payload, func(progress coreagent.BackupPayload) {
		if progressRunID == "" {
			return
		}
		stage := strings.TrimSpace(progress.Stage)
		if stage == "" {
			stage = "restore_download"
		}
		s.updateRunProgress(ctx, progressRunID, stage, agentBackupProgressMessage(progress))
	})
	if err != nil {
		return nil, err
	}
	if result == nil || len(result.Restored) == 0 {
		return nil, ErrBackupRestoreNoRestorableEntries
	}
	return &RestoreBackupResult{RunID: run.ID, Restored: result.Restored}, nil
}

func (s *Service) copyRestoreEntryToContainer(ctx context.Context, cli *client.Client, run *BackupRun, progressRunID, progressStage, label, entryName, targetPath string, reader io.Reader, size int64, secretKey string) error {
	if !s.pool.IsAgentEnv(run.EnvironmentID) {
		return cli.CopyToContainer(ctx, run.ContainerID, targetPath, reader, container.CopyToContainerOptions{AllowOverwriteDirWithFile: false})
	}
	return s.copyRestoreEntryToContainerViaAgent(ctx, run, progressRunID, progressStage, label, entryName, targetPath, size, secretKey)
}

func (s *Service) copyRestoreEntryToContainerViaAgent(ctx context.Context, run *BackupRun, progressRunID, progressStage, label, entryName, targetPath string, size int64, secretKey string) error {
	agentConn, ok := s.pool.AgentConnection(run.EnvironmentID)
	if !ok {
		return fmt.Errorf("agent not connected for environment %s", run.EnvironmentID)
	}

	entry, err := restoreTransfers.create(run.ID, secretKey, entryName, size)
	if err != nil {
		return err
	}
	transferURL := "/api/container-backups/internal/transfers/" + entry.ID

	progress := func(bytes int64, stage string) {
		if progressRunID == "" {
			return
		}
		message := backupRestoreProgressMessage(label, bytes, size)
		if strings.EqualFold(stage, "apply") {
			message = label + ": applying archive to container."
		}
		s.updateRunProgress(ctx, progressRunID, progressStage, message)
	}

	err = agentConn.Transport.StartRestoreTransfer(ctx, entry.ID, run.ContainerID, targetPath, transferURL, entry.Token, size, progress)
	if err != nil {
		restoreTransfers.cancel(entry.ID)
		return err
	}
	if progressRunID != "" {
		s.updateRunProgress(ctx, progressRunID, progressStage, backupRestoreProgressMessage(label, size, size))
	}
	return nil
}

func (s *Service) openRestoreArchive(run *BackupRun, secretKey string) (*os.File, io.ReadCloser, error) {
	archivePath, err := s.validatedArchivePath(run.ArchivePath)
	if err != nil {
		return nil, nil, err
	}

	cryptoSvc, err := s.restoreCrypto(run, secretKey)
	if err != nil {
		return nil, nil, err
	}

	file, err := os.Open(archivePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, ErrBackupRunNotDownloadable
		}
		return nil, nil, fmt.Errorf("opening backup archive for restore: %w", err)
	}

	decrypted, _, err := cryptoSvc.DecryptReader(file)
	if err != nil {
		if closeErr := file.Close(); closeErr != nil && s.logger != nil {
			s.logger.Warn("backup archive close after decrypt failure failed", "run", run.ID, "error", closeErr)
		}
		return nil, nil, ErrBackupRestoreKeyInvalid
	}
	return file, decrypted, nil
}

// RestoreUploaded stores an uploaded encrypted archive and restores it to the selected container.
func (s *Service) RestoreUploaded(ctx context.Context, input RestoreUploadedBackupInput) (*RestoreBackupResult, error) {
	if strings.TrimSpace(input.EnvironmentID) == "" || strings.TrimSpace(input.ContainerID) == "" || input.Reader == nil {
		return nil, fmt.Errorf("uploaded backup restore input is invalid")
	}

	archive := archiveResult{}
	secretKey := strings.TrimSpace(input.SecretKey)
	switch {
	case secretKey != "":
		cryptoSvc, err := backupcrypto.NewFromKeyMaterial(secretKey)
		if err != nil {
			return nil, ErrBackupRestoreKeyInvalid
		}
		archive.encryption = backupcrypto.Algorithm
		archive.keyID = cryptoSvc.KeyID()
	case s.backupCrypto != nil:
		archive.encryption = backupcrypto.Algorithm
		archive.keyID = s.backupCrypto.KeyID()
	default:
		return nil, ErrBackupRestoreSecretRequired
	}

	run, err := s.createRun(ctx, "", input.EnvironmentID, input.ContainerID)
	if err != nil {
		return nil, err
	}

	archive.path, archive.size, err = s.storeUploadedArchive(run.ID, input.Reader)
	if err != nil {
		if cleanupErr := s.deleteRunArchive(ctx, run.ID, archive.path); cleanupErr != nil && s.logger != nil {
			s.logger.Warn("uploaded backup cleanup failed", "run", run.ID, "error", cleanupErr)
		}
		if _, finishErr := s.finishRun(ctx, run.ID, archive, err); finishErr != nil && s.logger != nil {
			s.logger.Warn("uploaded backup failure finalization failed", "run", run.ID, "error", finishErr)
		}
		return nil, err
	}
	run, err = s.finishRun(ctx, run.ID, archive, nil)
	if err != nil {
		return nil, err
	}
	return s.Restore(ctx, run.ID, RestoreBackupInput{SecretKey: secretKey})
}

func (s *Service) restoreCrypto(run *BackupRun, secretKey string) (*backupcrypto.Service, error) {
	if strings.TrimSpace(run.ArchiveKeyID) == "" {
		return nil, ErrBackupRunNotDownloadable
	}
	if s.backupCrypto != nil && run.ArchiveKeyID == s.backupCrypto.KeyID() {
		return s.backupCrypto, nil
	}
	if strings.TrimSpace(secretKey) == "" {
		return nil, ErrBackupRestoreSecretRequired
	}
	cryptoSvc, err := backupcrypto.NewFromKeyMaterial(secretKey)
	if err != nil {
		return nil, ErrBackupRestoreKeyInvalid
	}
	if cryptoSvc.KeyID() != run.ArchiveKeyID {
		return nil, ErrBackupRestoreKeyInvalid
	}
	return cryptoSvc, nil
}

func (s *Service) storeUploadedArchive(runID string, reader io.Reader) (string, int64, error) {
	if err := os.MkdirAll(s.backupDir(), 0750); err != nil {
		return "", 0, fmt.Errorf("creating backup directory: %w", err)
	}
	runDir := filepath.Join(s.backupDir(), safeArchiveName.ReplaceAllString(runID, "-"))
	if err := os.MkdirAll(runDir, 0750); err != nil {
		return "", 0, fmt.Errorf("creating backup upload directory: %w", err)
	}
	archivePath := filepath.Join(runDir, "mcharbor.tar")
	file, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return "", 0, fmt.Errorf("creating uploaded backup archive: %w", err)
	}
	size, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		return archivePath, size, fmt.Errorf("saving uploaded backup archive: %w", copyErr)
	}
	if closeErr != nil {
		return archivePath, size, fmt.Errorf("closing uploaded backup archive: %w", closeErr)
	}
	if size == 0 {
		return archivePath, 0, fmt.Errorf("uploaded backup archive is empty")
	}
	return archivePath, size, nil
}

func (s *Service) deleteRunArchive(ctx context.Context, runID, archivePath string) error {
	if strings.TrimSpace(archivePath) != "" {
		if err := os.RemoveAll(filepath.Dir(archivePath)); err != nil {
			return fmt.Errorf("removing uploaded backup archive: %w", err)
		}
	}
	if strings.TrimSpace(runID) != "" {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM container_backup_runs WHERE id = ?", runID); err != nil {
			return fmt.Errorf("deleting uploaded backup run: %w", err)
		}
	}
	return nil
}

type backupArchiveManifest struct {
	Plan BackupPlan `json:"plan"`
}

func restoreMountTargets(reader io.Reader) map[string]string {
	targets := map[string]string{}
	var manifest backupArchiveManifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return targets
	}
	for _, mountPath := range manifest.Plan.SelectedMounts {
		mountPath = strings.TrimSpace(mountPath)
		if mountPath == "" {
			continue
		}
		targets[backupMountEntryName(mountPath)] = mountPath
	}
	return targets
}

func restoreArchiveOptions(reader io.Reader) ([]BackupOption, error) {
	tr := tar.NewReader(reader)
	mountTargets := map[string]string{}
	items := []BackupOption{}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading backup restore options: %w", err)
		}
		if header == nil || header.Typeflag != tar.TypeReg {
			continue
		}

		switch {
		case header.Name == "manifest.json":
			mountTargets = restoreMountTargets(tr)
		case header.Name == "image/image.tar":
			items = append(items, BackupOption{
				Key:         "image",
				Type:        "image",
				Label:       "Container image",
				Description: "Load the image archive saved in this backup.",
				Default:     true,
			})
		case header.Name == "container/filesystem.tar":
			items = append(items, BackupOption{
				Key:         "filesystem",
				Type:        "filesystem",
				Label:       "Container filesystem",
				Description: "Restore the writable container filesystem layer.",
				Default:     true,
			})
		case strings.HasPrefix(header.Name, "mounts/") && strings.HasSuffix(header.Name, ".tar"):
			target := mountTargets[header.Name]
			if target == "" {
				target = strings.TrimSuffix(strings.TrimPrefix(header.Name, "mounts/"), ".tar")
			}
			items = append(items, BackupOption{
				Key:         "mount:" + target,
				Type:        "mount",
				Label:       target,
				Description: "Restore mounted data saved for this container path.",
				Default:     true,
			})
		}
	}
	return items, nil
}

func restoreSelection(items []string) func(string) bool {
	selected := map[string]struct{}{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			selected[item] = struct{}{}
		}
	}
	if len(selected) == 0 {
		return func(string) bool { return true }
	}
	return func(item string) bool {
		_, ok := selected[item]
		return ok
	}
}

func (s *Service) restoreProgressReader(ctx context.Context, runID, stage, label string, reader io.Reader, total int64) io.Reader {
	if strings.TrimSpace(runID) == "" {
		return reader
	}
	return &backupRestoreProgressReader{
		reader: reader,
		onProgress: func(read int64) {
			s.updateRunProgress(ctx, runID, stage, backupRestoreProgressMessage(label, read, total))
		},
	}
}

type backupRestoreProgressReader struct {
	reader     io.Reader
	bytes      int64
	lastUpdate time.Time
	onProgress func(read int64)
}

func (r *backupRestoreProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.bytes += int64(n)
		if time.Since(r.lastUpdate) >= 5*time.Second {
			r.flush()
		}
	}
	if errors.Is(err, io.EOF) {
		r.flush()
	}
	return n, err
}

func (r *backupRestoreProgressReader) flush() {
	if r.onProgress == nil || r.bytes == 0 {
		return
	}
	r.lastUpdate = time.Now()
	r.onProgress(r.bytes)
}

func backupRestoreProgressMessage(label string, read, total int64) string {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "Restoring data"
	}
	if total <= 0 {
		return fmt.Sprintf("%s (%s).", label, formatBackupUploadBytes(read))
	}
	return fmt.Sprintf("%s (%s of %s).", label, formatBackupUploadBytes(read), formatBackupUploadBytes(total))
}

func restoreImageArchive(ctx context.Context, cli *client.Client, reader io.Reader) error {
	resp, err := cli.ImageLoad(ctx, reader, client.ImageLoadWithQuiet(true))
	if err != nil {
		return fmt.Errorf("restoring backup image archive: %w", err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("reading backup image restore response: %w", err)
	}
	return nil
}

func (s *Service) downloadFileName(ctx context.Context, run *BackupRun) (string, error) {
	containerName := s.downloadContainerName(ctx, run)
	environmentName, err := s.downloadEnvironmentName(ctx, run.EnvironmentID)
	if err != nil {
		return "", err
	}
	timestamp := backupDownloadTimestamp(run)

	return fmt.Sprintf(
		"%s-%s-%s.mcharbor.tar",
		slugFilePart(containerName, "container"),
		slugFilePart(environmentName, "environment"),
		timestamp,
	), nil
}

func (s *Service) downloadContainerName(ctx context.Context, run *BackupRun) string {
	if run.PlanID != "" {
		var name string
		err := s.db.QueryRowContext(ctx, "SELECT container_name FROM container_backup_plans WHERE id = ?", run.PlanID).Scan(&name)
		if err == nil && strings.TrimSpace(name) != "" {
			return name
		}
	}

	name, err := s.containerName(ctx, run.EnvironmentID, run.ContainerID)
	if err == nil && strings.TrimSpace(name) != "" {
		return name
	}
	return shortID(run.ContainerID)
}

func (s *Service) downloadEnvironmentName(ctx context.Context, envID string) (string, error) {
	var name string
	err := s.db.QueryRowContext(ctx, "SELECT name FROM environments WHERE id = ? LIMIT 1", envID).Scan(&name)
	if err == sql.ErrNoRows {
		return envID, nil
	}
	if err != nil {
		return "", fmt.Errorf("reading backup environment name: %w", err)
	}
	if strings.TrimSpace(name) == "" {
		return envID, nil
	}
	return name, nil
}

func backupDownloadTimestamp(run *BackupRun) string {
	value := strings.TrimSpace(run.CompletedAt)
	if value == "" {
		value = strings.TrimSpace(run.StartedAt)
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "unknown-time"
	}
	return parsed.UTC().Format("2006-01-02-15-04-05")
}

func slugFilePart(value, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		value = fallback
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return fallback
	}
	return slug
}

func (s *Service) validatedArchivePath(path string) (string, error) {
	backupRoot, err := filepath.Abs(s.backupDir())
	if err != nil {
		return "", fmt.Errorf("resolving backup directory: %w", err)
	}
	archivePath, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("resolving backup archive: %w", err)
	}

	resolvedRoot, err := filepath.EvalSymlinks(backupRoot)
	if err != nil {
		return "", fmt.Errorf("resolving backup directory links: %w", err)
	}
	resolvedArchive, err := filepath.EvalSymlinks(archivePath)
	if err != nil {
		return "", fmt.Errorf("resolving backup archive links: %w", err)
	}

	rel, err := filepath.Rel(resolvedRoot, resolvedArchive)
	if err != nil {
		return "", fmt.Errorf("checking backup archive path: %w", err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", ErrBackupRunNotDownloadable
	}
	if filepath.Base(resolvedArchive) != "mcharbor.tar" {
		return "", ErrBackupRunNotDownloadable
	}

	return resolvedArchive, nil
}

func (s *Service) writeArchive(ctx context.Context, plan *BackupPlan, runID string) (archiveResult, error) {
	if s.backupCrypto == nil {
		return archiveResult{}, ErrBackupEncryptionKeyNotConfigured
	}
	s.updateRunProgress(ctx, runID, "connecting", "")
	cli, err := s.client(plan.EnvironmentID)
	if err != nil {
		return archiveResult{}, err
	}
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	s.updateRunProgress(opCtx, runID, "inspecting", "")
	info, resolvedID, err := s.resolveContainerForBackup(ctx, opCtx, cli, plan, s.logger)
	if err != nil {
		return archiveResult{}, fmt.Errorf("inspecting container for backup: %w", err)
	}
	if resolvedID != plan.ContainerID {
		s.logger.Info(
			"container backup: resolved stale container id to current id (Compose restart likely)",
			"run", runID, "plan_id", plan.ID,
			"old_id", plan.ContainerID, "new_id", resolvedID,
		)
		plan.ContainerID = resolvedID
	}

	// Pause the container for the duration of the archive so the writable
	// layer, mount copies, and log read are point-in-time consistent.
	// Pause is best-effort: some runtimes (Windows containers, certain
	// custom runtimes) don't support it. If pause fails we log and
	// proceed without it.
	paused := pauseContainerForBackup(opCtx, cli, plan.ContainerID, s.logger)
	defer func() {
		// Use a fresh background context for unpause so we still
		// recover the container if the opCtx was cancelled.
		unpauseCtx, unpauseCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer unpauseCancel()
		unpauseContainerAfterBackup(unpauseCtx, cli, plan.ContainerID, paused, s.logger)
	}()

	s.updateRunProgress(opCtx, runID, "preparing", "")
	if err := os.MkdirAll(s.backupDir(), 0750); err != nil {
		return archiveResult{}, fmt.Errorf("creating backup directory: %w", err)
	}
	runDir := filepath.Join(s.backupDir(), safeArchiveName.ReplaceAllString(runID, "-"))
	if err := os.MkdirAll(runDir, 0750); err != nil {
		return archiveResult{}, fmt.Errorf("creating backup run directory: %w", err)
	}
	archivePath := filepath.Join(runDir, "mcharbor.tar")
	file, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return archiveResult{}, fmt.Errorf("creating backup archive: %w", err)
	}
	defer file.Close()

	encryptedWriter, metadata, err := s.backupCrypto.EncryptWriter(file)
	if err != nil {
		return archiveResult{path: archivePath}, err
	}

	s.updateRunProgress(opCtx, runID, "writing", "")
	tw := tar.NewWriter(encryptedWriter)
	writeErr := s.writeArchiveEntries(opCtx, cli, tw, plan, info, runID)
	s.updateRunProgress(opCtx, runID, "finalizing", "")
	closeErr := tw.Close()
	if writeErr != nil {
		_ = encryptedWriter.Close()
		return archiveResult{path: archivePath, encryption: metadata.Algorithm, keyID: metadata.KeyID}, writeErr
	}
	if closeErr != nil {
		_ = encryptedWriter.Close()
		return archiveResult{path: archivePath, encryption: metadata.Algorithm, keyID: metadata.KeyID}, fmt.Errorf("closing backup archive: %w", closeErr)
	}
	if err := encryptedWriter.Close(); err != nil {
		return archiveResult{path: archivePath, encryption: metadata.Algorithm, keyID: metadata.KeyID}, fmt.Errorf("closing encrypted backup archive: %w", err)
	}
	stat, err := file.Stat()
	if err != nil {
		return archiveResult{path: archivePath, encryption: metadata.Algorithm, keyID: metadata.KeyID}, fmt.Errorf("stating backup archive: %w", err)
	}
	return archiveResult{path: archivePath, size: stat.Size(), encryption: metadata.Algorithm, keyID: metadata.KeyID}, nil
}

// pauseContainerForBackup pauses the container for snapshot consistency
// during the backup. Returns true if the container is paused (and therefore
// needs to be unpaused), false if pause was skipped or failed.
func pauseContainerForBackup(ctx context.Context, cli *client.Client, containerID string, logger *slog.Logger) bool {
	if err := cli.ContainerPause(ctx, containerID); err != nil {
		if logger != nil {
			logger.Warn("container backup: pause failed, proceeding without snapshot consistency",
				"container", containerID, "error", err)
		}
		return false
	}
	if logger != nil {
		logger.Info("container backup: container paused for snapshot consistency", "container", containerID)
	}
	return true
}

// unpauseContainerAfterBackup releases the container. If pause was skipped
// (returns false) or the container is already running, this is a no-op.
func unpauseContainerAfterBackup(ctx context.Context, cli *client.Client, containerID string, wasPaused bool, logger *slog.Logger) {
	if !wasPaused {
		return
	}
	if err := cli.ContainerUnpause(ctx, containerID); err != nil {
		if logger != nil {
			logger.Error("container backup: unpause failed, container may remain paused",
				"container", containerID, "error", err)
		}
		return
	}
	if logger != nil {
		logger.Info("container backup: container unpaused", "container", containerID)
	}
}

func (s *Service) writeArchiveEntries(ctx context.Context, cli *client.Client, tw *tar.Writer, plan *BackupPlan, info types.ContainerJSON, runID string) error {
	s.updateRunProgress(ctx, runID, "manifest", "")
	manifest, err := json.MarshalIndent(map[string]any{
		"format":      "mcharbor.container.backup.v1",
		"createdAt":   time.Now().UTC().Format(time.RFC3339),
		"plan":        plan,
		"destination": "local",
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("creating backup manifest: %w", err)
	}
	if err := writeBytes(tw, "manifest.json", manifest); err != nil {
		return err
	}
	if plan.IncludeConfig {
		s.updateRunProgress(ctx, runID, "config", "")
		inspect, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding container inspect: %w", err)
		}
		if err := writeBytes(tw, "container/inspect.json", inspect); err != nil {
			return err
		}
	}
	if plan.IncludeLogs {
		s.updateRunProgress(ctx, runID, "logs", "")
		tail := plan.LogTailLines
		tailArg := "all"
		if tail > 0 {
			tailArg = strconv.Itoa(tail)
		}
		reader, err := cli.ContainerLogs(ctx, plan.ContainerID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Timestamps: true, Tail: tailArg})
		if err != nil {
			return fmt.Errorf("reading container logs: %w", err)
		}
		if err := writeStream(ctx, tw, "container/logs.txt", reader, s.backupTempDir(), func() {
			s.updateRunProgress(ctx, runID, "logs", "")
		}); err != nil {
			return err
		}
	}
	if plan.IncludeFilesystem {
		s.updateRunProgress(ctx, runID, "filesystem", "")
		reader, err := cli.ContainerExport(ctx, plan.ContainerID)
		if err != nil {
			return fmt.Errorf("exporting container filesystem: %w", err)
		}
		if err := writeStream(ctx, tw, "container/filesystem.tar", reader, s.backupTempDir(), func() {
			s.updateRunProgress(ctx, runID, "filesystem", "")
		}); err != nil {
			return err
		}
	}
	if plan.IncludeImage {
		s.updateRunProgress(ctx, runID, "image", "")
		inspect, err := cli.ContainerInspect(ctx, plan.ContainerID)
		if err != nil {
			return fmt.Errorf("inspecting container image: %w", err)
		}
		imageRef := ""
		if inspect.Config != nil {
			imageRef = inspect.Config.Image
		}
		if imageRef != "" {
			reader, err := cli.ImageSave(ctx, []string{imageRef})
			if err != nil {
				return fmt.Errorf("saving container image: %w", err)
			}
			if err := writeStream(ctx, tw, "image/image.tar", reader, s.backupTempDir(), func() {
				s.updateRunProgress(ctx, runID, "image", "")
			}); err != nil {
				return err
			}
		}
	}
	allowedMounts := map[string]bool{}
	for _, mount := range info.Mounts {
		if mount.Destination != "" && (mount.Type == "volume" || mount.Type == "bind") {
			allowedMounts[mount.Destination] = true
		}
	}
	for _, mountPath := range plan.SelectedMounts {
		cleanMount := strings.TrimSpace(mountPath)
		if cleanMount == "" || strings.Contains(cleanMount, "..") || !strings.HasPrefix(cleanMount, "/") || !allowedMounts[cleanMount] {
			return fmt.Errorf("invalid backup mount path")
		}
		s.updateRunProgress(ctx, runID, "mounts", "")
		reader, _, err := cli.CopyFromContainer(ctx, plan.ContainerID, cleanMount)
		if err != nil {
			return fmt.Errorf("copying mounted data %s: %w", cleanMount, err)
		}
		name := backupMountEntryName(cleanMount)
		if err := writeStream(ctx, tw, name, reader, s.backupTempDir(), func() {
			s.updateRunProgress(ctx, runID, "mounts", "")
		}); err != nil {
			return err
		}
	}
	return nil
}

func backupMountEntryName(mountPath string) string {
	return "mounts/" + safeArchiveName.ReplaceAllString(strings.Trim(mountPath, "/"), "-") + ".tar"
}

func agentBackupProgressMessage(progress coreagent.BackupPayload) string {
	stage := strings.TrimSpace(progress.Stage)
	switch stage {
	case "agent_backup", "manifest", "config", "logs", "filesystem", "image", "mounts", "finalizing":
		if progress.Bytes > 0 {
			return fmt.Sprintf("Agent is creating the backup archive (%s written).", formatBackupUploadBytes(progress.Bytes))
		}
		return "Agent is creating the backup archive locally."
	case "uploading":
		name := strings.TrimSpace(progress.StorageLocationID)
		if progress.Size > 0 {
			return fmt.Sprintf("Agent is uploading backup to %s (%s of %s).", name, formatBackupUploadBytes(progress.Bytes), formatBackupUploadBytes(progress.Size))
		}
		if progress.Bytes > 0 {
			return fmt.Sprintf("Agent is uploading backup to %s (%s sent).", name, formatBackupUploadBytes(progress.Bytes))
		}
		return "Agent is uploading backup to " + name + "."
	case "restore_download":
		if progress.Size > 0 {
			return fmt.Sprintf("Agent is downloading backup archive (%s of %s).", formatBackupUploadBytes(progress.Bytes), formatBackupUploadBytes(progress.Size))
		}
		return fmt.Sprintf("Agent is downloading backup archive (%s received).", formatBackupUploadBytes(progress.Bytes))
	case "restore_apply":
		return "Agent is restoring backup entries locally."
	default:
		return strings.TrimSpace(progress.Stage)
	}
}

func writeBytes(tw *tar.Writer, name string, data []byte) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data))}); err != nil {
		return fmt.Errorf("writing backup header %s: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("writing backup entry %s: %w", name, err)
	}
	return nil
}

// backupMemorySpoolLimit is the threshold below which a backup entry is
// buffered in memory rather than spooled to disk. Logs and mount copies
// usually fit under this; container filesystem exports do not.
const backupMemorySpoolLimit = 64 * 1024 * 1024 // 64 MiB

func writeStream(ctx context.Context, tw *tar.Writer, name string, reader io.ReadCloser, tempDir string, onProgress func()) error {
	defer reader.Close()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = reader.Close()
		case <-done:
		}
	}()
	defer close(done)

	// Fast path for entries that fit in memory: write the tar header and
	// the entry data in one pass with no disk I/O. We cap at 64 MiB so a
	// pathological export can't OOM the server.
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, reader); err != nil {
		return fmt.Errorf("spooling backup entry %s: %w", name, err)
	}
	size := int64(buf.Len())
	if size <= backupMemorySpoolLimit {
		if onProgress != nil {
			onProgress()
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: size}); err != nil {
			return fmt.Errorf("writing backup header %s: %w", name, err)
		}
		if _, err := tw.Write(buf.Bytes()); err != nil {
			return fmt.Errorf("writing backup entry %s: %w", name, err)
		}
		return nil
	}

	// Large entry: spool to disk to know the byte count, then re-read into
	// the encrypted tar. This is the previous behavior; kept because the
	// archive/tar package requires the entry size in the header before the
	// data is written, and we cannot determine the size without spooling.
	if onProgress != nil {
		onProgress()
	}
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		return fmt.Errorf("creating backup temp directory: %w", err)
	}
	tmp, err := os.CreateTemp(tempDir, "mcharbor-backup-entry-*")
	if err != nil {
		return fmt.Errorf("creating backup temp entry: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return fmt.Errorf("spooling backup entry %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing backup temp entry %s: %w", name, err)
	}
	in, err := os.Open(tmpName)
	if err != nil {
		return fmt.Errorf("opening backup temp entry %s: %w", name, err)
	}
	defer in.Close()
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: size}); err != nil {
		return fmt.Errorf("writing backup header %s: %w", name, err)
	}
	if _, err := io.Copy(tw, in); err != nil {
		return fmt.Errorf("writing backup entry %s: %w", name, err)
	}
	return nil
}

type backupProgressWriter struct {
	writer     io.Writer
	bytes      int64
	lastUpdate time.Time
	onProgress func()
}

func (w *backupProgressWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 {
		w.bytes += int64(n)
		if time.Since(w.lastUpdate) >= 5*time.Second {
			w.flush()
		}
	}
	return n, err
}

func (w *backupProgressWriter) flush() {
	if w.onProgress == nil || w.bytes == 0 {
		return
	}
	w.lastUpdate = time.Now()
	w.onProgress()
}

func (s *Service) backupDir() string {
	return filepath.Join(s.dataDir, "backups", "containers")
}

func (s *Service) backupTempDir() string {
	return filepath.Join(s.dataDir, "backups", "tmp")
}

func (s *Service) hydratePlanStorageLocations(ctx context.Context, plan *BackupPlan) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT storage_location_id
		FROM container_backup_plan_storage_locations
		WHERE plan_id = ?
		ORDER BY created_at ASC, storage_location_id ASC
		LIMIT 1000`, plan.ID)
	if err != nil {
		return fmt.Errorf("listing backup plan storage locations: %w", err)
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scanning backup plan storage location: %w", err)
		}
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating backup plan storage locations: %w", err)
	}

	if len(ids) == 0 && strings.TrimSpace(plan.StorageLocationID) != "" {
		ids = []string{strings.TrimSpace(plan.StorageLocationID)}
	}
	requiredIDs, err := s.requiredBackupStorageLocationIDs(ctx, "", ids)
	if err != nil {
		return err
	}
	plan.StorageLocationIDs = requiredIDs
	plan.StorageLocationID = firstStorageLocationID(plan.StorageLocationIDs)
	return nil
}

func (s *Service) setPlanStorageLocations(ctx context.Context, planID string, ids []string) error {
	ids, err := s.requiredBackupStorageLocationIDs(ctx, "", ids)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting backup destination update: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) && s.logger != nil {
			s.logger.Warn("backup plan destination rollback failed", "plan", planID, "error", err)
		}
	}()

	if _, err := tx.ExecContext(ctx, "DELETE FROM container_backup_plan_storage_locations WHERE plan_id = ?", planID); err != nil {
		return fmt.Errorf("clearing backup plan destinations: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range backupStorageLocationIDs("", ids) {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO container_backup_plan_storage_locations (plan_id, storage_location_id, created_at)
			VALUES (?, ?, ?)`, planID, id, now); err != nil {
			return fmt.Errorf("adding backup plan destination: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing backup destination update: %w", err)
	}
	committed = true
	return nil
}

func (s *Service) requiredBackupStorageLocationIDs(ctx context.Context, legacy string, ids []string) ([]string, error) {
	out := backupStorageLocationIDs(legacy, ids)
	localID, err := s.defaultLocalStorageLocationID(ctx)
	if err != nil {
		return nil, err
	}
	if localID == "" {
		return out, nil
	}
	return backupStorageLocationIDs(localID, out), nil
}

func (s *Service) defaultLocalStorageLocationID(ctx context.Context) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
		SELECT id
		FROM storage_locations
		WHERE location_type = 'local'
		  AND enabled = 1
		ORDER BY
		  CASE
		    WHEN id = ? THEN 0
		    WHEN COALESCE(base_path, '') = '/mnt/backup' THEN 1
		    ELSE 2
		  END,
		  created_at ASC,
		  id ASC
		LIMIT 1`, defaultLocalStorageID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading default local backup storage location: %w", err)
	}
	return id, nil
}

func scanPlan(scanner interface{ Scan(dest ...any) error }) (BackupPlan, error) {
	var plan BackupPlan
	var selected string
	if err := scanner.Scan(
		&plan.ID, &plan.Name, &plan.EnvironmentID, &plan.ContainerID, &plan.ContainerName,
		&plan.StorageLocationID, &plan.IncludeConfig, &plan.IncludeLogs, &plan.IncludeFilesystem,
		&plan.IncludeImage, &selected, &plan.LogTailLines, &plan.Cron, &plan.Enabled, &plan.RetentionCount,
		&plan.RetentionDays, &plan.LastRunAt, &plan.NextRunAt, &plan.CreatedAt, &plan.UpdatedAt,
	); err != nil {
		return BackupPlan{}, err
	}
	if selected != "" {
		if err := json.Unmarshal([]byte(selected), &plan.SelectedMounts); err != nil {
			return BackupPlan{}, fmt.Errorf("decoding selected mounts: %w", err)
		}
	}
	if plan.SelectedMounts == nil {
		plan.SelectedMounts = []string{}
	}
	plan.StorageLocationIDs = backupStorageLocationIDs(plan.StorageLocationID, nil)
	return plan, nil
}

func scanRun(scanner interface{ Scan(dest ...any) error }) (BackupRun, error) {
	var run BackupRun
	if err := scanner.Scan(
		&run.ID, &run.PlanID, &run.Operation, &run.SourceRunID, &run.EnvironmentID, &run.ContainerID, &run.Status,
		&run.ArchivePath, &run.ArchiveSize, &run.ArchiveEncryption, &run.ArchiveKeyID,
		&run.Error, &run.ProgressStage, &run.ProgressMessage, &run.ProgressUpdatedAt, &run.StartedAt,
		&run.CompletedAt, &run.DurationMS, &run.CreatedAt, &run.UpdatedAt,
	); err != nil {
		return BackupRun{}, err
	}
	if run.Operation == "" {
		run.Operation = "backup"
	}
	return run, nil
}

func nullString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func backupStorageLocationIDs(legacy string, ids []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	add(legacy)
	for _, id := range ids {
		add(id)
	}
	return out
}

func firstStorageLocationID(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
