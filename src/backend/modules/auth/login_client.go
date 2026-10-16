// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// resolveLoginClient attributes a login to its origin through the trusted-proxy
// resolver, because per-address lockout keys on the result and a spoofable
// header would let a client dodge its own lock or lock out someone else's
// address. With no resolver wired it falls back to the socket peer.
func resolveLoginClient(app *router.AppDeps, r *http.Request) auth.LoginClient {
	client := auth.LoginClient{UserAgent: r.UserAgent()}
	if app == nil || app.ResolveClientAddress == nil {
		addr := (*httpx.ClientAddressResolver)(nil).Resolve(r)
		client.IP, client.Source = addr.ClientIP, addr.Source
		return client
	}
	_, client.IP, client.Source = app.ResolveClientAddress(r)
	return client
}
