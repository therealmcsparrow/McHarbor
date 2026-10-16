// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"context"
	"testing"
)

// TestCreateServicePrincipal pins the row a service principal is: no password
// hash to verify, a provider that no sign-in path accepts, the kind the RBAC
// resolver reads, and an ordinary role assignment the role editor shows.
func TestCreateServicePrincipal(t *testing.T) {
	ctx := context.Background()
	database, svc := openGovernanceDB(t)
	role := "role_viewer"
	u, err := svc.Create(ctx, CreateUserInput{Username: "Runbook-Bot", RoleID: &role, Kind: KindService})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.Kind != KindService {
		t.Fatalf("created kind %q, want service", u.Kind)
	}
	var hash, provider, kind string
	if err := database.QueryRow(`SELECT password_hash, auth_provider, principal_kind FROM users WHERE id = ?`, u.ID).
		Scan(&hash, &provider, &kind); err != nil {
		t.Fatal(err)
	}
	if hash != "" || provider != "service" || kind != "service" {
		t.Fatalf("service principal row: hash %q provider %q kind %q", hash, provider, kind)
	}
	roles, err := svc.ListRoles(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 1 || roles[0].RoleID != role {
		t.Fatalf("want the one role assignment the role editor shows, got %+v", roles)
	}
	if err := svc.AdminSetPassword(ctx, u.ID, "a long enough passphrase 123!"); err == nil {
		t.Fatal("an admin set a password on a service principal")
	}
	if err := svc.Disable(ctx, u.ID, "a1", "retired"); err != nil {
		t.Fatalf("an admin must be able to disable a service principal: %v", err)
	}

	listed, _, err := svc.List(ctx, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range listed {
		want := KindUser
		if item.ID == u.ID {
			want = KindService
		}
		if item.Kind != want {
			t.Fatalf("user %s lists as kind %q, want %q", item.Username, item.Kind, want)
		}
	}
}
