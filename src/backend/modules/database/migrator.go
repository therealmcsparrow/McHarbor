// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/dbconfig"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/middleware"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
)

// maxDeltaPasses caps the catch-up loop.
//
// With the background workers held still by the quiesce gate, the first pass
// normally finds nothing and the loop ends immediately. It exists for the
// tick that was already running when the gate closed, and for any writer not
// wired to the gate — both of which are bounded, so the number of rows each
// pass finds shrinks. Three is the point at which "it is still finding rows"
// stops meaning "nearly done" and starts meaning "something is writing that
// the gate does not cover", which is worth reporting rather than looping on.
const maxDeltaPasses = 3

// drainTimeout is how long the migration waits for ticks that were already
// running when the gate closed. Every worker involved finishes a tick in far
// less than this; giving up is not fatal, because the delta passes catch
// whatever the straggler wrote.
const drainTimeout = 30 * time.Second

// ErrMigrationRunning is returned when a second migration is requested
// while one is still going.
var ErrMigrationRunning = errors.New("a database migration is already running")

// Restarter is the slice of the system module the migrator needs: the
// ability to restart the McHarbor container once the new database is in
// place. It is an interface so this module does not import modules/system,
// which would be a module-to-module dependency the router does not have.
type Restarter interface {
	RestartSelf(ctx context.Context) error
}

// Migrator runs one database migration at a time and exposes its progress.
//
// Only one job can be in flight, enforced with a compare-and-swap rather
// than a mutex, because the guarantee that matters is "a second POST is
// rejected", not "a second POST waits". Two concurrent copies into the same
// target would each clear the other's tables.
type Migrator struct {
	svc       *Service
	source    *sql.DB
	logger    *slog.Logger
	restarter Restarter

	running atomic.Bool

	mu     sync.RWMutex
	status *MigrationStatus
	cancel context.CancelFunc
}

// NewMigrator wires the migrator to the live connection it will read from.
func NewMigrator(svc *Service, source *sql.DB, logger *slog.Logger, restarter Restarter) *Migrator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Migrator{svc: svc, source: source, logger: logger, restarter: restarter}
}

// Status returns a copy of the current or most recent job, or nil when none
// has run in this process. A copy, not the pointer, because the caller
// serialises it on a request goroutine while the job keeps writing.
func (m *Migrator) Status() *MigrationStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.status == nil {
		return nil
	}
	snapshot := *m.status
	snapshot.Tables = append([]MigrationTable(nil), m.status.Tables...)
	snapshot.Steps = append([]TestStep(nil), m.status.Steps...)
	snapshot.Warnings = append([]string(nil), m.status.Warnings...)
	return &snapshot
}

// Cancel asks a running job to stop. The copy checks its context between
// batches, so a cancel takes effect within one batch rather than instantly.
// Cancelling before the persist phase leaves the running application on its
// original database, which is the whole point of persisting last.
func (m *Migrator) Cancel() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel == nil || m.status == nil || !m.status.Running {
		return false
	}
	m.cancel()
	return true
}

// Start validates the target and launches the job. It returns as soon as
// the job is accepted; progress is read through Status.
func (m *Migrator) Start(ctx context.Context, req MigrationRequest) (*MigrationStatus, error) {
	if !m.running.CompareAndSwap(false, true) {
		return nil, ErrMigrationRunning
	}

	target := req.Target
	restartAfter := true
	if req.RestartAfter != nil {
		restartAfter = *req.RestartAfter
	}

	driver := dbconfig.NormalizeDriver(target.Driver)
	redacted := ""
	if driver == driverSQLite {
		redacted = strings.TrimSpace(target.Path)
		if redacted == "" {
			redacted = m.svc.cfg.DatabasePath
		}
	} else if dsn, err := BuildDSN(target); err == nil {
		redacted = dbconfig.Redact(driver, dsn)
	}

	from := string(coredb.DriverOf())
	if from == "" {
		from = driverSQLite
	}

	// The job outlives the request that started it; cancellation goes through m.cancel.
	jobCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	status := &MigrationStatus{
		ID:         xid.New().String(),
		Running:    true,
		Phase:      phaseQueued,
		FromDriver: from,
		ToDriver:   driver,
		ToTarget:   redacted,
		StartedAt:  time.Now().UTC(),
	}

	m.mu.Lock()
	m.status = status
	m.cancel = cancel
	m.mu.Unlock()

	go func() {
		defer cancel()
		defer m.running.Store(false)
		m.run(jobCtx, target, restartAfter)
	}()

	return m.Status(), nil
}

