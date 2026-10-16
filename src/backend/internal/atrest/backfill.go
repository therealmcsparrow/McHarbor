// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package atrest encrypts credential columns that still hold plaintext written before
// their module encrypted on write. It lives under internal/ so no plugin can link it.
package atrest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// ErrEncryptionUnavailable means the backfill was asked to run without the host
// encryption service, so it did nothing.
var ErrEncryptionUnavailable = errors.New("atrest: encryption service is not configured")

// batchSize bounds one read and one transaction, so a large table is walked in pages
// rather than held in memory or locked for the whole pass.
const batchSize = 500

// Column names one credential column whose module reads it through Decrypt, so a
// whole-value ciphertext is what it already expects. Every identifier is a constant
// from Columns, never caller input.
type Column struct {
	Table string
	ID    string
	Value string
	// Where narrows the rows, for a table that holds credentials beside ordinary
	// values (settings). It is a fixed SQL literal from Columns.
	Where string
	// Keep lists values the writer stores unencrypted on purpose, such as "{}".
	Keep []string
}

// Columns is every column the matrix marks encrypted whose reader decrypts the whole
// value. Columns encrypted per field inside JSON, or owned by a module backfill (stack
// env vars, snapshot configs, desired states), are deliberately absent.
var Columns = []Column{
	{Table: "environments", ID: "id", Value: "tls_ca"},
	{Table: "environments", ID: "id", Value: "tls_cert"},
	{Table: "environments", ID: "id", Value: "tls_key"},
	{Table: "environments", ID: "id", Value: "ssh_key"},
	{Table: "environments", ID: "id", Value: "agent_token"},
	{Table: "registries", ID: "id", Value: "password"},
	{Table: "settings", ID: "id", Value: "value", Where: "key IN ('tls_cert', 'tls_key', 'webpush_vapid_private')"},
	{Table: "webhooks", ID: "id", Value: "secret"},
	{Table: "git_repos", ID: "id", Value: "credential_ref"},
	{Table: "git_repos", ID: "id", Value: "webhook_secret"},
	{Table: "workflows", ID: "id", Value: "variables"},
	{Table: "workflows", ID: "id", Value: "canvas_data"},
	{Table: "identity_providers", ID: "id", Value: "client_secret"},
	{Table: "identity_providers", ID: "id", Value: "sp_private_key"},
	{Table: "scim_endpoints", ID: "id", Value: "encrypted_token"},
	{Table: "stack_webhooks", ID: "id", Value: "secret"},
	{Table: "communication_channels", ID: "id", Value: "config_enc"},
	{Table: "communication_channels", ID: "id", Value: "webhook_secret_enc"},
	{Table: "communication_channels", ID: "id", Value: "webhook_token_enc"},
	{Table: "storage_locations", ID: "id", Value: "password"},
	{Table: "storage_locations", ID: "id", Value: "access_key_id"},
	{Table: "storage_locations", ID: "id", Value: "secret_access_key"},
	{Table: "storage_locations", ID: "id", Value: "client_secret"},
	{Table: "storage_locations", ID: "id", Value: "refresh_token"},
	{Table: "storage_locations", ID: "id", Value: "token"},
	{Table: "storage_locations", ID: "id", Value: "private_key"},
	{Table: "storage_locations", ID: "id", Value: "passphrase"},
	{Table: "storage_locations", ID: "id", Value: "ca_certificate"},
	{Table: "storage_locations", ID: "id", Value: "client_certificate"},
	{Table: "storage_locations", ID: "id", Value: "client_key"},
	{Table: "storage_locations", ID: "id", Value: "config", Keep: []string{"{}"}},
	{Table: "user_mfa", ID: "user_id", Value: "secret_encrypted"},
	{Table: "user_mfa", ID: "user_id", Value: "recovery_codes_encrypted"},
	{Table: "secrets", ID: "id", Value: "value"},
	{Table: "secret_versions", ID: "id", Value: "value"},
	{Table: "ai_provider_connections", ID: "id", Value: "api_key"},
	{Table: "ai_provider_connections", ID: "id", Value: "oauth_client_secret"},
	{Table: "ai_provider_connections", ID: "id", Value: "oauth_access_token"},
	{Table: "ai_provider_connections", ID: "id", Value: "oauth_refresh_token"},
	{Table: "ai_provider_connections", ID: "id", Value: "oauth_pending_device_code"},
	{Table: "licenses", ID: "id", Value: "license_key"},
	{Table: "license_installs", ID: "id", Value: "install_key"},
	{Table: "push_subscriptions", ID: "id", Value: "endpoint"},
	{Table: "push_subscriptions", ID: "id", Value: "p256dh"},
	{Table: "push_subscriptions", ID: "id", Value: "auth_secret"},
	{Table: "instance_backup_schedules", ID: "id", Value: "key_secret_enc"},
	{Table: "approval_requests", ID: "id", Value: "body_encrypted"},
}

// Result is what one column's pass did. Skipped counts rows a concurrent writer
// changed between the read and the write, which keep the newer value.
type Result struct {
	Table     string
	Column    string
	Encrypted int
	Skipped   int
	Missing   bool
}

