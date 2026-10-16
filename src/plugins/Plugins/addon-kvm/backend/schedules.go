// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/xid"
)

// Scheduled snapshots.
//
// Shaped like the host's container backup plans on purpose: a five-field cron
// spec in the server's local timezone, an enabled flag, the last and next run,
// and a retention count — which is the model an operator already has for
// backups, and a second vocabulary for the same idea would only be a second
// thing to learn.
//
// Retention only ever removes snapshots the same schedule created. A policy
// that could delete an operator's own named snapshot because it happened to be
// the oldest would be a data-loss bug wearing a feature's clothes.

// defaultKeepCount is how many snapshots a new schedule keeps.
//
// Seven, so a daily schedule holds a week. Enough to be useful, small enough
// that the qcow2 chain does not grow to the depth where reads slow measurably.
const defaultKeepCount = 7

// maxKeepCount bounds retention.
//
// Each snapshot on a `dir` pool adds a link to the guest's backing chain, and
// every read has to walk it. maxSnapshotChain already refuses a deeper chain at
// creation; this stops a schedule being configured to aim past that.
const maxKeepCount = 100

// scheduleTickInterval is how often schedules are checked.
const scheduleTickInterval = time.Minute

// autoSnapshotPrefix marks a snapshot a schedule created, in its name as well
// as its schedule_id, so it reads as automatic in a list that also shows
// hand-made ones.
const autoSnapshotPrefix = "auto-"

// Errors the schedule handlers map onto i18n codes.
var (
	ErrInvalidCron = errors.New("that is not a valid five-field cron expression")
	ErrInvalidKeep = errors.New("the retention count is out of range")
)

// SnapshotSchedule is one VM's scheduled snapshot policy.
type SnapshotSchedule struct {
	ID      string `json:"id"`
	VMID    string `json:"vmId"`
	Name    string `json:"name"`
	Cron    string `json:"cron"`
	Enabled bool   `json:"enabled"`

	KeepCount  int  `json:"keepCount"`
	WithMemory bool `json:"withMemory"`
	// PauseWithoutAgent stops the guest's vCPUs for the duration of the
	// snapshot when it has no guest agent to quiesce its filesystems. On by
	// default: a few seconds of pause buys a snapshot that is not the
	// equivalent of pulling the power.
	PauseWithoutAgent bool `json:"pauseWithoutAgent"`

	LastRunAt *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt *time.Time `json:"nextRunAt,omitempty"`
	// LastError is the reason the previous run failed, kept so a schedule that
	// has been failing quietly for a week is visible rather than inferred from
	// snapshots that are not there.
	LastError string `json:"lastError,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SnapshotRun is one firing of a schedule.
type SnapshotRun struct {
	ID           string     `json:"id"`
	ScheduleID   string     `json:"scheduleId"`
	VMID         string     `json:"vmId"`
	Status       string     `json:"status"`
	SnapshotID   string     `json:"snapshotId,omitempty"`
	SnapshotName string     `json:"snapshotName,omitempty"`
	Pruned       int        `json:"pruned"`
	Error        string     `json:"error,omitempty"`
	StartedAt    time.Time  `json:"startedAt"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
}

// ScheduleRequest is the create/update body.
type ScheduleRequest struct {
	Name              string `json:"name"`
	Cron              string `json:"cron"`
	Enabled           *bool  `json:"enabled,omitempty"`
	KeepCount         *int   `json:"keepCount,omitempty"`
	WithMemory        *bool  `json:"withMemory,omitempty"`
	PauseWithoutAgent *bool  `json:"pauseWithoutAgent,omitempty"`
}

