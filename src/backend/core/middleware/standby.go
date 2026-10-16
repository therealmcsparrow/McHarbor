// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package middleware

import (
	"net/http"
	"strings"
	"sync/atomic"
)

var standby atomic.Bool

// SetStandby marks this node as an active/passive standby. The HA controller flips it
// on every role change; everything else only reads it.
func SetStandby(on bool) { standby.Store(on) }

// IsStandby reports whether this node is currently a standby.
func IsStandby() bool { return standby.Load() }

// standbyExemptPrefixes stay reachable on a standby: signing in, the cluster controls
// that promote or drain it, and the probes a load balancer reads.
var standbyExemptPrefixes = []string{
	"/api/auth/",
	"/api/cluster/",
	"/api/livez",
	"/api/readyz",
	"/api/health",
}

// Standby refuses writes and agent connections on a standby node: a write there would
// race the active node, and an agent attached there would be unreachable from it.
func Standby() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !standby.Load() || isStandbyExempt(r.URL.Path) ||
				(isReadOnlyMethod(r.Method) && !strings.HasPrefix(r.URL.Path, "/api/agent/ws")) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "10")
			w.WriteHeader(http.StatusServiceUnavailable)
			if _, err := w.Write([]byte(`{"success":false,"error":"This McHarbor node is a standby; changes are made on the active node","code":"err.cluster.not_ready","reason":"standby"}`)); err != nil {
				return
			}
		})
	}
}

func isStandbyExempt(path string) bool {
	for _, prefix := range standbyExemptPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
