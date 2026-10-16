// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"
	"strconv"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

// historySeededKey marks demo.db as carrying its stored history; historyVersion
// is bumped only when the seed changes shape and must be written again.
const (
	historySeededKey = "demo.history_seeded"
	historyVersion   = "1"
)

// DefaultHistoryDays is how far back the seeded history reaches unless the
// operator chose another period when entering demo mode.
const DefaultHistoryDays = 90

// auditRowsPer90Days is the audit volume the history seeds per 90 days, scaled
// with the chosen period so a year of history is as busy as a quarter.
const auditRowsPer90Days = 20000

// historyHost is one simulated environment as the history seed sees it: the
// estate host, its environment row, and the container snapshot taken once.
type historyHost struct {
	host       *estate.Host
	envID      string
	containers []estate.Container
	stacks     []string
}

// historyActor is someone the seeded history attributes an action to.
type historyActor struct {
	id       string
	username string
	ip       string
	weight   int
}

// historyCtx carries what every per-kind seeder needs, so each can stay a
// plain function over one transaction.
type historyCtx struct {
	window    time.Duration
	auditRows int
	now       time.Time
	hosts     []historyHost
	actors    []historyActor
	rng       *rand.Rand
	// secret is the instance secret the seeded audit rows are sealed with (cfg.Secret).
	secret string
}

// seedHistory fills demo.db with the default period of stored history.
func seedHistory(ctx context.Context, conn *sql.DB, e *estate.Estate, envIDs map[string]string, now time.Time, secret string) error {
	return seedHistoryFor(ctx, conn, e, envIDs, now, DefaultHistoryDays, secret)
}

// seedHistoryFor fills demo.db with days of stored history for the estate, sealing the
// audit rows with secret. The window ends a day short of the audit retention, which is
// raised to fit it.
func seedHistoryFor(ctx context.Context, conn *sql.DB, e *estate.Estate, envIDs map[string]string, now time.Time, days int, secret string) error {
	if days < 1 {
		days = DefaultHistoryDays
	}
	if conn == nil || e == nil {
		return nil
	}
	done, err := historyAlreadySeeded(ctx, conn)
	if err != nil {
		return err
	}
	if done {
		return nil
	}
	hc := &historyCtx{
		window:    time.Duration(days-1) * 24 * time.Hour,
		auditRows: min(max(auditRowsPer90Days*days/90, 500), 80000),
		secret:    secret,
		now:       now.UTC().Truncate(time.Second), rng: rand.New(rand.NewPCG(estate.DefaultSeed, 0x6869_7374_6f72_79))} //nolint:gosec // a fixed seed keeps the demo history reproducible; nothing here is security-sensitive
	for _, h := range e.Hosts {
		envID, ok := envIDs[h.Key]
		if !ok {
			continue
		}
		hh := historyHost{host: h, envID: envID, containers: h.Containers()}
		for _, st := range h.Stacks() {
			hh.stacks = append(hh.stacks, st.Name)
		}
		sort.Strings(hh.stacks)
		hc.hosts = append(hc.hosts, hh)
	}
	if days > DefaultHistoryDays {
		if err := raiseAuditRetention(ctx, conn, days+7, hc.now); err != nil {
			return err
		}
	}
	if len(hc.hosts) == 0 {
		return nil
	}

	steps := []struct {
		name string
		fn   func(context.Context, *sql.DB, *historyCtx) error
	}{
		{"users", seedHistoryUsers},
		{"audit log", seedHistoryAudit},
		{"workflows", seedHistoryWorkflows},
		{"schedules", seedHistorySchedules},
		{"alerts", seedHistoryAlerts},
		{"incidents", seedHistoryIncidents},
	}
	for _, step := range steps {
		if err := step.fn(ctx, conn, hc); err != nil {
			return fmt.Errorf("seeding demo %s: %w", step.name, err)
		}
	}
	return markHistorySeeded(ctx, conn, hc.now)
}

func historyAlreadySeeded(ctx context.Context, conn *sql.DB) (bool, error) {
	ok, err := tableExists(ctx, conn, "settings")
	if err != nil || !ok {
		return false, err
	}
	var value string
	err = conn.QueryRowContext(ctx, "SELECT COALESCE(value, '') FROM settings WHERE key = ?", historySeededKey).Scan(&value)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("reading %s: %w", historySeededKey, err)
	}
	return value == historyVersion, nil
}

