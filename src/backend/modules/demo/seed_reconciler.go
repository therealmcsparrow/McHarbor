// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

// reconcilerSeededKey marks demo.db as carrying the reconciler story.
const (
	reconcilerSeededKey   = "demo.reconciler_seeded"
	reconcilerSeedVersion = "1"
)

// reconcilerSeedTables are the tables the reconciler seed writes.
var reconcilerSeedTables = []string{"settings", "environments", "desired_states"}

// reconcilerSeedCounts sizes what one seed wrote.
type reconcilerSeedCounts struct{ Sources, Watches, Drifted int }

// driftShape is the state one watch rests in. The four together are the states an
// operator has to be able to tell apart at a glance, and the report is what the
// drift panel renders — a drifted watch with an empty report is the defect this
// seed exists to make visible.
type driftShape struct {
	mode     string
	drifted  bool
	report   string
	lastSeen int
}

// driftShapes cycles through in-sync, proposed, enforced and never-reconciled.
func driftShapes() []driftShape {
	return []driftShape{
		{mode: "observe", drifted: false, report: "", lastSeen: 1},
		{mode: "propose", drifted: true, lastSeen: 1, report: `{"fields":[` +
			`{"field":"image","desired":"nginx:1.27.2","actual":"nginx:1.27.0","changedBy":"cluster"},` +
			`{"field":"restartPolicy","desired":"unless-stopped","actual":"no","changedBy":"cluster"}` +
			`],"summary":"the image was rolled back and the restart policy was cleared"}`},
		{mode: "enforce", drifted: true, lastSeen: 2, report: `{"fields":[` +
			`{"field":"env.LOG_LEVEL","desired":"info","actual":"debug","changedBy":"cluster"}` +
			`],"summary":"LOG_LEVEL was raised to debug on the host and not in the desired state",` +
			`"blocked":"applying this needs a recreate, which enforcement never takes unattended"}`},
		{mode: "observe", drifted: false, report: "", lastSeen: 0},
	}
}

// reconcilerSeeded reports whether demo.db already carries this version.
func reconcilerSeeded(ctx context.Context, conn *sql.DB) (bool, error) {
	ok, err := tableExists(ctx, conn, "settings")
	if err != nil || !ok {
		return false, err
	}
	var value string
	err = conn.QueryRowContext(ctx, "SELECT COALESCE(value, '') FROM settings WHERE key = ?", reconcilerSeededKey).Scan(&value)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("reading %s: %w", reconcilerSeededKey, err)
	}
	return value == reconcilerSeedVersion, nil
}

