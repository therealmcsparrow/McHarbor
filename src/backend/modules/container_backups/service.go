// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"archive/tar"
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
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/db"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
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

// Service handles container backup plans and archive execution.
type Service struct {
	db           *sql.DB
	pool         *coredocker.ClientPool
	dataDir      string
	backupCrypto *backupcrypto.Service
	logger       *slog.Logger
}

var safeArchiveName = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

// NewService creates a backup service.
func NewService(database *sql.DB, pool *coredocker.ClientPool, dataDir string, backupCrypto *backupcrypto.Service, logger *slog.Logger) *Service {
	return &Service{db: database, pool: pool, dataDir: dataDir, backupCrypto: backupCrypto, logger: logger}
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
	storageIDs := backupStorageLocationIDs(input.StorageLocationID, input.StorageLocationIDs)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO container_backup_plans (
			id, name, environment_id, container_id, container_name, storage_location_id,
			include_config, include_logs, include_filesystem, include_image, selected_mounts,
			cron, enabled, retention_count, retention_days, next_run_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, envID, input.ContainerID, containerName, nullString(firstStorageLocationID(storageIDs)),
		input.IncludeConfig, input.IncludeLogs, input.IncludeFilesystem, input.IncludeImage, string(selected),
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
	if input.StorageLocationID != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET storage_location_id = ?, updated_at = ? WHERE id = ?", nullString(*input.StorageLocationID), now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan storage location: %w", err)
		}
		if input.StorageLocationIDs == nil {
			if err := s.setPlanStorageLocations(ctx, id, backupStorageLocationIDs(*input.StorageLocationID, nil)); err != nil {
				return nil, err
			}
		}
	}
	if input.StorageLocationIDs != nil {
		ids := backupStorageLocationIDs("", *input.StorageLocationIDs)
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(plan_id, ''), environment_id, container_id, status, COALESCE(archive_path, ''),
		       archive_size, COALESCE(archive_encryption, ''), COALESCE(archive_key_id, ''), COALESCE(error, ''),
		       started_at, COALESCE(completed_at, ''), duration_ms, created_at, updated_at
		FROM container_backup_runs
		WHERE environment_id = ? AND container_id = ?
		ORDER BY started_at DESC
		LIMIT 100`, envID, containerID)
	if err != nil {
		return nil, fmt.Errorf("listing backup runs: %w", err)
	}
	defer rows.Close()

	runs := []BackupRun{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		s.annotateRunKeyRequirement(&run)
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// RunPlan executes a saved backup plan synchronously.
func (s *Service) RunPlan(ctx context.Context, planID string) (*BackupRun, error) {
	plan, err := s.Plan(ctx, planID)
	if err != nil || plan == nil {
		return nil, err
	}
	run, err := s.createRun(ctx, plan.ID, plan.EnvironmentID, plan.ContainerID)
	if err != nil {
		return nil, err
	}
	archive, execErr := s.writeArchive(ctx, plan, run.ID)
	return s.finishRun(ctx, run.ID, archive, execErr)
}

// RunAdhoc executes an unsaved backup request.
func (s *Service) RunAdhoc(ctx context.Context, envID, containerID string, input RunBackupInput) (*BackupRun, error) {
	name := input.Name
	if strings.TrimSpace(name) == "" {
		name = "Manual backup"
	}
	containerName, err := s.containerName(ctx, envID, containerID)
	if err != nil {
		return nil, err
	}
	plan := &BackupPlan{
		ID:                 "",
		Name:               name,
		EnvironmentID:      envID,
		ContainerID:        containerID,
		ContainerName:      containerName,
		StorageLocationID:  input.StorageLocationID,
		StorageLocationIDs: backupStorageLocationIDs(input.StorageLocationID, input.StorageLocationIDs),
		IncludeConfig:      true,
		IncludeLogs:        input.IncludeLogs,
		IncludeFilesystem:  input.IncludeFilesystem,
		IncludeImage:       input.IncludeImage,
		SelectedMounts:     input.SelectedMounts,
	}
	run, err := s.createRun(ctx, "", envID, containerID)
	if err != nil {
		return nil, err
	}
	archive, execErr := s.writeArchive(ctx, plan, run.ID)
	return s.finishRun(ctx, run.ID, archive, execErr)
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

func (s *Service) createRun(ctx context.Context, planID, envID, containerID string) (*BackupRun, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO container_backup_runs (id, plan_id, environment_id, container_id, status, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'running', ?, ?, ?)`, id, nullString(planID), envID, containerID, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("creating backup run: %w", err)
	}
	return &BackupRun{ID: id, PlanID: planID, EnvironmentID: envID, ContainerID: containerID, Status: "running", StartedAt: now, CreatedAt: now, UpdatedAt: now}, nil
}