// ensureScheduleSchema creates the schedule tables if they are missing.
//
// A plugin's migrations run once, at install, so 0002_snapshot_schedules.sql
// never reaches an installation that already has this plugin. Every statement
// names this plugin's own prefix, which is what the host's DB guard requires.
func (s *Service) ensureScheduleSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS plugin_addon_kvm_snapshot_schedules (
		    id          TEXT PRIMARY KEY,
		    vm_id       TEXT NOT NULL,
		    name        TEXT NOT NULL,
		    cron        TEXT NOT NULL,
		    enabled     INTEGER NOT NULL DEFAULT 1,
		    keep_count  INTEGER NOT NULL DEFAULT 7,
		    with_memory INTEGER NOT NULL DEFAULT 0,
		    last_run_at TEXT,
		    next_run_at TEXT,
		    last_error  TEXT,
		    created_at  TEXT NOT NULL,
		    updated_at  TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_snapshot_schedules_vm
		    ON plugin_addon_kvm_snapshot_schedules(vm_id)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_snapshot_schedules_enabled
		    ON plugin_addon_kvm_snapshot_schedules(enabled)`,
		`CREATE TABLE IF NOT EXISTS plugin_addon_kvm_snapshot_runs (
		    id            TEXT PRIMARY KEY,
		    schedule_id   TEXT,
		    vm_id         TEXT NOT NULL,
		    status        TEXT NOT NULL,
		    snapshot_id   TEXT,
		    snapshot_name TEXT,
		    pruned        INTEGER NOT NULL DEFAULT 0,
		    error         TEXT,
		    started_at    TEXT NOT NULL,
		    completed_at  TEXT,
		    created_at    TEXT NOT NULL,
		    updated_at    TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_snapshot_runs_schedule
		    ON plugin_addon_kvm_snapshot_runs(schedule_id)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_snapshot_runs_started
		    ON plugin_addon_kvm_snapshot_runs(started_at)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(ctx, statement); err != nil {
			return fmt.Errorf("preparing the snapshot schedule tables: %w", err)
		}
	}
	// SQLite has no ADD COLUMN IF NOT EXISTS, so a second run errors with
	// "duplicate column name" and that is the expected state, not a failure.
	if _, err := s.db.Exec(ctx,
		`ALTER TABLE plugin_addon_kvm_snapshots ADD COLUMN schedule_id TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return fmt.Errorf("adding the snapshot schedule column: %w", err)
	}
	if _, err := s.db.Exec(ctx,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_snapshots_schedule
		    ON plugin_addon_kvm_snapshots(schedule_id)`); err != nil {
		return fmt.Errorf("indexing the snapshot schedule column: %w", err)
	}
	return nil
}

// normalizeSchedule validates and fills in a request.
func normalizeSchedule(req *ScheduleRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Cron = strings.TrimSpace(req.Cron)
	if req.Name == "" {
		req.Name = "Snapshots"
	}
	if !validCronSpec(req.Cron) {
		return fmt.Errorf("%w: %q", ErrInvalidCron, req.Cron)
	}
	if req.KeepCount != nil && (*req.KeepCount < 1 || *req.KeepCount > maxKeepCount) {
		return fmt.Errorf("%w: %d", ErrInvalidKeep, *req.KeepCount)
	}
	return nil
}

