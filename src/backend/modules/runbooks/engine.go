// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/quiesce"
)

// EngineRole is the singleton role the automation engine is elected under, so a
// cluster starts each scheduled or alert-triggered run exactly once.
const EngineRole = "runbook-automation"

// engineInterval is how often the engine looks for due schedules and new
// alerts. Schedules have minute resolution; half a minute keeps alert response
// prompt without a second loop.
const engineInterval = 30 * time.Second

// Coordinator elects the one node that runs unattended runbooks.
type Coordinator interface {
	LeaderOf(ctx context.Context, name string) bool
}

// Engine runs automated runbooks on their schedule and on their alert, as their
// service principal. It holds no state of its own: next_run_at and the runs
// table are the whole record, so a restart or a leader change loses nothing.
type Engine struct {
	Service *Service
	Coord   Coordinator
	Logger  *slog.Logger

	done     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewEngine returns an engine over svc, elected through coord when there is one.
func NewEngine(svc *Service, coord Coordinator) *Engine {
	return &Engine{Service: svc, Coord: coord, Logger: svc.Logger}
}

// Start runs the engine until Stop or ctx ends.
func (e *Engine) Start(ctx context.Context) {
	e.done = make(chan struct{})
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(engineInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-e.done:
				return
			case <-ticker.C:
				quiesce.Run(func() { e.Tick(ctx) })
			}
		}
	}()
}

// Stop ends the engine and waits for a tick in flight, so no run is cut off
// half-recorded by shutdown.
func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		if e.done != nil {
			close(e.done)
		}
	})
	e.wg.Wait()
}

func (e *Engine) log() *slog.Logger {
	if e.Logger != nil {
		return e.Logger
	}
	return slog.Default()
}

// Tick runs one pass: due schedules first, then new alerts. It is exported so
// tests drive the engine without timers.
func (e *Engine) Tick(ctx context.Context) {
	if e.Coord != nil && !e.Coord.LeaderOf(ctx, EngineRole) {
		return
	}
	e.runSchedules(ctx)
	e.runAlerts(ctx)
}

func (e *Engine) runSchedules(ctx context.Context) {
	svc := e.Service
	now := svc.clock().UTC().Format(time.RFC3339)
	due, err := svc.Store.DueScheduled(ctx, now)
	if err != nil {
		e.log().Error("runbooks: reading due schedules failed", "error", err)
		return
	}
	for i := range due {
		if ctx.Err() != nil {
			return
		}
		rb := &due[i]
		if err := svc.Store.AdvanceSchedule(ctx, rb.ID, svc.nextRun(rb.Automation.Cron)); err != nil {
			e.log().Error("runbooks: advancing a schedule failed", "error", err, "runbook", rb.ID)
			continue
		}
		e.run(ctx, rb, TriggerSchedule, nil)
	}
}

func (e *Engine) runAlerts(ctx context.Context) {
	svc := e.Service
	books, err := svc.Store.AlertAutomated(ctx)
	if err != nil {
		e.log().Error("runbooks: reading alert-triggered runbooks failed", "error", err)
		return
	}
	floor := svc.clock().Add(-alertLookback).UTC().Format(time.RFC3339)
	for i := range books {
		rb := &books[i]
		since := rb.Automation.Since
		if since < floor {
			since = floor
		}
		alerts, err := svc.Store.FiringAlertsFor(ctx, *rb, since)
		if err != nil {
			e.log().Error("runbooks: reading alerts for a runbook failed", "error", err, "runbook", rb.ID)
			continue
		}
		for j := range alerts {
			if ctx.Err() != nil {
				return
			}
			e.run(ctx, rb, TriggerAlert, &alerts[j])
		}
	}
}

// run executes one unattended run and records its outcome on the runbook, so the
// editor can show why the last one stopped without opening the run.
func (e *Engine) run(ctx context.Context, rb *Runbook, trigger string, alert *firingAlert) {
	svc := e.Service
	lastError := ""
	run, err := svc.RunUnattended(ctx, rb, trigger, alert)
	switch {
	case errors.Is(err, ErrPrincipalInvalid):
		lastError = "the service principal is missing or disabled; nothing ran"
	case err != nil:
		lastError = "the run could not be started; see the server log"
		e.log().Error("runbooks: unattended run failed", "error", err, "runbook", rb.ID, "trigger", trigger)
	case run.Outcome == OutcomeAbandoned:
		lastError = run.Note
	}
	if err := svc.Store.RecordAutomationOutcome(ctx, rb.ID, lastError); err != nil {
		e.log().Error("runbooks: recording an automation outcome failed", "error", err, "runbook", rb.ID)
	}
}
