// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// Service handles settings business logic and database operations.
type Service struct {
	db         *sql.DB
	encryption *encryption.Service
	dataDir    string
}

// NewService creates a new settings service.
func NewService(db *sql.DB, enc *encryption.Service, dataDir string) *Service {
	return &Service{db: db, encryption: enc, dataDir: dataDir}
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
	defer rows.Close()

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
func upsertInTx(tx *sql.Tx, key, value, category, now string) error {
	result, err := tx.Exec(
		"UPDATE settings SET value = ?, category = ?, updated_at = ? WHERE key = ?",
		value, category, now, key,
	)
	if err != nil {
		return fmt.Errorf("updating setting %q: %w", key, err)
	}

	if db.RowsAffected(result) == 0 {
		id := xid.New().String()
		_, err = tx.Exec(
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
	defer tx.Rollback()

	updateStmt, err := tx.Prepare(
		"UPDATE settings SET value = ?, category = ?, updated_at = ? WHERE key = ?",
	)
	if err != nil {
		return BulkUpdateResult{}, fmt.Errorf("preparing update statement: %w", err)
	}
	defer updateStmt.Close()

	insertStmt, err := tx.Prepare(
		"INSERT INTO settings (id, key, value, category, updated_at) VALUES (?, ?, ?, ?, ?)",
	)
	if err != nil {
		return BulkUpdateResult{}, fmt.Errorf("preparing insert statement: %w", err)
	}
	defer insertStmt.Close()

	for _, si := range settings {
		if si.Key == "" {
			continue
		}

		result, err := updateStmt.Exec(si.Value, si.Category, now, si.Key)
		if err != nil {
			return BulkUpdateResult{}, fmt.Errorf("updating setting %q: %w", si.Key, err)
		}

		if db.RowsAffected(result) == 0 {
			id := xid.New().String()
			_, err = insertStmt.Exec(id, si.Key, si.Value, si.Category, now)
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

// AgentSettings returns the current agent settings from the database.
func (s *Service) AgentSettings() coreSettings.AgentSettings {
	return coreSettings.ReadAgentSettings(s.db)
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

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

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
	}

	for key, value := range kvs {
		if err := upsertInTx(tx, key, value, "agent", now); err != nil {
			return fmt.Errorf("upserting agent setting %q: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// RetentionSettings returns the current retention settings from the database.
func (s *Service) RetentionSettings() coreSettings.RetentionSettings {
	return coreSettings.ReadRetentionSettings(s.db)
}

// UpdateRetentionSettings validates and persists retention settings.
func (s *Service) UpdateRetentionSettings(ctx context.Context, input RetentionSettingsInput) error {
	if input.AuditRetentionDays < 0 || input.AuditRetentionDays > 3650 {
		return fmt.Errorf("audit retention days must be between 0 and 3650")
	}
	if input.ActivityRetentionDays < 0 || input.ActivityRetentionDays > 3650 {
		return fmt.Errorf("activity retention days must be between 0 and 3650")
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	kvs := map[string]string{
		"retention_audit_days":    strconv.Itoa(input.AuditRetentionDays),
		"retention_activity_days": strconv.Itoa(input.ActivityRetentionDays),
	}

	for key, value := range kvs {
		if err := upsertInTx(tx, key, value, "retention", now); err != nil {
			return fmt.Errorf("upserting retention setting %q: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// ScannerSettings returns the current scanner settings from the database.
func (s *Service) ScannerSettings() coreSettings.ScannerSettings {
	return coreSettings.ReadScannerSettings(s.db)
}

// UpdateScannerSettings validates and persists scanner settings.
func (s *Service) UpdateScannerSettings(ctx context.Context, input ScannerSettingsInput) error {
	// Validate
	if input.DefaultScanner != "trivy" && input.DefaultScanner != "grype" && input.DefaultScanner != "clair" {
		return fmt.Errorf("invalid default scanner: %s", input.DefaultScanner)
	}
	if input.ScanTimeout < 30 || input.ScanTimeout > 1800 {
		return fmt.Errorf("scan timeout must be between 30 and 1800")
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	boolStr := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}

	kvs := map[string]string{
		"scanner_trivy_enabled":    boolStr(input.TrivyEnabled),
		"scanner_grype_enabled":    boolStr(input.GrypeEnabled),
		"scanner_clair_enabled":    boolStr(input.ClairEnabled),
		"scanner_clair_url":        input.ClairURL,
		"scanner_default":          input.DefaultScanner,
		"scanner_timeout":          strconv.Itoa(input.ScanTimeout),
		"scanner_scan_on_install":  boolStr(input.ScanOnInstall),
		"scanner_scan_on_update":   boolStr(input.ScanOnUpdate),
	}

	for key, value := range kvs {
		if err := upsertInTx(tx, key, value, "scanners", now); err != nil {
			return fmt.Errorf("upserting scanner setting %q: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// TLSStatus returns the current TLS configuration status.
func (s *Service) TLSStatus(ctx context.Context) TLSStatus {
	status := TLSStatus{}

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

	// Read encrypted cert to extract metadata
	var encCert string
	err = s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", "tls_cert").Scan(&encCert)
	if err == nil && encCert != "" {
		certPEM, decErr := s.encryption.Decrypt(encCert)
		if decErr == nil {
			status.HasCert = true
			status.CertInfo = parseCertInfo(certPEM)
		}
	}

	return status
}

// ErrValidation is returned when input validation fails.
type ErrValidation struct {
	Message string
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
	defer tx.Rollback()

	upsert := func(key, value string) error {
		return upsertInTx(tx, key, value, "tls", now)
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