// ListSchedules returns one guest's schedules.
func (s *Service) ListSchedules(ctx context.Context, vmID string) ([]SnapshotSchedule, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, vm_id, name, cron, enabled, keep_count, with_memory,
		       COALESCE(pause_guest, 1),
		       COALESCE(last_run_at, ''), COALESCE(next_run_at, ''),
		       COALESCE(last_error, ''), created_at, updated_at
		  FROM plugin_addon_kvm_snapshot_schedules
		 WHERE vm_id = ?
		 ORDER BY created_at
		 LIMIT ?`, vmID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("listing snapshot schedules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []SnapshotSchedule{}
	for rows.Next() {
		schedule, scanErr := scanSchedule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, schedule)
	}
	return out, rows.Err()
}

// scheduleScanner is the one method scanSchedule needs.
type scheduleScanner interface {
	Scan(dest ...any) error
}

// scanSchedule reads one schedule row.
func scanSchedule(rows scheduleScanner) (SnapshotSchedule, error) {
	var out SnapshotSchedule
	var enabled, withMemory, pauseGuest int
	var lastRun, nextRun, created, updated string
	if err := rows.Scan(&out.ID, &out.VMID, &out.Name, &out.Cron, &enabled,
		&out.KeepCount, &withMemory, &pauseGuest, &lastRun, &nextRun, &out.LastError,
		&created, &updated); err != nil {
		return SnapshotSchedule{}, fmt.Errorf("scanning a snapshot schedule: %w", err)
	}
	out.Enabled = enabled == 1
	out.WithMemory = withMemory == 1
	out.PauseWithoutAgent = pauseGuest == 1
	out.LastRunAt = parseOptionalTime(lastRun)
	out.NextRunAt = parseOptionalTime(nextRun)
	out.CreatedAt, _ = time.Parse(time.RFC3339, created)
	out.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return out, nil
}

// parseOptionalTime turns a stored timestamp into a pointer, so "never run" is
// null in the API rather than the zero instant.
func parseOptionalTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}

// CreateSchedule adds a schedule to a guest.
func (s *Service) CreateSchedule(ctx context.Context, vmID string, req ScheduleRequest) (SnapshotSchedule, error) {
	if err := normalizeSchedule(&req); err != nil {
		return SnapshotSchedule{}, err
	}
	if _, err := s.Get(ctx, vmID); err != nil {
		return SnapshotSchedule{}, err
	}

	now := time.Now().UTC()
	schedule := SnapshotSchedule{
		ID: xid.New().String(), VMID: vmID, Name: req.Name, Cron: req.Cron,
		Enabled: true, KeepCount: defaultKeepCount, PauseWithoutAgent: true,
		CreatedAt: now, UpdatedAt: now,
	}
	if req.Enabled != nil {
		schedule.Enabled = *req.Enabled
	}
	if req.KeepCount != nil {
		schedule.KeepCount = *req.KeepCount
	}
	if req.WithMemory != nil {
		schedule.WithMemory = *req.WithMemory
	}
	if req.PauseWithoutAgent != nil {
		schedule.PauseWithoutAgent = *req.PauseWithoutAgent
	}
	next := nextCronRun(schedule.Cron, now)
	schedule.NextRunAt = &next

	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_snapshot_schedules
			(id, vm_id, name, cron, enabled, keep_count, with_memory, pause_guest,
			 next_run_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		schedule.ID, schedule.VMID, schedule.Name, schedule.Cron,
		boolToInt(schedule.Enabled), schedule.KeepCount, boolToInt(schedule.WithMemory),
		boolToInt(schedule.PauseWithoutAgent),
		next.UTC().Format(time.RFC3339),
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return SnapshotSchedule{}, fmt.Errorf("creating a snapshot schedule: %w", err)
	}
	return schedule, nil
}

// UpdateSchedule changes a schedule.
//
// The next run is recomputed on every save rather than only when the cron
// changes: a schedule re-enabled after a week would otherwise carry a next run
// long in the past and fire immediately.
func (s *Service) UpdateSchedule(ctx context.Context, vmID, scheduleID string, req ScheduleRequest) (SnapshotSchedule, error) {
	if err := normalizeSchedule(&req); err != nil {
		return SnapshotSchedule{}, err
	}
	existing, err := s.schedule(ctx, vmID, scheduleID)
	if err != nil {
		return SnapshotSchedule{}, err
	}

	existing.Name = req.Name
	existing.Cron = req.Cron
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.KeepCount != nil {
		existing.KeepCount = *req.KeepCount
	}
	if req.WithMemory != nil {
		existing.WithMemory = *req.WithMemory
	}
	if req.PauseWithoutAgent != nil {
		existing.PauseWithoutAgent = *req.PauseWithoutAgent
	}

	now := time.Now().UTC()
	next := nextCronRun(existing.Cron, now)
	existing.NextRunAt = &next
	existing.UpdatedAt = now

	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_snapshot_schedules
		   SET name = ?, cron = ?, enabled = ?, keep_count = ?, with_memory = ?,
		       pause_guest = ?, next_run_at = ?, updated_at = ?
		 WHERE id = ? AND vm_id = ?`,
		existing.Name, existing.Cron, boolToInt(existing.Enabled),
		existing.KeepCount, boolToInt(existing.WithMemory),
		boolToInt(existing.PauseWithoutAgent),
		next.UTC().Format(time.RFC3339), now.Format(time.RFC3339),
		scheduleID, vmID); err != nil {
		return SnapshotSchedule{}, fmt.Errorf("updating a snapshot schedule: %w", err)
	}
	return existing, nil
}