// seedReconciler writes the desired-state watches and, when the demo's Git story
// is present, the Git source two of them read their intent from.
func seedReconciler(ctx context.Context, conn *sql.DB, e *estate.Estate,
	envIDs map[string]string, now time.Time) (reconcilerSeedCounts, error) {
	var counts reconcilerSeedCounts
	if !coredemo.Active() {
		return counts, errNotDemo
	}
	seeded, err := reconcilerSeeded(ctx, conn)
	if err != nil || seeded {
		return counts, err
	}
	if ok, err := allTablesExist(ctx, conn, reconcilerSeedTables...); err != nil || !ok {
		return counts, err
	}

	stamp := now.UTC().Format(time.RFC3339)
	sourceID, err := seedDesiredStateSource(ctx, conn, now)
	if err != nil {
		return counts, err
	}
	if sourceID != "" {
		counts.Sources = 1
	}

	shapes := driftShapes()
	i := 0
	// bounded by the fixed demo estate; the seed is marked done only after it completes
	for _, h := range e.Hosts {
		envID, ok := envIDs[h.Key]
		if !ok {
			continue
		}
		perHost := 0
		// bounded by the fixed demo estate; the seed is marked done only after it completes
		for _, c := range h.Containers() {
			if c.State != estate.StateRunning || perHost >= 2 {
				continue
			}
			perHost++
			shape := shapes[i%len(shapes)]
			i++

			id := stableID("desired-state", envID+"/"+c.Name)
			var lastReconcile, lastEnforced, lastReport any
			if shape.lastSeen > 0 {
				lastReconcile = now.Add(-time.Duration(shape.lastSeen) * time.Hour).UTC().Format(time.RFC3339)
			}
			if shape.mode == "enforce" && shape.drifted {
				lastEnforced = now.Add(-6 * time.Hour).UTC().Format(time.RFC3339)
			}
			if shape.report != "" {
				lastReport = shape.report
			}
			// Two of the watches read their intent from Git; the rest are held in
			// McHarbor. A source id on every row would hide that distinction.
			var srcID any
			srcPath := ""
			if sourceID != "" && i%4 == 1 {
				srcID = sourceID
				srcPath = "stacks/" + c.Name + "/compose.yaml"
			}

			_, err := conn.ExecContext(ctx, `
				INSERT INTO desired_states (
					id, name, description, env_id, container_name, image_ref, desired_status,
					restart_policy, config, last_reconcile, drift_detected, created_at, updated_at,
					kind, stack_name, last_report, mode, source_id, source_path, source_compose,
					last_enforced_at, source, subject_id, spec, plugin_env
				) VALUES (?, ?, ?, ?, ?, ?, 'running', 'unless-stopped', '{}', ?, ?, ?, ?,
					'container', '', ?, ?, ?, ?, '', ?, ?, ?, '{}', '')
				ON CONFLICT(id) DO NOTHING`,
				id, c.Name, "Keep "+c.Name+" on its approved image and restart policy.",
				envID, c.Name, c.Image, lastReconcile, boolToInt(shape.drifted), stamp, stamp,
				lastReport, shape.mode, srcID, srcPath, lastEnforced,
				sourceKindFor(srcID), c.ID)
			if err != nil {
				return counts, fmt.Errorf("inserting desired state: %w", err)
			}
			counts.Watches++
			if shape.drifted {
				counts.Drifted++
			}
		}
		if counts.Watches >= 16 {
			break
		}
	}

	if err := markSeeded(ctx, conn, reconcilerSeededKey, reconcilerSeedVersion, now); err != nil {
		return counts, err
	}
	return counts, nil
}

// seedDesiredStateSource points a Git source at the demo's own repository when the
// Git seed has run. It returns an empty id when there is no repo to point at,
// which leaves every watch McHarbor-held rather than inventing a dangling repo id.
func seedDesiredStateSource(ctx context.Context, conn *sql.DB, now time.Time) (string, error) {
	ok, err := allTablesExist(ctx, conn, "desired_state_sources", "git_repos")
	if err != nil || !ok {
		return "", err
	}
	var repoID string
	err = conn.QueryRowContext(ctx, "SELECT id FROM git_repos ORDER BY created_at LIMIT 1").Scan(&repoID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("reading demo git repo: %w", err)
	}

	stamp := now.UTC().Format(time.RFC3339)
	id := stableID("desired-state-source", repoID)
	_, err = conn.ExecContext(ctx, `
		INSERT INTO desired_state_sources (
			id, name, repo_id, branch, path, env_id, enabled, last_commit,
			last_synced_at, last_error, row_count, created_at, updated_at
		) VALUES (?, 'Platform intent (main)', ?, 'main', 'stacks', '', 1, ?, ?, '', ?, ?, ?)
		ON CONFLICT(id) DO NOTHING`,
		id, repoID, "9f2c1ab", now.Add(-40*time.Minute).UTC().Format(time.RFC3339), 4, stamp, stamp)
	if err != nil {
		return "", fmt.Errorf("inserting desired state source: %w", err)
	}
	return id, nil
}

// sourceKindFor names where a watch's intent comes from, which the list renders as
// a badge.
func sourceKindFor(srcID any) string {
	if srcID == nil {
		return "mcharbor"
	}
	return "git"
}
