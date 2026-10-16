// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugin_runtime

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount attaches the diagnostic routes under the protected
// middleware group, plus the one public read the login screen needs
// (login-endpoints) before any session exists.
//
// The read-only contribution surface is registered by h.Mount and is
// authenticated only: the frontend loader reads it for every signed-in
// user. The reload route is registered here instead, wrapped in
// PermPluginsManage — the comment this replaced claimed the handler
// enforced that itself, and it did not.
//
// This is the single-arg form used by every other module; the
// handler is constructed from app so the module does not bypass
// the standard *router.AppDeps injection convention.
func Mount(app *router.AppDeps) *Handler {
	h := NewHandler(app)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		// The loader's own diagnostic routes are recorded like any other.
		// Plugin-contributed routes are not: they are registered dynamically
		// by the runtime and reach the host through the /api catch-all.
		h.Mount(r)
		r.With(rbac.RequirePermission(app.RBACService, rbac.PermPluginsManage)).
			Post("/plugins/loader/reload", h.handleReload)
	})
	app.RegisterPublicRoutes(func(r *apispec.Router) {
		r.Get("/plugins/loader/login-endpoints", h.handleLoginEndpoints, apispec.Response([]LoginEndpoint{}))
	})
	return h
}
