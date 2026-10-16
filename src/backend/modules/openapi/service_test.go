// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package openapi

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
)

// TestPluginOperationsCarryDeclaredGates proves the live document shows each
// plugin route with the permission and scope its plugin declared, and exempts
// an undeclared mutating route by name rather than leaving it silently ungated.
func TestPluginOperationsCarryDeclaredGates(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "openapi.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing the test database: %v", err)
		}
	})
	if _, err := db.Exec(`
CREATE TABLE plugin_routes (id TEXT, origin TEXT, method TEXT, path TEXT, requires_permission TEXT, is_public INTEGER);
CREATE TABLE plugin_permissions (origin TEXT, permission_key TEXT, scope TEXT);
INSERT INTO plugin_routes VALUES
  ('1', 'addon-kvm', 'GET',  '/api/vms',            'vm.view',   0),
  ('2', 'addon-kvm', 'POST', '/api/vms/{id}/start', 'vm.manage', 0),
  ('3', 'comm-hook', 'POST', '/hooks/in',           '',          1);
INSERT INTO plugin_permissions VALUES
  ('addon-kvm', 'vm.view', 'env'),
  ('addon-kvm', 'vm.manage', 'env');`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ops, err := NewService(db).PluginOperations(context.Background())
	if err != nil {
		t.Fatalf("PluginOperations: %v", err)
	}
	byKey := map[string]apispec.Operation{}
	for _, op := range ops {
		byKey[op.Method+" "+op.Path] = op
	}

	start, ok := byKey["POST /api/vms/{id}/start"]
	if !ok || len(start.Perms) != 1 || string(start.Perms[0].Keys[0]) != "vm.manage" || string(start.Perms[0].Scope) != "environment" {
		t.Errorf("start route = %+v, want vm.manage at environment scope", start)
	}
	hook, ok := byKey["POST /api/hooks/in"]
	if !ok {
		t.Fatalf("relative plugin path was not prefixed with /api: %v", byKey)
	}
	if hook.Group != apispec.GroupPublic || hook.Exempt == "" {
		t.Errorf("public undeclared route = %+v, want public band with a stated exemption", hook)
	}

	reg := apispec.NewRegistry()
	for _, op := range ops {
		reg.Add(op)
	}
	doc := apispec.Document(reg, "test")
	paths := doc["paths"].(map[string]any)
	if _, ok := paths["/api/vms/{id}/start"]; !ok {
		t.Errorf("merged document lacks the plugin path")
	}
}