type archiveResult struct {
	path       string
	size       int64
	encryption string
	keyID      string
}

func (s *Service) finishRun(ctx context.Context, runID string, archive archiveResult, execErr error) (*BackupRun, error) {
	now := time.Now().UTC()
	status := "success"
	errorText := ""
	if execErr != nil {
		status = "failure"
		errorText = "backup failed; check McHarbor logs"
	}
	var started string
	if err := s.db.QueryRowContext(ctx, "SELECT started_at FROM container_backup_runs WHERE id = ?", runID).Scan(&started); err != nil {
		return nil, fmt.Errorf("reading backup run start time: %w", err)
	}
	startedAt, _ := time.Parse(time.RFC3339, started)
	duration := now.Sub(startedAt).Milliseconds()
	_, err := s.db.ExecContext(ctx, `
		UPDATE container_backup_runs
		SET status = ?, archive_path = ?, archive_size = ?, archive_encryption = ?, archive_key_id = ?,
		    error = ?, completed_at = ?, duration_ms = ?, updated_at = ?
		WHERE id = ?`,
		status, nullString(archive.path), archive.size, archive.encryption, archive.keyID,
		nullString(errorText), now.Format(time.RFC3339), duration, now.Format(time.RFC3339), runID)
	if err != nil {
		return nil, fmt.Errorf("finishing backup run: %w", err)
	}
	if execErr != nil {
		return nil, execErr
	}
	run, err := s.runByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if err := s.pruneBackupPlanRuns(ctx, run); err != nil && s.logger != nil {
		s.logger.Warn("container backup retention pruning failed", "plan", run.PlanID, "env", run.EnvironmentID, "container", run.ContainerID, "error", err)
	}
	return run, nil
}

