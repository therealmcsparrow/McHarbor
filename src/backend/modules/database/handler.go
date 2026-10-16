// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/dbconfig"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler serves the database configuration surface: the protected
// settings routes and the three first-run routes that have to work before
// any account exists.
type Handler struct {
	app      *router.AppDeps
	svc      *Service
	migrator *Migrator
	checker  *Checker
}

// NewHandler builds the handler and the module's three singletons. The
// migrator and checker are per-process state (one job at a time each), so
// they are created here and shared by every request rather than per call.
func NewHandler(app *router.AppDeps, restarter Restarter) *Handler {
	svc := NewService(app.Config, app.Encryption, app.DB, app.Logger)
	return &Handler{
		app:      app,
		svc:      svc,
		migrator: NewMigrator(svc, app.DB, app.Logger, restarter),
		checker:  NewChecker(svc, app.DB, app.Logger),
	}
}

// HandleGetConfig returns the active database, its origin, and a size
// readout. Gated by database.view.
func (h *Handler) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	cfg, err := h.svc.Current(r.Context())
	if err != nil {
		h.app.Logger.Error("database: reading the active configuration", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrDatabaseConnectFailed, err)
		return
	}
	cfg.RestartSupported = h.migrator.restarter != nil
	response.OK(w, cfg)
}

// HandleTest runs the step-by-step connection test against a candidate
// database. Gated by database.manage: the response carries sanitized driver
// diagnostics, and running it dials an arbitrary host from inside the
// McHarbor container.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var in ConnectionInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	result, conn := h.svc.Test(r.Context(), in)
	h.closeProbe(conn)

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "database.test",
		EntityType: "database",
		EntityName: result.Driver,
		Details:    "connection test against " + result.Target + " (" + testOutcome(result) + ")",
	})

	response.OK(w, result)
}

// HandleStartMigration launches a data migration onto another backend.
func (h *Handler) HandleStartMigration(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	if h.svc.EnvPinned() {
		response.ConflictCode(w, r, i18n.ErrDatabaseEnvPinned)
		return
	}

	var req MigrationRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if _, err := BuildDSN(req.Target); err != nil {
		response.BadRequestCause(w, r, i18n.ErrDatabaseConfigInvalid, err)
		return
	}
	if h.svc.SameAsCurrent(req.Target) {
		response.BadRequestCode(w, r, i18n.ErrDatabaseSameTarget)
		return
	}

	status, err := h.migrator.Start(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrMigrationRunning) {
			response.ConflictCode(w, r, i18n.ErrDatabaseMigrationRunning)
			return
		}
		h.app.Logger.Error("database: starting the migration", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrDatabaseMigrationFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "database.migrate.start",
		Severity:   "warning",
		EntityType: "database",
		EntityID:   status.ID,
		EntityName: status.ToDriver,
		Details:    "migrating from " + status.FromDriver + " to " + status.ToDriver + " at " + status.ToTarget,
	})

	response.OK(w, status)
}

// HandleMigrationStatus reports the current or most recent job. This is the
// endpoint the progress modal polls, which is why it stays reachable while
// the maintenance pause is up.
func (h *Handler) HandleMigrationStatus(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	status := h.migrator.Status()
	if status == nil {
		response.NotFoundCode(w, r, i18n.ErrDatabaseMigrationNotFound)
		return
	}
	response.OK(w, status)
}

// HandleCancelMigration asks a running job to stop.
func (h *Handler) HandleCancelMigration(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	if !h.migrator.Cancel() {
		response.NotFoundCode(w, r, i18n.ErrDatabaseMigrationNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "database.migrate.cancel",
		Severity:   "warning",
		EntityType: "database",
		Details:    "database migration cancelled by an administrator",
	})

	response.OKMsg(w, r, i18n.MsgDatabaseMigrationStarted)
}

// HandleCleanupStatus reports whether an old SQLite file is sitting on disk
// and may be deleted.
func (h *Handler) HandleCleanupStatus(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	response.OK(w, h.svc.Cleanup())
}

// HandleCleanup deletes the SQLite file a completed migration left behind.
//
// Irreversible, and gated by database.manage. The service re-checks its own
// guard rather than trusting the status the browser was holding: refusing
// only in the UI would mean a stale page could ask McHarbor to delete the
// database it is currently running on.
func (h *Handler) HandleCleanup(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	status := h.svc.Cleanup()
	if !status.Available {
		response.ConflictCode(w, r, i18n.ErrDatabaseCleanupUnavailable)
		return
	}

	result, err := h.svc.RemovePreviousSQLite()
	if err != nil {
		h.app.Logger.Error("database: removing the previous SQLite database", "error", err, "path", status.Path)
		response.InternalErrorCause(w, r, i18n.ErrDatabaseCleanupFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "database.cleanup",
		Severity:   "warning",
		EntityType: "database",
		EntityName: driverSQLite,
		EntityID:   status.Path,
		Details:    "deleted the previous SQLite database and its sidecar files: " + strings.Join(result.Removed, ", "),
	})

	response.OK(w, result)
}