// DeleteSchedule removes a schedule.
//
// The snapshots it made are left alone. They are the guest's restore points;
// deleting a policy is not a request to throw away what it produced.
func (s *Service) DeleteSchedule(ctx context.Context, vmID, scheduleID string) error {
	affected, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_kvm_snapshot_schedules WHERE id = ? AND vm_id = ?`,
		scheduleID, vmID)
	if err != nil {
		return fmt.Errorf("deleting a snapshot schedule: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// schedule reads one row.
func (s *Service) schedule(ctx context.Context, vmID, scheduleID string) (SnapshotSchedule, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, vm_id, name, cron, enabled, keep_count, with_memory,
		       COALESCE(pause_guest, 1),
		       COALESCE(last_run_at, ''), COALESCE(next_run_at, ''),
		       COALESCE(last_error, ''), created_at, updated_at
		  FROM plugin_addon_kvm_snapshot_schedules
		 WHERE id = ? AND vm_id = ?
		 LIMIT 1`, scheduleID, vmID)
	if err != nil {
		return SnapshotSchedule{}, fmt.Errorf("reading a snapshot schedule: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return SnapshotSchedule{}, ErrNotFound
	}
	return scanSchedule(rows)
}

// ListScheduleRuns returns a schedule's recent firings, newest first.
func (s *Service) ListScheduleRuns(ctx context.Context, vmID string, limit int) ([]SnapshotRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, COALESCE(schedule_id, ''), vm_id, status,
		       COALESCE(snapshot_id, ''), COALESCE(snapshot_name, ''), pruned,
		       COALESCE(error, ''), started_at, COALESCE(completed_at, '')
		  FROM plugin_addon_kvm_snapshot_runs
		 WHERE vm_id = ?
		 ORDER BY started_at DESC
		 LIMIT ?`, vmID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing snapshot runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []SnapshotRun{}
	for rows.Next() {
		var run SnapshotRun
		var started, completed string
		if err := rows.Scan(&run.ID, &run.ScheduleID, &run.VMID, &run.Status,
			&run.SnapshotID, &run.SnapshotName, &run.Pruned, &run.Error,
			&started, &completed); err != nil {
			return nil, fmt.Errorf("scanning a snapshot run: %w", err)
		}
		run.StartedAt, _ = time.Parse(time.RFC3339, started)
		run.CompletedAt = parseOptionalTime(completed)
		out = append(out, run)
	}
	return out, rows.Err()
}

// scheduleRunsBetween returns a guest's scheduled firings started inside
// [from, to), oldest first, for the snapshot schedule report.
func (s *Service) scheduleRunsBetween(ctx context.Context, vmID string, from, to time.Time) ([]SnapshotRun, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, COALESCE(schedule_id, ''), vm_id, status,
		       COALESCE(snapshot_id, ''), COALESCE(snapshot_name, ''), pruned,
		       COALESCE(error, ''), started_at, COALESCE(completed_at, '')
		  FROM plugin_addon_kvm_snapshot_runs
		 WHERE vm_id = ? AND started_at >= ? AND started_at < ?
		 ORDER BY started_at
		 LIMIT ?`,
		vmID, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339), maxRows)
	if err != nil {
		return nil, fmt.Errorf("listing snapshot runs in a period: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []SnapshotRun{}
	for rows.Next() {
		var run SnapshotRun
		var started, completed string
		if err := rows.Scan(&run.ID, &run.ScheduleID, &run.VMID, &run.Status,
			&run.SnapshotID, &run.SnapshotName, &run.Pruned, &run.Error,
			&started, &completed); err != nil {
			return nil, fmt.Errorf("scanning a snapshot run: %w", err)
		}
		run.StartedAt = parseTime(started)
		run.CompletedAt = parseOptionalTime(completed)
		out = append(out, run)
	}
	return out, rows.Err()
}

// scheduledSnapshotCounts counts the snapshots each of a guest's schedules
// still holds, which is what retention adherence is measured against.
func (s *Service) scheduledSnapshotCounts(ctx context.Context, vmID string) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT schedule_id, COUNT(*) FROM plugin_addon_kvm_snapshots
		 WHERE vm_id = ? AND schedule_id IS NOT NULL AND schedule_id != ''
		 GROUP BY schedule_id
		 LIMIT ?`, vmID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("counting scheduled snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]int{}
	for rows.Next() {
		var scheduleID string
		var count int
		if err := rows.Scan(&scheduleID, &count); err != nil {
			return nil, fmt.Errorf("counting scheduled snapshots: %w", err)
		}
		out[scheduleID] = count
	}
	return out, rows.Err()
}

// dueSchedules returns the enabled schedules whose next run has passed.
func (s *Service) dueSchedules(ctx context.Context, now time.Time) ([]SnapshotSchedule, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, vm_id, name, cron, enabled, keep_count, with_memory,
		       COALESCE(pause_guest, 1),
		       COALESCE(last_run_at, ''), COALESCE(next_run_at, ''),
		       COALESCE(last_error, ''), created_at, updated_at
		  FROM plugin_addon_kvm_snapshot_schedules
		 WHERE enabled = 1 AND next_run_at IS NOT NULL AND next_run_at <= ?
		 LIMIT ?`, now.UTC().Format(time.RFC3339), maxRows)
	if err != nil {
		return nil, fmt.Errorf("finding due snapshot schedules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SnapshotSchedule
	for rows.Next() {
		schedule, scanErr := scanSchedule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, schedule)
	}
	return out, rows.Err()
}

