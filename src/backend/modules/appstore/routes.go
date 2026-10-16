// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/modules/appstore/plugins"
)

// Mount registers app store routes using the default runtime service.
func Mount(app *router.AppDeps) {
	MountWithService(app, NewService(app.DB, nil, nil, app.Logger))
}

// MountWithService registers app store routes and seeds enabled catalogs.
func MountWithService(app *router.AppDeps, service *Service) {
	h := NewHandler(app, service)

	if err := h.service.SeedCatalogs(app.ContextOrBackground()); err != nil {
		app.Logger.Error("failed to seed app store catalog", "error", err)
	}

	// Cards report the version and release date the registry publishes, not
	// the ones the catalog author typed. The lookups run here, off the request
	// path, and stop with the server context.
	h.service.StartRegistryMetaRefresher(app.ContextOrBackground())

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/app-store", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsView)).Get("/", h.HandleList, apispec.Paginated(), apispec.Query("category", ""), apispec.Query("search", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsView)).Get("/categories", h.HandleCategories)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsView)).Get("/sync/status", h.HandleSyncStatus)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsManage)).Post("/install/stream", h.HandleInstallStream, apispec.SSE(), apispec.Request(InstallRequest{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsManage)).Post("/installations/{id}/uninstall/stream", h.HandleUninstallStream, apispec.SSE())
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Post("/sync", h.HandleSync)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermStoreAppsView)).Get("/{slug}", h.HandleGet)
		})
	})
}

// MountPlugins registers the /api/appstore/plugins/* route group using
// a plugin Handler built in the bootstrap layer. It is split from
// MountWithService so docker-app routes and plugin routes can be added
// independently — the plugin store reads from settings_catalogs (via
// the CatalogResolver injected into the handler) and not from the
// docker-apps SeedBundledCatalog pipeline.
//
// RBAC lives in plugins.Mount, with every other module's gates, rather
// than here: view (or manage) for reads, manage for install / update /
// configure, manage-or-delete for uninstall.
//
// This used to call the handler's own Mount, which registered the same
// paths with no permission middleware at all — so any authenticated
// account could install, configure or uninstall a plugin. The gated
// variant existed in the plugins package the whole time and simply had
// no caller. Delegating is what wires it.
func MountPlugins(app *router.AppDeps, h *plugins.Handler) {
	plugins.Mount(app, h)
}
