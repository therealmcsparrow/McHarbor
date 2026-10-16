// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

type recordingNodes struct {
	added map[string]plugin.NodeDescriptor
}

func (r *recordingNodes) Add(key string, d plugin.NodeDescriptor) error {
	r.added[key] = d
	return nil
}

func (r *recordingNodes) Remove(string) error { return nil }

// TestContainerFieldIsAPickerThatStillTakesARawID: the id field is a plugin-select over the
// declared `ct` picker-source, and it keeps accepting a typed id, so a workflow saved when the
// field was plain text resolves exactly as it did.
func TestContainerFieldIsAPickerThatStillTakesARawID(t *testing.T) {
	reg := &recordingNodes{added: map[string]plugin.NodeDescriptor{}}
	if err := (&LXCPlugin{}).registerNodes(reg); err != nil {
		t.Fatal(err)
	}
	if len(reg.added) == 0 {
		t.Fatal("no nodes registered")
	}
	for key, d := range reg.added {
		var field map[string]any
		for _, f := range d.Fields {
			if f["key"] == "containerId" {
				field = f
			}
		}
		if field == nil {
			t.Fatalf("%s has no containerId field", key)
		}
		src, _ := field["source"].(map[string]any)
		if field["type"] != "plugin-select" || field["allowCustom"] != true || src["target"] != "ct" || src["plugin"] != "addon-lxc" {
			t.Errorf("%s containerId field = %v", key, field)
		}
	}
	in := plugin.NodeExecInput{Config: map[string]any{"containerId": "ct-0042"}}
	if got := resolveContainerID(in); got != "ct-0042" {
		t.Fatalf("a stored raw id resolved to %q", got)
	}
}
