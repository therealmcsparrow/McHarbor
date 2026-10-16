// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/snapshotdrivers"
)

// ErrSnapshotDriverUnavailable means no driver answers to the name a
// snapshot configuration stores: the plugin providing it was removed
// or disabled, and it is not one of the builtin drivers either. The
// handler turns this into a 404 so the UI can tell the operator to
// reinstall the plugin rather than showing a server error.
var ErrSnapshotDriverUnavailable = errors.New("no snapshot driver registered for this configuration")

// ListSnapshotConfigs returns every snapshot config row for the
// given environment, ordered by name ASC. The host admin UI calls
// this from the Snapshots tab. The 1000-row cap matches
// ListPlans / ListRuns so a runaway environment can't wedge the
// endpoint.
func (s *Service) ListSnapshotConfigs(ctx context.Context, envID string) ([]SnapshotConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, environment_id, name, driver, plugin_slug, enabled,
		       COALESCE(pool_name, ''), COALESCE(dataset_prefix, ''),
		       COALESCE(ssh_target, ''), use_lxd, config,
		       COALESCE(last_probed_at, ''), COALESCE(last_probe, ''),
		       created_at, updated_at
		FROM container_backup_snapshot_configs
		WHERE environment_id = ?
		ORDER BY name ASC LIMIT 1000`, envID)
	if err != nil {
		return nil, fmt.Errorf("listing snapshot configs: %w", err)
	}
	defer func() { db.CloseRows(rows) }()

	out := []SnapshotConfig{}
	for rows.Next() {
		cfg, err := scanSnapshotConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s.publicSnapshotConfig(cfg))
	}
	return out, rows.Err()
}

// SnapshotConfigByID returns the named config or (nil, nil) when no
// row matches, with secret fields still encrypted: it feeds the driver
// path, and API responses go through publicSnapshotConfig instead.
func (s *Service) SnapshotConfigByID(ctx context.Context, id string) (*SnapshotConfig, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, environment_id, name, driver, plugin_slug, enabled,
		       COALESCE(pool_name, ''), COALESCE(dataset_prefix, ''),
		       COALESCE(ssh_target, ''), use_lxd, config,
		       COALESCE(last_probed_at, ''), COALESCE(last_probe, ''),
		       created_at, updated_at
		FROM container_backup_snapshot_configs
		WHERE id = ?`, id)
	cfg, err := scanSnapshotConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// CreateSnapshotConfig inserts a new row and returns the populated
