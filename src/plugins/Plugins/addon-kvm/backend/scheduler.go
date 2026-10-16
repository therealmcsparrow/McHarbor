// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"sync"
	"time"
)

// scheduleSweepTimeout bounds one pass over the due schedules.
//
// Generous because a memory-state snapshot of a large guest legitimately takes
// minutes, and a sweep that gave up halfway would leave a run row reading
// "running" with nothing behind it.
const scheduleSweepTimeout = 45 * time.Minute

// snapshotScheduler fires snapshot schedules as they come due.
//
// One goroutine with an explicit stop, like the reconciler beside it: the
// plugin contract has a Shutdown and a background loop that outlived it would
// keep writing to a database the host is closing.
type snapshotScheduler struct {
	svc    *Service
	logger pluginLogger
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
	// running guards against a slow schedule being started twice. A snapshot
	// can outlast the tick interval, and a second one firing on top of the
	// first would race two agents over the same disk.
	running sync.Map
}

func newSnapshotScheduler(svc *Service, logger pluginLogger) *snapshotScheduler {
	return &snapshotScheduler{
		svc:    svc,
		logger: logger,
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Start runs the loop until Stop is called.
func (s *snapshotScheduler) Start() {
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(scheduleTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				s.sweep()
			}
		}
	}()
}

// Stop ends the loop and waits for the current sweep to finish.
func (s *snapshotScheduler) Stop() {
	s.once.Do(func() { close(s.stop) })
	select {
	case <-s.done:
	case <-time.After(30 * time.Second):
		// A snapshot in flight can legitimately outlast this. Shutdown is not
		// held any longer than that; the run row is closed out by the sweep if
		// it survives, and corrected by the next start if it does not.
		s.logger.Warn("kvm: the snapshot scheduler did not stop in time")
	}
}

// sweep runs every schedule that has come due.
func (s *snapshotScheduler) sweep() {
	ctx, cancel := context.WithTimeout(context.Background(), scheduleSweepTimeout)
	defer cancel()

	now := time.Now().UTC()
	due, err := s.svc.dueSchedules(ctx, now)
	if err != nil {
		s.logger.Error("kvm: reading due snapshot schedules failed", "error", err)
		return
	}
	for _, schedule := range due {
		if _, busy := s.running.LoadOrStore(schedule.ID, true); busy {
			s.logger.Warn("kvm: a snapshot schedule is still running from its last firing; skipping this one",
				"schedule", schedule.ID, "vm", schedule.VMID)
			continue
		}
		func() {
			defer s.running.Delete(schedule.ID)
			if err := s.svc.RunSchedule(ctx, schedule); err != nil {
				// Already recorded on the run row and the schedule, which is
				// what the UI reads; the log line is for an operator watching
				// the container rather than the page.
				s.logger.Warn("kvm: a scheduled snapshot failed",
					"schedule", schedule.ID, "vm", schedule.VMID, "error", err)
				// A hole in the retention window is exactly the kind of thing
				// an audit trail is read to find, and the run row alone is
				// only visible to somebody already looking at that schedule.
				s.svc.recordAudit("plugin.kvm.snapshot_schedule.failed", map[string]any{
					"scheduleId": schedule.ID, "vmId": schedule.VMID,
					"name": schedule.Name, "error": err.Error(),
				})
				return
			}
			s.svc.recordAudit("plugin.kvm.snapshot_schedule.ran", map[string]any{
				"scheduleId": schedule.ID, "vmId": schedule.VMID,
				"name": schedule.Name,
			})
		}()
		select {
		case <-s.stop:
			return
		default:
		}
	}
}
