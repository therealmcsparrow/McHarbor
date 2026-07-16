// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// Mount registers vulnerability scan module routes.
//
// The first read of scanner settings happens asynchronously after the
// HTTP server is up so a slow or degraded SQLite database cannot stall
// the startup sequence (the deadlock that previously locked the entire
// process during scans.Mount when the database was unable to serve
// concurrent reads).
func Mount(app *router.AppDeps) {
	reg := NewScannerRegistry("")
	svc := NewService(app.DB, reg, app.Logger)
	h := &Handler{app: app, service: svc}

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/scans", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansManage)).Post("/", h.HandleStartScan)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/summary", h.HandleSummary)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/scanners", h.HandleAvailableScanners)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/by-image", h.HandleScanByImage)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/{id}", h.HandleGetScan)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansManage)).Delete("/{id}", h.HandleDelete)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Delete("/", h.HandlePurge)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/{id}/vulnerabilities", h.HandleGetVulnerabilities)
		})
	})

	// Load the actual scanner settings after the HTTP server has had a
	// moment to bind. If the read fails (e.g. DB lock contention) we
	// retry with backoff until it succeeds or the process is shut down.
	go refreshScannerRegistry(app.DB, reg, app.Logger)
}

func refreshScannerRegistry(db *sql.DB, reg *ScannerRegistry, logger *slog.Logger) {
	delay := 2 * time.Second
	for attempt := 1; attempt <= 12; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		settings, err := readScannerSettingsBounded(ctx, db)
		cancel()
		if err == nil {
			reg.Reload(settings.ClairURL)
			logger.Info("scanner settings loaded", "attempt", attempt, "clair_configured", settings.ClairURL != "")
			return
		}
		logger.Warn("scanner settings load failed; will retry", "attempt", attempt, "error", err)
		time.Sleep(delay)
		if delay < 30*time.Second {
			delay *= 2
		}
	}
	logger.Warn("scanner settings load gave up after retries; scanner registry running with defaults")
}

// readScannerSettingsBounded runs the existing settings reader inside a
// goroutine bounded by the supplied context. The underlying
// ReadScannerSettings uses database/sql without a context, so we wrap
// it here to keep the startup path responsive.
func readScannerSettingsBounded(ctx context.Context, db *sql.DB) (coreSettings.ScannerSettings, error) {
	type result struct {
		settings coreSettings.ScannerSettings
		err      error
	}
	ch := make(chan result, 1)
	go func() {
		ch <- result{settings: coreSettings.ReadScannerSettings(db)}
	}()
	select {
	case <-ctx.Done():
		return coreSettings.ScannerSettings{}, ctx.Err()
	case r := <-ch:
		return r.settings, nil
	}
}
