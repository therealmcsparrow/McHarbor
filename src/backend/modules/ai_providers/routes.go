// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_providers

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers AI provider connection module routes. Mirrors the
// RBAC tiering from communications: view = PermSettingsView (so the
// chat composer can fetch the default), manage = PermSettingsManage
// (so only admins can create / edit / delete / set-default / test).
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/ai-providers", func(r *apispec.Router) {
			// Provider connections are host configuration: the global settings
			// keys unlock them, and so does the dedicated ai_assistant.manage key.
			view := rbac.RequireAnyGlobalPermission(app.RBACService, rbac.PermSettingsView, rbac.PermAIAssistantManage)
			manage := rbac.RequireAnyGlobalPermission(app.RBACService, rbac.PermSettingsManage, rbac.PermAIAssistantManage)
			r.With(view).Get("/", h.HandleList)
			r.With(view).Get("/default", h.HandleGetDefault)
			r.With(view).Get("/oauth-defaults", h.HandleOAuthDefaults)
			r.With(manage).Post("/", h.HandleCreate, apispec.Request(CreateAIProviderInput{}))
			r.Route("/{id}", func(r *apispec.Router) {
				r.With(view).Get("/", h.HandleGet)
				r.With(manage).Put("/", h.HandleUpdate, apispec.Request(UpdateAIProviderInput{}))
				r.With(manage).Delete("/", h.HandleDelete)
				r.With(manage).Post("/default", h.HandleSetDefault)
				r.With(manage).Post("/test", h.HandleTest)
				// "Sign in to connect" -- the OAuth device flow that
				// is the alternative to a pasted API key. Reading the
				// state is a view concern; starting, completing or
				// dropping a sign-in is a mutation, so it takes the
				// same manage key as every other write here.
				r.With(view).Get("/oauth/status", h.HandleOAuthStatus)
				r.With(manage).Post("/oauth/begin", h.HandleOAuthBegin)
				r.With(manage).Post("/oauth/poll", h.HandleOAuthPoll)
				r.With(manage).Post("/oauth/disconnect", h.HandleOAuthDisconnect)
			})
		})
	})
}
