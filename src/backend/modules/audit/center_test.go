// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

func openCenterDB(t *testing.T) (*sql.DB, *coreAudit.Logger, *Service) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "center.db"))
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := database.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	logger := coreAudit.NewLogger(database, "secret")
	svc := NewService(database, enc, slog.New(slog.NewTextHandler(io.Discard, nil)), "secret")
	return database, logger, svc
}

func TestParseDenialDetailsReadsBothHookFormats(t *testing.T) {
	rbacRow := parseDenialDetails("permission=containers.manage env=env1 path=/api/containers/abc/stop")
	if rbacRow.Permission != "containers.manage" || rbacRow.Environment != "env1" || rbacRow.Route != "/api/containers/abc/stop" {
		t.Fatalf("rbac details: %+v", rbacRow)
	}
	abacRow := parseDenialDetails("policy_id=pol1 perm=stacks.delete resource_type=stack resource_id=s1 user_id=u1")
	if abacRow.Permission != "stacks.delete" || abacRow.PolicyID != "pol1" || abacRow.UserID != "u1" || abacRow.ResourceID != "s1" {
		t.Fatalf("abac details: %+v", abacRow)
	}
}

func TestListDenialsFiltersByPermissionAndActor(t *testing.T) {
	database, logger, svc := openCenterDB(t)
	ctx := context.Background()
	if _, err := database.Exec(`INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES ('u1', 'alice', 'x', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	write := func(userID, username string, e coreAudit.Entry) {
		t.Helper()
		if err := logger.Record(userID, username, e); err != nil {
			t.Fatal(err)
		}
	}
	write("u1", "alice", coreAudit.Entry{Action: actionPermissionDenied, EntityType: "user", EntityID: "u1", Details: "permission=stacks.manage env=e1 path=/api/stacks"})
	write("u2", "bob", coreAudit.Entry{Action: actionPermissionDenied, EntityType: "user", EntityID: "u2", Details: "permission=stacks.manage_all env= path=/api/x"})
	write("", "", coreAudit.Entry{Action: actionABACDenied, EntityType: "policy", EntityID: "pol1", Details: "policy_id=pol1 perm=stacks.manage resource_type=stack resource_id=s1 user_id=u1"})
	write("u1", "alice", coreAudit.Entry{Action: "stack.created", EntityType: "stack"})

	rows, total, err := svc.ListDenials(ctx, listFilter{Page: 1, PerPage: 25}, "stacks.manage")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(rows) != 2 {
		t.Fatalf("permission filter must match exactly the key, not a prefix: total=%d rows=%+v", total, rows)
	}
	for _, row := range rows {
		if row.UserID != "u1" || row.Username != "alice" || row.Permission != "stacks.manage" {
			t.Fatalf("denial row: %+v", row)
		}
	}

	filter := listFilter{Page: 1, PerPage: 25, Actor: "ALICE", ActorIDs: svc.resolveActorIDs(ctx, "ALICE")}
	_, total, err = svc.ListDenials(ctx, filter, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("actor filter must match the RBAC row by username and the ABAC row by details user_id: %d", total)
	}
}

func TestListSeverityAndActorFilters(t *testing.T) {
	_, logger, svc := openCenterDB(t)
	for _, e := range []struct{ user, sev string }{{"alice", "info"}, {"alice", "error"}, {"bob", "error"}, {"bob", "warning"}} {
		if err := logger.Record("", e.user, coreAudit.Entry{Action: "thing.done", Severity: e.sev, EntityType: "thing"}); err != nil {
			t.Fatal(err)
		}
	}
	_, total, err := svc.List(context.Background(), listFilter{Page: 1, PerPage: 25, Severities: []string{"error", "warning"}, Actor: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("severity+actor: want 2, got %d", total)
	}
	if _, ok := parseSeverities([]string{"info,critical"}); ok {
		t.Fatal("a severity that is never stored must be rejected, not silently match nothing")
	}
}

func TestExportFieldSelectionIsSigned(t *testing.T) {
	_, logger, svc := openCenterDB(t)
	if err := logger.Record("u1", "alice", coreAudit.Entry{Action: "thing.done", EntityType: "thing", Details: "secret-ish"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	fields, ok := parseExportFields([]string{"action,id", "username"})
	if !ok || strings.Join(fields, ",") != "id,username,action" {
		t.Fatalf("fields must be validated and put in canonical order: %v %t", fields, ok)
	}
	if _, ok := parseExportFields([]string{"password"}); ok {
		t.Fatal("an unknown field must be rejected")
	}

	payload, meta, err := svc.Export(ctx, listFilter{Fields: fields}, "json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Metadata ExportMetadata   `json:"metadata"`
		Entries  []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Entries) != 1 || len(doc.Entries[0]) != 3 || doc.Entries[0]["details"] != nil {
		t.Fatalf("entries must carry only the selected fields: %+v", doc.Entries)
	}
	if strings.Join(meta.Manifest.Columns, ",") != "id,username,action" {
		t.Fatalf("manifest columns: %v", meta.Manifest.Columns)
	}

	_, full, err := svc.Export(ctx, listFilter{}, "json")
	if err != nil {
		t.Fatal(err)
	}
	tampered := meta
	tampered.Manifest.Columns = full.Manifest.Columns
	if svc.signExportMetadata(tampered) == meta.Signature {
		t.Fatal("the signature must cover the column selection")
	}
	narrowed := meta
	narrowed.Filters.Severities = []string{"error"}
	if svc.signExportMetadata(narrowed) == meta.Signature {
		t.Fatal("the signature must cover the severity filter")
	}

	csvPayload, _, err := svc.Export(ctx, listFilter{Fields: fields}, "csv")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(csvPayload), "\nid,username,action\n") {
		t.Fatalf("csv header must be the selection: %s", csvPayload)
	}
}

func TestVerifyCrossesRetentionGapsOnlyThroughTombstones(t *testing.T) {
	database, logger, svc := openCenterDB(t)
	ctx := context.Background()
	for _, sev := range []string{"info", "warning", "info", "error", "info"} {
		if err := logger.Record("", "admin", coreAudit.Entry{Action: "thing.done", Severity: sev, EntityType: "thing"}); err != nil {
			t.Fatal(err)
		}
	}
	policy := coreAudit.RetentionPolicy{DefaultDays: 0, BySeverity: map[string]int{"info": 1}}
	if _, err := coreAudit.PruneWithPolicy(ctx, database, "secret", policy, time.Now().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	result, err := svc.VerifyHashChain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || result.PrunedGaps != 2 || result.PrunedRows != 2 || result.Tombstones != 3 {
		t.Fatalf("a pruned chain must verify and report its gaps: %+v", result)
	}

	if _, err := database.Exec("DELETE FROM audit_chain_tombstones WHERE entry_hash = ''"); err != nil {
		t.Fatal(err)
	}
	result, err = svc.VerifyHashChain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verified || result.Reason != "previous hash mismatch" {
		t.Fatalf("a gap with no tombstone must fail verification: %+v", result)
	}
}

func TestAITrailGroupsStepsAndRedacts(t *testing.T) {
	database, logger, svc := openCenterDB(t)
	ctx := context.Background()
	if _, err := database.Exec(`INSERT INTO ai_assistant_action_log (id, conversation_id, user_id, tool, args_json, plan_json, result_json, executed_at, status, error)
		VALUES ('act1', 'conv1', 'u1', 'restart_container', '{"password":"hunter2","name":"web"}', '{"steps":["restart web"]}', '{"token":"abc123"}', '2026-09-01T10:00:00Z', 'executed', '')`); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"ai.action.approved", "ai.action.executed"} {
		if err := logger.Record("u1", "alice", coreAudit.Entry{Action: action, EntityType: aiActionEntityType, EntityID: "act1", EntityName: "restart_container", Details: "status=ok"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := logger.Record("u1", "alice", coreAudit.Entry{Action: "ai.action.denied", EntityType: aiActionEntityType, EntityID: "act2", EntityName: "delete_volume"}); err != nil {
		t.Fatal(err)
	}

	groups, total, err := svc.ListAIActions(ctx, aiTrailFilter{Page: 1, PerPage: 25})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(groups) != 2 {
		t.Fatalf("want 2 action groups, got %d: %+v", total, groups)
	}
	var executed AITrailGroup
	for _, g := range groups {
		if g.ActionID == "act1" {
			executed = g
		}
		if g.ActionID == "act2" && (!g.RecordMissing || g.Status != "denied" || g.Tool != "delete_volume") {
			t.Fatalf("an action known only from the audit log: %+v", g)
		}
	}
	kinds := []string{}
	for _, step := range executed.Steps {
		kinds = append(kinds, step.Kind)
	}
	if strings.Join(kinds, ",") != "preview,approved,executed" {
		t.Fatalf("steps: %v", kinds)
	}
	if strings.Contains(executed.Arguments, "hunter2") || strings.Contains(executed.Result, "abc123") {
		t.Fatalf("payloads must be redacted: args=%s result=%s", executed.Arguments, executed.Result)
	}
	if !strings.Contains(executed.Arguments, "web") {
		t.Fatalf("redaction must keep non-secret values: %s", executed.Arguments)
	}
}
