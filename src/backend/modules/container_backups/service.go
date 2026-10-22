// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package container_backups snapshots container filesystems, mounts and configuration to encrypted archives, uploads them to storage destinations and restores them.
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
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/rs/xid"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/db"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/snapshotdrivers"

	"github.com/therealmcsparrow/mcharbor/core/plugin/services"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
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

// ErrBackupDestinationRequired is returned when a plan or an ad-hoc run
// is submitted with no storage destination at all. The default local
// location used to be force-added server-side; it is now a UI default
// the user may clear, so the emptiness has to be rejected explicitly.
var ErrBackupDestinationRequired = errors.New("at least one backup destination is required")

// ErrSnapshotConfigRequired is returned when a plan asks for snapshot
// consistency without naming the snapshot configuration to use.
var ErrSnapshotConfigRequired = errors.New("snapshot consistency requires a snapshot configuration")

// ErrBackupRestoreMountMissing means the named mount tar does not exist in the archive.
var ErrBackupRestoreMountMissing = errors.New("backup archive does not contain that mount tar")

// ErrBackupRestoreConfigUnsupported means the selection named only container
// configuration. The archive carries container/inspect.json, but nothing applies
// it: env, labels, ports and restart policy cannot be changed on a live
// container, so restoring config means recreating the container, which the
// restore pipeline does not do. Selecting config alone can therefore never
// succeed, and saying so beats a generic "no restorable entries".
var ErrBackupRestoreConfigUnsupported = errors.New("restoring container configuration is not supported")

// ErrBackupRestoreConfigFilesystemConflict means config and the container
// filesystem were selected together. Applying config recreates the container
// from its image, which discards whatever the filesystem restore just wrote, so
// the two must be run as separate restores.
var ErrBackupRestoreConfigFilesystemConflict = errors.New("container configuration and filesystem cannot be restored together")

// restoreConfigInspectMaxBytes caps the archived inspect payload read into
// memory. A container config is kilobytes; this only stops a corrupt or hostile
// archive from ballooning the process.
const restoreConfigInspectMaxBytes = 8 << 20

// ErrBackupRestoreConfigMissing means the archive has no container/inspect.json (includeConfig was disabled at backup time).
var ErrBackupRestoreConfigMissing = errors.New("backup archive does not contain a container config")

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
	agentBackupMinVersion        = "1.5.2"
	// Agents below this cannot filter an inner tar down to individual files,
	// so a selective restore must not be handed to them — they would restore
	// the whole mount instead. Such a restore falls back to the server-side
	// path, which is correct everywhere, just slower.
	agentSelectiveRestoreMinVersion = "2.1.0"
)

// Service handles container backup plans and archive execution.
type Service struct {
	db           *sql.DB
	pool         *coredocker.ClientPool
	dataDir      string
	backupCrypto *backupcrypto.Service
	enc          *encryption.Service
	logger       *slog.Logger

	snapshotAdapter any

	// builtinSnapshots owns the drivers McHarbor ships itself (zfs,
	// btrfs, lvm, vss). A plugin registering the same driver name
	// takes precedence; see resolveSnapshotAdapter.
	builtinSnapshots *snapshotdrivers.Service

	// pluginStorageAdapter is the loaded plugin runtime (or nil when
	// no plugins are loaded), set by SetPluginStorageAdapter after
	// bootPluginRuntime finishes wiring. Held as `any` to avoid an
	// import cycle between modules/container_backups and the plugin
	// runtime; storage_upload.go does the type assertion to
	// plugin.StorageAdapterSource. The upload dispatcher consults
	// the runtime first and falls through to the built-in switch
	// when no plugin owns the location_type.
	pluginStorageAdapter any

	// containerRecreator performs the recreate a config restore needs. The
	// containers module owns the implementation; main.go injects the bridge
	// via SetContainerRecreator. Nil until then, in which case a config
	// restore reports the operation as unsupported rather than half-applying.
	containerRecreator ContainerRecreator

	// restorePassHook replaces restoreFromRun in tests that only care about the
	// order and target of the passes a combined restore makes. Nil in production.
	restorePassHook func(ctx context.Context, progressRunID string, run *BackupRun, input RestoreBackupInput) (*RestoreBackupResult, error)

	// runStageCache tracks the last known progress stage per run so the
	// backup_log writer only emits a row on actual transitions, not
	// on every progress update. The cache is read-mostly; a stale
	// entry is harmless because the next genuine change re-syncs.
	runStageCache sync.Map
}

// SetContainerRecreator injects the container recreate collaborator used by
// config restores. Called from main.go after the containers service exists.
func (s *Service) SetContainerRecreator(r ContainerRecreator) { s.containerRecreator = r }

// SetPluginStorageAdapter injects the loaded plugin runtime into
// the service. The runtime is held as `any` to avoid the import
// cycle; the dispatcher does the type assertion to
// plugin.StorageAdapterSource. Safe to call multiple times —
// later calls overwrite the prior handle. Called from main.go
// after bootPluginRuntime.
//
// The same runtime answers for plugin snapshot drivers: their ConfigSchema is what
// names the secret fields a snapshot config must encrypt.
func (s *Service) SetPluginStorageAdapter(runtime any) {
	s.pluginStorageAdapter = runtime
	s.snapshotAdapter = runtime
}

var safeArchiveName = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

var activeBackupRuns sync.Map

// runCancelers tracks cancel functions for in-flight backup and restore runs so
// the user can cancel a long-running operation from the UI. Keys are run IDs.
var runCancelers sync.Map

// NewService creates a backup service.
func NewService(database *sql.DB, pool *coredocker.ClientPool, dataDir string, backupCrypto *backupcrypto.Service, enc *encryption.Service, logger *slog.Logger) *Service {
	return &Service{
		db:           database,
		pool:         pool,
		dataDir:      dataDir,
		backupCrypto: backupCrypto,
		enc:          enc,
		logger:       logger,
		// The builtin drivers are always available: they are core, not
		// a plugin, so there is nothing to install before an operator
		// can snapshot a ZFS or btrfs host.
		builtinSnapshots: snapshotdrivers.New(pool),
	}
}

func (s *Service) client(ctx context.Context, envID string) (*client.Client, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}
	return cli, nil
}

// Options returns the sensible backup choices for the inspected container.
func (s *Service) Options(ctx context.Context, envID, containerID string) (*BackupOptions, error) {
	cli, err := s.client(ctx, envID)
	if err != nil {
		return nil, err
	}
	opCtx, cancel := context.WithTimeout(ctx, coredocker.CallTimeout)
	defer cancel()

	infoRes, err := cli.ContainerInspect(opCtx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("inspecting container for backup options: %w", err)
	}
	info := infoRes.Container

	name := strings.TrimPrefix(info.Name, "/")
	if name == "" {
		name = shortID(info.ID)
	}

	options := []BackupOption{
		// Required is deliberately false: configuration used to be
		// unturnable-off, which meant a user who wanted only their volumes
		// still shipped the container definition. It is a default, not a
		// mandate.
		{Key: "config", Type: "config", Label: "Container configuration", Description: "Inspect data, environment, labels, networks, ports, restart policy, and host config.", Default: true, Partial: true},
	}
	if info.Config != nil && info.Config.Image != "" {
		options = append(options, BackupOption{Key: "image", Type: "image", Label: "Image archive", Description: "The Docker image used by this container.", Default: false})
	}
	options = append(options, BackupOption{Key: "filesystem", Type: "filesystem", Label: "Container filesystem", Description: "The writable container filesystem layer. Mounted volumes are backed up separately.", Default: false, Partial: true})
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
			Partial:     true,
		})
	}

	return &BackupOptions{
		ContainerID:    info.ID,
		ContainerName:  name,
		Options:        options,
		ConfigSections: configSectionCatalog(),
	}, nil
}

// ListPlans returns backup plans for an environment and optional container.
func (s *Service) ListPlans(ctx context.Context, envID, containerID string) ([]BackupPlan, error) {
	query := `
		SELECT p.id, p.name, p.environment_id, p.container_id, p.container_name, COALESCE(p.storage_location_id, ''),
		       p.include_config, p.include_logs, p.include_filesystem, p.include_image, p.selected_mounts,
		       COALESCE(p.selections, ''), p.log_tail_lines,
		       COALESCE(p.cron, ''), p.enabled, p.retention_count, p.retention_days, COALESCE(p.last_run_at, ''), COALESCE(p.next_run_at, ''),
		       COALESCE(p.consistency_mode, 'pause'), COALESCE(p.snapshot_config_id, ''),
		       p.created_at, p.updated_at, COALESCE(sc.name, '')
		FROM container_backup_plans p
		LEFT JOIN container_backup_snapshot_configs sc ON sc.id = p.snapshot_config_id
		WHERE p.environment_id = ?`
	args := []any{envID}
	if containerID != "" {
		query += " AND p.container_id = ?"
		args = append(args, containerID)
	}
	query += " ORDER BY p.name ASC LIMIT 1000"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing backup plans: %w", err)
	}
	defer func() { db.CloseRows(rows) }()

	plans := []BackupPlan{}
	for rows.Next() {
		plan, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		if err := s.hydratePlanStorageLocations(ctx, &plan); err != nil {
			return nil, err
		}
		// Auto-refresh stale container_id. Failures are swallowed
		// (logged via refreshPlanContainerID) so a flaky
		// orchestrator never breaks the list endpoint. The
		// returned `stale` flag tells the UI to show the
		// "Re-linked" badge on this request.
		refreshedPlan, stale := s.refreshPlanContainerID(ctx, plan)
		refreshedPlan.ContainerIDStale = stale
		plans = append(plans, refreshedPlan)
	}
	return plans, rows.Err()
}

// CreatePlan creates a backup plan.
// Consistency modes a backup plan can run in.
const (
	// ConsistencyPause stops the container while the archive is
	// written. Correct everywhere, at the cost of downtime.
	ConsistencyPause = "pause"
	// ConsistencySnapshot freezes the filesystem instead, so the
	// container keeps serving traffic while the backup reads a frozen
	// copy. Needs a snapshot config whose driver is reachable.
	ConsistencySnapshot = "snapshot"
	// ConsistencyLive reads the running container's files as they are.
	// Fastest, and the only mode that can produce a torn archive.
	ConsistencyLive = "live"
)

// normalizeConsistency validates a plan's consistency choice and
// returns the pair to persist. Snapshot mode without a config is a
// user error worth rejecting rather than silently downgrading: a plan
// that says "snapshot" and quietly pauses is the kind of thing an
// operator only discovers during an outage.
func normalizeConsistency(mode, snapshotConfigID string) (string, string, error) {
	mode = strings.TrimSpace(mode)
	snapshotConfigID = strings.TrimSpace(snapshotConfigID)
	switch mode {
	case "":
		return ConsistencyPause, "", nil
	case ConsistencyPause, ConsistencyLive:
		return mode, "", nil
	case ConsistencySnapshot:
		if snapshotConfigID == "" {
			return "", "", ErrSnapshotConfigRequired
		}
		return mode, snapshotConfigID, nil
	}
	return "", "", fmt.Errorf("unknown consistency mode %q", mode)
}

