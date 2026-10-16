// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package webhooks

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers webhook module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/webhooks", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermWebhooksView)).Get("/", h.HandleList, apispec.Paginated())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermWebhooksManage)).Post("/", h.HandleCreate, apispec.Request(CreateWebhookInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermWebhooksView)).Get("/{id}", h.HandleGet)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermWebhooksManage)).Put("/{id}", h.HandleUpdate, apispec.Request(UpdateWebhookInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermWebhooksDelete)).Delete("/{id}", h.HandleDelete)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermWebhooksManage)).Post("/{id}/test", h.HandleTest)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermWebhooksView)).Get("/{id}/deliveries", h.HandleListDeliveries, apispec.Paginated())
		})
	})
}
