// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"context"
	"fmt"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/cronspec"
)

// Notifier announces a newly available agent release to the operator.
type Notifier func(ctx context.Context, title, message string)

// failedCheckRetry is how soon a failed check runs again, so one DNS or network
// blip does not leave the catalog unread until the next daily cron fire.
const failedCheckRetry = 15 * time.Minute

// LeaderFunc reports whether this node should run the scheduled check; on a
// cluster only the leader pulls, so two nodes never download the same release.
type LeaderFunc func(ctx context.Context) bool

// RunScheduler checks for new releases on the configured cron until ctx ends.
// Each tick re-reads the settings, so a changed schedule applies within a minute.
func (s *Store) RunScheduler(ctx context.Context, notify Notifier, leader LeaderFunc) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		var next time.Time
		for {
			next = s.tick(ctx, s.now(), next, notify, leader)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// tick runs one scheduler minute and returns when the next check is due.
func (s *Store) tick(ctx context.Context, now, next time.Time, notify Notifier, leader LeaderFunc) time.Time {
	settings, err := s.Settings(ctx)
	if err != nil {
		s.logger.Warn("agent-releases: reading settings", "error", err)
		return next
	}
	if !settings.Enabled {
		return time.Time{}
	}
	if next.IsZero() {
		next = cronspec.Next(settings.CheckCron, now)
		if s.lastCheckFailed(ctx) {
			next = now
		}
		s.recordNextCheck(ctx, next)
		return next
	}
	if now.Before(next) {
		return next
	}
	next = cronspec.Next(settings.CheckCron, now)
	s.recordNextCheck(ctx, next)
	if leader != nil && !leader(ctx) {
		return next
	}
	checkCtx, cancel := context.WithTimeout(ctx, pullBudget)
	defer cancel()
	if err := s.CheckAndAct(checkCtx, notify); err != nil {
		s.logger.Warn("agent-releases: scheduled check failed", "error", err)
		if s.lastCheckFailed(ctx) {
			if retry := now.Add(failedCheckRetry); retry.Before(next) {
				next = retry
				s.recordNextCheck(ctx, next)
			}
		}
	}
	return next
}

// lastCheckFailed reports whether the cached state has no answer from any
// source: never checked, or the last check ended in an error.
func (s *Store) lastCheckFailed(ctx context.Context) bool {
	st, err := s.State(ctx)
	if err != nil {
		return false
	}
	return st.LastCheckedAt == "" || st.LastError != ""
}

func (s *Store) recordNextCheck(ctx context.Context, next time.Time) {
	st, err := s.State(ctx)
	if err != nil {
		return
	}
	st.NextCheckAt = next.UTC().Format(time.RFC3339)
	if err := s.saveState(ctx, st); err != nil {
		s.logger.Debug("agent-releases: recording next check", "error", err)
	}
}

// CheckAndAct runs one check and then does what the configured mode asks:
// announce a newer release once, download it, or download it and make it the
// target so the agent auto-update settings roll it out.
func (s *Store) CheckAndAct(ctx context.Context, notify Notifier) error {
	st, err := s.Check(ctx)
	if err != nil {
		return err
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	if st.LatestVersion == "" || !s.isNewerThanServed(ctx, st.LatestVersion, st.LatestBuild) {
		return nil
	}
	if (st.NotifiedVersion != st.LatestVersion || st.NotifiedBuild != st.LatestBuild) && notify != nil {
		label := st.LatestVersion
		if st.LatestBuild != "" {
			label += " (build " + st.LatestBuild + ")"
		}
		notify(ctx, fmt.Sprintf("McHarbor agent %s is available", label),
			fmt.Sprintf("Agent release %s was found at %s. Review it under Settings > Updates > Agents.", label, st.LatestSource))
		st.NotifiedVersion, st.NotifiedBuild = st.LatestVersion, st.LatestBuild
		if err := s.saveState(ctx, st); err != nil {
			return err
		}
	}
	if settings.Mode == ModeNotify {
		return nil
	}
	rel, err := s.Pull(ctx, st.LatestVersion, "", "scheduler")
	if err != nil {
		return err
	}
	if settings.Mode == ModeRollout && rel.Status == StatusReady {
		return s.SetTarget(ctx, rel.Version, rel.Build)
	}
	return nil
}

// isNewerThanServed compares a release against what agents are given today:
// the target when one is set, else the agent this McHarbor was built with.
func (s *Store) isNewerThanServed(ctx context.Context, version, build string) bool {
	if rel, ok := s.targetRelease(ctx); ok {
		return newer(version, build, rel.Version, rel.Build)
	}
	return newer(version, build, coreagent.BundledAgentVersion, "")
}
