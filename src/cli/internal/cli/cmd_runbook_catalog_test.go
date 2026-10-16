// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRunbookCatalogListsHostAndPluginEntries: both lists are flattened into one
// table, each row carries its step kind, and a plugin entry names its plugin and
// the plugin's own key — the command sends nothing but the one read.
func TestRunbookCatalogListsHostAndPluginEntries(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		ok(w, map[string]any{
			"actions": []map[string]any{
				{"key": "vm.start", "plugin": "addon-kvm", "target": "vm", "requiredPermissions": []string{"vm.manage"}, "destructive": true},
			},
			"diagnostics": []map[string]any{
				{"key": "container.state", "target": "container", "requiredPermissions": []string{"containers.view"}},
			},
			"targets": []map[string]string{{"kind": "vm", "labelKey": "runbook.targets.vm", "labelNs": "vm"}},
		})
	}))
	t.Cleanup(srv.Close)
	f := &fakeServer{Server: srv}

	code, out, stderr := run(t, f, goodKey, "-o", "json", "runbook", "catalog")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("output is not a JSON list: %v\n%s", err, out)
	}
	if len(rows) != 2 || rows[0]["key"] != "vm.start" || rows[0]["kind"] != "action" || rows[1]["kind"] != "diagnostic" {
		t.Fatalf("rows = %v", rows)
	}

	code, out, _ = run(t, f, goodKey, "runbook", "catalog")
	if code != ExitOK || !strings.Contains(out, "addon-kvm") || !strings.Contains(out, "vm.manage") {
		t.Fatalf("table must name the plugin and its key; exit %d:\n%s", code, out)
	}
	for _, p := range paths {
		if p != "GET /api/runbooks/catalog" {
			t.Fatalf("catalog made an unexpected request: %s", p)
		}
	}
}
