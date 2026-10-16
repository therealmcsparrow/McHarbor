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
)

// runbooksSeededKey marks demo.db as carrying the runbook story.
const (
	runbooksSeededKey  = "demo.runbooks_seeded"
	runbookSeedVersion = "1"
	demoRunbookAuthor  = "Demo Operations"
)

// runbookSeedTables are the tables the runbook seed writes.
var runbookSeedTables = []string{"settings", "runbooks", "runbook_steps", "runbook_runs"}

// runbookSeedCounts sizes what one seed wrote.
type runbookSeedCounts struct{ Runbooks, Steps, Runs int }

// runbookStep is one step of a seeded runbook.
type runbookStep struct {
	kind          string
	title         string
	body          string
	diagnosticKey string
	actionKey     string
	required      bool
}

// runbookSpec is one seeded runbook: what attaches it, and what it asks the
// operator to do.
type runbookSpec struct {
	name         string
	description  string
	attachKind   string
	conditionKey string
	cron         string
	onAlert      bool
	steps        []runbookStep
}

// demoRunbooks are written against the conditions the demo's alerts actually
// raise, so opening an incident shows its runbook attached rather than an empty
// panel. Each mixes a diagnostic, a judgement step and an action, because a
// runbook of only prose does not exercise the page.
func demoRunbooks() []runbookSpec {
	return []runbookSpec{
		{
			name:         "Container is restarting in a loop",
			description:  "Triage a container whose restart count is climbing, before it takes the stack with it.",
			attachKind:   "alert_condition",
			conditionKey: "container_restart_loop",
			onAlert:      true,
			steps: []runbookStep{
				{kind: "diagnostic", title: "Read the last 200 log lines", diagnosticKey: "container.logs",
					body: "Look for the last line before each restart. A config error repeats identically; an OOM kill does not.", required: true},
				{kind: "diagnostic", title: "Check the exit code and OOM flag", diagnosticKey: "container.inspect",
					body: "Exit 137 with OOMKilled means the limit is the cause, not the image.", required: true},
				{kind: "instruction", title: "Is the loop caused by the memory limit?",
					body: "If OOMKilled is set, raise the limit or fix the leak. If not, treat it as a start failure and read the logs again."},
				{kind: "action", title: "Restart once, with the logs open", actionKey: "container.restart",
					body: "One restart tells you whether the failure is deterministic. Do not loop on this step."},
				{kind: "instruction", title: "Record what you changed",
					body: "A restart that fixed nothing is still worth writing down: the next responder should not repeat it."},
			},
		},
		{
			name:         "Host disk is filling",
			description:  "Reclaim space on a Docker host before writes start failing.",
			attachKind:   "alert_condition",
			conditionKey: "docker_disk",
			onAlert:      true,
			steps: []runbookStep{
				{kind: "diagnostic", title: "Break the usage down by kind", diagnosticKey: "system.df",
					body: "Images, containers, volumes and build cache fail differently. Prune the one that is actually large.", required: true},
				{kind: "instruction", title: "Is any of it a volume?",
					body: "Volume data is not reclaimable by pruning. If volumes dominate, this is a retention problem, not a cleanup one."},
				{kind: "action", title: "Prune dangling images", actionKey: "images.prune",
					body: "Dangling only. A full prune removes images no container is running right now, which includes every rollback target."},
				{kind: "instruction", title: "Raise the retention question",
					body: "A host that fills monthly needs a retention change, not a monthly prune."},
			},
		},
		{
			name:         "Nightly backup failed",
			description:  "Work out whether last night's failure left you without a recoverable copy.",
			attachKind:   "alert_condition",
			conditionKey: "backup_failed",
			cron:         "0 7 * * *",
			steps: []runbookStep{
				{kind: "diagnostic", title: "Read the run's error in full", diagnosticKey: "backup.run",
					body: "The message names the destination that refused. A local copy may still have completed.", required: true},
				{kind: "instruction", title: "Did any destination complete?",
					body: "One completed destination means you have a copy and time to think. None means this is an incident."},
				{kind: "action", title: "Re-run the plan now", actionKey: "backup.run_now",
					body: "A re-run against a transient upload failure is the cheapest fix. If it fails the same way, stop and read the destination."},
				{kind: "instruction", title: "Check the chain, not just the run",
					body: "An incremental whose base was pruned cannot be restored from. Confirm a full exists inside the retention window."},
			},
		},
		{
			name:         "Drift detected on a reconciled container",
			description:  "Decide whether to accept the change or put the container back.",
			attachKind:   "alert_condition",
			conditionKey: "desired_state_drift",
			onAlert:      true,
			steps: []runbookStep{
				{kind: "diagnostic", title: "Read the drift report", diagnosticKey: "desired_state.report",
					body: "The report names each field that differs and which side changed it.", required: true},
				{kind: "instruction", title: "Was the change deliberate?",
					body: "A deliberate change belongs in the desired state. An accidental one belongs reverted. Guessing produces a fight between the operator and the reconciler."},
				{kind: "action", title: "Enforce the desired state", actionKey: "desired_state.enforce",
					body: "Enforcement never takes a disruptive step unattended, so a recreate still waits for you."},
			},
		},
		{
			name:         "Agent went offline",
			description:  "Tell a restarted host apart from a wedged agent.",
			attachKind:   "alert_condition",
			conditionKey: "agent_offline",
			onAlert:      true,
			steps: []runbookStep{
				{kind: "diagnostic", title: "Check when it was last seen", diagnosticKey: "agent.status",
					body: "A gap that matches a reboot window is a reboot. A gap that started mid-day is not.", required: true},
				{kind: "instruction", title: "A wedged agent still reports as running",
					body: "The service can be active while the connection is dead, so do not trust the unit state alone."},
				{kind: "action", title: "Restart the agent service", actionKey: "agent.restart",
					body: "This is the fix for a wedged write loop. It does not help a host that is genuinely down."},
			},
		},
	}
}

