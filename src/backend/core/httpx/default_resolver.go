// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package httpx

import (
	"net/http"
	"sync/atomic"
)

var defaultResolver atomic.Pointer[ClientAddressResolver]

// SetDefaultResolver installs the resolver ClientIP consults; main sets it from
// MCHARBOR_TRUSTED_PROXY_CIDRS before the router serves anything.
func SetDefaultResolver(r *ClientAddressResolver) {
	defaultResolver.Store(r)
}

// ClientIP is the origin address for logs, audit rows and login throttling. It
// believes forwarding headers only from a trusted proxy, so a client cannot pick
// its own address to dodge a per-IP lockout or forge an audit trail.
func ClientIP(r *http.Request) string {
	return defaultResolver.Load().Resolve(r).ClientIP
}

// IsTrustedProxy reports whether ip lies inside the default resolver's trusted
// proxy CIDRs. Per-address lockout uses it to refuse to count a proxy, because a
// proxy address only reaches it when attribution failed and locking it locks everyone.
func IsTrustedProxy(ip string) bool {
	return defaultResolver.Load().TrustsAddress(ip)
}
