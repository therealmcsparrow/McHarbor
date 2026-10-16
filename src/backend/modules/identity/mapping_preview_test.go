// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

func TestMappingDecisionsLastMappingWins(t *testing.T) {
	mappings := []GroupMapping{
		{ProviderGroup: "devs", McharborGroupID: "g-dev"},
		{ProviderGroup: "ops", McharborGroupID: "g-ops"},
		{ProviderGroup: "contractors", McharborGroupID: "g-dev"},
		{ProviderGroup: "ignored", McharborGroupID: ""},
	}
	order, decisions := mappingDecisions(mappings, []string{"devs", "ops"})
	if !reflect.DeepEqual(order, []string{"g-dev", "g-ops"}) {
		t.Fatalf("order %v", order)
	}
	if decisions["g-dev"] || !decisions["g-ops"] {
		t.Fatalf("the later contractors mapping must decide g-dev (absent -> removed): %v", decisions)
	}
}

func TestSimulateLoginWithoutClaimsChangesNothing(t *testing.T) {
	current := []membership{{GroupID: "g-dev", Source: "manual"}}
	cfg := MappingConfig{GroupMappingEnabled: true, AutoImportGroups: true, GroupMappings: []GroupMapping{{ProviderGroup: "x", McharborGroupID: "g-dev"}}}
	got := simulateLogin(current, cfg, nil, "saml", nil)
	if !reflect.DeepEqual(got, map[string]bool{"g-dev": true}) {
		t.Fatalf("a login with no groups must leave memberships alone, got %v", got)
	}
}