func markHistorySeeded(ctx context.Context, conn *sql.DB, now time.Time) error {
	ok, err := tableExists(ctx, conn, "settings")
	if err != nil || !ok {
		return err
	}
	ts := now.Format(time.RFC3339)
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO settings (id, key, value, category, created_at, updated_at)
		VALUES (?, ?, ?, 'demo', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		stableID("setting", historySeededKey), historySeededKey, historyVersion, ts, ts); err != nil {
		return fmt.Errorf("recording %s: %w", historySeededKey, err)
	}
	return nil
}

// tableExists lets every seeder skip a table that a schema change removed,
// instead of failing the whole history over one missing feature.
func tableExists(ctx context.Context, conn *sql.DB, name string) (bool, error) {
	var found string
	err := conn.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&found)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("checking table %s: %w", name, err)
	}
	return true, nil
}

// allTablesExist reports whether every table a seeder writes is present.
func allTablesExist(ctx context.Context, conn *sql.DB, names ...string) (bool, error) {
	for _, n := range names {
		ok, err := tableExists(ctx, conn, n)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

// withTx runs fn in one transaction and rolls back on any failure.
func withTx(ctx context.Context, conn *sql.DB, fn func(tx *sql.Tx) error) (err error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				err = errors.Join(err, fmt.Errorf("rolling back: %w", rbErr))
			}
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	return nil
}

// insertAll prepares query once and executes it for every argument row.
func insertAll(ctx context.Context, tx *sql.Tx, query string, rows [][]any) (err error) {
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("preparing insert: %w", err)
	}
	defer func() {
		if cerr := stmt.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing statement: %w", cerr)
		}
	}()
	for _, args := range rows {
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return fmt.Errorf("inserting row: %w", err)
		}
	}
	return nil
}

func (hc *historyCtx) pickHost() *historyHost {
	return &hc.hosts[hc.rng.IntN(len(hc.hosts))]
}

func (hc *historyCtx) pickContainer(h *historyHost) (estate.Container, bool) {
	if len(h.containers) == 0 {
		return estate.Container{}, false
	}
	return h.containers[hc.rng.IntN(len(h.containers))], true
}

func (hc *historyCtx) pickActor() historyActor {
	total := 0
	for _, a := range hc.actors {
		total += a.weight
	}
	if total == 0 {
		return historyActor{}
	}
	n := hc.rng.IntN(total)
	for _, a := range hc.actors {
		if n < a.weight {
			return a
		}
		n -= a.weight
	}
	return hc.actors[len(hc.actors)-1]
}

// busyTime returns a moment in the history window, weighted the way people
// work: weekdays over weekends, office hours over the night.
func (hc *historyCtx) busyTime() time.Time {
	start := hc.now.Add(-hc.window)
	for {
		t := start.Add(time.Duration(hc.rng.Int64N(int64(hc.window))))
		weight := 1.0
		if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
			weight *= 0.2
		}
		if h := t.Hour(); h < 7 || h >= 19 {
			weight *= 0.15
		}
		if hc.rng.Float64() < weight {
			return t.Truncate(time.Second)
		}
	}
}

// anyTime returns a uniformly random moment within the last d.
func (hc *historyCtx) anyTime(d time.Duration) time.Time {
	return hc.now.Add(-time.Duration(hc.rng.Int64N(int64(d)))).Truncate(time.Second)
}

func stackOf(c estate.Container) string {
	return c.Labels["com.docker.compose.project"]
}

func rfc(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// raiseAuditRetention keeps the audit prune from deleting a history longer than
// the default retention; it only ever raises the setting, never lowers it.
func raiseAuditRetention(ctx context.Context, conn *sql.DB, days int, now time.Time) error {
	var current string
	err := conn.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'retention_audit_days'").Scan(&current)
	if err == nil {
		if n, perr := strconv.Atoi(current); perr == nil && (n == 0 || n >= days) {
			return nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("reading audit retention: %w", err)
	}
	stamp := now.UTC().Format(time.RFC3339)
	if _, err := conn.ExecContext(ctx, `INSERT INTO settings (id, key, value, category, created_at, updated_at)
		VALUES (?, 'retention_audit_days', ?, 'retention', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		stableID("setting", "retention_audit_days"), strconv.Itoa(days), stamp, stamp); err != nil {
		return fmt.Errorf("raising audit retention: %w", err)
	}
	return nil
}
