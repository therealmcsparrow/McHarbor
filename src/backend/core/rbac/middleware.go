// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// RequirePermission returns middleware that checks if the authenticated user
// has the specified permission for the requested environment.
func RequirePermission(svc *Service, perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
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

			allowed, err := svc.HasPermission(user.ID, envID, perm)
			if err != nil {
				response.InternalErrorCode(w, r, i18n.ErrInternalServer)
				return
			}
			if !allowed {
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
			allowed, err := svc.HasAnyStackPermission(user.ID, envID, perm)
			if err != nil {
				response.InternalErrorCode(w, r, i18n.ErrInternalServer)
				return
			}
			if !allowed {
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

			allowed, err := svc.HasStackPermission(user.ID, envID, stackName, perm)
			if err != nil {
				response.InternalErrorCode(w, r, i18n.ErrInternalServer)
				return
			}
			if !allowed {
				response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
