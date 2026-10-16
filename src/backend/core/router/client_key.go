// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package router

import "net/http"

// ClientKey returns the origin address ResolveClientAddress attributes r to, for
// keying per-client throttles. It returns "" when no resolver is wired, which
// middleware.RateLimit treats as "key on the socket peer" rather than a shared bucket.
func (a *AppDeps) ClientKey(r *http.Request) string {
	if a == nil || a.ResolveClientAddress == nil {
		return ""
	}
	_, clientIP, _ := a.ResolveClientAddress(r)
	return clientIP
}
