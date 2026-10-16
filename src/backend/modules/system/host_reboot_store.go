// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Reboot statuses, in the order a reboot moves through them.
const (
	RebootStatusScheduled         = "scheduled"
	RebootStatusRunning           = "running"
	RebootStatusStoppingWorkloads = "stopping_workloads"
	RebootStatusRebooting         = "rebooting"
	RebootStatusDone              = "done"
	RebootStatusFailed            = "failed"
)

// rebootActive reports whether a status means the reboot is past the point
// where it can be cancelled or replaced.
func rebootActive(status string) bool {
	switch status {
	case RebootStatusRunning, RebootStatusStoppingWorkloads, RebootStatusRebooting:
		return true
	}
	return false
}

// hostRebootsSchema is created by the system module rather than a numbered
// migration: migrations are embedded in core/db, and changing that package
// would unload every installed plugin.
var hostRebootsSchema = []string{
	`CREATE TABLE IF NOT EXISTS host_reboots (
		id                 TEXT PRIMARY KEY,
		target_id          TEXT NOT NULL UNIQUE,
		status             TEXT NOT NULL,
		phase              TEXT NOT NULL DEFAULT '',
		scheduled_at       TEXT NOT NULL,
		drained_externally INTEGER NOT NULL DEFAULT 0,
		requested_by       TEXT NOT NULL DEFAULT '',
		requested_by_name  TEXT NOT NULL DEFAULT '',
		started_at         TEXT,
		reboot_issued_at   TEXT,
		finished_at        TEXT,
		failure_reason     TEXT NOT NULL DEFAULT '',
		last_error         TEXT NOT NULL DEFAULT '',
		restore_plan       TEXT NOT NULL DEFAULT '',
		created_at         TEXT NOT NULL,
		updated_at         TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_host_reboots_status ON host_reboots(status, scheduled_at)`,
}

// ensureRebootSchema creates the host_reboots table when it is missing.
func ensureRebootSchema(ctx context.Context, database *sql.DB) error {
	// bounded by the fixed schema statement list
	for _, statement := range hostRebootsSchema {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("creating host_reboots: %w", err)
		}
	}
	return nil
}

// HostReboot is one scheduled, running or finished reboot of a host.
type HostReboot struct {
	ID                string     `json:"id"`
	Status            string     `json:"status"`
	Phase             string     `json:"phase,omitempty"`
	ScheduledAt       time.Time  `json:"scheduledAt"`
	DrainedExternally bool       `json:"drainedExternally"`
	RequestedBy       string     `json:"requestedBy,omitempty"`
	StartedAt         *time.Time `json:"startedAt,omitempty"`
	RebootIssuedAt    *time.Time `json:"rebootIssuedAt,omitempty"`
	FinishedAt        *time.Time `json:"finishedAt,omitempty"`
	FailureReason     string     `json:"failureReason,omitempty"`
	LastError         string     `json:"lastError,omitempty"`

	targetID    string
	requesterID string
	plan        restorePlan
	updatedAt   time.Time
}

// target is the host the row names.
func (r HostReboot) target() HostTarget {
	if r.targetID == selfRowID {
		return SelfTarget()
	}
	return EnvTarget(r.targetID)
}

// restorePlan is what a finished reboot brings back: the containers a normal
// boot would have restarted, and the Swarm availability McHarbor changed.
type restorePlan struct {
	Containers        []restoreContainer `json:"containers,omitempty"`
	SwarmNodeID       string             `json:"swarmNodeId,omitempty"`
	SwarmAvailability string             `json:"swarmAvailability,omitempty"`
}

// restoreContainer names one container to start again.
type restoreContainer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

const rebootColumns = `id, target_id, status, phase, scheduled_at, drained_externally, requested_by,
	requested_by_name, started_at, reboot_issued_at, finished_at, failure_reason, last_error, restore_plan`

// rebootStore persists host reboots.
type rebootStore struct {
	database *sql.DB
}

