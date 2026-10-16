// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/db"
)

// hostUpdateCheckTimeout bounds a package list refresh over the agent. The
// agent gives itself five minutes; the server allows a little more so the
// agent's own answer wins the race and the operator gets a real error.
const hostUpdateCheckTimeout = 6 * time.Minute

// hostUpdateLogCap is how many output lines an apply job keeps. A dist-upgrade
// of 400 packages prints tens of thousands and nobody reads past the tail.
const hostUpdateLogCap = 5000

// HostUpdateService owns host OS inventory, the scheduled update check, and
// the apply jobs whose progress the UI streams.
type HostUpdateService struct {
	svc       *Service
	agentPool *coreagent.AgentPool
	database  *sql.DB
	logger    *slog.Logger

	mu   sync.Mutex
	jobs map[string]*HostUpdateJob
}

// NewHostUpdateService wires the host update service to the plain system
// service, which still owns the container-based path for local environments.
func NewHostUpdateService(svc *Service, agentPool *coreagent.AgentPool, database *sql.DB, logger *slog.Logger) *HostUpdateService {
	return &HostUpdateService{
		svc:       svc,
		agentPool: agentPool,
		database:  database,
		logger:    logger,
		jobs:      make(map[string]*HostUpdateJob),
	}
}

// OSInfo describes the operating system the target host runs.
func (s *HostUpdateService) OSInfo(ctx context.Context, target HostTarget) (*HostOSInfo, error) {
	if target.Self {
		info, err := s.osInfoViaContainer(ctx, target)
		if reason, ok := selfHostReason(err); ok {
			return &HostOSInfo{Unsupported: reason}, nil
		}
		return info, err
	}
	envID := target.EnvID
	if !s.svc.pool.IsAgentEnvContext(ctx, envID) {
		return s.osInfoViaContainer(ctx, target)
	}
	t := s.agentPool.ResolveTransport(envID)
	if t == nil {
		return &HostOSInfo{Unsupported: "The agent for this environment is not connected."}, nil
	}
	// An unsupported agent is a fact about the environment, not a server fault:
	// returning it as data lets the tab explain itself and disable its controls
	// instead of showing a red failure the operator cannot act on.
	if !t.HostUpdatesSupported() {
		return &HostOSInfo{Unsupported: "This environment's agent is too old to report host OS details. Update the agent from Settings, Updates."}, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	payload, err := t.HostOSInfo(callCtx)
	if err != nil {
		return nil, err
	}
	return &HostOSInfo{
		PrettyName:     payload.PrettyName,
		Distro:         payload.Distro,
		Version:        payload.Version,
		Kernel:         payload.Kernel,
		Arch:           payload.Arch,
		Hostname:       payload.Hostname,
		UptimeSeconds:  payload.UptimeSeconds,
		Manager:        payload.Manager,
		Virtualization: payload.Virtualization,
		RebootRequired: payload.RebootRequired,
		Unsupported:    payload.Error,
	}, nil
}

// osInfoViaContainer reads the same facts for a local environment, or the self
// host, through the utility container that already backs logs and checks there.
func (s *HostUpdateService) osInfoViaContainer(ctx context.Context, target HostTarget) (*HostOSInfo, error) {
	result, err := s.svc.runTarget(ctx, target, hostOSInfoScript, false, 60*time.Second)
	if err != nil {
		return nil, err
	}
	info := &HostOSInfo{}
	for _, line := range strings.Split(result.Output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"`)
		switch key {
		case "MCHARBOR_PRETTY":
			info.PrettyName = value
		case "MCHARBOR_DISTRO":
			info.Distro = value
		case "MCHARBOR_VERSION":
			info.Version = value
		case "MCHARBOR_KERNEL":
			info.Kernel = value
		case "MCHARBOR_ARCH":
			info.Arch = value
		case "MCHARBOR_HOSTNAME":
			info.Hostname = value
		case "MCHARBOR_UPTIME":
			info.UptimeSeconds = parseUptimeSeconds(value)
		case "MCHARBOR_MANAGER":
			info.Manager = value
		case "MCHARBOR_VIRT":
			info.Virtualization = value
		case "MCHARBOR_REBOOT":
			info.RebootRequired = value == "1"
		}
	}
	return info, nil
}

// Check refreshes the pending-package list and caches it on the target so the
// tab can show a result without re-running the package manager.
func (s *HostUpdateService) Check(ctx context.Context, target HostTarget) (*OSUpdateCheckResult, error) {
	var result *OSUpdateCheckResult
	envID := target.EnvID

	if !target.Self && s.svc.pool.IsAgentEnvContext(ctx, envID) {
		t := s.agentPool.ResolveTransport(envID)
		if t == nil {
			return nil, fmt.Errorf("agent is not connected for environment %s", envID)
		}
		callCtx, cancel := context.WithTimeout(ctx, hostUpdateCheckTimeout)
		defer cancel()

		payload, err := t.HostUpdateCheck(callCtx)
		if err != nil {
			return nil, err
		}
		if payload.Error != "" {
			return nil, fmt.Errorf("%s", payload.Error)
		}
		result = &OSUpdateCheckResult{
			Manager:   payload.Manager,
			Available: payload.Available,
			Updates:   payload.Updates,
			Output:    payload.Output,
			CheckedAt: time.Now().UTC(),
		}
	} else {
		checked, err := s.svc.CheckUpdates(ctx, target)
		if err != nil {
			return nil, err
		}
		result = checked
	}

	if err := s.cacheResult(ctx, target, result); err != nil {
		s.logger.Error("system: caching host update result failed", "target", target.rowID(), "error", err)
	}
	return result, nil
}

// cacheResult persists the last check so the UI and the environment list can
// show "updates available" without paying for a package-manager run.
func (s *HostUpdateService) cacheResult(ctx context.Context, target HostTarget, result *OSUpdateCheckResult) error {
	packages, err := json.Marshal(result.Updates)
	if err != nil {
		return fmt.Errorf("encoding package list: %w", err)
	}
	available := 0
	if result.Available {
		available = 1
	}
	_, err = s.database.ExecContext(ctx, `UPDATE `+target.table()+ // SQL identifier: HostTarget.table() returns one of two constants
		` SET host_update_last_checked_at = ?,
		    host_update_available = ?,
		    host_update_count = ?,
		    host_update_manager = ?,
		    host_update_packages = ?
		WHERE id = ?`,
		result.CheckedAt.Format(time.RFC3339), available, len(result.Updates),
		result.Manager, string(packages), target.rowID())
	if err != nil {
		return fmt.Errorf("caching host update result: %w", err)
	}
	return nil
}

// State returns the cached check plus the schedule, which is what the tab
// renders before the operator asks for anything.
func (s *HostUpdateService) State(ctx context.Context, target HostTarget) (*HostUpdateState, error) {
	envID := target.rowID()
	query := "SELECT " + // SQL identifier: HostTarget.table() returns one of two constants
		`host_update_check_enabled, host_update_check_time, host_update_check_days,
		       host_update_last_checked_at, host_update_available, host_update_count,
		       host_update_manager, host_update_packages, host_update_with_new_packages,
		       host_update_skip_unavailable, host_update_remove_unused
		FROM ` + target.table() + ` WHERE id = ?`
	row := s.database.QueryRowContext(ctx, query, envID)

	var enabled, available, withNew, skipUnavailable, removeUnused int
	var checkTime, checkDays string
	var lastChecked, manager, packages sql.NullString
	var count int
	if err := row.Scan(&enabled, &checkTime, &checkDays, &lastChecked, &available,
		&count, &manager, &packages, &withNew, &skipUnavailable, &removeUnused); err != nil {
		return nil, fmt.Errorf("reading host update state: %w", err)
	}

	state := &HostUpdateState{
		Schedule: HostUpdateSchedule{
			Enabled:         enabled == 1,
			Time:            checkTime,
			Days:            splitDays(checkDays),
			WithNewPackages: withNew == 1,
			SkipUnavailable: skipUnavailable == 1,
			RemoveUnused:    removeUnused == 1,
		},
		Available: available == 1,
		Count:     count,
		Manager:   manager.String,
	}
	if !target.Self && s.svc.pool.IsAgentEnvContext(ctx, envID) {
		if t := s.agentPool.ResolveTransport(envID); t != nil && !t.HostUpdateRecoverySupported() {
			state.RecoveryUnsupported = true
		}
	}
	if lastChecked.Valid && lastChecked.String != "" {
		if parsed, err := time.Parse(time.RFC3339, lastChecked.String); err == nil {
			state.LastCheckedAt = &parsed
		}
	}
	if packages.Valid && packages.String != "" {
		if err := json.Unmarshal([]byte(packages.String), &state.Updates); err != nil {
			s.logger.Warn("system: cached host package list is unreadable", "env", envID)
		}
	}
	return state, nil
}

// SaveSchedule stores when the automatic check should run for the target, and
// its default upgrade policy.
func (s *HostUpdateService) SaveSchedule(ctx context.Context, target HostTarget, schedule HostUpdateSchedule) error {
	if !validClockTime(schedule.Time) {
		return fmt.Errorf("a schedule time must be HH:MM")
	}
	days := normaliseDays(schedule.Days)
	if schedule.Enabled && days == "" {
		return fmt.Errorf("an enabled schedule needs at least one day")
	}
	enabled := 0
	if schedule.Enabled {
		enabled = 1
	}
	result, err := s.database.ExecContext(ctx, `UPDATE `+target.table()+ // SQL identifier: HostTarget.table() returns one of two constants
		` SET host_update_check_enabled = ?, host_update_check_time = ?, host_update_check_days = ?,
		    host_update_with_new_packages = ?, host_update_skip_unavailable = ?,
		    host_update_remove_unused = ?
		WHERE id = ?`, enabled, schedule.Time, days, boolInt(schedule.WithNewPackages),
		boolInt(schedule.SkipUnavailable), boolInt(schedule.RemoveUnused), target.rowID())
	if err != nil {
		return fmt.Errorf("saving host update schedule: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// HostUpdateJob is one running or finished upgrade, kept in memory so the
// progress stream survives a browser reload while the upgrade runs.
type HostUpdateJob struct {
	ID        string
	EnvID     string
	Target    HostTarget
	StartedAt time.Time

	mu       sync.Mutex
	lines    []string
	done     bool
	success  bool
	failure  string
	hint     string
	finished time.Time
	waiters  []chan struct{}
}

// Snapshot returns the job's output from offset onwards plus its state, which
// is exactly what one poll or one SSE frame needs.
func (j *HostUpdateJob) Snapshot(offset int) ([]string, bool, bool, string, int) {
	lines, done, success, failure, _, total := j.SnapshotWithHint(offset)
	return lines, done, success, failure, total
}

// SnapshotWithHint is Snapshot plus the hint a failed run carries, naming the
// option that would have let it through.
func (j *HostUpdateJob) SnapshotWithHint(offset int) ([]string, bool, bool, string, string, int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if offset < 0 || offset > len(j.lines) {
		offset = len(j.lines)
	}
	// The tail is allocated non-nil even when empty: a nil slice marshals as
	// JSON null, and the stream's final frame carries no new lines.
	tail := make([]string, 0, len(j.lines)-offset)
	tail = append(tail, j.lines[offset:]...)
	return tail, j.done, j.success, j.failure, j.hint, len(j.lines)
}

// Wait blocks until the job produces more output than offset, or until ctx
// ends. It is what keeps the SSE handler from polling in a tight loop.
func (j *HostUpdateJob) Wait(ctx context.Context, offset int) {
	j.mu.Lock()
	if j.done || len(j.lines) > offset {
		j.mu.Unlock()
		return
	}
	signal := make(chan struct{})
	j.waiters = append(j.waiters, signal)
	j.mu.Unlock()

	select {
	case <-signal:
	case <-ctx.Done():
	}
}

// append records one output line and wakes every reader waiting on progress.
func (j *HostUpdateJob) append(line string) {
	j.mu.Lock()
	if len(j.lines) < hostUpdateLogCap {
		j.lines = append(j.lines, line)
	}
	j.wake()
	j.mu.Unlock()
}

// finish marks the job complete and releases every reader.
func (j *HostUpdateJob) finish(success bool, failure string) {
	j.finishWithHint(success, failure, "")
}

// finishWithHint is finish for a run that may know which option would have let
// it through.
func (j *HostUpdateJob) finishWithHint(success bool, failure, hint string) {
	j.mu.Lock()
	j.done = true
	j.hint = hint
	j.success = success
	j.failure = failure
	j.finished = time.Now().UTC()
	j.wake()
	j.mu.Unlock()
}

// wake releases the waiters. The caller holds j.mu.
func (j *HostUpdateJob) wake() {
	for _, waiter := range j.waiters {
		close(waiter)
	}
	j.waiters = nil
}

// Job returns a running or recently finished apply job.
func (s *HostUpdateService) Job(jobID string) *HostUpdateJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobs[jobID]
}

// ActiveJob returns the target's current apply job, so a reloaded tab
// reattaches to the upgrade already in flight instead of offering to start a
// second one.
func (s *HostUpdateService) ActiveJob(target HostTarget) *HostUpdateJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, job := range s.jobs {
		if job.Target != target {
			continue
		}
		job.mu.Lock()
		running := !job.done
		job.mu.Unlock()
		if running {
			return job
		}
	}
	return nil
}

// StartApply begins an upgrade and returns immediately with the job that
// carries its progress.
//
// The job is detached from ctx's cancellation on purpose: an upgrade must not be
// cancelled because the operator closed the tab that started it.
func (s *HostUpdateService) StartApply(ctx context.Context, target HostTarget, overrides HostUpdateOverrides) (*HostUpdateJob, error) {
	if existing := s.ActiveJob(target); existing != nil {
		return existing, nil
	}

	job := &HostUpdateJob{
		ID:        xid.New().String(),
		EnvID:     target.EnvID,
		Target:    target,
		StartedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()

	go s.runApply(ctx, job, overrides)
	return job, nil
}

// HostUpdateOverrides replaces the environment's stored upgrade policy for one
// run; a nil field keeps the stored value.
type HostUpdateOverrides struct {
	WithNewPackages *bool
	SkipUnavailable *bool
	RemoveUnused    *bool
}

// applyPolicy reads how far this host's operator allowed an upgrade to go and
// applies the run's overrides. A read failure yields the conservative answer
// rather than an error, because refusing to upgrade at all is worse.
func (s *HostUpdateService) applyPolicy(ctx context.Context, target HostTarget, overrides HostUpdateOverrides) hostUpgradeOptions {
	var withNew, skipUnavailable, removeUnused int
	err := s.database.QueryRowContext(ctx, `
		SELECT host_update_with_new_packages, host_update_skip_unavailable, host_update_remove_unused
		FROM `+target.table()+` WHERE id = ?`, target.rowID()).Scan(&withNew, &skipUnavailable, &removeUnused)
	if err != nil {
		s.logger.Warn("system: reading host update package policy failed", "target", target.rowID(), "error", err)
		withNew, skipUnavailable, removeUnused = 0, 0, 0
	}
	opts := hostUpgradeOptions{WithNewPackages: withNew == 1, SkipUnavailable: skipUnavailable == 1, RemoveUnused: removeUnused == 1}
	if overrides.WithNewPackages != nil {
		opts.WithNewPackages = *overrides.WithNewPackages
	}
	if overrides.SkipUnavailable != nil {
		opts.SkipUnavailable = *overrides.SkipUnavailable
	}
	if overrides.RemoveUnused != nil {
		opts.RemoveUnused = *overrides.RemoveUnused
	}
	return opts
}

// failureHint names the option that would have let a failed run through: a
// download failure on a manager that honours Skip unavailable, with it off.
func failureHint(success bool, manager, output string, opts hostUpgradeOptions) string {
	if success || opts.SkipUnavailable || !skipUnavailableHonoured(manager) {
		return ""
	}
	if downloadFailed(manager, output) {
		return hostUpdateHintSkipUnavailable
	}
	return ""
}

// hostUpdateHintSkipUnavailable is the hint the UI translates into "turn on
// Skip unavailable packages".
const hostUpdateHintSkipUnavailable = "skipUnavailable"

// boolInt stores a bool in the INTEGER 0/1 columns SQLite uses for flags.
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// runApply drives one upgrade to completion and records the outcome on the job.
func (s *HostUpdateService) runApply(parent context.Context, job *HostUpdateJob, overrides HostUpdateOverrides) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Hour)
	defer cancel()

	opts := s.applyPolicy(ctx, job.Target, overrides)

	if job.Target.Self || !s.svc.pool.IsAgentEnvContext(ctx, job.EnvID) {
		result, err := s.svc.ApplyUpdates(ctx, job.Target, opts)
		if err != nil {
			job.finish(false, err.Error())
			return
		}
		for _, line := range strings.Split(strings.TrimRight(result.Output, "\n"), "\n") {
			job.append(line)
		}
		job.finishWithHint(result.Success, "", failureHint(result.Success, result.Manager, result.Output, opts))
		s.refreshAfterApply(ctx, job.Target)
		return
	}

	t := s.agentPool.ResolveTransport(job.EnvID)
	if t == nil {
		job.finish(false, "agent is not connected")
		return
	}
	if !t.HostUpdateRecoverySupported() {
		job.append("McHarbor: this agent predates Skip unavailable packages and the automatic download retry; update the agent to use them.")
	}
	stream, err := t.HostUpdateApply(ctx, coreagent.HostUpdateApplyOptions{
		WithNewPackages: opts.WithNewPackages,
		SkipUnavailable: opts.SkipUnavailable,
		RemoveUnused:    opts.RemoveUnused,
	})
	if err != nil {
		job.finish(false, err.Error())
		return
	}
	for line := range stream.Lines {
		job.append(line)
	}
	payload, err := stream.Result()
	if err != nil {
		job.finish(false, err.Error())
		return
	}
	if payload != nil && payload.Error != "" {
		job.finish(false, payload.Error)
		return
	}
	success := payload != nil && payload.Success
	manager, output := "", ""
	if payload != nil {
		manager, output = payload.Manager, payload.Output
	}
	job.finishWithHint(success, "", failureHint(success, manager, output, opts))
	s.refreshAfterApply(ctx, job.Target)
}

// refreshAfterApply re-runs the check so the cached count reflects what is left
// rather than what was pending before the upgrade.
func (s *HostUpdateService) refreshAfterApply(parent context.Context, target HostTarget) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), hostUpdateCheckTimeout)
	defer cancel()
	if _, err := s.Check(ctx, target); err != nil {
		s.logger.Warn("system: post-upgrade host update check failed", "target", target.rowID(), "error", err)
	}
}

// PruneJobs drops finished jobs older than an hour so a long-lived server does
// not accumulate upgrade transcripts forever.
func (s *HostUpdateService) PruneJobs() {
	cutoff := time.Now().UTC().Add(-time.Hour)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, job := range s.jobs {
		job.mu.Lock()
		stale := job.done && job.finished.Before(cutoff)
		job.mu.Unlock()
		if stale {
			delete(s.jobs, id)
		}
	}
}
