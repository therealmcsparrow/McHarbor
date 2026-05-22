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
	return func(next http.Handler) http.Handler {
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

			cookie, err := r.Cookie(SessionCookie)
			if err != nil || cookie.Value == "" {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}

			user, err := authSvc.ValidateSession(cookie.Value)
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
