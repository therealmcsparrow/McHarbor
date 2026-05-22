// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package custom_nodes

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers custom node routes and initializes the data directory.
// Returns the Executor so the workflow service can use it.
func Mount(app *router.AppDeps) *Executor {
	svc := NewService(app.Config.DataDir, app.Logger)
	if err := svc.Init(); err != nil {
		app.Logger.Error("custom-nodes: failed to init directory", "error", err)
	}

	executor := NewExecutor(svc, app.Logger)
	h := NewHandler(app, svc, executor)

	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Route("/custom-nodes", func(r chi.Router) {
			r.Get("/", h.HandleList)
			r.Post("/", h.HandleCreate)
			r.Post("/test", h.HandleTest)
			r.Route("/{key}", func(r chi.Router) {
				r.Get("/", h.HandleGet)
				r.Put("/", h.HandleUpdate)
				r.Delete("/", h.HandleDelete)
			})
		})
	})

	return executor
}
