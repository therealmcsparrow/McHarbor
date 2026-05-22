// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// Mount registers vulnerability scan module routes.
func Mount(app *router.AppDeps) {
	scannerSettings := coreSettings.ReadScannerSettings(app.DB)
	reg := NewScannerRegistry(scannerSettings.ClairURL)
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
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/{id}/vulnerabilities", h.HandleGetVulnerabilities)
		})
	})
}
