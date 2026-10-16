// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
)

// TestDeniedAuditRecordsOncePerWindow guards the denial trail: a 403 lands in the
// audit log, and a page polling the same forbidden route does not flood it.
func TestDeniedAuditRecordsOncePerWindow(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	SetDeniedAuditHook(DefaultDeniedAuditHook(audit.NewLogger(database, "s")), nil)
	t.Cleanup(func() { SetDeniedAuditHook(nil, nil) })
	deniedAuditThrottle = &auditThrottle{seen: map[string]time.Time{}, window: time.Minute}

	user := &auth.User{ID: "u1", Username: "op"}
	for i := 0; i < 3; i++ {
		RecordDenied(httptest.NewRequest("GET", "/api/audit", nil), user, PermAuditView, "")
	}
	RecordDenied(httptest.NewRequest("GET", "/api/roles", nil), user, PermAuditView, "")

	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'permission.denied' AND entity_id = 'u1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("denial rows = %d, want 2 (one per route per window)", n)
	}
}
