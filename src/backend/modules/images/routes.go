// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package images

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers image module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/images", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesView)).Get("/", h.HandleList, apispec.Scoped(), apispec.Query("all", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesManage)).Post("/", h.HandlePull, apispec.Scoped(), apispec.Request(PullRequest{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesManage)).Post("/prune", h.HandlePrune, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesManage)).Post("/import", h.HandleImport, apispec.Scoped())

			r.Route("/{id}", func(r *apispec.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesView)).Get("/", h.HandleInspect, apispec.Scoped(),
					apispec.Query("reveal", "Return credential-looking Config.Env values in the clear; requires images.manage and is audit-logged."))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesDelete)).Delete("/", h.HandleRemove, apispec.Scoped(),
					apispec.Query("force", "Remove the image even when containers use it."),
					apispec.Query("noprune", "Keep untagged parent images."))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesManage)).Post("/tag", h.HandleTag, apispec.Scoped(), apispec.Request(TagRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesView)).Get("/history", h.HandleHistory, apispec.Scoped(),
					apispec.Query("reveal", "Return credential-looking ENV and build-argument values in the clear; requires images.manage and is audit-logged."))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermImagesManage)).Get("/export", h.HandleExport, apispec.Scoped())
			})
		})
	})
}
