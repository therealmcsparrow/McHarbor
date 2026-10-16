// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package requestid tags every request with an ID that the access log, the
// failure envelope and the X-Request-Id header all carry, so an operator can
// hand a toast's ID to whoever reads the server log.
package requestid

import (
	"context"
	"net/http"

	"github.com/rs/xid"
)

// Header is the response header the ID is returned in.
const Header = "X-Request-Id"

type ctxKey struct{}

// Middleware stamps a fresh ID on the request context and the response header.
// An inbound X-Request-Id is ignored: a client-chosen ID would let a caller
// make its own failures collide with someone else's in the log.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := xid.New().String()
		w.Header().Set(Header, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

// From returns the request's ID, or "" outside a request the middleware saw.
func From(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}
