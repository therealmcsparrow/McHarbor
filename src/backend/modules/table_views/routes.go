// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package table_views

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers the table-views routes without a permission key: like
// PUT /auth/preferences they act only on the session user's own rows.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/table-views", func(r *apispec.Router) {
			r.Post("/import", h.HandleImport, apispec.Exempt("self-service: table views are scoped to the caller's user id"), apispec.Request(ImportInput{}))
			r.Get("/{tableId}", h.HandleGet)
			r.Put("/{tableId}/layout", h.HandlePutLayout, apispec.Exempt("self-service: table views are scoped to the caller's user id"))
			r.Delete("/{tableId}/layout", h.HandleDeleteLayout, apispec.Exempt("self-service: table views are scoped to the caller's user id"))
			r.Post("/{tableId}/filters", h.HandleCreateFilter, apispec.Exempt("self-service: table views are scoped to the caller's user id"), apispec.Request(FilterInput{}))
			r.Put("/{tableId}/filters/{id}", h.HandleUpdateFilter, apispec.Exempt("self-service: table views are scoped to the caller's user id"), apispec.Request(FilterUpdate{}))
			r.Delete("/{tableId}/filters/{id}", h.HandleDeleteFilter, apispec.Exempt("self-service: table views are scoped to the caller's user id"))
		})
	})
}