// RunSchedule fires one schedule: take a snapshot, then prune to the keep count.
//
// The next run is advanced whatever happens, including on failure. A schedule
// that failed because the host was down must not then fire on every tick for
// as long as it stays down.
func (s *Service) RunSchedule(ctx context.Context, schedule SnapshotSchedule) error {
	now := time.Now().UTC()
	runID := xid.New().String()
	stamp := now.Format(time.RFC3339)

	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_snapshot_runs
			(id, schedule_id, vm_id, status, started_at, created_at, updated_at)
		VALUES (?, ?, ?, 'running', ?, ?, ?)`,
		runID, schedule.ID, schedule.VMID, stamp, stamp, stamp); err != nil {
		return fmt.Errorf("recording a snapshot run: %w", err)
	}

	runErr := s.runScheduleOnce(ctx, schedule, runID)
	s.advanceSchedule(ctx, schedule, now, runErr)
	return runErr
}

// runScheduleOnce is RunSchedule's body, split out so every exit updates the
// run row exactly once.
func (s *Service) runScheduleOnce(ctx context.Context, schedule SnapshotSchedule, runID string) error {
	name := autoSnapshotPrefix + time.Now().UTC().Format("20060102-150405")

	snapshot, err := s.CreateSnapshot(withIntentOrigin(ctx, "schedule:"+schedule.ID, "schedule"), schedule.VMID, SnapshotRequest{
		Name:        name,
		Description: "Created by the schedule " + schedule.Name,
		WithMemory:  schedule.WithMemory,
		PauseGuest:  schedule.PauseWithoutAgent,
	})
	if err != nil {
		s.finishRun(ctx, runID, "failure", "", "", 0, err.Error())
		return err
	}

	// Tagged after the fact rather than threaded through CreateSnapshot: the
	// snapshot is already real at this point, and a schedule column is
	// bookkeeping rather than part of what makes it valid.
	if _, err := s.db.Exec(ctx,
		`UPDATE plugin_addon_kvm_snapshots SET schedule_id = ? WHERE id = ?`,
		schedule.ID, snapshot.ID); err != nil {
		s.logger.Warn("kvm: tagging a scheduled snapshot failed; retention will not see it",
			"snapshot", snapshot.ID, "schedule", schedule.ID, "error", err)
	}

	pruned, pruneErr := s.pruneSchedule(ctx, schedule)
	if pruneErr != nil {
		// The snapshot exists, so the run succeeded; retention did not. Said
		// plainly rather than reported as a failed snapshot, which would send
		// somebody looking for a snapshot that is right there.
		s.finishRun(ctx, runID, "success", snapshot.ID, snapshot.Name, pruned,
			"retention: "+pruneErr.Error())
		return nil
	}
	s.finishRun(ctx, runID, "success", snapshot.ID, snapshot.Name, pruned, "")
	return nil
}

// finishRun closes out a run row.
func (s *Service) finishRun(ctx context.Context, runID, status, snapshotID, snapshotName string, pruned int, errText string) {
	stamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_snapshot_runs
		   SET status = ?, snapshot_id = ?, snapshot_name = ?, pruned = ?,
		       error = ?, completed_at = ?, updated_at = ?
		 WHERE id = ?`,
		status, nullIfEmpty(snapshotID), nullIfEmpty(snapshotName), pruned,
		nullIfEmpty(errText), stamp, stamp, runID); err != nil {
		s.logger.Error("kvm: closing a snapshot run failed", "run", runID, "error", err)
	}
}