// record. The handler validates input (non-empty name, known
// driver, enabled default true) before calling.
func (s *Service) CreateSnapshotConfig(ctx context.Context, input CreateSnapshotConfigInput) (*SnapshotConfig, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("snapshot config name is required")
	}
	driver := strings.TrimSpace(input.Driver)
	if driver == "" {
		return nil, fmt.Errorf("snapshot config driver is required")
	}
	pluginSlug := strings.TrimSpace(input.PluginSlug)
	if pluginSlug == "" {
		return nil, fmt.Errorf("snapshot config plugin slug is required")
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	useLXD := false
	if input.UseLXD != nil {
		useLXD = *input.UseLXD
	}
	sealed, err := s.sealSnapshotConfig(driver, input.Config, map[string]string{})
	if err != nil {
		return nil, err
	}
	configJSON, err := marshalSnapshotConfigMap(sealed)
	if err != nil {
		return nil, err
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO container_backup_snapshot_configs (
			id, environment_id, name, driver, plugin_slug, enabled,
			pool_name, dataset_prefix, ssh_target, use_lxd, config,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.EnvironmentID, name, driver, pluginSlug, enabled,
		nullString(strings.TrimSpace(input.PoolName)),
		nullString(strings.TrimSpace(input.DatasetPrefix)),
		nullString(strings.TrimSpace(input.SSHTarget)),
		useLXD, configJSON, now, now,
	); err != nil {
		return nil, fmt.Errorf("creating snapshot config: %w", err)
	}
	return s.publicSnapshotConfigByID(ctx, id)
}

// UpdateSnapshotConfig patches a row by id. Only the fields whose
// pointer is non-nil are written; nil pointers leave the column
// unchanged. Updates the updated_at timestamp on every call.
func (s *Service) UpdateSnapshotConfig(ctx context.Context, id string, input UpdateSnapshotConfigInput) (*SnapshotConfig, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("snapshot config name cannot be blank")
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_snapshot_configs SET name = ?, updated_at = ? WHERE id = ?", name, now, id); err != nil {
			return nil, fmt.Errorf("updating snapshot config name: %w", err)
		}
	}
	if input.Enabled != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_snapshot_configs SET enabled = ?, updated_at = ? WHERE id = ?", *input.Enabled, now, id); err != nil {
			return nil, fmt.Errorf("updating snapshot config enabled: %w", err)
		}
	}
	if input.PoolName != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_snapshot_configs SET pool_name = ?, updated_at = ? WHERE id = ?", nullString(strings.TrimSpace(*input.PoolName)), now, id); err != nil {
			return nil, fmt.Errorf("updating snapshot config pool: %w", err)
		}
	}
	if input.DatasetPrefix != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_snapshot_configs SET dataset_prefix = ?, updated_at = ? WHERE id = ?", nullString(strings.TrimSpace(*input.DatasetPrefix)), now, id); err != nil {
			return nil, fmt.Errorf("updating snapshot config dataset prefix: %w", err)
		}
	}
	if input.SSHTarget != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_snapshot_configs SET ssh_target = ?, updated_at = ? WHERE id = ?", nullString(strings.TrimSpace(*input.SSHTarget)), now, id); err != nil {
			return nil, fmt.Errorf("updating snapshot config ssh target: %w", err)
		}
	}
	if input.UseLXD != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_snapshot_configs SET use_lxd = ?, updated_at = ? WHERE id = ?", *input.UseLXD, now, id); err != nil {
			return nil, fmt.Errorf("updating snapshot config use_lxd: %w", err)
		}
	}
	if input.Config != nil {
		stored, err := s.SnapshotConfigByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if stored == nil {
			return nil, nil
		}
		sealed, err := s.sealSnapshotConfig(stored.Driver, *input.Config, stored.Config)
		if err != nil {
			return nil, err
		}
		configJSON, err := marshalSnapshotConfigMap(sealed)
		if err != nil {
			return nil, err
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE container_backup_snapshot_configs SET config = ?, updated_at = ? WHERE id = ?", configJSON, now, id); err != nil {
			return nil, fmt.Errorf("updating snapshot config map: %w", err)
		}
	}
	return s.publicSnapshotConfigByID(ctx, id)
}

// publicSnapshotConfigByID is SnapshotConfigByID in the shape the API returns.
func (s *Service) publicSnapshotConfigByID(ctx context.Context, id string) (*SnapshotConfig, error) {
	cfg, err := s.SnapshotConfigByID(ctx, id)
	if err != nil || cfg == nil {
		return cfg, err
	}
	public := s.publicSnapshotConfig(*cfg)
	return &public, nil
}

// marshalSnapshotConfigMap encodes a config map for the config column.
func marshalSnapshotConfigMap(cfg map[string]string) (string, error) {
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("encoding snapshot config map: %w", err)
	}
	return string(encoded), nil
}

// DeleteSnapshotConfig removes a row by id. The FK from
// container_backup_plans.snapshot_config_id is ON DELETE SET NULL,
// so any plans referencing the deleted config keep working (they
// just revert to consistency_mode='pause' at runtime).
func (s *Service) DeleteSnapshotConfig(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM container_backup_snapshot_configs WHERE id = ?", id); err != nil {
		return fmt.Errorf("deleting snapshot config: %w", err)
	}
	return nil
}

