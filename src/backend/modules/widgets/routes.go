// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package widgets

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/plugin/host"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers widget definition routes and seeds built-in widgets.
func Mount(app *router.AppDeps) {
	// The widget catalog is verified with the plugin trust keys, so one signing
	// key and one MCHARBOR_PLUGIN_TRUST_KEYS setting cover plugins and widgets.
	trustKeys, err := host.LoadTrustedKeysFromEnv()
	if err != nil {
		app.Logger.Error("widgets: trust keys could not be loaded; catalog widgets will be refused", "error", err)
		trustKeys = nil
	}
	svc := NewService(app.Config.DataDir, app.Logger).WithCatalog(app.Config.PluginCatalogDirs, trustKeys)
	if err := svc.Seed(); err != nil {
		app.Logger.Error("widgets: failed to seed built-in widgets", "error", err)
	}

	h := NewHandler(app, svc)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/widgets", func(r *apispec.Router) {
			// store_widgets.* is the App Store tier; widgets.* is the
			// dashboard tier the UI gates its layout controls with. Either
			// unlocks the definitions and the install / configure routes.
			view := rbac.RequireAnyPermission(app.RBACService, rbac.PermStoreWidgetsView, rbac.PermWidgetsView)
			manage := rbac.RequireAnyPermission(app.RBACService, rbac.PermStoreWidgetsManage, rbac.PermWidgetsManage)
			r.With(view).Get("/definitions", h.HandleList)
			r.With(manage).Put("/{key}", h.HandleUpdate, apispec.Request(UpdateWidgetStateInput{}))

			// Downloaded widgets run third-party UI code in every viewer's
			// browser, so installing and removing one takes its own key.
			install := rbac.RequirePermission(app.RBACService, rbac.PermDashboardWidgetsInstall)
			r.With(view).Get("/catalog", h.HandleCatalog, apispec.Response([]CatalogWidget{}))
			r.With(install).Post("/catalog/{key}/install", h.HandleInstall, apispec.Response(WidgetDefinition{}))
			r.With(install).Delete("/{key}", h.HandleRemove)
			r.With(view).Get("/{key}/bundle", h.HandleBundle)
		})
	})
}