// CreatePlan validates and stores a backup plan for one container, deriving the selection map from the legacy booleans when the caller sends none.
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
	// Selections are the source of truth when the caller sends them; a
	// caller that only knows the old booleans still works, and gets the
	// equivalent all-or-nothing selection map. The legacy columns are
	// written either way so nothing that reads them regresses.
	selections := normalizeSelections(input.Selections)
	if selections == nil {
		selections = selectionsFromLegacy(
			input.IncludeConfig, input.IncludeLogs, input.IncludeFilesystem, input.IncludeImage,
			input.SelectedMounts,
		)
	}
	includeConfig, includeLogs, includeFilesystem, includeImage, selectedMounts := legacyFlagsFromSelections(selections)
	encodedSelections, err := encodeSelections(selections)
	if err != nil {
		return nil, err
	}
	selected, err := json.Marshal(selectedMounts)
	if err != nil {
		return nil, fmt.Errorf("encoding selected mounts: %w", err)
	}
	id := xid.New().String()
	now := time.Now().Format(time.RFC3339)
	nextRun := ""
	if input.Enabled && strings.TrimSpace(input.Cron) != "" {
		nextRun = nextCronRun(input.Cron, time.Now()).Format(time.RFC3339)
	}
	storageIDs := backupStorageLocationIDs(input.StorageLocationID, input.StorageLocationIDs)
	if len(storageIDs) == 0 {
		return nil, ErrBackupDestinationRequired
	}
	consistencyMode, snapshotConfigID, err := normalizeConsistency(input.ConsistencyMode, input.SnapshotConfigID)
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO container_backup_plans (
			id, name, environment_id, container_id, container_name, storage_location_id,
			include_config, include_logs, include_filesystem, include_image, selected_mounts,
			selections, log_tail_lines,
			cron, enabled, retention_count, retention_days, next_run_at,
			consistency_mode, snapshot_config_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, envID, input.ContainerID, containerName, nullString(firstStorageLocationID(storageIDs)),
		includeConfig, includeLogs, includeFilesystem, includeImage, string(selected),
		encodedSelections, input.LogTailLines,
		nullString(input.Cron), input.Enabled, input.RetentionCount, input.RetentionDays, nullString(nextRun),
		consistencyMode, nullString(snapshotConfigID), now, now,
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
		SELECT p.id, p.name, p.environment_id, p.container_id, p.container_name, COALESCE(p.storage_location_id, ''),
		       p.include_config, p.include_logs, p.include_filesystem, p.include_image, p.selected_mounts,
		       COALESCE(p.selections, ''), p.log_tail_lines,
		       COALESCE(p.cron, ''), p.enabled, p.retention_count, p.retention_days, COALESCE(p.last_run_at, ''), COALESCE(p.next_run_at, ''),
		       COALESCE(p.consistency_mode, 'pause'), COALESCE(p.snapshot_config_id, ''),
		       p.created_at, p.updated_at, COALESCE(sc.name, '')
		FROM container_backup_plans p
		LEFT JOIN container_backup_snapshot_configs sc ON sc.id = p.snapshot_config_id
		WHERE p.id = ?`, id)
	plan, err := scanPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
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
		ids := backupStorageLocationIDs(*input.StorageLocationID, nil)
		if len(ids) == 0 {
			return nil, ErrBackupDestinationRequired
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET storage_location_id = ?, updated_at = ? WHERE id = ?", nullString(firstStorageLocationID(ids)), now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan storage location: %w", err)
		}
		if err := s.setPlanStorageLocations(ctx, id, ids); err != nil {
			return nil, err
		}
	}
	if input.StorageLocationIDs != nil {
		ids := backupStorageLocationIDs("", *input.StorageLocationIDs)
		if len(ids) == 0 {
			return nil, ErrBackupDestinationRequired
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
	// Selections supersede the individual include_* / selected_mounts
	// fields when present: they carry the same information plus the
	// per-option mode and paths. The legacy columns are rewritten from
	// them so the two never disagree.
	if input.Selections != nil {
		selections := normalizeSelections(*input.Selections)
		encoded, err := encodeSelections(selections)
		if err != nil {
			return nil, err
		}
		includeConfig, includeLogs, includeFilesystem, includeImage, mounts := legacyFlagsFromSelections(selections)
		selected, err := json.Marshal(mounts)
		if err != nil {
			return nil, fmt.Errorf("encoding selected mounts: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, `
			UPDATE container_backup_plans
			SET selections = ?, include_config = ?, include_logs = ?, include_filesystem = ?,
			    include_image = ?, selected_mounts = ?, updated_at = ?
			WHERE id = ?`,
			encoded, includeConfig, includeLogs, includeFilesystem, includeImage, string(selected), now, id,
		); err != nil {
			return nil, fmt.Errorf("updating backup plan selections: %w", err)
		}
	} else if input.SelectedMounts != nil {
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
	if input.ConsistencyMode != nil || input.SnapshotConfigID != nil {
		mode := plan.ConsistencyMode
		if input.ConsistencyMode != nil {
			mode = *input.ConsistencyMode
		}
		configID := plan.SnapshotConfigID
		if input.SnapshotConfigID != nil {
			configID = *input.SnapshotConfigID
		}
		normalizedMode, normalizedConfig, err := normalizeConsistency(mode, configID)
		if err != nil {
			return nil, err
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_plans SET consistency_mode = ?, snapshot_config_id = ?, updated_at = ? WHERE id = ?", normalizedMode, nullString(normalizedConfig), now, id); err != nil {
			return nil, fmt.Errorf("updating backup plan consistency: %w", err)
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
			nextRun = nextCronRun(cron, time.Now()).Format(time.RFC3339)
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

	// Build the query dynamically so callers can list all runs for
	// an environment (no container filter), one container's runs
	// (the original path), or a single run by id. The Backups page
	// hits the env-only path so it can show every container's recent
	// runs side-by-side; the per-container Backups tab uses the
	// container-scoped path; deep-links use the by-id path.
	//
	// container_name comes off the run row itself; the LEFT JOIN on
	// container_backup_plans is only the fallback for rows written
	// before migration 134 added that column. Rows that satisfy
	// neither are named from the live orchestrator further down.
	query := `
		SELECT r.id, COALESCE(r.plan_id, ''), COALESCE(r.operation, 'backup'), COALESCE(r.source_run_id, ''),
		       r.environment_id, r.container_id, r.status, COALESCE(r.archive_path, ''),
		       r.archive_size, COALESCE(r.archive_encryption, ''), COALESCE(r.archive_key_id, ''), COALESCE(r.error, ''),
		       COALESCE(r.progress_stage, ''), COALESCE(r.progress_message, ''), COALESCE(r.progress_updated_at, ''),
		       r.started_at, COALESCE(r.completed_at, ''), r.duration_ms, r.created_at, r.updated_at,
		       COALESCE(NULLIF(r.container_name, ''), p.container_name, '') AS container_name,
		       COALESCE(r.included_items, ''), COALESCE(r.available_items, ''), COALESCE(p.name, ''), COALESCE(r.origin, '')
		FROM container_backup_runs r
		LEFT JOIN container_backup_plans p ON r.plan_id = p.id
		WHERE r.environment_id = ?`
	args := []any{envID}
	if containerID != "" {
		query += " AND r.container_id = ?"
		args = append(args, containerID)
	}
	query += " ORDER BY r.started_at DESC LIMIT 1000"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing backup runs: %w", err)
	}

	runs, err := scanRunRows(rows)
	if err != nil {
		return nil, err
	}
	for i := range runs {
		run := &runs[i]
		s.annotateRunKeyRequirement(run)
		// Auto-refresh stale container_id. The lookup keys on the
		// run's own container_name, falling back to the plan's when
		// the run predates that column. Failures are swallowed so a
		// single broken env doesn't fail the whole list.
		runStale := false
		refreshed, planErr := s.refreshPlanContainerIDForRun(ctx, run)
		s.logRefreshFailure(run.ID, planErr)
		if refreshed {
			runStale = true
		}
		runRefreshed, runErr := s.refreshRunContainerID(ctx, run)
		s.logRefreshFailure(run.ID, runErr)
		if runRefreshed {
			runStale = true
		}
		run.ContainerIDStale = runStale
	}

	// Rows written before the run table carried its own container_name
	// (and rows whose plan was deleted) still have an empty name. Fill
	// them from the live orchestrator once and persist, so the backup
	// list stops showing a bare container id for historical runs.
	s.backfillRunContainerNames(ctx, envID, runs)

	for i := range runs {
		if err := s.hydrateRunDestinations(ctx, &runs[i]); err != nil {
			return nil, err
		}
	}
	return runs, nil
}

// Run returns one run by id, or (nil, nil) when it does not exist, so the
// handler can answer 404 without inspecting sql errors. Same convention as
// Restore / StartRestore.
func (s *Service) Run(ctx context.Context, id string) (*BackupRun, error) {
	// refreshLinks=false: this is the endpoint the restore dialog polls, and
	// re-linking the container on every tick is what wedged runs on "running".
	run, err := s.runByIDLinked(ctx, strings.TrimSpace(id), false)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return run, nil
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

	runs, err := scanAbandonedRuns(rows)
	if err != nil {
		return err
	}

	for _, run := range runs {
		if backupRunIsActive(run.id) {
			continue
		}
		// Best-effort unpause before finalising so a backup that crashed
		// during the archive-write phase does not leave the container in a
		// paused state on the Docker daemon. The release runs on a
		// cancellation-free copy of ctx so a near-deadline parent does not cancel it.
		if run.environmentID != "" && run.containerID != "" {
			s.releaseStuckContainerPause(ctx, run.environmentID, run.containerID)
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
func (s *Service) releaseStuckContainerPause(ctx context.Context, envID, containerID string) {
	if s.pool == nil {
		return
	}
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		if s.logger != nil {
			s.logger.Debug("abandoned backup: docker client unavailable for unpause", "env", envID, "container", containerID, "error", err)
		}
		return
	}
	unpauseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), coredocker.CallTimeout)
	defer cancel()
	if _, err := cli.ContainerUnpause(unpauseCtx, containerID, client.ContainerUnpauseOptions{}); err != nil {
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
	included, available := s.runItemsForPlan(ctx, plan)
	run, err := s.createRun(ctx, plan.ID, plan.EnvironmentID, plan.ContainerID, plan.ContainerName, included, available)
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
	included, available := s.runItemsForPlan(ctx, plan)
	run, err := s.createRun(ctx, plan.ID, plan.EnvironmentID, plan.ContainerID, plan.ContainerName, included, available)
	if err != nil {
		return nil, err
	}
	s.startBackgroundRun(ctx, plan, run.ID)
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
	included, available := s.runItemsForPlan(ctx, plan)
	run, err := s.createRun(ctx, "", envID, containerID, plan.ContainerName, included, available)
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
	included, available := s.runItemsForPlan(ctx, plan)
	run, err := s.createRun(ctx, "", envID, containerID, plan.ContainerName, included, available)
	if err != nil {
		return nil, err
	}
	s.startBackgroundRun(ctx, plan, run.ID)
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
	storageIDs := backupStorageLocationIDs(input.StorageLocationID, input.StorageLocationIDs)
	if len(storageIDs) == 0 {
		return nil, ErrBackupDestinationRequired
	}
	// An ad-hoc run used to hard-code IncludeConfig: true, which is what
	// made the config option unturnable-off in the UI. It now follows the
	// caller's selection like every other option.
	selections := normalizeSelections(input.Selections)
	if selections == nil {
		selections = selectionsFromLegacy(
			input.IncludeConfig, input.IncludeLogs, input.IncludeFilesystem, input.IncludeImage,
			input.SelectedMounts,
		)
	}
	includeConfig, includeLogs, includeFilesystem, includeImage, selectedMounts := legacyFlagsFromSelections(selections)
	return &BackupPlan{
		ID:                 "",
		Name:               name,
		EnvironmentID:      envID,
		ContainerID:        containerID,
		ContainerName:      containerName,
		StorageLocationID:  firstStorageLocationID(storageIDs),
		StorageLocationIDs: storageIDs,
		IncludeConfig:      includeConfig,
		IncludeLogs:        includeLogs,
		IncludeFilesystem:  includeFilesystem,
		IncludeImage:       includeImage,
		SelectedMounts:     selectedMounts,
		Selections:         selections,
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
	if s.backupCrypto == nil || plan == nil || !s.pool.IsAgentEnvContext(ctx, plan.EnvironmentID) || !s.pool.AgentAtLeastContext(ctx, plan.EnvironmentID, agentBackupMinVersion) {
		return false
	}
	// The agent builds its archive from the include_* booleans alone and
	// knows nothing about per-option path selections, so a partial plan
	// routed through it would silently capture everything the user
	// deselected. Fall back to host-side capture instead: the Docker
	// streams still come over the agent transport, but the filtering
	// happens here where the selection is understood.
	//
	// The cost is bandwidth — the whole export crosses the wire before
	// being trimmed — which is the right trade against capturing data the
	// user explicitly excluded. Teaching the agent to filter would remove
	// that cost and is the natural follow-up.
	if anySelectionPartial(plan.Selections) {
		if s.logger != nil {
			s.logger.Info(
				"container backup: partial selection forces host-side capture on agent environment",
				"env", plan.EnvironmentID, "plan", plan.ID,
			)
		}
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
	agentConn, ok := s.pool.AgentConnectionContext(ctx, plan.EnvironmentID)
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

	// Freeze the agent host's filesystem before it builds the archive.
	// When the driver hands back readable paths the agent reads those
	// instead of the live container, and never pauses it.
	frozen := s.freezeForBackup(ctx, plan, runID)
	defer func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer releaseCancel()
		s.releaseSnapshot(releaseCtx, plan, runID, frozen)
	}()

	payload := coreagent.BackupPayload{
		TransferID:          runID,
		ContainerID:         plan.ContainerID,
		ContainerName:       plan.ContainerName,
		IncludeConfig:       plan.IncludeConfig,
		IncludeLogs:         plan.IncludeLogs,
		IncludeFilesystem:   plan.IncludeFilesystem,
		IncludeImage:        plan.IncludeImage,
		SelectedMounts:      plan.SelectedMounts,
		SnapshotMounts:      frozen.mountPaths,
		ConsistencyMode:     plan.ConsistencyMode,
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

func (s *Service) startBackgroundRun(ctx context.Context, plan *BackupPlan, runID string) {
	bgCtx := context.WithoutCancel(ctx)
	planCopy := *plan
	go func() {
		runCtx, cancel := context.WithTimeout(bgCtx, backupRunBackgroundTimeout)
		runCancelers.Store(runID, cancel)
		defer func() {
			cancel()
			runCancelers.Delete(runID)
		}()
		if _, err := s.executeRun(runCtx, &planCopy, runID); err != nil && s.logger != nil {
			s.logger.Error("background container backup failed", "run", runID, "env", planCopy.EnvironmentID, "container", planCopy.ContainerID, "error", err)
		}
	}()
}

func (s *Service) containerName(ctx context.Context, envID, containerID string) (string, error) {
	cli, err := s.client(ctx, envID)
	if err != nil {
		return "", err
	}
	opCtx, cancel := context.WithTimeout(ctx, coredocker.CallTimeout)
	defer cancel()
	infoRes, err := cli.ContainerInspect(opCtx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return "", fmt.Errorf("inspecting container: %w", err)
	}
	info := infoRes.Container
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
func (s *Service) resolveContainerForBackup(ctx context.Context, opCtx context.Context, cli *client.Client, plan *BackupPlan, logger *slog.Logger) (container.InspectResponse, string, error) {
	infoRes, err := cli.ContainerInspect(opCtx, plan.ContainerID, client.ContainerInspectOptions{})
	info := infoRes.Container
	if err == nil {
		return info, plan.ContainerID, nil
	}
	if !isContainerNotFound(err) {
		return container.InspectResponse{}, plan.ContainerID, err
	}
	if plan.ContainerName == "" {
		return container.InspectResponse{}, plan.ContainerID, fmt.Errorf("inspecting container for backup: %w", err)
	}
	if logger != nil {
		logger.Info(
			"container backup: stored container id is stale, resolving by name",
			"container_id", plan.ContainerID, "container_name", plan.ContainerName,
		)
	}
	f := make(client.Filters)
	f.Add("name", plan.ContainerName)
	listRes, listErr := cli.ContainerList(opCtx, client.ContainerListOptions{All: true, Filters: f})
	if listErr != nil {
		return container.InspectResponse{}, plan.ContainerID, fmt.Errorf("resolving container by name: %w", listErr)
	}
	list := listRes.Items
	for _, c := range list {
		if strings.TrimPrefix(c.Names[0], "/") == plan.ContainerName {
			infoRes, inspectErr := cli.ContainerInspect(opCtx, c.ID, client.ContainerInspectOptions{})
			if inspectErr != nil {
				return container.InspectResponse{}, plan.ContainerID, fmt.Errorf("inspecting resolved container: %w", inspectErr)
			}
			info := infoRes.Container
			return info, c.ID, nil
		}
	}
	return container.InspectResponse{}, plan.ContainerID, fmt.Errorf("no container named %q found in environment", plan.ContainerName)
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

// createRun inserts the queued run row. containerName is stored on the
// run itself rather than being recovered later through the plan join:
// ad-hoc runs have no plan, and a plan can be deleted out from under a
// run (ON DELETE SET NULL), which would otherwise leave the backup list
// showing a bare container id.
// included / available snapshot what this run will capture and what the
// container had on offer, so the Backups overview can label the run full
// or partial long after the plan (or the container) has changed. See
// migration 140.
func (s *Service) createRun(ctx context.Context, planID, envID, containerID, containerName string, included, available []string) (*BackupRun, error) {
	return s.createRunWithOrigin(ctx, planID, envID, containerID, containerName, "", included, available)
}

// createRunWithOrigin is createRun with the run's origin stamped on insert; an empty origin
// means McHarbor produced the archive itself, "upload" means an operator supplied it.
func (s *Service) createRunWithOrigin(ctx context.Context, planID, envID, containerID, containerName, origin string, included, available []string) (*BackupRun, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	containerName = strings.TrimSpace(strings.TrimPrefix(containerName, "/"))
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO container_backup_runs (id, plan_id, operation, environment_id, container_id, container_name, status, started_at, created_at, updated_at, included_items, available_items, origin)
		VALUES (?, ?, 'backup', ?, ?, ?, 'running', ?, ?, ?, ?, ?, ?)`, id, nullString(planID), envID, containerID, containerName, now, now, now,
		encodeRunItems(included), encodeRunItems(available), nullString(origin))
	if err != nil {
		return nil, fmt.Errorf("creating backup run: %w", err)
	}
	if err := s.setRunProgress(ctx, id, "queued", ""); err != nil && s.logger != nil {
		s.logger.Warn("container backup progress update failed", "run", id, "stage", "queued", "error", err)
	}
	return &BackupRun{ID: id, PlanID: planID, EnvironmentID: envID, ContainerID: containerID, ContainerName: containerName, Status: "running", Origin: origin, ProgressStage: "queued", ProgressUpdatedAt: now, StartedAt: now, CreatedAt: now, UpdatedAt: now, IncludedItems: included, AvailableItems: available, Scope: runScope(included, available)}, nil
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
		errorText = backupRunFailureText(operation, progressStage, execErr)
		progressMessage = errorText
	}
	if execErr != nil && s.logger != nil {
		s.logger.Error("container backup run failed", "run", runID, "env", envID, "container", containerID, "stage", progressStage, "error", execErr)
	}
	startedAt := s.parseRunStart(runID, started)
	duration := now.Sub(startedAt).Milliseconds()
	// This is the write that moves a run off "running". Losing it to
	// transient write contention strands the row forever and the UI spins on
	// a run that is actually over, so it is the one place that must retry.
	var result sql.Result
	err := db.RetryOnBusy(finishCtx, 5, func() error {
		var execErr error
		result, execErr = s.db.ExecContext(finishCtx, `
		UPDATE container_backup_runs
		SET status = ?, archive_path = ?, archive_size = ?, archive_encryption = ?, archive_key_id = ?,
		    error = ?, progress_stage = ?, progress_message = ?, progress_updated_at = ?,
		    completed_at = ?, duration_ms = ?, updated_at = ?
		WHERE id = ? AND status = 'running'`,
			status, nullString(archive.path), archive.size, archive.encryption, archive.keyID,
			nullString(errorText), finalProgressStage, progressMessage, now.Format(time.RFC3339),
			now.Format(time.RFC3339), duration, now.Format(time.RFC3339), runID)
		return execErr
	})
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
	telemetry.BackupJobFinished(operation, status, time.Duration(duration)*time.Millisecond)
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
	// Emit a terminal backup_log entry so the Logging tab always
	// reflects the run's outcome even when the last in-flight
	// stage predates the failure (e.g. a backup that dies mid-upload).
	s.recordFinalRunLog(finishCtx, run)
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

