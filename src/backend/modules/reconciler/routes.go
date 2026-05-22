// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers reconciler module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/reconciler", func(r chi.Router) {
			r.Get("/", h.HandleList)
			r.Post("/", h.HandleCreate)
			r.Get("/{id}", h.HandleGet)
			r.Put("/{id}", h.HandleUpdate)
			r.Delete("/{id}", h.HandleDelete)
			r.Post("/{id}/reconcile", h.HandleReconcile)
			r.Get("/{id}/drift", h.HandleDrift)
		})
	})
}
