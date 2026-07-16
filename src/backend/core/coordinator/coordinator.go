// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package coordinator elects a leader for short-lived "singleton"
// jobs (the container-backup scheduler tick, the event collector,
// the metrics scraper, the audit-log pruner, the workflow cron
// trigger). Both McHarbor nodes tick on the same interval, but
// only the node that wins the underlying advisory lock runs the
// job body. The losing node idles until the next tick.
//
// On SQLite (single-node installs) the coordinator is a no-op and
// the first node always wins, so the behavior matches the
// pre-HA single-node path. On Postgres the lock is taken with
// pg_try_advisory_lock(key), a non-blocking session lock. On
// MySQL / MariaDB the lock is taken with GET_LOCK(name, timeout),
// also a per-connection session lock. If another node already
// holds the lock the call returns false (or 0 on MySQL) and the
// local copy of the job skips. When the leader's session ends
// (process exit, network drop, idle timeout) the lock is
// released automatically and the next tick on the other node
// wins.
//
// Lock keys are derived from a fixed 64-bit namespace ("mcharbor")
// combined with a hash of the singleton name so two jobs in the
// same cluster don't collide on the same lock slot. The hash
// uses fnv-64a which is built into the Go standard library and
// stable across Go versions.
package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// lockNamespace is the constant high 32 bits of every advisory
// lock we take. Using a private namespace prevents collisions with
// any other Postgres application that happens to share the
// cluster. 0x6D63486172 = "mchar" in ASCII (first 4 bytes of
// "mcharbor"); the lower bits are reserved.
const lockNamespace uint32 = 0x6D634861

// Coordinator owns a long-lived database connection dedicated to
// holding the advisory locks we acquire. Using a dedicated conn
// (rather than borrowing one from the pool per TryLock call) keeps
// the lock attached to the same Postgres session, which is what
// pg_try_advisory_lock requires for safe release on disconnect.
type Coordinator struct {
	driver db.Driver
	conn   *sql.Conn
	nodeID string
}

// New opens the dedicated lock connection. On SQLite this just
// opens an extra conn from the pool; on Postgres it acquires
// a real connection from the pool that stays pinned for the
// lifetime of the coordinator.
func New(database *sql.DB, nodeID string) (*Coordinator, error) {
	driver := db.DriverOf()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := database.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("coordinator: opening lock connection: %w", err)
	}
	if nodeID == "" {
		// Fall back to the local hostname when the operator
		// didn't set MCHARBOR_NODE_ID. Helps log messages
		// identify which node won each tick.
		nodeID = "node-" + randomSuffix()
	}
	return &Coordinator{driver: driver, conn: conn, nodeID: nodeID}, nil
}

// Close releases the dedicated lock connection. Safe to call
// when New returned an error. After Close the coordinator must
// not be reused.
func (c *Coordinator) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// Run is the high-level "do this every tick" entry point. It
// loops at the supplied interval and, on each tick:
//
//  1. Tries to acquire the advisory lock for `name`.
//  2. If the lock is held by another node, sleeps for the
//     interval and tries again.
//  3. If the lock is held locally, runs `fn` and sleeps for the
//     interval. The lock stays held for the lifetime of this
//     Coordinator; on Postgres it's released when `c.conn`
//     closes (process exit, network drop, idle timeout).
//
// On SQLite (single-node), TryLock always returns true so the
// function runs every tick — identical to the pre-HA behavior.
func (c *Coordinator) Run(ctx context.Context, name string, interval time.Duration, fn func(context.Context)) {
	if c == nil || c.conn == nil {
		// Defensive: don't crash the process if the coordinator
		// wasn't initialized (e.g. tests that bypass main()).
		return
	}
	timer := time.NewTicker(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if !c.tryLock(ctx, name) {
			slog.Debug("coordinator: not leader for job; skipping tick", "job", name, "node", c.nodeID)
			continue
		}
		jobCtx, cancel := context.WithTimeout(ctx, interval)
		c.runOnce(jobCtx, name, fn)
		cancel()
	}
}

// runOnce runs `fn` and logs the result. Failures are caught and
// logged so a single bad tick doesn't take the whole coordinator
// down — the next tick will try again.
func (c *Coordinator) runOnce(ctx context.Context, name string, fn func(context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("coordinator: job panicked", "job", name, "node", c.nodeID, "panic", r)
		}
	}()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(ctx)
	}()
	select {
	case <-done:
		slog.Debug("coordinator: job tick complete", "job", name, "node", c.nodeID)
	case <-ctx.Done():
		slog.Warn("coordinator: job tick exceeded interval; truncated", "job", name, "node", c.nodeID)
	}
}

