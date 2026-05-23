// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package customnodes

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers custom node routes with default runtime services.
func Mount(app *router.AppDeps) {
	executor, svc := buildRuntimeModule(app)
	MountWithExecutor(app, svc, executor)
}

// MountWithExecutor registers custom node routes and initializes the data directory.
func MountWithExecutor(app *router.AppDeps, svc *Service, executor *Executor) {
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

}

func buildRuntimeModule(app *router.AppDeps) (*Executor, *Service) {
	svc := NewService(app.Config.DataDir, app.Logger)
	if err := svc.Init(); err != nil {
		app.Logger.Error("custom-nodes: failed to init directory", "error", err)
	}
	return NewExecutor(svc, app.Logger), svc
}

// NewRuntimeModule exposes the initialized service and executor for workflow bootstrap.
func NewRuntimeModule(app *router.AppDeps) (*Service, *Executor) {
	executor, svc := buildRuntimeModule(app)
	return svc, executor
}
