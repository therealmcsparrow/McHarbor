// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package communications

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers communication channel module routes.
func Mount(app *router.AppDeps) {
	MountWithRuntime(app, nil)
}

// MountWithRuntime registers communication channel routes with optional plugin adapters.
func MountWithRuntime(app *router.AppDeps, runtime any) {
	h := NewHandler(app)
	if runtime != nil {
		h.SetPluginRuntime(runtime)
	}

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/communication-channels", func(r *apispec.Router) {
			r.Get("/capabilities", h.HandleCapabilities)
			// Per-adapter declared capabilities — chiefly "can this
			// channel carry a file". Gated on the same view permission
			// as the channel list it annotates; the response carries
			// only channel types and boolean flags, never credentials.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsView)).
				Get("/adapters", h.HandleAdapterCapabilities)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsManage)).Post("/", h.HandleCreate, apispec.Request(CreateChannelInput{}))
			r.Route("/{id}", func(r *apispec.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsView)).Get("/", h.HandleGet)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsManage)).Put("/", h.HandleUpdate, apispec.Request(UpdateChannelInput{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsManage)).Delete("/", h.HandleDelete)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsManage)).Post("/default", h.HandleSetDefault)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsManage)).Post("/test", h.HandleTest)
			})
		})
	})
}
