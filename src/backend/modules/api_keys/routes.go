// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeys

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers API key management module routes.
//
// Reads accept either api_keys.view or api_keys.manage: a holder of only
// manage still sees their own keys (the handler narrows the list). Every
// mutation requires api_keys.manage; the handler adds the ownership rule.
//
// Clearing usage history is the one exception. It is gated by
// settings.manage, because deciding how long telemetry is kept — and
// destroying it early — is a retention decision about the whole install
// rather than an action on any one key.
//
// No new permission key was needed for any of this: detail and history are
// reads of an API key, and the existing view/manage pair already describes
// exactly that.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/api-keys", func(r *apispec.Router) {
			read := rbac.RequireAnyPermission(app.RBACService, rbac.PermAPIKeysView, rbac.PermAPIKeysManage)
			manage := rbac.RequirePermission(app.RBACService, rbac.PermAPIKeysManage)
			settings := rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)

			r.With(read).Get("/", h.HandleList, apispec.Response([]APIKey{}),
				apispec.Query("rotateAfterDays", "Flag active keys older than this many days as due for rotation; 90 when empty."),
				apispec.Query("expiringWithinDays", "Flag active keys expiring within this many days; 14 when empty."))
			r.With(manage).Get("/options", h.HandleGrantOptions)
			// Declared before /{id} so the static segment reads first, even
			// though chi would prefer it regardless of order.
			r.With(settings).Delete("/usage", h.HandleClearUsage)
			r.With(read).Get("/{id}", h.HandleDetail, apispec.Response(APIKeyDetail{}))
			r.With(read).Get("/{id}/usage", h.HandleUsageSeries, apispec.Query("from", ""), apispec.Query("range", ""), apispec.Query("to", ""))
			r.With(read).Get("/{id}/usage/events", h.HandleUsageEvents, apispec.Paginated(), apispec.Query("auth_outcome", ""), apispec.Query("environment_id", ""), apispec.Query("from", ""), apispec.Query("method", ""), apispec.Query("status_class", ""), apispec.Query("to", ""))
			r.With(manage).Post("/", h.HandleCreate, apispec.Request(CreateAPIKeyInput{}), apispec.Response(CreateAPIKeyResult{}))
			r.With(manage).Patch("/{id}", h.HandleUpdate, apispec.Request(UpdateAPIKeyInput{}), apispec.Response(APIKey{}))
			r.With(manage).Delete("/{id}", h.HandleRevoke)
			// Rotation revokes the old secret and issues a new one with the same
			// grants; it is a change to the key, so manage is the right key.
			r.With(manage).Post("/{id}/rotate", h.HandleRotate, apispec.Response(CreateAPIKeyResult{}))
		})
	})
}
