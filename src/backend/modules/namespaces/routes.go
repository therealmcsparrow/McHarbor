// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package namespaces

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers namespace routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/namespaces", func(r chi.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermNamespacesView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermNamespacesView)).Get("/{name}", h.HandleGet)
		})
	})
}
