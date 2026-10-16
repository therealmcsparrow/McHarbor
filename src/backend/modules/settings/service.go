// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"github.com/therealmcsparrow/mcharbor/core/certgen"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/tlsconfig"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/core/statshub"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Service handles settings business logic and database operations.
type Service struct {
	db         *sql.DB
	encryption *encryption.Service
	dataDir    string
	// vault reads this module's adopted credential through the secrets vault
	// once Phase 5b's start-up backfill has linked it, and falls back to the
	// legacy column until then. Built here rather than injected: every
	// construction site already supplies the database and the encryption
	// service, and a service built without it would quietly read the legacy
	// column forever.
	vault *secrets.Service

	// reloader is the cert hot-reload helper that the host binary
	// constructs once and shares between the listeners (main,
	// transfer helper) and the settings UI (Phase 7 default-HTTPS).
	// Nil in tests that don't construct a real host.
	reloader *tlsconfig.Reloader
	// tlsHosts / tlsIPs are the SAN entries fed into the
	// self-signed cert generator. Captured at construction so
	// regeneration uses the same SAN list as the first-boot cert.
	tlsHosts []string
	tlsIPs   []string
	// tlsMinVersion mirrors the host's MCHARBOR_TLS_MIN_VERSION env
	// (`tls.VersionTLS12` by default). Surfaced in the TLS status
	// response so the UI can show "Min TLS version" without a
	// second config fetch.
	tlsMinVersion uint16
}

// NewService creates a new settings service.
//
// `reloader` may be nil (e.g. unit tests). `hosts`/`ips` are the
// SAN entries used when the operator clicks "Regenerate" on the
// self-signed cert — they match what was used at first boot.
// `minVersion` is the TLS floor (e.g. tls.VersionTLS12) and is
// surfaced verbatim in the TLS status payload.
func NewService(db *sql.DB, enc *encryption.Service, dataDir string, reloader *tlsconfig.Reloader, hosts []string, ips []string, minVersion uint16) *Service {
	svc := &Service{
		db:            db,
		encryption:    enc,
		dataDir:       dataDir,
		reloader:      reloader,
		tlsHosts:      hosts,
		tlsIPs:        ips,
		tlsMinVersion: minVersion,
	}
	if db != nil && enc != nil {
		svc.vault = secrets.NewService(db, enc, nil)
	}
	return svc
}

// readTLSMaterial reads the stored certificate or private key through the
// vault when Phase 5b has adopted it, falling back to the legacy `settings`
// row until then.
//
// These are the first multi-KB values the vault holds, and the reason
// `secrets.value` must never become a keyed column: a MySQL VARCHAR(255)
// would not hold a certificate chain.
func (s *Service) readTLSMaterial(ctx context.Context, secretID sql.NullString, legacy string) (string, error) {
	if s.vault == nil {
		return s.encryption.Decrypt(legacy)
	}
	return s.vault.ReadThrough(ctx, secretID, legacy)
}

// TimezoneStatus is the General settings timezone payload.
type TimezoneStatus struct {
	Timezone          string `json:"timezone"`
	RuntimeTimezone   string `json:"runtimeTimezone"`
	RestartRequired   bool   `json:"restartRequired"`
	RecreateScheduled bool   `json:"recreateScheduled,omitempty"`
	Message           string `json:"message,omitempty"`
}

// TimezoneInput updates the McHarbor container TZ environment variable.
type TimezoneInput struct {
	Timezone string `json:"timezone"`
}

const timezoneSettingKey = "mcharbor_timezone"

// Timezone returns the configured and currently running timezone.
func (s *Service) Timezone(ctx context.Context) TimezoneStatus {
	configured := strings.TrimSpace(os.Getenv("TZ"))
	if configured == "" {
		configured = "UTC"
	}
	if setting, err := s.ByKey(ctx, timezoneSettingKey); err == nil && strings.TrimSpace(setting.Value) != "" {
		configured = strings.TrimSpace(setting.Value)
	}
	runtime := strings.TrimSpace(os.Getenv("TZ"))
	if runtime == "" {
		runtime = "UTC"
	}
	return TimezoneStatus{
		Timezone:        configured,
		RuntimeTimezone: runtime,
		RestartRequired: configured != runtime,
	}
}

// ErrTimezoneNative refuses a timezone change on a native install: McHarbor follows the host
// clock there and has no container to recreate with TZ, so saving would only half-apply.
var ErrTimezoneNative = errors.New("timezone follows the host on a native install")

// UpdateTimezone persists the requested timezone and recreates McHarbor with TZ set.
func (s *Service) UpdateTimezone(ctx context.Context, input TimezoneInput) (TimezoneStatus, error) {
	if _, _, native := nativeHostConfig(); native {
		return TimezoneStatus{}, ErrTimezoneNative
	}
	tz := strings.TrimSpace(input.Timezone)
	if tz == "" {
		return TimezoneStatus{}, &ErrValidation{Code: i18n.ErrInvalidBody, Message: "timezone is required"}
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return TimezoneStatus{}, &ErrValidation{Code: i18n.ErrInvalidBody, Message: "invalid timezone"}
	}

	if _, err := s.SetByKey(ctx, timezoneSettingKey, SettingInput{Key: timezoneSettingKey, Value: tz, Category: "general"}); err != nil {
		return TimezoneStatus{}, err
	}

	message, err := s.scheduleTimezoneRecreate(ctx, tz)
	if err != nil {
		return TimezoneStatus{}, err
	}
	status := s.Timezone(ctx)
	status.Timezone = tz
	status.RecreateScheduled = true
	status.Message = message
	return status, nil
}

