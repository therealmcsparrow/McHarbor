// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/auth"
)

func TestAllowedOnChecksTheNamedEnvironment(t *testing.T) {
	database := migratedRBACDB(t)
	mustExec(t, database, `INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`)
	mustExec(t, database, `INSERT INTO roles (id, name, permissions) VALUES ('ops', 'ops', '["containers.manage"]')`)
	mustExec(t, database, `INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'u1', 'ops', 'envA')`)
	svc := NewService(database)

	// The request's own ?env= is envA, which must not stand in for the target.
	req := httptest.NewRequest(http.MethodPost, "/api/containers/c1/move?env=envA", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "op"}))

	for env, want := range map[string]bool{"envA": true, "envB": false} {
		got, err := svc.AllowedOn(req, env, PermContainersManage)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("AllowedOn(%s) = %v, want %v", env, got, want)
		}
	}

	anonymous := httptest.NewRequest(http.MethodPost, "/api/containers/c1/move", nil)
	if got, err := svc.AllowedOn(anonymous, "envA", PermContainersManage); err != nil || got {
		t.Fatalf("no user must never be allowed: got %v, %v", got, err)
	}
}
