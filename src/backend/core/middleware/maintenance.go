// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package middleware

import (
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/quiesce"
)

// The pause itself lives in core/quiesce, not here.
//
// It started as a flag in this file, which was wrong: the HTTP layer is only
// half of what writes to the database. The background collectors and
// schedulers hold *sql.DB directly and never pass through a middleware, so a
// flag owned by the request path could only ever pause the half of the
// problem it could see. One gate, read by both, is the only version that can
// actually promise nothing is being written.
//
// BeginMaintenance / EndMaintenance / MaintenanceActive stay as thin
// forwarders so callers on the HTTP side read in HTTP vocabulary.

// BeginMaintenance pauses mutating API requests and background database
// writers. Safe to call more than once; the last reason wins.
func BeginMaintenance(reason string) { quiesce.Begin(reason) }

// EndMaintenance resumes normal service.
func EndMaintenance() { quiesce.End() }

// MaintenanceActive reports whether mutating requests are currently paused.
func MaintenanceActive() bool { return quiesce.Active() }

// maintenanceExemptPrefixes are the request paths that keep working during
// a pause. Auth has to: an operator watching the progress modal must not be
// logged out because their session could not refresh. The database routes
// have to: they are what reports the progress and what would cancel it.
var maintenanceExemptPrefixes = []string{
	"/api/auth/",
	"/api/database/",
}

// Maintenance rejects mutating requests while a database migration is
// copying rows.
//
// Reads are left alone deliberately. The application keeps serving its UI
// out of the source database for the whole copy — it is the source that is
// being read, and nothing about a SELECT invalidates a row that has already
// been copied. Writes are the problem: a row written to the source after
// its table has been copied would not reach the new backend, and the
// operator would discover the loss only after the restart.
//
// The background collectors are held still by the same gate, through
// quiesce.Run at the top of each tick, so raising this flag stops both halves
// of what writes to the database rather than only the request path.
func Maintenance() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !quiesce.Active() || isReadOnlyMethod(r.Method) || isMaintenanceExempt(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			reason := quiesce.Reason()
			if reason == "" {
				reason = "maintenance"
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusServiceUnavailable)
			// Hand-rolled rather than routed through core/response: that
			// package imports this one's siblings, and a maintenance
			// response has exactly one shape.
			if _, err := w.Write([]byte(`{"success":false,"error":"McHarbor is migrating its database; changes are paused until it finishes","code":"err.database.migration_in_progress","reason":"` + reason + `"}`)); err != nil {
				// The client hung up before the body landed. There is no
				// second channel to report that on and nothing left to
				// serve, so the request just ends here.
				return
			}
		})
	}
}

// isReadOnlyMethod reports whether a method can be served unchanged during
// a pause.
func isReadOnlyMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// isMaintenanceExempt reports whether a path stays writable during a pause.
func isMaintenanceExempt(path string) bool {
	for _, prefix := range maintenanceExemptPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
