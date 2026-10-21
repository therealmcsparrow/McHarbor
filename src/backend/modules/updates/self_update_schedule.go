// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/cronspec"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
	"github.com/therealmcsparrow/mcharbor/core/runningwork"
)

// Reasons an install waits, stable codes the interface translates.
const (
	BusyMaintenance   = "maintenance"
	BusyBackupRunning = "backup_running"
	BusyExportRunning = "export_running"
)

// BusyChecker names work that an update would interrupt, as one of the Busy
// codes, or returns "" when the instance is quiet enough to restart.
type BusyChecker func(ctx context.Context) string

// schedulePlan is the checker's view of its two schedules. It lives in memory:
// after a restart the next check and window are computed afresh from now.
type schedulePlan struct {
	nextCheck   time.Time
	nextInstall time.Time
	windowOpen  time.Time
	windowDone  bool
}

// SetBusyChecker wires what counts as work in flight.
func (c *SelfUpdateChecker) SetBusyChecker(b BusyChecker) { c.busy = b }

// ValidSchedule reports whether both crons are ones the scheduler accepts.
func ValidSchedule(s SelfUpdateSettings) bool {
	return (s.CheckCron == "" || cronspec.Valid(s.CheckCron)) &&
		(s.InstallCron == "" || cronspec.Valid(s.InstallCron))
}

func normalizeSchedule(in SelfUpdateSettings) (checkCron, installCron string, window int) {
	checkCron, installCron, window = strings.TrimSpace(in.CheckCron), strings.TrimSpace(in.InstallCron), in.InstallWindowMinutes
	if checkCron == "" {
		checkCron = defaultCheckCron
	}
	if installCron == "" {
		installCron = defaultInstallCron
	}
	if window <= 0 {
		window = defaultInstallWindowMin
	}
	if window > maxInstallWindowMin {
		window = maxInstallWindowMin
	}
	return checkCron, installCron, window
}

// tick is one minute of the scheduler: run the check when its cron comes
// due, open the install window when its cron does, and inside the window
// install only when nothing is in flight.
func (c *SelfUpdateChecker) tick(ctx context.Context, now time.Time) {
	settings, err := c.Settings(ctx)
	if err != nil {
		c.logger.Warn("self-update: failed to read settings", "error", err)
		return
	}
	if !settings.Enabled {
		return
	}
	if !c.leaderFor(ctx) {
		c.logger.Debug("self-update: not leader this tick; skipping", "node", c.nodeID)
		return
	}
	checkCron, installCron, windowMin := normalizeSchedule(settings)

	c.mu.Lock()
	if c.plan.nextCheck.IsZero() {
		c.plan.nextCheck = cronspec.Next(checkCron, now)
	}
	if c.plan.nextInstall.IsZero() {
		c.plan.nextInstall = cronspec.Next(installCron, now)
	}
	checkDue := !now.Before(c.plan.nextCheck)
	if checkDue {
		c.plan.nextCheck = cronspec.Next(checkCron, now)
	}
	if !now.Before(c.plan.nextInstall) {
		c.plan.windowOpen, c.plan.windowDone = c.plan.nextInstall, false
		c.plan.nextInstall = cronspec.Next(installCron, now)
	}
	plan := c.plan
	c.state.NextCheckAt = plan.nextCheck.UTC().Format(time.RFC3339)
	c.state.NextInstallAt = plan.nextInstall.UTC().Format(time.RFC3339)
	c.mu.Unlock()

	if checkDue {
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if _, err := c.CheckNow(checkCtx); err != nil && !errors.Is(err, ErrSelfUpdateDisabled) {
			c.logger.Warn("self-update: scheduled check failed", "error", err)
		}
		cancel()
	}

	inWindow := !plan.windowOpen.IsZero() && !plan.windowDone &&
		now.Before(plan.windowOpen.Add(time.Duration(windowMin)*time.Minute))
	if !inWindow {
		if !plan.windowOpen.IsZero() && !plan.windowDone {
			c.closeWindow(ctx)
		}
		return
	}
	state := c.State()
	pendingInstall := settings.AutoInstall && state.UpdateAvailable && state.LatestVersion != "" && state.AutoInstallVersion != releaseLabel(state.LatestVersion, state.LatestBuild)
	if !pendingInstall && !settings.AutoUpdateAgents {
		c.closeWindow(ctx)
		return
	}
	if reason := c.busyReason(ctx); reason != "" {
		c.deferInstall(ctx, reason)
		return
	}
	if state.InstallDeferred != "" || state.InstallSkipped {
		state.InstallDeferred, state.InstallSkipped = "", false
		c.persistAndCache(ctx, state)
	}
	if settings.AutoUpdateAgents {
		agentCtx, cancelAgents := context.WithTimeout(ctx, agentUpdateAckTimeout+time.Minute)
		c.UpdateAgents(agentCtx, false, settings.AgentExcludedEnvIDs)
		cancelAgents()
	}
	c.closeWindow(ctx)
	if pendingInstall {
		installCtx, cancelInstall := context.WithTimeout(ctx, 2*time.Minute)
		c.AutoInstall(installCtx, settings, c.State())
		cancelInstall()
	}
}

func (c *SelfUpdateChecker) busyReason(ctx context.Context) string {
	if quiesce.Active() {
		return BusyMaintenance
	}
	if c.busy == nil {
		return ""
	}
	return c.busy(ctx)
}

func (c *SelfUpdateChecker) deferInstall(ctx context.Context, reason string) {
	state := c.State()
	if state.InstallDeferred != reason || state.InstallSkipped {
		c.logger.Info("self-update: install window open, waiting for work to finish", "reason", reason)
		state.InstallDeferred, state.InstallSkipped = reason, false
		c.persistAndCache(ctx, state)
	}
}

// closeWindow ends the current window. A window that closes while still
// deferred is recorded as skipped, so the operator learns why nothing ran.
func (c *SelfUpdateChecker) closeWindow(ctx context.Context) {
	c.mu.Lock()
	c.plan.windowDone = true
	deferred := c.state.InstallDeferred != "" && !c.state.InstallSkipped
	c.mu.Unlock()
	if !deferred {
		return
	}
	state := c.State()
	state.InstallSkipped = true
	c.persistAndCache(ctx, state)
}

// DBBusyChecker reports work an update would interrupt, from the rows that
// record it: container backup and restore runs, and instance exports and
// verifications. An instance restore puts the server in maintenance mode,
// which the checker sees on its own.
func DBBusyChecker(db *sql.DB) BusyChecker {
	reasons := map[runningwork.Kind]string{
		runningwork.ContainerBackups: BusyBackupRunning,
		runningwork.InstanceExports:  BusyExportRunning,
	}
	return func(ctx context.Context) string {
		// bounded by runningwork.Count, a fixed list of two kinds.
		for _, r := range runningwork.Count(ctx, db) {
			// Unreadable counts as busy: restarting over work that could not be
			// ruled out is the failure this check exists to prevent.
			if r.Err != nil || r.Count > 0 {
				return reasons[r.Kind]
			}
		}
		return ""
	}
}