// runbooksSeeded reports whether demo.db already carries this version of the seed.
func runbooksSeeded(ctx context.Context, conn *sql.DB) (bool, error) {
	ok, err := tableExists(ctx, conn, "settings")
	if err != nil || !ok {
		return false, err
	}
	var value string
	err = conn.QueryRowContext(ctx, "SELECT COALESCE(value, '') FROM settings WHERE key = ?", runbooksSeededKey).Scan(&value)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("reading %s: %w", runbooksSeededKey, err)
	}
	return value == runbookSeedVersion, nil
}

// seedRunbooks writes the runbook library and a short run history. A runbook is
// global (no environment) so it attaches wherever its condition fires.
func seedRunbooks(ctx context.Context, conn *sql.DB, now time.Time) (runbookSeedCounts, error) {
	var counts runbookSeedCounts
	if !coredemo.Active() {
		return counts, errNotDemo
	}
	seeded, err := runbooksSeeded(ctx, conn)
	if err != nil || seeded {
		return counts, err
	}
	if ok, err := allTablesExist(ctx, conn, runbookSeedTables...); err != nil || !ok {
		return counts, err
	}

	stamp := now.UTC().Format(time.RFC3339)
	// bounded by the fixed demo estate; the seed is marked done only after it completes
	for i, spec := range demoRunbooks() {
		id := stableID("runbook", spec.name)
		nextRun := ""
		if spec.cron != "" {
			nextRun = now.AddDate(0, 0, 1).UTC().Format(time.RFC3339)
		}
		_, err := conn.ExecContext(ctx, `
			INSERT INTO runbooks (
				id, name, description, attach_kind, attach_source_type, attach_condition_key,
				attach_alert_id, attach_subject, environment_id, enabled, created_by,
				created_at, updated_at, automation_principal_id, automation_on_alert,
				automation_cron, automation_since, automation_next_run_at,
				automation_last_run_at, automation_last_error, automation_updated_by
			) VALUES (?, ?, ?, ?, 'host', ?, '', '', '', 1, ?, ?, ?, '', ?, ?, '', ?, ?, '', ?)
			ON CONFLICT(id) DO NOTHING`,
			id, spec.name, spec.description, spec.attachKind, spec.conditionKey,
			demoRunbookAuthor, stamp, stamp, boolToInt(spec.onAlert), spec.cron, nextRun,
			now.AddDate(0, 0, -(i+1)).UTC().Format(time.RFC3339), demoRunbookAuthor)
		if err != nil {
			return counts, fmt.Errorf("inserting runbook: %w", err)
		}
		counts.Runbooks++

		// bounded by the fixed demo estate; the seed is marked done only after it completes
		for pos, step := range spec.steps {
			_, err := conn.ExecContext(ctx, `
				INSERT INTO runbook_steps (
					id, runbook_id, position, kind, title, body, diagnostic_key, action_key,
					action_params, required, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '{}', ?, ?, ?)
				ON CONFLICT(id) DO NOTHING`,
				stableID("runbook-step", fmt.Sprintf("%s/%d", id, pos)), id, pos, step.kind,
				step.title, step.body, step.diagnosticKey, step.actionKey,
				boolToInt(step.required), stamp, stamp)
			if err != nil {
				return counts, fmt.Errorf("inserting runbook step: %w", err)
			}
			counts.Steps++
		}

		// Two runs each: one that resolved and one abandoned. A history of only
		// successes hides the outcome column.
		// bounded by the fixed demo estate; the seed is marked done only after it completes
		for r, outcome := range []string{"completed", "abandoned"} {
			started := now.AddDate(0, 0, -(r*4 + 2))
			_, err := conn.ExecContext(ctx, `
				INSERT INTO runbook_runs (
					id, runbook_id, runbook_name, environment_id, subject_kind, subject_id,
					subject_name, incident_id, alert_event_id, started_by, started_by_name,
					started_at, finished_at, outcome, note, trigger_kind
				) VALUES (?, ?, ?, '', 'container', '', ?, '', '', '', ?, ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO NOTHING`,
				stableID("runbook-run", fmt.Sprintf("%s/%d", id, r)), id, spec.name,
				"demo-subject", demoRunbookAuthor,
				started.UTC().Format(time.RFC3339),
				started.Add(time.Duration(18+r*25)*time.Minute).UTC().Format(time.RFC3339),
				outcome, noteFor(outcome), triggerFor(spec.onAlert))
			if err != nil {
				return counts, fmt.Errorf("inserting runbook run: %w", err)
			}
			counts.Runs++
		}
	}

	if err := markSeeded(ctx, conn, runbooksSeededKey, runbookSeedVersion, now); err != nil {
		return counts, err
	}
	return counts, nil
}

// noteFor is what the responder wrote when they closed the run.
func noteFor(outcome string) string {
	if outcome == "completed" {
		return "Memory limit was the cause; raised to 1 GiB and the loop stopped."
	}
	return "Handed to the platform team — the host itself was being rebooted."
}

// triggerFor distinguishes a run an alert opened from one a person started.
func triggerFor(onAlert bool) string {
	if onAlert {
		return "alert"
	}
	return "manual"
}
