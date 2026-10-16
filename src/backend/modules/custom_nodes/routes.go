// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package customnodes

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
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

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/custom-nodes", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermCustomNodesView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermCustomNodesManage)).Post("/", h.HandleCreate, apispec.Request(CreateCustomNodeInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermCustomNodesManage)).Post("/test", h.HandleTest, apispec.Request(TestCustomNodeInput{}))
			r.Route("/{key}", func(r *apispec.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCustomNodesView)).Get("/", h.HandleGet)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCustomNodesManage)).Put("/", h.HandleUpdate, apispec.Request(UpdateCustomNodeInput{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCustomNodesManage)).Delete("/", h.HandleDelete)
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