func TestSimulateLoginAutoImportDropsOnlyItsOwnSource(t *testing.T) {
	current := []membership{{GroupID: "g-old", Source: "saml"}, {GroupID: "g-manual", Source: "manual"}}
	cfg := MappingConfig{AutoImportGroups: true}
	got := simulateLogin(current, cfg, []string{"Platform", "Brand New"}, "saml", map[string]string{"Platform": "g-platform"})
	want := map[string]bool{"g-manual": true, "g-platform": true, newGroupPrefix + "Brand New": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func openIdentityTestDB(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	database, err := coredb.OpenSQLite(filepath.Join(dir, "identity_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	enc, err := encryption.New(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	return NewService(database, enc, nil, nil), database
}

func mustExec(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func membershipSet(t *testing.T, database *sql.DB, userID string) map[string]bool {
	t.Helper()
	rows, err := database.QueryContext(context.Background(), `SELECT group_id FROM group_members WHERE user_id = ?`, userID)
	if err != nil {
		t.Fatal(err)
	}
	defer closeRows(rows)
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func seedMappingFixtures(t *testing.T, database *sql.DB) {
	t.Helper()
	mustExec(t, database, `INSERT INTO roles (id, name, permissions) VALUES ('r-view', 'Test Viewer', '["containers.view"]'), ('r-ops', 'Test Operator', '["containers.manage"]'), ('r-def', 'Test Default', '[]')`)
	mustExec(t, database, `INSERT INTO groups (id, name) VALUES ('g-dev', 'T Developers'), ('g-ops', 'T Operations'), ('g-manual', 'T Manual')`)
	mustExec(t, database, `INSERT INTO group_roles (id, group_id, role_id) VALUES ('gr-1', 'g-dev', 'r-view'), ('gr-2', 'g-ops', 'r-ops')`)
	mustExec(t, database, `INSERT INTO identity_providers (id, name, provider_type, enabled, client_id, client_secret, group_mapping_enabled, group_mappings)
		VALUES ('p-1', 'Corp', 'saml_2_0', 1, '', '', 1, '[{"providerGroup":"devs","mcharborGroupId":"g-dev"}]')`)
	for _, u := range []string{"u-ann", "u-ben", "u-cid"} {
		mustExec(t, database, `INSERT INTO users (id, username, password_hash, identity_provider_id) VALUES (?, ?, 'x', 'p-1')`, u, u[2:])
	}
}

func TestApplyGroupMappingsMatchesTheSimulation(t *testing.T) {
	svc, database := openIdentityTestDB(t)
	seedMappingFixtures(t, database)
	mustExec(t, database, `INSERT INTO group_members (id, group_id, user_id) VALUES ('gm-1', 'g-ops', 'u-ann'), ('gm-2', 'g-manual', 'u-ann')`)
	mappings := []GroupMapping{{ProviderGroup: "devs", McharborGroupID: "g-dev"}, {ProviderGroup: "ops", McharborGroupID: "g-ops"}, {ProviderGroup: "ops2", McharborGroupID: "g-dev"}}
	claims := []string{"devs"}

	members, _, err := svc.userMembershipsAndDirect(context.Background(), "u-ann")
	if err != nil {
		t.Fatal(err)
	}
	predicted := simulateLogin(members, MappingConfig{GroupMappingEnabled: true, GroupMappings: mappings}, claims, "saml_2_0", nil)
	svc.applyGroupMappings(context.Background(), "u-ann", `[{"providerGroup":"devs","mcharborGroupId":"g-dev"},{"providerGroup":"ops","mcharborGroupId":"g-ops"},{"providerGroup":"ops2","mcharborGroupId":"g-dev"}]`, claims)
	if got := membershipSet(t, database, "u-ann"); !reflect.DeepEqual(got, predicted) {
		t.Fatalf("login wrote %v but the preview predicted %v", got, predicted)
	}
}

func TestPreviewMappingSampleAndImpact(t *testing.T) {
	svc, database := openIdentityTestDB(t)
	seedMappingFixtures(t, database)
	ctx := context.Background()
	mustExec(t, database, `INSERT INTO group_members (id, group_id, user_id) VALUES ('gm-1', 'g-dev', 'u-ann')`)
	mustExec(t, database, `INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur-1', 'u-ben', 'r-ops')`)
	svc.recordGroupClaims(context.Background(), "p-1", "u-ann", []string{"devs"})
	svc.recordGroupClaims(context.Background(), "p-1", "u-ben", []string{"ops", "devs"})
	svc.recordGroupClaims(context.Background(), "p-1", "u-ben", []string{"ops"})

	proposed := MappingConfig{
		GroupMappingEnabled: true,
		GroupMappings:       []GroupMapping{{ProviderGroup: "ops", McharborGroupID: "g-ops"}, {ProviderGroup: "contractors", McharborGroupID: "g-dev"}},
		AutoProvision:       true,
		DefaultRoleID:       "r-def",
	}
	preview, err := svc.PreviewMapping(ctx, MappingPreviewInput{ProviderID: "p-1", Config: proposed, SampleGroups: []string{"ops", "sales", " ops "}})
	if err != nil {
		t.Fatalf("PreviewMapping: %v", err)
	}

	if len(preview.Sample.Groups) != 1 || preview.Sample.Groups[0].GroupID != "g-ops" || !reflect.DeepEqual(preview.Sample.Groups[0].From, []string{"ops"}) {
		t.Fatalf("sample groups %+v", preview.Sample.Groups)
	}
	if !reflect.DeepEqual(preview.Sample.Unmapped, []string{"sales"}) {
		t.Fatalf("unmapped %v", preview.Sample.Unmapped)
	}
	var sampleRoles []string
	for _, r := range preview.Sample.Roles {
		sampleRoles = append(sampleRoles, r.RoleName)
	}
	sort.Strings(sampleRoles)
	if !reflect.DeepEqual(sampleRoles, []string{"Test Default", "Test Operator"}) {
		t.Fatalf("sample roles %v", sampleRoles)
	}

	impact := preview.Impact
	if impact == nil || impact.Evaluated != 2 || len(impact.Unknown) != 1 || impact.Unknown[0].UserID != "u-cid" {
		t.Fatalf("impact counts %+v", impact)
	}
	if len(impact.Users) != 2 {
		t.Fatalf("ann and ben both change, got %+v", impact.Users)
	}
	ann, ben := impact.Users[0], impact.Users[1]
	if ann.UserID != "u-ann" || len(ann.Lost) != 1 || ann.Lost[0].RoleName != "Test Viewer" || len(ann.Gained) != 0 || len(ann.Left) != 1 || ann.Left[0].GroupID != "g-dev" {
		t.Fatalf("ann should leave Developers and lose Viewer, got %+v", ann)
	}
	if ben.UserID != "u-ben" || len(ben.Joined) != 1 || ben.Joined[0].GroupID != "g-ops" || len(ben.Gained) != 0 {
		t.Fatalf("ben joins Operations but already holds Operator directly, so gains no role: %+v", ben)
	}
	if impact.Unchanged != 0 {
		t.Fatalf("nobody evaluated is unchanged, got %d", impact.Unchanged)
	}
}

func TestPreviewMappingRemovingAMappingKeepsMemberships(t *testing.T) {
	svc, database := openIdentityTestDB(t)
	seedMappingFixtures(t, database)
	mustExec(t, database, `INSERT INTO group_members (id, group_id, user_id) VALUES ('gm-1', 'g-dev', 'u-ann')`)
	svc.recordGroupClaims(context.Background(), "p-1", "u-ann", []string{"devs"})
	preview, err := svc.PreviewMapping(context.Background(), MappingPreviewInput{ProviderID: "p-1", Config: MappingConfig{GroupMappingEnabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Impact.Users) != 0 || preview.Impact.Unchanged != 1 {
		t.Fatalf("an unmapped group is no longer managed, so sign-in leaves the membership alone: %+v", preview.Impact)
	}
}
