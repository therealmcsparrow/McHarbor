// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package middleware

import (
	"log/slog"
	"net/http"
	"sync"
)

// forwardingHeaders are the headers a reverse proxy adds. Any of them arriving
// while no proxy is trusted means attribution is silently using the proxy's
// own address for every client.
var forwardingHeaders = []string{"X-Forwarded-For", "Forwarded", "X-Real-IP"}

// ForwardedHeaderWarning logs one warning, the first time a request carries a
// forwarding header while MCHARBOR_TRUSTED_PROXY_CIDRS is empty. It fires once
// per process so a proxied host does not log on every request.
func ForwardedHeaderWarning(logger *slog.Logger, trustedProxiesConfigured bool) func(http.Handler) http.Handler {
	if trustedProxiesConfigured || logger == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	var once sync.Once
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, h := range forwardingHeaders {
				if r.Header.Get(h) == "" {
					continue
				}
				once.Do(func() {
					logger.Warn("security posture: a request arrived through a proxy, but no proxy is trusted; "+
						"client addresses are recorded as the proxy's until it is declared",
						"env", "MCHARBOR_TRUSTED_PROXY_CIDRS", "header", h)
				})
				break
			}
			next.ServeHTTP(w, r)
		})
	}
}
