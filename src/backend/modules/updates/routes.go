// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers auto-update policy module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/updates", func(r chi.Router) {
			r.Get("/", h.HandleList)
			r.Post("/", h.HandleCreate)
			r.Get("/check", h.HandleCheckUpdate)
			r.Get("/{id}", h.HandleGet)
			r.Put("/{id}", h.HandleUpdate)
			r.Delete("/{id}", h.HandleDelete)
			r.Get("/{id}/history", h.HandleHistory)
		})
	})
}
