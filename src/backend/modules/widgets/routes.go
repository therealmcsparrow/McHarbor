// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package widgets

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers widget definition routes and seeds built-in widgets.
func Mount(app *router.AppDeps) {
	svc := NewService(app.Config.DataDir, app.Logger)
	if err := svc.Seed(); err != nil {
		app.Logger.Error("widgets: failed to seed built-in widgets", "error", err)
	}

	h := NewHandler(app, svc)
	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/widgets", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreWidgetsView)).Get("/definitions", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreWidgetsManage)).Post("/", h.HandleInstall)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreWidgetsManage)).Put("/{key}", h.HandleUpdate)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreWidgetsManage)).Delete("/{key}", h.HandleUninstall)
		})
	})
}