func (s *Service) runByID(ctx context.Context, id string) (*BackupRun, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(plan_id, ''), environment_id, container_id, status, COALESCE(archive_path, ''),
		       archive_size, COALESCE(archive_encryption, ''), COALESCE(archive_key_id, ''), COALESCE(error, ''),
		       started_at, COALESCE(completed_at, ''), duration_ms, created_at, updated_at
		FROM container_backup_runs WHERE id = ?`, id)
	run, err := scanRun(row)
	if err != nil {
		return nil, err
	}
	s.annotateRunKeyRequirement(&run)
	return &run, nil
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

// Restore applies restorable entries from a completed encrypted backup run to its original container.
func (s *Service) Restore(ctx context.Context, runID string, input RestoreBackupInput) (*RestoreBackupResult, error) {
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
		return nil, err
	}

	cryptoSvc, err := s.restoreCrypto(run, input.SecretKey)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(archivePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening backup archive for restore: %w", err)
	}
	defer file.Close()

	decrypted, _, err := cryptoSvc.DecryptReader(file)
	if err != nil {
		return nil, ErrBackupRestoreKeyInvalid
	}
	defer decrypted.Close()

	cli, err := s.client(run.EnvironmentID)
	if err != nil {
		return nil, err
	}
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	if _, err := cli.ContainerInspect(opCtx, run.ContainerID); err != nil {
		return nil, fmt.Errorf("inspecting container before restore: %w", err)
	}

	tr := tar.NewReader(decrypted)
	mountTargets := map[string]string{}
	restored := []string{}
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
			if err := restoreImageArchive(opCtx, cli, tr); err != nil {
				return nil, err
			}
			restored = append(restored, "image")
		case header.Name == "container/filesystem.tar":
			if err := cli.CopyToContainer(opCtx, run.ContainerID, "/", tr, container.CopyToContainerOptions{AllowOverwriteDirWithFile: false}); err != nil {
				return nil, fmt.Errorf("restoring container filesystem: %w", err)
			}
			restored = append(restored, "filesystem")
		case strings.HasPrefix(header.Name, "mounts/") && strings.HasSuffix(header.Name, ".tar"):
			target := mountTargets[header.Name]
			if target == "" {
				return nil, fmt.Errorf("backup mount target is missing")
			}
			restoreTarget := filepath.Dir(target)
			if restoreTarget == "." {
				restoreTarget = "/"
			}
			if err := cli.CopyToContainer(opCtx, run.ContainerID, restoreTarget, tr, container.CopyToContainerOptions{AllowOverwriteDirWithFile: false}); err != nil {
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
	cli, err := s.client(plan.EnvironmentID)
	if err != nil {
		return archiveResult{}, err
	}
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	info, err := cli.ContainerInspect(opCtx, plan.ContainerID)
	if err != nil {
		return archiveResult{}, fmt.Errorf("inspecting container for backup: %w", err)
	}

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

	tw := tar.NewWriter(encryptedWriter)
	writeErr := s.writeArchiveEntries(opCtx, cli, tw, plan, info)
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

func (s *Service) writeArchiveEntries(ctx context.Context, cli *client.Client, tw *tar.Writer, plan *BackupPlan, info types.ContainerJSON) error {
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
		inspect, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding container inspect: %w", err)
		}
		if err := writeBytes(tw, "container/inspect.json", inspect); err != nil {
			return err
		}
	}
	if plan.IncludeLogs {
		reader, err := cli.ContainerLogs(ctx, plan.ContainerID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Timestamps: true, Tail: "all"})
		if err != nil {
			return fmt.Errorf("reading container logs: %w", err)
		}
		if err := writeStream(tw, "container/logs.txt", reader, s.backupTempDir()); err != nil {
			return err
		}
	}
	if plan.IncludeFilesystem {
		reader, err := cli.ContainerExport(ctx, plan.ContainerID)
		if err != nil {
			return fmt.Errorf("exporting container filesystem: %w", err)
		}
		if err := writeStream(tw, "container/filesystem.tar", reader, s.backupTempDir()); err != nil {
			return err
		}
	}
	if plan.IncludeImage {
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
			if err := writeStream(tw, "image/image.tar", reader, s.backupTempDir()); err != nil {
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
		reader, _, err := cli.CopyFromContainer(ctx, plan.ContainerID, cleanMount)
		if err != nil {
			return fmt.Errorf("copying mounted data %s: %w", cleanMount, err)
		}
		name := backupMountEntryName(cleanMount)
		if err := writeStream(tw, name, reader, s.backupTempDir()); err != nil {
			return err
		}
	}
	return nil
}

func backupMountEntryName(mountPath string) string {
	return "mounts/" + safeArchiveName.ReplaceAllString(strings.Trim(mountPath, "/"), "-") + ".tar"
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

func writeStream(tw *tar.Writer, name string, reader io.ReadCloser, tempDir string) error {
	defer reader.Close()
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		return fmt.Errorf("creating backup temp directory: %w", err)
	}
	tmp, err := os.CreateTemp(tempDir, "mcharbor-backup-entry-*")
	if err != nil {
		return fmt.Errorf("creating backup temp entry: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	size, copyErr := io.Copy(tmp, reader)
	closeErr := tmp.Close()
	if copyErr != nil {
		return fmt.Errorf("spooling backup entry %s: %w", name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("closing backup temp entry %s: %w", name, closeErr)
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
	plan.StorageLocationIDs = backupStorageLocationIDs("", ids)
	plan.StorageLocationID = firstStorageLocationID(plan.StorageLocationIDs)
	return nil
}

func (s *Service) setPlanStorageLocations(ctx context.Context, planID string, ids []string) error {
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

func scanPlan(scanner interface{ Scan(dest ...any) error }) (BackupPlan, error) {
	var plan BackupPlan
	var selected string
	if err := scanner.Scan(
		&plan.ID, &plan.Name, &plan.EnvironmentID, &plan.ContainerID, &plan.ContainerName,
		&plan.StorageLocationID, &plan.IncludeConfig, &plan.IncludeLogs, &plan.IncludeFilesystem,
		&plan.IncludeImage, &selected, &plan.Cron, &plan.Enabled, &plan.RetentionCount,
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
		&run.ID, &run.PlanID, &run.EnvironmentID, &run.ContainerID, &run.Status,
		&run.ArchivePath, &run.ArchiveSize, &run.ArchiveEncryption, &run.ArchiveKeyID,
		&run.Error, &run.StartedAt,
		&run.CompletedAt, &run.DurationMS, &run.CreatedAt, &run.UpdatedAt,
	); err != nil {
		return BackupRun{}, err
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
