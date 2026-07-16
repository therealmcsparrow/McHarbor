// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers storage location routes. The container-backup
// migration endpoint requires the backup migrator to be wired in
// via MountWithBackupMigrator; without it the migration endpoint
// returns 503.
func Mount(app *router.AppDeps) {
	MountWithBackupMigrator(app, nil)
}

// MountWithBackupMigrator registers storage location routes and
// injects the runtime adapter that fulfils the
// migrate-container-backups flow. main.go calls this with the
// concrete container-backups adapter.
func MountWithBackupMigrator(app *router.AppDeps, migrator BackupMigrator) {
	h := NewHandler(app)
	h.SetBackupMigrator(migrator)

		app.RegisterProtectedRoutes(func(r chi.Router) {
			r.Route("/storage-locations", func(r chi.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsView)).Get("/", h.HandleList)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Post("/", h.HandleCreate)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Get("/oauth/callback", h.HandleOAuthCallback)
				r.Route("/{id}", func(r chi.Router) {
					r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsView)).Get("/", h.HandleGet)
					r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Put("/", h.HandleUpdate)
					r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Delete("/", h.HandleDelete)
					r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Post("/migrate-container-backups", h.HandleMigrateContainerBackups)
					r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Post("/oauth/authorize", h.HandleOAuthAuthorize)
					r.With(rbac.RequirePermission(app.RBACService, rbac.PermSettingsManage)).Post("/test", h.HandleTestConnection)
				})
			})
		})
}
