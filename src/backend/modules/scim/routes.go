// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers both halves of the SCIM surface:
//
//  1. Admin CRUD endpoints under /api/scim/endpoints, gated by
//     PermScimView / PermScimManage on the regular auth stack.
//  2. SCIM 2.0 protocol endpoints under /api/scim/v2/*, mounted on
//     the public router with BearerAuth so external IdPs and the
//     McHarbor poller can reach them without a session cookie.
//
// We deliberately split them between RegisterProtectedRoutes and
// RegisterPublicRoutes — the SCIM v2 routes MUST be reachable from
// external systems, while the admin UI requires a logged-in user.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	svc := NewService(app.DB, app.Encryption)
	poller := NewPoller(svc, app.Logger)
	if registered, ok := app.LookupService("coordinator"); ok {
		if coord, ok := registered.(pollerCoordinator); ok {
			poller = NewPollerWithCoordinator(svc, app.Logger, coord, coord.NodeID())
		}
	}
	go poller.Start(app.ContextOrBackground())
	app.RegisterService("scim_poller", poller)

	// --- Admin CRUD ------------------------------------------------
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/scim/endpoints", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScimView)).Get("/", h.HandleList)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScimView)).Get("/{id}", h.HandleGet)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScimManage)).Post("/", h.HandleCreate, apispec.Request(EndpointInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScimManage)).Put("/{id}", h.HandleUpdate, apispec.Request(EndpointInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScimManage)).Delete("/{id}", h.HandleDelete)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScimManage)).Post("/{id}/sync-now", h.HandleSyncNow)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScimManage)).Post("/{id}/dry-run", h.HandleDryRun, apispec.Response(DryRunResult{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScimManage)).Post("/{id}/rotate-token", h.HandleRotateToken, apispec.Response(RotatedToken{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScimView)).Get("/{id}/history", h.HandleHistory, apispec.Query("limit", ""), apispec.Response([]SyncLogEntry{}))
		})
	})

	// --- SCIM 2.0 protocol ----------------------------------------
	app.RegisterPublicRoutes(func(r *apispec.Router) {
		// Path is RELATIVE because the parent router already wraps
		// the public-mount group under `/api`. Using `/api/scim/v2`
		// would produce the doubled path `/api/api/scim/v2` and the
		// SCIM v2 endpoints would silently 404. See also the
		// parallel pattern in plugin_runtime/handler.go.
		r.Route("/scim/v2", func(r *apispec.Router) {
			r.Use(BearerAuth(svc))

			r.Get("/ServiceProviderConfig", func(w http.ResponseWriter, _ *http.Request) {
				writeSCIMJSON(w, http.StatusOK, ServiceProviderConfig())
			})
			r.Get("/Schemas", func(w http.ResponseWriter, _ *http.Request) {
				writeSCIMJSON(w, http.StatusOK, ListResponse[map[string]any]{
					Schemas:      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
					TotalResults: len(Schemas()),
					ItemsPerPage: len(Schemas()),
					StartIndex:   1,
					Resources:    Schemas(),
				})
			})

			// /Users collection + /Users/{id}
			r.Get("/Users", h.HandleUsers, apispec.Query("count", ""), apispec.Query("filter", ""), apispec.Query("startIndex", ""))
			r.Get("/Users/{id}", h.HandleUserByID)

			// /Groups collection + /Groups/{id}
			r.Get("/Groups", h.HandleGroups, apispec.Query("count", ""), apispec.Query("startIndex", ""))
			r.Get("/Groups/{id}", h.HandleGroupByID)
		})
	})
}
