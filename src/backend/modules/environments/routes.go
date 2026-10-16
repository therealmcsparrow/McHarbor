// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers environment module routes (all protected).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	// Published so the GitOps preview path can create and destroy its own
	// ephemeral environments without importing this package. It is the first
	// programmatic caller of Create; every other one is the HTTP route.
	app.RegisterService("environments", h.service)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/environments", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsView)).Get("/", h.HandleList, apispec.Response([]Environment{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Post("/", h.HandleCreate, apispec.Request(CreateRequest{}), apispec.Response(Environment{}), apispec.Describe("For an agent environment, data is instead {environment, agentToken}; the agent token is returned only here, once."))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsView)).Get("/detect-socket", h.HandleDetectSocket)
			// Ahead of the /{id} group so chi does not read "duplicates" as an id.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsView)).Get("/duplicates", h.HandleDuplicates)

			r.Route("/{id}", func(r *apispec.Router) {
				r.Use(rbac.PinEnvToPath("id"))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsView)).Get("/", h.HandleGet, apispec.Response(Environment{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Put("/", h.HandleUpdate, apispec.Request(UpdateRequest{}), apispec.Response(Environment{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Delete("/", h.HandleDelete)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Post("/test", h.HandleTestConnection)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsView)).Get("/topology", h.HandleTopology)
				// Migration and merge both mint or destroy credentials and can
				// delete a row, so they sit under manage beside create and delete.
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Post("/migrate", h.HandleMigrateConnection, apispec.Request(ConnectionMigrationRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Delete("/migrate", h.HandleCancelMigration)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Post("/merge", h.HandleMergeEnvironments, apispec.Request(MergeRequest{}), apispec.Query("dryRun", "Report what would be merged without changing anything."))
			})
		})
	})
}
