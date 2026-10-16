// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package scim implements SCIM 2.0 provisioning: the inbound protocol endpoints, the outbound directory puller, and endpoint management.
package scim

import (
	"context"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// contextKey is unexported so other packages can't accidentally
// collide on the same lookup key.
type contextKey int

const (
	// ctxEndpointKey stores the authenticated *Endpoint on the
	// request context once BearerAuth has matched a bearer token.
	ctxEndpointKey contextKey = iota
)

// EndpointFromContext returns the SCIM endpoint that authenticated
// the current request. The boolean reports whether one was found;
// callers MUST treat a missing endpoint as unauthenticated.
func EndpointFromContext(ctx context.Context) (*Endpoint, bool) {
	ep, ok := ctx.Value(ctxEndpointKey).(*Endpoint)
	return ep, ok
}

// BearerAuth is a chi middleware that authenticates a SCIM 2.0
// request by parsing the Authorization header, hashing it, and
// looking up the corresponding endpoint. The endpoint is stored on
// the request context for downstream handlers.
//
// SCIM routes are mounted under RegisterPublicRoutes so this
// middleware replaces the normal session check. We deliberately
// avoid the 10/min auth rate limit because an IdP poll cadence is
// far higher than user logins.
func BearerAuth(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bearer := ExtractBearerToken(r)
			if bearer == "" {
				response.UnauthorizedCode(w, r, i18n.ErrSCIMTokenInvalid)
				return
			}
			ep, err := svc.LookupByBearerToken(r.Context(), bearer)
			if err != nil {
				// Distinguish "no token" from "internal error" only
				// for the audit log; to the client we always return
				// 401 so we don't leak whether the token was
				// recognised.
				response.UnauthorizedCode(w, r, i18n.ErrSCIMTokenInvalid)
				return
			}
			ctx := context.WithValue(r.Context(), ctxEndpointKey, ep)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
