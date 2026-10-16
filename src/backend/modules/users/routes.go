// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers user management module routes.
func Mount(app *router.AppDeps) {
	mountRoutes(app, NewHandler(app))
}

// MountWithAPIKeyArchiver registers the module with the API-key archive
// collaborator a user delete needs (see bridges.go). Used by main.go.
func MountWithAPIKeyArchiver(app *router.AppDeps, archiver APIKeyArchiver) {
	h := NewHandler(app)
	h.service.SetAPIKeyArchiver(archiver)
	mountRoutes(app, h)
}

func mountRoutes(app *router.AppDeps, h *Handler) {
	// Public password-reset flow (Phase 4). Mounted on the AUTH
	// group, not the public one: both carry no auth middleware, so
	// a logged-out user with a token can still consume it, but the
	// auth group is the one behind middleware.RateLimit(10). The
	// public group is explicitly documented as having no auth rate
	// limit, which left reset-token guessing and reset-email
	// triggering unbounded.
	app.RegisterAuthRoutes(func(r *apispec.Router) {
		r.Route("/auth/password", func(r *apispec.Router) {
			// Exempt: pre-auth. Requesting a reset mail is what a locked-out
			// user does; rate limiting, not a permission, is the control.
			r.Post("/forgot", h.HandleForgotPassword, apispec.Exempt("pre-auth: rate limiting, not a permission, is the control"), apispec.Request(ForgotPasswordRequest{}))
			// Exempt: pre-auth token flow; the single-use token is the credential.
			r.Post("/reset", h.HandleResetPassword, apispec.Exempt("pre-auth: the single-use reset token is the credential"), apispec.Request(ResetPasswordRequest{}))
		})
	})

	// Self-service session endpoints (Phase 4). Mounted under
	// /auth/sessions so the profile page can list and revoke the
	// current user's other sessions without needing the broader
	// users.manage grant.
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/auth/sessions", func(r *apispec.Router) {
			// Exempt: self-service. {sessionId} is validated against the
			// caller's own session list in the handler, so this cannot reach
			// another user's session.
			r.Get("/", h.HandleMySessions)
			r.With(rbac.RequireSelfService(app.RBACService)).Delete("/{sessionId}", h.HandleRevokeMySession)
			r.With(rbac.RequireSelfService(app.RBACService)).Post("/revoke-all", h.HandleRevokeAllMySessions)
		})
	})

	// Break-glass emergency access. Activation and ending one's own session are
	// self-service by design: the designation is the authorisation, and the
	// audit row plus the page to account managers are the control.
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/break-glass", func(r *apispec.Router) {
			r.Get("/status", h.HandleBreakGlassStatus, apispec.Response(BreakGlassStatus{}))
			r.Post("/activate", h.HandleBreakGlassActivate, apispec.Exempt("self-service: only a designated break-glass account can activate, for itself"), apispec.Request(ActivateInput{}), apispec.Response(BreakGlassSession{}))
			r.Post("/end", h.HandleBreakGlassEnd, apispec.Exempt("self-service: ends the caller's own session"), apispec.Response(BreakGlassSession{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersView)).Get("/accounts", h.HandleBreakGlassAccounts, apispec.Response([]BreakGlassAccount{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersView)).Get("/sessions", h.HandleBreakGlassSessions, apispec.Response([]BreakGlassSession{}), apispec.Query("limit", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAccountManage)).Put("/accounts/{userId}", h.HandleBreakGlassDesignate, apispec.Request(DesignateInput{}), apispec.Response(BreakGlassStatus{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAccountManage)).Delete("/accounts/{userId}", h.HandleBreakGlassUndesignate)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAccountManage)).Post("/accounts/{userId}/end", h.HandleBreakGlassRevokeSession, apispec.Response(BreakGlassSession{}))
		})
	})

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/users", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersView)).Get("/", h.HandleList, apispec.Paginated())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Post("/", h.HandleCreate, apispec.Request(CreateUserInput{}))
			// Lifecycle view and bulk actions. Static segments, so chi reads them
			// ahead of /{id}. Bulk delete is its own route so it carries
			// users.manage and falls under the user.delete approval rule.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersView)).Get("/lifecycle", h.HandleLifecycle, apispec.Response(LifecycleReport{}), apispec.Query("inactiveDays", "Days without a sign-in before an account counts as inactive; 90 when empty."), apispec.Query("elevatedDays", "Days an elevation or break-glass session counts as recent; 7 when empty."))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermAccountManage)).Post("/bulk/account", h.HandleBulkAccount, apispec.Request(BulkInput{}), apispec.Response(BulkResult{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Post("/bulk/delete", h.HandleBulkDelete, apispec.Request(BulkInput{}), apispec.Response(BulkResult{}))
			r.Route("/{id}", func(r *apispec.Router) {
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersView)).Get("/", h.HandleGet)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Put("/", h.HandleUpdate, apispec.Request(UpdateUserInput{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Delete("/", h.HandleDelete)
				// Intentionally ungated at the route: this serves two
				// audiences (a user proving their current password, and an
				// admin holding account.password.reset) and the branch lives
				// in HandleChangePassword. Route middleware cannot express
				// "self OR permission" — see handler.go HandleChangePassword.
				r.Put("/password", h.HandleChangePassword, apispec.Exempt("gated in handler: self with current password, otherwise account.password.reset"), apispec.Request(ChangePasswordInput{}))

				// Admin account-state actions (Phase 4).
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermAccountManage)).Post("/disable", h.HandleDisable, apispec.Request(DisableAccountInput{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermAccountManage)).Post("/enable", h.HandleEnable)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermAccountManage)).Post("/unlock", h.HandleUnlock)

				// Admin password overrides (Phase 4).
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermAccountPasswordReset)).Post("/force-reset-password", h.HandleForceResetPassword)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermAccountPasswordReset)).Post("/password/admin", h.HandleAdminSetPassword, apispec.Request(AdminSetPasswordInput{}))

				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersView)).Get("/groups", h.HandleListGroups)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersView)).Get("/roles", h.HandleListRoles, apispec.Response([]UserRole{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Post("/roles", h.HandleAssignRole, apispec.Request(AssignRoleInput{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersManage)).Delete("/roles/{assignmentId}", h.HandleUnassignRole)

				// Per-user grant/deny permission overrides (Phase 2 of
				// the RBAC roadmap). Read requires users.view; write
				// requires rbac.policies.manage so only admins who can
				// already grant permissions can also override them.
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermUsersView)).Get("/overrides", h.HandleListOverrides)
				r.With(rbac.RequireAnyPermission(app.RBACService, rbac.PermRbacPoliciesManage, rbac.PermPermissionsAssign)).Put("/overrides", h.HandleSetOverrides, apispec.Request(SetUserOverridesInput{}))

				// Admin session management (Phase 4).
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSessionsManage)).Get("/sessions", h.HandleListSessions)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSessionsManage)).Delete("/sessions/{sessionId}", h.HandleRevokeSession)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermSessionsManage)).Post("/sessions/revoke-all", h.HandleRevokeAllSessions, apispec.Query("includeSelf", ""))
			})
		})
	})
}
