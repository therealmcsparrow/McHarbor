// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package k8sservices

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers Kubernetes service routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/k8s-services", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermK8sServicesView)).Get("/", h.HandleList)
			r.Route("/{namespace}/{name}", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermK8sServicesView)).Get("/", h.HandleGet)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermK8sServicesDelete)).Delete("/", h.HandleDelete)
			})
		})
	})
}
