// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeys

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers API key management module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/api-keys", func(r chi.Router) {
			r.Use(rbac.RequirePermission(app.RBACService, rbac.PermAPIKeysManage))
			r.Get("/", h.HandleList)
			r.Post("/", h.HandleCreate)
			r.Get("/{id}", h.HandleGet)
			r.Delete("/{id}", h.HandleRevoke)
		})
	})
}