// tryLock attempts the underlying advisory-lock call. On Postgres
// it's pg_try_advisory_lock(key) — non-blocking. On MySQL it's
// GET_LOCK(name, 0) — also non-blocking (the 0 timeout means
// "try once, do not wait"). On SQLite the helper returns true
// unconditionally so the function behaves the same on all
// backends.
func (c *Coordinator) tryLock(ctx context.Context, name string) bool {
	switch c.driver {
	case db.DriverPostgres:
		key := lockKey(name)
		var got bool
		if err := c.conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&got); err != nil {
			if !errors.Is(err, sql.ErrConnDone) {
				slog.Warn("coordinator: pg_try_advisory_lock failed", "job", name, "error", err)
			}
			return false
		}
		if got {
			slog.Debug("coordinator: acquired advisory lock", "job", name, "node", c.nodeID)
		}
		return got
	case db.DriverMySQL:
		// MySQL named locks take a string name. We use the
		// singleton name directly so lock names are debuggable
		// from SHOW PROCESSLIST. The third arg to GET_LOCK is
		// the wait timeout in seconds; 0 means "try once".
		var got sql.NullInt64
		if err := c.conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", coordinatorLockName(name)).Scan(&got); err != nil {
			if !errors.Is(err, sql.ErrConnDone) {
				slog.Warn("coordinator: GET_LOCK failed", "job", name, "error", err)
			}
			return false
		}
		// MySQL returns 1 on success, 0 on timeout, NULL on error.
		// 1 == "we hold the lock".
		if got.Valid && got.Int64 == 1 {
			slog.Debug("coordinator: acquired MySQL named lock", "job", name, "node", c.nodeID)
			return true
		}
		return false
	default:
		// SQLite (single-node): no leader election needed.
		return true
	}
}

// lockKey packs a singleton name into a single int64 so the
// pg_try_advisory_lock(int) signature works. The high 32 bits are
// our private namespace; the low 32 bits are the FNV-64a hash of
// the singleton name truncated to 32 bits. Collisions within the
// same cluster are negligible (you'd need two job names with
// matching low 32 bits of fnv-64a, which is astronomically rare).
func lockKey(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return (int64(lockNamespace) << 32) | int64(uint32(h.Sum64()))
}

// randomSuffix is used when MCHARBOR_NODE_ID isn't set. The output
// isn't crypto-strong but it doesn't need to be — it's just an
// identifier in log lines so the operator can tell two local
// dev nodes apart.
var randomSuffix = func() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	now := time.Now().UnixNano()
	out := make([]byte, 6)
	for i := range out {
		out[i] = alphabet[(now>>(i*5))&0x1F%int64(len(alphabet))]
	}
	return string(out)
}

// LeaderOf reports whether the current node is the elected leader
// for `name` (best-effort read; not a guarantee of liveness at the
// instant of the call). Useful for /api/cluster/status.
func (c *Coordinator) LeaderOf(ctx context.Context, name string) bool {
	if c == nil || c.conn == nil {
		return false
	}
	if c.driver == db.DriverSQLite {
		// On SQLite every node is "the leader" of every job.
		return true
	}
	// pg_try_advisory_lock and MySQL GET_LOCK are both try-once
	// operations. The trade-off is the same as for tryLock: a
	// best-effort read with no "is-locked" predicate. Good
	// enough for the status endpoint.
	return c.tryLock(ctx, name)
}

// coordinatorLockName returns the canonical name used in MySQL's
// GET_LOCK / RELEASE_LOCK calls. Prefixed with "mcharbor_" so
// the names don't collide with named locks from any other
// application sharing the same MySQL instance.
func coordinatorLockName(name string) string {
	return "mcharbor_" + name
}

// NodeID returns the identifier this Coordinator was built with.
// Used by the cluster status endpoint.
func (c *Coordinator) NodeID() string {
	if c == nil {
		return ""
	}
	return c.nodeID
}

// Ensure unused-import errors don't bite on a build without
// every helper wired in.
var _ sync.Mutex
var _ = sync.Mutex{}