// scanReboot reads one row in rebootColumns order.
func scanReboot(row interface{ Scan(...any) error }) (*HostReboot, error) {
	var rec HostReboot
	var scheduled string
	var drained int
	var started, issued, finished sql.NullString
	var plan string
	if err := row.Scan(&rec.ID, &rec.targetID, &rec.Status, &rec.Phase, &scheduled, &drained,
		&rec.requesterID, &rec.RequestedBy, &started, &issued, &finished,
		&rec.FailureReason, &rec.LastError, &plan); err != nil {
		return nil, err
	}
	rec.DrainedExternally = drained == 1
	if parsed, err := time.Parse(time.RFC3339, scheduled); err == nil {
		rec.ScheduledAt = parsed
	}
	rec.StartedAt = parseOptionalTime(started)
	rec.RebootIssuedAt = parseOptionalTime(issued)
	rec.FinishedAt = parseOptionalTime(finished)
	if plan != "" {
		if err := json.Unmarshal([]byte(plan), &rec.plan); err != nil {
			return nil, fmt.Errorf("decoding restore plan: %w", err)
		}
	}
	return &rec, nil
}

// parseOptionalTime reads a nullable RFC3339 column.
func parseOptionalTime(value sql.NullString) *time.Time {
	if !value.Valid || value.String == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value.String)
	if err != nil {
		return nil
	}
	return &parsed
}

// stamp formats a time the way every host_reboots column stores it, so due
// rows can be found by string comparison.
func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// get returns the target's reboot row, or nil when there is none.
func (s *rebootStore) get(ctx context.Context, targetID string) (*HostReboot, error) {
	row := s.database.QueryRowContext(ctx, `SELECT `+rebootColumns+` FROM host_reboots WHERE target_id = ?`, targetID)
	rec, err := scanReboot(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading host reboot: %w", err)
	}
	return rec, nil
}

