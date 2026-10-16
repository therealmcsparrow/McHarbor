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
// and on SQL Server with sp_getapplock, both also per-connection
// session locks. If another node already holds the lock the call
// returns false (0 on MySQL, a negative code on SQL Server) and
// the local copy of the job skips. When the leader's session ends
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
	db     *sql.DB
	nodeID string

	mu       sync.Mutex
	conn     *sql.Conn
	held     map[string]bool
	eligible func(name string) bool
}

// SetEligible installs a gate consulted before every lock attempt; a standby node
// uses it to compete only for the active lock and win no singleton job.
func (c *Coordinator) SetEligible(fn func(name string) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eligible = fn
}

func (c *Coordinator) isEligible(name string) bool {
	c.mu.Lock()
	fn := c.eligible
	c.mu.Unlock()
	return fn == nil || fn(name)
}

// ReleaseAll gives up every lock this node holds in one call. Demotion needs it: a
// standby that kept a job lock would stop that job running anywhere.
func (c *Coordinator) ReleaseAll(ctx context.Context) error {
	if c == nil || !c.locking() {
		return nil
	}
	conn, err := c.lockConn(ctx)
	if err != nil {
		return err
	}
	if c.driver == db.DriverMSSQL {
		return c.releaseEachMSSQL(ctx, conn)
	}
	query := "SELECT pg_advisory_unlock_all()"
	if c.driver == db.DriverMySQL {
		query = "SELECT RELEASE_ALL_LOCKS()"
	}
	if _, err := conn.ExecContext(ctx, query); err != nil {
		c.dropConn(conn)
		return fmt.Errorf("coordinator: releasing locks: %w", err)
	}
	c.mu.Lock()
	c.held = map[string]bool{}
	c.mu.Unlock()
	return nil
}

// releaseEachMSSQL releases the application locks one at a time, because SQL Server has
// no equivalent of pg_advisory_unlock_all or RELEASE_ALL_LOCKS. A name that has already
// gone with a dropped session is not an error worth failing a demotion over.
func (c *Coordinator) releaseEachMSSQL(ctx context.Context, conn *sql.Conn) error {
	c.mu.Lock()
	names := make([]string, 0, len(c.held))
	for name := range c.held {
		names = append(names, name)
	}
	c.mu.Unlock()
	// bounded by the application locks this node holds, one per coordinated job
	for _, name := range names {
		_, err := conn.ExecContext(ctx,
			"EXEC sp_releaseapplock @Resource = ?, @LockOwner = 'Session'",
			coordinatorLockName(name))
		if err != nil {
			slog.Warn("coordinator: releasing application lock", "job", name, "error", err)
		}
	}
	c.mu.Lock()
	c.held = map[string]bool{}
	c.mu.Unlock()
	return nil
}

// locking reports a backend with a real named-lock primitive. SQLite has none and needs
// none: a single-node install is always its own leader.
func (c *Coordinator) locking() bool {
	switch c.driver {
	case db.DriverPostgres, db.DriverMySQL, db.DriverMSSQL:
		return true
	}
	return false
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
	return &Coordinator{driver: driver, db: database, conn: conn, nodeID: nodeID, held: map[string]bool{}}, nil
}

// Close releases the dedicated lock connection. Safe to call
// when New returned an error. After Close the coordinator must
// not be reused.
func (c *Coordinator) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.db = nil
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// lockConn returns the pinned lock connection, reopening it when a previous call
// dropped it; a coordinator that never reconnected could never lead again.
func (c *Coordinator) lockConn(ctx context.Context) (*sql.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn, nil
	}
	if c.db == nil {
		return nil, sql.ErrConnDone
	}
	conn, err := c.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("coordinator: reopening lock connection: %w", err)
	}
	c.conn = conn
	slog.Info("coordinator: lock connection reopened", "node", c.nodeID)
	return conn, nil
}

// dropConn discards a failed lock connection. Advisory and named locks are session
// scoped, so every lock this node held went with it.
func (c *Coordinator) dropConn(conn *sql.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != conn || c.conn == nil {
		return
	}
	if err := c.conn.Close(); err != nil {
		slog.Debug("coordinator: closing failed lock connection", "error", err)
	}
	c.conn = nil
	c.held = map[string]bool{}
}

func (c *Coordinator) setHeld(name string, held bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.held == nil {
		c.held = map[string]bool{}
	}
	if held {
		c.held[name] = true
	} else {
		delete(c.held, name)
	}
}