func (s *Service) scheduleTimezoneRecreate(ctx context.Context, timezone string) (string, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return "", fmt.Errorf("creating Docker client for timezone recreate: %w", err)
	}
	defer func() { closeutil.Report(cli.Close(), "cli") }()

	opCtx, cancel := context.WithTimeout(ctx, coredocker.CallTimeout)
	defer cancel()
	current, err := coredocker.CurrentMcHarborContainer(opCtx, cli)
	if err != nil {
		return "", fmt.Errorf("inspecting current McHarbor container: %w", err)
	}

	message, err := coredocker.ScheduleDetachedSelfUpdateHelperForImageWithEnvOverrides(
		ctx,
		cli,
		current,
		s.dataDir,
		"",
		"timezone",
		current.Config.Image,
		map[string]string{"TZ": timezone},
	)
	if err != nil {
		return "", fmt.Errorf("scheduling timezone recreate: %w", err)
	}
	return message, nil
}

// List returns all settings, optionally filtered by category.
// Sensitive keys (tls_cert, tls_key) are excluded from results.
func (s *Service) List(ctx context.Context, category string) ([]Setting, error) {
	var rows *sql.Rows
	var err error

	if category != "" {
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, key, value, category, updated_at FROM settings WHERE category = ? ORDER BY key ASC LIMIT 1000",
			category,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, key, value, category, updated_at FROM settings ORDER BY category ASC, key ASC LIMIT 1000",
		)
	}

	if err != nil {
		return nil, fmt.Errorf("querying settings: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var items []Setting
	for rows.Next() {
		var st Setting
		var cat sql.NullString
		if err := rows.Scan(&st.ID, &st.Key, &st.Value, &cat, &st.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning setting row: %w", err)
		}
		if _, blocked := sensitiveKeys[st.Key]; blocked {
			continue
		}
		st.Category = cat.String
		items = append(items, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating settings: %w", err)
	}

	if items == nil {
		items = []Setting{}
	}

	return items, nil
}

// ByKey returns a single setting by its key.
// Returns sql.ErrNoRows if the key does not exist.
func (s *Service) ByKey(ctx context.Context, key string) (Setting, error) {
	var st Setting
	var cat sql.NullString

	err := s.db.QueryRowContext(ctx,
		"SELECT id, key, value, category, updated_at FROM settings WHERE key = ?", key,
	).Scan(&st.ID, &st.Key, &st.Value, &cat, &st.UpdatedAt)

	if err != nil {
		return Setting{}, err
	}

	st.Category = cat.String
	return st, nil
}

// upsertInTx performs an upsert within an existing transaction.
func upsertInTx(ctx context.Context, tx *sql.Tx, key, value, category, now string) error {
	// value_secret_id is cleared on every settings write. Only the two TLS
	// rows ever carry one, and clearing it is how a re-uploaded certificate
	// avoids being shadowed by the vault copy of the one it replaced.
	result, err := tx.ExecContext(ctx,
		"UPDATE settings SET value = ?, value_secret_id = NULL, category = ?, updated_at = ? WHERE key = ?",
		value, category, now, key,
	)
	if err != nil {
		return fmt.Errorf("updating setting %q: %w", key, err)
	}

	if db.RowsAffected(result) == 0 {
		id := xid.New().String()
		_, err = tx.ExecContext(ctx,
			"INSERT INTO settings (id, key, value, category, updated_at) VALUES (?, ?, ?, ?, ?)",
			id, key, value, category, now,
		)
		if err != nil {
			return fmt.Errorf("inserting setting %q: %w", key, err)
		}
	}

	return nil
}

// SetByKey creates or updates a single setting by key. Returns the resulting setting.
func (s *Service) SetByKey(ctx context.Context, key string, input SettingInput) (Setting, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := s.db.ExecContext(ctx,
		"UPDATE settings SET value = ?, category = ?, updated_at = ? WHERE key = ?",
		input.Value, input.Category, now, key,
	)
	if err != nil {
		return Setting{}, fmt.Errorf("updating setting %q: %w", key, err)
	}

	if db.RowsAffected(result) == 0 {
		id := xid.New().String()
		_, err = s.db.ExecContext(ctx,
			"INSERT INTO settings (id, key, value, category, updated_at) VALUES (?, ?, ?, ?, ?)",
			id, key, input.Value, input.Category, now,
		)
		if err != nil {
			return Setting{}, fmt.Errorf("inserting setting %q: %w", key, err)
		}
	}

	return Setting{
		Key:       key,
		Value:     input.Value,
		Category:  input.Category,
		UpdatedAt: now,
	}, nil
}

// BulkUpdateResult holds the outcome of a bulk update operation.
type BulkUpdateResult struct {
	Updated int `json:"updated"`
}

// BulkUpdate updates multiple settings in a single transaction.
func (s *Service) BulkUpdate(ctx context.Context, settings []SettingInput) (BulkUpdateResult, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BulkUpdateResult{}, fmt.Errorf("beginning transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	updateStmt, err := tx.PrepareContext(ctx,
		"UPDATE settings SET value = ?, category = ?, updated_at = ? WHERE key = ?",
	)
	if err != nil {
		return BulkUpdateResult{}, fmt.Errorf("preparing update statement: %w", err)
	}
	defer func() { closeutil.Report(updateStmt.Close(), "updateStmt") }()

	insertStmt, err := tx.PrepareContext(ctx,
		"INSERT INTO settings (id, key, value, category, updated_at) VALUES (?, ?, ?, ?, ?)",
	)
	if err != nil {
		return BulkUpdateResult{}, fmt.Errorf("preparing insert statement: %w", err)
	}
	defer func() { closeutil.Report(insertStmt.Close(), "insertStmt") }()

	for _, si := range settings {
		if si.Key == "" {
			continue
		}

		result, err := updateStmt.ExecContext(ctx, si.Value, si.Category, now, si.Key)
		if err != nil {
			return BulkUpdateResult{}, fmt.Errorf("updating setting %q: %w", si.Key, err)
		}

		if db.RowsAffected(result) == 0 {
			id := xid.New().String()
			_, err = insertStmt.ExecContext(ctx, id, si.Key, si.Value, si.Category, now)
			if err != nil {
				return BulkUpdateResult{}, fmt.Errorf("inserting setting %q: %w", si.Key, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return BulkUpdateResult{}, fmt.Errorf("committing transaction: %w", err)
	}

	return BulkUpdateResult{Updated: len(settings)}, nil
}

// AgentSettingsView is the agent settings response: the shared settings plus
// the stats push interval, which lives outside core/settings because that
// package is linked by plugins.
type AgentSettingsView struct {
	coreSettings.AgentSettings
	StatsPushInterval int `json:"statsPushInterval"`
}

// AgentSettings returns the stored agent settings with their defaults applied.
func (s *Service) AgentSettings(ctx context.Context) AgentSettingsView {
	return AgentSettingsView{
		AgentSettings:     coreSettings.ReadAgentSettingsContext(ctx, s.db),
		StatsPushInterval: int(statshub.ReadPushIntervalContext(ctx, s.db) / time.Second),
	}
}

// UpdateAgentSettings validates and persists agent settings.
func (s *Service) UpdateAgentSettings(ctx context.Context, input AgentSettingsInput) error {
	// Validate
	if input.EventMode != "poll" && input.EventMode != "stream" {
		return fmt.Errorf("invalid event mode: %s", input.EventMode)
	}
	if input.EventPollInterval < 10 || input.EventPollInterval > 300 {
		return fmt.Errorf("event poll interval must be between 10 and 300")
	}
	if input.PingInterval < 10 || input.PingInterval > 120 {
		return fmt.Errorf("ping interval must be between 10 and 120")
	}
	if input.RequestTimeout < 5 || input.RequestTimeout > 120 {
		return fmt.Errorf("request timeout must be between 5 and 120")
	}
	if input.StatsPushInterval == 0 {
		input.StatsPushInterval = int(statshub.ReadPushIntervalContext(ctx, s.db) / time.Second)
	}
	minPush, maxPush := int(statshub.MinPushInterval/time.Second), int(statshub.MaxPushInterval/time.Second)
	if input.StatsPushInterval < minPush || input.StatsPushInterval > maxPush {
		return fmt.Errorf("stats push interval must be between %d and %d", minPush, maxPush)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	metricsVal := "false"
	if input.MetricsEnabled {
		metricsVal = "true"
	}

	kvs := map[string]string{
		"agent_event_mode":          input.EventMode,
		"agent_event_poll_interval": strconv.Itoa(input.EventPollInterval),
		"agent_ping_interval":       strconv.Itoa(input.PingInterval),
		"agent_metrics_enabled":     metricsVal,
		"agent_request_timeout":     strconv.Itoa(input.RequestTimeout),
		statshub.PushIntervalKey:    strconv.Itoa(input.StatsPushInterval),
	}

	for key, value := range kvs {
		if err := upsertInTx(ctx, tx, key, value, "agent", now); err != nil {
			return fmt.Errorf("upserting agent setting %q: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// RetentionSettings returns the current retention settings from the database.
func (s *Service) RetentionSettings(ctx context.Context) RetentionSettingsResponse {
	settings := coreSettings.ReadRetentionSettingsContext(ctx, s.db)
	presets, err := s.AuditRetentionPresets(ctx)
	if err != nil {
		presets = []AuditRetentionPreset{}
	}
	return RetentionSettingsResponse{RetentionSettings: settings, AuditRetentionPresets: presets}
}

// UpdateRetentionSettings validates and persists retention settings.
func (s *Service) UpdateRetentionSettings(ctx context.Context, input RetentionSettingsInput) error {
	if input.AuditRetentionDays < 0 || input.AuditRetentionDays > 3650 {
		return fmt.Errorf("audit retention days must be between 0 and 3650")
	}
	if input.ActivityRetentionDays < 0 || input.ActivityRetentionDays > 3650 {
		return fmt.Errorf("activity retention days must be between 0 and 3650")
	}
	if input.BackupRetentionCount < 0 || input.BackupRetentionCount > 1000 {
		return fmt.Errorf("backup retention count must be between 0 and 1000")
	}
	if input.BackupRetentionDays < 0 || input.BackupRetentionDays > 3650 {
		return fmt.Errorf("backup retention days must be between 0 and 3650")
	}
	if input.LifecycleRetentionDays < 0 || input.LifecycleRetentionDays > 3650 {
		return fmt.Errorf("lifecycle retention days must be between 0 and 3650")
	}
	if input.BackupLogRetentionDays < 0 || input.BackupLogRetentionDays > 3650 {
		return fmt.Errorf("backup log retention days must be between 0 and 3650")
	}
	if input.ScanRetentionDays < 0 || input.ScanRetentionDays > 3650 {
		return fmt.Errorf("scan retention days must be between 0 and 3650")
	}

	// Metrics history is tiered, so an omitted field must not be read as a
	// request to set it to zero: unlike the log retentions above, 0 is not a
	// legal "keep forever" here — an unbounded 30s series would grow without
	// limit. Fall back to what is stored.
	current := coreSettings.ReadRetentionSettingsContext(ctx, s.db)
	if input.MetricsRawHours == 0 {
		input.MetricsRawHours = current.MetricsRawHours
	}
	if input.MetricsRollupDays == 0 {
		input.MetricsRollupDays = current.MetricsRollupDays
	}
	metricsDiskEnabled := current.MetricsDiskEnabled
	if input.MetricsDiskEnabled != nil {
		metricsDiskEnabled = *input.MetricsDiskEnabled
	}
	recordExecEvents := current.RecordExecEvents
	if input.RecordExecEvents != nil {
		recordExecEvents = *input.RecordExecEvents
	}
	apiKeyUsageRetentionDays := current.APIKeyUsageRetentionDays
	if input.APIKeyUsageRetentionDays != nil {
		apiKeyUsageRetentionDays = *input.APIKeyUsageRetentionDays
	}
	if apiKeyUsageRetentionDays < 0 || apiKeyUsageRetentionDays > 3650 {
		return fmt.Errorf("api key usage retention days must be between 0 and 3650")
	}
	if input.MetricsRawHours < 1 || input.MetricsRawHours > 8760 {
		return fmt.Errorf("metrics raw retention hours must be between 1 and 8760")
	}
	if input.MetricsRollupDays < 1 || input.MetricsRollupDays > 3650 {
		return fmt.Errorf("metrics rollup retention days must be between 1 and 3650")
	}
	input.ActiveAuditRetentionPreset = strings.TrimSpace(input.ActiveAuditRetentionPreset)
	if input.ActiveAuditRetentionPreset == "" {
		input.ActiveAuditRetentionPreset = "custom"
	}
	preset, err := s.AuditRetentionPreset(ctx, input.ActiveAuditRetentionPreset)
	if err != nil {
		return err
	}
	if preset == nil {
		return fmt.Errorf("audit retention preset not found")
	}
	if !preset.IsCustom {
		input.AuditRetentionDays = preset.RetentionDays
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	kvs := map[string]string{
		"retention_audit_days":      strconv.Itoa(input.AuditRetentionDays),
		"retention_activity_days":   strconv.Itoa(input.ActivityRetentionDays),
		"retention_lifecycle_days":  strconv.Itoa(input.LifecycleRetentionDays),
		"retention_backup_log_days": strconv.Itoa(input.BackupLogRetentionDays),
		"retention_scan_days":       strconv.Itoa(input.ScanRetentionDays),

		"retention_api_key_usage_days": strconv.Itoa(apiKeyUsageRetentionDays),
		"retention_backup_count":       strconv.Itoa(input.BackupRetentionCount),
		"retention_backup_days":        strconv.Itoa(input.BackupRetentionDays),
		"retention_audit_preset":       input.ActiveAuditRetentionPreset,

		"retention_metrics_raw_hours":    strconv.Itoa(input.MetricsRawHours),
		"retention_metrics_rollup_days":  strconv.Itoa(input.MetricsRollupDays),
		"retention_metrics_disk_enabled": strconv.FormatBool(metricsDiskEnabled),
		"retention_record_exec_events":   strconv.FormatBool(recordExecEvents),
	}

	for key, value := range kvs {
		if err := upsertInTx(ctx, tx, key, value, "retention", now); err != nil {
			return fmt.Errorf("upserting retention setting %q: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// AuditRetentionPresets returns built-in and custom audit retention presets.
func (s *Service) AuditRetentionPresets(ctx context.Context) ([]AuditRetentionPreset, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, framework, retention_days, description, is_custom, sort_order
		FROM audit_retention_presets
		ORDER BY sort_order ASC, name ASC
		LIMIT 100
	`)
	if err != nil {
		return nil, fmt.Errorf("querying audit retention presets: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	presets := []AuditRetentionPreset{}
	for rows.Next() {
		var preset AuditRetentionPreset
		var isCustom int
		if err := rows.Scan(&preset.ID, &preset.Name, &preset.Framework, &preset.RetentionDays, &preset.Description, &isCustom, &preset.SortOrder); err != nil {
			return nil, fmt.Errorf("scanning audit retention preset: %w", err)
		}
		preset.IsCustom = isCustom == 1
		presets = append(presets, preset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating audit retention presets: %w", err)
	}
	return presets, nil
}

// AuditRetentionPreset returns one audit retention preset by ID.
func (s *Service) AuditRetentionPreset(ctx context.Context, id string) (*AuditRetentionPreset, error) {
	var preset AuditRetentionPreset
	var isCustom int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, framework, retention_days, description, is_custom, sort_order
		FROM audit_retention_presets
		WHERE id = ?
		LIMIT 1
	`, id).Scan(&preset.ID, &preset.Name, &preset.Framework, &preset.RetentionDays, &preset.Description, &isCustom, &preset.SortOrder)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying audit retention preset: %w", err)
	}
	preset.IsCustom = isCustom == 1
	return &preset, nil
}

// ScannerSettings returns the current scanner settings from the database.
func (s *Service) ScannerSettings(ctx context.Context) coreSettings.ScannerSettings {
	return coreSettings.ReadScannerSettingsContext(ctx, s.db)
}

// UpdateScannerSettings validates and persists scanner settings.
func (s *Service) UpdateScannerSettings(ctx context.Context, input ScannerSettingsInput) error {
	// Validate. The default-scanner check that lived here is gone: it
	// hard-coded the three built-in engine names, so a fourth scanner
	// plugin would have been rejected by a settings form that had no
	// business knowing engine names at all. The default now lives on
	// a `scanner_configs` row and is elected there.
	if input.ScanTimeout < 30 || input.ScanTimeout > 1800 {
		return fmt.Errorf("scan timeout must be between 30 and 1800")
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	boolStr := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}

	kvs := map[string]string{
		"scanner_timeout":         strconv.Itoa(input.ScanTimeout),
		"scanner_scan_on_install": boolStr(input.ScanOnInstall),
		"scanner_scan_on_update":  boolStr(input.ScanOnUpdate),
	}

	for key, value := range kvs {
		if err := upsertInTx(ctx, tx, key, value, "scanners", now); err != nil {
			return fmt.Errorf("upserting scanner setting %q: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// TLSStatus returns the current TLS configuration status. Phase 7
// (default-HTTPS) extends the payload with certSource, tlsMinVersion,
// hstsMaxAge, and transferPort so the UI can render the new banner
// and info rows without a second config round-trip.
func (s *Service) TLSStatus(ctx context.Context) TLSStatus {
	status := TLSStatus{
		TLSMinVersion: s.tlsMinVersion,
	}

	// Read tls_enabled
	var enabledVal string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", "tls_enabled").Scan(&enabledVal)
	if err == nil {
		status.Enabled = enabledVal == "true"
	}

	// Read tls_force_https
	var forceVal string
	err = s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", "tls_force_https").Scan(&forceVal)
	if err == nil {
		status.ForceHttps = forceVal == "true"
	}

	// HSTS opt-in: 0 means disabled (the safe default to preserve
	// the HTTP→HTTPS recovery redirect listener).
	if v, ok := readSetting(ctx, s.db, "hsts_max_age"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			status.HSTSMaxAge = n
		}
	}

	// transferPort: surfaced so operators can confirm the host
	// matches their docker-compose port mapping. Falls back to the
	// canonical 8788 if not set.
	if v, ok := readSetting(ctx, s.db, "transfer_port"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			status.TransferPort = n
		}
	}
	if status.TransferPort == 0 {
		status.TransferPort = 8788
	}

	// Root CA metadata. Absent on an install that has only ever served
	// an operator upload, which is why every field is optional.
	tlsDir := filepath.Join(s.dataDir, "tls")
	if fp, caErr := certgen.CAFingerprint(tlsDir); caErr == nil {
		status.HasCA = true
		status.CAFingerprint = fp
		if notAfter, naErr := certgen.CANotAfter(tlsDir); naErr == nil {
			status.CANotAfter = notAfter.UTC().Format(time.RFC3339)
		}
	}

	// Read encrypted cert to extract metadata and source.
	var encCert string
	var certSecretID sql.NullString
	err = s.db.QueryRowContext(ctx,
		"SELECT value, value_secret_id FROM settings WHERE key = ?", "tls_cert",
	).Scan(&encCert, &certSecretID)
	if err == nil && (encCert != "" || certSecretID.String != "") {
		certPEM, decErr := s.readTLSMaterial(ctx, certSecretID, encCert)
		if decErr == nil && certPEM != "" {
			status.HasCert = true
			status.CertInfo = parseCertInfo(certPEM)
			status.CertSource = "user-uploaded"
		}
	}

	// No DB-stored cert means the running listener is using the
	// self-signed file under $DATA_DIR/tls/cert.pem.
	if !status.HasCert {
		_, statErr := os.Stat(filepath.Join(s.dataDir, "tls", "cert.pem"))
		if statErr == nil {
			status.HasCert = true
			status.CertSource = "self-signed"
			// Re-parse the file directly to populate CertInfo —
			// the on-disk cert carries the SAN list and validity
			// dates the operator actually serves.
			if data, readErr := os.ReadFile(filepath.Join(s.dataDir, "tls", "cert.pem")); readErr == nil {
				status.CertInfo = parseCertInfo(string(data))
			}
		}
	}

	return status
}

// readSetting is a small helper for the TLSStatus builder. Returns
// (value, true) when the setting exists, ("", false) otherwise. Used
// for the opt-in HSTS and transfer-port overrides.
func readSetting(ctx context.Context, db *sql.DB, key string) (string, bool) {
	var value string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		return "", false
	}
	return value, true
}

// ErrValidation is returned when input validation fails. The optional
// Code carries an i18n.MsgCode so handlers can return a more specific
// error code than the generic ErrInvalidBody; handlers fall back to
// ErrInvalidBody when Code is empty (legacy call sites).
type ErrValidation struct {
	Message string
	Code    i18n.MsgCode
}

func (e *ErrValidation) Error() string {
	return e.Message
}

// UpdateTLS validates and persists TLS configuration. If cert/key are provided,
// they are encrypted in the database and written as PEM files to the data directory.
func (s *Service) UpdateTLS(ctx context.Context, input TLSUpdateRequest) (TLSStatus, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	// Validate cert+key pair if both provided
	if input.Cert != nil && input.Key != nil {
		if _, err := tls.X509KeyPair([]byte(*input.Cert), []byte(*input.Key)); err != nil {
			return TLSStatus{}, &ErrValidation{Message: "invalid certificate/key pair"}
		}
	} else if (input.Cert != nil) != (input.Key != nil) {
		return TLSStatus{}, &ErrValidation{Message: "cert and key must be provided together"}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TLSStatus{}, fmt.Errorf("beginning transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	upsert := func(key, value string) error {
		return upsertInTx(ctx, tx, key, value, "tls", now)
	}

	// Track whether we need to write cert files after commit
	var pendingCert, pendingKey string

	// Store encrypted cert+key
	if input.Cert != nil && input.Key != nil {
		encCert, err := s.encryption.Encrypt(*input.Cert)
		if err != nil {
			return TLSStatus{}, fmt.Errorf("encrypting certificate: %w", err)
		}
		encKey, err := s.encryption.Encrypt(*input.Key)
		if err != nil {
			return TLSStatus{}, fmt.Errorf("encrypting key: %w", err)
		}

		if err := upsert("tls_cert", encCert); err != nil {
			return TLSStatus{}, fmt.Errorf("saving certificate: %w", err)
		}
		if err := upsert("tls_key", encKey); err != nil {
			return TLSStatus{}, fmt.Errorf("saving key: %w", err)
		}

		pendingCert = *input.Cert
		pendingKey = *input.Key
	}

	if input.Enabled != nil {
		val := "false"
		if *input.Enabled {
			val = "true"
		}
		if err := upsert("tls_enabled", val); err != nil {
			return TLSStatus{}, fmt.Errorf("saving tls_enabled: %w", err)
		}
	}

	if input.ForceHttps != nil {
		val := "false"
		if *input.ForceHttps {
			val = "true"
		}
		if err := upsert("tls_force_https", val); err != nil {
			return TLSStatus{}, fmt.Errorf("saving tls_force_https: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return TLSStatus{}, fmt.Errorf("committing transaction: %w", err)
	}

	// Write raw PEM to disk only after DB commit succeeds
	if pendingCert != "" && pendingKey != "" {
		if err := writeTLSFiles(s.dataDir, pendingCert, pendingKey); err != nil {
			return TLSStatus{}, fmt.Errorf("writing tls files: %w", err)
		}
	}

	return s.TLSStatus(ctx), nil
}

// writeTLSFiles writes cert and key PEM to $DATA_DIR/tls/ with secure permissions.
func writeTLSFiles(dataDir, certPEM, keyPEM string) error {
	tlsDir := filepath.Join(dataDir, "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		return fmt.Errorf("creating tls directory: %w", err)
	}

	if err := os.WriteFile(filepath.Join(tlsDir, "cert.pem"), []byte(certPEM), 0o600); err != nil {
		return fmt.Errorf("writing cert.pem: %w", err)
	}

	if err := os.WriteFile(filepath.Join(tlsDir, "key.pem"), []byte(keyPEM), 0o600); err != nil {
		return fmt.Errorf("writing key.pem: %w", err)
	}

	return nil
}

// RegenerateSelfSigned overwrites the on-disk self-signed cert with a
// freshly generated one (same SANs as first boot). The host's cert
// reload watcher picks up the new fingerprint on its next 30-second
// poll and atomically swaps every listener's TLSConfig.
//
// This is a no-op when no cert files exist yet — the operator hasn't
// enabled TLS, so there's nothing to rotate. Returns the new
// fingerprint (colon-separated SHA-256 hex) on success.
//
// Errors propagate to the caller so the handler can return a
// translated i18n code; the in-memory config is never touched, so a
// failed regen leaves the previous cert serving.
func (s *Service) RegenerateSelfSigned(ctx context.Context) (string, error) {
	// Determine the SAN list. We reuse what the host passed in at
	// construction; if empty (e.g. the reloader was wired without
	// SANs), fall back to the canonical "always include localhost"
	// set so the cert at least validates locally.
	hosts := s.tlsHosts
	if len(hosts) == 0 {
		hosts = []string{"localhost"}
	}

	// Translate the captured IPs into net.IP slice that certgen wants.
	var ips []net.IP
	for _, raw := range s.tlsIPs {
		if ip := net.ParseIP(raw); ip != nil {
			ips = append(ips, ip)
		}
	}

	// IssueLeaf overwrites cert.pem and key.pem in place but leaves the
	// root alone, so every operator who installed it keeps their trust.
	// Rotating the root here would silently break every machine that had
	// been set up, which is the opposite of what this button is for.
	tlsDir := filepath.Join(s.dataDir, "tls")
	if _, _, err := certgen.IssueLeaf(tlsDir, hosts, ips); err != nil {
		return "", fmt.Errorf("reissuing server certificate: %w", err)
	}

	// Trigger a watcher poll immediately so the operator doesn't wait
	// up to 30s for the next tick.
	if s.reloader != nil {
		fp, relErr := s.reloader.Reload()
		if relErr != nil {
			return "", fmt.Errorf("reloading tls config after regen: %w", relErr)
		}
		return fp, nil
	}

	// Without a reloader (e.g. unit tests), the in-memory config in
	// main.go is stale until the next tick. Surface a fingerprint
	// from disk so the UI can at least show "new cert" without the
	// in-memory swap.
	return tlsconfig.Fingerprint(filepath.Join(tlsDir, "cert.pem"))
}

// CACertificate returns the local root certificate as PEM. Only the
// public half is ever read: the signing key beside it stays on disk.
func (s *Service) CACertificate() ([]byte, error) {
	path := certgen.CACertPath(filepath.Join(s.dataDir, "tls"))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading local ca certificate: %w", err)
	}
	return data, nil
}

// ─── Toasts ───────────────────────────────────────────────────────────

// ToastSettings returns the app-wide toast defaults.
func (s *Service) ToastSettings(ctx context.Context) coreSettings.ToastSettings {
	return coreSettings.ReadToastSettingsContext(ctx, s.db)
}

// UpdateToastSettings validates and persists the app-wide toast defaults.
//
// Unlike the retention writer this is a total write: the toast form posts
// every field on every save, so there is nothing to merge with what is
// stored and an omitted field is a malformed request, not an intent to keep.
func (s *Service) UpdateToastSettings(ctx context.Context, input ToastSettingsInput) error {
	if !coreSettings.ValidToastPosition(input.Position) {
		return fmt.Errorf("toast position %q is not one of %v", input.Position, coreSettings.ToastPositions)
	}
	if input.DurationMs < coreSettings.ToastMinDurationMs || input.DurationMs > coreSettings.ToastMaxDurationMs {
		return fmt.Errorf("toast duration must be between %d and %d ms",
			coreSettings.ToastMinDurationMs, coreSettings.ToastMaxDurationMs)
	}
	if input.MaxVisible < coreSettings.ToastMinVisible || input.MaxVisible > coreSettings.ToastMaxVisible {
		return fmt.Errorf("toast max visible must be between %d and %d",
			coreSettings.ToastMinVisible, coreSettings.ToastMaxVisible)
	}
	// A toast that never dismisses itself and has no close button cannot be
	// got rid of at all, so the pairing is rejected rather than silently
	// corrected — the operator should see which of the two they meant.
	if input.DurationMs == 0 && !input.CloseButton {
		return fmt.Errorf("a toast that never auto-dismisses needs a close button")
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	kvs := map[string]string{
		"toast_position":            input.Position,
		"toast_duration_ms":         strconv.Itoa(input.DurationMs),
		"toast_max_visible":         strconv.Itoa(input.MaxVisible),
		"toast_rich_colors":         strconv.FormatBool(input.RichColors),
		"toast_close_button":        strconv.FormatBool(input.CloseButton),
		"toast_allow_user_override": strconv.FormatBool(input.AllowUserOverride),
	}

	for key, value := range kvs {
		if err := upsertInTx(ctx, tx, key, value, "toast", now); err != nil {
			return fmt.Errorf("upserting toast setting %q: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// ResetUserToastOverrides clears every account's per-user toast columns so
// the whole install follows the app-wide default again. It reports how many
// accounts actually had an override, which is what the confirmation toast
// tells the operator — "reset 0 accounts" is a useful answer.
func (s *Service) ResetUserToastOverrides(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET toast_position = NULL,
		    toast_duration_ms = NULL,
		    toast_max_visible = NULL,
		    toast_rich_colors = NULL,
		    toast_close_button = NULL,
		    updated_at = ?
		WHERE toast_position IS NOT NULL
		   OR toast_duration_ms IS NOT NULL
		   OR toast_max_visible IS NOT NULL
		   OR toast_rich_colors IS NOT NULL
		   OR toast_close_button IS NOT NULL`,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("resetting user toast overrides: %w", err)
	}
	return db.RowsAffected(result), nil
}

// ─── Appearance ───────────────────────────────────────────────────────

// AppearanceSettings returns the app-wide appearance defaults.
func (s *Service) AppearanceSettings(ctx context.Context) coreSettings.AppearanceSettings {
	return coreSettings.ReadAppearanceSettingsContext(ctx, s.db)
}

// UpdateAppearanceSettings validates and persists the app-wide appearance
// defaults.
//
// Like the toast writer this is a total write: the form posts every field on
// every save, so an omitted field is a malformed request rather than an
// intent to keep what is stored.
func (s *Service) UpdateAppearanceSettings(ctx context.Context, input AppearanceSettingsInput) error {
	if !coreSettings.ValidAppearanceThemeID(input.ThemeID) {
		return fmt.Errorf("appearance theme id %q is not a usable id", input.ThemeID)
	}
	if !coreSettings.ValidAppearanceAccent(input.Accent) {
		return fmt.Errorf("appearance accent %q is not one of %v", input.Accent, coreSettings.AppearanceAccents)
	}
	if !coreSettings.ValidAppearanceFont(input.Font) {
		return fmt.Errorf("appearance font %q is not one of %v", input.Font, coreSettings.AppearanceFonts)
	}
	if input.BaseFontPx < coreSettings.AppearanceMinBaseFontPx || input.BaseFontPx > coreSettings.AppearanceMaxBaseFontPx {
		return fmt.Errorf("appearance base font size must be between %d and %d px",
			coreSettings.AppearanceMinBaseFontPx, coreSettings.AppearanceMaxBaseFontPx)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	kvs := map[string]string{
		"appearance_theme_id":            input.ThemeID,
		"appearance_accent":              input.Accent,
		"appearance_font":                input.Font,
		"appearance_base_font_px":        strconv.Itoa(input.BaseFontPx),
		"appearance_allow_user_override": strconv.FormatBool(input.AllowUserOverride),
	}

	for key, value := range kvs {
		if err := upsertInTx(ctx, tx, key, value, "appearance", now); err != nil {
			return fmt.Errorf("upserting appearance setting %q: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// ResetUserAppearanceOverrides clears every account's per-user appearance
// columns so the whole install follows the app-wide default again. It reports
// how many accounts actually had an override, which is what the confirmation
// toast tells the operator — "reset 0 accounts" is a useful answer.
func (s *Service) ResetUserAppearanceOverrides(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET appearance_theme_id = NULL,
		    appearance_accent = NULL,
		    appearance_font = NULL,
		    appearance_base_font_px = NULL,
		    updated_at = ?
		WHERE appearance_theme_id IS NOT NULL
		   OR appearance_accent IS NOT NULL
		   OR appearance_font IS NOT NULL
		   OR appearance_base_font_px IS NOT NULL`,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("resetting user appearance overrides: %w", err)
	}
	return db.RowsAffected(result), nil
}

// ─── Branding ─────────────────────────────────────────────────────────

// BrandingSettings returns the operator-supplied application marks. Empty
// fields mean "use the bundled McHarbor mark".
func (s *Service) BrandingSettings(ctx context.Context) coreSettings.BrandingSettings {
	return coreSettings.ReadBrandingSettingsContext(ctx, s.db)
}

// UpdateBrandingSettings validates and persists both marks.
//
// A total write, like the appearance writer: the card posts both fields on
// every save, and an empty field is an explicit "clear this mark" rather than
// "keep what is stored". That is what makes Clear + Save work without a
// second endpoint.
func (s *Service) UpdateBrandingSettings(ctx context.Context, input BrandingSettingsInput) error {
	marks := map[string]string{
		"logoWide":     input.LogoWide,
		"logoWideDark": input.LogoWideDark,
		"logoIcon":     input.LogoIcon,
		"logoIconDark": input.LogoIconDark,
	}
	for field, value := range marks {
		if err := coreSettings.ValidateBrandingImage(field, value); err != nil {
			return err
		}
	}
	rasters := map[string]string{
		"logoWidePng": input.LogoWidePNG,
		"logoIconPng": input.LogoIconPNG,
	}
	for field, value := range rasters {
		if err := coreSettings.ValidateBrandingRaster(field, value); err != nil {
			return err
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	kvs := map[string]string{
		"branding_logo_wide":      input.LogoWide,
		"branding_logo_wide_dark": input.LogoWideDark,
		"branding_logo_icon":      input.LogoIcon,
		"branding_logo_icon_dark": input.LogoIconDark,
		"branding_logo_wide_png":  input.LogoWidePNG,
		"branding_logo_icon_png":  input.LogoIconPNG,
	}
	for key, value := range kvs {
		if err := upsertInTx(ctx, tx, key, value, "branding", now); err != nil {
			return fmt.Errorf("upserting branding setting %q: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}
