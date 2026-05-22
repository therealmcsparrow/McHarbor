// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers app store routes and seeds the catalog on startup.
func Mount(app *router.AppDeps, service *Service) {
	h := NewHandler(app, service)

	// Seed bundled catalog into DB
	if err := h.service.SeedBundledCatalog(); err != nil {
		app.Logger.Error("failed to seed app store catalog", "error", err)
	}

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/app-store", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsView)).Get("/categories", h.HandleCategories)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsView)).Get("/installed", h.HandleInstalled)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsView)).Get("/sync/status", h.HandleSyncStatus)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsManage)).Post("/install", h.HandleInstall)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsManage)).Post("/install/stream", h.HandleInstallStream)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsManage)).Post("/sync", h.HandleSync)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsView)).Get("/{slug}", h.HandleGet)
		})
	})
}
