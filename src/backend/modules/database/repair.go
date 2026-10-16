// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

// ErrNoReport is returned when a repair is asked for before any run.
var ErrNoReport = errors.New("no database check report is available")

// Repair applies the remedies named by ids.
//
// Every finding is re-verified before it is acted on: a report can be minutes
// old, and deleting rows from a stale count is exactly the failure this
// feature exists to catch.
func (c *Checker) Repair(ctx context.Context, ids []string, compact *coredb.CompactManager) (RepairResponse, error) {
	status := c.Status()
	if status == nil {
		return RepairResponse{}, ErrNoReport
	}
	if status.Running {
		// A run in progress means Status returned a partial report, and a
		// mutation now would race the sweep still reading the same tables.
		return RepairResponse{}, ErrCheckRunning
	}

	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}

	var selected []Finding
	for _, f := range status.Findings {
		if wanted[f.ID] && f.Remedy != "" {
			selected = append(selected, f)
		}
	}
	if len(selected) != len(wanted) {
		return RepairResponse{}, fmt.Errorf("%w: a requested finding is not in the current report", ErrNoReport)
	}

	// Non-destructive first, but VACUUM dead last whatever its flag says:
	// CheckpointAndVacuum returns immediately and leaves a background VACUUM
	// holding SQLite's exclusive lock, which a following orphan DELETE would
	// wait on until busy_timeout expires. Ordering it after everything else
	// removes the contention; it rewrites the file and touches no rows, so it
	// has nothing to gain from running first.
	sort.SliceStable(selected, func(i, j int) bool {
		return remedyRank(selected[i]) < remedyRank(selected[j])
	})

	driver := coredb.Driver(status.Driver)
	out := RepairResponse{Outcomes: make([]RepairOutcome, 0, len(selected))}

	for _, f := range selected {
		outcome := RepairOutcome{FindingID: f.ID, Remedy: f.Remedy}
		switch f.Remedy {
		case remedyVacuum:
			if compact == nil {
				outcome.Code = string(i18n.ErrDatabaseRepairFailed)
				break
			}
			// CheckpointAndVacuum stamps StartedAt only on the call that
			// actually launched the goroutine; a call that found one already
			// running returns the previous run's timestamps with a zero
			// StartedAt. That zero is the only honest signal that this
			// request was a no-op rather than a fresh vacuum.
			state := compact.CheckpointAndVacuum(ctx)
			if state.StartedAt.IsZero() {
				outcome.Code = string(i18n.ErrDatabaseRepairFailed)
				break
			}
			outcome.OK = true
		case remedyDeleteOrphans:
			rows, skipped, err := c.deleteOrphans(ctx, driver, f)
			outcome.Rows, outcome.Skipped = rows, skipped
			if err != nil {
				outcome.Code = string(i18n.ErrDatabaseRepairFailed)
				outcome.Detail = sanitize(err, "")
				break
			}
			outcome.OK = true
		case remedyCreateIndex:
			skipped, err := c.createIndex(ctx, driver, f)
			outcome.Skipped = skipped
			if err != nil {
				outcome.Code = string(i18n.ErrDatabaseRepairFailed)
				outcome.Detail = sanitize(err, "")
				break
			}
			outcome.OK = true
		case remedyApplyMigrations:
			if err := coredb.MigrateContext(ctx, c.source, driver); err != nil {
				outcome.Code = string(i18n.ErrDatabaseRepairFailed)
				outcome.Detail = sanitize(err, "")
				break
			}
			outcome.OK = true
		default:
			// Not ErrDatabaseRepairUnknown: the finding is in the report, it
			// is its remedy this build does not know how to apply.
			outcome.Code = string(i18n.ErrDatabaseRepairFailed)
		}
		out.Rows += outcome.Rows
		out.Outcomes = append(out.Outcomes, outcome)
	}

	return out, nil
}

// remedyRank orders one remedy within a repair batch: non-destructive first,
// then destructive, then VACUUM. It is a total order on a small integer, so
// sorting by it is a valid strict weak ordering.
func remedyRank(f Finding) int {
	switch {
	case f.Remedy == remedyVacuum:
		return 2
	case f.Destructive:
		return 1
	default:
		return 0
	}
}

// deleteOrphans removes the child rows whose parent is gone, after counting
// them again. A count that has fallen to zero means somebody else cleaned up
// between the report and the button, and the remedy skips rather than runs.
func (c *Checker) deleteOrphans(ctx context.Context, driver coredb.Driver, f Finding) (int64, bool, error) {
	// Defence in depth. checks_orphans.go never attaches this remedy to a
	// governed table, so reaching here means the report was tampered with or the
	// two fell out of step — either way, refuse rather than delete audit history.
	if isGovernedTable(f.Table) {
		return 0, false, fmt.Errorf("refusing to delete rows from governed table %s", f.Table)
	}

	rel := relation{Table: f.Table, Column: f.Column, RefTable: f.RefTable, RefColumn: f.RefColumn}

	n, err := orphanCount(ctx, c.source, driver, rel)
	if err != nil {
		return 0, false, err
	}
	if n == 0 {
		return 0, true, nil
	}

	runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	child := coredb.QuoteIdent(driver, rel.Table)
	parent := coredb.QuoteIdent(driver, rel.RefTable)
	childCol := coredb.QuoteIdent(driver, rel.Column)
	parentCol := coredb.QuoteIdent(driver, rel.RefColumn)

	// The inner query aliases the parent table so a self-referencing relation
	// (child == parent) still correlates against the outer row instead of
	// degenerating into an uncorrelated existence check that would delete
	// every row with a non-NULL foreign key.
	query := fmt.Sprintf( // SQL identifier: every %s is a coredb.QuoteIdent result or Placeholder
		`DELETE FROM %s WHERE %s IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM %s AS mch_p WHERE mch_p.%s = %s.%s)`,
		child, childCol, parent, parentCol, child, childCol,
	)

	result, err := c.source.ExecContext(runCtx, query)
	if err != nil {
		return 0, false, fmt.Errorf("deleting orphans from %s.%s: %w", rel.Table, rel.Column, err)
	}
	return coredb.RowsAffected(result), false, nil
}

// createIndex creates a declared index the database is missing.
func (c *Checker) createIndex(ctx context.Context, driver coredb.Driver, f Finding) (bool, error) {
	declared, _, err := declaredIndexes()
	if err != nil {
		return false, err
	}
	// declaredIndexes keys its map on the lowercased index name, and carries
	// the net CREATE statement — the one the last migration to touch this
	// index left behind, not the first one to mention it.
	decl, ok := declared[strings.ToLower(f.Index)]
	if !ok {
		return true, nil
	}

	live, err := c.liveIndexes(ctx, driver)
	if err != nil {
		return false, err
	}
	// liveIndexes lowercases its keys, so the lookup must too.
	if live[strings.ToLower(f.Index)] {
		return true, nil
	}

	runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	if _, err := c.source.ExecContext(runCtx, coredb.TranslateStatement(driver, decl.Stmt)); err != nil {
		return false, fmt.Errorf("creating index %s: %w", f.Index, err)
	}
	return false, nil
}
