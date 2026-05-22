// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers settings module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/settings", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Put("/", h.HandleBulkUpdate)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsView)).Get("/agent", h.HandleGetAgentSettings)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Put("/agent", h.HandleUpdateAgentSettings)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsView)).Get("/scanners", h.HandleGetScannerSettings)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Put("/scanners", h.HandleUpdateScannerSettings)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsView)).Get("/retention", h.HandleGetRetentionSettings)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Put("/retention", h.HandleUpdateRetentionSettings)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsView)).Get("/tls", h.HandleGetTLS)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Put("/tls", h.HandleUpdateTLS)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsView)).Get("/{key}", h.HandleGetByKey)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Put("/{key}", h.HandleSetByKey)
		})
	})
}
