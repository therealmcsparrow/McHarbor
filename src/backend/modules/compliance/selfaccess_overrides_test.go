// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliance

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// TestSelfEnvAccessNamesDirectContainersManageGrants is T6.9: a user granted
// containers.manage on McHarbor's environment by a permission override, not a role, is
// just as admin-equivalent there and is named. A holder who also has host_self.manage
// (by global role or by global grant), an expired grant, a grant on another environment,
// and a deny are not flagged.
func TestSelfEnvAccessNamesDirectContainersManageGrants(t *testing.T) {
	db, _ := openTestDB(t)
	if err := rbac.UpsertCatalog(db); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO environments (id, name) VALUES ('env-self', 'local'), ('env-remote', 'remote')`)
	mustExec(t, db, `INSERT INTO users (id, username, password_hash) VALUES
		('u1','direct','x'), ('u2','global-direct','x'), ('u3','guarded-role','x'), ('u4','guarded-grant','x'),
		('u5','expired','x'), ('u6','elsewhere','x'), ('u7','denied','x'), ('u8','guarded-group','x')`)
	mustExec(t, db, `INSERT INTO roles (id, name, permissions) VALUES ('r-hs', 'HostSelf', '["host_self.manage"]')`)
	mustExec(t, db, `INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('a1', 'u3', 'r-hs', NULL)`)
	mustExec(t, db, `INSERT INTO groups (id, name) VALUES ('g1', 'platform')`)
	mustExec(t, db, `INSERT INTO group_members (id, group_id, user_id) VALUES ('m1', 'g1', 'u8')`)
	mustExec(t, db, `INSERT INTO group_roles (id, group_id, role_id, environment_id) VALUES ('ga', 'g1', 'r-hs', NULL)`)
	mustExec(t, db, `INSERT INTO user_permissions (id, user_id, permission_key, effect, environment_id, granted_by, expires_at) VALUES
		('p1', 'u1', 'containers.manage', 'grant', 'env-self', 'admin', NULL),
		('p2', 'u2', 'containers.manage', 'grant', NULL, 'admin', NULL),
		('p3', 'u3', 'containers.manage', 'grant', 'env-self', 'admin', NULL),
		('p4', 'u4', 'containers.manage', 'grant', 'env-self', 'admin', NULL),
		('p4h', 'u4', 'host_self.manage', 'grant', NULL, 'admin', NULL),
		('p5', 'u5', 'containers.manage', 'grant', 'env-self', 'admin', '2020-01-01T00:00:00Z'),
		('p6', 'u6', 'containers.manage', 'grant', 'env-remote', 'admin', NULL),
		('p7', 'u7', 'containers.manage', 'deny', 'env-self', 'admin', NULL),
		('p8', 'u8', 'containers.manage', 'grant', 'env-self', 'admin', NULL)`)

	svc := NewService(db, Sources{
		SelfEnvironments: func(context.Context) ([]SelfEnvironment, error) {
			return []SelfEnvironment{{ID: "env-self", Name: "local"}}, nil
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := svc.Report(context.Background(), []string{SectionSelfEnvAccess}, false)
	s := r.Sections[0]
	if s.Status != StatusWarn || !hasReason(s, "containersManageOverrideIsAdmin") {
		t.Fatalf("status %s reasons %v; want warn containersManageOverrideIsAdmin", s.Status, s.Reasons)
	}
	holders := strings.Split(factValue(t, r, SectionSelfEnvAccess, "containersManageUsers"), ", ")
	for _, want := range []string{"direct", "global-direct", "guarded-role", "guarded-grant", "guarded-group"} {
		if !slices.Contains(holders, want) {
			t.Errorf("containersManageUsers %q is missing %s", holders, want)
		}
	}
	for _, not := range []string{"expired", "elsewhere", "denied"} {
		if slices.Contains(holders, not) {
			t.Errorf("containersManageUsers %q lists %s", holders, not)
		}
	}
	unguarded := factValue(t, r, SectionSelfEnvAccess, "adminEquivalentUsers")
	if got := strings.Split(unguarded, ", "); !slices.Equal(got, []string{"direct", "global-direct"}) {
		t.Errorf("adminEquivalentUsers = %q; want direct, global-direct", unguarded)
	}
}
