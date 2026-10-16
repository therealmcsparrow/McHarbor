// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// TestPluginRouteDeclaredSeverityIsWritten: a plugin reveal route declaring audit_severity:
// warning must land at warning, like the host's own reveals, rather than at the inferred level.
func TestPluginRouteDeclaredSeverityIsWritten(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTest(t, database) })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	logger := NewLogger(database, "secret")
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	call := func(mw func(http.Handler) http.Handler) {
		mw(ok).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/swarm/services/s1/env", nil))
	}
	call(PluginFallbackMiddlewareWithSeverity(logger, "addon-swarm", "plugin.swarm.service.env_revealed", "swarm_service", "warning", true))
	call(PluginFallbackMiddleware(logger, "addon-swarm", "plugin.swarm.service.inspected", "swarm_service", true))

	severity := func(action string) string {
		var s string
		if err := database.QueryRow(`SELECT severity FROM audit_logs WHERE action = ? LIMIT 1`, action).Scan(&s); err != nil {
			t.Fatalf("reading the %s row: %v", action, err)
		}
		return s
	}
	if got := severity("plugin.swarm.service.env_revealed"); got != "warning" {
		t.Fatalf("declared audit_severity warning was stored as %q", got)
	}
	if got := severity("plugin.swarm.service.inspected"); got == "warning" {
		t.Fatalf("an undeclared successful GET was stored as %q; it must keep its inferred severity", got)
	}
}
