// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const pickerTestManifest = `slug: addon-pickertest
adds_permissions:
  - key: "pickertest.view"
contributes:
  routes:
    - method: GET
      path: "/api/pickertest/things"
      requires_permission: "pickertest.view"
  nodes: ["thing-trigger"]
  capabilities:
    - kind: runbook-provider
      domain: thing
      i18n_ns: pickertest
      targets:
        - {kind: thing, label_key: runbook.targets.thing, path: "/api/pickertest/things", id: id, name: [name]}
    - kind: picker-source
      target: thing
`

// TestNodePickerSourceIsPinnedToTheRegisteringPlugin: a plugin-select field that names
// another plugin still lists through its own plugin, so no node can borrow a sibling's list.
func TestNodePickerSourceIsPinnedToTheRegisteringPlugin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(pickerTestManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Runtime{
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		nodeExecutors: map[string]plugin.NodeExecFunc{},
	}
	reg := &nodeRegistry{runtime: r, origin: "addon-pickertest", dir: dir}

	own := map[string]any{"key": "thing", "type": "plugin-select", "source": map[string]any{"target": "thing"}}
	foreign := map[string]any{"key": "other", "type": "plugin-select",
		"source": map[string]any{"plugin": "addon-other", "target": "vm"}}
	plain := map[string]any{"key": "note", "type": "text"}
	if err := reg.Add("thing-trigger", plugin.NodeDescriptor{Fields: []map[string]any{own, foreign, plain}}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	for _, field := range []map[string]any{own, foreign} {
		src, _ := field["source"].(map[string]any)
		if src["plugin"] != "addon-pickertest" {
			t.Errorf("field %v: want source.plugin pinned to addon-pickertest, got %v", field["key"], src["plugin"])
		}
	}
	if _, touched := plain["source"]; touched {
		t.Error("a field that is not plugin-select must be left alone")
	}
	if len(reg.capabilities()) == 0 {
		t.Error("the registry read no capabilities from the plugin's manifest")
	}
}
