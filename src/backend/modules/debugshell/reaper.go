// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package debugshell

import (
	"context"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// ReapInterval is the background reaper's cadence (trigger three); with the 15
// minute TTL a leftover busybox outlives its session by at most a minute.
var ReapInterval = time.Minute

var (
	sharedMu sync.Mutex
	shared   *Service
)

// Shared returns the process-wide Service. The terminal and the container routes
// must see one registry of attached shells, or expiry could not close a socket.
func Shared(app *router.AppDeps) *Service {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if shared == nil {
		var auditor Auditor
		if app.AuditLog != nil {
			auditor = app.AuditLog
		}
		shared = NewService(app.DB, app.DockerPool, auditor, app.Logger)
	}
	return shared
}

// Start runs the startup sweep (trigger four) and then the minute reaper (trigger
// three) on one goroutine that Stop ends; the sweep is off the boot path because
// an agent environment can take seconds to answer.
func (s *Service) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.sweep(ctx, TriggerStartup)
		ticker := time.NewTicker(ReapInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sweep(ctx, TriggerReaper)
			}
		}
	}()
}

// Stop ends the reaper and waits for an in-flight sweep to finish.
func (s *Service) Stop() {
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
	})
	s.wg.Wait()
}

func (s *Service) sweep(ctx context.Context, trigger Trigger) {
	removed, err := s.Reap(ctx, trigger)
	if err != nil {
		s.logger.Error("debug shell: sweep left sessions open", "trigger", trigger, "removed", removed, "error", err)
		return
	}
	if removed > 0 {
		s.logger.Info("debug shell: sweep removed expired sessions", "trigger", trigger, "removed", removed)
	}
}