// HandleStartCheck launches a health check run. Gated by database.manage
// rather than view: a full sweep across every table is real load on the
// database every other request shares.
func (h *Handler) HandleStartCheck(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	status, err := h.checker.Start(r.Context())
	if err != nil {
		if errors.Is(err, ErrCheckRunning) {
			response.ConflictCode(w, r, i18n.ErrDatabaseCheckRunning)
			return
		}
		h.app.Logger.Error("database: starting the health check", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrDatabaseCheckFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "database.check.start",
		Severity:   "info",
		EntityType: "database",
		EntityID:   status.ID,
		EntityName: status.Driver,
		Details:    "database health check started",
	})

	response.OK(w, status)
}

// HandleCheckStatus reports the current or most recent run. This is what the
// progress dialog polls.
func (h *Handler) HandleCheckStatus(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	status := h.checker.Status()
	if status == nil {
		response.NotFoundCode(w, r, i18n.ErrDatabaseCheckNotFound)
		return
	}
	response.OK(w, status)
}

// HandleCancelCheck asks a running check to stop.
func (h *Handler) HandleCancelCheck(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	if !h.checker.Cancel() {
		response.NotFoundCode(w, r, i18n.ErrDatabaseCheckNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "database.check.cancel",
		Severity:   "info",
		EntityType: "database",
		Details:    "database health check cancelled",
	})

	response.OKMsg(w, r, i18n.MsgDatabaseCheckCancelled)
}

// HandleRepair applies the remedies the operator selected.
func (h *Handler) HandleRepair(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var req RepairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if len(req.FindingIDs) == 0 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	result, err := h.checker.Repair(r.Context(), req.FindingIDs, h.app.Compact)
	if err != nil {
		if errors.Is(err, ErrCheckRunning) {
			response.ConflictCode(w, r, i18n.ErrDatabaseCheckRunning)
			return
		}
		h.app.Logger.Error("database: applying repairs", "error", err)
		response.BadRequestCause(w, r, i18n.ErrDatabaseRepairUnknown, err)
		return
	}

	for _, outcome := range result.Outcomes {
		h.app.AuditLog.Log(r, audit.Entry{
			Action:     "database.check.repair",
			Severity:   "warning",
			EntityType: "database",
			EntityID:   outcome.FindingID,
			EntityName: outcome.Remedy,
			Details: fmt.Sprintf("remedy %s: ok=%t rows=%d skipped=%t",
				outcome.Remedy, outcome.OK, outcome.Rows, outcome.Skipped),
		})
	}

	response.OK(w, result)
}

// --- First-run routes ---------------------------------------------------
//
// These three run before any account exists, so they cannot be behind a
// permission. They are guarded instead by the same condition POST
// /auth/setup uses: the moment a user exists, they stop answering. They are
// mounted in the rate-limited auth group for the same reason.

// HandleSetupStatus tells the first-run wizard whether it should show the
// database step at all.
func (h *Handler) HandleSetupStatus(w http.ResponseWriter, r *http.Request) {
	response.OK(w, SetupStatusResponse{
		NeedsSetup:       !h.app.AuthService.HasAnyUserContext(r.Context()),
		Configured:       h.svc.Configured(),
		EnvPinned:        h.svc.EnvPinned(),
		Driver:           currentDriverName(),
		AvailableDrivers: AvailableDrivers(),
		RestartSupported: h.migrator.restarter != nil,
	})
}

// HandleSetupTest is HandleTest without the permission, for the installer
// who has no account yet.
func (h *Handler) HandleSetupTest(w http.ResponseWriter, r *http.Request) {
	if !h.firstRunOpen(w, r) {
		return
	}

	var in ConnectionInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if !h.sqlitePathConfined(w, r, in) {
		return
	}

	result, conn := h.svc.Test(r.Context(), in)
	h.closeProbe(conn)
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "database.setup.test",
		EntityType: "database",
		EntityName: dbconfig.NormalizeDriver(in.Driver),
		Details:    fmt.Sprintf("first-run connection test ok=%t", result.OK),
	})
	response.OK(w, result)
}

