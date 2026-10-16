// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

type contextKey string

// UserContextKey is exported for use by API key middleware.
const UserContextKey contextKey = "user"

// Middleware validates the session cookie and injects the user into context.
func Middleware(authSvc *Service) func(http.Handler) http.Handler {
	return func(inner http.Handler) http.Handler {
		// Only a caller past this gate may read a failure's cause; a signed-out
		// caller on a pre-auth route still gets the code and request ID alone.
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			inner.ServeHTTP(w, r.WithContext(response.WithDetail(r.Context())))
		})
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If auth is disabled, inject a system user
			if !authSvc.IsAuthEnabled() {
				dn := "Admin"
				systemUser := &User{
					ID:          "system",
					Username:    "admin",
					DisplayName: &dn,
				}
				ctx := context.WithValue(r.Context(), UserContextKey, systemUser)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// A Bearer API key authenticated upstream (rbac.APIKeyMiddleware)
			// already put the owner in context. A session cookie is not
			// required on top of it; the key principal in the same context
			// fences what that user may do.
			if existing := UserFromContext(r.Context()); existing != nil {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(SessionCookie)
			if err != nil || cookie.Value == "" {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}

			user, err := authSvc.validateSession(r.Context(), cookie.Value)
			if err != nil || user == nil {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserFromContext extracts the authenticated user from the request context.
func UserFromContext(ctx context.Context) *User {
	user, _ := ctx.Value(UserContextKey).(*User)
	return user
}

// RequireAuth extracts the user from context, returning 401 if not found.
func RequireAuth(r *http.Request) *User {
	return UserFromContext(r.Context())
}