// advanceSchedule moves the schedule on and records the outcome.
func (s *Service) advanceSchedule(ctx context.Context, schedule SnapshotSchedule, ranAt time.Time, runErr error) {
	next := nextCronRun(schedule.Cron, ranAt)
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_snapshot_schedules
		   SET last_run_at = ?, next_run_at = ?, last_error = ?, updated_at = ?
		 WHERE id = ?`,
		ranAt.Format(time.RFC3339), next.UTC().Format(time.RFC3339),
		nullIfEmpty(errText), ranAt.Format(time.RFC3339), schedule.ID); err != nil {
		s.logger.Error("kvm: advancing a snapshot schedule failed; "+
			"it may fire again on the next tick",
			"schedule", schedule.ID, "error", err)
	}
}

// pruneSchedule deletes this schedule's snapshots past its keep count.
//
// Newest are kept. Deleting the oldest first is also the only order that works
// on a qcow2 chain: an older link can be folded into the one above it, and the
// reverse is not true.
func (s *Service) pruneSchedule(ctx context.Context, schedule SnapshotSchedule) (int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id FROM plugin_addon_kvm_snapshots
		 WHERE vm_id = ? AND schedule_id = ?
		 ORDER BY created_at DESC
		 LIMIT ? OFFSET ?`,
		schedule.VMID, schedule.ID, maxRows, schedule.KeepCount)
	if err != nil {
		return 0, fmt.Errorf("finding snapshots to prune: %w", err)
	}

	var expired []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scanning a snapshot to prune: %w", scanErr)
		}
		expired = append(expired, id)
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if rowsErr != nil {
		return 0, fmt.Errorf("finding snapshots to prune: %w", rowsErr)
	}

	// Oldest first, which the DESC + OFFSET above returns in reverse.
	pruned := 0
	for i := len(expired) - 1; i >= 0; i-- {
		if err := s.DeleteSnapshot(ctx, schedule.VMID, expired[i]); err != nil {
			// Stop at the first failure rather than carrying on: the chain is
			// ordered, and deleting past a link that would not go is how a
			// backing file ends up missing.
			return pruned, fmt.Errorf("pruning snapshot %s: %w", expired[i], err)
		}
		pruned++
	}
	return pruned, nil
}

// --- cron -----------------------------------------------------------------
//
// A five-field parser, deliberately a copy of the host's rather than an import:
// `modules/container_backups` is a host package and a plugin cannot reach into
// one. Same dialect, same local-timezone interpretation, so a spec that works
// for a container backup works here.

// validCronSpec reports whether a five-field spec parses.
func validCronSpec(spec string) bool {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return false
	}
	return cronFieldValid(fields[0], 0, 59) &&
		cronFieldValid(fields[1], 0, 23) &&
		cronFieldValid(fields[2], 1, 31) &&
		cronFieldValid(fields[3], 1, 12) &&
		cronFieldValid(fields[4], 0, 7)
}

