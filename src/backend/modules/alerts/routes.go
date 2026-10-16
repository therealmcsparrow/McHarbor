// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers alert rule module routes.
//
// It also hands the module its late-bound view of the plugin runtime.
// The runtime does not exist yet at this point — pluginruntime.Boot
// runs later — so a closure is stored and read on first use, the same
// ordering the communications bridge works around.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	provider := func() any { return app.PluginAdapters }
	registry.setPluginSourceProvider(provider)
	pluginRuntimeProvider = provider

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/alerts", func(r *apispec.Router) {
			// Reads are gated on alerts.view. The list route carried
			// only RequireAuth before this change; the catalog and
			// history routes are new, and gating all three together
			// closes that gap rather than widening it.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAlertsView)).Get("/", h.HandleList, apispec.Paginated())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAlertsView)).Get("/conditions", h.HandleConditions)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAlertsView)).Get("/template-reference", h.HandleTemplateReference, apispec.Query("source", ""), apispec.Query("condition", ""), apispec.Response(TemplateReference{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAlertsView)).Post("/template-preview", h.HandleTemplatePreview, apispec.Request(TemplatePreviewInput{}), apispec.Response(TemplatePreview{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAlertsView)).Get("/events", h.HandleEvents, apispec.Paginated(), apispec.Query("alertId", ""), apispec.Query("severity", ""), apispec.Query("state", ""))

			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAlertsManage)).Post("/", h.HandleCreate, apispec.Request(CreateAlertInput{}), apispec.Response(Alert{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAlertsManage)).Put("/{id}", h.HandleUpdate, apispec.Request(UpdateAlertInput{}), apispec.Response(Alert{}))
			// Test-fire sends real messages to real channels, so it
			// takes the manage key rather than the view key.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAlertsManage)).Post("/{id}/test", h.HandleTest)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAlertsDelete)).Delete("/{id}", h.HandleDelete)
		})
	})
}
