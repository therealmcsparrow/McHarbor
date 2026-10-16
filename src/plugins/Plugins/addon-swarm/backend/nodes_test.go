// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "testing"

// TestIDFieldsArePickersThatStillTakeARawID: the service and node fields list through the
// declared picker-sources and keep accepting a typed id or name, so a workflow saved when they were
// plain text resolves as before.
func TestIDFieldsArePickersThatStillTakeARawID(t *testing.T) {
	want := map[string]string{"service": "swarm_service", "node": "swarm_node"}
	seen := 0
	for _, n := range (&SwarmPlugin{}).swarmNodes() {
		for _, f := range n.desc.Fields {
			key, _ := f["key"].(string)
			target, ok := want[key]
			if !ok {
				continue
			}
			seen++
			src, _ := f["source"].(map[string]any)
			if f["type"] != "plugin-select" || f["allowCustom"] != true || src["target"] != target || src["plugin"] != "addon-swarm" {
				t.Errorf("%s field %s = %v", n.key, key, f)
			}
		}
	}
	if seen != 3 {
		t.Fatalf("saw %d id fields, want 3 (two service nodes, one node)", seen)
	}
	cfg := map[string]any{"service": "web_api", "node": "n0abc"}
	if nodeString(cfg, "service", "") != "web_api" || nodeString(cfg, "node", "") != "n0abc" {
		t.Fatal("a stored raw id no longer resolves")
	}
}
