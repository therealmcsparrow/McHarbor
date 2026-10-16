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
)

// TestSelfEnvAccessNamesAdminEquivalentRoles is the warning half: a role holding
// containers.manage on the environment McHarbor runs on is admin-equivalent there, and the
// report names it. A role that also holds a global host_self.manage, a wildcard admin, and
// a role granted only on another environment are not flagged.
func TestSelfEnvAccessNamesAdminEquivalentRoles(t *testing.T) {
	db, _ := openTestDB(t)
	mustExec(t, db, `INSERT INTO environments (id, name) VALUES ('env-self', 'local'), ('env-remote', 'remote')`)
	mustExec(t, db, `INSERT INTO users (id, username, password_hash) VALUES ('u1','a','x'), ('u2','b','x'), ('u3','c','x'), ('u4','d','x')`)
	mustExec(t, db, `INSERT INTO groups (id, name) VALUES ('g1', 'ops-group')`)
	mustExec(t, db, `INSERT INTO roles (id, name, permissions) VALUES
		('r1', 'Deployers', '["containers.view","containers.manage"]'),
		('r2', 'Group Operators', '["containers.manage"]'),
		('r3', 'Platform', '["containers.manage","host_self.manage"]'),
		('r4', 'Everything', '["*"]'),
		('r5', 'Elsewhere', '["containers.manage"]'),
		('r6', 'Viewers', '["containers.view"]')`)
	mustExec(t, db, `INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES
		('a1', 'u1', 'r1', 'env-self'),
		('a3', 'u2', 'r3', NULL),
		('a4', 'u3', 'r4', NULL),
		('a5', 'u4', 'r5', 'env-remote'),
		('a6', 'u4', 'r6', NULL)`)
	mustExec(t, db, `INSERT INTO group_roles (id, group_id, role_id, environment_id) VALUES ('g-a', 'g1', 'r2', NULL)`)

	svc := NewService(db, Sources{
		SelfEnvironments: func(context.Context) ([]SelfEnvironment, error) {
			return []SelfEnvironment{{ID: "env-self", Name: "local"}}, nil
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := svc.Report(context.Background(), []string{SectionSelfEnvAccess}, false)
	if len(r.Sections) != 1 {
		t.Fatalf("sections = %d", len(r.Sections))
	}
	s := r.Sections[0]
	if s.Status != StatusWarn || !hasReason(s, "containersManageIsAdmin") {
		t.Fatalf("status %s reasons %v; want warn containersManageIsAdmin", s.Status, s.Reasons)
	}
	// The migrated database carries the preset roles too; only the fixture roles are asserted.
	unguarded := strings.Split(factValue(t, r, SectionSelfEnvAccess, "adminEquivalentRoles"), ", ")
	for _, want := range []string{"Deployers", "Group Operators"} {
		if !slices.Contains(unguarded, want) {
			t.Errorf("adminEquivalentRoles %q is missing %s", unguarded, want)
		}
	}
	for _, not := range []string{"Platform", "Everything", "Elsewhere", "Viewers"} {
		if slices.Contains(unguarded, not) {
			t.Errorf("adminEquivalentRoles %q lists %s", unguarded, not)
		}
	}
	holders := factValue(t, r, SectionSelfEnvAccess, "containersManageRoles")
	for _, want := range []string{"Deployers", "Group Operators", "Platform", "Everything"} {
		if !strings.Contains(holders, want) {
			t.Errorf("containersManageRoles %q is missing %s", holders, want)
		}
	}
	for _, not := range []string{"Elsewhere", "Viewers"} {
		if strings.Contains(holders, not) {
			t.Errorf("containersManageRoles %q lists %s, which holds no containers.manage on the self environment", holders, not)
		}
	}
	if got := factValue(t, r, SectionSelfEnvAccess, "selfEnvironments"); got != "local" {
		t.Errorf("selfEnvironments = %q", got)
	}
}

func TestSelfEnvAccessVerdicts(t *testing.T) {
	in := healthy()
	in.SelfAccessRoles = []string{"Platform"}
	if s := section(t, in, SectionSelfEnvAccess); s.Status != StatusPass {
		t.Errorf("only guarded holders: status %s, want pass", s.Status)
	}
	in.SelfEnvironments = nil
	if s := section(t, in, SectionSelfEnvAccess); s.Status != StatusInfo || !hasReason(s, "selfEnvironmentNotFound") {
		t.Errorf("no self environment: status %s reasons %v", s.Status, s.Reasons)
	}
	in = healthy()
	in.SelfAccessKnown = false
	if s := section(t, in, SectionSelfEnvAccess); s.Status != StatusUnknown {
		t.Errorf("unreadable source: status %s, want unknown", s.Status)
	}
}
