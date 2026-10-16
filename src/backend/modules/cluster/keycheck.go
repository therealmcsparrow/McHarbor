// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rs/xid"
)

const keyCheckName = "encryption_key_check"

// ErrEncryptionKeyMismatch means this node's data-encryption key is not the one
// the database was written with, so every encrypted column would read as garbage.
var ErrEncryptionKeyMismatch = errors.New("cluster: this node's encryption key does not match the database")

// KeyHasher is the slice of the encryption service the check needs: a keyed digest
// that proves which key is loaded without ever storing the key.
type KeyHasher interface {
	StableHash(value string) string
}

// KeyCheckResult says what VerifyEncryptionKey found, so the caller decides whether a
// mismatch is fatal for its deployment.
type KeyCheckResult struct {
	Recorded bool
	Match    bool
}

// VerifyEncryptionKey records a digest of the key on first boot and compares it on
// every later one; reset overwrites the digest after a deliberate key change.
func VerifyEncryptionKey(ctx context.Context, db *sql.DB, enc KeyHasher, reset bool) (KeyCheckResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	digest := enc.StableHash("mcharbor-key-check")
	var stored string
	err := db.QueryRowContext(ctx, `SELECT value FROM cluster_state WHERE name = ?`, keyCheckName).Scan(&stored)
	now := time.Now().UTC().Format(time.RFC3339)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := db.ExecContext(ctx, `INSERT INTO cluster_state (id, name, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			xid.New().String(), keyCheckName, digest, now, now); err != nil {
			return KeyCheckResult{}, fmt.Errorf("cluster: recording key check: %w", err)
		}
		return KeyCheckResult{Recorded: true, Match: true}, nil
	case err != nil:
		return KeyCheckResult{}, fmt.Errorf("cluster: reading key check: %w", err)
	case stored == digest:
		return KeyCheckResult{Match: true}, nil
	case reset:
		if _, err := db.ExecContext(ctx, `UPDATE cluster_state SET value = ?, updated_at = ? WHERE name = ?`, digest, now, keyCheckName); err != nil {
			return KeyCheckResult{}, fmt.Errorf("cluster: resetting key check: %w", err)
		}
		return KeyCheckResult{Recorded: true, Match: true}, nil
	}
	return KeyCheckResult{}, ErrEncryptionKeyMismatch
}
