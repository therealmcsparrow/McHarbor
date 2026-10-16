// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// TestNodeTargetResolvesARawNameAndAPickedNamespacedID: a workflow saved with a bare name and
// a separate namespace field resolves as before, and the picker's namespace/name id fills both.
func TestNodeTargetResolvesARawNameAndAPickedNamespacedID(t *testing.T) {
	cases := []struct {
		name string
		cfg  map[string]any
		ns   string
		obj  string
	}{
		{"raw name with a namespace field", map[string]any{"name": "web", "namespace": "prod"}, "prod", "web"},
		{"picked id carries the namespace", map[string]any{"name": "staging/web", "namespace": "prod"}, "staging", "web"},
		{"picked id, no namespace field", map[string]any{"name": "staging/web"}, "staging", "web"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ns, name := nodeTarget(plugin.NodeExecInput{Config: c.cfg})
			if ns != c.ns || name != c.obj {
				t.Fatalf("nodeTarget = %q/%q, want %q/%q", ns, name, c.ns, c.obj)
			}
		})
	}
}

// TestNameFieldsArePickersThatStillTakeARawName: the deployment and pod fields list through the
// declared picker-sources and keep accepting a typed name.
func TestNameFieldsArePickersThatStillTakeARawName(t *testing.T) {
	seen := map[string]int{}
	for _, n := range (&KubernetesPlugin{}).kubernetesNodes() {
		for _, f := range n.desc.Fields {
			src, ok := f["source"].(map[string]any)
			if !ok {
				continue
			}
			seen[src["target"].(string)]++
			if f["type"] != "plugin-select" || f["allowCustom"] != true || src["plugin"] != "addon-kubernetes" {
				t.Errorf("%s field %v = %v", n.key, f["key"], f)
			}
		}
	}
	if seen["k8s_deployment"] != 2 || seen["k8s_pod"] != 2 {
		t.Fatalf("picker fields per target = %v, want two deployments and two pods", seen)
	}
}
