// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package in_app_notifications

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers in-app notification routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/in-app-notifications", func(r chi.Router) {
			r.Get("/", h.HandleList)
			r.Get("/unread-count", h.HandleUnreadCount)
			r.Post("/read-all", h.HandleMarkAllRead)
			r.Post("/{id}/read", h.HandleMarkRead)
			r.Delete("/{id}", h.HandleDelete)
		})
	})
}
