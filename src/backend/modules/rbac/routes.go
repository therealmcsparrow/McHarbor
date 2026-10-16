// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers the /api/rbac/policies admin endpoints. Both view
// and manage routes are gated so that read-only admins can list
// policies without being able to enable or disable them.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/rbac/policies", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRbacPoliciesView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRbacPoliciesManage)).Post("/", h.HandleCreate, apispec.Request(PolicyInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRbacPoliciesView)).Get("/{id}", h.HandleGet)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRbacPoliciesManage)).Put("/{id}", h.HandleUpdate, apispec.Request(PolicyInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRbacPoliciesManage)).Delete("/{id}", h.HandleDelete)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRbacPoliciesView)).Get("/{id}/history", h.HandleHistory)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRbacPoliciesManage)).Post("/{id}/enable", h.HandleToggleEnable, apispec.Request(ToggleEnableRequest{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRbacPoliciesManage)).Post("/{id}/test", h.HandleTest, apispec.Request(TestInput{}))
		})

		r.With(rbac.RequirePermission(app.RBACService, rbac.PermRbacPoliciesView)).Get("/rbac/policy-reference", h.HandleReference, apispec.Response(PolicyReference{}))

		r.Route("/rbac/users", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRbacPoliciesManage)).Get("/{userID}/labels", h.HandleUserLabelsGet)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermRbacPoliciesManage)).Put("/{userID}/labels", h.HandleUserLabelsPut, apispec.Request(UserLabelsInput{}))
		})
	})
}
