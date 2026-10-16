// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package approvals

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers the approvals API and installs the gate on the protected
// router. Rules are empty on a fresh install, so the gate changes nothing until
// an admin adds one.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	app.ApprovalGate = NewGate(h.svc, app.RBACService, app.AuditLog, app.Logger, h.notifyApprovers).Middleware

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/approvals", func(r *apispec.Router) {
			read := rbac.RequireAnyGlobalPermission(app.RBACService, rbac.PermApprovalsView, rbac.PermApprovalsDecide)
			rules := rbac.RequireAnyGlobalPermission(app.RBACService, rbac.PermApprovalsView, rbac.PermApprovalsManage)
			manage := rbac.RequireGlobalPermission(app.RBACService, rbac.PermApprovalsManage)
			decide := rbac.RequireGlobalPermission(app.RBACService, rbac.PermApprovalsDecide)

			r.With(read).Get("/", h.HandleList, apispec.Paginated(), apispec.Query("status", "pending, approved, denied, expired, cancelled, executing or executed."))
			r.Get("/mine", h.HandleListMine, apispec.Paginated(), apispec.Query("status", ""))
			r.With(rules).Get("/categories", h.HandleCategories, apispec.Response([]CategoryInfo{}))
			r.With(rules).Get("/rules", h.HandleListRules, apispec.Response([]Rule{}))
			r.With(manage).Post("/rules", h.HandleCreateRule, apispec.Request(RuleInput{}), apispec.Response(Rule{}))
			r.With(manage).Put("/rules/{id}", h.HandleUpdateRule, apispec.Request(RuleInput{}), apispec.Response(Rule{}))
			r.With(manage).Delete("/rules/{id}", h.HandleDeleteRule)

			r.Get("/{id}", h.HandleGet, apispec.Response(Request{}))
			r.With(decide).Post("/{id}/decide", h.HandleDecide, apispec.Request(DecideInput{}), apispec.Response(Request{}))
			r.With(rbac.RequireSelfService(app.RBACService)).Post("/{id}/execute", h.HandleExecute, apispec.Response(ExecuteResult{}))
			r.With(rbac.RequireSelfService(app.RBACService)).Post("/{id}/cancel", h.HandleCancel, apispec.Response(Request{}))
		})
	})
}