// cronMatches reports whether a spec fires at this minute.
func cronMatches(spec string, now time.Time) bool {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return false
	}
	return cronFieldMatches(fields[0], now.Minute(), 0, 59, false) &&
		cronFieldMatches(fields[1], now.Hour(), 0, 23, false) &&
		cronFieldMatches(fields[2], now.Day(), 1, 31, false) &&
		cronFieldMatches(fields[3], int(now.Month()), 1, 12, false) &&
		cronFieldMatches(fields[4], int(now.Weekday()), 0, 7, true)
}

// nextCronRun is the first minute after `from` that the spec fires.
//
// Interpreted in the server's local timezone so `0 1 * * *` means 01:00 local;
// the container's TZ drives time.Local, and Go falls back to UTC when it is
// unset. Searching a year forward covers every spec that ever fires; one that
// never does — 31 February — falls back to a minute from now rather than
// looping, so it is visible as a schedule that keeps firing instead of a
// scheduler that hangs.
func nextCronRun(spec string, from time.Time) time.Time {
	start := from.In(time.Local).Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		candidate := start.Add(time.Duration(i) * time.Minute)
		if cronMatches(spec, candidate) {
			return candidate
		}
	}
	return start
}

// cronFieldValid checks one comma-separated field.
func cronFieldValid(expr string, minValue, maxValue int) bool {
	if strings.TrimSpace(expr) == "" {
		return false
	}
	for _, part := range strings.Split(expr, ",") {
		if !cronPartValid(strings.TrimSpace(part), minValue, maxValue) {
			return false
		}
	}
	return true
}

// cronFieldMatches checks one comma-separated field against a value.
func cronFieldMatches(expr string, value, minValue, maxValue int, weekday bool) bool {
	for _, part := range strings.Split(expr, ",") {
		if cronPartMatches(strings.TrimSpace(part), value, minValue, maxValue, weekday) {
			return true
		}
	}
	return false
}

// cronRange parses one part into its bounds and step.
func cronRange(part string, minValue, maxValue int) (start, end, step int, ok bool) {
	step = 1
	if strings.Contains(part, "/") {
		pieces := strings.SplitN(part, "/", 2)
		if len(pieces) != 2 || pieces[0] == "" {
			return 0, 0, 0, false
		}
		parsed, err := strconv.Atoi(pieces[1])
		if err != nil || parsed <= 0 {
			return 0, 0, 0, false
		}
		part, step = pieces[0], parsed
	}
	start, end = minValue, maxValue
	if part == "*" {
		return start, end, step, true
	}
	if strings.Contains(part, "-") {
		pieces := strings.SplitN(part, "-", 2)
		if len(pieces) != 2 {
			return 0, 0, 0, false
		}
		var err error
		if start, err = strconv.Atoi(pieces[0]); err != nil {
			return 0, 0, 0, false
		}
		if end, err = strconv.Atoi(pieces[1]); err != nil {
			return 0, 0, 0, false
		}
		return start, end, step, true
	}
	parsed, err := strconv.Atoi(part)
	if err != nil {
		return 0, 0, 0, false
	}
	return parsed, parsed, step, true
}

// cronPartValid checks one part of a field.
func cronPartValid(part string, minValue, maxValue int) bool {
	if part == "" {
		return false
	}
	start, end, _, ok := cronRange(part, minValue, maxValue)
	if !ok {
		return false
	}
	return start >= minValue && end <= maxValue && start <= end
}

// cronPartMatches checks one part of a field against a value.
//
// Sunday is both 0 and 7 in every cron dialect, so a field whose range reaches
// 7 sees Sunday as 7 rather than missing it.
func cronPartMatches(part string, value, minValue, maxValue int, weekday bool) bool {
	start, end, step, ok := cronRange(part, minValue, maxValue)
	if !ok {
		return false
	}
	if weekday && end == 7 && value == 0 {
		value = 7
	}
	if value < start || value > end {
		return false
	}
	return (value-start)%step == 0
}