func backupRunFailureText(operation, stage string, cause error) string {
	return joinFailureDetail(backupRunFailureHeadline(operation, stage), cause)
}

// backupRunFailureHeadline names the stage that broke, in operator terms.
// It is the first half of the stored error; sanitizeBackupError supplies
// the second half, which is what the failure dialog is actually opened for.
func backupRunFailureHeadline(operation, stage string) string {
	if operation == "restore" {
		switch backupRunFailureStage(operation, stage) {
		case "restore_connecting":
			return "Restore failed while connecting to the Docker environment."
		case "restore_inspecting":
			return "Restore failed while inspecting the target container."
		case "restore_scanning":
			return "Restore failed while reading the backup archive."
		case "restore_image":
			return "Restore failed while loading the container image."
		case "restore_filesystem":
			return "Restore failed while restoring the container filesystem."
		case "restore_mounts":
			return "Restore failed while restoring mounted data."
		case "restore_config":
			return "Container configuration cannot be restored: applying it would require recreating the container. Restore files, mounts or the image instead."
		default:
			return "Restore failed."
		}
	}
	switch backupRunFailureStage(operation, stage) {
	case "connecting":
		return "Backup failed while connecting to the Docker environment."
	case "inspecting":
		return "Backup failed while inspecting the container."
	case "preparing":
		return "Backup failed while preparing the archive."
	case "writing", "manifest", "config", "finalizing":
		return "Backup failed while writing the archive."
	case "logs":
		return "Backup failed while reading container logs."
	case "filesystem":
		return "Backup failed while exporting the container filesystem."
	case "image":
		return "Backup failed while saving the container image."
	case "mounts":
		return "Backup failed while copying mounted data."
	case "uploading":
		return "Backup failed while uploading to selected storage."
	default:
		return "Backup failed."
	}
}

// joinFailureDetail appends the sanitized cause to a stage headline. A
// headline on its own is what the old text had, and it sent the operator
// to the container logs the failure dialog exists to replace.
func joinFailureDetail(headline string, cause error) string {
	detail := sanitizeBackupError(cause)
	if detail == "" {
		return headline
	}
	return headline + " " + detail
}

// backupErrorSecretPattern strips the parts of a storage error that carry
// credentials: presigned-URL query strings (S3/Azure SAS hand back the whole
// signed URL on failure) and userinfo in an embedded URL.
var (
	backupErrorQueryPattern    = regexp.MustCompile(`(?i)(https?://[^\s"']+)\?[^\s"']*`)
	backupErrorUserinfoPattern = regexp.MustCompile(`(?i)([a-z0-9+.-]+://)[^\s/@"']*:[^\s/@"']*@`)
)

// backupErrorDetailLimit caps the stored detail. Docker and the storage
// SDKs can hand back multi-kilobyte bodies; the run row is read on every
// list query, so the tail is dropped rather than paged into the UI.
const backupErrorDetailLimit = 800

// sanitizeBackupError turns an internal error into text safe to store on
// the run row and show in the failure dialog.
//
// The api-routes rule says never to hand a raw err.Error() to a client,
// and for an ordinary endpoint that is right. A backup failure record is
// the documented exception, for the same reason the database connection
// tester carries its driver message: the operator opened the dialog to
// find out what broke, and a stage name alone does not answer that. The
// runs are readable only behind the backups permissions, and the two
// things an error here plausibly leaks — signed storage URLs and
// embedded credentials — are scrubbed before the text is stored.
func sanitizeBackupError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return ""
	}
	msg = backupErrorQueryPattern.ReplaceAllString(msg, "$1?<redacted>")
	msg = backupErrorUserinfoPattern.ReplaceAllString(msg, "$1<redacted>@")
	// Collapse newlines: the value is rendered in a single dialog block
	// and stored in one column, so a stack-trace-shaped body would push
	// the useful first line out of view.
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > backupErrorDetailLimit {
		msg = msg[:backupErrorDetailLimit] + "..."
	}
	return msg
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

	runs, err := scanAbandonedRestoreRuns(rows)
	if err != nil {
		return err
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
	// Skip destinations updated within the last 5 minutes: those are
	// either in-flight uploads from the original backup run or an
	// operator-triggered destination retry upload. Without this guard
	// the reaper races the retry goroutine and marks its `uploading`
	// destination as `failure` mid-upload, hiding the live byte
	// progress from the operator.
	reaperCutoff := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		UPDATE container_backup_run_destinations
		SET status = 'failure',
		    error = CASE WHEN COALESCE(error, '') = '' THEN 'Upload did not finish before the backup run ended.' ELSE error END,
		    updated_at = ?
		WHERE status = 'uploading'
		  AND updated_at < ?
		  AND run_id IN (SELECT id FROM container_backup_runs WHERE status <> 'running')`,
		now, reaperCutoff)
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
		return
	}
	// Emit a backup_log row only when the stage actually changes.
	// The in-memory cache keeps the write rate bounded by the
	// number of unique stages per run, not by the number of progress
	// updates.
	prevAny, _ := s.runStageCache.Load(runID)
	prev, _ := prevAny.(string)
	if prev == stage {
		return
	}
	s.runStageCache.Store(runID, stage)
	s.recordStageLog(ctx, runID, stage, message)
}

// recordStageLog writes a single backup_log row for a stage change.
// Container id, container name, and plan name are looked up via
// the runs join so the row is self-describing for the UI even
// if the join is later extended.
func (s *Service) recordStageLog(ctx context.Context, runID, stage, message string) {
	row := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(plan_id, ''), COALESCE(container_id, ''),
		       COALESCE(environment_id, ''),
		       COALESCE((SELECT name FROM container_backup_plans WHERE id = plan_id), ''),
		       COALESCE(status, 'running')
		FROM container_backup_runs
		WHERE id = ?`,
		runID,
	)
	var planID, containerID, environmentID, planName, runStatus string
	if err := row.Scan(&planID, &containerID, &environmentID, &planName, &runStatus); err != nil {
		// Don't fail the progress path if the log insert is blocked;
		// just log and move on. The run will keep progressing.
		if s.logger != nil {
			s.logger.Debug("container backup: log stage query failed", "run", runID, "error", err)
		}
		return
	}
	severity := severityForStage(stage, runStatus)
	action := actionForStage(stage, runStatus)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO container_backup_log
			(id, environment_id, plan_id, plan_name, run_id,
			 container_id, container_name, action, phase, severity,
			 message, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'backup', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		xid.New().String(),
		nullableString(environmentID), nullableString(planID), nullableString(planName),
		runID, nullableString(containerID), "",
		action, stage, severity, nullableString(message),
	)
	if err != nil && s.logger != nil {
		s.logger.Debug("container backup: log stage insert failed", "run", runID, "stage", stage, "error", err)
	}
}

