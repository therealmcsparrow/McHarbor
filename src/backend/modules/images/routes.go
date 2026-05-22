// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package images

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers image module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/images", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesManage)).Post("/", h.HandlePull)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesManage)).Post("/prune", h.HandlePrune)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesManage)).Post("/import", h.HandleImport)

			r.Route("/{id}", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesView)).Get("/", h.HandleInspect)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesDelete)).Delete("/", h.HandleRemove)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesManage)).Post("/tag", h.HandleTag)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesView)).Get("/history", h.HandleHistory)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesView)).Get("/export", h.HandleExport)
			})
		})
	})
}