// HandleSetupApply records the operator's choice on a fresh install.
//
// There is nothing to copy — the wizard runs before the first account and
// before any data is gathered, which is the entire reason the database
// question is asked at this point rather than after. So the work is: write
// the encrypted secret, then restart into it. Choosing SQLite writes the
// secret too, because "the operator has chosen" is a fact that cannot live
// in a database a later migration would carry along.
func (h *Handler) HandleSetupApply(w http.ResponseWriter, r *http.Request) {
	h.app.FirstRun.Lock()
	defer h.app.FirstRun.Unlock()
	if !h.firstRunOpen(w, r) {
		return
	}
	if h.svc.EnvPinned() {
		response.ConflictCode(w, r, i18n.ErrDatabaseEnvPinned)
		return
	}

	var in ConnectionInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if !h.sqlitePathConfined(w, r, in) {
		return
	}

	driver := dbconfig.NormalizeDriver(in.Driver)
	if driver != driverSQLite {
		result, conn := h.svc.Test(r.Context(), in)
		h.closeProbe(conn)
		if !result.OK {
			response.BadRequestCodeWithData(w, r, i18n.ErrDatabaseConnectFailed, result)
			return
		}
	}

	// The connection test above can take seconds against a remote server; an
	// account created meanwhile must not be followed by a restart into an empty
	// database where setup is open again.
	if h.app.AuthService.HasAnyUserContext(r.Context()) {
		response.ConflictCode(w, r, i18n.ErrDatabaseSetupCompleted)
		return
	}
	if err := h.svc.Persist(in); err != nil {
		h.app.Logger.Error("database: writing the first-run connection secret", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrDatabaseConfigInvalid, err)
		return
	}

	out := SetupApplyResponse{Driver: driver}
	if driver == driverSQLite {
		out.Target = in.Path
		if out.Target == "" {
			out.Target = h.app.Config.DatabasePath
		}
	} else if dsn, err := BuildDSN(in); err == nil {
		out.Target = dbconfig.Redact(driver, dsn)
	}

	// SQLite chosen on a SQLite install changes nothing about the running
	// process, so there is no reason to bounce the container and drop the
	// operator's page mid-wizard.
	if driver == currentDriverName() {
		out.RestartScheduled = false
		out.RestartRequired = false
	} else {
		out.RestartScheduled, out.RestartRequired = h.migrator.scheduleRestart(r.Context(), true)
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "database.setup.apply",
		Severity:   "warning",
		EntityType: "database",
		EntityName: driver,
		Details:    "first-run database selected: " + driver + " at " + out.Target,
	})

	response.OK(w, out)
}

// firstRunOpen enforces the one guard the unauthenticated routes have: they
// answer only while no account exists.
func (h *Handler) firstRunOpen(w http.ResponseWriter, r *http.Request) bool {
	if h.app.AuthService.HasAnyUserContext(r.Context()) {
		response.ConflictCode(w, r, i18n.ErrDatabaseSetupCompleted)
		return false
	}
	if !h.app.FirstRun.Valid(r) {
		response.ErrCodeWithData(w, r, http.StatusForbidden, i18n.ErrForbidden, map[string]string{"reason": "setup_token"})
		return false
	}
	return true
}

// sqlitePathConfined refuses a SQLite path outside the data directory: before the
// first account these routes are reachable by anyone holding the setup token, and
// a test or an apply creates the file it names.
func (h *Handler) sqlitePathConfined(w http.ResponseWriter, r *http.Request, in ConnectionInput) bool {
	if dbconfig.NormalizeDriver(in.Driver) != driverSQLite || strings.TrimSpace(in.Path) == "" {
		return true
	}
	if pathWithin(h.app.Config.DataDir, in.Path) {
		return true
	}
	response.BadRequestCodeWithData(w, r, i18n.ErrDatabaseConfigInvalid, map[string]string{"reason": "path_outside_data_dir"})
	return false
}

// pathWithin reports whether path resolves inside dir.
func pathWithin(dir, path string) bool {
	base, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(base, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// closeProbe releases the connection a passing test leaves open. The
// migrator keeps its own; a test does not.
func (h *Handler) closeProbe(conn *sql.DB) {
	if conn == nil {
		return
	}
	if err := conn.Close(); err != nil {
		h.app.Logger.Warn("database: closing the tested connection", "error", err)
	}
}

// testOutcome renders a one-word result for the audit line.
func testOutcome(result TestResponse) string {
	if result.OK {
		return "passed"
	}
	return "failed"
}

// currentDriverName is the driver this process booted against, defaulting
// to sqlite before core/db.Open has run.
func currentDriverName() string {
	if driver := coredb.DriverOf(); driver != "" {
		return string(driver)
	}
	return driverSQLite
}