// recordFinalRunLog writes the terminal backup_log row for a
// run once it has been finalized. This guarantees the Logging
// tab always has an error row for failed/cancelled runs even
// when the last in-flight stage was something like "uploading".
func (s *Service) recordFinalRunLog(ctx context.Context, run *BackupRun) {
	if run == nil {
		return
	}
	stage := strings.TrimSpace(run.ProgressStage)
	if stage == "" {
		stage = "complete"
	}
	message := strings.TrimSpace(run.Error)
	if message == "" {
		switch run.Status {
		case "success":
			if run.Operation == "restore" {
				message = "Restore completed"
			} else {
				message = "Backup completed"
			}
		default:
			message = "Run finished with status " + run.Status
		}
	}
	severity := severityForStage(stage, run.Status)
	action := actionForStage(stage, run.Status)
	// Look up the plan name so the terminal row matches the
	// in-flight rows emitted by recordStageLog.
	var planName sql.NullString
	if run.PlanID != "" {
		if err := s.db.QueryRowContext(ctx,
			`SELECT name FROM container_backup_plans WHERE id = ?`,
			run.PlanID,
		).Scan(&planName); err != nil && !errors.Is(err, sql.ErrNoRows) && s.logger != nil {
			s.logger.Warn("container backups: reading plan name for run log failed", "run", run.ID, "error", err)
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO container_backup_log
			(id, environment_id, plan_id, plan_name, run_id,
			 container_id, container_name, action, phase, severity,
			 message, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'backup', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		xid.New().String(),
		nullableString(run.EnvironmentID), nullableString(run.PlanID),
		nullableStringFromNull(planName),
		run.ID, nullableString(run.ContainerID), nullableString(run.ContainerName),
		action, stage, severity, nullableString(message),
	)
	if err != nil && s.logger != nil {
		s.logger.Debug("container backup: final log insert failed", "run", run.ID, "error", err)
	}
}

// nullableStringFromNull returns nil for an invalid (NULL) string
// and the value for a valid one, so the SQL driver writes NULL
// instead of ” for unknown plan names.
func nullableStringFromNull(value sql.NullString) interface{} {
	if !value.Valid {
		return nil
	}
	return value.String
}

// severityForStage derives the UI severity from the run's stage
// and current run status. The run status takes precedence when
// the run has already finalized (e.g. failure or cancelled) so
// the log table reflects the actual outcome instead of the
// last in-flight stage.
func severityForStage(stage, runStatus string) string {
	switch runStatus {
	case "failure", "cancelled":
		return "error"
	case "success":
		return "success"
	}
	switch stage {
	case "failed", "restore_failed", "failure":
		return "error"
	case "complete", "restore_complete", "success":
		return "success"
	}
	return "info"
}

// actionForStage maps a stage + run status to the UI-facing
// action label. The action key is what the i18n bundle keys off
// (e.g. backups.logs.action.fail).
func actionForStage(stage, runStatus string) string {
	if runStatus == "failure" || runStatus == "cancelled" {
		return "fail"
	}
	if runStatus == "success" {
		return "complete"
	}
	switch stage {
	case "failed", "restore_failed", "failure":
		return "fail"
	case "complete", "restore_complete", "success":
		return "complete"
	}
	return stage
}

// nullableString returns the string as a pointer or nil for empty
// values so the SQL driver writes NULL instead of ”.
func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func (s *Service) runByID(ctx context.Context, id string) (*BackupRun, error) {
	return s.runByIDLinked(ctx, id, true)
}

// runByIDLinked reads one run, optionally refreshing its container link.
//
// The refresh resolves the container against the live orchestrator under a 5s
// timeout and persists the result, so it is both slow and a writer. That is
// fine for the one-shot reads that surround a restore, and wrong for the
// progress poll: at one request a second against an unreachable environment it
// stacks five concurrent writers and starves finishRun into SQLITE_BUSY, which
// strands the run on "running" forever. Pass false wherever the caller only
// needs status and progress.
func (s *Service) runByIDLinked(ctx context.Context, id string, refreshLinks bool) (*BackupRun, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT r.id, COALESCE(r.plan_id, ''), COALESCE(r.operation, 'backup'), COALESCE(r.source_run_id, ''),
		       r.environment_id, r.container_id, r.status, COALESCE(r.archive_path, ''),
		       r.archive_size, COALESCE(r.archive_encryption, ''), COALESCE(r.archive_key_id, ''), COALESCE(r.error, ''),
		       COALESCE(r.progress_stage, ''), COALESCE(r.progress_message, ''), COALESCE(r.progress_updated_at, ''),
		       r.started_at, COALESCE(r.completed_at, ''), r.duration_ms, r.created_at, r.updated_at,
		       COALESCE(NULLIF(r.container_name, ''), p.container_name, '') AS container_name,
		       COALESCE(r.included_items, ''), COALESCE(r.available_items, ''), COALESCE(p.name, ''), COALESCE(r.origin, '')
		FROM container_backup_runs r
		LEFT JOIN container_backup_plans p ON r.plan_id = p.id
		WHERE r.id = ?`, id)
	run, err := scanRun(row)
	if err != nil {
		return nil, err
	}
	s.annotateRunKeyRequirement(&run)
	// Auto-refresh stale container_id (plan + run side). Same
	// resilience contract as ListRuns: failures are swallowed so a
	// single broken env doesn't fail the run lookup.
	if refreshLinks {
		planRefreshed, planErr := s.refreshPlanContainerIDForRun(ctx, &run)
		s.logRefreshFailure(run.ID, planErr)
		if planRefreshed {
			run.ContainerIDStale = true
		}
		runRefreshed, runErr := s.refreshRunContainerID(ctx, &run)
		s.logRefreshFailure(run.ID, runErr)
		if runRefreshed {
			run.ContainerIDStale = true
		}
	}
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

// DeleteRun deletes a finished or failed backup run and removes the
// archive from every storage destination (local + remote providers
// like OneDrive / SharePoint that we have a delete function for).
//
// Per-destination deletes are best-effort: failures are logged but do
// not stop the run row from being removed. The operator still gets
// the local archive and the database row gone; they can clean up
// remote stragglers by hand using the audit log entry that records
// which destinations failed to delete.
func (s *Service) DeleteRun(ctx context.Context, id string) (bool, error) {
	run, err := s.runByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if run.Status == "running" {
		return false, ErrBackupRunActive
	}

	// Remove the archive from every destination before touching the
	// local archive directory. We load destinations + their storage
	// location configs so we have OAuth credentials handy for the
	// Microsoft Graph delete calls.
	if err := s.hydrateRunDestinations(ctx, run); err != nil {
		if s.logger != nil {
			s.logger.Warn("container backup destination hydrate before delete failed", "run", id, "error", err)
		}
	}
	locationsByID := map[string]*backupStorageDestination{}
	for _, dest := range run.Destinations {
		if strings.TrimSpace(dest.StorageLocationID) == "" {
			continue
		}
		if _, ok := locationsByID[dest.StorageLocationID]; ok {
			continue
		}
		location, err := s.backupStorageDestination(ctx, dest.StorageLocationID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("container backup destination load before delete failed",
					"run", id, "destination", dest.ID, "storage", dest.StorageLocationID, "error", err)
			}
			continue
		}
		if location == nil {
			continue
		}
		locationsByID[dest.StorageLocationID] = location
	}
	for _, dest := range run.Destinations {
		var location *backupStorageDestination
		if strings.TrimSpace(dest.StorageLocationID) != "" {
			location = locationsByID[dest.StorageLocationID]
		}
		if err := s.deleteDestinationFile(ctx, dest, location, s.logger); err != nil {
			if s.logger != nil {
				s.logger.Warn("container backup destination delete failed",
					"run", id, "destination", dest.ID,
					"type", dest.LocationType, "name", dest.StorageLocationName,
					"path", dest.Path, "error", err)
			}
		}
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

	// Always explicitly delete the destination rows — never rely on
	// ON DELETE CASCADE alone, because SQLite's `PRAGMA foreign_keys`
	// is per-connection and a connection that ran a migration with
	// `PRAGMA foreign_keys = OFF` retains the disabled state even
	// after returning to the pool. A silent cascade failure would
	// leave destination rows pointing at cloud files nobody will
	// ever clean up. We delete here AND keep CASCADE in the schema
	// as a belt-and-braces fallback for future code paths.
	destinationIDs := make([]string, 0, len(run.Destinations))
	for _, dest := range run.Destinations {
		destinationIDs = append(destinationIDs, dest.ID)
	}
	if len(destinationIDs) > 0 {
		// Re-enable FK on whichever connection lands this DELETE.
		// Even on Postgres / non-SQLite drivers PRAGMA is a no-op,
		// so this is safe to leave in unconditionally.
		if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil && s.logger != nil {
			s.logger.Debug("container backup: enabling foreign keys before delete failed", "error", err)
		}
		if _, err := s.db.ExecContext(ctx,
			"DELETE FROM container_backup_run_destinations WHERE run_id = ?", id,
		); err != nil {
			if s.logger != nil {
				s.logger.Warn(
					"container backup destination row delete failed",
					"run", id, "error", err,
				)
			}
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
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if run.Status != "running" {
		return run, ErrBackupRunNotCancellable
	}

	now := time.Now().UTC()
	startedAt := s.parseRunStart(run.ID, run.StartedAt)
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
	if errors.Is(err, sql.ErrNoRows) {
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
	defer func() { db.CloseRows(rows) }()

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

	// Through DeleteRun rather than a bare row delete: removing only the row
	// and the staging directory left every destination copy behind, which
	// is a retention policy that never frees any space.
	for _, run := range prune {
		if _, err := s.DeleteRun(ctx, run.id); err != nil {
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
	if errors.Is(err, sql.ErrNoRows) {
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
	if errors.Is(err, sql.ErrNoRows) {
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
	defer closeLogged(file, "backup archive")
	defer closeLogged(decrypted, "decrypted backup stream")

	options, err := restoreArchiveOptions(decrypted)
	if err != nil {
		return nil, err
	}
	return &RestoreBackupOptions{RunID: run.ID, Items: options}, nil
}

// StartRestore starts a background restore run and returns the tracked run.
func (s *Service) StartRestore(ctx context.Context, runID string, input RestoreBackupInput) (*BackupRun, error) {
	sourceRun, err := s.runByID(ctx, strings.TrimSpace(runID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if sourceRun.Operation != "backup" || sourceRun.Status != "success" || strings.TrimSpace(sourceRun.ArchivePath) == "" {
		return nil, ErrBackupRunNotDownloadable
	}
	// Configuration and the filesystem layer together are restored in that
	// order (restoreConfigThenFilesystem), so there is nothing to refuse here.
	if _, err := s.restoreCrypto(sourceRun, input.SecretKey); err != nil {
		return nil, err
	}
	if err := s.guardSelfRestoreFor(ctx, sourceRun.EnvironmentID, sourceRun.ContainerID, input.WorkflowSelfAuthorized); err != nil {
		return nil, err
	}
	run, err := s.createRestoreRun(ctx, sourceRun, input.RestoreItems)
	if err != nil {
		return nil, err
	}
	s.startBackgroundRestore(ctx, run.ID, sourceRun.ID, input)
	return run, nil
}

// Restore applies restorable entries from a completed encrypted backup run to its original container.
func (s *Service) Restore(ctx context.Context, runID string, input RestoreBackupInput) (*RestoreBackupResult, error) {
	run, err := s.runByID(ctx, strings.TrimSpace(runID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.restoreFromRun(ctx, "", run, input)
}

// requested is the caller's item selection; empty means "everything in
// the archive". The row is seeded with that intent so a restore that is
// still running already shows the right full/partial label — the actual
// restored set overwrites included_items when the run finishes.
func (s *Service) createRestoreRun(ctx context.Context, sourceRun *BackupRun, requested []string) (*BackupRun, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	included, available := restoreRunItems(sourceRun, requested)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO container_backup_runs (id, plan_id, operation, source_run_id, environment_id, container_id, container_name, status, started_at, created_at, updated_at, included_items, available_items)
		VALUES (?, NULL, 'restore', ?, ?, ?, ?, 'running', ?, ?, ?, ?, ?)`,
		id, sourceRun.ID, sourceRun.EnvironmentID, sourceRun.ContainerID, sourceRun.ContainerName, now, now, now,
		encodeRunItems(included), encodeRunItems(available))
	if err != nil {
		return nil, fmt.Errorf("creating restore run: %w", err)
	}
	if err := s.setRunProgress(ctx, id, "queued", ""); err != nil && s.logger != nil {
		s.logger.Warn("container restore progress update failed", "run", id, "stage", "queued", "error", err)
	}
	return &BackupRun{ID: id, Operation: "restore", SourceRunID: sourceRun.ID, EnvironmentID: sourceRun.EnvironmentID, ContainerID: sourceRun.ContainerID, ContainerName: sourceRun.ContainerName, Status: "running", ProgressStage: "queued", ProgressUpdatedAt: now, StartedAt: now, CreatedAt: now, UpdatedAt: now, IncludedItems: included, AvailableItems: available, Scope: runScope(included, available)}, nil
}

func (s *Service) startBackgroundRestore(parent context.Context, restoreRunID, sourceRunID string, input RestoreBackupInput) {
	bgCtx := context.WithoutCancel(parent)
	go func() {
		done := markBackupRunActive(restoreRunID)
		defer done()

		ctx, cancel := context.WithTimeout(bgCtx, backupRunBackgroundTimeout)
		runCancelers.Store(restoreRunID, cancel)
		defer func() {
			cancel()
			runCancelers.Delete(restoreRunID)
		}()

		sourceRun, err := s.runByID(ctx, sourceRunID)
		if err == nil && sourceRun != nil {
			var result *RestoreBackupResult
			result, err = s.restoreFromRun(ctx, restoreRunID, sourceRun, input)
			// Record what the archive actually yielded rather than what
			// was asked for: entries can be absent (a mount the source
			// backup skipped) and the overview must not claim they were
			// written back.
			if result != nil && len(result.Restored) > 0 {
				if itemErr := s.setRunItems(ctx, restoreRunID, result.Restored, result.Restorable); itemErr != nil && s.logger != nil {
					s.logger.Warn("container restore item record failed", "run", restoreRunID, "error", itemErr)
				}
			}
		}
		archive := archiveResult{}
		if _, finishErr := s.finishRun(ctx, restoreRunID, archive, err); finishErr != nil && s.logger != nil {
			s.logger.Error("background container restore failed", "run", restoreRunID, "source", sourceRunID, "error", finishErr)
		}
	}()
}

// restorePass runs one restore pass; tests replace it to observe the order.
func (s *Service) restorePass(ctx context.Context, progressRunID string, run *BackupRun, input RestoreBackupInput) (*RestoreBackupResult, error) {
	if s.restorePassHook != nil {
		return s.restorePassHook(ctx, progressRunID, run, input)
	}
	return s.restoreFromRun(ctx, progressRunID, run, input)
}

func (s *Service) restoreFromRun(ctx context.Context, progressRunID string, run *BackupRun, input RestoreBackupInput) (*RestoreBackupResult, error) {
	if run.Operation != "backup" || run.Status != "success" || strings.TrimSpace(run.ArchivePath) == "" {
		return nil, ErrBackupRunNotDownloadable
	}
	// Applying configuration recreates the container and drops its filesystem
	// layer, so a restore that wants both does it in two passes, in that order.
	if wanted := newRestorePlan(input.RestoreItems); !input.deferFilesystem &&
		wanted.wantsUnit(runItemConfig) && wanted.wantsUnit(runItemFilesystem) {
		return s.restoreConfigThenFilesystem(ctx, progressRunID, run, input, wanted)
	}
	if err := s.guardSelfRestoreFor(ctx, run.EnvironmentID, run.ContainerID, input.WorkflowSelfAuthorized); err != nil {
		return nil, err
	}
	if s.canRunAgentRestore(ctx, run, newRestorePlan(input.RestoreItems)) {
		return s.restoreFromRunViaAgentArchive(ctx, progressRunID, run, input)
	}
	if progressRunID != "" {
		s.updateRunProgress(ctx, progressRunID, "restore_scanning", "")
	}
	file, decrypted, err := s.openRestoreArchive(run, input.SecretKey)
	if err != nil {
		return nil, err
	}
	defer closeLogged(file, "backup archive")
	defer closeLogged(decrypted, "decrypted backup stream")

	if progressRunID != "" {
		s.updateRunProgress(ctx, progressRunID, "restore_connecting", "")
	}
	cli, err := s.client(ctx, run.EnvironmentID)
	if err != nil {
		return nil, err
	}
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	if progressRunID != "" {
		s.updateRunProgress(opCtx, progressRunID, "restore_inspecting", "")
	}
	if _, err := cli.ContainerInspect(opCtx, run.ContainerID, client.ContainerInspectOptions{}); err != nil {
		return nil, fmt.Errorf("inspecting container before restore: %w", err)
	}

	tr := tar.NewReader(decrypted)
	mountTargets := map[string]string{}
	restored := []string{}
	// Every entry this pipeline could have restored, recorded before the
	// selection filter. Without it a partial restore has no denominator and
	// the Backups overview cannot honestly call it partial — the source
	// backup run's own item list is not a substitute, since runs written
	// before migration 140 have none.
	restorable := []string{}
	var savedInspect []byte
	plan := newRestorePlan(input.RestoreItems)
	if input.deferFilesystem {
		plan = plan.without(runItemFilesystem)
	}
	recreatedID := ""
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
		case header.Name == "container/inspect.json":
			restorable = append(restorable, runItemConfig)
			if !plan.wantsUnit(runItemConfig) {
				continue
			}
			// Buffered, not applied here: recreating the container replaces it,
			// so config goes last — after any mounts in this same restore have
			// been written back.
			savedInspect, err = io.ReadAll(io.LimitReader(tr, restoreConfigInspectMaxBytes))
			if err != nil {
				return nil, fmt.Errorf("reading archived container config: %w", err)
			}
		case header.Name == "image/image.tar":
			restorable = append(restorable, "image")
			if !plan.wantsUnit("image") {
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
			restorable = append(restorable, "filesystem")
			if !plan.wantsUnit("filesystem") {
				continue
			}
			if progressRunID != "" {
				s.updateRunProgress(opCtx, progressRunID, "restore_filesystem", "")
			}
			if !plan.isWhole("filesystem") {
				applied, err := s.restoreSubsetEntry(opCtx, cli, run, progressRunID, "restore_filesystem", "Restoring selected files", header.Name, "/", tr, header.Size, plan.subsetPaths("filesystem"), input.SecretKey)
				if err != nil {
					return nil, fmt.Errorf("restoring selected filesystem entries: %w", err)
				}
				if applied {
					restored = append(restored, plan.subsetItems("filesystem")...)
				}
				continue
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
			unit := "mount:" + target
			restorable = append(restorable, unit)
			if !plan.wantsUnit(unit) {
				continue
			}
			if progressRunID != "" {
				s.updateRunProgress(opCtx, progressRunID, "restore_mounts", "Restoring mounted data "+target+".")
			}
			restoreTarget := filepath.Dir(target)
			if restoreTarget == "." {
				restoreTarget = "/"
			}
			if !plan.isWhole(unit) {
				applied, err := s.restoreSubsetEntry(opCtx, cli, run, progressRunID, "restore_mounts", "Restoring selected files in "+target, header.Name, restoreTarget, tr, header.Size, plan.subsetPaths(unit), input.SecretKey)
				if err != nil {
					return nil, fmt.Errorf("restoring selected mount entries: %w", err)
				}
				if applied {
					restored = append(restored, plan.subsetItems(unit)...)
				}
				continue
			}
			reader := s.restoreProgressReader(opCtx, progressRunID, "restore_mounts", "Restoring mounted data "+target, tr, header.Size)
			if err := s.copyRestoreEntryToContainer(opCtx, cli, run, progressRunID, "restore_mounts", "Restoring mounted data "+target, header.Name, restoreTarget, reader, header.Size, input.SecretKey); err != nil {
				return nil, fmt.Errorf("restoring mounted data: %w", err)
			}
			restored = append(restored, unit)
		}
	}
	// Config last: the recreate below replaces the container, so anything copied
	// into the old one's filesystem layer would be lost. Mounts survive because
	// they live outside that layer.
	if plan.wantsUnit(runItemConfig) && len(savedInspect) > 0 {
		applied, newID, err := s.restoreContainerConfig(opCtx, run, progressRunID, savedInspect, plan)
		if err != nil {
			return nil, err
		}
		restored = append(restored, applied...)
		recreatedID = newID
	}

	if len(restored) == 0 {
		return nil, ErrBackupRestoreNoRestorableEntries
	}

	return &RestoreBackupResult{RunID: run.ID, Restored: restored, Restorable: restorable, recreatedID: recreatedID}, nil
}

// restoreConfigThenFilesystem restores everything but the filesystem layer
// first — configuration last, which recreates the container — and then writes
// the filesystem layer into the recreated container. Done in one pass, the
// recreate would discard the layer just restored.
func (s *Service) restoreConfigThenFilesystem(ctx context.Context, progressRunID string, run *BackupRun, input RestoreBackupInput, wanted *restorePlan) (*RestoreBackupResult, error) {
	first := input
	first.deferFilesystem = true
	result, err := s.restorePass(ctx, progressRunID, run, first)
	if err != nil {
		return nil, err
	}

	target := *run
	if result.recreatedID != "" {
		target.ContainerID = result.recreatedID
	}
	second := input
	second.RestoreItems = wanted.filesystemItems()
	fsResult, err := s.restorePass(ctx, progressRunID, &target, second)
	if err != nil {
		return nil, fmt.Errorf("restoring the filesystem layer into the recreated container: %w", err)
	}
	result.Restored = append(result.Restored, fsResult.Restored...)
	return result, nil
}

func (s *Service) canRunAgentRestore(ctx context.Context, run *BackupRun, plan *restorePlan) bool {
	if run == nil || !s.pool.IsAgentEnvContext(ctx, run.EnvironmentID) || !s.pool.AgentAtLeastContext(ctx, run.EnvironmentID, agentBackupMinVersion) {
		return false
	}
	// Applying container config means recreating the container, which only the
	// server can orchestrate — the agent restore protocol has no such message.
	if plan != nil && plan.wantsUnit(runItemConfig) {
		return false
	}
	// A per-file selection needs an agent that knows how to filter. Older
	// agents would apply the whole mount, which is exactly the bug this
	// replaced, so route those restores through the server instead.
	if plan != nil && plan.hasSubset() && !s.pool.AgentAtLeastContext(ctx, run.EnvironmentID, agentSelectiveRestoreMinVersion) {
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
	agentConn, ok := s.pool.AgentConnectionContext(ctx, run.EnvironmentID)
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
	// The agent reports what it wrote back, not what the archive held. With
	// no selection it restores everything, so the two sets coincide and the
	// restored list doubles as the inventory. With a selection they do not,
	// and guessing would mislabel the run — leave it unknown instead.
	var restorable []string
	if len(input.RestoreItems) == 0 {
		restorable = result.Restored
	}
	return &RestoreBackupResult{RunID: run.ID, Restored: result.Restored, Restorable: restorable}, nil
}

func (s *Service) copyRestoreEntryToContainer(ctx context.Context, cli *client.Client, run *BackupRun, progressRunID, progressStage, label, entryName, targetPath string, reader io.Reader, size int64, secretKey string) error {
	if !s.pool.IsAgentEnvContext(ctx, run.EnvironmentID) {
		_, err := cli.CopyToContainer(ctx, run.ContainerID, client.CopyToContainerOptions{DestinationPath: targetPath, Content: reader, AllowOverwriteDirWithFile: false})
		return err
	}
	return s.copyRestoreEntryToContainerViaAgent(ctx, run, progressRunID, progressStage, label, entryName, targetPath, size, secretKey)
}

func (s *Service) copyRestoreEntryToContainerViaAgent(ctx context.Context, run *BackupRun, progressRunID, progressStage, label, entryName, targetPath string, size int64, secretKey string) error {
	agentConn, ok := s.pool.AgentConnectionContext(ctx, run.EnvironmentID)
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

// RestoreUploaded stores an uploaded encrypted archive and restores all of it to the
// selected container, in one call, for clients that have no selection step.
func (s *Service) RestoreUploaded(ctx context.Context, input RestoreUploadedBackupInput) (*RestoreBackupResult, error) {
	if strings.TrimSpace(input.EnvironmentID) == "" || strings.TrimSpace(input.ContainerID) == "" || input.Reader == nil {
		return nil, fmt.Errorf("uploaded backup restore input is invalid")
	}
	// Guarded before staging so a refused restore leaves no run and no archive behind.
	if err := s.guardSelfRestore(ctx, input.EnvironmentID, input.ContainerID); err != nil {
		return nil, err
	}

	run, err := s.StageUploaded(ctx, input)
	if err != nil {
		return nil, err
	}
	result, err := s.Restore(ctx, run.ID, RestoreBackupInput{SecretKey: strings.TrimSpace(input.SecretKey)})
	if err != nil {
		return nil, err
	}
	// No selection was passed, so everything the archive held was restored.
	if result != nil && len(result.Restored) > 0 {
		if itemErr := s.setRunItems(ctx, run.ID, result.Restored, result.Restorable); itemErr != nil && s.logger != nil {
			s.logger.Warn("uploaded backup item record failed", "run", run.ID, "error", itemErr)
		}
	}
	return result, nil
}

// StageUploaded stores an uploaded encrypted archive as a completed backup run without
// restoring anything, so the operator can pick what to restore from it like any other run.
//
// The archive is opened with the supplied key before the run is marked successful: a wrong
// key or an unreadable file fails the upload here, rather than surfacing later as an empty
// selection list in the restore dialog.
func (s *Service) StageUploaded(ctx context.Context, input RestoreUploadedBackupInput) (*BackupRun, error) {
	if strings.TrimSpace(input.EnvironmentID) == "" || strings.TrimSpace(input.ContainerID) == "" || input.Reader == nil {
		return nil, fmt.Errorf("uploaded backup input is invalid")
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

	// Best-effort name for the run row: an unreachable orchestrator
	// must not block a restore, so an empty name is acceptable here —
	// the list path resolves it later.
	uploadedName, nameErr := s.containerName(ctx, input.EnvironmentID, input.ContainerID)
	if nameErr != nil && s.logger != nil {
		s.logger.Debug("uploaded restore: container name lookup failed",
			"env", input.EnvironmentID, "container", input.ContainerID, "error", nameErr)
	}

	// An uploaded archive advertises nothing up front — its contents are
	// only known once it has been unpacked, so the item lists are stamped
	// on after the archive has been read below.
	run, err := s.createRunWithOrigin(ctx, "", input.EnvironmentID, input.ContainerID, uploadedName, RunOriginUpload, nil, nil)
	if err != nil {
		return nil, err
	}

	archive.path, archive.size, err = s.storeUploadedArchive(run.ID, input.Reader)
	if err == nil {
		var available []string
		available, err = s.uploadedArchiveItems(run.ID, archive, secretKey)
		if err == nil {
			run, err = s.finishRun(ctx, run.ID, archive, nil)
			if err != nil {
				return nil, err
			}
			if itemErr := s.setRunItems(ctx, run.ID, available, available); itemErr != nil && s.logger != nil {
				s.logger.Warn("uploaded backup item record failed", "run", run.ID, "error", itemErr)
			}
			// Re-read so the response carries the items just stamped; the run is
			// staged either way, so a failed re-read is logged, not returned.
			staged, readErr := s.Run(ctx, run.ID)
			if readErr != nil && s.logger != nil {
				s.logger.Warn("uploaded backup re-read failed", "run", run.ID, "error", readErr)
			}
			if staged != nil {
				return staged, nil
			}
			return run, nil
		}
	}

	if cleanupErr := s.deleteRunArchive(ctx, run.ID, archive.path); cleanupErr != nil && s.logger != nil {
		s.logger.Warn("uploaded backup cleanup failed", "run", run.ID, "error", cleanupErr)
	}
	if _, finishErr := s.finishRun(ctx, run.ID, archive, err); finishErr != nil && s.logger != nil {
		s.logger.Warn("uploaded backup failure finalization failed", "run", run.ID, "error", finishErr)
	}
	return nil, err
}

// uploadedArchiveItems opens a just-stored upload with the key it will be restored with
// and lists its item keys, which proves the key and the tar before the run is offered.
func (s *Service) uploadedArchiveItems(runID string, archive archiveResult, secretKey string) ([]string, error) {
	probe := &BackupRun{
		ID:                runID,
		ArchivePath:       archive.path,
		ArchiveEncryption: archive.encryption,
		ArchiveKeyID:      archive.keyID,
	}
	file, decrypted, err := s.openRestoreArchive(probe, secretKey)
	if err != nil {
		return nil, err
	}
	defer closeLogged(file, "uploaded backup archive")
	defer closeLogged(decrypted, "decrypted uploaded backup stream")

	options, err := restoreArchiveOptions(decrypted)
	if err != nil {
		return nil, err
	}
	if len(options) == 0 {
		return nil, ErrBackupRestoreNoRestorableEntries
	}
	items := make([]string, 0, len(options))
	for _, option := range options {
		items = append(items, option.Key)
	}
	return items, nil
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
		case header.Name == "container/inspect.json":
			raw, readErr := io.ReadAll(io.LimitReader(tr, header.Size))
			if readErr != nil {
				return nil, fmt.Errorf("reading inspect.json: %w", readErr)
			}
			rootItems, _, cerr := enumerateConfigChildrenAt(raw, "")
			if cerr != nil {
				return nil, cerr
			}
			// The synthetic `config` row holds ParentMount="config" so
			// the picker can locate the subtree under it; subsequent
			// folder rows keep ParentMount="config" too.
			for i := range rootItems {
				if rootItems[i].Key == "config:Config" {
					rootItems[i].ParentMount = "config"
					rootItems[i].EntryKind = "folder"
				} else if rootItems[i].Type == "folder" || rootItems[i].Type == "file" {
					rootItems[i].ParentMount = "config"
				}
			}
			items = append(items, rootItems...)
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
			// Inline-emit children of the mount so the picker has
			// something to drill into without a second round-trip.
			// Expand one level at the root so the operator sees nested
			// entries (e.g. `data/sub/`) immediately.
			innerBytes, readErr := io.ReadAll(io.LimitReader(tr, header.Size))
			if readErr != nil {
				return nil, fmt.Errorf("reading mount %s: %w", header.Name, readErr)
			}
			expanded, truncated, cerr := enumerateMountChildrenRecursive(tar.NewReader(bytes.NewReader(innerBytes)), target, "", restoreOptionsMaxMountEntries)
			if cerr != nil {
				return nil, cerr
			}
			// Attach truncated flag to the parent mount row.
			for i := range items {
				if items[i].Key == "mount:"+target {
					items[i].Truncated = truncated
					break
				}
			}
			items = append(items, expanded...)
		}
	}
	return items, nil
}

// restoreChildrenCap is the maximum number of immediate children enumerated
// from any one parent path in a tree-listing endpoint (filesystem/mounts/
// config). Larger directories surface a `truncated` flag so the UI can show
// a "more items not shown" hint instead of streaming the whole archive.
const restoreChildrenCap = 200

// restoreOptionsMaxMountEntries caps how many mount entries the
// restore-options response will enumerate per mount tar. Anything
// larger is truncated with Truncated=true and the operator must drill
// into the mount subtree via the lazy-load endpoint to see the rest.
const restoreOptionsMaxMountEntries = 1000

// enumerateTarChildren walks an outer-tar reader until the entry named
// `outerEntry` is found, then streams the nested tar inside that entry and
// returns the first-level children of `innerPath` (slash-delimited, "" = root).
// `wireTarget` is the canonical selection target stamped onto each row's Key
// and ParentMount — the real mount path (e.g. "/data") for a mount tar, and
// the literal "filesystem" for the container filesystem layer.
// The returned slice contains one BackupOption per directory or regular file;
// other tar types (symlinks, devices, fifos, hardlinks) are skipped. `truncated`
// is true when more siblings would have followed `limit`.
func enumerateTarChildren(decrypted io.Reader, outerEntry string, wireTarget string, innerPath string, limit int) (items []BackupOption, truncated bool, err error) {
	if limit <= 0 {
		limit = restoreChildrenCap
	}
	items = []BackupOption{}
	outer := tar.NewReader(decrypted)
	for {
		h, err := outer.Next()
		if errors.Is(err, io.EOF) {
			return nil, false, fmt.Errorf("outer archive does not contain %s", outerEntry)
		}
		if err != nil {
			return nil, false, err
		}
		if h == nil || h.Typeflag != tar.TypeReg || h.Name != outerEntry {
			continue
		}
		inner := tar.NewReader(io.LimitReader(outer, h.Size))
		prefix := strings.Trim(innerPath, "/")
		if prefix != "" {
			prefix += "/"
		}
		seen := map[string]struct{}{}
		for {
			innerHeader, innerErr := inner.Next()
			if errors.Is(innerErr, io.EOF) {
				break
			}
			if innerErr != nil {
				return items, truncated, innerErr
			}
			if innerHeader == nil {
				continue
			}
			// Only descend into entries under the requested prefix.
			if prefix != "" && !strings.HasPrefix(innerHeader.Name, prefix) {
				if err := skipTarEntry(innerHeader, inner); err != nil {
					return items, truncated, err
				}
				continue
			}
			// Strip the prefix and take the first path segment.
			rel := strings.TrimPrefix(innerHeader.Name, prefix)
			if rel == "" || strings.HasPrefix(rel, "./") {
				if err := skipTarEntry(innerHeader, inner); err != nil {
					return items, truncated, err
				}
				continue
			}
			seg := rel
			if idx := strings.Index(seg, "/"); idx >= 0 {
				seg = seg[:idx]
			}
			if seg == "" {
				if err := skipTarEntry(innerHeader, inner); err != nil {
					return items, truncated, err
				}
				continue
			}
			if _, ok := seen[seg]; ok {
				if err := skipTarEntry(innerHeader, inner); err != nil {
					return items, truncated, err
				}
				continue
			}
			seen[seg] = struct{}{}
			// A segment is a folder when its own header says so, or when
			// the header path continues past the segment. A tar that omits
			// directory headers still resolves correctly because the deeper
			// entry carries `<seg>/...`. Do not read ahead here: every
			// header consumed would be dropped without becoming a row.
			isFolder := innerHeader.Typeflag == tar.TypeDir || strings.HasPrefix(rel, seg+"/")
			childPath := strings.Trim(innerPath, "/")
			if childPath != "" {
				childPath += "/"
			}
			childPath += seg
			optType := "file"
			size := innerHeader.Size
			if isFolder {
				optType = "folder"
				size = 0
			}
			// Path/EntryKind/ParentMount/Size are load-bearing: the picker
			// feeds Path back as the next innerPath, so omitting it makes
			// every drill-down re-request the root listing.
			items = append(items, BackupOption{
				Key:         buildTreeWireKey(wireTarget, childPath, optType),
				Type:        optType,
				EntryKind:   optType,
				Label:       seg,
				Path:        childPath,
				ParentMount: wireTarget,
				Size:        size,
				Description: "",
				Default:     false,
				Required:    false,
			})
			if err := skipTarEntry(innerHeader, inner); err != nil {
				return items, truncated, err
			}
			if len(items) >= limit {
				truncated = true
				return items, truncated, nil
			}
		}
		return items, false, nil
	}
}

// buildTreeWireKey returns the canonical restoreItems key for a tree leaf or
// folder, mirroring the frontend convention in RestoreFileTree.tsx:
//
//	folder:<outerEntry>:<path>     for folders
//	file:<outerEntry>:<path>       for files
//
// where `<outerEntry>` is "filesystem" for the container filesystem and the
// mount target (e.g. "/data") for a per-mount tree.
func buildTreeWireKey(outerEntry, innerPath, optType string) string {
	cleanOuter := strings.TrimSuffix(outerEntry, ".tar")
	// A mount is named by its target, "/data", in every key the restore reads
	// (splitRestoreItem, splitMountPath, the agent's plan). The listings pass
	// "mounts/data", with or without ".tar"; left as is, a folder ticked inside
	// a mount named no mount at all and restored nothing from it.
	if rest, ok := strings.CutPrefix(cleanOuter, "mounts/"); ok {
		cleanOuter = "/" + strings.TrimPrefix(rest, "/")
	}
	if cleanOuter == "container/filesystem" || cleanOuter == "filesystem" {
		cleanOuter = "filesystem"
	}
	prefix := "file:"
	if optType == "folder" {
		prefix = "folder:"
	}
	return prefix + cleanOuter + ":" + strings.TrimLeft(innerPath, "/")
}

// enumerateMountChildrenAt returns the immediate children of `innerPath`
// inside the inner mount tar held by `mountReader`. The reader is
// expected to be positioned at the start of the inner mount tar (i.e.,
// the caller has already advanced the outer archive to the matching
// `mounts/<safeTarget>.tar` entry). Each entry is rendered as a
// `BackupOption` with `Path`/`ParentMount`/`Size` populated for the
// tree view; folder rows carry `Type="folder"`, file rows carry
// `Type="file"`.
//
// innerPath == "" yields top-level entries. Otherwise the function
// returns the children whose tar name has `innerPath` as a parent
// segment.
func enumerateMountChildrenAt(mountReader *tar.Reader, mountTarget, innerPath string) ([]BackupOption, bool, error) {
	items := []BackupOption{}
	truncated := false
	seen := map[string]bool{}
	if mountReader == nil {
		return items, false, nil
	}
	prefix := ""
	if innerPath != "" {
		prefix = strings.TrimRight(innerPath, "/") + "/"
	}
	for {
		hdr, err := mountReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false, err
		}
		if hdr == nil {
			continue
		}
		name := strings.TrimRight(hdr.Name, "/")
		if innerPath != "" {
			if !strings.HasPrefix(name+"/", prefix) && name != innerPath {
				continue
			}
			if name == innerPath {
				continue
			}
		} else if strings.Contains(name, "/") {
			// Top-level entry: extract first segment.
			parts := strings.SplitN(name, "/", 2)
			name = parts[0]
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		isDir := hdr.Typeflag == tar.TypeDir
		if len(items) >= restoreOptionsMaxMountEntries {
			truncated = true
			continue
		}
		if isDir {
			items = append(items, BackupOption{
				Key:         buildTreeWireKey("mounts/"+strings.TrimPrefix(mountTarget, "/"), name, "folder"),
				Type:        "folder",
				EntryKind:   "folder",
				Label:       name,
				Path:        name,
				ParentMount: mountTarget,
			})
		} else {
			items = append(items, BackupOption{
				Key:         buildTreeWireKey("mounts/"+strings.TrimPrefix(mountTarget, "/"), name, "file"),
				Type:        "file",
				EntryKind:   "file",
				Label:       name,
				Path:        name,
				ParentMount: mountTarget,
				Size:        hdr.Size,
			})
		}
	}
	return items, truncated, nil
}

// enumerateMountChildrenRecursive walks a mount tar and emits every
// entry whose path starts with `innerPath`. Unlike enumerateMountChildrenAt
// (which only lists immediate children), this variant surfaces files
// at any depth inside `innerPath` so the picker shows nested entries
// inline. Folder keys end with `/`, file keys do not. `limit` bounds the
// total row count; once hit the rest is dropped and `truncated` is set
// true.
func enumerateMountChildrenRecursive(mountReader *tar.Reader, mountTarget, innerPath string, limit int) ([]BackupOption, bool, error) {
	items := []BackupOption{}
	truncated := false
	if mountReader == nil {
		return items, false, nil
	}
	prefix := ""
	if innerPath != "" {
		prefix = strings.TrimRight(innerPath, "/") + "/"
	}
	for {
		hdr, err := mountReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false, err
		}
		if hdr == nil {
			continue
		}
		name := strings.TrimRight(hdr.Name, "/")
		if name == "" {
			continue
		}
		var rel string
		switch {
		case innerPath == "":
			rel = name
		case name == innerPath:
			continue
		case strings.HasPrefix(name+"/", prefix):
			rel = strings.TrimPrefix(name, prefix)
		default:
			continue
		}
		if rel == "" {
			continue
		}
		if len(items) >= limit {
			truncated = true
			continue
		}
		isDir := hdr.Typeflag == tar.TypeDir
		if isDir {
			items = append(items, BackupOption{
				Key:         buildTreeWireKey("mounts/"+strings.TrimPrefix(mountTarget, "/"), rel+"/", "folder"),
				Type:        "folder",
				EntryKind:   "folder",
				Label:       pathBase(rel),
				Path:        rel,
				ParentMount: mountTarget,
			})
		} else {
			items = append(items, BackupOption{
				Key:         buildTreeWireKey("mounts/"+strings.TrimPrefix(mountTarget, "/"), rel, "file"),
				Type:        "file",
				EntryKind:   "file",
				Label:       pathBase(rel),
				Path:        rel,
				ParentMount: mountTarget,
				Size:        hdr.Size,
			})
		}
	}
	return items, truncated, nil
}

// pathBase returns the final segment of a slash-delimited path.
func pathBase(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// decodeRestoreMountTarget reverses the URL encoding chi leaves in place
// for path parameters. Without this, a multi-segment mount like
// `/var/www/html/data` would arrive as `%2Fvar%2Fwww%2Fhtml%2Fdata`
// and never match the manifest's selectedMounts list.
func decodeRestoreMountTarget(s string) string {
	if s == "" {
		return s
	}
	decoded, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

// precomputeWantedFilesByMount builds two per-mount maps from the
// operator's selection list:
//   - files:   mountTarget -> set of file paths the operator wants
//   - folders: mountTarget -> set of folder paths the operator wants
//
// Both prefixes (`file:` and `folder:`) are honored so the picker
// treats either as a positive selection. A `file:` key with a trailing
// slash is treated as a folder selection (some pickers emit that shape).
// `mount:<target>` keys contribute the whole mount (its full set, no
// per-file filtering).
func precomputeWantedFilesByMount(items []string) (files, folders map[string]map[string]bool) {
	files = map[string]map[string]bool{}
	folders = map[string]map[string]bool{}
	for _, raw := range items {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		asFolder := false
		switch {
		case strings.HasPrefix(key, "file:"):
			rest := strings.TrimPrefix(key, "file:")
			mount, path, ok := splitMountPath(rest)
			if !ok {
				continue
			}
			if strings.HasSuffix(path, "/") {
				asFolder = true
			}
			path = strings.TrimRight(path, "/")
			if path == "" {
				continue
			}
			if asFolder {
				if folders[mount] == nil {
					folders[mount] = map[string]bool{}
				}
				folders[mount][path] = true
			} else {
				if files[mount] == nil {
					files[mount] = map[string]bool{}
				}
				files[mount][path] = true
			}
		case strings.HasPrefix(key, "folder:"):
			rest := strings.TrimPrefix(key, "folder:")
			mount, path, ok := splitMountPath(rest)
			if !ok {
				continue
			}
			path = strings.TrimRight(path, "/")
			if path == "" {
				continue
			}
			if folders[mount] == nil {
				folders[mount] = map[string]bool{}
			}
			folders[mount][path] = true
		}
	}
	return files, folders
}

// splitMountPath accepts wire keys of the form `<target>:<path>` where
// target starts with `/`. Returns mount target, inner path, ok.
func splitMountPath(s string) (string, string, bool) {
	idx := strings.IndexByte(s, ':')
	if idx <= 0 {
		return "", "", false
	}
	mount := s[:idx]
	path := s[idx+1:]
	if !strings.HasPrefix(mount, "/") {
		return "", "", false
	}
	return mount, path, true
}

// innerPathMatchesWanted reports whether a tar entry at `path` should
// be kept because it matches one of the operator's file selections
// directly, or because it lives under one of the selected folders.
func innerPathMatchesWanted(path string, files, folders map[string]bool) bool {
	if files[path] {
		return true
	}
	for folder := range folders {
		if path == folder {
			return true
		}
		if strings.HasPrefix(path, strings.TrimRight(folder, "/")+"/") {
			return true
		}
	}
	return false
}

// hasWantedDescendant reports whether any selected file or folder is
// nested under `path`. Used by filterMountTarEntries to keep ancestor
// directories in the rebuilt tar so the resulting tree is structurally
// valid for downstream extractors.
func hasWantedDescendant(path string, files, folders map[string]bool) bool {
	base := strings.TrimRight(path, "/") + "/"
	for f := range files {
		if strings.HasPrefix(f, base) {
			return true
		}
	}
	for f := range folders {
		if strings.HasPrefix(f, base) {
			return true
		}
	}
	return false
}

// filterMountTarEntries reads `mountReader`, copies every entry whose
// path matches the operator's wanted set into a fresh tar buffer, and
// returns the rebuilt bytes plus the total payload size. Entries that
// don't match are dropped — useful when the operator picks a subset
// of files inside a mount tar (e.g. `data/config.php` only). Ancestor
// directories of any kept entry are also kept so the resulting tar is
// structurally valid for downstream extractors.
func filterMountTarEntries(mountReader *tar.Reader, files, folders map[string]bool) (*bytes.Buffer, int64, error) {
	out := &bytes.Buffer{}
	tw := tar.NewWriter(out)
	var total int64
	// The caller passes an already-streaming *tar.Reader (the inner
	// `mounts/<target>.tar` extracted from the outer archive). Wrapping
	// it in tar.NewReader would feed it entry bodies as tar headers and
	// silently produce an empty output — use the reader directly.
	tr := mountReader
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		if hdr == nil {
			continue
		}
		cleanName := strings.TrimRight(hdr.Name, "/")
		if cleanName == "" {
			continue
		}
		matched := innerPathMatchesWanted(cleanName, files, folders)
		if !matched {
			if hasWantedDescendant(cleanName, files, folders) {
				matched = true
			}
		}
		if !matched {
			if err := skipTarEntry(hdr, tr); err != nil {
				return nil, 0, err
			}
			continue
		}
		newHdr := *hdr
		newHdr.Name = cleanName
		if err := tw.WriteHeader(&newHdr); err != nil {
			return nil, 0, err
		}
		if hdr.Size > 0 {
			n, err := io.Copy(tw, tr)
			if err != nil {
				return nil, 0, err
			}
			total += n
		}
	}
	if err := tw.Close(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// enumerateConfigChildrenAt walks a marshaled ContainerJSON (or just the
// Config subtree) and returns synthetic BackupOption rows for the requested
// parentKey. Signature and semantics are pinned by config_children_test.go.
//
//	parentKey == "" (or "config")
//	  -> synthetic "config:Config" folder + one row per top-level field
//	     (array/object = folder, scalar = file)
//	parentKey == "config:Config.<Field>"
//	  -> drill into that field:
//	     * object: one file row per key
//	     * array:  one file row per element (Env is special-cased to
//	               split on "=" and emit a row per NAME, dropping
//	               malformed entries like "EMPTY=", "=leadingeq", "BAD")
//	     * scalar: error "unknown config parent key" (defensive; the
//	               frontend should never ask for leaves under a scalar)
//	parentKey == anything else
//	  -> error "unknown config parent key" so the backend surfaces a
//	     500 instead of returning []
func enumerateConfigChildrenAt(inspect []byte, parentKey string) ([]BackupOption, bool, error) {
	items := []BackupOption{}
	truncated := false
	if len(inspect) == 0 {
		return nil, false, errors.New("empty inspect payload")
	}
	// Accept either a full ContainerJSON (`{"Config":{...}}`) or the bare
	// Config subtree. Detect by checking for a top-level "Config" key.
	var root map[string]json.RawMessage
	if err := json.Unmarshal(inspect, &root); err != nil {
		return nil, false, err
	}
	var config map[string]json.RawMessage
	if raw, ok := root["Config"]; ok {
		if err := json.Unmarshal(raw, &config); err != nil {
			return nil, false, err
		}
	} else {
		config = root
	}
	if parentKey == "" || parentKey == "config" {
		items = append(items, BackupOption{
			Key: "config", Type: "config", Label: "Config",
			EntryKind:   "folder",
			Description: "Container configuration saved in this backup.",
		})
		// Walk canonical keys first so the picker renders a stable
		// shape even when the archive lacks data for some fields.
		emitted := map[string]bool{}
		for _, k := range canonicalConfigKeys() {
			val := config[k]
			item, err := configFieldItem(configRootKey(k), k, val)
			if err != nil {
				return nil, false, err
			}
			if len(items) >= restoreChildrenCap {
				truncated = true
				continue
			}
			items = append(items, item)
			emitted[k] = true
		}
		for _, k := range sortedKeys(config) {
			if emitted[k] {
				continue
			}
			val := config[k]
			item, err := configFieldItem(configRootKey(k), k, val)
			if err != nil {
				return nil, false, err
			}
			if len(items) >= restoreChildrenCap {
				truncated = true
				continue
			}
			items = append(items, item)
		}
		return items, truncated, nil
	}
	// Drill: parentKey is "config:Config.<Field>".
	fieldName := strings.TrimPrefix(parentKey, "config:")
	fieldName = strings.TrimPrefix(fieldName, "Config.")
	if fieldName == "" {
		return nil, false, fmt.Errorf("unknown config parent key %q", parentKey)
	}
	val, ok := config[fieldName]
	if !ok {
		// A canonical field the archive carries no value for (Volumes and Args
		// are routinely absent or null). The picker renders every canonical
		// field as a folder, so it will ask for these; an empty child list is
		// the honest answer, not a 500.
		if configFolderKeys[fieldName] || configScalarKeys[fieldName] {
			return []BackupOption{}, false, nil
		}
		return nil, false, fmt.Errorf("unknown config parent key %q", parentKey)
	}
	if isNullOrEmptyJSON(val) {
		return []BackupOption{}, false, nil
	}
	if err := configChildrenForField(&items, parentKey, fieldName, val, &truncated); err != nil {
		return nil, false, err
	}
	return items, truncated, nil
}

// configRootKey returns the wire key for a top-level Config field
// ("config:Config.<field>").
// isNullOrEmptyJSON reports whether a raw config value carries nothing to
// enumerate — absent, JSON null, or an empty object/array.
func isNullOrEmptyJSON(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	switch trimmed {
	case "", "null", "{}", "[]":
		return true
	}
	return false
}

func configRootKey(field string) string {
	return "config:Config." + field
}

// configFolderKeys is the canonical set of Container Config keys that
// always render as folders (object/array values) in the picker. When
// the archive carries no value for a key, the picker still surfaces a
// folder row so the operator sees a stable shape across backups made
// with different IncludeConfig sub-options.
var configFolderKeys = map[string]bool{
	"Env":          true,
	"Cmd":          true,
	"Entrypoint":   true,
	"Args":         true,
	"Labels":       true,
	"ExposedPorts": true,
	"Volumes":      true,
}

// configScalarKeys is the canonical set of scalar keys. Mirrors
// configFolderKeys.
var configScalarKeys = map[string]bool{
	"WorkingDir": true,
	"User":       true,
	"Image":      true,
}

// canonicalConfigKeys returns the canonical Container Config keys in
// stable order so the picker renders identically across backups that
// happened to omit some sub-fields.
func canonicalConfigKeys() []string {
	return []string{
		"Env", "Cmd", "Entrypoint", "Args", "Labels", "ExposedPorts", "Volumes",
		"WorkingDir", "User", "Image",
	}
}

// configFieldItem returns the row for a single top-level Config field.
// The type is decided by the canonical set when the key is known, or
// by the JSON shape when the key is unknown.
func configFieldItem(key, label string, raw json.RawMessage) (BackupOption, error) {
	if configFolderKeys[label] {
		return BackupOption{Key: key, Type: "folder", Label: label}, nil
	}
	if configScalarKeys[label] {
		return BackupOption{Key: key, Type: "file", Label: label}, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return BackupOption{Key: key, Type: "file", Label: label}, nil
	}
	switch trimmed[0] {
	case '{', '[':
		return BackupOption{Key: key, Type: "folder", Label: label}, nil
	}
	return BackupOption{Key: key, Type: "file", Label: label}, nil
}

// configChildrenForField appends rows for the children of `parentKey`
// when parentKey refers to one specific Config field. The semantics
// match the test expectations:
//
//   - object: one row per key (label = key name)
//   - array:  one row per element; Env is special-cased to split on "="
//     and emit a row per NAME, dropping malformed entries
//   - scalar: error "unknown config parent key"
func configChildrenForField(items *[]BackupOption, parentKey, label string, raw json.RawMessage, truncated *bool) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return fmt.Errorf("unknown config parent key %q", parentKey)
	}
	switch trimmed[0] {
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return fmt.Errorf("unknown config parent key %q", parentKey)
		}
		for _, k := range sortedKeys(obj) {
			if len(*items) >= restoreChildrenCap {
				*truncated = true
				continue
			}
			*items = append(*items, BackupOption{
				Key:   parentKey + "." + k,
				Type:  "file",
				Label: k,
			})
		}
		return nil
	case '[':
		if label == "Env" {
			var strs []string
			if err := json.Unmarshal(raw, &strs); err != nil {
				return fmt.Errorf("unknown config parent key %q", parentKey)
			}
			for _, entry := range strs {
				eq := strings.IndexByte(entry, '=')
				if eq <= 0 {
					// Malformed (e.g. "", "=leadingeq", "BAD") — drop.
					continue
				}
				name := entry[:eq]
				value := entry[eq+1:]
				if name == "" || value == "" {
					// Drop empty-name and empty-value entries.
					continue
				}
				if len(*items) >= restoreChildrenCap {
					*truncated = true
					continue
				}
				*items = append(*items, BackupOption{
					Key:   parentKey + "." + name,
					Type:  "file",
					Label: name,
				})
			}
			return nil
		}
		// Generic array: one file leaf per element.
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return fmt.Errorf("unknown config parent key %q", parentKey)
		}
		for i := range arr {
			if len(*items) >= restoreChildrenCap {
				*truncated = true
				continue
			}
			name := fmt.Sprintf("%s[%d]", label, i)
			*items = append(*items, BackupOption{
				Key:   parentKey + "." + name,
				Type:  "file",
				Label: name,
			})
		}
		return nil
	}
	return fmt.Errorf("unknown config parent key %q", parentKey)
}

