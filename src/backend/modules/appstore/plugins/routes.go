// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount attaches the appstore plugins HTTP routes to the supplied
// router. Every route is session-protected and carries a plugins.*
// gate: view (or manage) for reads, manage for install / update, and
// manage-or-delete for uninstall. Routes are registered
// here rather than through Handler.Mount so the gates live in routes.go
// with every other module.
func Mount(app *router.AppDeps, h *Handler) {
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		view := rbac.RequireAnyPermission(app.RBACService, rbac.PermPluginsView, rbac.PermPluginsManage)
		manage := rbac.RequirePermission(app.RBACService, rbac.PermPluginsManage)
		remove := rbac.RequireAnyPermission(app.RBACService, rbac.PermPluginsManage, rbac.PermPluginsDelete)
		r.With(view).Get("/appstore/plugins/installed", h.handleListInstalled)
		r.With(view).Get("/appstore/plugins/catalog", h.handleListCatalog)
		r.With(view).Get("/appstore/plugins/{slug}", h.handleGet)
		r.With(view).Get("/appstore/plugins/{slug}/icon", h.handleCatalogIcon, apispec.Binary(), apispec.Query("variant", "dark for the plugin's dark-mode mark, falling back to the plain one"))
		r.With(manage).Post("/appstore/plugins/install/stream", h.handleInstallStream, apispec.SSE(), apispec.Request(InstallRequest{}))
		r.With(manage).Post("/appstore/plugins/{slug}/update/stream", h.handleUpdateStream, apispec.SSE(), apispec.Request(UpdateRequest{}))
		r.With(manage).Post("/appstore/plugins/{slug}/reconcile", h.handleReconcile, apispec.Request(ReconcileRequest{}), apispec.Response(ReconcileResult{}))
		r.With(remove).Post("/appstore/plugins/uninstall", h.handleUninstall, apispec.Request(UninstallRequest{}))
	})
}