// update mutates the live status under the write lock.
func (m *Migrator) update(mutate func(*MigrationStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status != nil {
		mutate(m.status)
	}
}

// fail records a terminal failure and lifts the maintenance pause.
func (m *Migrator) fail(code i18n.MsgCode, detail string) {
	middleware.EndMaintenance()
	finished := time.Now().UTC()
	m.update(func(s *MigrationStatus) {
		s.Running = false
		s.Phase = phaseFailed
		s.Error = string(code)
		s.ErrorDetail = detail
		s.FinishedAt = &finished
	})
	m.logger.Error("database: migration failed", "code", code, "detail", detail)
}

// run is the job. Every phase before persist is reversible by doing
// nothing: the running application stays on its source database until the
// encrypted secret is written, and a failure at any earlier point leaves
// that secret untouched.
func (m *Migrator) run(ctx context.Context, target ConnectionInput, restartAfter bool) {
	m.update(func(s *MigrationStatus) { s.Phase = phasePrepare })

	if m.svc.EnvPinned() {
		m.fail(i18n.ErrDatabaseEnvPinned, "MCHARBOR_DB_DRIVER is set in the environment")
		return
	}
	if m.svc.SameAsCurrent(target) {
		m.fail(i18n.ErrDatabaseSameTarget, "")
		return
	}

	// Pre-flight. The same seven steps the form runs, re-run against the
	// target at the moment of the migration — the credentials may have been
	// tested an hour ago, and a target that has become unreachable since
	// should stop the job before it clears a single table.
	test, conn := m.svc.Test(ctx, target)
	m.update(func(s *MigrationStatus) { s.Steps = test.Steps })
	if !test.OK || conn == nil {
		detail := ""
		code := i18n.ErrDatabaseConnectFailed
		for _, step := range test.Steps {
			if step.Status == stepFail {
				detail = step.Detail
				if step.Code != "" {
					code = i18n.MsgCode(step.Code)
				}
				break
			}
		}
		if detail == "" {
			detail = errTestFailed.Error()
		}
		m.fail(code, detail)
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			m.logger.Warn("database: closing migration target", "error", err)
		}
	}()

	targetDriver := coredb.Driver(dbconfig.NormalizeDriver(target.Driver))

	// Schema. Migrate() is idempotent and driver-aware, so a target that
	// already carries a partial McHarbor schema is brought up to date
	// rather than rejected.
	m.update(func(s *MigrationStatus) { s.Phase = phaseSchema })
	if err := coredb.MigrateContext(ctx, conn, targetDriver); err != nil {
		m.logger.Error("database: creating schema on migration target", "error", err)
		m.fail(i18n.ErrDatabaseSchemaFailed, sanitize(err, ""))
		return
	}

	m.update(func(s *MigrationStatus) { s.Phase = phaseInventory })

	// Hold the whole application still. quiesce.Begin stops both halves of
	// what writes to this database: the HTTP maintenance middleware starts
	// answering 503 to mutating requests, and every background worker skips
	// its tick. Drain then waits for the ticks that were already running when
	// the gate closed — without it the copy would begin while a collector was
	// halfway through a batch of inserts.
	//
	// A drain that times out is not a failure. It means one worker is slow,
	// and the delta passes after the copy exist precisely to cover what it
	// wrote. Resuming is deferred so a panic or an early return cannot leave
	// the application permanently read-only.
	middleware.BeginMaintenance("database-migration")
	defer middleware.EndMaintenance()

	drainCtx, cancelDrain := context.WithTimeout(ctx, drainTimeout)
	if err := quiesce.Drain(drainCtx); err != nil {
		m.logger.Warn("database: background workers still running at the start of the copy",
			"inFlight", quiesce.InFlight(), "error", err)
	}
	cancelDrain()

	m.update(func(s *MigrationStatus) { s.Phase = phaseCopy })

	sourceDriver := coredb.DriverOf()
	if sourceDriver == "" {
		sourceDriver = coredb.DriverSQLite
	}

	// Batch sizes, foreign-key suspension and identity inserts are chosen per
	// driver inside the copier, for SQL Server as for the others.
	report, err := coredb.CopyAll(ctx, m.source, sourceDriver, conn, targetDriver, coredb.CopyOptions{
		Logger:     m.logger,
		OnProgress: m.onProgress,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			m.markCancelled()
			return
		}
		m.logger.Error("database: copying rows to migration target", "error", err)
		m.fail(i18n.ErrDatabaseMigrationFailed, sanitize(err, ""))
		return
	}

	m.update(func(s *MigrationStatus) {
		s.Phase = phaseVerify
		s.Warnings = warningsFrom(report)
		s.RowsCopied = report.TotalRows
		for i := range s.Tables {
			s.Tables[i].Done = true
		}
	})

	// Catch-up passes. Anything written to the source after its table was
	// read — a tick already in flight when the gate closed, a writer not
	// wired to the gate — is copied here, keyed on primary key so a row that
	// already made the trip is never written twice.
	caught, deltaWarnings, err := m.runDeltaPasses(ctx, sourceDriver, conn, targetDriver)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			m.markCancelled()
			return
		}
		m.logger.Error("database: catching up rows written during the copy", "error", err)
		m.fail(i18n.ErrDatabaseMigrationFailed, sanitize(err, ""))
		return
	}
	m.update(func(s *MigrationStatus) {
		s.RowsCopied += caught
		s.RowsCaughtUp = caught
		s.Warnings = append(s.Warnings, deltaWarnings...)
	})

	// Persist. The first and only step that changes what McHarbor boots
	// against.
	m.update(func(s *MigrationStatus) { s.Phase = phasePersist })
	if err := m.svc.Persist(target); err != nil {
		m.logger.Error("database: writing the connection secret", "error", err)
		m.fail(i18n.ErrDatabaseMigrationFailed, sanitize(err, ""))
		return
	}

	// Writers resume only now: the secret is written, so from this point the
	// old database is no longer the one that matters.
	middleware.EndMaintenance()

	m.update(func(s *MigrationStatus) { s.Phase = phaseRestart })
	scheduled, required := m.scheduleRestart(ctx, restartAfter)

	finished := time.Now().UTC()
	m.update(func(s *MigrationStatus) {
		s.Running = false
		s.Phase = phaseDone
		s.RestartScheduled = scheduled
		s.RestartRequired = required
		s.FinishedAt = &finished
	})
	m.logger.Info("database: migration complete",
		"to", targetDriver, "rows", report.TotalRows, "tables", len(report.Tables),
		"durationMs", report.DurationMS, "restartScheduled", scheduled)
}

