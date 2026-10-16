// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// observeDenied counts a refusal by the key, or the "|"-joined keys, that
// would have let the request through; route gates are static, so it stays bounded.
func observeDenied(perms ...Permission) {
	keys := make([]string, len(perms))
	for i, p := range perms {
		keys[i] = string(p)
	}
	telemetry.PermissionDenied(strings.Join(keys, "|"))
}

// RequirePermission returns middleware that checks if the authenticated user
// has the specified permission for the requested environment.
func RequirePermission(svc *Service, perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if p, ok := probed(next, ScopeEnvironment, false, perm); ok {
			return p
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}

			// System user (AUTH_DISABLE=true) bypasses RBAC
			if user.ID == "system" {
				next.ServeHTTP(w, r)
				return
			}

			envID := response.ParseEnvID(r)

			allowed, err := svc.decideRoute(r, user.ID, envID, perm)
			if err != nil {
				response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
				return
			}
			if !allowed {
				RecordDenied(r, user, perm, envID)
				response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireGlobalPermission checks a permission without accepting an
// environment scope from the request. Use it for application-wide operations.
func RequireGlobalPermission(svc *Service, perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if p, ok := probed(next, ScopeGlobal, false, perm); ok {
			return p
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}

			if user.ID == "system" {
				next.ServeHTTP(w, r)
				return
			}

			allowed, err := svc.HasPermission(r.Context(), user.ID, "", perm)
			if err != nil {
				response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
				return
			}
			if !allowed {
				observeDenied(perm)
				response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyStackPermission allows access when the user has the permission for the
// current environment or at least one stack in that environment.
func RequireAnyStackPermission(svc *Service, perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if p, ok := probed(next, ScopeStack, false, perm); ok {
			return p
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}

			if user.ID == "system" {
				next.ServeHTTP(w, r)
				return
			}

			envID := response.ParseEnvID(r)
			allowed, err := svc.HasAnyStackPermission(r.Context(), user.ID, envID, perm)
			if err != nil {
				response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
				return
			}
			if !allowed {
				observeDenied(perm)
				response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireStackPermission checks the given permission for a specific stack route.
func RequireStackPermission(svc *Service, perm Permission, param string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if p, ok := probed(next, ScopeStack, false, perm); ok {
			return p
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}

			if user.ID == "system" {
				next.ServeHTTP(w, r)
				return
			}

			envID := response.ParseEnvID(r)
			stackName := chi.URLParam(r, param)

			allowed, err := svc.HasStackPermission(r.Context(), user.ID, envID, stackName, perm)
			if err != nil {
				response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
				return
			}
			if !allowed {
				observeDenied(perm)
				response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyGlobalPermission is RequireAnyPermission without an
// environment scope: the request passes when the principal holds at least
// one of perms globally. Use it where a host-level key (settings.*) and a
// narrower feature key both unlock the same route.
func RequireAnyGlobalPermission(svc *Service, perms ...Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if p, ok := probed(next, ScopeGlobal, true, perms...); ok {
			return p
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}

			if user.ID == "system" {
				next.ServeHTTP(w, r)
				return
			}

			for _, perm := range perms {
				allowed, err := svc.HasPermission(r.Context(), user.ID, "", perm)
				if err != nil {
					response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
					return
				}
				if allowed {
					next.ServeHTTP(w, r)
					return
				}
			}

			observeDenied(perms...)
			response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
		})
	}
}

// RequireAnyPermission allows the request when the principal holds at
// least one of perms for the requested environment. Use it for read
// routes that a "view" and a "manage" key both unlock.
func RequireAnyPermission(svc *Service, perms ...Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if p, ok := probed(next, ScopeEnvironment, true, perms...); ok {
			return p
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}

			// System user (AUTH_DISABLE=true) bypasses RBAC
			if user.ID == "system" {
				next.ServeHTTP(w, r)
				return
			}

			envID := response.ParseEnvID(r)
			for _, perm := range perms {
				allowed, err := svc.decideRoute(r, user.ID, envID, perm)
				if err != nil {
					response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
					return
				}
				if allowed {
					next.ServeHTTP(w, r)
					return
				}
			}

			if len(perms) > 0 {
				RecordDenied(r, user, perms[0], envID)
			}
			response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
		})
	}
}