// ProbeSnapshotConfig dispatches a Capability call to the plugin
// that owns the config's Driver and persists the result. Used by
// the admin UI's "Test connection" button.
//
// Returns ErrSnapshotDriverUnavailable when the runtime hasn't
// loaded a plugin for the driver (e.g. plugin disabled / not
// installed). The handler translates this into a 404 with an i18n
// message so the admin UI can prompt the operator to install the
// plugin.
func (s *Service) ProbeSnapshotConfig(ctx context.Context, id string) (*ProbeSnapshotResult, error) {
	cfg, err := s.SnapshotConfigByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	client, _, ok := s.resolveSnapshotAdapter(cfg.Driver)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSnapshotDriverUnavailable, cfg.Driver)
	}
	cfgMap, err := s.driverConfigMap(*cfg)
	if err != nil {
		return nil, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	capability, err := client.Capability(probeCtx, cfgMap)
	if err != nil {
		// Surface the error as a ProbeSnapshotResult so the UI
		// still sees a structured payload. The error text is
		// kept on the result so the user sees the actual cause
		// without needing a separate log lookup.
		capability.Error = err.Error()
		capability.Available = false
		capability.Driver = cfg.Driver
	}
	out := &ProbeSnapshotResult{
		Available: capability.Available,
		ToolPath:  capability.ToolPath,
		Version:   capability.Version,
		Error:     capability.Error,
		Targets:   make([]ProbeSnapshotTargetResult, 0, len(capability.Targets)),
	}
	for _, t := range capability.Targets {
		out.Targets = append(out.Targets, ProbeSnapshotTargetResult{
			Name: t.Name, Mountpoint: t.Mountpoint, UsedBytes: t.UsedBytes, Driver: t.Driver,
		})
	}
	probeJSON, marshalErr := json.Marshal(out)
	if marshalErr != nil && s.logger != nil {
		s.logger.Warn("snapshot config probe result could not be encoded", "id", id, "error", marshalErr)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE container_backup_snapshot_configs
		SET last_probed_at = ?, last_probe = ?, updated_at = ?
		WHERE id = ?`, now, string(probeJSON), now, id); err != nil {
		// Non-fatal: probe result is already in hand; persistence
		// is best-effort.
		if s.logger != nil {
			s.logger.Warn("snapshot config probe persistence failed", "id", id, "error", err)
		}
	}
	return out, nil
}

// RecentSnapshots returns the latest N backup runs that used
// consistency_mode="snapshot", ordered by started_at DESC. The
// admin UI uses this for the "Recent snapshots" card on the
// Snapshots settings tab.
//
// limit is clamped to [1, 100]; default callers pass 20.
func (s *Service) RecentSnapshots(ctx context.Context, envID string, limit int) ([]RecentSnapshot, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, COALESCE(r.plan_id, ''), r.environment_id, r.container_id,
		       COALESCE(NULLIF(r.container_name, ''), p.container_name, '') AS container_name,
		       COALESCE(r.snapshot_name, ''), COALESCE(r.snapshot_driver, ''),
		       COALESCE(r.snapshot_mount_path, ''), COALESCE(r.snapshot_created_at, ''),
		       COALESCE(r.snapshot_removed_at, ''), COALESCE(r.snapshot_error, ''),
		       r.started_at, r.status
		FROM container_backup_runs r
		LEFT JOIN container_backup_plans p ON r.plan_id = p.id
		WHERE r.environment_id = ?
		  AND r.snapshot_name <> ''
		  AND r.snapshot_name IS NOT NULL
		ORDER BY r.started_at DESC
		LIMIT ?`, envID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing recent snapshots: %w", err)
	}
	defer func() { db.CloseRows(rows) }()

	out := []RecentSnapshot{}
	for rows.Next() {
		var s RecentSnapshot
		if err := rows.Scan(
			&s.RunID, &s.PlanID, &s.EnvironmentID, &s.ContainerID, &s.ContainerName,
			&s.SnapshotName, &s.SnapshotDriver, &s.SnapshotMountPath, &s.SnapshotCreatedAt,
			&s.SnapshotRemovedAt, &s.SnapshotError,
			&s.StartedAt, &s.Status,
		); err != nil {
			return nil, fmt.Errorf("scanning recent snapshot row: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// persistSnapshotRef stamps the Create result on the backup run row.
// Called from writeArchive's snapshot-mode branch after the adapter
// returns a non-error SnapshotRef. Errors are non-fatal: the run
// already has the ref in memory and the deferred Remove still has
// the snapshot name to clean up.
func (s *Service) persistSnapshotRef(ctx context.Context, runID string, ref plugin.SnapshotRef, driver string) {
	now := time.Now().UTC().Format(time.RFC3339)
	createdAt := ""
	if !ref.CreatedAt.IsZero() {
		createdAt = ref.CreatedAt.UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE container_backup_runs
		SET snapshot_name = ?, snapshot_driver = ?, snapshot_mount_path = ?,
		    snapshot_created_at = ?, updated_at = ?
		WHERE id = ?`,
		ref.SnapshotName, driver, ref.MountPath, createdAt, now, runID)
	if err != nil && s.logger != nil {
		s.logger.Warn("snapshot ref persistence failed", "run", runID, "error", err)
	}
}

// markSnapshotRemoved stamps snapshot_removed_at on the backup run.
// Called from writeArchive's deferred Remove block after the
// adapter confirms the snapshot is gone. Errors are non-fatal: the
// run is already complete; an operator running a manual reconcile
// can wipe the stale "in use" state.
func (s *Service) markSnapshotRemoved(ctx context.Context, runID string, errMsg string) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		UPDATE container_backup_runs
		SET snapshot_removed_at = ?, snapshot_error = ?, updated_at = ?
		WHERE id = ?`, now, errMsg, now, runID)
	if err != nil && s.logger != nil {
		s.logger.Warn("snapshot remove-mark failed", "run", runID, "error", err)
	}
}

// scanSnapshotConfig scans one row from
// container_backup_snapshot_configs. The config JSON column is
// decoded into a map[string]string so handlers can hand it to the
// plugin without further marshaling.
func scanSnapshotConfig(scanner interface{ Scan(dest ...any) error }) (SnapshotConfig, error) {
	var configJSON string
	return scanSnapshotConfigWithRaw(scanner, &configJSON)
}

// scanSnapshotConfigWithRaw is scanSnapshotConfig that also hands back the stored
// config column verbatim, which the backfill compares against before it writes.
func scanSnapshotConfigWithRaw(scanner interface{ Scan(dest ...any) error }, raw *string) (SnapshotConfig, error) {
	var cfg SnapshotConfig
	var enabledInt, useLXDInt int
	if err := scanner.Scan(
		&cfg.ID, &cfg.EnvironmentID, &cfg.Name, &cfg.Driver, &cfg.PluginSlug, &enabledInt,
		&cfg.PoolName, &cfg.DatasetPrefix, &cfg.SSHTarget, &useLXDInt, raw,
		&cfg.LastProbedAt, &cfg.LastProbe, &cfg.CreatedAt, &cfg.UpdatedAt,
	); err != nil {
		return SnapshotConfig{}, err
	}
	cfg.Enabled = enabledInt != 0
	cfg.UseLXD = useLXDInt != 0
	if *raw != "" {
		if err := json.Unmarshal([]byte(*raw), &cfg.Config); err != nil {
			return SnapshotConfig{}, fmt.Errorf("decoding snapshot config map: %w", err)
		}
	}
	if cfg.Config == nil {
		cfg.Config = map[string]string{}
	}
	return cfg, nil
}

// snapshotConfigToMap flattens a SnapshotConfig to the
// map[string]string the plugin's SnapshotAdapterClient methods
// expect. The plugin picks which keys it needs.
func snapshotConfigToMap(cfg SnapshotConfig) map[string]string {
	out := make(map[string]string, len(cfg.Config)+6)
	for k, v := range cfg.Config {
		out[k] = v
	}
	if cfg.PoolName != "" {
		out["pool_name"] = cfg.PoolName
	}
	if cfg.DatasetPrefix != "" {
		out["dataset_prefix"] = cfg.DatasetPrefix
	}
	if cfg.SSHTarget != "" {
		out["ssh_target"] = cfg.SSHTarget
	}
	if cfg.PluginSlug != "" {
		out["plugin_slug"] = cfg.PluginSlug
	}
	if cfg.UseLXD {
		out["use_lxd"] = "1"
	}
	out["name"] = cfg.Name
	out["driver"] = cfg.Driver
	// Every driver, builtin or plugin-owned, reads the environment from
	// the same key: a snapshot only means anything on the host that
	// actually holds the data.
	out[snapshotdrivers.EnvIDKey] = cfg.EnvironmentID
	return out
}

// snapshotRefLike removed — Go disallows a struct with both a
// SnapshotName field and a SnapshotName method, so the mirror
// type uses .Name + .MountPath directly and the resolveSnapshotAdapter
// translates from plugin.SnapshotRef at the boundary.

// resolveSnapshotAdapter looks up the plugin-owned SnapshotAdapter
// for the given driver. Same typing pattern as the storage
// adapter resolver: the runtime is stashed in Service.snapshotAdapter
// as `any` and the method asserts to plugin.SnapshotAdapterSource
// at boot. When the runtime is unset (the plugin loader hasn't
// been wired yet, or the plugin providing the driver is disabled)
// the bool is false and the caller falls back to docker pause.
func (s *Service) resolveSnapshotAdapter(driver string) (plugin.SnapshotAdapterClient, plugin.SnapshotAdapterSpec, bool) {
	// A plugin that claims the driver wins. An operator who installed
	// a snapshot plugin chose that implementation, and silently running
	// the builtin instead would ignore the plugin's own filters.
	if s.snapshotAdapter != nil {
		if src, ok := s.snapshotAdapter.(plugin.SnapshotAdapterSource); ok {
			if cli, spec, found := src.SnapshotAdapter(driver); found {
				return cli, spec, true
			}
		}
	}
	if s.builtinSnapshots == nil {
		return nil, plugin.SnapshotAdapterSpec{}, false
	}
	return s.builtinSnapshots.Client(driver)
}

// SnapshotDriverStatuses reports which snapshot drivers this
// environment can actually use, merging what the host or its agent
// found on disk with the drivers plugins contribute. This is what the
// Snapshots settings tab lists, and it is why an operator never has to
// guess whether a driver would work before saving a config.
func (s *Service) SnapshotDriverStatuses(ctx context.Context, envID string) ([]snapshotdrivers.Status, error) {
	if strings.TrimSpace(envID) == "" {
		return nil, fmt.Errorf("environment id is required")
	}
	var out []snapshotdrivers.Status
	if s.builtinSnapshots != nil {
		detected, err := s.builtinSnapshots.Detect(ctx, envID)
		if err != nil {
			// A disconnected agent is a normal state, not a failure of
			// the endpoint: report it as one unavailable row so the UI
			// can explain it rather than showing an error page.
			out = append(out, snapshotdrivers.Status{
				Driver:      "agent",
				DisplayName: "Agent host",
				Available:   false,
				Origin:      "agent",
				Error:       err.Error(),
			})
		} else {
			out = append(out, detected...)
		}
	}
	out = append(out, s.pluginSnapshotStatuses(out)...)
	return out, nil
}

// pluginSnapshotStatuses lists plugin-contributed drivers that the
// builtin detection did not already cover. Their availability is not
// probed here: a vendor API call on every page load would be slow and
// would hit the NAS whether or not the operator asked. The Test button
// on a saved config is where that check belongs.
func (s *Service) pluginSnapshotStatuses(existing []snapshotdrivers.Status) []snapshotdrivers.Status {
	if s.snapshotAdapter == nil {
		return nil
	}
	src, ok := s.snapshotAdapter.(interface {
		SnapshotAdapterSpecs() map[string]plugin.SnapshotAdapterSpec
	})
	if !ok {
		return nil
	}
	seen := make(map[string]bool, len(existing))
	for _, row := range existing {
		seen[row.Driver] = true
	}
	specs := src.SnapshotAdapterSpecs()
	out := make([]snapshotdrivers.Status, 0, len(specs))
	for driver, spec := range specs {
		if seen[driver] {
			continue
		}
		out = append(out, snapshotdrivers.Status{
			Driver:      driver,
			DisplayName: spec.DisplayName,
			Available:   true,
			Origin:      "plugin",
			PauseFree:   spec.Capabilities.SupportsMount,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Driver < out[j].Driver })
	return out
}

// snapshotForBackup asks the plugin-owned SnapshotAdapterClient to
// freeze the container's writable layer. Called from writeArchive's
// snapshot branch when the plan has ConsistencyMode="snapshot" and
// a SnapshotConfigID pointing at a row whose plugin is currently
// loaded. Returns the SnapshotRef + driver name on success; the
// caller persists the ref and registers the deferred Remove.
//
// Errors surface to the caller, which decides whether to fall back
// to the legacy pause path (typical for "plugin not installed" or
// "driver CLI not on PATH"). The host logs the failure and proceeds
// without the freeze so a flaky snapshot stack never breaks a
// backup.
func (s *Service) snapshotForBackup(ctx context.Context, plan *BackupPlan, runID string) (plugin.SnapshotRef, string, error) {
	if plan.SnapshotConfigID == "" {
		return plugin.SnapshotRef{}, "", fmt.Errorf("plan %q has no SnapshotConfigID", plan.ID)
	}
	cfg, err := s.SnapshotConfigByID(ctx, plan.SnapshotConfigID)
	if err != nil {
		return plugin.SnapshotRef{}, "", fmt.Errorf("loading snapshot config %q: %w", plan.SnapshotConfigID, err)
	}
	if cfg == nil {
		return plugin.SnapshotRef{}, "", fmt.Errorf("snapshot config %q not found", plan.SnapshotConfigID)
	}
	if !cfg.Enabled {
		return plugin.SnapshotRef{}, "", fmt.Errorf("snapshot config %q is disabled", cfg.ID)
	}
	client, spec, ok := s.resolveSnapshotAdapter(cfg.Driver)
	if !ok {
		return plugin.SnapshotRef{}, "", fmt.Errorf("no plugin registered for driver %q (config %q)", cfg.Driver, cfg.ID)
	}
	cfgMap, err := s.driverConfigMap(*cfg)
	if err != nil {
		return plugin.SnapshotRef{}, "", err
	}
	targets := s.resolveSnapshotTargets(ctx, plan, cfg)
	ref, err := client.Create(ctx, cfgMap, plugin.SnapshotCreateRequest{
		Targets:    targets,
		SnapshotID: runID,
		ReadOnly:   true,
		Timeout:    3600,
	})
	if err != nil {
		return plugin.SnapshotRef{}, "", fmt.Errorf("snapshot create failed (driver %s): %w", spec.Driver, err)
	}
	return ref, spec.Driver, nil
}

// removeSnapshotAfterBackup asks the plugin-owned SnapshotAdapterClient
// to destroy the snapshot taken by snapshotForBackup. Called from
// writeArchive's deferred block (LIFO before unpause). Best-effort:
// a non-nil error is logged but does not fail the run since the
// archive is already on disk and the operator prefers a leaked
// snapshot over a failed backup.
func (s *Service) removeSnapshotAfterBackup(ctx context.Context, plan *BackupPlan, ref plugin.SnapshotRef, driver string) error {
	if plan.SnapshotConfigID == "" || ref.SnapshotName == "" {
		return nil
	}
	client, _, ok := s.resolveSnapshotAdapter(driver)
	if !ok {
		return fmt.Errorf("no plugin registered for driver %q (snapshot %s left behind)", driver, ref.SnapshotName)
	}
	cfg, err := s.SnapshotConfigByID(ctx, plan.SnapshotConfigID)
	if err != nil {
		return fmt.Errorf("loading snapshot config for remove: %w", err)
	}
	if cfg == nil {
		return fmt.Errorf("snapshot config %q vanished mid-run", plan.SnapshotConfigID)
	}
	cfgMap, err := s.driverConfigMap(*cfg)
	if err != nil {
		return err
	}
	return client.Remove(ctx, cfgMap, ref)
}

// resolveSnapshotTargets maps the plan's selected mounts to the
// dataset / subvol / LV names the snapshot adapter expects. The
// rule of thumb: if the snapshot config declared a dataset_prefix,
// targets are prefixed; otherwise the union of bind-mount sources
// is forwarded verbatim. The plugin is free to ignore the list when
// it has its own resolver (Synology uses the share name, TrueNAS
// uses the dataset path stored in cfg.Config).
func (s *Service) resolveSnapshotTargets(ctx context.Context, plan *BackupPlan, cfg *SnapshotConfig) []string {
	if cfg.DatasetPrefix != "" {
		return []string{cfg.DatasetPrefix}
	}
	if names, ok := cfg.Config["targets"]; ok && names != "" {
		return strings.Split(names, ",")
	}
	// Fallback: use the bind-mount sources from the plan. Volumes
	// declared without a Source are skipped; the plugin's Capability
	// reply will tell the admin which volumes are snapshot-capable.
	out := make([]string, 0, len(plan.SelectedMounts))
	for _, m := range plan.SelectedMounts {
		if m != "" {
			out = append(out, m)
		}
	}
	return out
}