// scheduleRestart asks the container to restart into the new database, and
// says whether the operator has to do it themselves instead.
func (m *Migrator) scheduleRestart(ctx context.Context, wanted bool) (scheduled, required bool) {
	if !wanted {
		return false, true
	}
	if m.restarter == nil {
		return false, true
	}
	if err := m.restarter.RestartSelf(ctx); err != nil {
		m.logger.Warn("database: could not restart automatically after migration", "error", err)
		return false, true
	}
	return true, false
}

// onProgress folds a copier tick into the job status. It runs on the copy
// goroutine and must stay cheap; the per-table slice is appended to rather
// than rebuilt.
func (m *Migrator) onProgress(p coredb.CopyProgress) {
	m.update(func(s *MigrationStatus) {
		s.CurrentTable = p.Table
		s.TableIndex = p.TableIndex
		s.TableTotal = p.TableTotal
		s.RowsCopied = p.TotalRowsCopied
		s.RowsTotal = p.TotalRows

		for i := range s.Tables {
			if s.Tables[i].Table == p.Table {
				s.Tables[i].Rows = p.RowsCopied
				s.Tables[i].Total = p.RowsTotal
				return
			}
		}
		// A new table started: mark the previous one finished, since the
		// copier moves strictly forward through its ordered list.
		if n := len(s.Tables); n > 0 {
			s.Tables[n-1].Done = true
		}
		s.Tables = append(s.Tables, MigrationTable{
			Table: p.Table,
			Rows:  p.RowsCopied,
			Total: p.RowsTotal,
		})
	})
}

