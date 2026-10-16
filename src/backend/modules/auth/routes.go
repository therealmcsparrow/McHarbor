// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/middleware"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers auth module routes.
// Login, logout, and setup are public (no auth middleware).
// Session is protected (requires auth middleware).
func Mount(app *router.AppDeps) {
	mountRoutes(app, NewHandler(app))
}

// MountWithMFA registers the module with the second-factor verifier
// injected (see bridges.go). Used by main.go.
func MountWithMFA(app *router.AppDeps, verifier MFAVerifier) {
	mountRoutes(app, NewHandlerWithMFA(app, verifier))
}

func mountRoutes(app *router.AppDeps, h *Handler) {
	// Public auth routes (rate-limited, no auth middleware)
	app.RegisterAuthRoutes(func(r *apispec.Router) {
		r.Route("/auth", func(r *apispec.Router) {
			r.Get("/status", h.HandleStatus)
			// Ungated on purpose: the setup wizard and the token-based
			// reset page run logged out and need the rules before the
			// user types. Complexity metadata only — see the handler.
			r.Get("/password-policy", h.HandlePasswordPolicy)
			// Exempt: pre-auth. Both run before a session exists, so there is
			// no principal for a permission to be checked against.
			r.Post("/login", h.HandleLogin, apispec.Exempt("pre-auth: no session exists yet"), apispec.Request(loginRequest{}))
			r.Post("/login/mfa", h.HandleLoginMFA, apispec.Exempt("pre-auth: no session exists yet"), apispec.Request(loginMFARequest{}))
			// Exempt: session teardown acts only on the caller's own cookie.
			r.Post("/logout", h.HandleLogout, apispec.Exempt("self-service: tears down the caller's own session"))
			// Exempt: first-run bootstrap runs before any account exists.
			r.Post("/setup", h.HandleSetup, apispec.Exempt("first-run: refuses once any account exists"), apispec.Request(setupRequest{}))
		})
	})

	// Protected auth routes
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Get("/auth/session", h.HandleSession)
		// Both act on the caller's own session, identified by the
		// cookie and never by a parameter, so there is nothing for a
		// permission to scope — same reasoning as /auth/profile below.
		// The policy is what the browser needs to run its own idle
		// countdown; the touch is that countdown reporting back.
		// Exempt: self-service. Both act on the caller's own session.
		r.Get("/auth/session/policy", h.HandleSessionPolicy)
		r.Post("/auth/session/touch", h.HandleTouchSession, apispec.Exempt("self-service: acts only on the caller's own session"))
		// Step-up re-authentication. Self-service like the two above: it acts
		// on the caller's own session and grants nothing on its own — a window
		// is always *additional* to whatever permission the action it unlocks
		// already requires.
		r.With(middleware.RateLimit(sudoAttemptsPerMinute, h.sudoAttemptKey), rbac.RequireSelfService(app.RBACService)).Post("/auth/sudo", h.HandleSudo, apispec.Request(SudoRequest{}))
		r.Get("/auth/me/permissions", h.HandleMyPermissions)
		// The catalog feeds the role editor; permissions.view is the
		// dedicated key, roles.view keeps existing role authors working.
		r.With(rbac.RequireAnyPermission(app.RBACService, rbac.PermRolesView, rbac.PermPermissionsView)).Get("/auth/permissions/catalog", h.HandlePermissionCatalog)
		// Intentionally ungated. Both act on the caller's own account,
		// taken from the session and never from a path parameter, so
		// there is nothing for a permission to scope. `account.self.manage`
		// exists in the catalog but reaches no seeded role, so requiring it
		// here would lock every non-admin out of their own profile.
		r.With(rbac.RequireSelfService(app.RBACService)).Put("/auth/profile", h.HandleUpdateProfile, apispec.Request(profileRequest{}))
		r.Put("/auth/preferences", h.HandleUpdatePreferences, apispec.Exempt("self-service: acts only on the caller's own account"), apispec.Request(preferencesRequest{}))
	})
}