// sortedKeys returns the keys of m in sorted order, skipping empty strings.
func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		if k == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// FilesystemEntriesForRestore returns the first-level children of `innerPath`
// inside `container/filesystem.tar` for the named backup run.
func (s *Service) FilesystemEntriesForRestore(ctx context.Context, runID, innerPath, secretKey string) ([]BackupOption, bool, int, error) {
	run, err := s.runByID(ctx, strings.TrimSpace(runID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, restoreChildrenCap, nil
	}
	if err != nil {
		return nil, false, restoreChildrenCap, err
	}
	if run == nil || run.Operation != "backup" || run.Status != "success" || strings.TrimSpace(run.ArchivePath) == "" {
		return nil, false, restoreChildrenCap, ErrBackupRunNotDownloadable
	}
	file, decrypted, err := s.openRestoreArchive(run, secretKey)
	if err != nil {
		return nil, false, restoreChildrenCap, err
	}
	defer closeLogged(file, "backup archive")
	defer closeLogged(decrypted, "decrypted backup stream")
	items, truncated, err := enumerateTarChildren(decrypted, "container/filesystem.tar", "filesystem", innerPath, restoreChildrenCap)
	return items, truncated, restoreChildrenCap, err
}

// MountEntriesForRestore returns the first-level children of `innerPath`
// inside `mounts/<safeTarget>.tar` for the named backup run.
func (s *Service) MountEntriesForRestore(ctx context.Context, runID, mountTarget, innerPath, secretKey string) ([]BackupOption, bool, int, error) {
	run, err := s.runByID(ctx, strings.TrimSpace(runID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, restoreChildrenCap, nil
	}
	if err != nil {
		return nil, false, restoreChildrenCap, err
	}
	if run == nil || run.Operation != "backup" || run.Status != "success" || strings.TrimSpace(run.ArchivePath) == "" {
		return nil, false, restoreChildrenCap, ErrBackupRunNotDownloadable
	}
	file, decrypted, err := s.openRestoreArchive(run, secretKey)
	if err != nil {
		return nil, false, restoreChildrenCap, err
	}
	defer closeLogged(file, "backup archive")
	defer closeLogged(decrypted, "decrypted backup stream")
	entry := backupMountEntryName(mountTarget)
	items, truncated, err := enumerateTarChildren(decrypted, entry, mountTarget, innerPath, restoreChildrenCap)
	if err != nil {
		// Distinguish missing-entry inside the archive from other errors.
		if strings.Contains(err.Error(), "outer archive does not contain") {
			return nil, false, restoreChildrenCap, ErrBackupRestoreMountMissing
		}
		return nil, false, restoreChildrenCap, err
	}
	return items, truncated, restoreChildrenCap, nil
}

// ConfigEntriesForRestore returns the synthetic config tree rows under the
// named parentKey for the named backup run.
func (s *Service) ConfigEntriesForRestore(ctx context.Context, runID, parentKey, secretKey string) ([]BackupOption, bool, int, error) {
	run, err := s.runByID(ctx, strings.TrimSpace(runID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, restoreChildrenCap, nil
	}
	if err != nil {
		return nil, false, restoreChildrenCap, err
	}
	if run == nil || run.Operation != "backup" || run.Status != "success" || strings.TrimSpace(run.ArchivePath) == "" {
		return nil, false, restoreChildrenCap, ErrBackupRunNotDownloadable
	}
	file, decrypted, err := s.openRestoreArchive(run, secretKey)
	if err != nil {
		return nil, false, restoreChildrenCap, err
	}
	defer closeLogged(file, "backup archive")
	defer closeLogged(decrypted, "decrypted backup stream")
	tr := tar.NewReader(decrypted)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, false, restoreChildrenCap, ErrBackupRestoreConfigMissing
		}
		if err != nil {
			return nil, false, restoreChildrenCap, err
		}
		if h == nil || h.Typeflag != tar.TypeReg {
			continue
		}
		if h.Name != "container/inspect.json" {
			if err := skipTarEntry(h, tr); err != nil {
				return nil, false, restoreChildrenCap, err
			}
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(tr, h.Size))
		if err != nil {
			return nil, false, restoreChildrenCap, err
		}
		items, truncated, err := enumerateConfigChildrenAt(raw, parentKey)
		if err != nil {
			return nil, false, restoreChildrenCap, err
		}
		return items, truncated, restoreChildrenCap, nil
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
	defer closeLogged(resp, "image load response")
	if _, err := io.Copy(io.Discard, resp); err != nil {
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
	if strings.TrimSpace(run.ContainerName) != "" {
		return run.ContainerName
	}
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
	if errors.Is(err, sql.ErrNoRows) {
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

// resolveBackupEnvironmentSegment returns a path-safe segment for the
// given environment. The backup destination layout always includes an
// environment segment between the storage base path and the
// `containers/<containerName>/` directory so a container named the
// same thing on two environments doesn't collide. The slug falls
// back to the environment id when the environment row is missing,
// the name is empty, or the name slugs to an empty string after
// sanitization — that way a transient database error or a renamed
// environment never blocks a backup from uploading.
func (s *Service) resolveBackupEnvironmentSegment(ctx context.Context, envID string) (string, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return "_unknown-environment", nil
	}
	name, err := s.downloadEnvironmentName(ctx, envID)
	if err != nil {
		return envID, err
	}
	return services.SlugifyOr(name, envID), nil
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
	cli, err := s.client(ctx, plan.EnvironmentID)
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

	// Freeze the filesystem first when the plan asks for it. A snapshot
	// that yields readable paths is what makes the pause unnecessary:
	// the archive reads a frozen copy while the container keeps
	// serving. Anything less (no snapshot config, driver unreachable,
	// a driver that cannot expose a path) falls back to pausing, which
	// is always correct and never silent.
	frozen := s.freezeForBackup(opCtx, plan, runID)
	defer func() {
		// A fresh context: the snapshot must be released even when the
		// backup itself was cancelled.
		releaseCtx, releaseCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer releaseCancel()
		s.releaseSnapshot(releaseCtx, plan, runID, frozen)
	}()

	// Pause the container for the duration of the archive so the writable
	// layer, mount copies, and log read are point-in-time consistent.
	// Pause is best-effort: some runtimes (Windows containers, certain
	// custom runtimes) don't support it. If pause fails we log and
	// proceed without it.
	// "live" is the operator saying they accept a torn copy rather than
	// any downtime, so it never pauses even though no snapshot was taken.
	paused := false
	switch {
	case frozen.pauseFree():
	case plan.ConsistencyMode == ConsistencyLive:
		if s.logger != nil {
			s.logger.Info("container backup: live consistency mode, container left running",
				"container", plan.ContainerID, "run", runID)
		}
	default:
		paused = pauseContainerForBackup(opCtx, cli, plan.ContainerID, s.logger)
	}
	defer func() {
		// Use a fresh background context for unpause so we still
		// recover the container if the opCtx was cancelled.
		unpauseCtx, unpauseCancel := context.WithTimeout(context.WithoutCancel(ctx), coredocker.CallTimeout)
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
	defer closeLogged(file, "backup archive")

	encryptedWriter, metadata, err := s.backupCrypto.EncryptWriter(file)
	if err != nil {
		return archiveResult{path: archivePath}, err
	}

	s.updateRunProgress(opCtx, runID, "writing", "")
	tw := tar.NewWriter(encryptedWriter)
	writeErr := s.writeArchiveEntries(opCtx, cli, tw, plan, info, runID, frozen)
	s.updateRunProgress(opCtx, runID, "finalizing", "")
	closeErr := tw.Close()
	if writeErr != nil {
		closeLogged(encryptedWriter, "encrypted backup stream")
		return archiveResult{path: archivePath, encryption: metadata.Algorithm, keyID: metadata.KeyID}, writeErr
	}
	if closeErr != nil {
		closeLogged(encryptedWriter, "encrypted backup stream")
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
	if _, err := cli.ContainerPause(ctx, containerID, client.ContainerPauseOptions{}); err != nil {
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
	if _, err := cli.ContainerUnpause(ctx, containerID, client.ContainerUnpauseOptions{}); err != nil {
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

func (s *Service) writeArchiveEntries(ctx context.Context, cli *client.Client, tw *tar.Writer, plan *BackupPlan, info container.InspectResponse, runID string, frozen snapshotHold) error {
	frozenMounts := frozen.mountPaths
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
	configSelection := selectionFor(plan.Selections, selectionKeyConfig)
	if configSelection.captures() {
		s.updateRunProgress(ctx, runID, "config", "")
		inspect, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding container inspect: %w", err)
		}
		if configSelection.partial() {
			inspect, err = pruneConfigDocument(inspect, configSelection.Paths)
			if err != nil {
				return err
			}
		}
		if err := writeBytes(tw, "container/inspect.json", inspect); err != nil {
			return err
		}
	}
	if selectionFor(plan.Selections, selectionKeyLogs).captures() {
		s.updateRunProgress(ctx, runID, "logs", "")
		tail := plan.LogTailLines
		tailArg := "all"
		if tail > 0 {
			tailArg = strconv.Itoa(tail)
		}
		reader, err := cli.ContainerLogs(ctx, plan.ContainerID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Timestamps: true, Tail: tailArg})
		if err != nil {
			return fmt.Errorf("reading container logs: %w", err)
		}
		if err := writeStream(ctx, tw, "container/logs.txt", reader, s.backupTempDir(), func() {
			s.updateRunProgress(ctx, runID, "logs", "")
		}); err != nil {
			return err
		}
	}
	filesystemSelection := selectionFor(plan.Selections, selectionKeyFilesystem)
	if filesystemSelection.captures() {
		s.updateRunProgress(ctx, runID, "filesystem", "")
		reader, err := cli.ContainerExport(ctx, plan.ContainerID, client.ContainerExportOptions{})
		if err != nil {
			return fmt.Errorf("exporting container filesystem: %w", err)
		}
		source := reader
		if filesystemSelection.partial() {
			source = filteredTarReader(reader, captureFilesystemPaths(filesystemSelection.Paths), "container/filesystem.tar", s.logger)
		}
		if err := writeStream(ctx, tw, "container/filesystem.tar", source, s.backupTempDir(), func() {
			s.updateRunProgress(ctx, runID, "filesystem", "")
		}); err != nil {
			return err
		}
	}
	if selectionFor(plan.Selections, selectionKeyImage).captures() {
		s.updateRunProgress(ctx, runID, "image", "")
		inspectRes, err := cli.ContainerInspect(ctx, plan.ContainerID, client.ContainerInspectOptions{})
		if err != nil {
			return fmt.Errorf("inspecting container image: %w", err)
		}
		inspect := inspectRes.Container
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
		mountSelection := selectionFor(plan.Selections, selectionKeyMountPfx+cleanMount)
		if !mountSelection.captures() {
			continue
		}
		s.updateRunProgress(ctx, runID, "mounts", "")
		name := backupMountEntryName(cleanMount)
		// A frozen path means this mount was snapshotted, so its data
		// is read from the snapshot and the container was never
		// paused. Partial selections still go through the live copy:
		// the path filter runs over a tar stream, not a directory.
		if frozenPath, frozen := frozenMountPath(frozenMounts, cleanMount); frozen && !mountSelection.partial() {
			if err := s.writeFrozenMount(ctx, tw, name, frozenPath, cleanMount, s.backupTempDir(), func() {
				s.updateRunProgress(ctx, runID, "mounts", "")
			}); err != nil {
				return err
			}
			continue
		}
		readerRes, err := cli.CopyFromContainer(ctx, plan.ContainerID, client.CopyFromContainerOptions{SourcePath: cleanMount})
		if err != nil {
			return fmt.Errorf("copying mounted data %s: %w", cleanMount, err)
		}
		reader := readerRes.Content
		source := reader
		if mountSelection.partial() {
			source = filteredTarReader(reader, captureMountPaths(cleanMount, mountSelection.Paths), name, s.logger)
		}
		if err := writeStream(ctx, tw, name, source, s.backupTempDir(), func() {
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
	defer closeLogged(reader, "backup entry stream")
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			closeLogged(reader, "backup entry stream")
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
	defer removeLogged(tmpName)
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		closeLogged(tmp, "backup temp entry")
		return fmt.Errorf("spooling backup entry %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing backup temp entry %s: %w", name, err)
	}
	in, err := os.Open(tmpName)
	if err != nil {
		return fmt.Errorf("opening backup temp entry %s: %w", name, err)
	}
	defer closeLogged(in, "backup temp entry")
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: size}); err != nil {
		return fmt.Errorf("writing backup header %s: %w", name, err)
	}
	if _, err := io.Copy(tw, in); err != nil {
		return fmt.Errorf("writing backup entry %s: %w", name, err)
	}
	return nil
}

func (s *Service) backupDir() string { return LocalBackupDir(s.dataDir) }

// LocalBackupDir is the absolute local backup root under dataDir; it must be absolute because
// cleanLocalBackupPath rejects the relative default DATA_DIR "./data".
func LocalBackupDir(dataDir string) string {
	abs, err := filepath.Abs(filepath.Join(dataDir, "backups", "containers"))
	if err != nil {
		return filepath.Join(dataDir, "backups", "containers")
	}
	return abs
}

func (s *Service) backupTempDir() string {
	abs, err := filepath.Abs(filepath.Join(s.dataDir, "backups", "tmp"))
	if err != nil {
		return filepath.Join(s.dataDir, "backups", "tmp")
	}
	return abs
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
	defer func() { db.CloseRows(rows) }()

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
	// Read path: report exactly what is stored. Local is no longer
	// backfilled here — a cloud-only plan must not grow a local
	// destination just by being read.
	plan.StorageLocationIDs = backupStorageLocationIDs("", ids)
	plan.StorageLocationID = firstStorageLocationID(plan.StorageLocationIDs)
	return nil
}

func (s *Service) setPlanStorageLocations(ctx context.Context, planID string, ids []string) error {
	// Callers (CreatePlan / UpdatePlan) have already rejected an empty
	// list, so this only normalises.
	ids = backupStorageLocationIDs("", ids)
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
	var selections string
	var snapshotConfigName string
	if err := scanner.Scan(
		&plan.ID, &plan.Name, &plan.EnvironmentID, &plan.ContainerID, &plan.ContainerName,
		&plan.StorageLocationID, &plan.IncludeConfig, &plan.IncludeLogs, &plan.IncludeFilesystem,
		&plan.IncludeImage, &selected, &selections, &plan.LogTailLines, &plan.Cron, &plan.Enabled, &plan.RetentionCount,
		&plan.RetentionDays, &plan.LastRunAt, &plan.NextRunAt, &plan.ConsistencyMode, &plan.SnapshotConfigID,
		&plan.CreatedAt, &plan.UpdatedAt, &snapshotConfigName,
	); err != nil {
		return BackupPlan{}, err
	}
	if snapshotConfigName != "" {
		plan.SnapshotConfigName = snapshotConfigName
	}
	if selected != "" {
		if err := json.Unmarshal([]byte(selected), &plan.SelectedMounts); err != nil {
			return BackupPlan{}, fmt.Errorf("decoding selected mounts: %w", err)
		}
	}
	if plan.SelectedMounts == nil {
		plan.SelectedMounts = []string{}
	}
	// A plan written before migration 142 has no selections. Deriving them
	// from the legacy booleans is what keeps it capturing exactly what it
	// captured yesterday, rather than silently capturing nothing.
	plan.Selections = decodeSelections(selections)
	if plan.Selections == nil {
		plan.Selections = selectionsFromLegacy(
			plan.IncludeConfig, plan.IncludeLogs, plan.IncludeFilesystem, plan.IncludeImage,
			plan.SelectedMounts,
		)
	}
	plan.StorageLocationIDs = backupStorageLocationIDs(plan.StorageLocationID, nil)
	return plan, nil
}

func scanRun(scanner interface{ Scan(dest ...any) error }) (BackupRun, error) {
	var run BackupRun
	var includedItems, availableItems, planName string
	if err := scanner.Scan(
		&run.ID, &run.PlanID, &run.Operation, &run.SourceRunID, &run.EnvironmentID, &run.ContainerID, &run.Status,
		&run.ArchivePath, &run.ArchiveSize, &run.ArchiveEncryption, &run.ArchiveKeyID,
		&run.Error, &run.ProgressStage, &run.ProgressMessage, &run.ProgressUpdatedAt, &run.StartedAt,
		&run.CompletedAt, &run.DurationMS, &run.CreatedAt, &run.UpdatedAt,
		&run.ContainerName, &includedItems, &availableItems, &planName, &run.Origin,
	); err != nil {
		return BackupRun{}, err
	}
	if run.Operation == "" {
		run.Operation = "backup"
	}
	if planName != "" {
		run.PlanName = planName
	}
	included, err := decodeRunItems(includedItems)
	if err != nil {
		return BackupRun{}, err
	}
	available, err := decodeRunItems(availableItems)
	if err != nil {
		return BackupRun{}, err
	}
	run.IncludedItems = included
	run.AvailableItems = available
	run.Scope = runScope(included, available)
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

// skipTarEntry drains the body of a tar entry the caller does not want so
// the reader is positioned at the next header. A read failure here means the
// archive is truncated or corrupt, which callers must surface instead of
// silently returning a partial listing.
func skipTarEntry(hdr *tar.Header, r io.Reader) error {
	if hdr == nil || hdr.Size <= 0 {
		return nil
	}
	if _, err := io.Copy(io.Discard, r); err != nil {
		return fmt.Errorf("skipping archive entry %q: %w", hdr.Name, err)
	}
	return nil
}

// The single-row environment lookups behind rbac.PinEnvToResource: a run, a plan
// or a snapshot config is authorized against the environment it belongs to.
const (
	runEnvQuery            = `SELECT COALESCE(environment_id, '') FROM container_backup_runs WHERE id = ?`
	planEnvQuery           = `SELECT COALESCE(environment_id, '') FROM container_backup_plans WHERE id = ?`
	snapshotConfigEnvQuery = `SELECT COALESCE(environment_id, '') FROM container_backup_snapshot_configs WHERE id = ?`
)

// RunEnvironment reports the environment a backup run belongs to.
func (s *Service) RunEnvironment(ctx context.Context, id string) (string, bool, error) {
	return rbac.EnvOfRow(ctx, s.db, runEnvQuery, id)
}

// PlanEnvironment reports the environment a backup plan belongs to.
func (s *Service) PlanEnvironment(ctx context.Context, id string) (string, bool, error) {
	return rbac.EnvOfRow(ctx, s.db, planEnvQuery, id)
}

// SnapshotConfigEnvironment reports the environment a snapshot config belongs to.
func (s *Service) SnapshotConfigEnvironment(ctx context.Context, id string) (string, bool, error) {
	return rbac.EnvOfRow(ctx, s.db, snapshotConfigEnvQuery, id)
}

// scanRunRows drains rows into runs and closes them, so the caller can issue follow-up queries without holding a connection.
func scanRunRows(rows *sql.Rows) ([]BackupRun, error) {
	defer func() { db.CloseRows(rows) }()
	runs := []BackupRun{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

type abandonedRun struct {
	id            string
	started       string
	environmentID string
	containerID   string
}

type abandonedRestoreRun struct {
	id      string
	started string
}

// scanAbandonedRuns drains rows into abandoned backup runs and closes them before recovery starts writing.
func scanAbandonedRuns(rows *sql.Rows) ([]abandonedRun, error) {
	defer func() { db.CloseRows(rows) }()
	runs := []abandonedRun{}
	for rows.Next() {
		var run abandonedRun
		if err := rows.Scan(&run.id, &run.started, &run.environmentID, &run.containerID); err != nil {
			return nil, fmt.Errorf("scanning abandoned backup run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

// scanAbandonedRestoreRuns drains rows into abandoned restore runs and closes them before recovery starts writing.
func scanAbandonedRestoreRuns(rows *sql.Rows) ([]abandonedRestoreRun, error) {
	defer func() { db.CloseRows(rows) }()
	runs := []abandonedRestoreRun{}
	for rows.Next() {
		var run abandonedRestoreRun
		if err := rows.Scan(&run.id, &run.started); err != nil {
			return nil, fmt.Errorf("scanning abandoned restore run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}
