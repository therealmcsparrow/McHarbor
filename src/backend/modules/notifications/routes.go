// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package notifications

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers notification channel module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/notifications", func(r chi.Router) {
			r.Get("/", h.HandleList)
			r.Post("/", h.HandleCreate)
			r.Get("/configured-types", h.HandleConfiguredTypes)
			r.Get("/{id}", h.HandleGet)
			r.Put("/{id}", h.HandleUpdate)
			r.Delete("/{id}", h.HandleDelete)
			r.Post("/{id}/test", h.HandleTestNotification)
		})
	})
}
