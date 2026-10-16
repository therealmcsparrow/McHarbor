// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const wormDeleteAllowedKey = "audit.worm_delete_allowed"

// WithSanctionedDelete temporarily enables audit-log deletes for retention
// purge/reset flows while WORM protection is active.
func WithSanctionedDelete(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning audit delete transaction: %w", err)
	}
	defer rollbackLogged(tx)

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO settings (id, key, value, category, created_at, updated_at)
		VALUES (?, ?, '1', 'audit', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, "setting_audit_worm_delete_allowed", wormDeleteAllowedKey, now, now); err != nil {
		return fmt.Errorf("enabling sanctioned audit delete: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	now = time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, "UPDATE settings SET value = '0', updated_at = ? WHERE key = ?", now, wormDeleteAllowedKey); err != nil {
		return fmt.Errorf("disabling sanctioned audit delete: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing audit delete transaction: %w", err)
	}
	return nil
}
