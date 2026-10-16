// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"net/http"
	"strings"
)

// guardedPrefix is a route family demo mode refuses because it reaches past the
// simulated estate: the machine McHarbor runs on, the real database files, the
// release server or the licence server. readsToo also refuses GET.
type guardedPrefix struct {
	prefix   string
	readsToo bool
}

var guarded = []guardedPrefix{
	{prefix: "/api/system/self", readsToo: true},
	{prefix: "/api/host-services/self", readsToo: true},
	{prefix: "/api/system/reset"},
	{prefix: "/api/updates/check", readsToo: true},
	{prefix: "/api/updates/agents/releases/check", readsToo: true},
	{prefix: "/api/updates"},
	{prefix: "/api/instance-backups"},
	{prefix: "/api/database"},
	{prefix: "/api/license"},
	{prefix: "/api/storage-locations"},
	{prefix: "/api/settings/timezone"},
	{prefix: "/api/settings/tls"},
	{prefix: "/api/settings/backup-key"},
	{prefix: "/api/appstore/plugins"},
	{prefix: "/api/plugins/loader/reload"},
}

// Blocked reports whether demo mode refuses method on path. It is exported so
// the route table test can assert the list stays in step with the router.
func Blocked(method, path string) bool {
	for _, g := range guarded {
		if path != g.prefix && !strings.HasPrefix(path, g.prefix+"/") && !strings.HasPrefix(path, g.prefix+"?") {
			continue
		}
		if g.readsToo {
			return true
		}
		return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
	}
	return false
}

// Guard refuses the routes in the guarded list while the process runs in demo
// mode, and is a pass-through otherwise, so it costs nothing on a normal boot.
func Guard(forbidden func(w http.ResponseWriter, r *http.Request)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if Active() && Blocked(r.Method, r.URL.Path) {
				w.Header().Set("X-McHarbor-Demo", "blocked")
				forbidden(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
