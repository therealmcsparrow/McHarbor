// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers stack module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/stacks", func(r chi.Router) {
			r.With(rbac.RequireAnyStackPermission(app.RBACService, rbac.PermStacksView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStacksManage)).Post("/", h.HandleCreate)
			r.With(rbac.RequireAnyStackPermission(app.RBACService, rbac.PermStacksView)).Post("/check-updates", h.HandleCheckImageUpdates)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStacksManage)).Post("/adopt/preview", h.HandleAdoptPreview)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStacksManage)).Post("/adopt", h.HandleAdopt)

			r.Route("/{name}", func(r chi.Router) {
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksView, "name")).Get("/", h.HandleGetDetail)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Put("/", h.HandleUpdate)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksDelete, "name")).Delete("/", h.HandleDelete)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/up", h.HandleUp)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/stop", h.HandleStop)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/down", h.HandleDown)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/restart", h.HandleRestart)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/update", h.HandleManagedUpdate)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/reinstall", h.HandleReinstall)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksView, "name")).Get("/compose", h.HandleGetCompose)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksView, "name")).Get("/logs", h.HandleGetLogs)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksView, "name")).Get("/containers", h.HandleGetContainers)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksView, "name")).Get("/env-vars", h.HandleGetEnvVars)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Put("/env-vars", h.HandleUpdateEnvVars)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/prune", h.HandlePrune)

				// Webhooks
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksView, "name")).Get("/webhooks", h.HandleListWebhooks)
				r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/webhooks", h.HandleCreateWebhook)
				r.Route("/webhooks/{id}", func(r chi.Router) {
					r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Put("/", h.HandleUpdateWebhook)
					r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Delete("/", h.HandleDeleteWebhook)
					r.With(rbac.RequireStackPermission(app.RBACService, rbac.PermStacksManage, "name")).Post("/test", h.HandleTestWebhook)
				})
			})
		})
	})
}
