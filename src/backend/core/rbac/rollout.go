// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"
)

// Rollout controls whether the v2 RBAC middleware (Authoritative, with
// owner rule + permission overrides) is the active gate. When the
// rollout flag is off, the legacy middleware continues to serve all
// routes — admins can audit the new model without breaking existing
// users.
//
// The flag is stored in the settings table under
// `rbac_v2_enabled` (seeded in migration 070; defaults adjusted for
// upgrades in migration 074). The default is 'true' for fresh installs
// and 'false' for upgrades so admins can opt in deliberately.
type Rollout struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache bool
	ts    time.Time
	ttl   time.Duration
}

// NewRollout creates a rollout state. The first IsEnabled() call reads
// from the database; subsequent reads within the TTL hit an in-memory
// cache so we don't query the settings table on every request.
func NewRollout(db *sql.DB) *Rollout {
	return &Rollout{db: db, ttl: 5 * time.Second}
}

// IsEnabled returns whether the v2 route gates are active. The check
// is cached for ttl so the hot path is a single mutex lookup.
func (r *Rollout) IsEnabled() bool {
	if r == nil || r.db == nil {
		return false
	}
	r.mu.RLock()
	if time.Since(r.ts) < r.ttl {
		v := r.cache
		r.mu.RUnlock()
		return v
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	// Re-check after acquiring the write lock in case another
	// goroutine refreshed the cache in the meantime.
	if time.Since(r.ts) < r.ttl {
		return r.cache
	}
	ctx, cancel := context.WithTimeout(context.Background(), authzQueryTimeout)
	defer cancel()
	var raw string
	err := r.db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = 'rbac_v2_enabled'`).Scan(&raw)
	if err != nil {
		// Default to enabled on missing row (fresh install) — the
		// migration seeds the value either way.
		slog.Warn("rbac: rollout flag missing, defaulting to enabled", "error", err)
		r.cache = true
		r.ts = time.Now()
		return true
	}
	r.cache = raw == "true" || raw == "1"
	r.ts = time.Now()
	return r.cache
}

// Invalidate drops the cached value so the next IsEnabled() call
// re-reads from the database. Call this after an admin flips the
// flag in Settings.
func (r *Rollout) Invalidate() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.ts = time.Time{}
	r.mu.Unlock()
}
