// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBackupLabelReachesTheLoaderJSON: the host's backup tables title a plugin source from
// labelKey in labelNs, so both must survive into the JSON the loader serves, while the
// runbook engine's own i18n_ns stays out of it.
func TestBackupLabelReachesTheLoaderJSON(t *testing.T) {
	caps, err := parseWith(t, `
    - kind: backup-row-actions
      source_kind: vm
      label_key: backups.sourceLabel
      i18n_ns: demo
      pages: {open: "/plugins/addon-demo", restore: "/plugins/addon-demo", edit_plan: "/plugins/addon-demo"}
      routes: {delete_run: "/api/demo/backups/{id}", delete_plan: "/api/demo/backup-plans/{id}"}
      permissions: {view: demo.view, manage: demo.backup.manage, delete: demo.backup.delete, restore: demo.restore.run}
`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(caps[0])
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, `"labelKey":"backups.sourceLabel"`) || !strings.Contains(got, `"labelNs":"demo"`) {
		t.Fatalf("backup label missing from the loader JSON: %s", got)
	}
}

// TestShippedBackupLabelsResolveInAllSixLocales: a declared label is looked up in the
// plugin's own bundle, so a key missing from any locale would print raw in the backup tables.
func TestShippedBackupLabelsResolveInAllSixLocales(t *testing.T) {
	root := filepath.Join("..", "..", "..", "plugins", "Plugins")
	dirs, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("plugin source tree not readable at %s: %v", root, err)
	}
	checked := 0
	for _, d := range dirs {
		raw, err := os.ReadFile(filepath.Join(root, d.Name(), "plugin.yaml"))
		if err != nil {
			continue
		}
		caps, err := Parse(d.Name(), raw)
		if err != nil {
			continue
		}
		for _, c := range caps {
			if c.Kind != KindBackupRowActions || c.LabelKey == "" {
				continue
			}
			for _, lang := range []string{"en", "nl", "de", "es", "fr", "pt"} {
				if bundleText(t, filepath.Join(root, d.Name(), "i18n", lang, c.LabelNS+".json"), c.LabelKey) == "" {
					t.Errorf("%s %s/%s.json has no %s", d.Name(), lang, c.LabelNS, c.LabelKey)
				}
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no shipped backup-row-actions declares a label; addon-kvm should")
	}
}

func bundleText(t *testing.T, path, key string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Errorf("%s: %v", path, err)
		return ""
	}
	for _, part := range strings.Split(key, ".") {
		next, _ := node.(map[string]any)
		node = next[part]
	}
	text, _ := node.(string)
	return strings.TrimSpace(text)
}
