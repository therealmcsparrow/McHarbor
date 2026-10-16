// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compute_console

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestTicketHonoursABACDenyPolicies guards the console gate: it used to ask
// HasPermission, which skips ABAC, so a policy denying consoles (by network, to
// API keys) did not apply to the one thing that is root in the guest.
func TestTicketHonoursABACDenyPolicies(t *testing.T) {
	for _, withPolicy := range []bool{false, true} {
		database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "console.db")+"?_pragma=foreign_keys(0)")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := database.Close(); err != nil {
				t.Errorf("closing test database: %v", err)
			}
		})
		if err := db.Migrate(database, db.DriverSQLite); err != nil {
			t.Fatal(err)
		}
		if err := rbac.UpsertCatalog(database); err != nil {
			t.Fatal(err)
		}
		for _, q := range []string{
			`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`,
			`INSERT INTO roles (id, name, permissions) VALUES ('c', 'c', '["vm.console"]')`,
			`INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur1', 'u1', 'c')`,
			`UPDATE settings SET value = 'true' WHERE key = 'rbac_v3_enabled'`,
		} {
			if _, err := database.Exec(q); err != nil {
				t.Fatalf("seed %q: %v", q, err)
			}
		}
		svc := rbac.NewService(database)
		policies := rbac.NewPolicyService(database)
		svc.SetPolicyService(policies)
		svc.SetAttributeBundle(rbac.NewAttributeBundle(nil, nil))
		svc.SetExecutor(rbac.NewABACExecutor(nil))
		svc.SetRolloutV3(rbac.NewRolloutV3(database))
		if withPolicy {
			if _, err := policies.Create(context.Background(), rbac.CreateInput{
				Name: "no consoles", Effect: "deny", Enabled: true, Priority: 10,
				Expression: `true`, Permissions: []string{"vm.console"}, CreatedBy: "u1",
			}); err != nil {
				t.Fatal(err)
			}
		}

		h := &Handler{app: &router.AppDeps{RBACService: svc, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, subjects: declaredSubjects()}
		req := httptest.NewRequest(http.MethodPost, "/api/compute/console/ticket",
			strings.NewReader(`{"envId":"envA","guestKind":"vm","guestId":"g1","kind":"vnc"}`))
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "op"}))
		rec := httptest.NewRecorder()
		h.HandleIssueTicket(rec, req)

		denied := rec.Code == http.StatusForbidden
		if denied != withPolicy {
			t.Fatalf("withPolicy=%v: got %d (denied=%v)", withPolicy, rec.Code, denied)
		}
	}
}
