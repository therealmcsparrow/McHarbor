// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers network module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/networks", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksView)).Get("/", h.HandleList, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksManage)).Post("/", h.HandleCreate, apispec.Scoped(), apispec.Request(CreateRequest{}))
			// networks.manage rather than networks.view: this exists only to
			// fill the create form, and a read-only viewer has no use for the
			// host's interface names.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksManage)).Get("/host-interfaces", h.HandleHostInterfaces, apispec.Scoped())

			r.Route("/{id}", func(r *apispec.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksView)).Get("/", h.HandleInspect, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksView)).Get("/host-link", h.HandleHostLink, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksManage)).Post("/host-link/apply", h.HandleHostLinkApply, apispec.Scoped())
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksDelete)).Delete("/", h.HandleRemove, apispec.Scoped())
				// Recreate removes and re-creates the network, so it takes the delete key as well as manage.
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksManage), rbac.RequirePermission(app.RBACService, rbac.PermNetworksDelete)).
					Post("/recreate", h.HandleRecreate, apispec.Scoped(), apispec.Request(CreateRequest{}), apispec.Response(RecreateResult{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksManage)).Post("/connect", h.HandleConnect, apispec.Scoped(), apispec.Request(ConnectRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermNetworksManage)).Post("/disconnect", h.HandleDisconnect, apispec.Scoped(), apispec.Request(DisconnectRequest{}))
			})
		})
	})
}
