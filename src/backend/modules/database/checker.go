// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

// ErrCheckRunning is returned when a second run is asked for.
//
// One at a time is deliberate: integrity_check and CHECK TABLE are heavy
// single-reader operations, and two concurrent runs would contend for the
// same pages while reporting half-speed progress to two dialogs.
var ErrCheckRunning = errors.New("a database check is already running")

// checkTimeout bounds one individual check. A wedged table must not hold the
// run open forever; it becomes a fail finding and the sweep continues.
const checkTimeout = 60 * time.Second

// Checker runs the health check as a background job, one per process.
type Checker struct {
	svc    *Service
	source *sql.DB
	logger *slog.Logger

	running atomic.Bool

	mu     sync.RWMutex
	status *CheckStatus
	cancel context.CancelFunc
}

// NewChecker wires the checker to the live connection it reads from.
func NewChecker(svc *Service, source *sql.DB, logger *slog.Logger) *Checker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Checker{svc: svc, source: source, logger: logger}
}

// Status returns a copy of the current or most recent run, or nil when none
// has run in this process. A copy, because the caller serialises it on a
// request goroutine while the job keeps writing.
func (c *Checker) Status() *CheckStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.status == nil {
		return nil
	}
	snapshot := *c.status
	snapshot.Findings = append([]Finding(nil), c.status.Findings...)
	snapshot.Counts = make(map[string]int, len(c.status.Counts))
	for k, v := range c.status.Counts {
		snapshot.Counts[k] = v
	}
	return &snapshot
}

// Cancel asks a running job to stop. The sweep checks its context between
// tables, so a cancel takes effect within one table rather than instantly.
func (c *Checker) Cancel() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel == nil || c.status == nil || !c.status.Running {
		return false
	}
	c.cancel()
	return true
}

// Start launches the run and returns as soon as it is accepted; progress is
// read through Status.
func (c *Checker) Start(ctx context.Context) (*CheckStatus, error) {
	if !c.running.CompareAndSwap(false, true) {
		return nil, ErrCheckRunning
	}

	driver := string(coredb.DriverOf())
	if driver == "" {
		driver = driverSQLite
	}

	// The job outlives the request that started it; cancellation goes through Cancel.
	jobCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	status := &CheckStatus{
		ID:        xid.New().String(),
		Running:   true,
		Phase:     checkPhaseQueued,
		Driver:    driver,
		Findings:  []Finding{},
		Counts:    map[string]int{},
		StartedAt: time.Now().UTC(),
	}

	c.mu.Lock()
	c.status = status
	c.cancel = cancel
	c.mu.Unlock()

	go func() {
		defer cancel()
		defer c.running.Store(false)
		c.run(jobCtx, coredb.Driver(driver))
	}()

	return c.Status(), nil
}

// update mutates the live status under the write lock.
func (c *Checker) update(mutate func(*CheckStatus)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status != nil {
		mutate(c.status)
	}
}

// add records a finding and keeps the severity histogram in step.
//
// Finding IDs are per-run xids minted here and set by no call site, which is
// what makes Repair's len(selected) != len(wanted) guard sound.
func (c *Checker) add(f Finding) {
	if f.ID == "" {
		f.ID = xid.New().String()
	}
	c.update(func(s *CheckStatus) {
		s.Findings = append(s.Findings, f)
		s.Counts[f.Severity]++
	})
}

// setPhase advances the run and clears the per-table readout.
func (c *Checker) setPhase(phase string) {
	c.update(func(s *CheckStatus) {
		s.Phase = phase
		s.CurrentTable = ""
		s.TableIndex = 0
		s.TableTotal = 0
	})
}

// setTable drives the progress bar within a phase.
func (c *Checker) setTable(name string, index, total int) {
	c.update(func(s *CheckStatus) {
		s.CurrentTable = name
		s.TableIndex = index
		s.TableTotal = total
	})
}

// fail records a terminal failure.
func (c *Checker) fail(code i18n.MsgCode, detail string) {
	finished := time.Now().UTC()
	c.update(func(s *CheckStatus) {
		s.Running = false
		s.Phase = checkPhaseFailed
		s.Error = string(code)
		s.ErrorDetail = detail
		s.FinishedAt = &finished
	})
	c.logger.Error("database: health check failed", "code", code, "detail", detail)
}

// finish closes the run on the phase it reached.
func (c *Checker) finish(phase string) {
	finished := time.Now().UTC()
	c.update(func(s *CheckStatus) {
		s.Running = false
		s.Phase = phase
		s.CurrentTable = ""
		s.FinishedAt = &finished
	})
}

// run is the sweep. Every family is independent: one that fails records a
// fail finding and the run continues, because one unreadable table must not
// cost the operator the other results.
func (c *Checker) run(ctx context.Context, driver coredb.Driver) {
	// This goroutine is the whole sweep; an unrecovered panic in any check
	// family would take the McHarbor process down with it, so the run ends as
	// failed instead — loudly, in the report and in the log.
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("database: health check panicked", "panic", r, "stack", string(debug.Stack()))
			c.fail(i18n.ErrDatabaseCheckFailed, fmt.Sprintf("panic: %v", r))
		}
	}()

	c.setPhase(checkPhaseInventory)

	tables, err := coredb.ListTables(ctx, c.source, driver)
	if err != nil {
		c.fail(i18n.ErrDatabaseCheckFailed, sanitize(err, ""))
		return
	}
	c.add(Finding{
		Key:      "table_count",
		Family:   familyBloat,
		Severity: severityInfo,
		Count:    int64(len(tables)),
	})

	if ctx.Err() != nil {
		c.finish(checkPhaseCancelled)
		return
	}
	c.checkIntegrity(ctx, driver, tables)

	if ctx.Err() != nil {
		c.finish(checkPhaseCancelled)
		return
	}

	c.checkOrphans(ctx, driver, tables)

	if ctx.Err() != nil {
		c.finish(checkPhaseCancelled)
		return
	}

	c.checkSchema(ctx, driver)

	if ctx.Err() != nil {
		c.finish(checkPhaseCancelled)
		return
	}

	c.checkBloat(ctx, driver, tables)

	if ctx.Err() != nil {
		c.finish(checkPhaseCancelled)
		return
	}

	c.finish(checkPhaseDone)
}
