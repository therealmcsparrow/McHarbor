// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package inappnotifications

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers in-app notification routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/in-app-notifications", func(r *apispec.Router) {
			// Reads carry the in_app_notifications.view key, which migration
			// 098 grants to every seeded role so the bell keeps working.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermInAppNotificationsView)).Get("/", h.HandleList, apispec.Paginated(), apispec.Query("q", ""), apispec.Query("mode", ""), apispec.Query("level", ""), apispec.Query("status", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermInAppNotificationsView)).Get("/unread-count", h.HandleUnreadCount)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermInAppNotificationsView)).Get("/counts", h.HandleCounts, apispec.Response(Counts{}))
			// Intentionally ungated: every one of these resolves the
			// notification through the session user, so a caller can only
			// ever mark or delete their own. A permission here would gate
			// a user out of dismissing their own notifications.
			// Exempt: self-service, own notifications only.
			r.Post("/read-all", h.HandleMarkAllRead, apispec.Exempt("self-service: acts only on the caller's own notifications"))
			r.Post("/{id}/read", h.HandleMarkRead, apispec.Exempt("self-service: acts only on the caller's own notifications"))
			r.Delete("/{id}", h.HandleDelete, apispec.Exempt("self-service: acts only on the caller's own notifications"))
		})
	})
}