// Holds reports whether this node currently owns the lock for name, from memory and
// without touching the database, so reading cluster status never changes leadership.
func (c *Coordinator) Holds(name string) bool {
	if c == nil {
		return false
	}
	if c.driver == db.DriverSQLite {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.held[name]
}

// Ping checks the lock session itself, which is what a readiness probe needs: a node
// whose lock connection is gone cannot take part in leader election.
func (c *Coordinator) Ping(ctx context.Context) error {
	if c == nil {
		return sql.ErrConnDone
	}
	conn, err := c.lockConn(ctx)
	if err != nil {
		return err
	}
	if err := conn.PingContext(ctx); err != nil {
		c.dropConn(conn)
		return fmt.Errorf("coordinator: lock connection: %w", err)
	}
	return nil
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
	if c == nil || c.db == nil {
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
// GET_LOCK(name, 0) and on SQL Server sp_getapplock with
// @LockTimeout = 0 — also non-blocking ("try once, do not
// wait"). On SQLite the helper returns true
// unconditionally so the function behaves the same on all
// backends.
func (c *Coordinator) tryLock(ctx context.Context, name string) bool {
	if !c.locking() {
		return true
	}
	if !c.isEligible(name) {
		return false
	}
	conn, err := c.lockConn(ctx)
	if err != nil {
		slog.Warn("coordinator: no lock connection", "job", name, "error", err)
		return false
	}
	got, err := c.tryLockOn(ctx, conn, name)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			slog.Warn("coordinator: lock call failed; reopening the lock connection", "job", name, "error", err)
			c.dropConn(conn)
		}
		return false
	}
	c.setHeld(name, got)
	return got
}

func (c *Coordinator) tryLockOn(ctx context.Context, conn *sql.Conn, name string) (bool, error) {
	switch c.driver {
	case db.DriverPostgres:
		key := lockKey(name)
		var got bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&got); err != nil {
			return false, err
		}
		if got {
			slog.Debug("coordinator: acquired advisory lock", "job", name, "node", c.nodeID)
		}
		return got, nil
	case db.DriverMySQL:
		// MySQL named locks take a string name. We use the
		// singleton name directly so lock names are debuggable
		// from SHOW PROCESSLIST. The third arg to GET_LOCK is
		// the wait timeout in seconds; 0 means "try once".
		var got sql.NullInt64
		if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", coordinatorLockName(name)).Scan(&got); err != nil {
			return false, err
		}
		// MySQL returns 1 on success, 0 on timeout, NULL on error.
		// 1 == "we hold the lock".
		if got.Valid && got.Int64 == 1 {
			slog.Debug("coordinator: acquired MySQL named lock", "job", name, "node", c.nodeID)
			return true, nil
		}
		return false, nil
	case db.DriverMSSQL:
		// sp_getapplock is a procedure, not a function, so its return code has to come
		// back through a local variable. @LockOwner = 'Session' ties the lock to this
		// connection, which is why the coordinator pins one; @LockTimeout = 0 makes it
		// try once. 0 and 1 both mean granted; every negative code means not granted.
		var code sql.NullInt64
		const getAppLock = "DECLARE @result int; " +
			"EXEC @result = sp_getapplock @Resource = ?, @LockMode = 'Exclusive', " +
			"@LockOwner = 'Session', @LockTimeout = 0; SELECT @result"
		if err := conn.QueryRowContext(ctx, getAppLock, coordinatorLockName(name)).Scan(&code); err != nil {
			return false, err
		}
		if code.Valid && code.Int64 >= 0 {
			slog.Debug("coordinator: acquired SQL Server application lock", "job", name, "node", c.nodeID)
			return true, nil
		}
		return false, nil
	}
	return true, nil
}

// lockKey packs a singleton name into a single int64 so the
// pg_try_advisory_lock(int) signature works. The high 32 bits are
// our private namespace; the low 32 bits are the FNV-64a hash of
// the singleton name truncated to 32 bits. Collisions within the
// same cluster are negligible (you'd need two job names with
// matching low 32 bits of fnv-64a, which is astronomically rare).
func lockKey(name string) int64 {
	const offset, prime = 14695981039346656037, 1099511628211
	h := uint64(offset)
	for i := 0; i < len(name); i++ {
		h ^= uint64(name[i])
		h *= prime
	}
	return (int64(lockNamespace) << 32) | int64(uint32(h))
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
	if c == nil || c.db == nil {
		return false
	}
	if c.driver == db.DriverSQLite {
		// On SQLite every node is "the leader" of every job.
		return true
	}
	// pg_try_advisory_lock, MySQL GET_LOCK and sp_getapplock are all
	// try-once operations. The trade-off is the same as for tryLock: a
	// best-effort read with no "is-locked" predicate. Good
	// enough for the status endpoint.
	return c.tryLock(ctx, name)
}

// coordinatorLockName returns the canonical name used in MySQL's
// GET_LOCK / RELEASE_LOCK and SQL Server's sp_getapplock calls.
// Prefixed with "mcharbor_" so
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

// ResolveNodeID returns id, or the same random node name New would pick when it is
// empty, so main knows the identity before the logger and the coordinator exist.
func ResolveNodeID(id string) string {
	if id != "" {
		return id
	}
	return "node-" + randomSuffix()
}
