// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package git

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers git repository module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/git", func(r chi.Router) {
			r.Get("/", h.HandleListRepos)
			r.Post("/", h.HandleCreateRepo)
			r.Get("/{id}", h.HandleGetRepo)
			r.Put("/{id}", h.HandleUpdateRepo)
			r.Delete("/{id}", h.HandleDeleteRepo)
			r.Post("/{id}/sync", h.HandleSync)
			r.Get("/{id}/deployments", h.HandleListDeployments)
		})
	})
}
