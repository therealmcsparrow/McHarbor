// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package git

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers git repository module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/git", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitView)).Get("/", h.HandleListRepos, apispec.Paginated())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitManage)).Post("/", h.HandleCreateRepo, apispec.Request(CreateRepoInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitView)).Get("/{id}", h.HandleGetRepo)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitManage)).Put("/{id}", h.HandleUpdateRepo, apispec.Request(UpdateRepoInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitManage)).Delete("/{id}", h.HandleDeleteRepo)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitManage)).Post("/{id}/sync", h.HandleSync)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermGitView)).Get("/{id}/deployments", h.HandleListDeployments, apispec.Paginated())
		})
	})
}
