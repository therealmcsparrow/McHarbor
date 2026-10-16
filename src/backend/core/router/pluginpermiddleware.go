// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package router

import (
	"context"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// PluginPermissionMiddleware returns a chi middleware that enforces
// the given permission on the authenticated user. It is the bridge
// between the plugin runtime's `routeRegistry.mount` and the host's
// RBAC service: every plugin-contributed route declared with
// `requires_permission` in its manifest is wrapped with this
// middleware before being mounted into the public / auth / protected
// router.
//
// Empty `perm` short-circuits to a pass-through so the call site
// doesn't need to special-case "no permission required". The
// permission check uses the user stashed in the request context by
// the auth middleware; anonymous requests land on 401.
//
// On deny the middleware writes a generic 403 with no error details
// (the real failure is logged at the call site; the client must
// not learn whether the user is unknown vs unauthorized).
func PluginPermissionMiddleware(deps interface {
	HasPermissionInContext(ctx context.Context, userID, envID, perm string) (bool, error)
}, perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if perm == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			// envID is empty at the routing layer — environment
			// scoping is resolved inside the plugin handler itself
			// (plugins read ?env=… from the request URL).
			allowed, err := deps.HasPermissionInContext(r.Context(), user.ID, "", perm)
			if err != nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if !allowed {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PluginRouteDecider is implemented by host deps that can evaluate ABAC policies for a
// plugin route; deps without it keep the key-only check.
type PluginRouteDecider interface {
	DecidePluginRoute(r *http.Request, userID, envID, perm, pattern string) (bool, error)
}

// PluginPermissionMiddlewareFor is PluginPermissionMiddleware that also knows the
// declared route pattern, so a policy bound to a plugin's resource type applies, and
// passes the request's environment instead of always checking globally.
func PluginPermissionMiddlewareFor(deps interface {
	HasPermissionInContext(ctx context.Context, userID, envID, perm string) (bool, error)
}, perm, pattern string) func(http.Handler) http.Handler {
	decider, ok := deps.(PluginRouteDecider)
	if !ok {
		return PluginPermissionMiddleware(deps, perm)
	}
	return func(next http.Handler) http.Handler {
		if perm == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			allowed, err := decider.DecidePluginRoute(r, user.ID, r.URL.Query().Get("env"), perm, pattern)
			if err != nil || !allowed {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
