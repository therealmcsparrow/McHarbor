// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeys

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// Rotation reminder defaults, in days. A key is due for rotation once it is
// older than the first, and flagged as expiring within the second.
const (
	DefaultRotateAfterDays    = 90
	DefaultExpiringWithinDays = 14
)

// ErrNotRotatable is returned for a key that is revoked or already deleted.
var ErrNotRotatable = errors.New("only an active key can be rotated")

// newKeyMaterial mints a plaintext key, the hash stored for it, and the prefix
// shown in lists. The plaintext leaves the server once, in the response.
func newKeyMaterial() (plain, hash, display string, err error) {
	keyBytes := make([]byte, 24)
	if _, err := randRead(keyBytes); err != nil {
		return "", "", "", fmt.Errorf("generating key bytes: %w", err)
	}
	plain = keyPrefix + hex.EncodeToString(keyBytes)
	sum := sha256.Sum256([]byte(plain))
	return plain, hex.EncodeToString(sum[:]), plain[:12] + "...", nil
}

// Rotate issues a replacement for an active key — same owner, name, roles,
// scopes, environments and lifetime — and revokes the old one in the same
// transaction, so there is never a moment with both or with neither.
func (s *Service) Rotate(ctx context.Context, old *APIKey) (*CreateAPIKeyResult, error) {
	if old == nil || !old.IsActive || old.DeletedAt != nil {
		return nil, ErrNotRotatable
	}
	plain, hash, display, err := newKeyMaterial()
	if err != nil {
		return nil, err
	}
	scopes, err := json.Marshal(nonNil(old.Scopes))
	if err != nil {
		return nil, fmt.Errorf("encoding scopes: %w", err)
	}
	roleIDs := make([]string, 0, len(old.Roles))
	for _, r := range old.Roles {
		roleIDs = append(roleIDs, r.ID)
	}
	nowT := time.Now().UTC()
	now := nowT.Format(time.RFC3339)
	expires := rotatedExpiry(old, nowT)
	id := xid.New().String()

	err = inTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE api_keys SET is_active = 0, updated_at = ? WHERE id = ? AND is_active = 1`, now, old.ID)
		if err != nil {
			return fmt.Errorf("revoking rotated key: %w", err)
		}
		if db.RowsAffected(res) == 0 {
			return ErrNotRotatable
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, scopes, expires_at, is_active, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			id, old.UserID, old.Name, hash, display, string(scopes), expires, now, now); err != nil {
			return fmt.Errorf("inserting rotated key: %w", err)
		}
		if err := replaceRoles(ctx, tx, id, roleIDs, now); err != nil {
			return err
		}
		return replaceEnvironments(ctx, tx, id, old.EnvironmentIDs, now)
	})
	if err != nil {
		return nil, err
	}
	s.rbac.InvalidateAPIKeyCache(old.ID)
	key, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, fmt.Errorf("rotated api key not found")
	}
	return &CreateAPIKeyResult{APIKey: *key, Key: plain}, nil
}

// rotatedExpiry keeps the old key's lifetime rather than its deadline, so a
// 90-day key rotated on day 80 gets 90 fresh days, not 10.
func rotatedExpiry(old *APIKey, now time.Time) *string {
	if old.ExpiresAt == nil || *old.ExpiresAt == "" {
		return nil
	}
	created, err1 := time.Parse(time.RFC3339, old.CreatedAt)
	expires, err2 := time.Parse(time.RFC3339, *old.ExpiresAt)
	if err1 != nil || err2 != nil || !expires.After(created) {
		v := *old.ExpiresAt
		return &v
	}
	v := now.Add(expires.Sub(created)).Format(time.RFC3339)
	return &v
}

// FlagRotation marks active keys older than rotateAfterDays as due and those
// expiring within expiringWithinDays as expiring soon. Revoked and deleted keys
// are never flagged: there is nothing left to rotate.
func FlagRotation(items []APIKey, now time.Time, rotateAfterDays, expiringWithinDays int) {
	if rotateAfterDays <= 0 {
		rotateAfterDays = DefaultRotateAfterDays
	}
	if expiringWithinDays <= 0 {
		expiringWithinDays = DefaultExpiringWithinDays
	}
	for i := range items {
		k := &items[i]
		if created, err := time.Parse(time.RFC3339, k.CreatedAt); err == nil {
			k.AgeDays = int(now.Sub(created).Hours() / 24)
		}
		if k.Status != "active" {
			continue
		}
		k.RotationDue = k.AgeDays >= rotateAfterDays
		if k.ExpiresAt != nil {
			if exp, err := time.Parse(time.RFC3339, *k.ExpiresAt); err == nil {
				k.ExpiringSoon = exp.After(now) && exp.Sub(now) <= time.Duration(expiringWithinDays)*24*time.Hour
			}
		}
	}
}