// warningsFrom renders a copy report's non-fatal findings as the short
// strings the progress modal lists.
func warningsFrom(report coredb.Report) []string {
	var out []string
	if len(report.Drifted) > 0 {
		out = append(out, fmt.Sprintf("rows were written to %d table(s) during the copy and did not make the trip: %s",
			len(report.Drifted), strings.Join(report.Drifted, ", ")))
	}
	if len(report.SkippedTables) > 0 {
		out = append(out, fmt.Sprintf("tables missing on the target were skipped: %s",
			strings.Join(report.SkippedTables, ", ")))
	}
	if !report.ForeignKeysRelaxed {
		out = append(out, "the target would not suspend foreign key checks; the copy relied on table order alone")
	}
	if w := untrustedWarning(report.UntrustedTables); w != "" {
		out = append(out, w)
	}
	for _, table := range report.Tables {
		if len(table.SkippedColumns) > 0 {
			out = append(out, fmt.Sprintf("%s: columns present on one side only were skipped: %s",
				table.Table, strings.Join(table.SkippedColumns, ", ")))
		}
	}
	return out
}

// untrustedWarning names the tables whose copied rows reference rows that do not
// exist, which SQL Server re-validates on restore and SQLite never checked.
func untrustedWarning(tables []string) string {
	if len(tables) == 0 {
		return ""
	}
	return "rows copied into these tables reference rows that do not exist, so their foreign keys " +
		"apply to new writes only: " + strings.Join(tables, ", ")
}

// RestarterFunc adapts a plain function to Restarter, so main.go can hand
// over modules/system's restart without this module importing it.
type RestarterFunc func(context.Context) error

// RestartSelf implements Restarter.
func (f RestarterFunc) RestartSelf(ctx context.Context) error { return f(ctx) }

// markCancelled records a job the operator stopped. Nothing has to be undone:
// the connection secret is written last, so a cancel before that leaves the
// application on the database it started on.
func (m *Migrator) markCancelled() {
	finished := time.Now().UTC()
	m.update(func(s *MigrationStatus) {
		s.Running = false
		s.Phase = phaseCancelled
		s.FinishedAt = &finished
	})
	m.logger.Info("database: migration cancelled")
}

// runDeltaPasses repeats a catch-up copy until a pass finds nothing, and
// returns how many rows it moved plus any warnings worth surfacing.
//
// It loops rather than running once because each pass takes time of its own,
// and anything written during pass N is only visible to pass N+1. With the
// quiesce gate up that is almost always zero on the first pass; the loop is
// what makes "almost always" into a bounded, reported outcome instead of an
// assumption.
func (m *Migrator) runDeltaPasses(ctx context.Context, sourceDriver coredb.Driver, target *sql.DB, targetDriver coredb.Driver) (int64, []string, error) {
	var (
		total    int64
		warnings []string
	)

	for pass := 1; pass <= maxDeltaPasses; pass++ {
		if err := ctx.Err(); err != nil {
			return total, warnings, err
		}
		m.update(func(s *MigrationStatus) {
			s.Phase = phaseCatchUp
			s.DeltaPass = pass
		})

		report, err := coredb.CopyDelta(ctx, m.source, sourceDriver, target, targetDriver, coredb.DeltaOptions{
			Logger: m.logger,
		})
		if err != nil {
			return total, warnings, err
		}
		total += report.Total
		if w := untrustedWarning(report.UntrustedTables); w != "" {
			warnings = append(warnings, w)
		}

		if report.Total == 0 {
			m.logger.Info("database: catch-up pass found nothing left to copy", "pass", pass)
			return total, warnings, nil
		}

		m.logger.Info("database: catch-up pass copied late rows",
			"pass", pass, "rows", report.Total, "tables", len(report.Copied))

		if pass == maxDeltaPasses {
			warnings = append(warnings, fmt.Sprintf(
				"rows were still arriving after %d catch-up passes (%d copied in the last one); "+
					"something is writing to the old database outside the pause",
				maxDeltaPasses, report.Total))
		}
	}
	return total, warnings, nil
}
