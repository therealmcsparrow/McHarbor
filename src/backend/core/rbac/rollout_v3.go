// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// RolloutV3 mirrors Rollout for the ABAC + SCIM feature set. Same
// shape and TTL — kept separate so a v2 rollback does not also flip
// v3 off. When both flags are off, Authorize() runs without ABAC.
type RolloutV3 struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache bool
	ts    time.Time
	ttl   time.Duration
}

// NewRolloutV3 constructs a rollout watcher backed by the settings table.
func NewRolloutV3(db *sql.DB) *RolloutV3 {
	return &RolloutV3{db: db, ttl: 5 * time.Second}
}

// IsEnabled returns true when the rbac_v3_enabled setting is "true".
// The result is cached for 5s; call Invalidate() after a settings write.
func (r *RolloutV3) IsEnabled() bool {
	return r.isEnabled(context.Background())
}

// isEnabled is IsEnabled with the caller's context for the settings read.
func (r *RolloutV3) isEnabled(ctx context.Context) bool {
	r.mu.RLock()
	if time.Since(r.ts) < r.ttl {
		out := r.cache
		r.mu.RUnlock()
		return out
	}
	r.mu.RUnlock()

	enabled := false
	if r.db != nil {
		var v string
		err := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'rbac_v3_enabled'`).Scan(&v)
		if err == nil && (v == "true" || v == "1") {
			enabled = true
		}
	}

	r.mu.Lock()
	r.cache = enabled
	r.ts = time.Now()
	r.mu.Unlock()
	return enabled
}

// Invalidate clears the cached value so the next IsEnabled call
// re-reads the settings table.
func (r *RolloutV3) Invalidate() {
	r.mu.Lock()
	r.cache = false
	r.ts = time.Time{}
	r.mu.Unlock()
}
