// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"
)

// CompactManager coordinates database compaction so that:
//   - PRAGMA wal_checkpoint(TRUNCATE) is run synchronously after a
//     retention purge so the WAL file is freed back to disk immediately.
//   - VACUUM (which rewrites the entire database file and is very
//     expensive on large DBs) runs in a background goroutine.
//   - Only one VACUUM runs at a time. SQLite does not support
//     concurrent writes, so a second trigger while one is in progress
//     becomes a no-op and is reported back as such.
//
// On platforms where VACUUM rewrites the whole file this is the
// difference between an O(seconds) DELETE and an O(minutes) wait. The
// endpoint returns immediately after the WAL checkpoint; the user
// gets a count of deleted rows in the response and a follow-up
// structured log line when the vacuum finishes.
type CompactManager struct {
	db     *sql.DB
	logger *slog.Logger

	mu       sync.Mutex
	running  bool
	lastAt   time.Time
	lastSize int64
}

// NewCompactManager creates a new CompactManager bound to db.
func NewCompactManager(db *sql.DB, logger *slog.Logger) *CompactManager {
	return &CompactManager{db: db, logger: logger}
}

// VacuumState describes the current state of the background vacuum.
type VacuumState struct {
	Running     bool      `json:"running"`
	LastRunAt   time.Time `json:"lastRunAt"`
	LastBytes   int64     `json:"lastBytes"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
}

// State returns a snapshot of the vacuum state for status endpoints.
func (c *CompactManager) State() VacuumState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return VacuumState{
		Running:   c.running,
		LastRunAt: c.lastAt,
		LastBytes: c.lastSize,
	}
}

// CheckpointAndVacuum runs PRAGMA wal_checkpoint(TRUNCATE) on the
// calling goroutine and then spawns a background VACUUM if one is
// not already running.
//
// Returns immediately with the current vacuum state.
func (c *CompactManager) CheckpointAndVacuum(ctx context.Context) VacuumState {
	if _, err := c.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		c.logger.Error("wal checkpoint failed", "error", err)
	}

	c.mu.Lock()
	if c.running {
		state := VacuumState{Running: true, LastRunAt: c.lastAt, LastBytes: c.lastSize}
		c.mu.Unlock()
		return state
	}
	c.running = true
	startedAt := time.Now()
	c.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				c.logger.Error("vacuum panicked", "panic", r)
			}
			c.mu.Lock()
			c.running = false
			c.lastAt = time.Now()
			c.mu.Unlock()
		}()

		c.logger.Info("starting database vacuum", "started_at", startedAt)
		if _, err := c.db.Exec("VACUUM"); err != nil {
			c.logger.Error("vacuum failed", "error", err, "duration", time.Since(startedAt))
			return
		}

		var pageSize, pageCount int64
		_ = c.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
		_ = c.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
		size := pageSize * pageCount
		c.mu.Lock()
		c.lastSize = size
		c.mu.Unlock()
		c.logger.Info(
			"vacuum completed",
			"duration", time.Since(startedAt),
			"size_bytes", size,
		)
	}()

	return VacuumState{Running: true, StartedAt: startedAt}
}
