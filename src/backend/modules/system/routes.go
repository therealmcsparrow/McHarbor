// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers system module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/system", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermTerminalAccess)).Get("/os-terminal/ws", h.HandleOSTerminalWS)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermLogsView)).Get("/os-logs", h.HandleOSLogs)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsView)).Get("/os-updates/check", h.HandleOSUpdateCheck)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Post("/os-updates/apply", h.HandleOSUpdateApply)
		})
	})
}
