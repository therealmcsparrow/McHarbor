// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
)

// TestRouteGateAppliesTypedPolicies guards the reason resource types exist in the
// editor: a deny policy bound to a type blocks that type's route for the matching
// resource only, even though the role grants the permission.
func TestRouteGateAppliesTypedPolicies(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "g.db")+"?_pragma=foreign_keys(1)")
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
	if err := UpsertCatalog(database); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('u1', 'op', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r1', 'r1', '["containers.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur1', 'u1', 'r1')`,
		`UPDATE settings SET value = 'true' WHERE key = 'rbac_v3_enabled'`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	svc := NewService(database)
	policies := NewPolicyService(database)
	svc.SetPolicyService(policies)
	svc.SetAttributeBundle(NewAttributeBundle(nil, nil))
	svc.SetExecutor(NewABACExecutor(nil))
	svc.SetRolloutV3(NewRolloutV3(database))

	RegisterResourceType(ResourceTypeInfo{Type: "t_machine", Source: "t-gate", Routes: []ResourceRoute{{"/api/machines/{id}", "id"}}},
		func(_ context.Context, _, id string) (*ResourceAttributes, error) {
			return &ResourceAttributes{Type: "t_machine", ID: id, Name: id, Labels: map[string]string{"env": map[bool]string{true: "prod", false: "dev"}[id == "m-prod"]}}, nil
		})
	t.Cleanup(func() { UnregisterResourceTypes("t-gate") })
	if _, err := policies.Create(context.Background(), CreateInput{
		Name: "no prod machines", Effect: "deny", ResourceType: "t_machine", Enabled: true, Priority: 10,
		Expression: `hasLabel(resource.labels, "env") == "prod"`, Permissions: []string{"containers.manage"}, CreatedBy: "u1",
	}); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Route("/api/machines/{id}", func(r chi.Router) {
		r.With(RequirePermission(svc, Permission("containers.manage"))).Post("/stop", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	})
	call := func(id string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/machines/"+id+"/stop", nil)
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "op"}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	if got := call("m-dev"); got != http.StatusOK {
		t.Fatalf("a dev machine must pass on the role grant: %d", got)
	}
	if got := call("m-prod"); got != http.StatusForbidden {
		t.Fatalf("the typed deny policy must block the prod machine at the route: %d", got)
	}
}
