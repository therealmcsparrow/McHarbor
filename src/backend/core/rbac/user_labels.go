// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"strings"
)

// Username resolves a user id to its username, empty when the lookup fails.
//
// Audit rows whose subject is an account need a name the reader recognises;
// the actor column on the row names the admin, not the account being changed.
func (s *Service) Username(ctx context.Context, userID string) string {
	var name string
	if err := s.db.QueryRowContext(ctx,
		`SELECT username FROM users WHERE id = ? LIMIT 1`, userID,
	).Scan(&name); err != nil {
		return ""
	}
	return name
}

// UserLabels returns the (key → value) free-form labels attached
// to a user. Labels are admin-only metadata; the rest of the
// permission engine ignores them.
func (s *Service) UserLabels(ctx context.Context, userID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM user_labels WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetUserLabels atomically replaces a user's label set. Empty/whitespace
// keys are skipped. The operation runs in a transaction so partial
// updates never leak to other readers.
func (s *Service) SetUserLabels(ctx context.Context, userID string, labels map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer coredb.RollbackTx(tx)
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_labels WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for k, v := range labels {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_labels (user_id, key, value) VALUES (?, ?, ?)`,
			userID, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetUserLabelsOnTx is the same as SetUserLabels but executed on a
// caller-provided transaction. The transaction is NOT committed
// here; the caller owns the commit/rollback boundary so this can
// be composed with other label-adjacent writes.
func (s *Service) SetUserLabelsOnTx(ctx context.Context, tx *sql.Tx, userID string, labels map[string]string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_labels WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for k, v := range labels {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_labels (user_id, key, value) VALUES (?, ?, ?)`,
			userID, k, v); err != nil {
			return err
		}
	}
	return nil
}
