// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers storage location routes with neither the backup
// migrator nor the plugin runtime. The container-backup migration
// endpoint returns 503 in that state, and plugin-owned LocationTypes
// cannot be tested; production wiring uses MountWithRuntime.
func Mount(app *router.AppDeps) {
	MountWithRuntime(app, nil, nil)
}

// MountWithRuntime registers storage location routes, injects
// the runtime adapter that fulfils the migrate-container-backups
// flow, and injects the loaded plugin runtime so the Test
// endpoint can dispatch to plugin-owned LocationTypes (any
// cloud-drive type contributed by a storage plugin).
//
// Both migrator and runtime are optional — pass nil for either
// to use the no-op default. main.go is the only caller today.
func MountWithRuntime(app *router.AppDeps, migrator BackupMigrator, runtime any) {
	h := NewHandler(app)
	h.SetBackupMigrator(migrator)
	if runtime != nil {
		h.SetPluginRuntime(runtime)
	}

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/storage-locations", func(r *apispec.Router) {
			// Storage locations are host configuration: the global settings
			// keys unlock them, and so do the dedicated storage_locations.* keys.
			view := rbac.RequireAnyGlobalPermission(app.RBACService, rbac.PermSettingsView, rbac.PermStorageLocationsView)
			manage := rbac.RequireAnyGlobalPermission(app.RBACService, rbac.PermSettingsManage, rbac.PermStorageLocationsManage)
			remove := rbac.RequireAnyGlobalPermission(app.RBACService, rbac.PermSettingsManage, rbac.PermStorageLocationsDelete)
			r.With(view).Get("/", h.HandleList, apispec.Response([]StorageLocation{}))
			r.With(manage).Post("/", h.HandleCreate, apispec.Request(CreateStorageLocationInput{}), apispec.Response(StorageLocation{}))
			r.With(manage).Get("/oauth/callback", h.HandleOAuthCallback, apispec.Query("code", ""), apispec.Query("state", ""))
			r.Route("/{id}", func(r *apispec.Router) {
				r.With(view).Get("/", h.HandleGet, apispec.Response(StorageLocation{}))
				r.With(manage).Get("/secrets", h.HandleGetSecrets)
				r.With(manage).Put("/", h.HandleUpdate, apispec.Request(UpdateStorageLocationInput{}), apispec.Response(StorageLocation{}))
				r.With(remove).Delete("/", h.HandleDelete)
				r.With(manage).Post("/migrate-container-backups", h.HandleMigrateContainerBackups)
				r.With(manage).Post("/oauth/authorize", h.HandleOAuthAuthorize)
				r.With(manage).Post("/test", h.HandleTestConnection)
				r.With(view).Get("/usage", h.HandleUsage)
			})
		})
	})
}