// schedule stores a new reboot for the target, replacing a scheduled or
// finished one. It refuses while a reboot is already running.
func (s *rebootStore) schedule(ctx context.Context, targetID string, at time.Time, drained bool, userID, username string) (*HostReboot, error) {
	existing, err := s.get(ctx, targetID)
	if err != nil {
		return nil, err
	}
	now := stamp(time.Now())
	if existing != nil {
		if rebootActive(existing.Status) {
			return nil, refuse(RebootReasonInProgress, "a reboot of this host is already in progress")
		}
		result, err := s.database.ExecContext(ctx, `
			UPDATE host_reboots
			SET status = ?, phase = '', scheduled_at = ?, drained_externally = ?, requested_by = ?,
			    requested_by_name = ?, started_at = NULL, reboot_issued_at = NULL, finished_at = NULL,
			    failure_reason = '', last_error = '', restore_plan = '', updated_at = ?
			WHERE target_id = ? AND status NOT IN (?, ?, ?)`,
			RebootStatusScheduled, stamp(at), boolInt(drained), userID, username, now, targetID,
			RebootStatusRunning, RebootStatusStoppingWorkloads, RebootStatusRebooting)
		if err != nil {
			return nil, fmt.Errorf("rescheduling host reboot: %w", err)
		}
		if db.RowsAffected(result) == 0 {
			return nil, refuse(RebootReasonInProgress, "a reboot of this host is already in progress")
		}
	} else {
		if _, err := s.database.ExecContext(ctx, `
			INSERT INTO host_reboots (id, target_id, status, scheduled_at, drained_externally, requested_by,
			    requested_by_name, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			xid.New().String(), targetID, RebootStatusScheduled, stamp(at), boolInt(drained), userID,
			username, now, now); err != nil {
			return nil, fmt.Errorf("scheduling host reboot: %w", err)
		}
	}
	return s.get(ctx, targetID)
}

// cancel removes a reboot that has not started yet.
func (s *rebootStore) cancel(ctx context.Context, targetID string) error {
	result, err := s.database.ExecContext(ctx,
		`DELETE FROM host_reboots WHERE target_id = ? AND status = ?`, targetID, RebootStatusScheduled)
	if err != nil {
		return fmt.Errorf("cancelling host reboot: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return refuse(RebootReasonNotScheduled, "there is no scheduled reboot to cancel")
	}
	return nil
}

// claim moves a due reboot to running, so only one node or tick executes it.
func (s *rebootStore) claim(ctx context.Context, id string) (bool, error) {
	now := stamp(time.Now())
	result, err := s.database.ExecContext(ctx, `
		UPDATE host_reboots SET status = ?, started_at = ?, updated_at = ?
		WHERE id = ? AND status = ?`, RebootStatusRunning, now, now, id, RebootStatusScheduled)
	if err != nil {
		return false, fmt.Errorf("claiming host reboot: %w", err)
	}
	return db.RowsAffected(result) == 1, nil
}

// setPhase records the status and the step the reboot is on.
func (s *rebootStore) setPhase(ctx context.Context, id, status, phase string) error {
	_, err := s.database.ExecContext(ctx, `
		UPDATE host_reboots SET status = ?, phase = ?, updated_at = ? WHERE id = ?`,
		status, phase, stamp(time.Now()), id)
	if err != nil {
		return fmt.Errorf("updating host reboot phase: %w", err)
	}
	return nil
}

// markRebooting stores the restore plan and the moment the reboot was issued,
// before the command runs: on the self host McHarbor may not live to write it after.
func (s *rebootStore) markRebooting(ctx context.Context, id string, plan restorePlan, issuedAt time.Time) error {
	encoded, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("encoding restore plan: %w", err)
	}
	_, err = s.database.ExecContext(ctx, `
		UPDATE host_reboots SET status = ?, phase = 'reboot', reboot_issued_at = ?, restore_plan = ?, updated_at = ?
		WHERE id = ?`, RebootStatusRebooting, stamp(issuedAt), string(encoded), stamp(time.Now()), id)
	if err != nil {
		return fmt.Errorf("marking host rebooting: %w", err)
	}
	return nil
}

// finish records the final status, with the reason and detail of a failure or
// of a partial restore.
func (s *rebootStore) finish(ctx context.Context, id, status, reason, detail string) error {
	now := stamp(time.Now())
	_, err := s.database.ExecContext(ctx, `
		UPDATE host_reboots SET status = ?, phase = '', failure_reason = ?, last_error = ?, finished_at = ?, updated_at = ?
		WHERE id = ?`, status, reason, detail, now, now, id)
	if err != nil {
		return fmt.Errorf("finishing host reboot: %w", err)
	}
	return nil
}

// list returns the rows in any of statuses.
func (s *rebootStore) list(ctx context.Context, statuses ...string) ([]HostReboot, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	placeholders := "?"
	args := []any{statuses[0]}
	for _, status := range statuses[1:] {
		placeholders += ", ?"
		args = append(args, status)
	}
	rows, err := s.database.QueryContext(ctx, `SELECT `+rebootColumns+`, updated_at FROM host_reboots
		WHERE status IN (`+placeholders+`) ORDER BY scheduled_at LIMIT 1000`, args...)
	if err != nil {
		return nil, fmt.Errorf("listing host reboots: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []HostReboot
	for rows.Next() {
		rec, err := scanRebootWithUpdated(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

// scanRebootWithUpdated reads rebootColumns plus updated_at, which only the
// staleness check reads.
func scanRebootWithUpdated(rows *sql.Rows) (*HostReboot, error) {
	var updated string
	rec, err := scanReboot(scanTail{rows: rows, tail: &updated})
	if err != nil {
		return nil, fmt.Errorf("reading host reboot: %w", err)
	}
	if parsed, err := time.Parse(time.RFC3339, updated); err == nil {
		rec.updatedAt = parsed
	}
	return rec, nil
}

// scanTail appends one destination to a Scan so a shared column list can be
// reused with an extra trailing column.
type scanTail struct {
	rows *sql.Rows
	tail any
}

// Scan scans dest plus the tail.
func (s scanTail) Scan(dest ...any) error {
	return s.rows.Scan(append(dest, s.tail)...)
}