// Backfill encrypts every plaintext value in columns. It is idempotent: a value that
// already carries the ciphertext prefix, an empty one, NULL, or a Keep value is left
// alone, so a second run changes nothing. One column's failure does not stop the rest.
func Backfill(ctx context.Context, database *sql.DB, enc *encryption.Service, columns []Column) ([]Result, error) {
	if enc == nil {
		return nil, ErrEncryptionUnavailable
	}
	results := make([]Result, 0, len(columns))
	var errs []error
	for _, col := range columns {
		res, err := backfillColumn(ctx, database, enc, col)
		results = append(results, res)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return results, errors.Join(errs...)
}

// Run is Backfill over Columns with one summary log line per column that had work or
// failed. It never returns an error: a credential left in plaintext must not stop the
// server from starting, and the next boot retries it.
func Run(ctx context.Context, database *sql.DB, enc *encryption.Service, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	results, err := Backfill(ctx, database, enc, Columns)
	if errors.Is(err, ErrEncryptionUnavailable) {
		logger.Warn("credential at-rest backfill skipped: no encryption service")
		return
	}
	for _, res := range results {
		if res.Missing {
			logger.Warn("credential at-rest backfill: column not in schema", "table", res.Table, "column", res.Column)
			continue
		}
		if res.Encrypted == 0 && res.Skipped == 0 {
			continue
		}
		logger.Info("credential at-rest backfill", "table", res.Table, "column", res.Column,
			"encrypted", res.Encrypted, "skipped", res.Skipped)
	}
	if err != nil {
		logger.Error("credential at-rest backfill incomplete", "error", err)
	}
}

// columnExists reports whether table has column, so a table or column a later
// migration drops is skipped rather than failing the pass.
func columnExists(ctx context.Context, database *sql.DB, table, column string) bool {
	cols, err := db.TableColumns(ctx, database, db.DriverOf(), table)
	if err != nil {
		return false
	}
	for _, name := range cols {
		if strings.EqualFold(name, column) {
			return true
		}
	}
	return false
}

// pendingRow is one plaintext value awaiting encryption.
type pendingRow struct{ id, value string }

// pendingPage reads the next page of plaintext values after afterID and releases the
// cursor before returning, so the caller can write on a single-connection database.
func pendingPage(ctx context.Context, database *sql.DB, col Column, afterID string) ([]pendingRow, error) {
	where := ""
	if col.Where != "" {
		where = " AND (" + col.Where + ")"
	}
	query := `SELECT ` + col.ID + `, ` + col.Value + ` FROM ` + col.Table + // SQL identifier: table, id and value columns are constants from Columns
		` WHERE ` + col.Value + ` IS NOT NULL AND ` + col.Value + ` <> '' AND ` + col.Value + ` NOT LIKE 'enc:v1:%'` + // SQL identifier: constants from Columns
		` AND ` + col.ID + ` > ?` + where + ` ORDER BY ` + col.ID + ` LIMIT ?` // SQL identifier: constants from Columns
	rows, err := database.QueryContext(ctx, query, afterID, batchSize)
	if err != nil {
		return nil, fmt.Errorf("reading %s.%s: %w", col.Table, col.Value, err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []pendingRow
	for rows.Next() {
		var row pendingRow
		if err := rows.Scan(&row.id, &row.value); err != nil {
			return nil, fmt.Errorf("reading %s.%s: %w", col.Table, col.Value, err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// kept reports whether value is one the writer leaves unencrypted on purpose.
func kept(col Column, value string) bool {
	for _, k := range col.Keep {
		if strings.TrimSpace(value) == k {
			return true
		}
	}
	return false
}

// backfillColumn walks one column page by page, one transaction per page. Each write
// is conditional on the value read, so another node or a module writer that changed
// the row in between keeps its value.
func backfillColumn(ctx context.Context, database *sql.DB, enc *encryption.Service, col Column) (Result, error) {
	res := Result{Table: col.Table, Column: col.Value}
	if !columnExists(ctx, database, col.Table, col.Value) {
		res.Missing = true
		return res, nil
	}
	afterID := ""
	for {
		page, err := pendingPage(ctx, database, col, afterID)
		if err != nil {
			return res, err
		}
		if len(page) == 0 {
			return res, nil
		}
		encrypted, skipped, err := writePage(ctx, database, enc, col, page)
		res.Encrypted += encrypted
		res.Skipped += skipped
		if err != nil {
			return res, err
		}
		afterID = page[len(page)-1].id
		if len(page) < batchSize {
			return res, nil
		}
	}
}

// writePage encrypts one page in one transaction. A row whose value no longer matches
// what was read is counted as skipped and keeps the newer value.
func writePage(ctx context.Context, database *sql.DB, enc *encryption.Service, col Column, page []pendingRow) (encrypted, skipped int, err error) {
	update := `UPDATE ` + col.Table + ` SET ` + col.Value + ` = ? WHERE ` + col.ID + ` = ? AND ` + col.Value + ` = ?` // SQL identifier: constants from Columns
	err = db.TxContext(ctx, database, func(tx *sql.Tx) error {
		encrypted, skipped = 0, 0
		for _, row := range page {
			if kept(col, row.value) || encryption.IsEncrypted(row.value) {
				continue
			}
			cipher, err := enc.Encrypt(row.value)
			if err != nil {
				return fmt.Errorf("encrypting %s.%s %s: %w", col.Table, col.Value, row.id, err)
			}
			result, err := tx.ExecContext(ctx, update, cipher, row.id, row.value)
			if err != nil {
				return fmt.Errorf("writing %s.%s %s: %w", col.Table, col.Value, row.id, err)
			}
			if db.RowsAffected(result) == 1 {
				encrypted++
			} else {
				skipped++
			}
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return encrypted, skipped, nil
}
