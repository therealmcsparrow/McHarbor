// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers audit module routes (all protected) without a plugin
// runtime. Kept for callers that have no plugin runtime to hand over;
// the log-forwarding routes still mount and simply report no adapters.
func Mount(app *router.AppDeps) {
	MountWithRuntime(app, nil)
}

// MountWithRuntime registers audit module routes and injects the loaded
// plugin runtime so the `logging-` family's forwarders are reachable.
//
// The /audit/logging/* subtree is host-owned even though every
// forwarder behind it ships as a plugin. This matches the storage and
// communication families: the host owns the admin surface (list,
// configure, test, history) and the plugin owns the adapter that does
// the work. Putting the CRUD in each plugin instead would give an
// operator two differently-shaped forms for the same job.
func MountWithRuntime(app *router.AppDeps, runtime any) {
	h := NewHandler(app)
	if runtime != nil {
		h.SetPluginRuntime(runtime)
	}

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/audit", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAuditView)).Get("/", h.HandleList, apispec.Paginated(), apispec.Query("expand", ""), apispec.Scoped(), apispec.Query("action", "repeatable"), apispec.Query("entity_type", ""), apispec.Query("entity_id", ""), apispec.Query("search", ""), apispec.Query("search_mode", ""), apispec.Query("from", "RFC 3339"), apispec.Query("to", "RFC 3339"), apispec.Query("actor", "username or user id"), apispec.Query("severity", "repeatable: info, success, warning, error"), apispec.Response([]Log{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAuditView)).Get("/actions", h.HandleActions, apispec.Query("module", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAuditView)).Get("/plugins/summary", h.HandlePluginSummary)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAuditView)).Get("/coverage", h.HandleCoverage)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAuditExport)).Get("/coverage/export", h.HandleCoverageExport)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAuditExport)).Get("/export", h.HandleExport, apispec.Query("format", ""), apispec.Scoped(), apispec.Query("action", "repeatable"), apispec.Query("entity_type", ""), apispec.Query("entity_id", ""), apispec.Query("search", ""), apispec.Query("search_mode", ""), apispec.Query("from", "RFC 3339"), apispec.Query("to", "RFC 3339"), apispec.Query("actor", "username or user id"), apispec.Query("severity", "repeatable: info, success, warning, error"), apispec.Query("fields", "comma-separated column selection"))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAuditView)).Get("/verify", h.HandleVerify, apispec.Response(VerifyResult{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAuditView)).Get("/denials", h.HandleDenials, apispec.Paginated(), apispec.Query("actor", "username or user id"), apispec.Query("permission", "permission key"), apispec.Query("environment", "environment id, exact"), apispec.Query("from", "RFC 3339"), apispec.Query("to", "RFC 3339"), apispec.Response([]DenialRow{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAuditView)).Get("/ai-actions", h.HandleAIActions, apispec.Paginated(), apispec.Query("actor", "username or user id"), apispec.Query("from", "RFC 3339"), apispec.Query("to", "RFC 3339"), apispec.Response([]AITrailGroup{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAuditView)).Get("/ai-tools", h.HandleAIToolEvents, apispec.Paginated(), apispec.Query("actor", "username or user id"), apispec.Query("from", "RFC 3339"), apispec.Query("to", "RFC 3339"), apispec.Response([]Log{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAuditView)).Get("/retention", h.HandleRetention, apispec.Response(RetentionView{}))
			r.With(
				rbac.RequirePermission(app.RBACService, rbac.PermAuditView),
				rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage),
			).Put("/retention", h.HandleUpdateRetention, apispec.Request(RetentionUpdate{}), apispec.Response(RetentionView{}))

			// Log forwarding. Reading the forwarder list needs both
			// audit.view and settings.manage: the response names every
			// configured sink an audit trail is being shipped to,
			// which is administrative configuration, not audit data.
			// Writes and tests take settings.manage on top, matching
			// the gates the replaced /audit/siem and /audit/syslog
			// routes carried.
			r.Route("/logging", func(r *apispec.Router) {
				r.With(
					rbac.RequirePermission(app.RBACService, rbac.PermAuditView),
					rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage),
				).Get("/", h.HandleForwarders)
				// Static segment before the {id} subtree so chi routes
				// /adapters to the adapter list rather than treating
				// "adapters" as a forwarder id.
				r.With(
					rbac.RequirePermission(app.RBACService, rbac.PermAuditView),
					rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage),
				).Get("/adapters", h.HandleForwarderAdapters)
				r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).
					Post("/", h.HandleCreateForwarder)
				r.Route("/{id}", func(r *apispec.Router) {
					r.With(
						rbac.RequirePermission(app.RBACService, rbac.PermAuditView),
						rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage),
					).Get("/history", h.HandleForwarderHistory)
					r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).
						Put("/", h.HandleSaveForwarder)
					r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).
						Post("/test", h.HandleTestForwarder)
					r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).
						Delete("/", h.HandleDeleteForwarder)
				})
			})

			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Delete("/", h.HandlePurge, apispec.Query("vacuum", ""))
		})
	})
}
